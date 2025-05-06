package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IPASpec defines the desired state of IPA.
type IPASpec struct {
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// IPAStatus defines the observed state of IPA.
type IPAStatus struct {
	// +kubebuilder:default=Pending
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase string `json:"phase"`
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +kubebuilder:validation:Optional
	Digest string `json:"digest,omitempty"`
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`
	// +kubebuilder:validation:Optional
	BundleName string `json:"bundleName,omitempty"`
	// +kubebuilder:validation:Optional
	BundleIdentifier string `json:"bundleIdentifier,omitempty"`
	// +kubebuilder:validation:Optional
	Icons []AppStatusIcon `json:"icons,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Digest",type=string,JSONPath=`.status.digest`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="BundleName",type=string,JSONPath=`.status.bundleName`
// +kubebuilder:printcolumn:name="BundleIdentifier",type=string,JSONPath=`.status.bundleIdentifier`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// IPA is the Schema for the IPAs API.
type IPA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPASpec   `json:"spec,omitempty"`
	Status IPAStatus `json:"status,omitempty"`
}

func (i IPA) GetKey() string {
	return i.Spec.Key
}

func (i IPA) GetIcons() []AppStatusIcon {
	return i.Status.Icons
}

// +kubebuilder:object:root=true

// IPAList contains a list of IPA.
type IPAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPA `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPA{}, &IPAList{})
}
