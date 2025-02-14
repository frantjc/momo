package android

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo/apktool"
	xslice "github.com/frantjc/x/slice"
	"gopkg.in/yaml.v3"
)

type APKDecoder struct {
	Name string

	apktool  string
	keytool  string
	dir      string
	decoded  bool
	manifest *Manifest
	metadata *apktool.Metadata
	icons    map[int]string
}

type APKDecoderOpt func(*APKDecoder)

func WithAPKTool(b string) APKDecoderOpt {
	return func(a *APKDecoder) {
		a.apktool = b
	}
}

func WithKeytool(b string) APKDecoderOpt {
	return func(a *APKDecoder) {
		a.keytool = b
	}
}

func WithDir(dir string) APKDecoderOpt {
	return func(a *APKDecoder) {
		a.dir = dir
	}
}

func NewAPKDecoder(name string, opts ...APKDecoderOpt) *APKDecoder {
	ad := &APKDecoder{Name: name, keytool: "keytool", apktool: "apktool"}

	for _, opt := range opts {
		opt(ad)
	}

	return ad
}

func (a *APKDecoder) decode(ctx context.Context) error {
	if a.decoded {
		return nil
	} else if a.dir == "" {
		var err error
		a.dir, err = os.MkdirTemp(filepath.Dir(a.Name), "*")
		if err != nil {
			return err
		}
	}

	opts := &apktool.DecodeOpts{
		Force:           true,
		NoSources:       true,
		OutputDirectory: a.dir,
	}

	if err := apktool.Command(a.apktool).Decode(ctx, a.Name, opts); err != nil {
		return err
	}

	a.decoded = true

	return nil
}

func (a *APKDecoder) Manifest(ctx context.Context) (*Manifest, error) {
	if err := a.decode(ctx); err != nil {
		return nil, err
	}

	if a.manifest != nil {
		return a.manifest, nil
	}

	file, err := os.Open(filepath.Join(a.dir, AndroidManifestName))
	if err != nil {
		return nil, err
	}

	a.manifest = &Manifest{}
	return a.manifest, xml.NewDecoder(file).Decode(a.manifest)
}

func (a *APKDecoder) Metadata(ctx context.Context) (*apktool.Metadata, error) {
	if err := a.decode(ctx); err != nil {
		return nil, err
	}

	if a.metadata != nil {
		return a.metadata, nil
	}

	f, err := os.Open(filepath.Join(a.dir, "apktool.yml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	a.metadata = &apktool.Metadata{}
	return a.metadata, yaml.NewDecoder(f).Decode(a.metadata)
}

func (a *APKDecoder) SHA256CertFingerprints(ctx context.Context) (string, error) {
	var (
		buf = new(bytes.Buffer)
		//nolint:gosec
		// TODO: Refactor to pkg like apktool.
		cmd = exec.CommandContext(ctx, a.keytool, "-printcert", "-jarfile", a.Name)
	)

	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "SHA256: ") {
			if fields := strings.Fields(line); len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("sha256 cert fingerprints not found")
}

func (a *APKDecoder) Icons(ctx context.Context) (io.Reader, error) {
	if _, err := a.Manifest(ctx); err != nil {
		return nil, err
	}

	var (
		iconType = "drawable"
		iconName = "ic_launcher"
	)

	for _, attr := range a.manifest.Attrs {
		if attr.Name.Space == "http://schemas.android.com/apk/res/android" && attr.Name.Local == "icon" {
			iconType, iconName = path.Split(attr.Value)
			iconType = strings.TrimPrefix(iconType, "@")
			break
		}
	}

	var (
		pr, pw = io.Pipe()
		tw     = tar.NewWriter(pw)
	)

	go func() {
		err := filepath.WalkDir(a.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			} else if d.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(a.dir, path)
			if err != nil {
				return err
			}

			var (
				base = strings.ToLower(filepath.Base(rel))
				ext  = filepath.Ext(base)
			)

			if xslice.Includes([]string{".png", ".jpg", ".jpeg"}, ext) && strings.Contains(rel, iconType) && strings.Contains(base, iconName) {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()

				fi, err := d.Info()
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

				if _, err = io.Copy(tw, f); err != nil {
					return err
				}
			}

			return nil
		})

		_ = tw.Close()
		_ = pw.CloseWithError(err)
	}()

	return pr, nil
}

func (a *APKDecoder) Close() error {
	if a.decoded {
		if err := os.RemoveAll(a.dir); err != nil {
			return err
		}
	}

	a.decoded = false
	a.metadata = nil
	a.manifest = nil
	a.icons = nil

	return os.Remove(a.Name)
}
