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

	"github.com/frantjc/momo"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/opencontainers/go-digest"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Finalizer = "momo.frantj.cc"
)

const (
	imageDisplayPx  = 57
	imageFullSizePx = 512
)

type Object interface {
	GetConditions() []metav1.Condition
	SetConditions(conditions []metav1.Condition)
	client.Object
}

func setCondition(obj Object, condition metav1.Condition) bool {
	conditions := obj.GetConditions()
	if conditions == nil {
		conditions = []metav1.Condition{}
	}

	for i, c := range conditions {
		if c.Type == condition.Type {
			if c.Message != condition.Message || c.Reason != condition.Reason || c.Status != condition.Status {
				condition.LastTransitionTime = metav1.Now()
				condition.ObservedGeneration = obj.GetGeneration()
				conditions[i] = condition
				obj.SetConditions(conditions)
				return true
			}
			return false
		}
	}

	condition.LastTransitionTime = metav1.Now()
	condition.ObservedGeneration = obj.GetGeneration()
	conditions = append(conditions, condition)
	obj.SetConditions(conditions)
	return true
}

type BinaryObject interface {
	GetKey() string
	GetIcons() []momov1alpha1.AppStatusIcon
	SetPhase(string)
	Object
}

type IconDecoder interface {
	Icons(context.Context) (io.Reader, error)
}

func unpackIcons(ctx context.Context, cli *blob.Bucket, dec IconDecoder, obj BinaryObject, r record.EventRecorder) ([]momov1alpha1.AppStatusIcon, error) {
	icons, err := dec.Icons(ctx)
	if err != nil {
		return nil, err
	}

	status := []momov1alpha1.AppStatusIcon{}

	var (
		tr  = tar.NewReader(icons)
		dir = filepath.Join(
			filepath.Dir(obj.GetKey()),
			obj.GetNamespace(),
			obj.GetName(),
		)
	)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}

		img, _, err := image.Decode(tr)
		if err != nil {
			r.Eventf(obj, corev1.EventTypeWarning, "DecodeImage", "Could not decode image %s: %v", hdr.Name, err)
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
				dir,
				fmt.Sprintf("%s-%dx%d%s",
					strings.ToLower(strings.TrimSuffix(hdr.Name, ext)),
					height,
					width,
					momo.ExtPNG,
				),
			)
		)

		if err = momoutil.UploadImage(ctx, cli, key, img); err != nil {
			return nil, err
		}

		status = append(status, momov1alpha1.AppStatusIcon{
			Key:  key,
			Size: height,
		})
	}

	return status, nil
}

func ignoreNotFound(err error) error {
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func downloadAndSumObject(ctx context.Context, cli client.Client, bucket *blob.Bucket, obj BinaryObject, ext, dir string) (string, digest.Digest, error) {
	var (
		upper = strings.ToUpper(strings.TrimPrefix(ext, "."))
	)

	rc, err := bucket.NewReader(ctx, obj.GetKey(), nil)
	if err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "ReadObject",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}
	defer func() {
		_ = rc.Close()
	}()

	tmp, err := os.CreateTemp(dir, fmt.Sprintf("*%s", ext))
	if err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "CreateTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}
	defer func() {
		_ = tmp.Close()
	}()

	if _, err := io.Copy(tmp, rc); err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "WriteTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}

	if err = rc.Close(); err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "CloseObject",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}

	dig, err := digest.FromReader(tmp)
	if err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "SumTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}

	if err = tmp.Close(); err != nil {
		obj.SetPhase(momov1alpha1.PhaseFailed)
		setCondition(obj, metav1.Condition{
			Type:    fmt.Sprintf("Get%s", upper),
			Reason:  "CloseTemp",
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})

		return "", "", cli.Status().Update(ctx, obj)
	}

	if setCondition(obj, metav1.Condition{
		Type:   fmt.Sprintf("Get%s", upper),
		Reason: "Downloaded",
		Status: metav1.ConditionTrue,
	}) {
		if err := cli.Status().Update(ctx, obj); err != nil {
			return "", "", err
		}
	}

	return tmp.Name(), dig, nil
}

func finalize(ctx context.Context, cli client.Client, recorder record.EventRecorder, bucket *blob.Bucket, obj BinaryObject) (ctrl.Result, error) {
	for _, icon := range obj.GetIcons() {
		err := bucket.Delete(ctx, icon.Key)
		switch gcerrors.Code(err) {
		case gcerrors.NotFound, gcerrors.OK:
		default:
			recorder.Eventf(obj, corev1.EventTypeWarning, "DeleteObject", "Deleting icon %s: %v", icon.Key, err)
			return ctrl.Result{RequeueAfter: time.Minute * 9}, nil
		}
	}

	if controllerutil.RemoveFinalizer(obj, Finalizer) {
		return ctrl.Result{}, ignoreNotFound(cli.Update(ctx, obj))
	}

	return ctrl.Result{}, nil
}
