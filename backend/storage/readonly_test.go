package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyPermitsReadsAndRejectsWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "readonly.db")
	writable, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := writable.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writable.ExecContext(ctx, `INSERT INTO items (name) VALUES ('existing')`); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable database: %v", err)
	}

	readonly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer readonly.Close()

	var name string
	if err := readonly.QueryRowContext(ctx, `SELECT name FROM items WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("SELECT error = %v", err)
	}
	if name != "existing" {
		t.Fatalf("SELECT name = %q, want existing", name)
	}
	if _, err := readonly.ExecContext(ctx, `INSERT INTO items (name) VALUES ('forbidden')`); err == nil {
		t.Fatal("INSERT error = nil, want read-only failure")
	}
}

func TestOpenReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(ctx, path); err == nil {
		t.Fatal("OpenReadOnly() error = nil, want missing database error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("os.Stat() error = %v, want file to remain missing", err)
	}
}

func TestOpenReadOnlyRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "relative.db"} {
		if _, err := OpenReadOnly(context.Background(), path); err == nil {
			t.Fatalf("OpenReadOnly(%q) error = nil, want invalid path error", path)
		}
	}
}
