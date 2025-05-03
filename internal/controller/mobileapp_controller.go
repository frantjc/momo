package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"github.com/opencontainers/go-digest"
	"golang.org/x/mod/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=apks;ipas,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

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

	mobileApp.Status.Phase = momov1alpha1.PhasePending

	if err := r.Client.Status().Update(ctx, mobileApp); err != nil {
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

	mobileApp.Status.APKs = []momov1alpha1.MobileAppStatusApp{}

	packageToSHA256CertFingerprints := map[string][]string{}

	for _, apk := range apks.Items {
		if apk.Status.Phase == momov1alpha1.PhaseReady {
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

	mobileApp.Status.APKs = markLatest(mobileApp.Status.APKs)
	SetCondition(mobileApp, metav1.Condition{
		Type:   "AggregatedAPKs",
		Reason: "GotAPKs",
		Status: metav1.ConditionTrue,
	})

	if err := r.Client.Status().Update(ctx, mobileApp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
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
	bundleIdentifiers := []string{}

	for _, ipa := range ipas.Items {
		if ipa.Status.Phase == momov1alpha1.PhaseReady {
			mobileApp.Status.IPAs = append(mobileApp.Status.IPAs, momov1alpha1.MobileAppStatusApp{
				Name:    ipa.Name,
				Bucket:  ipa.Spec.Bucket,
				Key:     ipa.Spec.Key,
				Version: ipa.Status.Version,
			})

			bundleIdentifiers = append(bundleIdentifiers, ipa.Status.BundleIdentifier)
		}
	}

	mobileApp.Status.IPAs = markLatest(mobileApp.Status.IPAs)
	SetCondition(mobileApp, metav1.Condition{
		Type:   "AggregatedIPAs",
		Reason: "GotIPAs",
		Status: metav1.ConditionTrue,
	})

	if err := r.Client.Status().Update(ctx, mobileApp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if mobileApp.Spec.UniversalLinks.Ingress.Host != "" {
		bundleIdentifiers = xslice.Unique(bundleIdentifiers)

		var (
			configMapData = map[string]string{}
			podLabels     = map[string]string{
				"app.kubernetes.io/name": mobileApp.Name,
			}
			deploymentSpec = appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: podLabels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      podLabels,
						Annotations: map[string]string{},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "well-known",
								Image: "nginx:1-alpine",
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      mobileApp.Name,
										MountPath: "/usr/share/nginx/html/.well-known",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: mobileApp.Name,
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: mobileApp.Name,
										},
									},
								},
							},
						},
					},
				},
			}
			serviceSpec = corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port:       8080,
						TargetPort: intstr.FromInt(80),
					},
				},
				Selector: deploymentSpec.Selector.MatchLabels,
			}
			ingressSpec = networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{
					{
						Hosts:      []string{mobileApp.Spec.UniversalLinks.Ingress.Host},
						SecretName: mobileApp.Name,
					},
				},
				Rules: []networkingv1.IngressRule{
					{
						Host: mobileApp.Spec.UniversalLinks.Ingress.Host,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &[]networkingv1.PathType{"Prefix"}[0],
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: mobileApp.Name,
												Port: networkingv1.ServiceBackendPort{
													Number: serviceSpec.Ports[0].Port,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
		)

		if len(packageToSHA256CertFingerprints) > 0 {
			assetLinks := []android.AssetLink{}

			for packageName, sha256CertFingerprints := range packageToSHA256CertFingerprints {
				assetLinks = append(assetLinks, android.AssetLink{
					Relation: []string{"delegate_permission/common.handle_all_urls"},
					Target: android.Target{
						Namespace:              "android_app",
						PackageName:            packageName,
						SHA256CertFingerprints: xslice.Unique(sha256CertFingerprints),
					},
				})
			}

			assetLinksJSON, err := json.Marshal(assetLinks)
			if err != nil {
				return ctrl.Result{}, err
			}

			configMapData["assetlinks.json"] = string(assetLinksJSON)
			deploymentSpec.Template.Annotations["momo.frantj.cc/assetlinks-hash"] = digest.FromBytes(assetLinksJSON).Encoded()
		}

		if len(bundleIdentifiers) > 0 {
			appleAppSiteAssociation := ios.AppleAppSiteAssociation{
				AppLinks: ios.AppLinks{
					Details: []ios.Details{
						{
							AppIDs: bundleIdentifiers,
							Components: []ios.Component{
								{
									Path:    "/",
									Comment: "Matches any URL.",
								},
							},
						},
					},
				},
			}

			appleAppSiteAssociationJSON, err := json.Marshal(appleAppSiteAssociation)
			if err != nil {
				return ctrl.Result{}, err
			}

			configMapData["apple-app-site-association"] = string(appleAppSiteAssociationJSON)
			deploymentSpec.Template.Annotations["momo.frantj.cc/apple-app-site-association-hash"] = digest.FromBytes(appleAppSiteAssociationJSON).Encoded()
		}

		var (
			objectMeta = metav1.ObjectMeta{
				Namespace: mobileApp.Namespace,
				Name:      mobileApp.Name,
			}
			configMap = &corev1.ConfigMap{
				ObjectMeta: objectMeta,
				Data:       configMapData,
			}
			deployment = &appsv1.Deployment{
				ObjectMeta: objectMeta,
				Spec:       deploymentSpec,
			}
			service = &corev1.Service{
				ObjectMeta: objectMeta,
				Spec:       serviceSpec,
			}
			ingress = &networkingv1.Ingress{
				ObjectMeta: objectMeta,
				Spec:       ingressSpec,
			}
		)
		for _, obj := range []client.Object{configMap, deployment, service, ingress} {
			if err := controllerutil.SetOwnerReference(mobileApp, obj, r.Scheme()); err != nil {
				return ctrl.Result{}, err
			}
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r, configMap, func() error {
			configMap.Data = configMapData
			return controllerutil.SetOwnerReference(mobileApp, configMap, r.Scheme())
		}); err != nil {
			return ctrl.Result{}, err
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r, deployment, func() error {
			deployment.Spec = deploymentSpec
			return controllerutil.SetOwnerReference(mobileApp, deployment, r.Scheme())
		}); err != nil {
			return ctrl.Result{}, err
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r, service, func() error {
			service.Spec = serviceSpec
			return controllerutil.SetOwnerReference(mobileApp, service, r.Scheme())
		}); err != nil {
			return ctrl.Result{}, err
		}

		var (
			ingressClasses = &networkingv1.IngressClassList{}
		)

		if err := r.List(ctx, ingressClasses); err != nil {
			return ctrl.Result{}, err
		}

		for _, ingressClass := range ingressClasses.Items {
			if len(ingressClasses.Items) == 1 ||
				(ingressClass.Annotations != nil && ingressClass.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true") {
				ingressSpec.IngressClassName = &ingressClass.Name
				ingress.Spec.IngressClassName = &ingressClass.Name
			}
		}

		if issuer := mobileApp.Spec.UniversalLinks.Ingress.Issuer; issuer.Name != "" {
			switch issuer.APIVersion {
			case v1.SchemeGroupVersion.String(), "":
				return ctrl.Result{}, fmt.Errorf("unsupported %s apiVersion %s", issuer.APIVersion, issuer.Kind)
			}

			switch issuer.Kind {
			case v1.ClusterIssuerKind:
				if issuer.Namespace != "" {
					return ctrl.Result{}, fmt.Errorf("ClusterIssuer is cluster-scoped, but got Namespace %s", issuer.Namespace)
				}

				var (
					clusterIssuer = &v1.ClusterIssuer{}
					key           = client.ObjectKey{
						Name: issuer.Name,
					}
				)

				if err := r.Get(ctx, key, clusterIssuer); err != nil {
					return ctrl.Result{}, err
				}

				ingress.Annotations = map[string]string{"cert-manager.io/cluster-issuer": clusterIssuer.Name}
			case v1.IssuerKind, "":
				var (
					issuer = &v1.Issuer{}
					key    = client.ObjectKey{
						Namespace: mobileApp.Spec.UniversalLinks.Ingress.Issuer.Namespace,
						Name:      mobileApp.Spec.UniversalLinks.Ingress.Issuer.Name,
					}
				)
				if key.Namespace != mobileApp.Namespace {
					return ctrl.Result{}, fmt.Errorf("cannot use Issuer from different Namespace %s", key.Namespace)
				}

				if err := r.Get(ctx, key, issuer); err != nil {
					return ctrl.Result{}, err
				}

				ingress.Annotations = map[string]string{"cert-manager.io/issuer": issuer.Name}
			default:
				return ctrl.Result{}, fmt.Errorf("unsupported kind %s", mobileApp.Spec.UniversalLinks.Ingress.Issuer.Kind)
			}
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r, ingress, func() error {
			ingress.Spec = ingressSpec
			ingress.Annotations = map[string]string{"cert-manager.io/cluster-issuer": "letsencrypt"}
			return controllerutil.SetOwnerReference(mobileApp, ingress, r.Scheme())
		}); err != nil {
			return ctrl.Result{}, err
		}

		SetCondition(mobileApp, metav1.Condition{
			Type:   "CreatedOrUpdatedWellKnown",
			Reason: "Created",
			Status: metav1.ConditionTrue,
		})
	} else {
		SetCondition(mobileApp, metav1.Condition{
			Type:   "CreatedOrUpdatedWellKnown",
			Reason: "Skipped",
			Status: metav1.ConditionFalse,
		})
	}

	mobileApp.Status.Phase = momov1alpha1.PhaseReady

	return ctrl.Result{}, r.Client.Status().Update(ctx, mobileApp)
}

func markLatest(apps []momov1alpha1.MobileAppStatusApp) []momov1alpha1.MobileAppStatusApp {
	var (
		latest  = -1
		version string
	)
	for i, app := range apps {
		if version == "" || semver.Compare(version, app.Version) < 0 {
			latest = i
			version = app.Version
		}
	}

	if latest >= 0 {
		apps[latest].Latest = true
	}

	return apps
}

// SetupWithManager sets up the controller with the Manager.
func (r *MobileAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.MobileApp{}).
		Watches(&momov1alpha1.APK{}, r.BinaryEventHandler()).
		Watches(&momov1alpha1.IPA{}, r.BinaryEventHandler()).
		Watches(&v1.Issuer{}, r.IssuerEventHandler()).
		Watches(&v1.ClusterIssuer{}, r.IssuerEventHandler()).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Named("mobileapp").
		Complete(r)
}

func (r *MobileAppReconciler) BinaryEventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		if lbls := obj.GetLabels(); lbls != nil {
			mobileApps := &momov1alpha1.MobileAppList{}

			if err := r.List(ctx, mobileApps, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
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

func (r *MobileAppReconciler) IssuerEventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		if lbls := obj.GetLabels(); lbls != nil {
			mobileApps := &momov1alpha1.MobileAppList{}

			if err := r.List(ctx, mobileApps, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
				return []ctrl.Request{}
			}

			return xslice.Map(
				xslice.Filter(mobileApps.Items, func(mobileApp momov1alpha1.MobileApp, _ int) bool {
					return mobileApp.Spec.UniversalLinks.Ingress.Issuer.Name == obj.GetName() &&
						mobileApp.Spec.UniversalLinks.Ingress.Issuer.Namespace == obj.GetNamespace() &&
						(mobileApp.Spec.UniversalLinks.Ingress.Issuer.Kind == obj.GetObjectKind().GroupVersionKind().Kind || mobileApp.Spec.UniversalLinks.Ingress.Issuer.Kind == "") &&
						(mobileApp.Spec.UniversalLinks.Ingress.Issuer.APIVersion == v1.SchemeGroupVersion.String() || mobileApp.Spec.UniversalLinks.Ingress.Issuer.APIVersion == "") &&
						mobileApp.Spec.UniversalLinks.Ingress.Host != ""
				}),
				func(mobileApp momov1alpha1.MobileApp, _ int) ctrl.Request {
					return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&mobileApp)}
				},
			)
		}

		return []ctrl.Request{}
	})
}
