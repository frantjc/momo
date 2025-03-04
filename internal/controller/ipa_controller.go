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

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/frantjc/momo/ios"
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

// IPAReconciler reconciles a IPA object
type IPAReconciler struct {
	client.Client
	record.EventRecorder
	TmpDir string
}

// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=ipas/finalizers,verbs=update
// +kubebuilder:rbac:groups=momo.frantj.cc,resources=buckets,verbs=get;list;watch

const (
	imageDisplayPx  = 57
	imageFullSizePx = 512
)

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

	defer func() {
		_ = r.Client.Status().Update(ctx, ipa)
	}()

	bucket := &momov1alpha1.Bucket{}

	if err := r.Get(ctx,
		client.ObjectKey{
			Namespace: ipa.ObjectMeta.Namespace,
			Name:      ipa.Spec.Bucket.Name,
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

	if !ipa.DeletionTimestamp.IsZero() {
		if err := cli.Delete(ctx, ipa.Spec.Key); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	rc, err := cli.NewReader(ctx, ipa.Spec.Key, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(r.TmpDir, "*.ipa")
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

	forceUnpack := shouldForceUnpack(ipa)
	if !forceUnpack && dig.String() == ipa.Status.Digest {
		return ctrl.Result{}, nil
	}

	ipa.Status.Phase = momov1alpha1.PhasePending

	ipaDecoder := ios.NewIPADecoder(tmp.Name())
	defer ipaDecoder.Close()

	info, err := ipaDecoder.Info(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	ipa.Status.BundleName = xslice.Coalesce(info.CFBundleName, info.CFBundleDisplayName)
	ipa.Status.BundleIdentifier = info.CFBundleIdentifier
	ipa.Status.Version = semver.Canonical(
		xstrings.EnsurePrefix(
			xslice.Coalesce(info.CFBundleVersion, info.CFBundleShortVersionString),
			"v",
		),
	)

	icons, err := ipaDecoder.Icons(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	ipa.Status.Icons = []momov1alpha1.AppStatusIcon{}

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
		)

		if height != width {
			continue
		}

		var (
			ext = filepath.Ext(hdr.Name)
			key = filepath.Join(
				filepath.Dir(ipa.Spec.Key),
				ipa.Namespace,
				ipa.Name,
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

		ipa.Status.Icons = append(ipa.Status.Icons, momov1alpha1.AppStatusIcon{
			Key:  key,
			Size: height,
		})
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
	if forceUnpack {
		delete(ipa.Annotations, AnnotationForceUnpack)
		if err := r.Update(ctx, ipa); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.EventRecorder = mgr.GetEventRecorderFor("momo")
	return ctrl.NewControllerManagedBy(mgr).
		For(&momov1alpha1.IPA{}).
		Named("ipa").
		Complete(r)
}

func (r *IPAReconciler) EventHandler() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		ipas := &momov1alpha1.IPAList{}

		if err := r.Client.List(ctx, ipas, &client.ListOptions{Namespace: obj.GetNamespace()}); err != nil {
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
