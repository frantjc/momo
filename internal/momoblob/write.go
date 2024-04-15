package momoblob

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoerr"
	"github.com/frantjc/momo/internal/momosql"
	"gocloud.dev/blob"
)

func WriteImage(w io.WriteCloser, img image.Image) error {
	if err := png.Encode(w, img); err != nil {
		return err
	}

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}

func WriteUpload(ctx context.Context, bucket *blob.Bucket, db *sql.DB, mediaType, boundary string, app *momo.App, body io.Reader) error {
	var (
		isMultipart = strings.EqualFold(mediaType, "multipart/form-data")
		isGzip      = strings.EqualFold(mediaType, "application/x-gzip")
		isTar       = strings.EqualFold(mediaType, "application/x-tar")
		isApk       = strings.EqualFold(mediaType, "application/vnd.android.package-archive")
		isIpa       = strings.EqualFold(mediaType, "application/octet-stream")
	)

	if isMultipart || isTar || isGzip || isApk || isIpa {
		if isMultipart && boundary == "" {
			return momoerr.HTTPStatusCodeError(
				fmt.Errorf("no boundary"),
				http.StatusBadRequest,
			)
		}

		if err := momosql.InsertApp(ctx, db, app); err != nil {
			return err
		}

		bw, err := bucket.NewWriter(ctx, TGZKey(app.ID), nil)
		if err != nil {
			return err
		}
		if !isGzip {
			defer bw.Close()
		}

		var (
			wc io.WriteCloser = bw
			bd                = body
		)
		if !isGzip {
			wc = gzip.NewWriter(bw)
		}
		defer wc.Close()

		switch {
		case isMultipart:
			bd = multipartToTar(multipart.NewReader(bd, boundary), nil)
		case isApk:
			bd = fileToTar(body, "app.apk", nil)
		case isIpa:
			bd = fileToTar(body, "app.ipa", nil)
		}

		if _, err = io.Copy(wc, bd); err != nil {
			return err
		}
	} else {
		return momoerr.HTTPStatusCodeError(
			fmt.Errorf("unsupported Content-Type %s", mediaType),
			http.StatusUnsupportedMediaType,
		)
	}

	return nil
}

type fileToTarOpts struct {
	Mode int64
}

func fileToTar(r io.Reader, name string, opts *fileToTarOpts) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		tw := tar.NewWriter(pw)
		defer tw.Close()

		var mode int64 = 0777
		if opts != nil && opts.Mode > 0 {
			mode = opts.Mode
		}

		if err := func() error {
			if err := tw.WriteHeader(&tar.Header{
				Name: name,
				Mode: mode,
			}); err != nil {
				return err
			}

			if _, err := io.Copy(tw, r); err != nil {
				return err
			}

			return nil
		}(); err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr
}

type multipartToTarOpts struct {
	Mode int64
}

// multipartToTar converts a *multipart.Reader to a io.Reader.
func multipartToTar(mr *multipart.Reader, opts *multipartToTarOpts) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()

		var mode int64 = 0777
		if opts != nil && opts.Mode > 0 {
			mode = opts.Mode
		}

		if err := func() error {
			for {
				p, err := mr.NextPart()
				if errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					return err
				}
				defer p.Close()

				if p.FormName() == "file" {
					// Need to get the size of the part.
					b, err := io.ReadAll(p)
					if err != nil {
						return err
					}

					if err := tw.WriteHeader(&tar.Header{
						Name: p.FileName(),
						Mode: mode,
						Size: int64(len(b)),
					}); err != nil {
						return err
					}

					if _, err := tw.Write(b); err != nil {
						return err
					}
				}
			}

			return nil
		}(); err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	return pr
}
