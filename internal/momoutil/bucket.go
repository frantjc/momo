package momoutil

import (
	"context"
	"fmt"
	"image"
	"image/png"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"gocloud.dev/blob"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetBucket(ctx context.Context, cli client.Client, key client.ObjectKey) (*momov1alpha1.Bucket, error) {
	bucket := &momov1alpha1.Bucket{}

	if err := cli.Get(ctx, key, bucket); err != nil {
		return nil, err
	}

	return bucket, nil
}

func OpenBucket(ctx context.Context, cli client.Client, bucket *momov1alpha1.Bucket) (*blob.Bucket, error) {
	if bucket.Spec.URL != "" {
		return blob.OpenBucket(ctx, bucket.Spec.URL)
	} else if bucket.Spec.URLFrom != nil {
		if bucket.Spec.URLFrom.ConfigMapKeyRef != nil {
			configMap := &corev1.ConfigMap{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      bucket.Spec.URLFrom.ConfigMapKeyRef.Name,
					Namespace: bucket.ObjectMeta.Namespace,
				},
				configMap,
			); err != nil {
				return nil, fmt.Errorf("get ConfigMap: %w", err)
			}
			value, ok := configMap.Data[bucket.Spec.URLFrom.ConfigMapKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in ConfigMap %s", bucket.Spec.URLFrom.ConfigMapKeyRef.Key, bucket.Spec.URLFrom.ConfigMapKeyRef.Name)
			}
			return blob.OpenBucket(ctx, value)
		}

		if bucket.Spec.URLFrom.SecretKeyRef != nil {
			secret := &corev1.Secret{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      bucket.Spec.URLFrom.SecretKeyRef.Name,
					Namespace: bucket.ObjectMeta.Namespace,
				},
				secret,
			); err != nil {
				return nil, fmt.Errorf("get Secret: %w", err)
			}
			value, ok := secret.Data[bucket.Spec.URLFrom.SecretKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in Secret %s", bucket.Spec.URLFrom.SecretKeyRef.Key, bucket.Spec.URLFrom.SecretKeyRef.Name)
			}
			return blob.OpenBucket(ctx, string(value))
		}

		if bucket.Spec.URLFrom.FieldRef != nil {
			return nil, fmt.Errorf(".spec.urlFrom.fieldRef unsupported")
		}

		if bucket.Spec.URLFrom.ResourceFieldRef != nil {
			return nil, fmt.Errorf(".spec.urlFrom.resourceFieldRef unsupported")
		}
	}

	return nil, fmt.Errorf("missing url in .spec")
}

func UploadImage(ctx context.Context, bucket *blob.Bucket, key string, img image.Image) error {
	w, err := bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return err
	}
	defer w.Close()

	if err = png.Encode(w, img); err != nil {
		return err
	}

	return nil
}
