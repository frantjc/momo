package ios

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo"
	xslice "github.com/frantjc/x/slice"
	"howett.net/plist"
)

const (
	InfoPlistName = "Info.plist"
)

type IPADecoder struct {
	Name string

	info    *Info
	infoDir string
}

func NewIPADecoder(name string) *IPADecoder {
	id := &IPADecoder{Name: name}

	return id
}

func (i *IPADecoder) zipReader() (*zip.Reader, error) {
	ipa, err := os.Open(i.Name)
	if err != nil {
		return nil, err
	}

	ipafi, err := ipa.Stat()
	if err != nil {
		return nil, err
	}

	return zip.NewReader(ipa, ipafi.Size())
}

func (i *IPADecoder) infoFromZipReader(zr *zip.Reader) (*Info, error) {
	for _, zf := range zr.File {
		if strings.EqualFold(InfoPlistName, filepath.Base(zf.Name)) {
			fsf, err := zr.Open(zf.Name)
			if err != nil {
				return nil, err
			}

			b, err := io.ReadAll(fsf)
			if err != nil {
				return nil, err
			}

			i.infoDir = filepath.Dir(zf.Name)
			i.info = &Info{}
			return i.info, plist.NewDecoder(bytes.NewReader(b)).Decode(i.info)
		}
	}

	return nil, fmt.Errorf("info not found in .ipa")
}

func (i *IPADecoder) Info(_ context.Context) (*Info, error) {
	if i.info != nil {
		return i.info, nil
	}

	zr, err := i.zipReader()
	if err != nil {
		return nil, err
	}

	return i.infoFromZipReader(zr)
}

var (
	_ momo.AppDecoder = &IPADecoder{}
)

func (i *IPADecoder) Icons(_ context.Context) (io.Reader, error) {
	zr, err := i.zipReader()
	if err != nil {
		return nil, err
	}

	if _, err := i.infoFromZipReader(zr); err != nil {
		return nil, err
	}

	var (
		pr, pw = io.Pipe()
		tw     = tar.NewWriter(pw)
	)

	go func() {
		if err := func() error {
			for _, bif := range i.info.CFBundleIconFiles {
				if xslice.Includes([]string{".png", ".jpg", ".jpeg"}, filepath.Ext(bif)) {
					fsf, err := zr.Open(filepath.Join(i.infoDir, bif))
					if err != nil {
						return err
					}

					fi, err := fsf.Stat()
					if err != nil {
						return err
					}

					hdr, err := tar.FileInfoHeader(fi, fi.Name())
					if err != nil {
						return err
					}

					if err = tw.WriteHeader(hdr); err != nil {
						return err
					}

					if _, err = io.Copy(tw, fsf); err != nil {
						return err
					}
				}
			}

			return nil
		}(); err != nil {
			_ = tw.Close()
			_ = pw.CloseWithError(err)
			return
		}

		_ = tw.Close()
		_ = pw.Close()
	}()

	return pr, nil
}

func (i *IPADecoder) Close() error {
	i.info = nil
	i.infoDir = ""
	return os.Remove(i.Name)
}
