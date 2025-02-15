package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MobileAppSpec defines the desired state of MobileApp.
type MobileAppSpec struct {
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:={"APK", "IPA"}
	Type MobileAppType `json:"type,omitempty"`
}

type MobileAppType string

const (
	MobileAppTypeAPK MobileAppType = "APK"
	MobileAppTypeIPA MobileAppType = "IPA"
)

type MobileAppStatusImage struct {
	// +kubebuilder:validation:Required
	Key string `json:"key,omitempty"`
	// +kubebuilder:validation:Required
	Height int `json:"height,omitempty"`
	// +kubebuilder:validation:Required
	Width int `json:"width,omitempty"`
	// +kubebuilder:validation:Optional
	Display bool `json:"display,omitempty"`
	// +kubebuilder:validation:Optional
	FullSize bool `json:"fullSize,omitempty"`
}

// MobileAppStatus defines the observed state of MobileApp.
type MobileAppStatus struct {
	// +kubebuilder:default=Pending
	Phase string `json:"phase"`

	Digest                 string `json:"digest,omitempty"`
	Version                string `json:"version,omitempty"`
	BundleName             string `json:"bundleName,omitempty"`
	BundleIdentifier       string `json:"bundleIdentifier,omitempty"`
	SHA256CertFingerprints string `json:"sha256CertFingerprints,omitempty"`

	Images []MobileAppStatusImage `json:"images,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MobileApp is the Schema for the mobileapps API.
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
