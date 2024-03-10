package momo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	HTTPClient *http.Client
	Base       *url.URL
}

func (c *Client) init() error {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.Base == nil {
		var err error
		c.Base, err = url.Parse("http://localhost:8080/")
		return err
	}
	return nil
}

func (c *Client) GetApp(ctx context.Context, app *App) error {
	if err := c.init(); err != nil {
		return err
	}

	if err := ValidateApp(app); err != nil {
		return err
	}

	elems := []string{"/api/v1/apps"}

	if app.ID != "" {
		elems = append(elems, app.ID)
	} else if app.Name != "" && app.Version != "" {
		elems = append(elems, app.Name, app.Version)
	} else if app.Name != "" {
		elems = append(elems, app.Name)
	} else {
		return fmt.Errorf("unable to uniquely identify app")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base.JoinPath(elems...).String(), nil)
	if err != nil {
		return err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body := map[string]string{}
		if err = json.NewDecoder(res.Body).Decode(&body); err == nil {
			if body["error"] != "" {
				return fmt.Errorf("http status code %d: %s", res.StatusCode, body["error"])
			}
		}

		return fmt.Errorf("http status code %d", res.StatusCode)
	}

	if err = json.NewDecoder(res.Body).Decode(app); err != nil {
		return err
	}

	return nil
}

func (c *Client) GetApps(ctx context.Context) ([]App, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base.JoinPath("/api/v1/apps").String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body := map[string]string{}
		if err = json.NewDecoder(res.Body).Decode(&body); err == nil {
			if body["error"] != "" {
				return nil, fmt.Errorf(body["error"])
			}
		}

		return nil, fmt.Errorf("http status code %d", res.StatusCode)
	}

	apps := []App{}
	if err = json.NewDecoder(res.Body).Decode(&apps); err != nil {
		return nil, err
	}

	return apps, nil
}

func (c *Client) UploadApp(ctx context.Context, body io.Reader, contentType string, app *App) error {
	if err := c.init(); err != nil {
		return err
	}

	if !strings.EqualFold(contentType, "application/x-gzip") && !strings.EqualFold(contentType, "application/x-tar") {
		return fmt.Errorf("invalid Content-Type %s", contentType)
	}

	if err := ValidateApp(app); err != nil {
		return err
	}

	elems := []string{"/api/v1/apps"}

	if app.Name != "" && app.Version != "" {
		elems = append(elems, app.Name, app.Version)
	} else if app.Name != "" {
		elems = append(elems, app.Name)
	} else {
		return fmt.Errorf("unable to uniquely identify app")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base.JoinPath(elems...).String(), body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", contentType)

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

	if err = json.NewDecoder(res.Body).Decode(app); err != nil {
		return err
	}

	return nil
}

func (c *Client) Readyz(ctx context.Context) error {
	if err := c.init(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base.JoinPath("/readyz").String(), nil)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base.JoinPath("/healthz").String(), nil)
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
