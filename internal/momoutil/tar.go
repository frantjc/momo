package momoutil

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	xio "github.com/frantjc/x/io"
)

type FileOpts struct {
	Mode int64
}

type FileOpt interface {
	Apply(*FileOpts)
}

func (t *FileOpts) Apply(opts *FileOpts) {
	opts.Mode = t.Mode
}

func ReqToApp(req *http.Request, opts ...FileOpt) (io.ReadCloser, string, error) {
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", err
	}

	switch mediaType {
	case ContentTypeAPK:
		return req.Body, mediaType, nil
	case ContentTypeIPA:
		return req.Body, mediaType, nil
	case ContentTypeMultiPart:
		boundary := params["boundary"]
		if boundary == "" {
			return nil, "", newHTTPStatusCodeError(
				fmt.Errorf("missing boundary"),
				http.StatusBadRequest,
			)
		}

		r, mediaType, err := TarToApp(MultipartToTar(multipart.NewReader(req.Body, params["boundary"])))
		if err != nil {
			return nil, "", err
		}

		return xio.ReadCloser{
			Reader: r,
			Closer: req.Body,
		}, mediaType, nil
	case ContentTypeTar:
		if strings.EqualFold(req.Header.Get("Content-Encoding"), "gzip") {
			zr, err := gzip.NewReader(req.Body)
			if err != nil {
				return nil, "", err
			}

			r, mediaType, err := TarToApp(zr)
			if err != nil {
				return nil, "", err
			}
			return xio.ReadCloser{
				Reader: r,
				Closer: req.Body,
			}, mediaType, nil
		}
	}

	return nil, "", newHTTPStatusCodeError(
		fmt.Errorf("unsupported Content-Type %s", mediaType),
		http.StatusUnsupportedMediaType,
	)
}

func TarToApp(r io.Reader, opts ...FileOpt) (io.Reader, string, error) {
	o := &FileOpts{
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

		ext := strings.ToLower(filepath.Ext(hdr.Name))

		switch ext {
		case ".ipa":
			return tr, ContentTypeIPA, nil
		case ".apk":
			return tr, ContentTypeAPK, nil
		}
	}

	return nil, "", fmt.Errorf("no app found before EOF")
}

// MultipartToTar converts a *multipart.Reader to a io.Reader.
func MultipartToTar(mr *multipart.Reader, opts ...FileOpt) io.Reader {
	o := &FileOpts{
		Mode: 0777,
	}

	for _, opt := range opts {
		opt.Apply(o)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()

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
						Mode: o.Mode,
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
