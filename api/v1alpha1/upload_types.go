package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UploadSpec defines the desired state of Upload.
type UploadSpec struct {
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// UploadStatus defines the observed state of Upload.
type UploadStatus struct {
	// +kubebuilder:default=Pending
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Upload is the Schema for the uploads API.
type Upload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UploadSpec   `json:"spec,omitempty"`
	Status UploadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UploadList contains a list of Upload.
type UploadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Upload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Upload{}, &UploadList{})
}
