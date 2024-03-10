package momohttp

import (
	"encoding/json"
	"net/http"

	"github.com/frantjc/momo/internal/momoerr"
)

func respondJSON(w http.ResponseWriter, a any, pretty bool) error {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}

	return enc.Encode(a)
}

func respondErrorJSON(w http.ResponseWriter, err error, pretty bool) error {
	w.WriteHeader(momoerr.HTTPStatusCode(err))

	return respondJSON(w, map[string]string{"error": err.Error()}, pretty)
}
