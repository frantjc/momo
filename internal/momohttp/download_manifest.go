package momohttp

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momosql"
)

func downloadManifest(app *momo.App, db *sql.DB, base *url.URL, w http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
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
}
