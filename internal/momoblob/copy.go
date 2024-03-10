package momoblob

import (
	"context"
	"io"

	"gocloud.dev/blob"
)

func Copy(ctx context.Context, bucket *blob.Bucket, key string, r io.Reader) error {
	w, err := bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return err
	}

	if _, err := io.Copy(w, r); err != nil {
		return err
	}

	if err = w.Close(); err != nil {
		return err
	}

	return nil
}
