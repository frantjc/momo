package v1alpha1

import (
	"context"
	"fmt"

	"gocloud.dev/blob"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BucketSpec defines the desired state of Bucket.
type BucketSpec struct {
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`
	// +kubebuilder:validation:Optional
	URLFrom *corev1.EnvVarSource `json:"urlFrom,omitempty"`
}

type SpecBucketKeyRef struct {
	// +kubebuilder:validation:Required
	Bucket corev1.LocalObjectReference `json:"bucket"`
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

func (b *Bucket) Open(ctx context.Context, cli client.Client) (*blob.Bucket, error) {
	if b.Spec.URL != "" {
		return blob.OpenBucket(ctx, b.Spec.URL)
	} else if b.Spec.URLFrom != nil {
		if b.Spec.URLFrom.ConfigMapKeyRef != nil {
			configMap := &corev1.ConfigMap{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      b.Spec.URLFrom.ConfigMapKeyRef.Name,
					Namespace: b.ObjectMeta.Namespace,
				},
				configMap,
			); err != nil {
				return nil, fmt.Errorf("get ConfigMap: %w", err)
			}
			value, ok := configMap.Data[b.Spec.URLFrom.ConfigMapKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in ConfigMap %s", b.Spec.URLFrom.ConfigMapKeyRef.Key, b.Spec.URLFrom.ConfigMapKeyRef.Name)
			}
			return blob.OpenBucket(ctx, value)
		}

		if b.Spec.URLFrom.SecretKeyRef != nil {
			secret := &corev1.Secret{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      b.Spec.URLFrom.SecretKeyRef.Name,
					Namespace: b.ObjectMeta.Namespace,
				},
				secret,
			); err != nil {
				return nil, fmt.Errorf("get Secret: %w", err)
			}
			value, ok := secret.Data[b.Spec.URLFrom.SecretKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in Secret %s", b.Spec.URLFrom.SecretKeyRef.Key, b.Spec.URLFrom.SecretKeyRef.Name)
			}
			return blob.OpenBucket(ctx, string(value))
		}

		if b.Spec.URLFrom.FieldRef != nil {
			return nil, fmt.Errorf(".spec.urlFrom.fieldRef unsupported")
		}

		if b.Spec.URLFrom.ResourceFieldRef != nil {
			return nil, fmt.Errorf(".spec.urlFrom.resourceFieldRef unsupported")
		}
	}

	return nil, fmt.Errorf("missing url in .spec")
}

// BucketStatus defines the observed state of Bucket.
type BucketStatus struct {
	// +kubebuilder:default:=Pending
	Phase string `json:"phase"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
