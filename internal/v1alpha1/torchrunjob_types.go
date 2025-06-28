package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TorchrunJob phase constants
const (
	PhasePending   = "Pending"
	PhaseSyncing   = "Syncing"
	PhaseQueued    = "Queued"
	PhaseRunning   = "Running"
	PhaseSucceeded = "Succeeded"
	PhaseSuspended = "Suspended"
	PhaseDeleted   = "Deleted"
	PhaseFailed    = "Failed"
	PhaseTimedOut  = "TimedOut"
	PhasePreempted = "Preempted"
	PhaseUnknown   = "Unknown"
)

// TorchrunJobSpec defines the desired state of TorchrunJob
type TorchrunJobSpec struct {
	// Name of the TorchrunQueue to use for this job
	Queue string `json:"queue"`

	// Application-level job name for this TorchrunJob.
	// Used as the rendezvous id (rdz-id) for torchrun and for features like job resumption.
	// If not provided, a random friendly name will be generated.
	JobName string `json:"jobName"`

	// Universally unique identifier (UUID) for this TorchrunJob.
	// Used to uniquely identify the job instance.
	JobID string `json:"jobID"`

	// Training command to execute
	Command string `json:"command"`

	// Optional command to run before training (e.g., download data, install packages)
	SetupCommand string `json:"setupCommand,omitempty"`

	// Number of nodes for training. If set, overrides minNodes and maxNodes to be equal.
	// +kubebuilder:validation:Minimum=1
	NumNodes int `json:"numNodes,omitempty"`

	// Overrides for storage configuration
	WorkspaceStorage WorkspaceStorageConfig `json:"workspaceStorage,omitempty"`

	// Reliability and lifecycle settings
	Reliability ReliabilityConfig `json:"reliability,omitempty"`

	// Additional environment variables (merged with JobQueue env)
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Volume overrides and additions
	Volumes *VolumeOverride `json:"volumes,omitempty"`

	// Create job in suspended state
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`

	// Annotations to add to worker pods
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to add to worker pods
	Labels map[string]string `json:"labels,omitempty"`
}

// ReliabilityConfig defines reliability and lifecycle settings
type ReliabilityConfig struct {
	// Maximum number of restart attempts
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	MaxRestarts int32 `json:"maxRestarts,omitempty"`

	// Restart policy for workers
	// +kubebuilder:validation:Enum=OnFailure;Never
	// +kubebuilder:default="OnFailure"
	RestartPolicy string `json:"restartPolicy,omitempty"`

	// Clean up job after this many seconds
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3600
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Maximum time the job can run
	// +kubebuilder:validation:Minimum=0
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
}

// VolumeOverride defines volume overrides and additions
type VolumeOverride struct {
	// Additional volume mounts (merged with JobQueue mounts)
	AdditionalMounts []AdditionalMount `json:"additionalMounts,omitempty"`

	// Additional volumes (merged with JobQueue volumes)
	AdditionalVolumes []corev1.Volume `json:"additionalVolumes,omitempty"`
}

// AdditionalMount defines additional volume mounts
type AdditionalMount struct {
	// Volume name from JobQueue or additionalVolumes
	Name string `json:"name"`

	// Mount path in container
	MountPath string `json:"mountPath"`

	// Sub-path within the volume
	SubPath string `json:"subPath,omitempty"`

	// Read only
	// +kubebuilder:default=false
	ReadOnly bool `json:"readOnly,omitempty"`
}

// TorchrunJobStatus defines the observed state of TorchrunJob
type TorchrunJobStatus struct {
	// Current phase of the job
	// +kubebuilder:validation:Enum=Running;Pending;Syncing;Succeeded;Suspended;Deleted;Failed;TimedOut;Preempted;Unknown
	Phase string `json:"phase,omitempty"`

	// Number of nodes for training
	NumNodes int `json:"numNodes,omitempty"`

	// Summary of worker status (e.g., "3/4 ready")
	WorkersStatus string `json:"workersStatus,omitempty"`

	// Detailed conditions
	Conditions []TorchrunJobCondition `json:"conditions,omitempty"`

	// Worker pod status
	Workers WorkerStatus `json:"workers,omitempty"`

	// Number of restart attempts
	Restarts int32 `json:"restarts,omitempty"`

	// Start time of the job
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Completion time of the job
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Last time the job was reconciled
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// WorkerStatus describes worker pod status
type WorkerStatus struct {
	// Pending workers
	Pending int32 `json:"pending,omitempty"`

	// Number of ready workers
	Ready int32 `json:"ready,omitempty"`

	// Number of running workers
	Running int32 `json:"running,omitempty"`

	// Number of failed workers
	Failed int32 `json:"failed,omitempty"`

	// Number of succeeded workers
	Succeeded int32 `json:"succeeded,omitempty"`
}

// TorchrunJobCondition describes the state of a TorchrunJob at a certain point
type TorchrunJobCondition struct {
	// Type of condition
	// +kubebuilder:validation:Enum=Provisioned;WorkspaceReady;WorkspaceSync;AllWorkersReady;Completed;JobCreated;QueueNotFound;Failed
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
// +kubebuilder:resource:shortName=tj;trj
// +kubebuilder:printcolumn:name="Queue",type="string",JSONPath=".spec.queue"
// +kubebuilder:printcolumn:name="Nodes",type="integer",JSONPath=".spec.numNodes"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Workers",type="string",JSONPath=".status.workersStatus"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TorchrunJob is the Schema for the torchrunjobs API
type TorchrunJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TorchrunJobSpec   `json:"spec,omitempty"`
	Status TorchrunJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TorchrunJobList contains a list of TorchrunJob
type TorchrunJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TorchrunJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TorchrunJob{}, &TorchrunJobList{})
}
