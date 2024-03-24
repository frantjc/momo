package momoblob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoerr"
	"github.com/frantjc/momo/internal/momoimg"
	"github.com/frantjc/momo/ios"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"howett.net/plist"
)

func NewAppFileReader(ctx context.Context, bucket *blob.Bucket, base *url.URL, app *momo.App, file string, pretty bool) (io.ReadCloser, string, error) {
	var (
		ext         = filepath.Ext(file)
		rc          io.ReadCloser
		contentType string
		err         error
	)
	switch {
	case strings.EqualFold(ext, ".apk"):
		contentType = "application/vnd.android.package-archive"
		rc, err = bucket.NewReader(ctx, APKKey(app.ID), nil)
	case strings.EqualFold(ext, ".ipa"):
		contentType = "application/octet-stream"
		rc, err = bucket.NewReader(ctx, IPAKey(app.ID), nil)
	case strings.EqualFold(file, "full-size.png"), strings.EqualFold(file, "full-size-image.png"), strings.EqualFold(file, "icon512.png"):
		contentType = "image/png"
		rc, err = bucket.NewReader(ctx, FullSizeImageKey(app.ID), nil)
	case strings.EqualFold(file, "display.png"), strings.EqualFold(file, "display-image.png"), strings.EqualFold(file, "icon57.png"):
		contentType = "image/png"
		rc, err = bucket.NewReader(ctx, DisplayImageKey(app.ID), nil)
	case strings.EqualFold(file, "manifest.plist"):
		if app.BundleIdentifier == "" || app.BundleName == "" || app.Version == "" {
			return nil, "", momoerr.HTTPStatusCodeError(fmt.Errorf("not found"), http.StatusPreconditionFailed)
		} else if exists, _ := bucket.Exists(ctx, IPAKey(app.ID)); !exists {
			return nil, "", momoerr.HTTPStatusCodeError(fmt.Errorf("not found"), http.StatusNotFound)
		}

		var (
			buf = new(bytes.Buffer)
			enc = plist.NewEncoder(buf)
		)
		if pretty {
			enc.Indent("  ")
		}

		if err = enc.Encode(&ios.Manifest{
			Items: []ios.ManifestItem{
				{
					Assets: []ios.ManifestItemAsset{
						{
							Kind: "software-package",
							URL:  base.JoinPath("/apps", app.ID, app.BundleName+".ipa").String(),
						},
						{
							Kind: "full-size-image",
							URL:  base.JoinPath("/apps", app.ID, "full-size-image.png").String(),
						},
						{
							Kind: "display-image",
							URL:  base.JoinPath("/apps", app.ID, "display-image.png").String(),
						},
					},
					Metadata: &ios.ManifestItemMetadata{
						Kind:               "software",
						PlatformIdentifier: "com.apple.platform.iphoneos",
						BundleIdentifier:   app.BundleIdentifier,
						BundleVersion:      app.Version,
						Title:              app.BundleName,
					},
				},
			},
		}); err != nil {
			return nil, "", err
		}

		return io.NopCloser(buf), "application/xml", nil
	}
	if gcerrors.Code(err) == gcerrors.NotFound || rc == nil {
		switch {
		case strings.EqualFold(ext, ".png"):
			return io.NopCloser(bytes.NewReader(momoimg.QuestionMark)), "image/png", nil
		}

		return nil, "", momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
	} else if err != nil {
		return nil, "", err
	}

	return rc, contentType, nil
}
