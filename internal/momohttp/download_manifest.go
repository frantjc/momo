package momohttp

import (
	"database/sql"
	"net/http"
	"net/url"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momosql"
)

func downloadManifest(db *sql.DB, base *url.URL) http.HandlerFunc {
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

		values := url.Values{}
		values.Add("action", "download-manifest")
		values.Add("url", base.JoinPath("/apps", app.Name, "manifest.plist").String())

		http.Redirect(w, r, (&url.URL{
			Scheme:   "itms-services",
			RawQuery: values.Encode(),
		}).String(), http.StatusMovedPermanently)
	}
}
