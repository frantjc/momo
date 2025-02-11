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
	"time"

	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"github.com/opencontainers/go-digest"
	"gocloud.dev/blob"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MobileAppReconciler reconciles a MobileApp object
type MobileAppReconciler struct {
	client.Client
	record.EventRecorder
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=mobileapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=mobileapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=mobileapps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MobileAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_         = log.FromContext(ctx)
		mobileApp = &momov1alpha1.MobileApp{}
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

	cli, err := bucket.Open(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	rc, err := cli.NewReader(ctx, mobileApp.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", fmt.Sprintf("*.%s", mobileApp.Spec.Type))
	if err != nil {
		return ctrl.Result{}, err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, rc); err != nil {
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

	mobileApp.Status.Digest = dig.String()

	if mobileApp.Spec.Images != nil {
		if mobileApp.Status.Images == nil {
			mobileApp.Status.Images = map[string]momov1alpha1.MobileAppStatusImage{}
		}

		for name, image := range mobileApp.Spec.Images {
			mobileApp.Status.Images[name] = momov1alpha1.MobileAppStatusImage(image)
		}
	}

	switch mobileApp.Spec.Type {
	case momov1alpha1.MobileAppTypeAPK:
		apk := android.NewAPKDecoder(tmp.Name())
		defer apk.Close()

		sha256CertFingerprints, err := apk.SHA256CertFingerprints(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		mobileApp.Status.SHA256CertFingerprints = sha256CertFingerprints

		metadata, err := apk.Metadata(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		mobileApp.Status.Version = xslice.Coalesce(metadata.VersionInfo.VersionName, metadata.Version, fmt.Sprint(metadata.VersionInfo.VersionCode))

		if mobileApp.Spec.Images == nil {
			if err := r.uploadBestFitIcons(ctx, apk, cli, mobileApp); err != nil {
				return ctrl.Result{}, err
			}
		}
	case momov1alpha1.MobileAppTypeIPA:
		ipa := ios.NewIPADecoder(tmp.Name())
		defer ipa.Close()

		info, err := ipa.Info(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		mobileApp.Status.BundleName = xslice.Coalesce(info.CFBundleName, info.CFBundleDisplayName)
		mobileApp.Status.BundleIdentifier = info.CFBundleIdentifier
		mobileApp.Status.Version = xslice.Coalesce(info.CFBundleVersion, info.CFBundleShortVersionString)

		if mobileApp.Spec.Images == nil {
			if err := r.uploadBestFitIcons(ctx, ipa, cli, mobileApp); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

type iconAppDecoder interface {
	Icons(context.Context) (io.Reader, error)
}

func (r *MobileAppReconciler) uploadBestFitIcons(ctx context.Context, iconAppDecoder iconAppDecoder, bucket *blob.Bucket, mobileApp *momov1alpha1.MobileApp) error {
	var (
		fullSizeImg image.Image
		displayImg  image.Image
	)

	icons, err := iconAppDecoder.Icons(ctx)
	if err != nil {
		return err
	}

	tr := tar.NewReader(icons)
	for {
		if _, err := tr.Next(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}

		img, _, err := image.Decode(tr)
		if err != nil {
			return err
		}

		var (
			bestFitFullSizeDimensions = 0
			bestFitDisplayDimensions  = 0
			imgDimensions             = img.Bounds().Dx()
		)
		if fullSizeImg != nil {
			bestFitFullSizeDimensions = fullSizeImg.Bounds().Dx()
		}

		if displayImg != nil {
			bestFitFullSizeDimensions = displayImg.Bounds().Dx()
		}

		if (bestFitFullSizeDimensions < 512 && imgDimensions > bestFitFullSizeDimensions) ||
			(bestFitFullSizeDimensions > 512 && imgDimensions < bestFitFullSizeDimensions && imgDimensions >= 512) {
			fullSizeImg = img
		}

		if (bestFitDisplayDimensions < 57 && imgDimensions > bestFitDisplayDimensions) ||
			(bestFitDisplayDimensions > 57 && imgDimensions < bestFitDisplayDimensions && imgDimensions >= 57) {
			displayImg = img
		}
	}

	if fullSizeImg != nil {
		key := filepath.Join(
			filepath.Dir(mobileApp.Spec.Key),
			mobileApp.Status.Digest,
			"fullSize.png",
		)

		if err = writeImage(ctx, fullSizeImg, bucket, key); err != nil {
			return err
		}

		mobileApp.Status.Images[momov1alpha1.MobileAppImageTypeDisplay] = momov1alpha1.MobileAppStatusImage{
			Key: key,
		}
	}

	if displayImg != nil {
		key := filepath.Join(
			filepath.Dir(mobileApp.Spec.Key),
			mobileApp.Status.Digest,
			"display.png",
		)

		if err = writeImage(ctx, fullSizeImg, bucket, key); err != nil {
			return err
		}

		mobileApp.Status.Images[momov1alpha1.MobileAppImageTypeFullSize] = momov1alpha1.MobileAppStatusImage{
			Key: key,
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MobileAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.MobileApp{}).
		Named("mobileapp").
		Complete(r)
}
