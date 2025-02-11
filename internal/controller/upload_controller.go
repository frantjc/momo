package controller

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"strings"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"gocloud.dev/blob"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// UploadReconciler reconciles a Upload object
type UploadReconciler struct {
	client.Client
	record.EventRecorder
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=uploads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=uploads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=uploads/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *UploadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_      = log.FromContext(ctx)
		upload = &momov1alpha1.Upload{}
	)

	if err := r.Get(ctx, req.NamespacedName, upload); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	defer func() {
		_ = r.Client.Status().Update(ctx, upload)
	}()

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: upload.ObjectMeta.Namespace,
			Name:      upload.Spec.Bucket.Name,
		},
		bucket,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if bucket.Status.Phase != "Ready" {
		return ctrl.Result{}, nil
	}

	cli, err := bucket.Open(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	rc, err := cli.NewReader(ctx, upload.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	zr, err := gzip.NewReader(rc)
	if err != nil {
		return ctrl.Result{}, err
	}

	var (
		tr         = tar.NewReader(zr)
		mobileApps = momov1alpha1.MobileAppList{}
		images     = map[string]momov1alpha1.MobileAppSpecImage{}
	)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		switch {
		case err != nil:
			return ctrl.Result{}, err
		case strings.HasPrefix(hdr.Name, "._"):
			continue
		}

		var (
			base = strings.ToLower(filepath.Base(hdr.Name))
			ext  = strings.ToLower(filepath.Ext(base))
			key  = filepath.Join(
				filepath.Dir(upload.Spec.Key),
				upload.Name,
				base,
			)
		)
		switch {
		case strings.EqualFold(ext, ".apk"):
			if err = cli.Upload(ctx, key, tr, &blob.WriterOptions{
				ContentType: momoutil.ContentTypeAPK,
			}); err != nil {
				return ctrl.Result{}, err
			}

			mobileApps.Items = append(mobileApps.Items,
				momov1alpha1.MobileApp{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: upload.Namespace,
						Name:      upload.Name + "-apk",
					},
					Spec: momov1alpha1.MobileAppSpec{
						SpecBucketKeyRef: momov1alpha1.SpecBucketKeyRef{
							Bucket: upload.Spec.Bucket,
							Key:    key,
						},
						Type: momov1alpha1.MobileAppTypeAPK,
					},
				},
			)
		case strings.EqualFold(ext, ".ipa"):
			if err = cli.Upload(ctx, key, tr, &blob.WriterOptions{
				ContentType: "application/octet-stream",
			}); err != nil {
				return ctrl.Result{}, err
			}

			mobileApps.Items = append(mobileApps.Items,
				momov1alpha1.MobileApp{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: upload.Namespace,
						Name:      upload.Name + "-ipa",
					},
					Spec: momov1alpha1.MobileAppSpec{
						SpecBucketKeyRef: momov1alpha1.SpecBucketKeyRef{
							Bucket: upload.Spec.Bucket,
							Key:    key,
						},
						Type: momov1alpha1.MobileAppTypeIPA,
					},
				},
			)
		case strings.EqualFold(ext, ".png"):
			switch {
			case strings.Contains(base, "full"):
				img, _, err := image.Decode(tr)
				if err != nil {
					return ctrl.Result{}, err
				}

				if err = writeImage(ctx, img, cli, key); err != nil {
					return ctrl.Result{}, err
				}

				images[momov1alpha1.MobileAppImageTypeFullSize] = momov1alpha1.MobileAppSpecImage{
					Key: key,
				}
			case strings.Contains(base, "display"):
				img, _, err := image.Decode(tr)
				if err != nil {
					return ctrl.Result{}, err
				}

				if err = writeImage(ctx, img, cli, key); err != nil {
					return ctrl.Result{}, err
				}

				images[momov1alpha1.MobileAppImageTypeDisplay] = momov1alpha1.MobileAppSpecImage{
					Key: key,
				}
			}
		}
	}

	for _, mobileApp := range mobileApps.Items {
		mobileApp.Spec.Images = images
		spec := mobileApp.Spec

		if err = controllerutil.SetControllerReference(upload, &mobileApp, r.Scheme()); err != nil {
			return ctrl.Result{}, err
		}

		if _, err := controllerutil.CreateOrPatch(ctx, r, &mobileApp, func() error {
			if err = controllerutil.SetControllerReference(upload, &mobileApp, r.Scheme()); err != nil {
				return err
			}

			mobileApp.Spec = spec

			return nil
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	upload.Status.Phase = "Ready"

	return ctrl.Result{}, nil
}

func writeImage(ctx context.Context, img image.Image, bucket *blob.Bucket, key string) error {
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

// SetupWithManager sets up the controller with the Manager.
func (r *UploadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.Upload{}).
		Named("upload").
		Complete(r)
}
