package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BucketSpec defines the desired state of Bucket.
type BucketSpec struct {
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`
	// +kubebuilder:validation:Optional
	URLFrom *corev1.EnvVarSource `json:"urlFrom,omitempty"`
}

// BucketStatus defines the observed state of Bucket.
type BucketStatus struct {
	// +kubebuilder:default=Pending
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase string `json:"phase"`
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Bucket is the Schema for the buckets API.
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec,omitempty"`
	Status BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket.
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}
