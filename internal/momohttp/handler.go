package momohttp

import (
	"database/sql"
	_ "embed"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoblob"
	"github.com/frantjc/momo/internal/momosql"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
)

func NewHandler(bucket *blob.Bucket, db *sql.DB, topic *pubsub.Topic, base *url.URL) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)

	r.Get("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx       = r.Context()
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
			limit, _  = strconv.Atoi(r.URL.Query().Get("limit"))
			offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
		)

		apps, err := momosql.SelectApps(ctx, db, limit, offset)
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		_ = respondJSON(w, apps, pretty)
	})

	r.Get(fmt.Sprintf("/api/v1/apps/%s", idParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				ID: getID(r),
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		_ = respondJSON(w, app, pretty)
	})

	r.Get(fmt.Sprintf("/apps/%s/%s", idParam, fileParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				ID: getID(r),
			}
			name      = chi.URLParam(r, "name")
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		rc, err := momoblob.NewAppFileReader(ctx, bucket, base, app, name, pretty)
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}
		defer rc.Close()

		_, _ = io.Copy(w, rc)
	})

	r.Get(fmt.Sprintf("/apps/%s/%s", appParam, fileParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name: getApp(r),
			}
			file      = getFile(r)
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		rc, err := momoblob.NewAppFileReader(ctx, bucket, base, app, file, pretty)
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}
		defer rc.Close()

		_, _ = io.Copy(w, rc)
	})

	r.Get(fmt.Sprintf("/apps/%s/%s/%s", appParam, versionParam, fileParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name:    getApp(r),
				Version: getVersion(r),
			}
			file      = getFile(r)
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		rc, err := momoblob.NewAppFileReader(ctx, bucket, base, app, file, pretty)
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}
		defer rc.Close()

		_, _ = io.Copy(w, rc)
	})

	r.Get(fmt.Sprintf("/apps/%s/itms-services", appParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name: getApp(r),
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		values := url.Values{}
		values.Add("action", "download-manifest")
		values.Add("url", base.JoinPath("/apps", app.Name, "manifest.plist").String())

		http.Redirect(w, r, (&url.URL{
			Scheme:   "itms-services",
			RawQuery: values.Encode(),
		}).String(), http.StatusMovedPermanently)
	})

	r.Get(fmt.Sprintf("/api/v1/apps/%s", appParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name: getApp(r),
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		r.Header.Set("Content-Type", "application/json")

		_ = respondJSON(w, app, pretty)
	})

	r.Get(fmt.Sprintf("/api/v1/apps/%s/%s", appParam, versionParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name:    getApp(r),
				Version: getVersion(r),
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		_ = respondJSON(w, app, pretty)
	})

	r.Post(fmt.Sprintf("/api/v1/apps/%s", appParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name:   chi.URLParam(r, "app"),
				Status: "uploaded",
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err = momoblob.WriteUpload(ctx, bucket, db, mediaType, params["boundary"], app, r.Body); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err = topic.Send(ctx, &pubsub.Message{
			Body: []byte(app.ID),
		}); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if strings.EqualFold(mediaType, "multipart/form-data") {
			if referer := r.Header.Get("Referer"); referer != "" {
				http.Redirect(w, r, referer, http.StatusFound)
				return
			}
		}

		w.WriteHeader(http.StatusCreated)
		_ = respondJSON(w, app, pretty)
	})

	r.Post(fmt.Sprintf("/api/v1/apps/%s/%s", appParam, versionParam), func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx = r.Context()
			app = &momo.App{
				Name:    getApp(r),
				Version: getVersion(r),
				Status:  "uploaded",
			}
			pretty, _ = strconv.ParseBool(r.URL.Query().Get("pretty"))
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err = momoblob.WriteUpload(ctx, bucket, db, mediaType, params["boundary"], app, r.Body); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err = r.Body.Close(); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err = topic.Send(ctx, &pubsub.Message{
			Body: []byte(app.ID),
		}); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if strings.EqualFold(mediaType, "multipart/form-data") {
			if referer := r.Header.Get("Referer"); referer != "" {
				http.Redirect(w, r, referer, http.StatusFound)
				return
			}
		}

		w.WriteHeader(http.StatusCreated)
		_ = respondJSON(w, app, pretty)
	})

	return r
}
