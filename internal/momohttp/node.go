package momohttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
)

func NewNodeHandlerWithPortFromEnv(ctx context.Context, node string, arg ...string) (http.Handler, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, node, arg...)

	lis, err := net.Listen("tcp", "0")
	if err != nil {
		return nil, nil, err
	}

	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", port))
	if err != nil {
		return nil, nil, err
	}

	cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%s", port))

	if err = lis.Close(); err != nil {
		return nil, nil, err
	}

	return httputil.NewSingleHostReverseProxy(u), cmd, nil
}
