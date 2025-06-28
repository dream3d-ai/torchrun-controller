package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	torchrunv1alpha1 "github.com/dream3d/torchrun-controller/internal/v1alpha1"
)

func TestTranslateResourceNames(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()
	jm := NewJobManager(client)

	// Create test queue with resources
	jq := &torchrunv1alpha1.TorchrunQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "default",
		},
		Spec: torchrunv1alpha1.JobQueueSpec{
			Resources: []torchrunv1alpha1.ResourceTemplate{
				{
					Name:     "datasets",
					NameMode: "prefix",
					Template: runtime.RawExtension{
						Raw: []byte(`{
							"apiVersion": "v1",
							"kind": "PersistentVolumeClaim",
							"metadata": {
								"name": "juicefs-datasets"
							}
						}`),
					},
				},
				{
					Name:     "checkpoints",
					NameMode: "prefix",
					Template: runtime.RawExtension{
						Raw: []byte(`{
							"apiVersion": "v1",
							"kind": "PersistentVolumeClaim",
							"metadata": {
								"name": "juicefs-checkpoints"
							}
						}`),
					},
				},
				{
					Name:     "shared-config",
					NameMode: "exact", // Not prefix, should not be translated
					Template: runtime.RawExtension{
						Raw: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "shared-config"
							}
						}`),
					},
				},
			},
		},
	}

	// Create test pod spec with volumes
	podSpec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "datasets",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "juicefs-datasets",
					},
				},
			},
			{
				Name: "checkpoints",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "juicefs-checkpoints",
					},
				},
			},
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "shared-config",
						},
					},
				},
			},
			{
				Name: "other-pvc",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "unrelated-pvc", // Should not be translated
					},
				},
			},
		},
	}

	// Run the translation
	err := jm.translateResourceNames(podSpec, jq)
	if err != nil {
		t.Fatalf("translateResourceNames failed: %v", err)
	}

	// Verify results
	tests := []struct {
		volumeName    string
		expectedClaim string
		description   string
	}{
		{
			volumeName:    "datasets",
			expectedClaim: "dev-datasets",
			description:   "datasets PVC should be prefixed",
		},
		{
			volumeName:    "checkpoints",
			expectedClaim: "dev-checkpoints",
			description:   "checkpoints PVC should be prefixed",
		},
		{
			volumeName:    "other-pvc",
			expectedClaim: "unrelated-pvc",
			description:   "unrelated PVC should not be changed",
		},
	}

	for _, test := range tests {
		for _, vol := range podSpec.Volumes {
			if vol.Name == test.volumeName && vol.PersistentVolumeClaim != nil {
				if vol.PersistentVolumeClaim.ClaimName != test.expectedClaim {
					t.Errorf("%s: expected %s, got %s",
						test.description,
						test.expectedClaim,
						vol.PersistentVolumeClaim.ClaimName)
				}
				break
			}
		}
	}
}
