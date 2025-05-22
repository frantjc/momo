package momoutil

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/ios"
	"github.com/google/uuid"
	"gocloud.dev/blob"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func GetBucket(ctx context.Context, cli client.Client, key client.ObjectKey) (*momov1alpha1.Bucket, error) {
	bucket := &momov1alpha1.Bucket{}

	if err := cli.Get(ctx, key, bucket); err != nil {
		return nil, err
	}

	return bucket, nil
}

var (
	mu sync.Mutex
)

func openBucket(ctx context.Context, urlstr string) (*blob.Bucket, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}

	if envs := u.Query()["env"]; len(envs) > 0 {
		mu.Lock()
		defer mu.Unlock()

		for _, env := range envs {
			parts := strings.Split(env, "=")

			if len(parts) >= 2 {
				prev, ok := os.LookupEnv(parts[0])
				if ok {
					defer func() {
						_ = os.Setenv(parts[0], prev)
					}()
				} else {
					defer func() {
						_ = os.Unsetenv(parts[0])
					}()
				}

				if err := os.Setenv(parts[0], strings.Join(parts[1:], "=")); err != nil {
					return nil, fmt.Errorf("set env %s: %w", parts[0], err)
				}
			}
		}

		q := u.Query()
		q.Del("env")
		u.RawQuery = q.Encode()

		return blob.OpenBucket(ctx, u.String())
	}

	return blob.OpenBucket(ctx, urlstr)
}

func OpenBucket(ctx context.Context, cli client.Client, bucket *momov1alpha1.Bucket) (*blob.Bucket, error) {
	if bucket.Spec.URL != "" {
		return openBucket(ctx, bucket.Spec.URL)
	} else if bucket.Spec.URLFrom != nil {
		if bucket.Spec.URLFrom.ConfigMapKeyRef != nil {
			configMap := &corev1.ConfigMap{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      bucket.Spec.URLFrom.ConfigMapKeyRef.Name,
					Namespace: bucket.Namespace,
				},
				configMap,
			); err != nil {
				return nil, fmt.Errorf("get ConfigMap: %w", err)
			}

			value, ok := configMap.Data[bucket.Spec.URLFrom.ConfigMapKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in ConfigMap %s", bucket.Spec.URLFrom.ConfigMapKeyRef.Key, bucket.Spec.URLFrom.ConfigMapKeyRef.Name)
			}

			return openBucket(ctx, value)
		}

		if bucket.Spec.URLFrom.SecretKeyRef != nil {
			secret := &corev1.Secret{}
			if err := cli.Get(ctx,
				client.ObjectKey{
					Name:      bucket.Spec.URLFrom.SecretKeyRef.Name,
					Namespace: bucket.Namespace,
				},
				secret,
			); err != nil {
				return nil, fmt.Errorf("get Secret: %w", err)
			}

			value, ok := secret.Data[bucket.Spec.URLFrom.SecretKeyRef.Key]
			if !ok {
				return nil, fmt.Errorf("get key %s in Secret %s", bucket.Spec.URLFrom.SecretKeyRef.Key, bucket.Spec.URLFrom.SecretKeyRef.Name)
			}

			return openBucket(ctx, string(value))
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
	w, err := bucket.NewWriter(ctx, key, &blob.WriterOptions{
		ContentType: "image/png",
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = w.Close()
	}()

	if err = png.Encode(w, img); err != nil {
		return err
	}

	return nil
}

func NewHTTPStatusCodeError(err error, httpStatusCode int) error {
	if err == nil {
		return nil
	}

	return &httpStatusCodeError{
		err:            err,
		httpStatusCode: httpStatusCode,
	}
}

type httpStatusCodeError struct {
	err            error
	httpStatusCode int
}

func (e *httpStatusCodeError) Error() string {
	if e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *httpStatusCodeError) Unwrap() error {
	return e.err
}

const (
	LabelApp = "momo.frantj.cc/app"
)

func HTTPStatusCode(err error) int {
	hscerr := &httpStatusCodeError{}
	if errors.As(err, &hscerr) {
		return hscerr.httpStatusCode
	}

	if apiStatus, ok := err.(apierrors.APIStatus); ok || errors.As(err, &apiStatus) {
		if code := int(apiStatus.Status().Code); code != 0 {
			return code
		}
	}

	return http.StatusInternalServerError
}

func UploadApp(ctx context.Context, cli client.Client, namespace, name, bucketName, mediaType string, r io.Reader) error {
	var (
		bucket = &momov1alpha1.Bucket{}
		ext    = momo.ExtIPA
	)

	switch mediaType {
	case android.ContentTypeAPK:
		ext = momo.ExtAPK
	case ios.ContentTypeIPA:
	default:
		return NewHTTPStatusCodeError(fmt.Errorf("unsupported Content-Type %s", mediaType), http.StatusUnsupportedMediaType)
	}

	if err := cli.Get(ctx, client.ObjectKey{Name: bucketName, Namespace: namespace}, bucket); err != nil {
		return err
	}

	b, err := OpenBucket(ctx, cli, bucket)
	if err != nil {
		return err
	}

	var (
		artifactName = fmt.Sprintf("%s-%s", name, uuid.NewString()[:5])
		key          = fmt.Sprintf("%s%s", artifactName, ext)
		selector     = map[string]string{
			LabelApp: name,
		}
		mobileApp = &momov1alpha1.MobileApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Spec: momov1alpha1.MobileAppSpec{
				Selector: selector,
			},
		}
	)
	switch mediaType {
	case android.ContentTypeAPK:
		if err = cli.Create(ctx,
			&momov1alpha1.APK{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      artifactName,
					Labels:    selector,
				},
				Spec: momov1alpha1.APKSpec{
					Bucket: corev1.LocalObjectReference{
						Name: bucketName,
					},
					Key: key,
				},
			},
		); err != nil {
			return err
		}
	case ios.ContentTypeIPA:
		if err = cli.Create(ctx,
			&momov1alpha1.IPA{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      artifactName,
					Labels:    selector,
				},
				Spec: momov1alpha1.IPASpec{
					Bucket: corev1.LocalObjectReference{
						Name: bucketName,
					},
					Key: key,
				},
			},
		); err != nil {
			return err
		}
	}
	wc, err := b.NewWriter(ctx, key, &blob.WriterOptions{ContentType: mediaType})
	if err != nil {
		return err
	}
	defer func() {
		_ = wc.Close()
	}()

	if _, err = io.Copy(wc, r); err != nil {
		return err
	}

	if _, err = controllerutil.CreateOrUpdate(ctx, cli, mobileApp, func() error {
		mobileApp.Spec.Selector = selector
		return nil
	}); err != nil {
		return err
	}

	return nil
}
