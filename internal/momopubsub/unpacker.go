package momopubsub

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	"github.com/frantjc/momo/internal/momoblob"
	"github.com/frantjc/momo/internal/momosql"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
)

func Unpack(ctx context.Context, bucket *blob.Bucket, tr *tar.Reader, app *momo.App) error {
	var (
		foundDisplayImage, foundFullSizeImage bool
		apkD                                  *android.APKDecoder
		ipaD                                  *ios.IPADecoder
	)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		switch {
		case err != nil:
			return err
		case strings.HasPrefix(hdr.Name, "._"):
			continue
		}

		var (
			base = filepath.Base(hdr.Name)
			ext  = filepath.Ext(base)
		)
		switch {
		case strings.EqualFold(ext, ".apk"):
			if apkD != nil {
				return fmt.Errorf("found multiple .apks")
			}

			f, err := os.CreateTemp("", app.ID+"-*.apk")
			if err != nil {
				return err
			}

			apkW, err := bucket.NewWriter(ctx, momoblob.APKKey(app.ID), nil)
			if err != nil {
				return err
			}

			if _, err := io.Copy(io.MultiWriter(f, apkW), tr); err != nil {
				return err
			}

			if err = f.Close(); err != nil {
				return err
			}

			if err = apkW.Close(); err != nil {
				return err
			}

			apkD = android.NewAPKDecoder(f.Name())

			if app.Version == "" {
				if metadata, err := apkD.Metadata(ctx); err == nil {
					if metadata.VersionInfo.VersionName != "" {
						app.Version = metadata.VersionInfo.VersionName
					} else if metadata.VersionInfo.VersionCode > 0 {
						app.Version = fmt.Sprint(metadata.VersionInfo.VersionCode)
					}
				}
			}

			if sha256CertFingerprints, err := apkD.SHA256CertFingerprints(ctx); err == nil {
				app.SHA256CertFingerprints = sha256CertFingerprints
			}
		case strings.EqualFold(ext, ".ipa"):
			if ipaD != nil {
				return fmt.Errorf("found multiple .ipas")
			}

			f, err := os.CreateTemp("", app.ID+"-*.ipa")
			if err != nil {
				return err
			}

			ipaW, err := bucket.NewWriter(ctx, momoblob.IPAKey(app.ID), nil)
			if err != nil {
				return err
			}

			if _, err := io.Copy(io.MultiWriter(f, ipaW), tr); err != nil {
				return err
			}

			if err = f.Close(); err != nil {
				return err
			}

			if err = ipaW.Close(); err != nil {
				return err
			}

			ipaD = ios.NewIPADecoder(f.Name())

			info, err := ipaD.Info(ctx)
			if err != nil {
				return err
			}

			app.BundleName = xslice.Coalesce(info.CFBundleName, info.CFBundleDisplayName)
			app.BundleIdentifier = info.CFBundleIdentifier
			if app.Version == "" {
				app.Version = info.CFBundleVersion
			}
		case strings.EqualFold(ext, ".png"):
			switch {
			case strings.Contains(base, "full") && strings.Contains(base, "size"):
				img, _, err := image.Decode(tr)
				if err != nil {
					return err
				}

				w, err := bucket.NewWriter(ctx, momoblob.FullSizeImageKey(app.ID), nil)
				if err != nil {
					return err
				}

				if err = momoblob.WriteImage(w, img); err != nil {
					return err
				}

				foundFullSizeImage = true
			case strings.Contains(base, "display"):
				img, _, err := image.Decode(tr)
				if err != nil {
					return err
				}

				w, err := bucket.NewWriter(ctx, momoblob.DisplayImageKey(app.ID), nil)
				if err != nil {
					return err
				}

				if err = momoblob.WriteImage(w, img); err != nil {
					return err
				}

				foundDisplayImage = true
			}
		}
	}

	appDs := []momo.AppDecoder{}
	if apkD != nil {
		appDs = append(appDs, apkD)
	}

	if ipaD != nil {
		appDs = append(appDs, ipaD)
	}

	if len(appDs) == 0 {
		return fmt.Errorf("no apps found")
	}

	if !foundFullSizeImage {
		if fullSizeImage, err := momo.BestFitIcon(ctx, 512, appDs...); err == nil {
			iconW, err := bucket.NewWriter(ctx, momoblob.FullSizeImageKey(app.ID), nil)
			if err != nil {
				return err
			}

			if err = momoblob.WriteImage(iconW, fullSizeImage); err != nil {
				return err
			}
		}
	}

	if !foundDisplayImage {
		if displayImage, err := momo.BestFitIcon(ctx, 57, appDs...); err == nil {
			iconW, err := bucket.NewWriter(ctx, momoblob.DisplayImageKey(app.ID), nil)
			if err != nil {
				return err
			}

			if err = momoblob.WriteImage(iconW, displayImage); err != nil {
				return err
			}
		}
	}

	for _, appD := range appDs {
		if err := appD.Close(); err != nil {
			return err
		}
	}

	app.Status = "unpacked"

	return nil
}

func Receive(ctx context.Context, bucket *blob.Bucket, db *sql.DB, subscription *pubsub.Subscription) error {
	var (
		log = momo.LoggerFrom(ctx)
		_   = log
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := subscription.Receive(ctx)
			if err != nil {
				return err
			}

			app := &momo.App{
				ID: string(msg.Body),
			}

			r, err := bucket.NewReader(ctx, momoblob.TGZKey(app.ID), nil)
			if err != nil {
				return err
			}

			gr, err := gzip.NewReader(r)
			if err != nil {
				return err
			}

			if err := momosql.SelectApp(ctx, db, app); err != nil {
				return err
			}

			if err = Unpack(ctx, bucket, tar.NewReader(gr), app); err != nil {
				return err
			}

			if err = momosql.UpdateApp(ctx, db, app); err != nil {
				return err
			}

			if err = gr.Close(); err != nil {
				return err
			}

			if err = r.Close(); err != nil {
				return err
			}

			msg.Ack()
		}
	}
}
