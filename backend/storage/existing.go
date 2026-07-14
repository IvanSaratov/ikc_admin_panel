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
	if expectedPath == "" {
		return nil, errors.New("owned database path is required")
	}
	if !filepath.IsAbs(expectedPath) {
		return nil, fmt.Errorf("owned database path must be absolute: %q", expectedPath)
	}
	if filepath.Clean(expectedPath) != expectedPath {
		return nil, errors.New("owned database path must be clean")
	}

	db, err := sql.Open(sqliteDriverName, existingDatabaseDSN(filepath.ToSlash(expectedPath)))
	if err != nil {
		return nil, fmt.Errorf("open existing SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(operationErr error) (*sql.DB, error) {
		return nil, errors.Join(operationErr, db.Close())
	}

	if err := db.PingContext(ctx); err != nil {
		return fail(fmt.Errorf("open existing SQLite database: %w", err))
	}
	actualPath, err := mainDatabasePath(ctx, db)
	if err != nil {
		return fail(fmt.Errorf("inspect existing SQLite database path: %w", err))
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

	if err := configure(ctx, db); err != nil {
		return fail(err)
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
	databaseURL := sqliteFileURL(path)
	query := url.Values{}
	query.Set("mode", "rw")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}
