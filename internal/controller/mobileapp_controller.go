package controller

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
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

	cli, err := momoutil.OpenBucket(ctx, r.Client, bucket)
	if err != nil {
		return ctrl.Result{}, err
	}

	rc, err := cli.NewReader(ctx, mobileApp.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp("", strings.ToLower(fmt.Sprintf("*.%s", mobileApp.Spec.Type)))
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

	var (
		isAPK = momoutil.IsAPK(mobileApp)
		isIPA = momoutil.IsIPA(mobileApp)
	)

	switch {
	case isAPK && isIPA:
		return ctrl.Result{}, fmt.Errorf(".spec.key has mismatched extension with .spec.type")
	case isAPK:
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

		if err := r.uploadIcons(ctx, apk, cli, mobileApp); err != nil {
			return ctrl.Result{}, err
		}
	case isIPA:
		ipa := ios.NewIPADecoder(tmp.Name())
		defer ipa.Close()

		info, err := ipa.Info(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		mobileApp.Status.BundleName = xslice.Coalesce(info.CFBundleName, info.CFBundleDisplayName)
		mobileApp.Status.BundleIdentifier = info.CFBundleIdentifier
		mobileApp.Status.Version = xslice.Coalesce(info.CFBundleVersion, info.CFBundleShortVersionString)

		if err := r.uploadIcons(ctx, ipa, cli, mobileApp); err != nil {
			return ctrl.Result{}, err
		}
	}

	mobileApp.Status.Digest = dig.String()
	mobileApp.Status.Phase = "Ready"

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

type iconAppDecoder interface {
	Icons(context.Context) (io.Reader, error)
}

func (r *MobileAppReconciler) uploadIcons(ctx context.Context, iconAppDecoder iconAppDecoder, bucket *blob.Bucket, mobileApp *momov1alpha1.MobileApp) error {
	icons, err := iconAppDecoder.Icons(ctx)
	if err != nil {
		return err
	}

	mobileApp.Status.Images = []momov1alpha1.MobileAppStatusImage{}

	tr := tar.NewReader(icons)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
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

		if err = momoutil.UploadImage(ctx, bucket, key, img); err != nil {
			return err
		}

		mobileApp.Status.Images = append(mobileApp.Status.Images, momov1alpha1.MobileAppStatusImage{
			Key:    key,
			Height: height,
			Width:  width,
		})
	}

	var (
		lenImages    = len(mobileApp.Status.Images)
		fullSizeMrgn = math.MaxInt
		fullSizeIdx  = lenImages - 1
		displayMrgn  = math.MaxInt
		displayIdx   = lenImages - 1
	)
	for i, image := range mobileApp.Status.Images {
		if image.Height == image.Width {
			if mrgn := int(math.Abs(float64(momoutil.ImageFullSizePx - image.Width))); mrgn < fullSizeMrgn {
				fullSizeIdx = i
				fullSizeMrgn = mrgn
			}

			if mrgn := int(math.Abs(float64(momoutil.ImageDisplayPx - image.Width))); mrgn < displayMrgn {
				displayIdx = i
				displayMrgn = mrgn
			}
		}
	}

	if fullSizeIdx >= 0 {
		mobileApp.Status.Images[fullSizeIdx].FullSize = true
	}

	if displayIdx >= 0 {
		mobileApp.Status.Images[displayIdx].Display = true
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
