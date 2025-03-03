package command

import (
	"fmt"
	"net/http"

	"k8s.io/client-go/rest"
)

type kubeAuthTransport struct {
	RestConfig   *rest.Config
	RoundTripper http.RoundTripper
}

func (t *kubeAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil {
		return http.DefaultTransport.RoundTrip(req)
	}

	if t.RoundTripper == nil {
		t.RoundTripper = http.DefaultTransport
	}

	if t.RestConfig == nil {
		return t.RoundTripper.RoundTrip(req)
	}

	if t.RestConfig.BearerToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.RestConfig.BearerToken))
	} else if t.RestConfig.Username != "" && t.RestConfig.Password != "" {
		req.SetBasicAuth(t.RestConfig.Username, t.RestConfig.Password)
	}

	return t.RoundTripper.RoundTrip(req)
}
