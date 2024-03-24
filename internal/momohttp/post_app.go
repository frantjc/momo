package momohttp

import (
	"database/sql"
	"mime"
	"net/http"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoblob"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
)

func postApp(bucket *blob.Bucket, db *sql.DB, topic *pubsub.Topic) http.HandlerFunc {
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

		app.Status = "uploaded"

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
	}
}
