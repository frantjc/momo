package api

import (
	"archive/tar"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	"github.com/frantjc/momo/ios"
	xio "github.com/frantjc/x/io"
)

type fileOpts struct {
	Mode int64
}

type fileOpt interface {
	Apply(*fileOpts)
}

func (t *fileOpts) Apply(opts *fileOpts) {
	opts.Mode = t.Mode
}

func decodeContent(req *http.Request) (io.ReadCloser, error) {
	var (
		rc               io.ReadCloser = req.Body
		contentEncodings               = strings.Split(strings.ToLower(req.Header.Get("Content-Encoding")), ",")
	)

	for i := len(contentEncodings) - 1; i >= 0; i-- {
		contentEncoding := strings.TrimSpace(contentEncodings[i])
		switch contentEncoding {
		case "gzip":
			zr, err := gzip.NewReader(rc)
			if err != nil {
				return nil, err
			}
			rc = zr
		case "deflate":
			rc = flate.NewReader(rc)
		case "":
		default:
			return nil, fmt.Errorf("unsupported Content-Encoding: %s", contentEncoding)
		}
	}

	return rc, nil
}

func reqToApp(req *http.Request, opts ...fileOpt) (io.ReadCloser, string, error) {
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", err
	}

	rc, err := decodeContent(req)
	if err != nil {
		return nil, "", err
	}

	switch mediaType {
	case android.ContentTypeAPK:
		return rc, mediaType, nil
	case ios.ContentTypeIPA:
		return rc, mediaType, nil
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return nil, "", newHTTPStatusCodeError(
				fmt.Errorf("missing boundary"),
				http.StatusBadRequest,
			)
		}

		r, mediaType, err := multipartToApp(rc, params["boundary"])
		if err != nil {
			return nil, "", err
		}

		return xio.ReadCloser{
			Reader: r,
			Closer: rc,
		}, mediaType, nil
	case "application/tar", "application/x-tar":
		r, mediaType, err := tarToApp(rc, opts...)
		if err != nil {
			return nil, "", err
		}

		return xio.ReadCloser{
			Reader: r,
			Closer: rc,
		}, mediaType, nil
	case "application/gzip", "application/x-gtar", "application/x-tgz":
		zr, err := gzip.NewReader(rc)
		if err != nil {
			return nil, "", err
		}

		r, mediaType, err := tarToApp(zr, opts...)
		if err != nil {
			return nil, "", err
		}

		return xio.ReadCloser{
			Reader: r,
			Closer: zr,
		}, mediaType, nil
	}

	return nil, "", newHTTPStatusCodeError(
		fmt.Errorf("unsupported Content-Type %s", mediaType),
		http.StatusUnsupportedMediaType,
	)
}

func tarToApp(r io.Reader, opts ...fileOpt) (io.Reader, string, error) {
	o := &fileOpts{
		Mode: 0777,
	}

	for _, opt := range opts {
		opt.Apply(o)
	}

	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, "", err
		}

		switch strings.ToLower(path.Ext(hdr.Name)) {
		case momo.ExtAPK:
			return tr, android.ContentTypeAPK, nil
		case momo.ExtIPA:
			return tr, ios.ContentTypeIPA, nil
		}
	}

	return nil, "", fmt.Errorf("no app found before %w", io.EOF)
}

func multipartToApp(r io.Reader, boundary string) (io.Reader, string, error) {
	mr := multipart.NewReader(r, boundary)

	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, "", err
		}
		defer p.Close()

		if p.FormName() == "file" {
			ext := strings.ToLower(path.Ext(p.FileName()))

			switch ext {
			case momo.ExtAPK:
				return p, android.ContentTypeAPK, nil
			case momo.ExtIPA:
				return p, ios.ContentTypeIPA, nil
			}
		}
	}

	return nil, "", fmt.Errorf("no app found before %w", io.EOF)
}
