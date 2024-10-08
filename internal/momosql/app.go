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
	"github.com/lib/pq"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS app (
	id UUID PRIMARY KEY,
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
		return wrapErr(err)
	}

	return nil
}

func SelectApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	switch {
	case app.ID != "":
		if err := db.QueryRowContext(ctx,
			"SELECT name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE id = $1",
			app.ID,
		).Scan(&app.Name, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); err != nil {
			return wrapErr(err)
		}
	case app.Name != "" && app.Version != "":
		if err := db.QueryRowContext(ctx,
			"SELECT id, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE name = $1 AND version = $2",
			app.Name, app.Version,
		).Scan(&app.ID, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); err != nil {
			return wrapErr(err)
		}
	case app.Name != "":
		if err := db.QueryRowContext(ctx,
			"SELECT id, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app WHERE name = $1 ORDER BY created",
			app.Name,
		).Scan(&app.ID, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); err != nil {
			return wrapErr(err)
		}
	default:
		return momoerr.HTTPStatusCodeError(
			fmt.Errorf("unable to uniquely identify app"),
			http.StatusBadRequest,
		)
	}

	return nil
}

func SelectApps(ctx context.Context, db *sql.DB, limit, offset int) ([]momo.App, error) {
	var (
		_limit  = 0
		_offset = 0
	)
	if limit > 0 {
		_limit = limit
	}

	if offset > 0 {
		_offset = offset
	}

	var (
		rows *sql.Rows
		err  error
	)
	if _limit > 0 {
		rows, err = db.QueryContext(ctx,
			"SELECT id, name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app ORDER BY created LIMIT $1 OFFSET $2",
			_limit, _offset,
		)
	} else {
		rows, err = db.QueryContext(ctx,
			"SELECT id, name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, created, updated FROM app ORDER BY created OFFSET $1",
			_offset,
		)
	}
	if err != nil {
		return nil, wrapErr(err)
	}

	apps := []momo.App{}
	for rows.Next() {
		app := momo.App{}

		if err = rows.Scan(&app.ID, &app.Name, &app.Version, &app.Status, &app.SHA256CertFingerprints, &app.BundleName, &app.BundleIdentifier, &app.Created, &app.Updated); err != nil {
			return nil, wrapErr(err)
		}

		apps = append(apps, app)
	}

	if err = rows.Err(); err != nil {
		return nil, wrapErr(err)
	}

	if err = rows.Close(); err != nil {
		return nil, wrapErr(err)
	}

	return apps, nil
}

func InsertApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	app.ID = uuid.NewString()

	if err := db.QueryRowContext(ctx,
		"INSERT INTO app (id, name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier) VALUES ($1, $2, $3, $4, $5, $6, $7) returning created, updated",
		app.ID, app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier,
	).Scan(&app.Created, &app.Updated); err != nil {
		return wrapErr(err)
	}

	return nil
}

func UpdateApp(ctx context.Context, db *sql.DB, app *momo.App) error {
	switch {
	case app.ID != "":
		app.Updated = time.Now()

		if err := db.QueryRowContext(ctx,
			"UPDATE app SET (name, version, status, sha256_cert_fingerprints, bundle_name, bundle_identifier, updated) = ($2, $3, $4, $5, $6, $7, $8) WHERE id = $1 RETURNING created",
			app.ID, app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier, app.Updated,
		).Scan(&app.Created); err != nil {
			return wrapErr(err)
		}
	case app.Name != "" && app.Version != "":
		app.Updated = time.Now()

		if err := db.QueryRowContext(ctx,
			"UPDATE app SET (status, sha256_cert_fingerprints, bundle_name, bundle_identifier, updated) = ($3, $4, $5, $6, $7) WHERE name = $1 AND version = $2 RETURNING id, created",
			app.Name, app.Version, app.Status, app.SHA256CertFingerprints, app.BundleName, app.BundleIdentifier, app.Updated,
		).Scan(&app.ID, &app.Created); err != nil {
			return wrapErr(err)
		}
	default:
		return momoerr.HTTPStatusCodeError(
			fmt.Errorf("unable to uniquely identify app"),
			http.StatusBadRequest,
		)
	}

	return nil
}

var (
	httpStatusCodes = map[pq.ErrorCode]int{
		"23505": http.StatusConflict,
	}
)

func wrapErr(err error) error {
	pqErr := &pq.Error{}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return momoerr.HTTPStatusCodeError(err, http.StatusNotFound)
	case errors.As(err, &pqErr):
		if httpStatusCode, ok := httpStatusCodes[pqErr.Code]; ok {
			return momoerr.HTTPStatusCodeError(pqErr, httpStatusCode)
		}
	}

	return err
}

func DeleteApp(ctx context.Context, db *sql.DB, appID string) error {
	if appID != "" {
		if _, err := db.ExecContext(ctx,
			"DELETE FROM app WHERE id = $1",
			appID,
		); err != nil {
			return wrapErr(err)
		}
	} else {
		return momoerr.HTTPStatusCodeError(
			fmt.Errorf("unable to uniquely identify app"),
			http.StatusBadRequest,
		)
	}

	return nil
}
