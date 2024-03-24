package momohttp

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gocloud.dev/blob"
)

func NewAppsHandler(bucket *blob.Bucket, db *sql.DB, base *url.URL, notFound http.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)

	var (
		getFile          = getFile(base, bucket, db)
		downloadManifest = downloadManifest(db, base)
	)

	r.Get(
		fmt.Sprintf("/apps/%s/%s", idParam, fileParam),
		getFile,
	)

	r.Get(
		fmt.Sprintf("/apps/%s/%s", appParam, fileParam),
		getFile,
	)

	r.Get(
		fmt.Sprintf("/apps/%s/%s/%s", appParam, versionParam, fileParam),
		getFile,
	)

	r.Get(
		fmt.Sprintf("/apps/%s/download-manifest", idParam),
		downloadManifest,
	)

	r.Get(
		fmt.Sprintf("/apps/%s/download-manifest", appParam),
		downloadManifest,
	)

	r.Get(
		fmt.Sprintf("/apps/%s/%s/download-manifest", appParam, versionParam),
		downloadManifest,
	)

	r.NotFound(notFound.ServeHTTP)

	return r
}
