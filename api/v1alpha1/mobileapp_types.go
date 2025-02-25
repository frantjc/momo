package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	PhaseReady   = "Ready"
	PhasePending = "Pending"
)

// MobileAppSpec defines the desired state of MobileApp.
type MobileAppSpec struct {
	// +kubebuilder:validation:Required
	Selector labels.Set `json:"selector"`
	// +kubebuilder:validation:Optional
	UniversalLinksHost string `json:"universalLinksHost,omitempty"`
}

// MobileAppStatus defines the observed state of MobileApp.
type MobileAppStatus struct {
	// +kubebuilder:default=Pending
	Phase string `json:"phase"`
	// +kubebuilder:validation:Optional
	APKs []MobileAppStatusApp `json:"apks,omitempty"`
	// +kubebuilder:validation:Optional
	IPAs []MobileAppStatusApp `json:"ipas,omitempty"`
}

type MobileAppStatusApp struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`
	// +kubebuilder:validation:Optional
	Latest bool `json:"latest,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MobileApp is the Schema for the MobileApps API.
type MobileApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MobileAppSpec   `json:"spec,omitempty"`
	Status MobileAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MobileAppList contains a list of MobileApp.
type MobileAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MobileApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MobileApp{}, &MobileAppList{})
}
