package momohttp

import (
	"database/sql"
	"net/http"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momosql"
)

func getApp(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			app    = app(r)
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

		_ = respondJSON(w, app, pretty)
	}
}
