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
	"strconv"
	"strings"
	"time"

	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	xslice "github.com/frantjc/x/slice"
	xstrings "github.com/frantjc/x/strings"
	"github.com/opencontainers/go-digest"
	"golang.org/x/mod/semver"
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
	TmpDir string
}

const (
	AnnotationForceUnpack = "momo.frantj.cc/force-unpack"
)

func shouldForceUnpack(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	b, _ := strconv.ParseBool(annotations[AnnotationForceUnpack])
	return b
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks/finalizers,verbs=update
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *APKReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_   = log.FromContext(ctx)
		apk = &momov1alpha1.APK{}
	)

	if err := r.Get(ctx, req.NamespacedName, apk); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: apk.ObjectMeta.Namespace,
			Name:      apk.Spec.Bucket.Name,
		},
		bucket,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if bucket.Status.Phase != momov1alpha1.PhaseReady {
		return ctrl.Result{}, nil
	}

	cli, err := momoutil.OpenBucket(ctx, r.Client, bucket)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !apk.DeletionTimestamp.IsZero() {
		if err := cli.Delete(ctx, apk.Spec.Key); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	defer func() {
		_ = r.Client.Status().Update(ctx, apk)
	}()

	rc, err := cli.NewReader(ctx, apk.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(r.TmpDir, "*.apk")
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

	forceUnpack := shouldForceUnpack(apk)

	if !forceUnpack && dig.String() == apk.Status.Digest {
		return ctrl.Result{}, nil
	}

	apk.Status.Phase = momov1alpha1.PhasePending

	apkDecoder := android.NewAPKDecoder(tmp.Name())
	defer apkDecoder.Close()

	sha256CertFingerprints, err := apkDecoder.SHA256CertFingerprints(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	apk.Status.SHA256CertFingerprints = sha256CertFingerprints

	metadata, err := apkDecoder.Metadata(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	apk.Status.Version = semver.Canonical(
		xstrings.EnsurePrefix(
			xslice.Coalesce(metadata.VersionInfo.VersionName, metadata.Version, fmt.Sprint(metadata.VersionInfo.VersionCode)),
			"v",
		),
	)

	manifest, err := apkDecoder.Manifest(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	apk.Status.Package = manifest.Package()

	icons, err := apkDecoder.Icons(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	apk.Status.Icons = []momov1alpha1.AppStatusIcon{}

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
				filepath.Dir(apk.Spec.Key),
				apk.Namespace,
				apk.Name,
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

		apk.Status.Icons = append(apk.Status.Icons, momov1alpha1.AppStatusIcon{
			Key:  key,
			Size: height,
		})
	}

	apk.Status.Digest = dig.String()
	apk.Status.Phase = momov1alpha1.PhaseReady
	if forceUnpack {
		delete(apk.Annotations, AnnotationForceUnpack)
		if err := r.Update(ctx, apk); err != nil {
			return ctrl.Result{}, err
		}
	}

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
