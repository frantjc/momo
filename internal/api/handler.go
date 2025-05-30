package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	swagger "github.com/swaggo/http-swagger/v2"
	"github.com/timewasted/go-accept-headers"
	"gocloud.dev/gcerrors"
	"howett.net/plist"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	paramNamespace = `{namespace:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramBucket    = `{bucket:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramApp       = `{app:[a-z0-9]([-a-z0-9]*[a-z0-9])?}`
	paramVersion   = `{version}`
	paramFile      = `{file}`
)

type Opts struct {
	Path     string
	Swagger  bool
	Fallback http.Handler
}

type Opt interface {
	Apply(*Opts)
}

func (o *Opts) Apply(opts *Opts) {
	if o != nil {
		if opts != nil {
			if o.Path != "" {
				opts.Path = path.Join("/", o.Path)
			}
			if o.Swagger {
				opts.Swagger = true
			}
			if o.Fallback != nil {
				opts.Fallback = o.Fallback
			}
		}
	}
}

func newOpts(opts ...Opt) *Opts {
	o := &Opts{Fallback: http.NotFoundHandler()}

	for _, opt := range opts {
		opt.Apply(o)
	}

	return o
}

type handler struct {
	Path   string
	Scheme *runtime.Scheme
}

func (h *handler) init() error {
	if h.Scheme == nil {
		var err error
		h.Scheme, err = momoutil.NewScheme(corev1.AddToScheme, momov1alpha1.AddToScheme)
		if err != nil {
			return err
		}
	}

	return nil
}

func NewHandler(opts ...Opt) (http.Handler, error) {
	o := newOpts(opts...)

	var (
		h = &handler{Path: o.Path}
		r = chi.NewRouter()
	)

	r.Use(middleware.RealIP)

	r.Route(path.Join("/", h.Path), func(r chi.Router) {
		if o.Swagger {
			r.Get("/", http.RedirectHandler(path.Join("/", h.Path, "swagger/index.html"), http.StatusMovedPermanently).ServeHTTP)

			r.Get("/doc.json", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(swaggerJSON)
			})

			r.Get("/*", swagger.Handler())
		}

		r.Post(
			fmt.Sprintf("/%s/upload/%s/%s", paramNamespace, paramBucket, paramApp),
			handleErr(h.handleUpload),
		)

		r.Get(
			fmt.Sprintf("/%s/install/%s", paramNamespace, paramApp),
			handleErr(h.handleInstall),
		)

		r.Get(
			fmt.Sprintf("/%s/install/%s/%s", paramNamespace, paramApp, paramVersion),
			handleErr(h.handleInstall),
		)

		r.Get(
			fmt.Sprintf("/%s/files/%s/%s", paramNamespace, paramApp, paramFile),
			handleErr(h.handleFiles),
		)

		r.Get(
			fmt.Sprintf("/%s/files/%s/%s/%s", paramNamespace, paramApp, paramVersion, paramFile),
			handleErr(h.handleFiles),
		)

		r.Get(
			fmt.Sprintf("/%s/apps", paramNamespace),
			handleErr(h.handleApps),
		)

		r.Get(
			fmt.Sprintf("/%s/apps/%s", paramNamespace, paramApp),
			handleErr(h.handleApp),
		)
	})

	r.NotFound(o.Fallback.ServeHTTP)

	return r, h.init()
}

func handleErr(handler func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handler(w, r); err != nil {
			if nErr := negotiate(w, r, "application/json"); nErr != nil {
				http.Error(w, err.Error(), momoutil.HTTPStatusCode(err))
				return
			}

			w.WriteHeader(momoutil.HTTPStatusCode(err))
			_ = respondJSON(w, r, &Error{Message: err.Error()})
		}
	}
}

func negotiate(w http.ResponseWriter, r *http.Request, contentType string) error {
	if _, err := accept.Negotiate(r.Header.Get("Accept"), contentType); err != nil {
		w.Header().Set("Accept", contentType)
		return momoutil.NewHTTPStatusCodeError(err, http.StatusUnsupportedMediaType)
	}

	w.Header().Set("Vary", "Accept")

	// if acceptEncoding := r.Header.Get("Accept-Encoding"); acceptEncoding != "" && xslice.Every([]string{"identity", "*"}, func(s string, _ int) bool {
	// 	return !strings.Contains(acceptEncoding, s)
	// }) {
	// 	w.Header().Set("Accept-Encoding", "identity")
	// 	return newHTTPStatusCodeError(fmt.Errorf("cannot satisfy Accept-Encoding: %s", acceptEncoding), http.StatusNotAcceptable)
	// }

	// w.Header().Add("Vary", "Accept-Encoding")

	w.Header().Set("Content-Type", contentType)

	return nil
}

type Error struct {
	Message string `json:"error,omitempty"`
}

type App struct {
	Name string `json:"name,omitempty"`
}

func appFromMobileApp(mobileApp momov1alpha1.MobileApp) *App {
	return &App{Name: mobileApp.Name}
}

// @Summary	Upload a mobile app
// @Tags		upload
// @Accept		multipart/form-data
// @Accept		application/tar
// @Accept		application/x-tar
// @Accept		application/octet-stream
// @Accept		application/vnd.android.package-archive
// @Accept		application/gzip
// @Accept		application/x-gtar
// @Accept		application/x-tgz
// @Param		namespace	path		string	true	"Namespace"
// @Param		bucket		path		string	true	"Bucket"
// @Param		app			path		string	true	"App"
// @Success	201			{object}	App
// @Success	307
// @Failure	406	{object}	Error
// @Failure	415	{object}	Error
// @Failure	500	{object}	Error
// @Router		/{namespace}/uploads/{bucket}/{app} [post]
func (h *handler) handleUpload(w http.ResponseWriter, r *http.Request) error {
	cli, err := h.newClient(r)
	if err != nil {
		return err
	}

	rc, mediaType, err := reqToApp(r)
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	var (
		ctx        = r.Context()
		namespace  = chi.URLParam(r, "namespace")
		bucketName = chi.URLParam(r, "bucket")
		appName    = chi.URLParam(r, "app")
	)

	if err := momoutil.UploadApp(ctx, cli, namespace, appName, bucketName, mediaType, rc); err != nil {
		return err
	}

	if mediaType == "multipart/form-data" {
		if referer := r.Header.Get("Referer"); referer != "" {
			w.WriteHeader(http.StatusTemporaryRedirect)
			w.Header().Set("Location", referer)
		}
	}

	w.WriteHeader(http.StatusCreated)

	if err := respondJSON(w, r, appFromMobileApp(momov1alpha1.MobileApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
	})); err != nil {
		return err
	}

	return nil
}

// @Summary	Install an iOS app
// @Tags		install
// @Produce	json
// @Param		namespace	path	string	true	"Namespace"
// @Param		app			path	string	true	"App"
// @Param		version		path	string	false	"Version"
// @Success	301
// @Failure	404	{object}	Error
// @Failure	500	{object}	Error
// @Router		/{namespace}/install/{app} [get]
// @Router		/{namespace}/install/{app}/{version} [get]
func (h *handler) handleInstall(w http.ResponseWriter, r *http.Request) error {
	cli, err := h.newClient(nil)
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
		values.Add("url", baseURL.JoinPath("/", h.Path, namespace, "files", appName, momo.FileManifestPlist).String())
	} else {
		values.Add("url", baseURL.JoinPath("/", h.Path, namespace, "files", appName, version, momo.FileManifestPlist).String())
	}

	http.Redirect(w, r,
		(&url.URL{Scheme: ios.SchemeITMSServices, RawQuery: values.Encode()}).String(),
		http.StatusMovedPermanently,
	)

	return nil
}

func (h *handler) handleFiles(w http.ResponseWriter, r *http.Request) error {
	var (
		ctx       = r.Context()
		mobileApp = &momov1alpha1.MobileApp{}
		namespace = strings.ToLower(chi.URLParam(r, "namespace"))
		appName   = strings.ToLower(chi.URLParam(r, "app"))
		version   = chi.URLParam(r, "version")
		file      = strings.ToLower(chi.URLParam(r, "file"))
		ext       = filepath.Ext(file)
	)

	cli, err := h.newClient(nil)
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
	case momo.ExtAPK:
		if apk.Key == "" {
			return fmt.Errorf("app does not have an %s", momo.ExtAPK)
		}
		key = apk.Key
		bucketName = apk.Bucket.Name
		contentType = android.ContentTypeAPK
	case momo.ExtIPA:
		if ipa.Key == "" {
			return fmt.Errorf("app does not have an %s", momo.ExtIPA)
		}
		key = ipa.Key
		bucketName = ipa.Bucket.Name
		contentType = ios.ContentTypeIPA
	case momo.ExtPNG, momo.ExtJPG, momo.ExtJPEG:
		type iconAndBucketName struct {
			icon       momov1alpha1.AppStatusIcon
			bucketName string
		}

		var (
			findAny = func(_ iconAndBucketName, _ int) bool {
				return true
			}
			find = func(icon iconAndBucketName, _ int) bool {
				return strings.TrimSuffix(filepath.Base(icon.icon.Key), filepath.Ext(icon.icon.Key)) == strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			}
			icons = []iconAndBucketName{}
		)

		switch ext {
		case momo.ExtJPG, momo.ExtJPEG:
			contentType = "image/jpeg"
		case momo.ExtPNG:
			contentType = "image/png"

			switch file {
			case momo.FileDisplayIcon:
				find = func(icon iconAndBucketName, _ int) bool {
					return icon.icon.Display
				}
			case momo.FileFullSizeIcon:
				find = func(icon iconAndBucketName, _ int) bool {
					return icon.icon.FullSize
				}
			}
		}

		if ipa.Name != "" {
			cr := &momov1alpha1.IPA{}

			if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ipa.Name}, cr); err != nil {
				return err
			}

			icons = append(icons, xslice.Map(cr.Status.Icons, func(icon momov1alpha1.AppStatusIcon, _ int) iconAndBucketName {
				return iconAndBucketName{icon: icon, bucketName: cr.Spec.Bucket.Name}
			})...)
		}

		if apk.Name != "" {
			cr := &momov1alpha1.APK{}

			if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: apk.Name}, cr); err != nil {
				return err
			}

			icons = append(icons, xslice.Map(cr.Status.Icons, func(icon momov1alpha1.AppStatusIcon, _ int) iconAndBucketName {
				return iconAndBucketName{icon: icon, bucketName: cr.Spec.Bucket.Name}
			})...)
		}

		icon := xslice.
			Coalesce(
				xslice.Find(icons, find),
				xslice.Find(icons, findAny),
			)
		key = icon.icon.Key
		bucketName = icon.bucketName
	default:
		if file == momo.FileManifestPlist {
			if ipa.Key == "" {
				return fmt.Errorf("app does not have an %s", momo.ExtIPA)
			}

			cr := &momov1alpha1.IPA{}

			if err = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ipa.Name}, cr); err != nil {
				return err
			}

			if cr.Status.BundleIdentifier == "" || cr.Status.BundleName == "" || cr.Status.Version == "" {
				return momoutil.NewHTTPStatusCodeError(fmt.Errorf("mobileapp is in phase %s", mobileApp.Status.Phase), http.StatusPreconditionFailed)
			}

			if err := negotiate(w, r, ios.ContentTypePlist); err != nil {
				return err
			}

			baseURL, err := urlFromReq(r)
			if err != nil {
				return err
			}

			enc := plist.NewEncoder(w)
			if wantsPretty(r) {
				enc.Indent("  ")
			}

			if err = enc.Encode(&ios.Manifest{
				Items: []ios.ManifestItem{
					{
						Assets: []ios.ManifestItemAsset{
							{
								Kind: "software-package",
								URL:  baseURL.JoinPath("/", h.Path, namespace, "files", appName, version, strings.ToLower(fmt.Sprintf("%s%s", cr.Status.BundleName, momo.ExtIPA))).String(),
							},
							{
								Kind: "full-size-image",
								URL:  baseURL.JoinPath("/", h.Path, namespace, "files", appName, version, momo.FileFullSizeIcon).String(),
							},
							{
								Kind: "display-image",
								URL:  baseURL.JoinPath("/", h.Path, namespace, "files", appName, version, momo.FileDisplayIcon).String(),
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

	bucket, err := momoutil.GetBucket(ctx, cli, client.ObjectKey{Namespace: namespace, Name: bucketName})
	if err != nil {
		return err
	}

	b, err := momoutil.OpenBucket(ctx, cli, bucket)
	if err != nil {
		return err
	}

	if err := negotiate(w, r, contentType); err != nil {
		return err
	}

	rc, err := b.NewReader(ctx, key, nil)
	if gcerrors.Code(err) == gcerrors.NotFound || rc == nil {
		switch ext {
		case momo.ExtPNG:
			if _, err := io.Copy(w, bytes.NewReader(questionMarkPNG)); err != nil {
				return err
			}

			return nil
		case momo.ExtJPG, momo.ExtJPEG:
			img, _, err := image.Decode(bytes.NewReader(questionMarkPNG))
			if err != nil {
				return err
			}

			quality, err := strconv.Atoi(r.URL.Query().Get("quality"))
			if err != nil {
				quality = jpeg.DefaultQuality
			} else if quality > 100 {
				quality = 100
			} else if quality < 1 {
				quality = 1
			}

			if err := jpeg.Encode(w, img, &jpeg.Options{Quality: quality}); err != nil {
				return err
			}

			return nil
		}

		return momoutil.NewHTTPStatusCodeError(err, http.StatusNotFound)
	} else if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	switch ext {
	case momo.ExtJPG, momo.ExtJPEG:
		img, _, err := image.Decode(rc)
		if err != nil {
			return err
		}

		quality, err := strconv.Atoi(r.URL.Query().Get("quality"))
		if err != nil {
			quality = jpeg.DefaultQuality
		} else if quality > 100 {
			quality = 100
		} else if quality < 1 {
			quality = 1
		}

		if err := jpeg.Encode(w, img, &jpeg.Options{Quality: quality}); err != nil {
			return err
		}

		return nil
	}

	if _, err := io.Copy(w, rc); err != nil {
		return err
	}

	return nil
}

// @Summary	List apps
// @Tags		apps
// @Produce	json
// @Param		namespace	path		string	true	"Namespace"
// @Success	200			{object}	[]App
// @Failure	404			{object}	Error
// @Failure	500			{object}	Error
// @Router		/{namespace}/apps [get]
func (h *handler) handleApps(w http.ResponseWriter, r *http.Request) error {
	cli, err := h.newClient(nil)
	if err != nil {
		return err
	}

	var (
		namespace  = chi.URLParam(r, "namespace")
		cont       = r.URL.Query().Get("continue")
		limit, _   = strconv.Atoi(r.URL.Query().Get("limit"))
		mobileApps = &momov1alpha1.MobileAppList{}
	)

	if limit <= 0 {
		limit = 10
	}

	if err := cli.List(r.Context(),
		mobileApps,
		&client.ListOptions{
			Namespace: namespace,
			Limit:     int64(limit),
			Continue:  cont,
		},
	); err != nil {
		return err
	}

	apps := xslice.Map(mobileApps.Items, func(mobileApp momov1alpha1.MobileApp, _ int) App {
		return *appFromMobileApp(mobileApp)
	})

	if err := respondJSON(w, r, apps); err != nil {
		return err
	}

	return nil
}

// @Summary	Get an app
// @Tags		apps
// @Produce	json
// @Param		namespace	path		string	true	"Namespace"
// @Param		app			path		string	true	"App"
// @Success	200			{object}	App
// @Failure	404			{object}	Error
// @Failure	500			{object}	Error
// @Router		/{namespace}/apps/{app} [get]
func (h *handler) handleApp(w http.ResponseWriter, r *http.Request) error {
	cli, err := h.newClient(nil)
	if err != nil {
		return err
	}

	var (
		namespace = chi.URLParam(r, "namespace")
		appName   = chi.URLParam(r, "app")
		mobileApp = &momov1alpha1.MobileApp{}
	)

	if err := cli.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: appName}, mobileApp); err != nil {
		return err
	}

	if err := respondJSON(w, r, appFromMobileApp(*mobileApp)); err != nil {
		return err
	}

	return nil
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

func respondJSON(w http.ResponseWriter, r *http.Request, a any) error {
	if err := negotiate(w, r, "application/json"); err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	if wantsPretty(r) {
		enc.SetIndent("", "  ")
	}

	return enc.Encode(a)
}

func wantsPretty(r *http.Request) bool {
	pretty, _ := strconv.ParseBool(r.URL.Query().Get("pretty"))
	return pretty
}

func (h *handler) newClient(r *http.Request) (client.Client, error) {
	if err := h.init(); err != nil {
		return nil, err
	}

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

	cli, err := client.New(cfg, client.Options{Scheme: h.Scheme})
	if err != nil {
		return nil, err
	}

	return cli, nil
}
