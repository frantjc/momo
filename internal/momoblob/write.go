package momoblob

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoerr"
	"github.com/frantjc/momo/internal/momosql"
	"gocloud.dev/blob"
)

func WriteUpload(ctx context.Context, bucket *blob.Bucket, db *sql.DB, mediaType, boundary string, app *momo.App, body io.Reader) error {
	var (
		isMultipart = strings.EqualFold(mediaType, "multipart/form-data")
		isGzip      = strings.EqualFold(mediaType, "application/x-gzip")
		isTar       = strings.EqualFold(mediaType, "application/x-tar")
	)

	if isMultipart || isTar || isGzip {
		if err := momosql.InsertApp(ctx, db, app); err != nil {
			return err
		}

		bw, err := bucket.NewWriter(ctx, TGZKey(app.ID), nil)
		if err != nil {
			return err
		}

		var (
			wc io.WriteCloser = bw
			bd                = body
		)
		if !isGzip {
			wc = gzip.NewWriter(bw)
		}

		if isMultipart {
			if boundary == "" {
				return momoerr.HTTPStatusCodeError(
					fmt.Errorf("no boundary"),
					http.StatusBadRequest,
				)
			}

			bd = multipartToTar(multipart.NewReader(bd, boundary), nil)
		}

		if _, err = io.Copy(wc, bd); err != nil {
			return err
		}

		if err = wc.Close(); err != nil {
			return err
		}

		if !isGzip {
			if err = bw.Close(); err != nil {
				return err
			}
		}
	} else {
		return momoerr.HTTPStatusCodeError(
			fmt.Errorf("unsupported Content-Type"),
			http.StatusNotAcceptable,
		)
	}

	return nil
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

		for {
			p, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				pw.CloseWithError(err)
				return
			}
			defer p.Close()

			if p.FormName() == "file" {
				// Need to get the size of the part.
				b, err := io.ReadAll(p)
				if err != nil {
					pw.CloseWithError(err)
					return
				}

				if err := tw.WriteHeader(&tar.Header{
					Name: p.FileName(),
					Mode: mode,
					Size: int64(len(b)),
				}); err != nil {
					pw.CloseWithError(err)
					return
				}

				if _, err := tw.Write(b); err != nil {
					pw.CloseWithError(err)
					return
				}
			}
		}
	}()

	return pr
}
