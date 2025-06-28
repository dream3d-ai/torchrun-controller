package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// WorkspaceManager handles workspace-related operations
type WorkspaceManager struct {
	client client.Client
}

// NewWorkspaceManager creates a new workspace manager
func NewWorkspaceManager(client client.Client) *WorkspaceManager {
	return &WorkspaceManager{
		client: client,
	}
}

// getDefaultStorageClass finds the default StorageClass in the cluster
func (wm *WorkspaceManager) getDefaultStorageClass(ctx context.Context) (string, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := wm.client.List(ctx, storageClasses); err != nil {
		return "", fmt.Errorf("failed to list storage classes: %w", err)
	}

	for _, sc := range storageClasses.Items {
		if sc.Annotations != nil && sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return sc.Name, nil
		}
	}

	return "", fmt.Errorf("no default storage class found in the cluster")
}

// CreateWorkspacePVC creates the workspace PVC
func (wm *WorkspaceManager) CreateWorkspacePVC(ctx context.Context, job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) error {
	log := log.FromContext(ctx)

	// Determine storage class to use, with job override taking precedence over jq
	storageClassName := ""
	if job.Spec.WorkspaceStorage.StorageClass != "" {
		storageClassName = job.Spec.WorkspaceStorage.StorageClass
	} else {
		storageClassName = jq.Spec.WorkspaceStorage.StorageClass
	}
	if storageClassName == "" {
		// Look up the default storage class
		defaultSC, err := wm.getDefaultStorageClass(ctx)
		if err != nil {
			log.Error(err, "Failed to find default storage class")
			return err
		}
		storageClassName = defaultSC
		log.Info("Using default storage class", "storageClass", storageClassName)
	}

	// Storage size, with job override taking precedence over jq
	storageSize := ""
	if job.Spec.WorkspaceStorage.Size != "" {
		storageSize = job.Spec.WorkspaceStorage.Size
	} else if jq.Spec.WorkspaceStorage.Size != "" {
		storageSize = jq.Spec.WorkspaceStorage.Size
	} else {
		storageSize = "1Gi"
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetWorkspacePVCName(job),
			Namespace: job.Namespace,
			Labels: map[string]string{
				"torchrun.ai/job-name":       job.Spec.JobName,
				"torchrun.ai/type":           "workspace",
				"torchrun.ai/sync-completed": "false",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, job.GroupVersionKind()),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
				corev1.ReadOnlyMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
		},
	}

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := wm.client.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, existingPVC)
	if err == nil {
		log.Info("Workspace PVC already exists", "name", pvc.Name)
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	log.Info("Creating workspace PVC", "name", pvc.Name, "storageClass", storageClassName)
	return wm.client.Create(ctx, pvc)
}

// CreateSyncPod creates the workspace sync pod
func (wm *WorkspaceManager) CreateSyncPod(ctx context.Context, job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) error {
	log := log.FromContext(ctx)

	// Build sync pod
	syncPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetSyncPodName(job),
			Namespace: job.Namespace,
			Labels: map[string]string{
				"torchrun.ai/job-name": job.Spec.JobName,
				"torchrun.ai/role":     "sync",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, job.GroupVersionKind()),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyOnFailure,
			ServiceAccountName: jq.Spec.ServiceAccountName,
			Containers: []corev1.Container{
				{
					Name:            "sync",
					Image:           jq.Spec.WorkspaceStorage.Image,
					ImagePullPolicy: jq.Spec.WorkspaceStorage.ImagePullPolicy,
					Command:         []string{"/bin/sh", "-c"},
					Args:            []string{wm.buildSyncCommand(job, jq)},
					WorkingDir:      "/workspace",
					Env:             wm.buildSyncEnvironment(job, jq),
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "workspace",
							MountPath: "/workspace",
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: GetWorkspacePVCName(job),
						},
					},
				},
			},
		},
	}

	// Check if sync pod already exists
	existingPod := &corev1.Pod{}
	err := wm.client.Get(ctx, types.NamespacedName{Name: syncPod.Name, Namespace: syncPod.Namespace}, existingPod)
	if err == nil {
		log.Info("Sync pod already exists", "name", syncPod.Name)
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	// Check if workspace PVC exists
	workspacePVC := &corev1.PersistentVolumeClaim{}
	err = wm.client.Get(ctx, types.NamespacedName{Name: GetWorkspacePVCName(job), Namespace: job.Namespace}, workspacePVC)
	if err != nil {
		return err
	}

	// Check if workspace PVC has sync-completed label
	if workspacePVC.Labels != nil && workspacePVC.Labels["torchrun.ai/sync-completed"] == "true" {
		log.Info("Workspace PVC already has sync completed label", "name", workspacePVC.Name)
		return nil
	}

	log.Info("Creating sync pod", "name", syncPod.Name)
	return wm.client.Create(ctx, syncPod)
}

// CheckWorkspacePVCStatus checks if the workspace PVC is ready by checking the sync-completed label,
// and if not, checks if the sync pod has completed and sets the label if so.
func (wm *WorkspaceManager) CheckWorkspacePVCStatus(ctx context.Context, job *torchrunv1alpha1.TorchrunJob) (bool, error) {
	pvcName := GetWorkspacePVCName(job)
	workspacePVC := &corev1.PersistentVolumeClaim{}
	err := wm.client.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: job.Namespace}, workspacePVC)
	if err != nil {
		return false, err
	}

	// If label is already set, return true
	if workspacePVC.Labels != nil && workspacePVC.Labels["torchrun.ai/sync-completed"] == "true" {
		return true, nil
	}

	// Check if sync pod exists and has completed successfully
	syncPodName := GetSyncPodName(job)
	syncPod := &corev1.Pod{}
	err = wm.client.Get(ctx, types.NamespacedName{Name: syncPodName, Namespace: job.Namespace}, syncPod)
	if err != nil {
		// If not found, just return false (not ready yet)
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check sync pod status
	switch syncPod.Status.Phase {
	case corev1.PodSucceeded:
		// Sync pod succeeded, set the label on the PVC
		patch := client.MergeFrom(workspacePVC.DeepCopy())
		if workspacePVC.Labels == nil {
			workspacePVC.Labels = map[string]string{}
		}
		workspacePVC.Labels["torchrun.ai/sync-completed"] = "true"
		if err := wm.client.Patch(ctx, workspacePVC, patch); err != nil {
			return false, err
		}
		return true, nil

	case corev1.PodFailed:
		// Sync pod failed, we should mark this as an error
		// The controller will need to handle this appropriately
		return false, fmt.Errorf("sync pod failed: %s", syncPod.Status.Message)

	default:
		// Pod is still running or pending
		return false, nil
	}
}

// buildSyncCommand builds the sync command based on workspace source
func (wm *WorkspaceManager) buildSyncCommand(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) string {
	var source string
	var url string

	// Determine workspace source
	if job.Spec.WorkspaceStorage.Source != "" {
		source = job.Spec.WorkspaceStorage.Source
		url = job.Spec.WorkspaceStorage.URL
	} else {
		source = jq.Spec.WorkspaceStorage.Source
		url = jq.Spec.WorkspaceStorage.URL
	}

	// Default to zip source if not specified
	if source == "" {
		source = "zip"
	}

	switch source {
	case "zip":
		if url == "" {
			return `
				echo "Waiting for workspace.zip to be uploaded (timeout: 10 minutes)..."
				start_time=$(date +%s)
				timeout_seconds=600   # 10 minutes

				while true; do
					if [ -f /workspace/workspace.zip ]; then
						if unzip -t /workspace/workspace.zip >/dev/null 2>&1; then
							break   # valid archive, proceed
						fi
						echo "workspace.zip detected but still copying – waiting..."
					fi

					# check timeout
					current_time=$(date +%s)
					elapsed=$((current_time - start_time))
					if [ "$elapsed" -ge "$timeout_seconds" ]; then
						echo "ERROR: Timed out waiting for workspace.zip to finish uploading"
						exit 1
					fi

					sleep 5
				done

				echo "Extracting workspace.zip..."
				unzip -q /workspace/workspace.zip -d /workspace/
				rm -f /workspace/workspace.zip
				echo "Workspace sync completed"
				touch /workspace/.sync_success
			`
		}
		// Download from URL
		return fmt.Sprintf(`
			echo "Downloading workspace from %s..."
			wget -q -O /workspace/workspace.zip "%s"
			echo "Extracting workspace.zip..."
			unzip -q /workspace/workspace.zip -d /workspace/
			rm -f /workspace/workspace.zip
			echo "Workspace sync completed"
			touch /workspace/.sync_success
		`, url, url)

	case "git":
		ref := "main"
		if url != "" {
			ref = url
		}
		return fmt.Sprintf(`
			echo "Cloning git repository %s..."
			git clone --branch %s --depth 1 %s /workspace/repo
			mv /workspace/repo/* /workspace/ 2>/dev/null || true
			mv /workspace/repo/.[^.]* /workspace/ 2>/dev/null || true
			rm -rf /workspace/repo
			echo "Workspace sync completed"
			touch /workspace/.sync_success
		`, url, ref, url)

	case "s3":
		return fmt.Sprintf(`
			echo "Downloading from S3: %s..."
			aws s3 cp %s /workspace/workspace.tar.gz
			tar -xzf /workspace/workspace.tar.gz -C /workspace/
			rm -f /workspace/workspace.tar.gz
			echo "Workspace sync completed"
			touch /workspace/.sync_success
		`, url, url)

	default:
		// Just create success marker for existing workspace
		return `
			echo "Using existing workspace"
			touch /workspace/.sync_success
		`
	}
}

// buildSyncEnvironment builds environment variables for sync pod
func (wm *WorkspaceManager) buildSyncEnvironment(job *torchrunv1alpha1.TorchrunJob, jq *torchrunv1alpha1.TorchrunQueue) []corev1.EnvVar {
	env := []corev1.EnvVar{}

	// Add job environment variables that might be needed for sync
	for _, e := range job.Spec.Env {
		// Only include AWS/cloud credentials that might be needed for S3 sync
		if strings.HasPrefix(e.Name, "AWS_") || strings.HasPrefix(e.Name, "GOOGLE_") || strings.HasPrefix(e.Name, "AZURE_") {
			env = append(env, e)
		}
	}

	return env
}
