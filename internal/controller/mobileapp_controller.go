package controller

import (
	"context"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	xslice "github.com/frantjc/x/slice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MobileAppReconciler reconciles a MobileApp object
type MobileAppReconciler struct {
	client.Client
	record.EventRecorder
}

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

	apks := &momov1alpha1.APKList{}

	if err := r.List(ctx,
		apks,
		&client.ListOptions{
			Namespace:     mobileApp.Namespace,
			LabelSelector: labels.SelectorFromSet(mobileApp.Spec.Selector),
		},
	); err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		_ = r.Client.Status().Update(ctx, mobileApp)
	}()

	mobileApp.Status.Phase = "Pending"
	mobileApp.Status.APKs = []momov1alpha1.MobileAppStatusApp{}
	mobileApp.Status.AssetLinkTargets = []momov1alpha1.MobileAppStatusTarget{}

	packageToSHA256CertFingerprints := map[string][]string{}

	for _, apk := range apks.Items {
		if apk.Status.Phase == "Ready" {
			mobileApp.Status.APKs = append(mobileApp.Status.APKs, momov1alpha1.MobileAppStatusApp{
				Name:    apk.Name,
				Bucket:  apk.Spec.Bucket,
				Key:     apk.Spec.Key,
				Version: apk.Status.Version,
			})

			if apk.Status.SHA256CertFingerprints != "" && apk.Status.Package != "" {
				if sha256CertFingerprints, ok := packageToSHA256CertFingerprints[apk.Status.Package]; ok {
					packageToSHA256CertFingerprints[apk.Status.Package] = append(sha256CertFingerprints, apk.Status.SHA256CertFingerprints)
				} else {
					packageToSHA256CertFingerprints[apk.Status.Package] = []string{apk.Status.SHA256CertFingerprints}
				}
			}
		}
	}

	for packageName, sha256CertFingerprints := range packageToSHA256CertFingerprints {
		mobileApp.Status.AssetLinkTargets = append(mobileApp.Status.AssetLinkTargets, momov1alpha1.MobileAppStatusTarget{
			Package:                packageName,
			SHA256CertFingerprints: sha256CertFingerprints,
		})
	}

	ipas := &momov1alpha1.IPAList{}

	if err := r.List(ctx,
		ipas,
		&client.ListOptions{
			Namespace:     mobileApp.Namespace,
			LabelSelector: labels.SelectorFromSet(mobileApp.Spec.Selector),
		},
	); err != nil {
		return ctrl.Result{}, err
	}

	mobileApp.Status.IPAs = []momov1alpha1.MobileAppStatusApp{}
	mobileApp.Status.AppleAppSiteAssociationAppIDs = []string{}

	for _, ipa := range ipas.Items {
		if ipa.Status.Phase == "Ready" {
			mobileApp.Status.IPAs = append(mobileApp.Status.IPAs, momov1alpha1.MobileAppStatusApp{
				Name:    ipa.Name,
				Bucket:  ipa.Spec.Bucket,
				Key:     ipa.Spec.Key,
				Version: ipa.Status.Version,
			})
		}

		if ipa.Status.BundleIdentifier != "" {
			mobileApp.Status.AppleAppSiteAssociationAppIDs = append(mobileApp.Status.AppleAppSiteAssociationAppIDs, ipa.Status.BundleIdentifier)
		}
	}

	mobileApp.Status.AppleAppSiteAssociationAppIDs = xslice.Unique(mobileApp.Status.AppleAppSiteAssociationAppIDs)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MobileAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.MobileApp{}).
		Watches(&momov1alpha1.APK{}, r.EventHandler()).
		Watches(&momov1alpha1.IPA{}, r.EventHandler()).
		Named("mobileapp").
		Complete(r)
}

func (r *MobileAppReconciler) EventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		if lbls := obj.GetLabels(); lbls != nil {
			mobileApps := &momov1alpha1.MobileAppList{}

			if err := r.Client.List(ctx, mobileApps, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
				return []ctrl.Request{}
			}

			return xslice.Map(
				xslice.Filter(mobileApps.Items, func(mobileApp momov1alpha1.MobileApp, _ int) bool {
					return mobileApp.Spec.Selector.AsSelector().Matches(labels.Set(lbls))
				}),
				func(mobileApp momov1alpha1.MobileApp, _ int) ctrl.Request {
					return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&mobileApp)}
				},
			)
		}

		return []ctrl.Request{}
	})
}
