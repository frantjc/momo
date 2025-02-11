package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MobileAppSpecImage struct {
	// +kubebuilder:validation:Required
	Key string `json:"key,omitempty"`
}

type MobileAppImageType string

const (
	MobileAppImageTypeDisplay  = "display"
	MobileAppImageTypeFullSize = "fullSize"
)

// MobileAppSpec defines the desired state of MobileApp.
type MobileAppSpec struct {
	// +kubebuilder:validation:Required
	SpecBucketKeyRef `json:",inline"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:={"apk", "ipa"}
	Type MobileAppType `json:"type,omitempty"`
	// +kubebuilder:validation:Optional
	Images map[string]MobileAppSpecImage `json:"images,omitempty"`
}

type MobileAppType string

const (
	MobileAppTypeAPK MobileAppType = "apk"
	MobileAppTypeIPA MobileAppType = "ipa"
)

type MobileAppStatusImage struct {
	// +kubebuilder:validation:Required
	Key string `json:"key,omitempty"`
}

// MobileAppStatus defines the observed state of MobileApp.
type MobileAppStatus struct {
	// +kubebuilder:default:=Pending
	Phase string `json:"phase"`

	Digest                 string `json:"digest,omitempty"`
	Version                string `json:"version"`
	BundleName             string `json:"bundleName,omitempty"`
	BundleIdentifier       string `json:"bundleIdentifier,omitempty"`
	SHA256CertFingerprints string `json:"sha256CertFingerprints,omitempty"`

	Images map[string]MobileAppStatusImage `json:"images,omitempty"`
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
