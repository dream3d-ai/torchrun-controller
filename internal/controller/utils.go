package controller

import (
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// quantityPtr returns a pointer to a resource quantity
func quantityPtr(q string) *resource.Quantity {
	quantity := resource.MustParse(q)
	return &quantity
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}

// completionModePtr returns a pointer to a completion mode
func completionModePtr(mode batchv1.CompletionMode) *batchv1.CompletionMode {
	return &mode
}
