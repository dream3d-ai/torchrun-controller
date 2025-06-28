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
// The pod template from the TorchrunQueue must contain a container named "trainer" as the first container.
// This is a reserved container name where:
// - The torchrun command will be executed
// - The workspace will be mounted
// - Environment variables will be injected
// - The main training workload will run
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

	// Translate resource names in volumes based on TorchrunQueue resources
	if err := jm.translateResourceNames(&podSpec, jq); err != nil {
		return err
	}

	// Set scheduler name
	podSpec.SchedulerName = "kai-scheduler"

	// Set restart policy
	podSpec.RestartPolicy = corev1.RestartPolicy(job.Spec.Reliability.RestartPolicy)

	// Attach the workspace to the trainer container
	jm.attachWorkspaceToTrainer(job, jq, &podSpec)

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
				"app":                   "torchrun",
				"torchrun.ai/job-id":    job.Spec.JobID,
				"torchrun.ai/job-name":  job.Spec.JobName,
				"torchrun.ai/job-queue": job.Spec.Queue,
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

// attachTrainerCommand builds the torchrun command and attaches it to the trainer container
func (jm *JobManager) attachTrainerCommand(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) {
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

	// if RdzvBackend is empty set it to c10d
	if jq.Spec.Distributed.RdzvBackend == "" {
		jq.Spec.Distributed.RdzvBackend = "c10d"
	}

	// Node configuration
	if job.Spec.NumNodes > 1 {
		cmdParts = append(cmdParts,
			"--node_rank", "$(JOB_COMPLETION_INDEX)",
			"--nnodes", strconv.Itoa(job.Spec.NumNodes),
			"--nproc-per-node", strconv.Itoa(nproc),
			"--rdzv-backend", jq.Spec.Distributed.RdzvBackend,
			"--rdzv-endpoint", jq.Spec.Distributed.RdzvEndpoint,
			"--rdzv-id", job.Spec.JobName,
			"--no-python",
		)
	} else {
		// Single node training
		cmdParts = append(cmdParts,
			"--standalone",
			"--nproc-per-node", strconv.Itoa(nproc),
			"--rdzv-id", job.Spec.JobName,
			"--no-python",
		)
	}

	// Add the actual command
	cmdParts = append(cmdParts, job.Spec.Command)

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
func (jm *JobManager) attachWorkspaceToTrainer(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue, podSpec *corev1.PodSpec) {
	// Attach the workspace pvc to the init container to copy files to the workspace volume
	podSpec.InitContainers = append(podSpec.InitContainers, corev1.Container{
		Name:            "workspace-sync",
		Image:           "alpine:3.18",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c"},
		Args: []string{
			fmt.Sprintf("while [ ! -f /workspace-pvc/.sync_success ]; do echo 'Waiting for workspace sync...'; sleep 5; done; cp -r /workspace-pvc/* %s", jq.Spec.WorkspaceStorage.MountPath),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "workspace-pvc",
				MountPath: "/workspace-pvc",
				ReadOnly:  true,
			},
			{
				Name:      "workspace",
				MountPath: jq.Spec.WorkspaceStorage.MountPath,
			},
		},
	})

	// Attach the workspace volume to the trainer container
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      "workspace",
		MountPath: jq.Spec.WorkspaceStorage.MountPath,
	})

	// Workspace PVC
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "workspace-pvc",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: GetWorkspacePVCName(job),
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

	// The first container must be named "trainer" - this is a reserved container name
	// The trainer container is where the torchrun command will be executed
	// and where the main training workload will run
	if podSpec.Containers[0].Name != "trainer" {
		return fmt.Errorf("first container must be named 'trainer' (this is a reserved container name for the training workload)")
	}

	// Validate that no other container is named "trainer"
	for i := 1; i < len(podSpec.Containers); i++ {
		if podSpec.Containers[i].Name == "trainer" {
			return fmt.Errorf("only the first container can be named 'trainer'")
		}
	}

	return nil
}

// buildPodLabels builds the pod labels
func (jm *JobManager) buildPodLabels(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) map[string]string {
	labels := map[string]string{
		"app":                   "torchrun",
		"torchrun.ai/job-id":    job.Spec.JobID,
		"torchrun.ai/job-name":  job.Spec.JobName,
		"torchrun.ai/job-queue": job.Spec.Queue,
		"kai.scheduler/queue":   jq.Spec.Queue.Name,
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
		"torchrun.ai/job-id":    job.Spec.JobID,
		"torchrun.ai/job-name":  job.Spec.JobName,
		"torchrun.ai/job-queue": job.Spec.Queue,
	}

	// Add user-specified annotations
	for k, v := range job.Spec.Annotations {
		annotations[k] = v
	}

	return annotations
}

// translateResourceNames translates resource names in pod volumes based on TorchrunQueue resource definitions
// For resources with nameMode=prefix, it replaces the original names with prefixed names
func (jm *JobManager) translateResourceNames(podSpec *corev1.PodSpec, jq *torchrunv1alpha1.TorchrunQueue) error {
	// Build a map of original resource names to their actual names
	resourceNameMap := make(map[string]string)

	for _, resource := range jq.Spec.Resources {
		// Parse the resource template to get the original name
		var resourceObj map[string]interface{}
		if err := json.Unmarshal(resource.Template.Raw, &resourceObj); err != nil {
			return fmt.Errorf("failed to unmarshal resource template %s: %w", resource.Name, err)
		}

		// Get metadata.name from the template
		metadata, ok := resourceObj["metadata"].(map[string]interface{})
		if !ok {
			continue
		}

		originalName, ok := metadata["name"].(string)
		if !ok {
			continue
		}

		// Determine the actual resource name based on nameMode
		actualName := resource.Name
		if resource.NameMode == "prefix" {
			actualName = fmt.Sprintf("%s-%s", jq.Name, resource.Name)
		}

		// Map original name to actual name
		resourceNameMap[originalName] = actualName
	}

	// Update volume references in the pod spec
	for i := range podSpec.Volumes {
		volume := &podSpec.Volumes[i]

		// Check if this volume references a PVC
		if volume.PersistentVolumeClaim != nil {
			claimName := volume.PersistentVolumeClaim.ClaimName

			// Check if this claim name needs to be translated
			if actualName, found := resourceNameMap[claimName]; found {
				// Volume reference will be updated from original to actual name
				volume.PersistentVolumeClaim.ClaimName = actualName
			}
		}
	}

	return nil
}
