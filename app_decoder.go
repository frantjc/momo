package momo

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
)

type AppDecoder interface {
	Icons(context.Context) (io.Reader, error)
	Close() error
}

func BestFitIcon(ctx context.Context, dimensions int, appDecoders ...AppDecoder) (image.Image, error) {
	if dimensions <= 0 {
		return nil, fmt.Errorf("dimensions <0")
	}

	var bestFitImg image.Image
	for _, appDecoder := range appDecoders {
		icons, err := appDecoder.Icons(ctx)
		if err != nil {
			return nil, err
		}

		tr := tar.NewReader(icons)
		for {
			if _, err := tr.Next(); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, err
			}

			img, _, err := image.Decode(tr)
			if err != nil {
				return nil, err
			}

			var (
				bestFitDimensions = 0
				imgDimensions     = img.Bounds().Dx()
			)
			if bestFitImg != nil {
				bestFitDimensions = bestFitImg.Bounds().Dx()
			}

			if (bestFitDimensions < dimensions && imgDimensions > bestFitDimensions) ||
				(bestFitDimensions > dimensions && imgDimensions < bestFitDimensions && imgDimensions >= dimensions) {
				bestFitImg = img
				if imgDimensions == dimensions {
					break
				}
			}
		}
	}

	if bestFitImg == nil {
		return nil, fmt.Errorf("icon not found")
	}

	return bestFitImg, nil
}
