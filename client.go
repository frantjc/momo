package momo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo/android"
	"github.com/frantjc/momo/ios"
)

const (
	ExtAPK  = ".apk"
	ExtIPA  = ".ipa"
	ExtPNG  = ".png"
	ExtJPG  = ".jpg"
	ExtJPEG = ".jpeg"
)

const (
	FileManifestPlist = "manifest.plist"
	FileDisplayIcon   = "display.png"
	FileFullSizeIcon  = "full-size.png"
)

const (
	DefaultPort = 8080
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
		c.BaseURL, err = url.Parse(fmt.Sprintf("http://localhost:%d/", DefaultPort))
		return err
	}
	return nil
}

func (c *Client) UploadApp(ctx context.Context, file, namespace, bucketName, appName string) error {
	if err := c.init(); err != nil {
		return err
	}

	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL.JoinPath(namespace, "uploads", bucketName, appName).String(), f)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ExtIPA:
		req.Header.Set("Content-Type", ios.ContentTypeIPA)
	case ExtAPK:
		req.Header.Set("Content-Type", android.ContentTypeAPK)
	default:
		return fmt.Errorf("unrecognized file extension: %s", ext)
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()

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

func (c *Client) Ping(ctx context.Context) error {
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
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http status code %d", res.StatusCode)
	}

	return nil
}
