package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APKSpec defines the desired state of APK.
type APKSpec struct {
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

type AppStatusIcon struct {
	// +kubebuilder:validation:Required
	Key string `json:"key,omitempty"`
	// +kubebuilder:validation:Required
	Size int `json:"size,omitempty"`
	// +kubebuilder:validation:Optional
	Display bool `json:"display,omitempty"`
	// +kubebuilder:validation:Optional
	FullSize bool `json:"fullSize,omitempty"`
}

// APKStatus defines the observed state of APK.
type APKStatus struct {
	// +kubebuilder:default=Pending
	Phase string `json:"phase"`
	// +kubebuilder:validation:Optional
	Digest string `json:"digest,omitempty"`
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`
	// +kubebuilder:validation:Optional
	Package string `json:"package,omitempty"`
	// +kubebuilder:validation:Optional
	SHA256CertFingerprints string `json:"sha256CertFingerprints,omitempty"`
	// +kubebuilder:validation:Optional
	Icons []AppStatusIcon `json:"icons,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// APK is the Schema for the APKs API.
type APK struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APKSpec   `json:"spec,omitempty"`
	Status APKStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APKList contains a list of APK.
type APKList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APK `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APK{}, &APKList{})
}
