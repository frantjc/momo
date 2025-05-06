package controller

import (
	"context"
	"io"
	"math"
	"os"
	"time"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	xstrings "github.com/frantjc/x/strings"
	"github.com/opencontainers/go-digest"
	"gocloud.dev/gcerrors"
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

// IPAReconciler reconciles an IPA object
type IPAReconciler struct {
	client.Client
	record.EventRecorder
	TmpDir string
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas/finalizers,verbs=update
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_   = log.FromContext(ctx)
		ipa = &momov1alpha1.IPA{}
	)

	if err := r.Get(ctx, req.NamespacedName, ipa); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if ipa.Status.Phase != momov1alpha1.PhasePending {
		ipa.Status.Phase = momov1alpha1.PhasePending

		if err := r.Client.Status().Update(ctx, ipa); err != nil {
			return ctrl.Result{}, ignoreNotFound(err)
		}
	}

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: ipa.Namespace,
			Name:      ipa.Spec.Bucket.Name,
		},
		bucket,
	); err != nil {
		if apierrors.IsNotFound(err) {
			r.Eventf(ipa, corev1.EventTypeWarning, "BucketNotFound", "Bucket %s is not found", bucket.Name)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if bucket.Status.Phase != momov1alpha1.PhaseReady {
		r.Eventf(ipa, corev1.EventTypeNormal, "BucketNotReady", "Bucket %s is not ready", bucket.Name)
		return ctrl.Result{}, nil
	}

	cli, err := momoutil.OpenBucket(ctx, r.Client, bucket)
	if err != nil {
		// If the Bucket is Ready, then this error
		// is likely temporary and will resolve itself.
		return ctrl.Result{Requeue: true}, nil
	}

	if !ipa.DeletionTimestamp.IsZero() {
		for _, icon := range ipa.Status.Icons {
			switch gcerrors.Code(cli.Delete(ctx, icon.Key)) {
			case gcerrors.NotFound, gcerrors.OK:
			default:
				r.Eventf(ipa, corev1.EventTypeWarning, "DeleteObject", "Deleting icon %s from bucket %s: %v", icon.Key, bucket.Name, err)
				return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
			}
		}

		if controllerutil.RemoveFinalizer(ipa, Finalizer) {
			return ctrl.Result{}, ignoreNotFound(r.Update(ctx, ipa))
		}

		return ctrl.Result{}, nil
	}

	rc, err := cli.NewReader(ctx, ipa.Spec.Key, nil)
	if err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "ReadObject",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}
	defer func() {
		_ = rc.Close()
	}()

	tmp, err := os.CreateTemp(r.TmpDir, "*.ipa")
	if err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "CreateTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	defer func() {
		_ = tmp.Close()
	}()

	if _, err := io.Copy(tmp, rc); err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "WriteTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}

	if err = rc.Close(); err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "CloseObject",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}

	dig, err := digest.FromReader(tmp)
	if err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "SumTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}

	if err = tmp.Close(); err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "GetIPA",
			Reason:  "CloseTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}

	if setCondition(ipa, metav1.Condition{
		Type:   "GetIPA",
		Reason: "Downloaded",
		Status: metav1.ConditionTrue,
	}) {
		if err := r.Client.Status().Update(ctx, ipa); err != nil {
			return ctrl.Result{}, ignoreNotFound(err)
		}
	}

	if dig.String() == ipa.Status.Digest {
		ipa.Status.Phase = momov1alpha1.PhaseReady
		return ctrl.Result{}, ignoreNotFound(r.Client.Status().Update(ctx, ipa))
	}

	ipaDecoder := ios.NewIPADecoder(tmp.Name())
	defer func() {
		_ = ipaDecoder.Close()
	}()

	info, err := ipaDecoder.Info(ctx)
	if err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "UnpackIPA",
			Reason:  "Info.plist",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Update(ctx, ipa))
	}

	ipa.Status.BundleName = xslice.Coalesce(info.CFBundleName, info.CFBundleDisplayName)
	ipa.Status.BundleIdentifier = info.CFBundleIdentifier
	ipa.Status.Version = semver.Canonical(
		xstrings.EnsurePrefix(
			xslice.Coalesce(info.CFBundleVersion, info.CFBundleShortVersionString),
			"v",
		),
	)

	if err := r.Client.Status().Update(ctx, ipa); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	if controllerutil.AddFinalizer(ipa, Finalizer) {
		if err := r.Update(ctx, ipa); err != nil {
			return ctrl.Result{}, ignoreNotFound(err)
		}
	}

	ipa.Status.Icons, err = unpackIcons(ctx, cli, ipaDecoder, ipa, r.EventRecorder)
	if err != nil {
		ipa.Status.Phase = momov1alpha1.PhaseFailed
		setCondition(ipa, metav1.Condition{
			Type:    "UnpackIPA",
			Reason:  "Icons",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return ctrl.Result{}, ignoreNotFound(r.Status().Update(ctx, ipa))
	}

	var (
		lenIcons     = len(ipa.Status.Icons)
		fullSizeMrgn = math.MaxInt
		fullSizeIdx  = lenIcons - 1
		displayMrgn  = math.MaxInt
		displayIdx   = lenIcons - 1
	)
	for i, image := range ipa.Status.Icons {
		if mrgn := int(math.Abs(float64(imageFullSizePx - image.Size))); mrgn < fullSizeMrgn {
			fullSizeIdx = i
			fullSizeMrgn = mrgn
		}

		if mrgn := int(math.Abs(float64(imageDisplayPx - image.Size))); mrgn < displayMrgn {
			displayIdx = i
			displayMrgn = mrgn
		}
	}

	if fullSizeIdx >= 0 {
		ipa.Status.Icons[fullSizeIdx].FullSize = true
	}

	if displayIdx >= 0 {
		ipa.Status.Icons[displayIdx].Display = true
	}

	ipa.Status.Digest = dig.String()
	ipa.Status.Phase = momov1alpha1.PhaseReady
	setCondition(ipa, metav1.Condition{
		Type:   "UnpackIPA",
		Reason: "Unpacked",
		Status: metav1.ConditionTrue,
	})

	if err := r.Client.Status().Update(ctx, ipa); err != nil {
		return ctrl.Result{}, ignoreNotFound(err)
	}

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.IPA{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&momov1alpha1.Bucket{}, r.EventHandler()).
		Named("ipa").
		Complete(r)
}

func (r *IPAReconciler) EventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		ipas := &momov1alpha1.IPAList{}

		if err := r.List(ctx, ipas, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
			return []ctrl.Request{}
		}

		return xslice.Map(
			xslice.Filter(ipas.Items, func(ipa momov1alpha1.IPA, _ int) bool {
				return ipa.Spec.Bucket.Name == obj.GetName()
			}),
			func(ipa momov1alpha1.IPA, _ int) ctrl.Request {
				return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&ipa)}
			},
		)
	})
}
