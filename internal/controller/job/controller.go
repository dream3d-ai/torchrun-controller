package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// TorchrunJobReconciler reconciles a TorchrunJob object
type TorchrunJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunjobs/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update

// Reconcile handles the reconciliation loop for TorchrunJob
// The flow is as follows:
// 1. Create workspace PVC if it doesn't exist
// 2. Check if workspace PVC is ready (has sync-completed label)
// 3. If PVC is not ready:
//   - Create sync pod to prepare the workspace
//   - Monitor sync pod completion and update PVC label when done
//
// 4. If PVC is ready:
//   - Create the Kubernetes Job for training
//
// 5. Update status based on the current state
func (r *TorchrunJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the TorchrunJob instance
	var job torchrunv1alpha1.TorchrunJob
	if err := r.Get(ctx, req.NamespacedName, &job); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if job is being deleted
	if job.DeletionTimestamp != nil {
		log.Info("Job is being deleted", "name", job.Name)
		return ctrl.Result{}, nil
	}

	// Fetch the referenced TorchrunQueue
	var jobQueue torchrunv1alpha1.TorchrunQueue
	if err := r.Get(ctx, types.NamespacedName{
		Name:      job.Spec.Queue,
		Namespace: job.Namespace,
	}, &jobQueue); err != nil {
		log.Error(err, "Failed to get JobQueue", "name", job.Spec.Queue)
		statusManager := NewStatusManager(r.Client)
		statusManager.UpdateCondition(&job, "QueueNotFound", "False", "QueueNotFound",
			fmt.Sprintf("TorchrunQueue %s not found", job.Spec.Queue))
		job.Status.Phase = torchrunv1alpha1.PhaseFailed
		return ctrl.Result{}, r.Status().Update(ctx, &job)
	}

	// Initialize managers
	workspaceManager := NewWorkspaceManager(r.Client)
	jobManager := NewJobManager(r.Client)
	statusManager := NewStatusManager(r.Client)

	// Step 1: Create workspace PVC if it doesn't exist
	if err := workspaceManager.CreateWorkspacePVC(ctx, &job, &jobQueue); err != nil {
		log.Error(err, "Failed to create workspace PVC")
		return ctrl.Result{}, err
	}

	// Step 2: Check if workspace PVC is ready (has sync-completed label)
	workspaceReady, err := workspaceManager.CheckWorkspacePVCStatus(ctx, &job)
	if err != nil {
		// Check if this is a sync pod failure
		if strings.Contains(err.Error(), "sync pod failed") {
			log.Error(err, "Sync pod failed")
			statusManager.UpdateCondition(&job, "WorkspaceSync", "False", "SyncFailed", err.Error())
			job.Status.Phase = torchrunv1alpha1.PhaseFailed
			if updateErr := r.Status().Update(ctx, &job); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			// Don't requeue on sync failure
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to check workspace PVC status")
		return ctrl.Result{}, err
	}

	// Step 3: If workspace is ready, create the job; otherwise create sync pod
	if workspaceReady {
		// Workspace is ready, create the job
		log.Info("Workspace is ready, creating job", "name", job.Name)
		statusManager.UpdateCondition(&job, "WorkspaceReady", "True", "WorkspaceReady", "Workspace sync completed successfully")

		if err := jobManager.CreateJob(ctx, &job, &jobQueue); err != nil {
			log.Error(err, "Failed to create job")
			statusManager.UpdateCondition(&job, "JobCreated", "False", "CreateFailed", err.Error())
			if updateErr := r.Status().Update(ctx, &job); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
		statusManager.UpdateCondition(&job, "JobCreated", "True", "JobCreated", "Kubernetes Job created successfully")
	} else {
		// Workspace not ready, create sync pod if it doesn't exist
		log.Info("Workspace not ready, creating sync pod", "name", job.Name)
		if err := workspaceManager.CreateSyncPod(ctx, &job, &jobQueue); err != nil {
			log.Error(err, "Failed to create sync pod")
			statusManager.UpdateCondition(&job, "WorkspaceSync", "False", "CreateSyncPodFailed", err.Error())
			return ctrl.Result{}, err
		}
		statusManager.UpdateCondition(&job, "WorkspaceSync", "True", "SyncInProgress", "Workspace sync pod created and running")

		// Requeue to check sync pod status
		if err := statusManager.UpdateStatus(ctx, &job); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update status
	if err := statusManager.UpdateStatus(ctx, &job); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TorchrunJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&torchrunv1alpha1.TorchrunJob{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
