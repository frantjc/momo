package momoutil

import (
	"archive/tar"
	"errors"
	"io"
	"mime/multipart"
)

type TarOpts struct {
	Mode int64
}

type TarOpt interface {
	Apply(*TarOpts)
}

func (t *TarOpts) Apply(opts *TarOpts) {
	opts.Mode = t.Mode
}

func FileToTar(r io.Reader, name string, opts ...TarOpt) io.Reader {
	o := &TarOpts{
		Mode: 0777,
	}

	for _, opt := range opts {
		opt.Apply(o)
	}

	pr, pw := io.Pipe()

	go func() {
		tw := tar.NewWriter(pw)
		defer tw.Close()

		if err := func() error {
			if err := tw.WriteHeader(&tar.Header{
				Name: name,
				Mode: o.Mode,
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

// MultipartToTar converts a *multipart.Reader to a io.Reader.
func MultipartToTar(mr *multipart.Reader, opts ...TarOpt) io.Reader {
	o := &TarOpts{
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
