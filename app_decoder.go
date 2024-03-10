package momo

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path/filepath"
	"strings"
)

var (
	ErrIconNotFound = errors.New("icon not found")
)

type AppDecoder interface {
	Icons(context.Context) (io.Reader, error)
	Close() error
}

func BestFitIcon(ctx context.Context, dimensions int, extension string, appDecoders ...AppDecoder) (io.Reader, error) {
	if dimensions <= 0 {
		return nil, fmt.Errorf("dimensions <0")
	} else if extension == "" {
		return nil, fmt.Errorf("extension empty")
	}

	var (
		bestFitDimensions int
		bestFitBytes      []byte
	)

	for _, appDecoder := range appDecoders {
		icons, err := appDecoder.Icons(ctx)
		if err != nil {
			return nil, err
		}

		tr := tar.NewReader(icons)
		for {
			hdr, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, err
			}

			var (
				img image.Image
				ext = strings.ToLower(filepath.Ext(hdr.Name))
			)
			if !strings.EqualFold(ext, extension) {
				continue
			}

			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}

			switch ext {
			case ".png":
				img, err = png.Decode(bytes.NewReader(b))
			case ".jpg", "jpeg":
				img, err = jpeg.Decode(bytes.NewReader(b))
			}
			if err != nil {
				continue
			}

			if imgDimensions := img.Bounds().Dx(); (bestFitDimensions < dimensions && imgDimensions > bestFitDimensions) ||
				(bestFitDimensions > dimensions && imgDimensions < bestFitDimensions && imgDimensions >= dimensions) {
				bestFitDimensions = imgDimensions
				bestFitBytes = b
			}

			if bestFitDimensions == dimensions {
				break
			}
		}
	}

	if bestFitDimensions == 0 || len(bestFitBytes) == 0 {
		return nil, ErrIconNotFound
	}

	return bytes.NewReader(bestFitBytes), nil
}
