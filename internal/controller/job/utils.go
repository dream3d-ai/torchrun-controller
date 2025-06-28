package controller

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// GetWorkspacePVCName returns the consistent name for the workspace PVC
func GetWorkspacePVCName(job *torchrunv1alpha1.TorchrunJob) string {
	return fmt.Sprintf("%s-workspace", job.Spec.JobName)
}

// GetSyncPodName returns the consistent name for the sync pod
func GetSyncPodName(job *torchrunv1alpha1.TorchrunJob) string {
	return fmt.Sprintf("%s-sync", job.Name)
}

// completionModePtr returns a pointer to a completion mode
func completionModePtr(mode batchv1.CompletionMode) *batchv1.CompletionMode {
	return &mode
}
