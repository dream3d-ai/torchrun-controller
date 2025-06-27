package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

// TorchrunQueueReconciler reconciles a TorchrunQueue object
type TorchrunQueueReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunqueues,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunqueues/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=torchrun.ai,resources=torchrunqueues/finalizers,verbs=update
//+kubebuilder:rbac:groups=scheduling.run.ai,resources=queues,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update

// Reconcile handles the reconciliation loop for JobQueue
func (r *TorchrunQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the JobQueue instance
	var jobQueue torchrunv1alpha1.TorchrunQueue
	if err := r.Get(ctx, req.NamespacedName, &jobQueue); err != nil {
		if errors.IsNotFound(err) {
			// JobQueue was deleted, delete the corresponding kai-scheduler Queue
			return r.deleteKaiQueue(ctx, req.Name)
		}
		return ctrl.Result{}, err
	}

	// Create or update the kai-scheduler Queue
	if err := r.createOrUpdateKaiQueue(ctx, &jobQueue); err != nil {
		log.Error(err, "Failed to create/update kai-scheduler Queue")
		return ctrl.Result{}, err
	}

	// Update JobQueue status
	if err := r.updateStatus(ctx, &jobQueue); err != nil {
		log.Error(err, "Failed to update JobQueue status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createOrUpdateKaiQueue creates or updates the kai-scheduler Queue resource
func (r *TorchrunQueueReconciler) createOrUpdateKaiQueue(ctx context.Context, jobQueue *torchrunv1alpha1.TorchrunQueue) error {
	log := log.FromContext(ctx)

	// Build the kai-scheduler Queue object
	kaiQueue := r.buildKaiQueue(jobQueue)

	// Check if the Queue already exists
	existingQueue := &unstructured.Unstructured{}
	existingQueue.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "scheduling.run.ai",
		Version: "v2",
		Kind:    "Queue",
	})

	err := r.Get(ctx, client.ObjectKey{Name: jobQueue.Spec.Queue.Name}, existingQueue)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the Queue
			log.Info("Creating kai-scheduler Queue", "name", jobQueue.Spec.Queue)
			return r.Create(ctx, kaiQueue)
		}
		return err
	}

	// Update the existing Queue
	log.Info("Updating kai-scheduler Queue", "name", jobQueue.Spec.Queue.Name)
	kaiQueue.SetResourceVersion(existingQueue.GetResourceVersion())
	return r.Update(ctx, kaiQueue)
}

// buildKaiQueue builds a kai-scheduler Queue object from a JobQueue
func (r *TorchrunQueueReconciler) buildKaiQueue(jobQueue *torchrunv1alpha1.TorchrunQueue) *unstructured.Unstructured {
	// Build the Queue spec with default values
	spec := map[string]interface{}{
		"resources": map[string]interface{}{
			"cpu": map[string]interface{}{
				"quota":           jobQueue.Spec.Queue.Resources.CPU.Quota,
				"limit":           jobQueue.Spec.Queue.Resources.CPU.Limit,
				"overQuotaWeight": jobQueue.Spec.Queue.Resources.CPU.OverQuotaWeight,
			},
			"gpu": map[string]interface{}{
				"quota":           jobQueue.Spec.Queue.Resources.GPU.Quota,
				"limit":           jobQueue.Spec.Queue.Resources.GPU.Limit,
				"overQuotaWeight": jobQueue.Spec.Queue.Resources.GPU.OverQuotaWeight,
			},
			"memory": map[string]interface{}{
				"quota":           jobQueue.Spec.Queue.Resources.Memory.Quota,
				"limit":           jobQueue.Spec.Queue.Resources.Memory.Limit,
				"overQuotaWeight": jobQueue.Spec.Queue.Resources.Memory.OverQuotaWeight,
			},
		},
	}

	// Add parent queue if specified (default is "default" from kubebuilder annotation)
	parentQueue := jobQueue.Spec.Queue.ParentQueue
	if parentQueue == "" {
		parentQueue = "default"
	}
	spec["parentQueue"] = parentQueue

	// Create the unstructured object
	kaiQueue := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "scheduling.run.ai/v2",
			"kind":       "Queue",
			"metadata": map[string]interface{}{
				"name": jobQueue.Spec.Queue,
				"labels": map[string]interface{}{
					"torchrun.ai/managed-by": "jobqueue-controller",
					"torchrun.ai/jobqueue":   jobQueue.Name,
				},
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         jobQueue.APIVersion,
						"kind":               jobQueue.Kind,
						"name":               jobQueue.Name,
						"uid":                jobQueue.UID,
						"controller":         true,
						"blockOwnerDeletion": true,
					},
				},
			},
			"spec": spec,
		},
	}

	return kaiQueue
}

// deleteKaiQueue deletes the kai-scheduler Queue resource
func (r *TorchrunQueueReconciler) deleteKaiQueue(ctx context.Context, queueName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Need to find the kai queue name from the JobQueue name
	// For now, we'll just try to delete using the same name
	// In a real implementation, you might want to list queues with the label

	// List all kai-scheduler Queues with our label
	queueList := &unstructured.UnstructuredList{}
	queueList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "scheduling.run.ai",
		Version: "v2",
		Kind:    "QueueList",
	})

	labelSelector := client.MatchingLabels{
		"torchrun.ai/jobqueue": queueName,
	}

	if err := r.List(ctx, queueList, labelSelector); err != nil {
		log.Error(err, "Failed to list kai-scheduler Queues")
		return ctrl.Result{}, err
	}

	// Delete all matching queues
	for _, queue := range queueList.Items {
		if err := r.Delete(ctx, &queue); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete kai-scheduler Queue", "name", queue.GetName())
			return ctrl.Result{}, err
		}
		log.Info("Deleted kai-scheduler Queue", "name", queue.GetName())
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the JobQueue status
func (r *TorchrunQueueReconciler) updateStatus(ctx context.Context, jobQueue *torchrunv1alpha1.TorchrunQueue) error {
	// Update status fields
	jobQueue.Status.Phase = "Active"
	now := metav1.Now()
	jobQueue.Status.LastUpdateTime = &now
	jobQueue.Status.ObservedGeneration = jobQueue.Generation

	// Check if kai-scheduler Queue exists and is ready
	kaiQueue := &unstructured.Unstructured{}
	kaiQueue.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "scheduling.run.ai",
		Version: "v2",
		Kind:    "Queue",
	})

	err := r.Get(ctx, client.ObjectKey{Name: jobQueue.Spec.Queue.Name}, kaiQueue)
	if err != nil {
		if errors.IsNotFound(err) {
			jobQueue.Status.Phase = "Updating"
			r.addCondition(jobQueue, "QueueReady", "False", "QueueNotFound", "Kai-scheduler Queue not found")
		} else {
			r.addCondition(jobQueue, "QueueReady", "Unknown", "GetQueueError", fmt.Sprintf("Failed to get Queue: %v", err))
		}
	} else {
		r.addCondition(jobQueue, "QueueReady", "True", "QueueExists", "Kai-scheduler Queue is ready")
	}

	return r.Status().Update(ctx, jobQueue)
}

// addCondition adds or updates a condition on the JobQueue
func (r *TorchrunQueueReconciler) addCondition(jobQueue *torchrunv1alpha1.TorchrunQueue, condType, status, reason, message string) {
	now := metav1.Now()
	newCondition := torchrunv1alpha1.JobQueueCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: &now,
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition
	for i, condition := range jobQueue.Status.Conditions {
		if condition.Type == condType {
			if condition.Status != status {
				jobQueue.Status.Conditions[i] = newCondition
			}
			return
		}
	}

	// Add new condition
	jobQueue.Status.Conditions = append(jobQueue.Status.Conditions, newCondition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *TorchrunQueueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&torchrunv1alpha1.TorchrunQueue{}).
		Complete(r)
}
