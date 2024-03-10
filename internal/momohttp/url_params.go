package momohttp

import (
	"fmt"
	"net/http"

	"github.com/frantjc/momo/internal/momoregexp"
	"github.com/go-chi/chi/v5"
)

var (
	idParam      = fmt.Sprintf("{id:%s}", momoregexp.UUID.String())
	fileParam    = `{file:[a-zA-Z0-9]+\.[a-zA-Z]+}`
	appParam     = "{app}"
	versionParam = "{version}"
)

func getID(r *http.Request) string {
	return chi.URLParam(r, "id")
}

func getFile(r *http.Request) string {
	return chi.URLParam(r, "file")
}

func getApp(r *http.Request) string {
	return chi.URLParam(r, "app")
}

func getVersion(r *http.Request) string {
	return chi.URLParam(r, "version")
}
