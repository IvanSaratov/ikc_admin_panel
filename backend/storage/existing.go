package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenExisting opens the already-owned database without allowing SQLite to
// create a missing file. expectedPath must be the absolute, clean, canonical
// path returned by OwnerLock.DatabasePath. The expected path is deliberately
// never canonicalized again: doing so after lock acquisition could silently
// follow a retargeted final symlink to a different database.
func OpenExisting(ctx context.Context, expectedPath string) (*sql.DB, error) {
	return openOwnedDatabase(ctx, expectedPath, "rw", true)
}

// OpenForPreparation opens an owned SQLite path and may create a missing file,
// but deliberately applies no connection PRAGMAs. expectedPath has the same
// immutable OwnerLock path contract as OpenExisting. Initializer.Prepare
// performs configuration only after identity and version validation.
func OpenForPreparation(ctx context.Context, expectedPath string) (*sql.DB, error) {
	return openOwnedDatabase(ctx, expectedPath, "rwc", false)
}

// OpenOwnedReadOnly opens an existing owned path in mode=ro and verifies that
// SQLite resolved it to the immutable path protected by OwnerLock. expectedPath
// has the same path contract as OpenExisting. No connection PRAGMAs are applied.
func OpenOwnedReadOnly(ctx context.Context, expectedPath string) (*sql.DB, error) {
	return openOwnedDatabase(ctx, expectedPath, "ro", false)
}

func openOwnedDatabase(ctx context.Context, expectedPath, mode string, configureConnection bool) (*sql.DB, error) {
	if expectedPath == "" {
		return nil, errors.New("owned database path is required")
	}
	if !filepath.IsAbs(expectedPath) {
		return nil, fmt.Errorf("owned database path must be absolute: %q", expectedPath)
	}
	if filepath.Clean(expectedPath) != expectedPath {
		return nil, errors.New("owned database path must be clean")
	}

	db, err := sql.Open(sqliteDriverName, ownedDatabaseDSN(filepath.ToSlash(expectedPath), mode))
	if err != nil {
		return nil, fmt.Errorf("open owned SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(operationErr error) (*sql.DB, error) {
		return nil, errors.Join(operationErr, db.Close())
	}

	if err := db.PingContext(ctx); err != nil {
		return fail(fmt.Errorf("open owned SQLite database: %w", err))
	}
	actualPath, err := mainDatabasePath(ctx, db)
	if err != nil {
		return fail(fmt.Errorf("inspect owned SQLite database path: %w", err))
	}
	canonicalActualPath, err := canonicalDatabasePath(actualPath)
	if err != nil {
		return fail(fmt.Errorf("canonicalize actual SQLite database path: %w", err))
	}
	if !sameOwnedDatabasePathLiteral(expectedPath, canonicalActualPath) {
		return fail(fmt.Errorf(
			"actual SQLite database path %q differs from owned database path %q",
			canonicalActualPath,
			expectedPath,
		))
	}

	if configureConnection {
		if err := configure(ctx, db); err != nil {
			return fail(err)
		}
	}
	return db, nil
}

func sameOwnedDatabasePathLiteral(expectedPath, actualPath string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(expectedPath, actualPath)
	}
	return expectedPath == actualPath
}

func existingDatabaseDSN(path string) string {
	return ownedDatabaseDSN(path, "rw")
}

func preparationDatabaseDSN(path string) string {
	return ownedDatabaseDSN(path, "rwc")
}

func ownedDatabaseDSN(path, mode string) string {
	databaseURL := sqliteFileURL(path)
	query := url.Values{}
	query.Set("mode", mode)
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}
