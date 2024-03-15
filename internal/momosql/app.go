package momosql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoerr"
	"github.com/google/uuid"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS app (
	id VARCHAR (36) PRIMARY KEY,
	name VARCHAR (32) NOT NULL,
	version VARCHAR (32),
	status TEXT NOT NULL DEFAULT 'unknown',
	sha256_cert_fingerprints VARCHAR (128),
	bundle_name VARCHAR (64),
	bundle_identifier VARCHAR (64),
	created TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	UNIQUE (name, version)
);`); err != nil {
		return err
	}

	return nil
}

func SelectApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	if app.ID != "" {
		if err := db.QueryRowContext(ctx,
			"SELECT name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE id = $1",
			app.ID,
		).Scan(&app.Name, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); errors.Is(err, sql.ErrNoRows) {
			return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
	} else if app.Name != "" && app.Version != "" {
		if err := db.QueryRowContext(ctx,
			"SELECT id, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE name = $1 AND version = $2",
			app.Name, app.Version,
		).Scan(&app.ID, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); errors.Is(err, sql.ErrNoRows) {
			return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
	} else if app.Name != "" {
		if err := db.QueryRowContext(ctx,
			"SELECT id, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE name = $1 ORDER BY created",
			app.Name,
		).Scan(&app.ID, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); errors.Is(err, sql.ErrNoRows) {
			return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unable to uniquely identify app")
	}

	return nil
}

func SelectApps(ctx context.Context, db *sql.DB, limit, offset int) ([]momo.App, error) {
	_limit := "ALL"
	if limit > 0 {
		_limit = fmt.Sprint(limit)
	}

	rows, err := db.QueryContext(ctx,
		"SELECT id, name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app ORDER BY created LIMIT $1 OFFSET $2",
		_limit, offset,
	)
	if err != nil {
		return nil, err
	}

	apps := []momo.App{}
	for rows.Next() {
		app := momo.App{}

		if err = rows.Scan(&app.ID, &app.Name, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); err != nil {
			return nil, err
		}

		apps = append(apps, app)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if err = rows.Close(); err != nil {
		return nil, err
	}

	return apps, nil
}

func InsertApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	app.ID = uuid.NewString()

	if err := db.QueryRowContext(ctx,
		"INSERT INTO app (id, name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier) VALUES ($1, $2, $3, $4, $5, $6, $7) returning created, updated",
		app.ID, app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier,
	).Scan(&app.Created, &app.Updated); errors.Is(err, sql.ErrNoRows) {
		return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
	} else if err != nil {
		return err
	}

	return nil
}

func UpdateApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	if app.ID != "" {
		app.Updated = time.Now()

		if err := db.QueryRowContext(ctx,
			"UPDATE app SET (name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, updated) = ($2, $3, $4, $5, $6, $7, $8) WHERE id = $1 RETURNING created",
			app.ID, app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier, app.Updated,
		).Scan(&app.Created); errors.Is(err, sql.ErrNoRows) {
			return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
	} else if app.Name != "" && app.Version != "" {
		app.Updated = time.Now()

		if err := db.QueryRowContext(ctx,
			"UPDATE app SET (status, sha256_cert_fingerprints, bundle_name, bundle_identifier, updated) = ($3, $4, $5, $6, $7) WHERE name = $1 AND version = $2 RETURNING id, created",
			app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier, app.Updated,
		).Scan(&app.ID, &app.Created); errors.Is(err, sql.ErrNoRows) {
			return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
		} else if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unable to uniquely identify app")
	}

	return nil
}
