package momo

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
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

func (c *Client) UploadApp(ctx context.Context, body io.Reader, contentType, namespace, bucketName, appName string) error {
	if err := c.init(); err != nil {
		return err
	}

	doGzip := contentType == momoutil.ContentTypeTar

	if doGzip {
		pr, pw := io.Pipe()

		go func() {
			zw := gzip.NewWriter(pw)
			_, copyErr := io.Copy(zw, body)
			err := errors.Join(zw.Close(), copyErr)
			_ = pw.CloseWithError(err)
		}()

		body = pr
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL.JoinPath("/api/v1", namespace, "uploads", bucketName, appName).String(), body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", contentType)
	if doGzip {
		req.Header.Set("Content-Encoding", "gzip")
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		body := map[string]string{}
		if err = json.NewDecoder(res.Body).Decode(&body); err == nil {
			if msg := body["error"]; msg != "" {
				return fmt.Errorf("http status code %d: %s", res.StatusCode, msg)
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
