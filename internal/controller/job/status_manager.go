package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// StatusManager handles status updates and condition management
type StatusManager struct {
	client client.Client
}

// NewStatusManager creates a new status manager
func NewStatusManager(client client.Client) *StatusManager {
	return &StatusManager{
		client: client,
	}
}

// UpdateStatus updates the TorchrunJob status
func (sm *StatusManager) UpdateStatus(ctx context.Context, job *torchrunv1alpha1.TorchrunJob) error {
	// First, check for deletion
	if job.DeletionTimestamp != nil {
		return sm.updatePhase(ctx, job, torchrunv1alpha1.PhaseDeleted)
	}

	// Get the underlying Kubernetes Job
	k8sJob := &batchv1.Job{}
	err := sm.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, k8sJob)

	if err != nil {
		if errors.IsNotFound(err) {
			// K8s job doesn't exist yet - determine phase based on workspace state
			return sm.updatePreJobPhase(ctx, job)
		}
		return err
	}

	// K8s job exists - update phase based on its status
	return sm.updateJobPhase(ctx, job, k8sJob)
}

// updatePreJobPhase determines the phase when K8s Job doesn't exist yet
func (sm *StatusManager) updatePreJobPhase(ctx context.Context, job *torchrunv1alpha1.TorchrunJob) error {
	workspaceReady, err := sm.isWorkspaceReady(ctx, job)

	var phase string
	if err != nil {
		// Workspace doesn't exist or error checking - we're in Pending
		phase = torchrunv1alpha1.PhasePending
	} else if workspaceReady {
		// Workspace is ready, waiting for scheduling
		phase = torchrunv1alpha1.PhaseQueued
	} else {
		// Workspace exists but not ready yet
		phase = torchrunv1alpha1.PhaseSyncing
	}

	return sm.updatePhase(ctx, job, phase)
}

// updateJobPhase updates phase based on K8s Job status
func (sm *StatusManager) updateJobPhase(ctx context.Context, job *torchrunv1alpha1.TorchrunJob, k8sJob *batchv1.Job) error {
	var phase string

	// Update worker counts from K8s Job
	job.Status.Workers.Running = k8sJob.Status.Active
	job.Status.Workers.Succeeded = k8sJob.Status.Succeeded
	job.Status.Workers.Failed = k8sJob.Status.Failed

	// Determine phase based on Job status
	switch {
	case k8sJob.Spec.Suspend != nil && *k8sJob.Spec.Suspend:
		phase = torchrunv1alpha1.PhaseSuspended

	case k8sJob.Status.Active > 0:
		phase = torchrunv1alpha1.PhaseRunning

	case k8sJob.Status.Succeeded > 0:
		phase = torchrunv1alpha1.PhaseSucceeded
		// Update completion time if not already set
		if job.Status.CompletionTime == nil && k8sJob.Status.CompletionTime != nil {
			job.Status.CompletionTime = k8sJob.Status.CompletionTime
		}

	case k8sJob.Status.Failed > 0:
		phase = torchrunv1alpha1.PhaseFailed

	default:
		// Job exists but no pods are active/succeeded/failed
		// Check workspace state to determine if we're queued or syncing
		workspaceReady, err := sm.isWorkspaceReady(ctx, job)
		if err != nil {
			phase = torchrunv1alpha1.PhasePending
		} else if workspaceReady {
			phase = torchrunv1alpha1.PhaseQueued
		} else {
			phase = torchrunv1alpha1.PhaseSyncing
		}
	}

	// Update workers status string
	if job.Status.NumNodes > 0 && phase == torchrunv1alpha1.PhaseRunning {
		job.Status.WorkersStatus = fmt.Sprintf("%d/%d running", job.Status.Workers.Running, job.Status.NumNodes)
	} else if job.Status.NumNodes > 0 && phase == torchrunv1alpha1.PhaseSucceeded {
		job.Status.WorkersStatus = fmt.Sprintf("%d/%d succeeded", job.Status.Workers.Succeeded, job.Status.NumNodes)
	} else if job.Status.NumNodes > 0 && phase == torchrunv1alpha1.PhaseFailed {
		job.Status.WorkersStatus = fmt.Sprintf("%d/%d failed", job.Status.Workers.Failed, job.Status.NumNodes)
	} else {
		job.Status.WorkersStatus = fmt.Sprintf("%d/%d ready", job.Status.Workers.Ready, job.Status.NumNodes)
	}

	return sm.updatePhase(ctx, job, phase)
}

// updatePhase updates the job phase and last reconcile time
func (sm *StatusManager) updatePhase(ctx context.Context, job *torchrunv1alpha1.TorchrunJob, phase string) error {
	job.Status.Phase = phase

	// Update last reconcile time
	now := metav1.Now()
	job.Status.LastReconcileTime = &now

	return sm.client.Status().Update(ctx, job)
}

// isWorkspaceReady checks if the workspace PVC has the sync-completed label
func (sm *StatusManager) isWorkspaceReady(ctx context.Context, job *torchrunv1alpha1.TorchrunJob) (bool, error) {
	pvcName := GetWorkspacePVCName(job)
	workspacePVC := &v1.PersistentVolumeClaim{}
	err := sm.client.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: job.Namespace}, workspacePVC)
	if err != nil {
		return false, err
	}

	return workspacePVC.Labels != nil && workspacePVC.Labels["torchrun.ai/sync-completed"] == "true", nil
}

// UpdateCondition adds or updates a condition on the TorchrunJob
func (sm *StatusManager) UpdateCondition(job *torchrunv1alpha1.TorchrunJob, condType, status, reason, message string) {
	now := metav1.Now()
	newCondition := torchrunv1alpha1.TorchrunJobCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: &now,
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition
	for i, condition := range job.Status.Conditions {
		if condition.Type == condType {
			if condition.Status != status {
				job.Status.Conditions[i] = newCondition
			}
			return
		}
	}

	// Add new condition
	job.Status.Conditions = append(job.Status.Conditions, newCondition)
}
