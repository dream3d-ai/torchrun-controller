package controller

import (
	"context"
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

	// Fetch the referenced JobQueue
	var jobQueue torchrunv1alpha1.TorchrunQueue
	if err := r.Get(ctx, types.NamespacedName{
		Name:      job.Spec.JobQueue,
		Namespace: job.Namespace,
	}, &jobQueue); err != nil {
		log.Error(err, "Failed to get JobQueue", "name", job.Spec.JobQueue)
		job.Status.Phase = torchrunv1alpha1.PhaseFailed
		return ctrl.Result{}, r.Status().Update(ctx, &job)
	}

	// Create workspace resources
	workspaceManager := NewWorkspaceManager(r.Client)
	if err := workspaceManager.CreateWorkspacePVC(ctx, &job, &jobQueue); err != nil {
		return ctrl.Result{}, err
	}

	// Create sync pod
	if err := workspaceManager.CreateSyncPod(ctx, &job, &jobQueue); err != nil {
		return ctrl.Result{}, err
	}

	// Check sync pod status before proceeding
	workspaceReady, err := workspaceManager.CheckWorkspacePVCStatus(ctx, &job)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !workspaceReady {
		// Workspace PVC not ready yet, update status and requeue
		job.Status.Phase = torchrunv1alpha1.PhaseSyncing
		if err := r.Status().Update(ctx, &job); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Workspace PVC is ready, proceed with creating the job
	jobManager := NewJobManager(r.Client)
	if err := jobManager.CreateJob(ctx, &job, &jobQueue); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	statusManager := NewStatusManager(r.Client)
	if err := statusManager.UpdateStatus(ctx, &job); err != nil {
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
