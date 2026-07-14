package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func OpenReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("database path must be absolute: %q", path)
	}

	dsn := readOnlyDatabaseDSN(filepath.ToSlash(filepath.Clean(path)))

	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open read-only SQLite database: %w", err)
	}
	return db, nil
}

func readOnlyDatabaseDSN(path string) string {
	databaseURL := url.URL{Scheme: "file"}
	switch {
	case strings.HasPrefix(path, "//"):
		databaseURL.Host, path, _ = strings.Cut(strings.TrimPrefix(path, "//"), "/")
		if path != "" {
			databaseURL.Path = "/" + path
		}
	case len(path) >= 3 && path[1] == ':' && path[2] == '/':
		databaseURL.Path = "/" + path
	default:
		databaseURL.Path = path
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "query_only(1)")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}
