package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// JobQueueSpec defines the desired state of JobQueue
type JobQueueSpec struct {
	// kai-scheduler queue name this JobQueue maps to
	Queue QueueConfig `json:"queue"`

	// Distributed training configuration
	Distributed DistributedConfig `json:"distributed,omitempty"`

	// Pod template configuration
	PodTemplateConfig PodTemplateConfig `json:"podTemplate,omitempty"`

	// Service account name
	// +kubebuilder:default="default"
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// QueueConfig defines the kai-scheduler queue configuration
type QueueConfig struct {
	// kai-scheduler queue name this JobQueue maps to
	Name string `json:"name"`

	// kai-scheduler parent queue name this JobQueue maps to
	// +kubebuilder:default="default"
	ParentQueue string `json:"parentQueue,omitempty"`

	// Resource quotas and limits for the queue
	Resources QueueResources `json:"resources,omitempty"`
}

// QueueResources defines resource quotas and limits
type QueueResources struct {
	// CPU resource configuration
	CPU ResourceConfig `json:"cpu,omitempty"`

	// GPU resource configuration
	GPU ResourceConfig `json:"gpu,omitempty"`

	// Memory resource configuration
	Memory ResourceConfig `json:"memory,omitempty"`
}

// ResourceConfig defines quota and limit configuration for a resource
type ResourceConfig struct {
	// Resource quota for the queue
	// +kubebuilder:default=-1
	Quota int `json:"quota,omitempty"`

	// Resource limit for the queue
	// +kubebuilder:default=-1
	Limit int `json:"limit,omitempty"`

	// Over quota weight for the queue
	// +kubebuilder:default=1
	OverQuotaWeight int `json:"overQuotaWeight,omitempty"`
}

// DistributedConfig defines distributed training configuration
type DistributedConfig struct {
	// PyTorch distributed backend
	// +kubebuilder:validation:Enum=nccl;gloo;mpi
	// +kubebuilder:default="nccl"
	Backend string `json:"backend,omitempty"`

	// Rendezvous backend for torchrun
	// +kubebuilder:validation:Enum=etcd-v2;c10d;static
	// +kubebuilder:default="etcd-v2"
	RdzvBackend string `json:"rdzvBackend,omitempty"`

	// Rendezvous endpoint (e.g., etcd service)
	// +kubebuilder:default="etcd.etcd-system.svc.cluster.local:2379"
	RdzvEndpoint string `json:"rdzvEndpoint,omitempty"`

	// Port for distributed training
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=29500
	Port int32 `json:"port,omitempty"`
}

// PodTemplateConfig defines the pod template for jobs
type PodTemplateConfig struct {
	// Metadata to be added to pod
	Metadata PodMetadata `json:"metadata,omitempty"`

	// Pod spec template - can contain any valid pod spec fields
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Spec runtime.RawExtension `json:"spec,omitempty"`
}

// PodMetadata defines metadata for pods
type PodMetadata struct {
	// Labels to add to pods
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to pods
	Annotations map[string]string `json:"annotations,omitempty"`
}

// JobQueueStatus defines the observed state of JobQueue
type JobQueueStatus struct {
	// Phase of the JobQueue
	// +kubebuilder:validation:Enum=Active;Updating;Terminating
	// +kubebuilder:default="Active"
	Phase string `json:"phase,omitempty"`

	// Last time the status was updated
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// Last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions
	Conditions []JobQueueCondition `json:"conditions,omitempty"`
}

// JobQueueCondition describes the state of a JobQueue
type JobQueueCondition struct {
	// Type of condition
	Type string `json:"type"`

	// Status of the condition
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status string `json:"status"`

	// Last time the condition transitioned
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition
	Reason string `json:"reason,omitempty"`

	// A human-readable message about the transition
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=jq
// +kubebuilder:printcolumn:name="Queue",type="string",JSONPath=".spec.queue.name"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TorchrunQueue is the Schema for the torchrunqueues API
type TorchrunQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobQueueSpec   `json:"spec,omitempty"`
	Status JobQueueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TorchrunQueueList contains a list of JobQueue
type TorchrunQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TorchrunQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TorchrunQueue{}, &TorchrunQueueList{})
}
