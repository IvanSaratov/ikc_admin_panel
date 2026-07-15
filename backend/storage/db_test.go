package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
)

func TestOpenAndMigrateConfiguresSQLiteConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ikc-test.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys pragma = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestConstraintErrorMappingDetectsUniqueViolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ikc-test.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	insertWorker(t, ctx, db, "worker@example.test")
	err = insertWorkerRaw(ctx, db, "worker-duplicate@example.test")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("duplicate worker error = %v, want ErrConflict", err)
	}
}

func insertWorker(t *testing.T, ctx context.Context, db *sql.DB, email string) {
	t.Helper()

	if err := insertWorkerRaw(ctx, db, email); err != nil {
		t.Fatalf("insert worker: %v", err)
	}
}

func insertWorkerRaw(ctx context.Context, db *sql.DB, email string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO workers (
			last_name,
			first_name,
			middle_name,
			snils,
			snils_normalized,
			email,
			birth_date,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"Petrov",
		"Petr",
		nil,
		"123-456-789 00",
		"12345678900",
		email,
		nil,
		"2026-05-27T00:00:00Z",
		"2026-05-27T00:00:00Z",
	)
	return storage.MapSQLiteError(err)
}
