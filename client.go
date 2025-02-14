package momo

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/frantjc/momo/internal/momoutil"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    *url.URL
}

func (c *Client) init() error {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.BaseURL == nil {
		var err error
		c.BaseURL, err = url.Parse("http://localhost:8080/")
		return err
	}
	return nil
}

func (c *Client) UploadApp(ctx context.Context, tar io.Reader, namespace, bucketName, appName string) error {
	if err := c.init(); err != nil {
		return err
	}

	pr, pw := io.Pipe()

	go func() {
		zw := gzip.NewWriter(pw)
		_, err := io.Copy(zw, tar)
		_ = zw.Close()
		_ = pw.CloseWithError(err)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL.JoinPath("/api/v1", namespace, bucketName, "uploads", appName).String(), pr)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", momoutil.ContentTypeTar)
	req.Header.Set("Content-Encoding", "gzip")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		body := map[string]string{}
		if err = json.NewDecoder(res.Body).Decode(&body); err == nil {
			if body["error"] != "" {
				return fmt.Errorf("http status code %d: %s", res.StatusCode, body["error"])
			}
		}

		return fmt.Errorf("http status code %d", res.StatusCode)
	}

	return nil
}

func (c *Client) Readyz(ctx context.Context) error {
	if err := c.init(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL.JoinPath("/readyz").String(), nil)
	if err != nil {
		return err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http status code %d", res.StatusCode)
	}

	return nil
}

func (c *Client) Healthz(ctx context.Context) error {
	if err := c.init(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL.JoinPath("/healthz").String(), nil)
	if err != nil {
		return err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http status code %d", res.StatusCode)
	}

	return nil
}
