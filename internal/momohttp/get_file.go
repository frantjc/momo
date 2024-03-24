package momohttp

import (
	"database/sql"
	"io"
	"net/http"
	"net/url"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoblob"
	"github.com/frantjc/momo/internal/momosql"
	"gocloud.dev/blob"
)

func getFile(base *url.URL, bucket *blob.Bucket, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			app    = app(r)
			file   = file(r)
			pretty = pretty(r)
		)

		if err := momo.ValidateApp(app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		if err := momosql.SelectApp(ctx, db, app); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		rc, contentType, err := momoblob.NewAppFileReader(ctx, bucket, base, app, file, pretty)
		if err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}
		defer rc.Close()

		w.Header().Set("Content-Type", contentType)
		_, _ = io.Copy(w, rc)
	}
}
