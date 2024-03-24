package momohttp

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoregexp"
	"github.com/go-chi/chi/v5"
)

var (
	idParam      = fmt.Sprintf("{id:%s}", momoregexp.UUID.String())
	fileParam    = `{file:[a-zA-Z0-9]+\.[a-zA-Z]+}`
	appParam     = "{app}"
	versionParam = "{version}"
)

func appID(r *http.Request) string {
	return chi.URLParam(r, "id")
}

func file(r *http.Request) string {
	return chi.URLParam(r, "file")
}

func appName(r *http.Request) string {
	return chi.URLParam(r, "app")
}

func appVersion(r *http.Request) string {
	return chi.URLParam(r, "version")
}

func app(r *http.Request) *momo.App {
	return &momo.App{
		ID:      appID(r),
		Name:    appName(r),
		Version: appVersion(r),
	}
}

func pretty(r *http.Request) bool {
	pretty, _ := strconv.ParseBool(r.URL.Query().Get("pretty"))
	return pretty
}
