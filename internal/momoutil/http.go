package momoutil

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"howett.net/plist"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	namespaceParam  = `{namespace:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	bucketNameParam = `{bucket:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	appNameParam    = `{app:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	fileNameParam   = `{file}`
)

func NewHandler(scheme *runtime.Scheme, baseURL *url.URL) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)

	z := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	r.Get("/readyz", z)
	r.Get("/livez", z)
	r.Get("/healthz", z)

	r.Post(
		fmt.Sprintf("/api/v1/%s/%s/uploads/%s", namespaceParam, bucketNameParam, appNameParam),
		handleErr(handleUpload(scheme)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/%s/manifests/%s", namespaceParam, bucketNameParam, appNameParam),
		handleErr(handleManifests(scheme, baseURL)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/%s/files/%s/%s", namespaceParam, bucketNameParam, appNameParam, fileNameParam),
		handleErr(handleFiles(scheme, baseURL)),
	)

	r.NotFound(http.NotFound)

	return r
}

func handleErr(handler func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handler(w, r); err != nil {
			_ = respondErrorJSON(w, err, wantsPretty(r))
		}
	}
}

func handleUpload(scheme *runtime.Scheme) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return err
		}

		var (
			boundary    = params["boundary"]
			isMultipart = strings.EqualFold(mediaType, ContentTypeMultiPart)
			isTar       = strings.EqualFold(mediaType, ContentTypeTar)
			isApk       = strings.EqualFold(mediaType, ContentTypeAPK)
			isIpa       = strings.EqualFold(mediaType, ContentTypeIPA)
		)

		if isMultipart || isTar || isApk || isIpa {
			if isMultipart && boundary == "" {
				return newHTTPStatusCodeError(
					fmt.Errorf("missing boundary"),
					http.StatusBadRequest,
				)
			}

			cli, err := newClientForReq(r, scheme)
			if err != nil {
				return err
			}

			var (
				bucket     = &momov1alpha1.Bucket{}
				ctx        = r.Context()
				namespace  = chi.URLParam(r, "namespace")
				bucketName = chi.URLParam(r, "bucket")
				appName    = chi.URLParam(r, "app")
			)

			if err = cli.Get(ctx, client.ObjectKey{Name: bucketName, Namespace: namespace}, bucket); err != nil {
				return err
			}

			b, err := OpenBucket(ctx, cli, bucket)
			if err != nil {
				return err
			}

			var (
				key    = fmt.Sprintf("%s.tar.gz", appName)
				upload = &momov1alpha1.Upload{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: fmt.Sprintf("%s-", appName),
						Namespace:    namespace,
					},
					Spec: momov1alpha1.UploadSpec{
						Key: key,
						Bucket: corev1.LocalObjectReference{
							Name: bucket.Name,
						},
					},
				}
			)

			if err = cli.Create(ctx, upload); err != nil {
				return err
			}

			bw, err := b.NewWriter(ctx, key, &blob.WriterOptions{
				ContentType:     ContentTypeTar,
				ContentEncoding: "gzip",
			})
			if err != nil {
				return err
			}

			isGzip := strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip")
			if !isGzip {
				defer bw.Close()
			}

			var (
				wc   io.WriteCloser = bw
				body io.Reader      = r.Body
			)
			if !isGzip {
				wc = gzip.NewWriter(bw)
			}
			defer wc.Close()

			switch {
			case isMultipart:
				body = MultipartToTar(multipart.NewReader(r.Body, boundary), nil)
			case isApk:
				body = FileToTar(r.Body, "app.apk", nil)
			case isIpa:
				body = FileToTar(r.Body, "app.ipa", nil)
			}

			if _, err = io.Copy(wc, body); err != nil {
				return err
			}
		} else {
			return newHTTPStatusCodeError(
				fmt.Errorf("unsupported Content-Type %s", mediaType),
				http.StatusUnsupportedMediaType,
			)
		}

		if isMultipart {
			if referer := r.Header.Get("Referer"); referer != "" {
				return err
			}
		}

		w.WriteHeader(http.StatusCreated)
		return respondJSON(w, map[string]string{"hello": "there"}, wantsPretty(r))
	}
}

func handleManifests(scheme *runtime.Scheme, baseURL *url.URL) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		cli, err := newClientForReq(nil, scheme)
		if err != nil {
			return err
		}

		var (
			ctx        = r.Context()
			mobileApp  = &momov1alpha1.MobileApp{}
			namespace  = chi.URLParam(r, "namespace")
			bucketName = chi.URLParam(r, "bucket")
			appName    = chi.URLParam(r, "app")
		)

		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, mobileApp); err != nil {
			return err
		}

		values := url.Values{}
		values.Add("action", "download-manifest")
		values.Add("url", baseURL.JoinPath("/api/v1", namespace, bucketName, "files", appName, "manifest.plist").String())

		http.Redirect(w, r,
			(&url.URL{Scheme: "itms-services", RawQuery: values.Encode()}).String(),
			http.StatusMovedPermanently,
		)

		return nil
	}
}

func IsAPK(mobileApp *momov1alpha1.MobileApp) bool {
	return mobileApp.Spec.Type == momov1alpha1.MobileAppTypeAPK || strings.EqualFold(filepath.Ext(mobileApp.Spec.Key), ".apk")
}

func IsIPA(mobileApp *momov1alpha1.MobileApp) bool {
	return mobileApp.Spec.Type == momov1alpha1.MobileAppTypeIPA || strings.EqualFold(filepath.Ext(mobileApp.Spec.Key), ".ipa")
}

func handleFiles(scheme *runtime.Scheme, baseURL *url.URL) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var (
			ctx        = r.Context()
			mobileApp  = &momov1alpha1.MobileApp{}
			namespace  = strings.ToLower(chi.URLParam(r, "namespace"))
			bucketName = strings.ToLower(chi.URLParam(r, "bucket"))
			appName    = strings.ToLower(chi.URLParam(r, "app"))
			file       = strings.ToLower(chi.URLParam(r, "file"))
			ext        = filepath.Ext(file)
		)

		cli, err := newClientForReq(nil, scheme)
		if err != nil {
			return err
		}

		bucket, err := GetBucket(ctx, cli, client.ObjectKey{Namespace: namespace, Name: bucketName})
		if err != nil {
			return err
		}

		b, err := OpenBucket(ctx, cli, bucket)
		if err != nil {
			return err
		}

		if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, mobileApp); err != nil {
			return err
		}

		var (
			key         = mobileApp.Spec.Key
			contentType string
		)
		switch ext {
		case ".apk":
			if !IsAPK(mobileApp) {
				return fmt.Errorf("app is not an .apk")
			}
			contentType = ContentTypeAPK
		case ".ipa":
			if !IsIPA(mobileApp) {
				return fmt.Errorf("app is not an .ipa")
			}
			contentType = ContentTypeIPA
		case ".png":
			var (
				findAny = func(image momov1alpha1.MobileAppStatusImage, _ int) bool {
					return true
				}
				find = findAny
			)

			if xslice.Some([]string{"57", "display"}, func(part string, _ int) bool {
				return strings.Contains(file, part)
			}) {
				find = func(image momov1alpha1.MobileAppStatusImage, _ int) bool {
					return image.Display
				}
			} else if xslice.Some([]string{"512", "full"}, func(part string, _ int) bool {
				return strings.Contains(file, part)
			}) {
				find = func(image momov1alpha1.MobileAppStatusImage, _ int) bool {
					return image.FullSize
				}
			}

			contentType = ContentTypePNG
			key = xslice.
				Coalesce(
					xslice.Find(mobileApp.Status.Images, find),
					xslice.Find(mobileApp.Status.Images, find),
				).
				Key
		default:
			if strings.EqualFold(file, "manifest.plist") {
				if mobileApp.Status.BundleIdentifier == "" || mobileApp.Status.BundleName == "" || mobileApp.Status.Version == "" {
					return newHTTPStatusCodeError(fmt.Errorf("not found"), http.StatusPreconditionFailed)
				} else if exists, _ := b.Exists(ctx, mobileApp.Spec.Key); !exists {
					return newHTTPStatusCodeError(fmt.Errorf("not found"), http.StatusNotFound)
				}

				enc := plist.NewEncoder(w)
				if wantsPretty(r) {
					enc.Indent("  ")
				}

				w.Header().Set("Content-Type", "application/xml")

				if err = enc.Encode(&ios.Manifest{
					Items: []ios.ManifestItem{
						{
							Assets: []ios.ManifestItemAsset{
								{
									Kind: "software-package",
									URL:  baseURL.JoinPath("/api/v1", namespace, bucketName, "files", appName, mobileApp.Spec.Key).String(),
								},
								{
									Kind: "full-size-image",
									URL:  baseURL.JoinPath("/api/v1", namespace, bucketName, "files", appName, "full-size.png").String(),
								},
								{
									Kind: "display-image",
									URL:  baseURL.JoinPath("/api/v1", namespace, bucketName, "files", appName, "display.png").String(),
								},
							},
							Metadata: &ios.ManifestItemMetadata{
								Kind:               "software",
								PlatformIdentifier: "com.apple.platform.iphoneos",
								BundleIdentifier:   mobileApp.Status.BundleIdentifier,
								BundleVersion:      mobileApp.Status.Version,
								Title:              mobileApp.Status.BundleName,
							},
						},
					},
				}); err != nil {
					return err
				}

				return nil
			}
		}
		rc, err := b.NewReader(ctx, key, nil)
		if gcerrors.Code(err) == gcerrors.NotFound || rc == nil {
			if ext == ".png" {
				w.Header().Set("Content-Type", contentType)

				if _, err := io.Copy(w, bytes.NewReader(questionMarkPNG)); err != nil {
					return err
				}

				return nil
			}

			return newHTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
		defer rc.Close()

		w.Header().Set("Content-Type", contentType)

		if _, err := io.Copy(w, rc); err != nil {
			return err
		}

		return nil
	}
}

func respondJSON(w http.ResponseWriter, a any, pretty bool) error {
	w.Header().Set("Content-Type", ContentTypeJSON)

	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}

	return enc.Encode(a)
}

func newHTTPStatusCodeError(err error, httpStatusCode int) error {
	if err == nil {
		return nil
	}

	if 600 >= httpStatusCode || httpStatusCode < 100 {
		httpStatusCode = 500
	}

	return &httpStatusCodeError{
		err:            err,
		httpStatusCode: httpStatusCode,
	}
}

type httpStatusCodeError struct {
	err            error
	httpStatusCode int
}

func (e *httpStatusCodeError) Error() string {
	if e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *httpStatusCodeError) Unwrap() error {
	return e.err
}

func httpStatusCode(err error) int {
	hscerr := &httpStatusCodeError{}
	if errors.As(err, &hscerr) {
		return hscerr.httpStatusCode
	}

	if apiStatus, ok := err.(apierrors.APIStatus); ok || errors.As(err, &apiStatus) {
		return int(apiStatus.Status().Code)
	}

	return http.StatusInternalServerError
}

func respondErrorJSON(w http.ResponseWriter, err error, pretty bool) error {
	w.WriteHeader(httpStatusCode(err))

	return respondJSON(w, map[string]string{"error": err.Error()}, pretty)
}

func wantsPretty(r *http.Request) bool {
	pretty, _ := strconv.ParseBool(r.URL.Query().Get("pretty"))
	return pretty
}

func newClientForReq(r *http.Request, scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	if r != nil {
		cfg.CertData = nil
		cfg.CertFile = ""
		cfg.KeyData = nil
		cfg.CertFile = ""
		cfg.BearerToken = ""
		cfg.BearerTokenFile = ""

		var (
			authorization = r.Header.Get("Authorization")
			ok            bool
		)
		cfg.Username, cfg.Password, ok = r.BasicAuth()
		if !ok && strings.HasPrefix(authorization, "Bearer ") {
			cfg.BearerToken = strings.TrimPrefix(authorization, "Bearer ")
		}
	}

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return cli, nil
}
