package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	xslice "github.com/frantjc/x/slice"
	xstrings "github.com/frantjc/x/strings"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// APKReconciler reconciles an APK object
type APKReconciler struct {
	client.Client
	record.EventRecorder
	TmpDir string
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
		return ctrl.Result{}, ignoreNotFound(err)
	}

	if apk.Status.Phase != momov1alpha1.PhasePending {
		apk.Status.Phase = momov1alpha1.PhasePending

		if err := r.Client.Status().Update(ctx, apk); err != nil {
			return ctrl.Result{}, ignoreNotFound(err)
		}
	}

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: apk.Namespace,
			Name:      apk.Spec.Bucket.Name,
		},
		bucket,
	); err != nil {
		if apierrors.IsNotFound(err) {
			r.Eventf(apk, corev1.EventTypeWarning, "BucketNotFound", "Bucket %s is not found", bucket.Name)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if bucket.Status.Phase != momov1alpha1.PhaseReady {
		r.Eventf(apk, corev1.EventTypeNormal, "BucketNotReady", "Bucket %s is not ready", bucket.Name)
		return ctrl.Result{}, nil
	}

	cli, err := momoutil.OpenBucket(ctx, r.Client, bucket)
	if err != nil {
		// If the Bucket is Ready, then this error
		// is likely temporary and will resolve itself.
		return ctrl.Result{Requeue: true}, nil
	}

	if !apk.DeletionTimestamp.IsZero() {
		return finalize(ctx, r, r, cli, apk)
	}

	path, dig, err := downloadAndSumObject(ctx, r, cli, apk, momo.ExtAPK, r.TmpDir)
	if err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}
	defer func() {
		_ = os.Remove(path)
	}()

	if dig.String() == apk.Status.Digest {
		apk.Status.Phase = momov1alpha1.PhaseReady
		return ctrl.Result{}, ignoreNotFound(r.Client.Status().Update(ctx, apk))
	}

	apkDecoder := android.NewAPKDecoder(path)
	defer func() {
		_ = apkDecoder.Close()
	}()

	sha256CertFingerprints, err := apkDecoder.SHA256CertFingerprints(ctx)
	if err != nil {
		apk.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(apk, metav1.Condition{
			Type:    "UnpackAPK",
			Reason:  "SHA256CertFingerprints",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, apk))
	}

	apk.Status.SHA256CertFingerprints = sha256CertFingerprints

	metadata, err := apkDecoder.Metadata(ctx)
	if err != nil {
		apk.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(apk, metav1.Condition{
			Type:    "UnpackAPK",
			Reason:  "APKToolMetadata",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, apk))
	}

	apk.Status.Version = semver.Canonical(
		xstrings.EnsurePrefix(
			xslice.Coalesce(metadata.VersionInfo.VersionName, metadata.Version, fmt.Sprint(metadata.VersionInfo.VersionCode)),
			"v",
		),
	)

	manifest, err := apkDecoder.Manifest(ctx)
	if err != nil {
		apk.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(apk, metav1.Condition{
			Type:    "UnpackAPK",
			Reason:  "Manifest.xml",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, apk))
	}

	apk.Status.Package = manifest.Package()

	if err := r.Client.Status().Update(ctx, apk); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	if controllerutil.AddFinalizer(apk, Finalizer) {
		if err := r.Update(ctx, apk); err != nil {
			return ctrl.Result{}, ignoreNotFound(err)
		}
	}

	apk.Status.Icons, err = unpackIcons(ctx, cli, apkDecoder, apk, r.EventRecorder)
	if err != nil {
		apk.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(apk, metav1.Condition{
			Type:    "UnpackAPK",
			Reason:  "Icons",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, apk))
	}

	apk.Status.Digest = dig.String()
	apk.Status.Phase = momov1alpha1.PhaseReady
	setCondition(apk, metav1.Condition{
		Type:   "UnpackAPK",
		Reason: "Unpacked",
		Status: metav1.ConditionTrue,
	})

	if err := r.Client.Status().Update(ctx, apk); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *APKReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.APK{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&momov1alpha1.Bucket{}, r.EventHandler()).
		Named("apk").
		Complete(r)
}

func (r *APKReconciler) EventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		apks := &momov1alpha1.APKList{}

		if err := r.List(ctx, apks, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
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
