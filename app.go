package momo

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/frantjc/momo/internal/momoerr"
	"github.com/frantjc/momo/internal/momoregexp"
)

type App struct {
	ID                     string    `json:"id,omitempty" unixtable:"-"`
	Name                   string    `json:"name,omitempty"`
	Version                string    `json:"version,omitempty"`
	Status                 string    `json:"status,omitempty"`
	BundleName             string    `json:"bundleName,omitempty" unixtable:"-"`
	BundleIdentifier       string    `json:"bundleIdentifier,omitempty"`
	SHA256CertFingerprints string    `json:"sha256CertFingerprints,omitempty"`
	Created                time.Time `json:"created,omitempty" unixtable:"-"`
	Updated                time.Time `json:"updated,omitempty" unixtable:"-"`
}

func ValidateApp(app *App) error {
	errs := []error{}

	if app.ID != "" && !momoregexp.IsUUID(app.ID) {
		errs = append(errs, fmt.Errorf("invalid app ID %s", app.ID))
	}

	if app.Name != "" && !momoregexp.IsAppName(app.Name) {
		errs = append(errs, fmt.Errorf("invalid app name %s", app.Name))
	}

	if app.Version != "" && !momoregexp.IsAppVersion(app.Version) {
		errs = append(errs, fmt.Errorf("invalid app version %s", app.Version))
	}

	return momoerr.HTTPStatusCodeError(errors.Join(errs...), http.StatusBadRequest)
}
