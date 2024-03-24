package momohttp

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/frantjc/momo/internal/momosql"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
)

func NewAPIHandler(bucket *blob.Bucket, db *sql.DB, topic *pubsub.Topic, notFound http.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)

	var (
		getApp    = getApp(db)
		postApp   = postApp(bucket, db, topic)
		deleteApp = deleteApp(bucket, db)
	)

	r.Get("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx       = r.Context()
			pretty    = pretty(r)
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

	r.Get(
		fmt.Sprintf("/api/v1/apps/%s", idParam),
		getApp,
	)

	r.Get(
		fmt.Sprintf("/api/v1/apps/%s", appParam),
		getApp,
	)

	r.Get(
		fmt.Sprintf("/api/v1/apps/%s/%s", appParam, versionParam),
		getApp,
	)

	r.Post(
		fmt.Sprintf("/api/v1/apps/%s", appParam),
		postApp,
	)

	r.Post(
		fmt.Sprintf("/api/v1/apps/%s/%s", appParam, versionParam),
		postApp,
	)

	r.Delete(
		fmt.Sprintf("/api/v1/apps/%s", idParam),
		deleteApp,
	)

	r.NotFound(notFound.ServeHTTP)

	return r
}
