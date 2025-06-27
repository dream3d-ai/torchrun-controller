package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// JobManager handles Kubernetes Job creation and management
type JobManager struct {
	client client.Client
}

// NewJobManager creates a new job manager
func NewJobManager(client client.Client) *JobManager {
	return &JobManager{
		client: client,
	}
}

// CreateJob creates the Kubernetes Job for training
func (jm *JobManager) CreateJob(ctx context.Context, job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) error {
	log := log.FromContext(ctx)

	// Parse the pod template config
	var podSpec corev1.PodSpec
	if err := json.Unmarshal(jq.Spec.PodTemplateConfig.Spec.Raw, &podSpec); err != nil {
		return err
	}

	// Validate the pod spec
	if err := jm.validatePodSpec(podSpec); err != nil {
		return err
	}

	// Set scheduler name
	podSpec.SchedulerName = "kai-scheduler"

	// Set restart policy
	podSpec.RestartPolicy = corev1.RestartPolicy(job.Spec.Reliability.RestartPolicy)

	// Attach the workspace to the trainer container
	jm.attachWorkspaceToTrainer(job, &podSpec)

	// Build trainer command
	jm.attachTrainerCommand(job, jq, &podSpec)

	// Extend the environment variables
	jm.attachEnvironment(job, jq, &podSpec)

	// Build additional volumes and mounts
	jm.attachVolumes(job, jq, &podSpec)

	// Calculate parallelism - each node is a single pod
	parallelism := int32(job.Spec.NumNodes)

	// Create job object
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels: map[string]string{
				"app":                     "torchrun",
				"torchrun.ai/job-id":      job.Status.JobID,
				"torchrun.ai/job-name":    job.Status.JobName,
				"torchrun.ai/job-queue":   job.Spec.JobQueue,
				"scheduling.kai.io/queue": jq.Spec.Queue.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, job.GroupVersionKind()),
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism:             &parallelism,
			Completions:             &parallelism,
			BackoffLimit:            &job.Spec.Reliability.MaxRestarts,
			TTLSecondsAfterFinished: job.Spec.Reliability.TTLSecondsAfterFinished,
			ActiveDeadlineSeconds:   job.Spec.Reliability.ActiveDeadlineSeconds,
			Suspend:                 &job.Spec.Suspend,
			CompletionMode:          completionModePtr(batchv1.IndexedCompletion),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      jm.buildPodLabels(job, jq),
					Annotations: jm.buildPodAnnotations(job, jq),
				},
				Spec: podSpec,
			},
		},
	}

	// Check if job already exists
	existingJob := &batchv1.Job{}
	err := jm.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob)
	if err == nil {
		// Job exists, update if needed
		log.Info("Job already exists", "name", job.Name)
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	// Create the job
	log.Info("Creating Job", "name", job.Name)
	return jm.client.Create(ctx, k8sJob)
}

// buildTorchrunCommand builds the torchrun command
func (jm *JobManager) buildTorchrunCommand(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) string {
	var cmdParts []string

	// Add setup command if provided
	if job.Spec.SetupCommand != "" {
		cmdParts = append(cmdParts, job.Spec.SetupCommand, "&&")
	}

	// Build torchrun command
	cmdParts = append(cmdParts, "torchrun")

	// Lookup nproc (num gpus) from resource requests nvidia.com/gpu on the pod spec
	// it will be on the "trainer" container
	nproc := 0
	for _, container := range podSpec.Containers {
		if container.Name == "trainer" {
			if val, ok := container.Resources.Requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
				nproc = int(val.Value())
			}
		}
	}

	// Node configuration
	if job.Spec.NumNodes > 1 {
		cmdParts = append(cmdParts,
			"--node_rank", "$(JOB_COMPLETION_INDEX)",
			"--nnodes", strconv.Itoa(job.Spec.NumNodes),
			"--nproc_per_node", strconv.Itoa(nproc),
			"--rdzv_backend", job.Spec.Distributed.RdzvBackend,
			"--rdzv_endpoint", job.Spec.Distributed.RdzvEndpoint,
			"--rdzv_id", job.Status.JobID,
			"--no-python",
		)
	} else {
		// Single node training
		cmdParts = append(cmdParts,
			"--standalone",
			"--nproc_per_node", strconv.Itoa(nproc),
			"--rdzv_backend", job.Spec.Distributed.RdzvBackend,
			"--rdzv_endpoint", job.Spec.Distributed.RdzvEndpoint,
			"--rdzv_id", job.Status.JobID,
			"--no-python",
		)
	}

	// Add the actual command
	cmdParts = append(cmdParts, job.Spec.Command)

	return strings.Join(cmdParts, " ")
}

// attachTrainerCommand builds the torchrun command and attaches it to the trainer container
func (jm *JobManager) attachTrainerCommand(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) {
	var cmdParts []string

	// Add the setup command if provided
	if job.Spec.SetupCommand != "" {
		cmdParts = append(cmdParts, job.Spec.SetupCommand, "&&")
	}

	// Add the torchrun command
	cmdParts = append(cmdParts, jm.buildTorchrunCommand(job, jq, podSpec))

	podSpec.Containers[0].Command = []string{"/bin/bash", "-c", strings.Join(cmdParts, " ")}
}

// attachEnvironment attaches the environment variables to the trainer container
func (jm *JobManager) attachEnvironment(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) {
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, job.Spec.Env...)
}

// attachVolumes attaches additional volumes and mounts to the pod spec
func (jm *JobManager) attachVolumes(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) {
	// Add additional volumes from job
	if job.Spec.Volumes != nil && job.Spec.Volumes.AdditionalVolumes != nil {
		podSpec.Volumes = append(podSpec.Volumes, job.Spec.Volumes.AdditionalVolumes...)
	}

	// Add additional mounts from job
	if job.Spec.Volumes != nil && job.Spec.Volumes.AdditionalMounts != nil {
		for _, mount := range job.Spec.Volumes.AdditionalMounts {
			podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      mount.Name,
				MountPath: mount.MountPath,
				SubPath:   mount.SubPath,
				ReadOnly:  mount.ReadOnly,
			})
		}
	}
}

// attachWorkspaceToTrainer attaches the workspace to the trainer container
func (jm *JobManager) attachWorkspaceToTrainer(job *torchrunv1alpha1.TorchrunJob, podSpec *corev1.PodSpec) {
	// Attach the workspace pvc to the init container to copy files to the workspace volume
	podSpec.InitContainers = append(podSpec.InitContainers, corev1.Container{
		Name:            "workspace-sync",
		Image:           "alpine:3.18",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/bash", "-c"},
		Args: []string{
			fmt.Sprintf("while [ ! -f /workspace-pvc/.sync_success ]; do echo 'Waiting for workspace sync...'; sleep 5; done; cp -r /workspace-pvc/* %s", job.Spec.WorkspaceStorage.MountPath),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "workspace-pvc",
				MountPath: "/workspace-pvc",
				ReadOnly:  true,
			},
			{
				Name:      "workspace",
				MountPath: job.Spec.WorkspaceStorage.MountPath,
			},
		},
	})

	// Attach the workspace volume to the trainer container
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      "workspace",
		MountPath: job.Spec.WorkspaceStorage.MountPath,
	})

	// Workspace PVC
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "workspace-pvc",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-workspace", job.Name),
			},
		},
	})

	// Local workspace volume (emptyDir) where init container copies files
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "workspace",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

// validatePodSpec validates the pod specification
func (jm *JobManager) validatePodSpec(podSpec corev1.PodSpec) error {
	// Check if the pod spec has a container named "trainer"
	if len(podSpec.Containers) == 0 {
		return fmt.Errorf("pod spec must have at least one container")
	}

	// The first container must be named "trainer"
	if podSpec.Containers[0].Name != "trainer" {
		return fmt.Errorf("pod spec must have a container named 'trainer'")
	}

	return nil
}

// buildPodLabels builds the pod labels
func (jm *JobManager) buildPodLabels(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) map[string]string {
	labels := map[string]string{
		"app":                     "torchrun",
		"torchrun.ai/job-name":    job.Name,
		"torchrun.ai/job-queue":   job.Spec.JobQueue,
		"scheduling.kai.io/queue": jq.Spec.Queue.Name,
	}

	// Add user-specified labels
	for k, v := range job.Spec.Labels {
		labels[k] = v
	}

	return labels
}

// buildPodAnnotations builds the pod annotations
func (jm *JobManager) buildPodAnnotations(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) map[string]string {
	annotations := map[string]string{
		"torchrun.ai/job-id": job.Status.JobID,
	}

	// Add user-specified annotations
	for k, v := range job.Spec.Annotations {
		annotations[k] = v
	}

	return annotations
}

// completionModePtr returns a pointer to a completion mode
func completionModePtr(mode batchv1.CompletionMode) *batchv1.CompletionMode {
	return &mode
}
