package controller

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	xslice "github.com/frantjc/x/slice"
	"github.com/opencontainers/go-digest"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// APKReconciler reconciles a APK object
type APKReconciler struct {
	client.Client
	record.EventRecorder
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *APKReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_         = log.FromContext(ctx)
		mobileApp = &momov1alpha1.APK{}
	)

	if err := r.Get(ctx, req.NamespacedName, mobileApp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	defer func() {
		_ = r.Client.Status().Update(ctx, mobileApp)
	}()

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: mobileApp.ObjectMeta.Namespace,
			Name:      mobileApp.Spec.Bucket.Name,
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

	cli, err := momoutil.OpenBucket(ctx, r.Client, bucket)
	if err != nil {
		return ctrl.Result{}, err
	}

	rc, err := cli.NewReader(ctx, mobileApp.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", "*.apk")
	if err != nil {
		return ctrl.Result{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, rc); err != nil {
		return ctrl.Result{}, err
	}

	if err = rc.Close(); err != nil {
		return ctrl.Result{}, err
	}

	dig, err := digest.FromReader(tmp)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = tmp.Close(); err != nil {
		return ctrl.Result{}, err
	}

	if dig.String() == mobileApp.Status.Digest {
		return ctrl.Result{}, nil
	}

	mobileApp.Status.Phase = "Pending"

	apkDecoder := android.NewAPKDecoder(tmp.Name())
	defer apkDecoder.Close()

	sha256CertFingerprints, err := apkDecoder.SHA256CertFingerprints(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	mobileApp.Status.SHA256CertFingerprints = sha256CertFingerprints

	metadata, err := apkDecoder.Metadata(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	mobileApp.Status.Version = xslice.Coalesce(metadata.VersionInfo.VersionName, metadata.Version, fmt.Sprint(metadata.VersionInfo.VersionCode))

	icons, err := apkDecoder.Icons(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	mobileApp.Status.Icons = []momov1alpha1.AppStatusIcon{}

	tr := tar.NewReader(icons)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return ctrl.Result{}, err
		}

		img, _, err := image.Decode(tr)
		if err != nil {
			// TODO: Event.
			continue
		}

		var (
			bounds = img.Bounds()
			height = bounds.Dy()
			width  = bounds.Dx()
			ext    = filepath.Ext(hdr.Name)
			key    = filepath.Join(
				filepath.Dir(mobileApp.Spec.Key),
				fmt.Sprintf("%s-%dx%d%s",
					strings.ToLower(strings.TrimSuffix(hdr.Name, ext)),
					height,
					width,
					".png",
				),
			)
		)

		if err = momoutil.UploadImage(ctx, cli, key, img); err != nil {
			return ctrl.Result{}, err
		}

		mobileApp.Status.Icons = append(mobileApp.Status.Icons, momov1alpha1.AppStatusIcon{
			Key:  key,
			Size: height,
		})
	}

	mobileApp.Status.Digest = dig.String()
	mobileApp.Status.Phase = "Ready"

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *APKReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.APK{}).
		Watches(&momov1alpha1.Bucket{}, r.EventHandler()).
		Named("apk").
		Complete(r)
}

func (r *APKReconciler) EventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		apks := &momov1alpha1.APKList{}

		if err := r.Client.List(ctx, apks, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
			return []ctrl.Request{}
		}

		return xslice.Map(
			xslice.Filter(apks.Items, func(apk momov1alpha1.APK, _ int) bool {
				return apk.Spec.Bucket.Name == obj.GetName()
			}),
			func(apk momov1alpha1.APK, _ int) ctrl.Request {
				return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&apk)}
			},
		)
	})
}
