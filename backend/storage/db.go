package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

// Open opens a SQLite file database and applies the connection settings the app
// relies on in production.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	db, err := sql.Open(sqliteDriverName, filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// SQLite pragmas are connection-local. Keep one connection until we have a
	// tested connector that applies pragmas on every new pooled connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := configure(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func configure(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}

	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure sqlite %q: %w", pragma, err)
		}
	}

	return nil
}
