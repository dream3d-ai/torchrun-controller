package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
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
	// Get the underlying Kubernetes Job
	k8sJob := &batchv1.Job{}
	err := sm.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, k8sJob)
	if err != nil {
		if errors.IsNotFound(err) {
			job.Status.Phase = torchrunv1alpha1.PhasePending
			return sm.client.Status().Update(ctx, job)
		}
		return err
	}

	// Update status based on Job status
	// Phase meanings:
	// - Running: Job has active pods
	// - Pending: Job created but no pods running yet
	// - Syncing: Workspace is being synchronized (TODO: implement sync pod check)
	// - Succeeded: All pods completed successfully
	// - Suspended: Job is suspended (spec.suspend = true)
	// - Deleted: Job is being deleted (DeletionTimestamp set)
	// - Failed: Job has failed pods or exceeded retry limit
	// - TimedOut: Job exceeded activeDeadlineSeconds (TODO: implement)
	// - Preempted: Job was preempted by higher priority job (TODO: implement)
	// - Unknown: Unable to determine job status

	// Check if job is being deleted
	if job.DeletionTimestamp != nil {
		job.Status.Phase = torchrunv1alpha1.PhaseDeleted
	} else if job.Spec.Suspend {
		job.Status.Phase = torchrunv1alpha1.PhaseSuspended
	} else if k8sJob.Status.Active > 0 {
		job.Status.Phase = torchrunv1alpha1.PhaseRunning
		job.Status.Workers.Running = k8sJob.Status.Active
	} else if k8sJob.Status.Succeeded > 0 {
		job.Status.Phase = torchrunv1alpha1.PhaseSucceeded
		job.Status.Workers.Succeeded = k8sJob.Status.Succeeded
		if job.Status.CompletionTime == nil {
			job.Status.CompletionTime = k8sJob.Status.CompletionTime
		}
	} else if k8sJob.Status.Failed > 0 {
		job.Status.Phase = torchrunv1alpha1.PhaseFailed
		job.Status.Workers.Failed = k8sJob.Status.Failed
	}

	// Update workers status string
	totalWorkers := job.Status.NumNodes
	job.Status.WorkersStatus = fmt.Sprintf("%d/%d ready", job.Status.Workers.Ready, totalWorkers)

	// Update last reconcile time
	now := metav1.Now()
	job.Status.LastReconcileTime = &now

	return sm.client.Status().Update(ctx, job)
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
