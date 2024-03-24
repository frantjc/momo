package momohttp

import (
	"database/sql"
	"errors"
	"io"
	"net/http"

	"github.com/frantjc/momo/internal/momosql"
	"gocloud.dev/blob"
)

func deleteApp(bucket *blob.Bucket, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			appID  = appID(r)
			pretty = pretty(r)
		)

		if err := momosql.DeleteApp(ctx, db, appID); err != nil {
			_ = respondErrorJSON(w, err, pretty)
			return
		}

		li := bucket.List(&blob.ListOptions{
			Prefix: appID,
		})

		for {
			lo, err := li.Next(ctx)
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				_ = respondErrorJSON(w, err, pretty)
				return
			}

			if err := bucket.Delete(ctx, lo.Key); err != nil {
				_ = respondErrorJSON(w, err, pretty)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
