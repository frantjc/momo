package controller

import (
	"context"
	"time"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	xslice "github.com/frantjc/x/slice"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	record.EventRecorder
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		_      = log.FromContext(ctx)
		bucket = &momov1alpha1.Bucket{}
	)

	if err := r.Get(ctx, req.NamespacedName, bucket); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	bucket.Status.Phase = momov1alpha1.PhasePending

	defer func() {
		_ = r.Client.Status().Update(ctx, bucket)
	}()

	if _, err := momoutil.OpenBucket(ctx, r.Client, bucket); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	bucket.Status.Phase = momov1alpha1.PhaseReady

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.Bucket{}).
		Watches(&corev1.Secret{}, r.EventHandler(func(bucket momov1alpha1.Bucket, obj client.Object) bool {
			return bucket.Namespace == obj.GetNamespace() &&
				bucket.Spec.URLFrom != nil &&
				bucket.Spec.URLFrom.SecretKeyRef != nil &&
				bucket.Spec.URLFrom.SecretKeyRef.Name == obj.GetName()
		})).
		Watches(&corev1.ConfigMap{}, r.EventHandler(func(bucket momov1alpha1.Bucket, obj client.Object) bool {
			return bucket.Namespace == obj.GetNamespace() &&
				bucket.Spec.URLFrom != nil &&
				bucket.Spec.URLFrom.ConfigMapKeyRef != nil &&
				bucket.Spec.URLFrom.ConfigMapKeyRef.Name == obj.GetName()
		})).
		Named("bucket").
		Complete(r)
}

func (r *BucketReconciler) EventHandler(filter func(bucket momov1alpha1.Bucket, obj client.Object) bool) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		buckets := &momov1alpha1.BucketList{}

		if err := r.List(ctx, buckets, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
			return []ctrl.Request{}
		}

		return xslice.Map(
			xslice.Filter(buckets.Items, func(bucket momov1alpha1.Bucket, _ int) bool {
				return filter(bucket, obj)
			}),
			func(bucket momov1alpha1.Bucket, _ int) ctrl.Request {
				return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&bucket)}
			},
		)
	})
}
