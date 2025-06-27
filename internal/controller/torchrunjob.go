package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	job "github.com/dream3d/torchrun-controller/internal/controller/job"
	queue "github.com/dream3d/torchrun-controller/internal/controller/queue"
)

// NewTorchrunJobReconciler creates a new JobReconciler
func NewTorchrunJobReconciler(client client.Client, scheme *runtime.Scheme) *job.TorchrunJobReconciler {
	return &job.TorchrunJobReconciler{
		Client: client,
		Scheme: scheme,
	}
}

// NewJobQueueReconciler creates a new QueueReconciler
func NewJobQueueReconciler(client client.Client, scheme *runtime.Scheme) *queue.TorchrunQueueReconciler {
	return &queue.TorchrunQueueReconciler{
		Client: client,
		Scheme: scheme,
	}
}
