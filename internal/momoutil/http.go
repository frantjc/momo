package momoutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	paramNamespace = `{namespace:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramBucket    = `{bucket:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramApp       = `{app:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramVersion   = `{version}`
	paramFile      = `{file}`
)

const (
	labelApp = "momo.frantj.cc/app"
)

func NewHandler(scheme *runtime.Scheme) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)

	r.Post(
		fmt.Sprintf("/api/v1/%s/uploads/%s/%s", paramNamespace, paramBucket, paramApp),
		handleErr(handleUpload(scheme)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/manifests/%s", paramNamespace, paramApp),
		handleErr(handleManifests(scheme)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/manifests/%s/%s", paramNamespace, paramApp, paramVersion),
		handleErr(handleManifests(scheme)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/files/%s/%s", paramNamespace, paramApp, paramFile),
		handleErr(handleFiles(scheme)),
	)

	r.Get(
		fmt.Sprintf("/api/v1/%s/files/%s/%s/%s", paramNamespace, paramApp, paramVersion, paramFile),
		handleErr(handleFiles(scheme)),
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
		cli, err := newClient(r, scheme)
		if err != nil {
			return err
		}

		rc, mediaType, err := ReqToApp(r)
		if err != nil {
			return err
		}
		defer rc.Close()

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

		ext := ".ipa"
		if mediaType == ContentTypeAPK {
			ext = ".apk"
		}

		var (
			key      = fmt.Sprintf("%s%s", appName, ext)
			selector = map[string]string{
				labelApp: appName,
			}
			mobileApp = &momov1alpha1.MobileApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      appName,
				},
				Spec: momov1alpha1.MobileAppSpec{
					Selector: selector,
				},
			}
		)

		if _, err = controllerutil.CreateOrUpdate(ctx, cli, mobileApp, func() error {
			mobileApp.Spec.Selector = selector
			return nil
		}); err != nil {
			return err
		}

		switch ext {
		case ".apk":
			if err = cli.Create(ctx,
				&momov1alpha1.APK{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    namespace,
						GenerateName: fmt.Sprintf("%s-", appName),
						Labels:       selector,
					},
					Spec: momov1alpha1.APKSpec{
						Bucket: corev1.LocalObjectReference{
							Name: bucketName,
						},
						Key: key,
					},
				},
			); err != nil {
				return err
			}
		default:
			if err = cli.Create(ctx,
				&momov1alpha1.IPA{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    namespace,
						GenerateName: fmt.Sprintf("%s-", appName),
						Labels:       selector,
					},
					Spec: momov1alpha1.IPASpec{
						Bucket: corev1.LocalObjectReference{
							Name: bucketName,
						},
						Key: key,
					},
				},
			); err != nil {
				return err
			}
		}
		wc, err := b.NewWriter(ctx, key, &blob.WriterOptions{ContentType: mediaType})
		if err != nil {
			return err
		}
		defer wc.Close()

		if _, err = io.Copy(wc, rc); err != nil {
			return err
		}

		if mediaType == ContentTypeMultiPart {
			referer := r.Header.Get("Referer")
			if referer != "" {
				return err
			}

			w.WriteHeader(http.StatusTemporaryRedirect)
			w.Header().Set("Location", referer)

			return nil
		}

		w.WriteHeader(http.StatusCreated)

		return nil
	}
}

func handleManifests(scheme *runtime.Scheme) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		cli, err := newClient(nil, scheme)
		if err != nil {
			return err
		}

		var (
			ctx       = r.Context()
			mobileApp = &momov1alpha1.MobileApp{}
			namespace = chi.URLParam(r, "namespace")
			appName   = chi.URLParam(r, "app")
			version   = chi.URLParam(r, "version")
		)

		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, mobileApp); err != nil {
			return err
		}

		baseURL, err := urlFromReq(r)
		if err != nil {
			return err
		}

		values := url.Values{}
		values.Add("action", "download-manifest")
		if version == "" {
			values.Add("url", baseURL.JoinPath("/api/v1", namespace, "files", appName, "manifest.plist").String())
		} else {
			values.Add("url", baseURL.JoinPath("/api/v1", namespace, "files", appName, version, "manifest.plist").String())
		}

		http.Redirect(w, r,
			(&url.URL{Scheme: "itms-services", RawQuery: values.Encode()}).String(),
			http.StatusMovedPermanently,
		)

		return nil
	}
}

func handleFiles(scheme *runtime.Scheme) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var (
			ctx       = r.Context()
			mobileApp = &momov1alpha1.MobileApp{}
			namespace = strings.ToLower(chi.URLParam(r, "namespace"))
			appName   = strings.ToLower(chi.URLParam(r, "app"))
			version   = chi.URLParam(r, "version")
			file      = strings.ToLower(chi.URLParam(r, "file"))
			ext       = filepath.Ext(file)
		)

		cli, err := newClient(nil, scheme)
		if err != nil {
			return err
		}

		if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appName}, mobileApp); err != nil {
			return err
		}

		var (
			findAny = func(app momov1alpha1.MobileAppStatusApp, _ int) bool {
				return true
			}
			findLatest = func(app momov1alpha1.MobileAppStatusApp, _ int) bool {
				return app.Latest
			}
			find = findLatest
		)
		if version != "" {
			find = func(app momov1alpha1.MobileAppStatusApp, _ int) bool {
				return strings.EqualFold(app.Version, version)
			}
		}

		var (
			ipa = xslice.Coalesce(
				xslice.Find(mobileApp.Status.IPAs, find),
				xslice.Find(mobileApp.Status.IPAs, findAny),
			)
			apk = xslice.Coalesce(
				xslice.Find(mobileApp.Status.APKs, find),
				xslice.Find(mobileApp.Status.APKs, findAny),
			)
			key         string
			bucketName  string
			contentType string
		)
		switch ext {
		case ".apk":
			if apk.Key == "" {
				return fmt.Errorf("app does not have an .apk")
			}
			key = apk.Key
			bucketName = apk.Bucket.Name
			contentType = ContentTypeAPK
		case ".ipa":
			if ipa.Key == "" {
				return fmt.Errorf("app does not have an .ipa")
			}
			key = ipa.Key
			bucketName = ipa.Bucket.Name
			contentType = ContentTypeIPA
		case ".png", ".jpg", ".jpeg":
			var (
				findAny = func(_ momov1alpha1.AppStatusIcon, _ int) bool {
					return true
				}
				find = func(icon momov1alpha1.AppStatusIcon, _ int) bool {
					return strings.TrimSuffix(filepath.Base(icon.Key), filepath.Ext(icon.Key)) == strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
				}
			)

			switch ext {
			case ".jpg", ".jpeg":
				contentType = ContentTypeJPEG
			case ".png":
				contentType = ContentTypePNG

				switch file {
				case "display.png":
					find = func(icon momov1alpha1.AppStatusIcon, _ int) bool {
						return icon.Display
					}
				case "full-size.png":
					find = func(icon momov1alpha1.AppStatusIcon, _ int) bool {
						return icon.FullSize
					}
				}
			}

			cr0 := &momov1alpha1.IPA{}

			if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ipa.Name}, cr0); err != nil {
				return err
			}

			cr1 := &momov1alpha1.APK{}

			if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: apk.Name}, cr1); err != nil {
				return err
			}

			icons := cr0.Status.Icons
			icons = append(icons, cr1.Status.Icons...)

			// TODO: I guess we search all apk and ipa icons?
			key = xslice.
				Coalesce(
					xslice.Find(icons, find),
					xslice.Find(icons, findAny),
				).
				Key
		default:
			if file == "manifest.plist" {
				if ipa.Key == "" {
					return fmt.Errorf("app does not have an .ipa")
				}

				cr := &momov1alpha1.IPA{}

				if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ipa.Name}, cr); err != nil {
					return err
				}

				if cr.Status.BundleIdentifier == "" || cr.Status.BundleName == "" || cr.Status.Version == "" {
					return newHTTPStatusCodeError(fmt.Errorf("mobileapp is in phase %s", mobileApp.Status.Phase), http.StatusPreconditionFailed)
				}

				baseURL, err := urlFromReq(r)
				if err != nil {
					return err
				}

				enc := plist.NewEncoder(w)
				if wantsPretty(r) {
					enc.Indent("  ")
				}

				w.Header().Set("Content-Type", ContentTypePlist)

				if err = enc.Encode(&ios.Manifest{
					Items: []ios.ManifestItem{
						{
							Assets: []ios.ManifestItemAsset{
								{
									Kind: "software-package",
									URL:  baseURL.JoinPath("/api/v1", namespace, "files", appName, version, strings.ToLower(fmt.Sprintf("%s.ipa", cr.Status.BundleName))).String(),
								},
								{
									Kind: "full-size-image",
									URL:  baseURL.JoinPath("/api/v1", namespace, "files", appName, version, "full-size.png").String(),
								},
								{
									Kind: "display-image",
									URL:  baseURL.JoinPath("/api/v1", namespace, "files", appName, version, "display.png").String(),
								},
							},
							Metadata: &ios.ManifestItemMetadata{
								Kind:               "software",
								PlatformIdentifier: "com.apple.platform.iphoneos",
								BundleIdentifier:   cr.Status.BundleIdentifier,
								BundleVersion:      cr.Status.Version,
								Title:              cr.Status.BundleName,
							},
						},
					},
				}); err != nil {
					return err
				}

				return nil
			}

			http.NotFound(w, r)

			return nil
		}

		bucket, err := GetBucket(ctx, cli, client.ObjectKey{Namespace: namespace, Name: bucketName})
		if err != nil {
			return err
		}

		b, err := OpenBucket(ctx, cli, bucket)
		if err != nil {
			return err
		}

		rc, err := b.NewReader(ctx, key, nil)
		if gcerrors.Code(err) == gcerrors.NotFound || rc == nil {
			w.Header().Set("Content-Type", contentType)

			switch ext {
			case ".png":
				if _, err := io.Copy(w, bytes.NewReader(questionMarkPNG)); err != nil {
					return err
				}

				return nil
			case ".jpg", ".jpeg":
				img, _, err := image.Decode(bytes.NewReader(questionMarkPNG))
				if err != nil {
					return err
				}

				if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 100}); err != nil {
					return err
				}

				return nil
			}

			return newHTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
		defer rc.Close()

		switch ext {
		case ".jpg", ".jpeg":
			img, _, err := image.Decode(rc)
			if err != nil {
				return err
			}

			w.Header().Set("Content-Type", contentType)

			if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 100}); err != nil {
				return err
			}

			return nil
		}

		w.Header().Set("Content-Type", contentType)

		if _, err := io.Copy(w, rc); err != nil {
			return err
		}

		return nil
	}
}

func urlFromReq(r *http.Request) (*url.URL, error) {
	if origin := r.Header.Get("Origin"); origin != "" {
		return url.Parse(origin)
	}

	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		var (
			params = strings.Split(forwarded, ";")
			scheme string
			host   string
		)
		for _, param := range params {
			parts := strings.SplitN(strings.TrimSpace(param), "=", 2)
			if len(parts) != 2 {
				continue
			}
			switch strings.ToLower(parts[0]) {
			case "proto":
				scheme = parts[1]
			case "host":
				host = parts[1]
			}
		}

		if scheme != "" && host != "" {
			return url.Parse(fmt.Sprintf("%s://%s", scheme, host))
		}
	}

	scheme := "http"
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	} else if r.TLS != nil {
		scheme = "https"
	}

	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	return url.Parse(fmt.Sprintf("%s://%s", scheme, host))
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

func newClient(r *http.Request, scheme *runtime.Scheme) (client.Client, error) {
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
