package momoutil

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

func RespondJSON(w http.ResponseWriter, a any, pretty bool) error {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}

	return enc.Encode(a)
}

func NewHTTPStatusCodeError(err error, httpStatusCode int) error {
	if err == nil {
		return nil
	}

	if 600 <= httpStatusCode || httpStatusCode < 100 {
		httpStatusCode = 500
	}

	return &HTTPStatusCodeError{
		err:            err,
		httpStatusCode: httpStatusCode,
	}
}

type HTTPStatusCodeError struct {
	err            error
	httpStatusCode int
}

func (e *HTTPStatusCodeError) Error() string {
	if e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *HTTPStatusCodeError) Unwrap() error {
	return e.err
}

func HTTPStatusCode(err error) int {
	hscerr := &HTTPStatusCodeError{}
	if errors.As(err, &hscerr) {
		return hscerr.httpStatusCode
	}

	if apiStatus, ok := err.(apierrors.APIStatus); ok || errors.As(err, &apiStatus) {
		return int(apiStatus.Status().Code)
	}

	return http.StatusInternalServerError
}

func RespondErrorJSON(w http.ResponseWriter, err error, pretty bool) error {
	w.WriteHeader(HTTPStatusCode(err))

	return RespondJSON(w, map[string]string{"error": err.Error()}, pretty)
}

func IsPretty(r *http.Request) bool {
	pretty, _ := strconv.ParseBool(r.URL.Query().Get("pretty"))
	return pretty
}

func GetConfigForRequest(r *http.Request) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	cfg.CertData = nil
	cfg.CertFile = ""
	cfg.KeyData = nil
	cfg.CertFile = ""
	cfg.BearerToken = ""
	cfg.BearerTokenFile = ""
	cfg.Username = ""
	cfg.Password = ""

	var (
		authorization = r.Header.Get("Authorization")
		ok bool
	)
	cfg.Username, cfg.Password, ok = r.BasicAuth()
	if !ok && strings.HasPrefix(authorization, "Bearer ") {
		cfg.BearerToken = strings.TrimPrefix(authorization, "Bearer ")
	}

	return cfg, nil
}
