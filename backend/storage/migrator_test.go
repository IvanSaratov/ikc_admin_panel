package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func migrationFS(includeSecond bool) fstest.MapFS {
	migrations := fstest.MapFS{
		"001_items.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
PRAGMA application_id = 0x494B4341;
CREATE TABLE items (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE items;
`)},
	}
	if includeSecond {
		migrations["002_item_status.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up
ALTER TABLE items ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

-- +goose Down
ALTER TABLE items DROP COLUMN status;
`)}
	}
	return migrations
}

func TestMigratorStatusAndUpAreIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "migrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := NewMigrator(db, migrationFS(true))
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}

	wantPending := []MigrationInfo{
		{Version: 1, Name: "001_items.sql"},
		{Version: 2, Name: "002_item_status.sql"},
	}
	catalog := migrator.Catalog()
	assertMigrationStatus(t, catalog, 0, 2, wantPending)

	status, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	assertMigrationStatus(t, status, 0, 2, wantPending)

	first, err := migrator.Up(ctx)
	if err != nil {
		t.Fatalf("first Up() error = %v", err)
	}
	if first.From != 0 || first.To != 2 || len(first.Applied) != 2 {
		t.Fatalf("first Up() = %+v, want From 0, To 2, 2 applied migrations", first)
	}

	second, err := migrator.Up(ctx)
	if err != nil {
		t.Fatalf("second Up() error = %v", err)
	}
	if second.From != 2 || second.To != 2 || len(second.Applied) != 0 {
		t.Fatalf("second Up() = %+v, want From 2, To 2, no applied migrations", second)
	}
}

func TestMigratorRejectsSchemaNewerThanBinary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "migrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	current, err := NewMigrator(db, migrationFS(true))
	if err != nil {
		t.Fatalf("NewMigrator(current) error = %v", err)
	}
	if _, err := current.Up(ctx); err != nil {
		t.Fatalf("current Up() error = %v", err)
	}

	old, err := NewMigrator(db, migrationFS(false))
	if err != nil {
		t.Fatalf("NewMigrator(old) error = %v", err)
	}
	_, err = old.Up(ctx)
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("old Up() error = %v, want ErrSchemaTooNew", err)
	}
}

func assertMigrationStatus(t *testing.T, got MigrationStatus, current, target int64, pending []MigrationInfo) {
	t.Helper()
	if got.Current != current || got.Target != target {
		t.Errorf("status versions = (%d, %d), want (%d, %d)", got.Current, got.Target, current, target)
	}
	if len(got.Pending) != len(pending) {
		t.Fatalf("status pending = %+v, want %+v", got.Pending, pending)
	}
	for i := range pending {
		if got.Pending[i] != pending[i] {
			t.Errorf("status pending[%d] = %+v, want %+v", i, got.Pending[i], pending[i])
		}
	}
}
