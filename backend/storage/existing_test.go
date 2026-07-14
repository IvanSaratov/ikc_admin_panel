package storage

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExistingDatabaseDSNUsesReadWriteExistingMode(t *testing.T) {
	t.Parallel()

	dsn := existingDatabaseDSN("/var/lib/IKC app.db")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN %q: %v", dsn, err)
	}
	if got := parsed.Query().Get("mode"); got != "rw" {
		t.Fatalf("mode = %q, want rw", got)
	}
	if pragmas := parsed.Query()["_pragma"]; len(pragmas) != 0 {
		t.Fatalf("pre-validation pragmas = %q, want none", pragmas)
	}
}

func TestOpenExistingDoesNotCreateMissingDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.db")
	db, err := OpenExisting(context.Background(), path)
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenExisting() database != nil, want nil")
	}
	if err == nil {
		t.Fatal("OpenExisting() error = nil, want missing database failure")
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("database stat error = %v, want not exist", statErr)
	}
}

func TestOpenExistingConfiguresCanonicalDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.db")
	created, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if _, err := created.ExecContext(ctx, `CREATE TABLE marker (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create marker table: %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("close created database: %v", err)
	}

	owner, err := AcquireOwnerLock(path)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()
	db, err := OpenExisting(ctx, owner.DatabasePath())
	if err != nil {
		t.Fatalf("OpenExisting() error = %v", err)
	}
	defer db.Close()

	var foreignKeys int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	var markerTables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE name = 'marker'`).Scan(&markerTables); err != nil {
		t.Fatalf("read marker table: %v", err)
	}
	if markerTables != 1 {
		t.Fatalf("marker table count = %d, want 1", markerTables)
	}
}

func TestOpenExistingDoesNotRecreateOwnedDatabaseDeletedAfterLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "deleted-after-lock.db")
	created, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if _, err := created.ExecContext(ctx, `CREATE TABLE marker (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create marker table: %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("close created database: %v", err)
	}
	owner, err := AcquireOwnerLock(path)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()
	if err := os.Remove(path); err != nil {
		t.Fatalf("delete database after ownership: %v", err)
	}

	db, err := OpenExisting(ctx, owner.DatabasePath())
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenExisting() database != nil, want deleted database failure")
	}
	if err == nil {
		t.Fatal("OpenExisting() error = nil, want deleted database failure")
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("database stat error = %v, want file to remain missing", statErr)
	}
}

func TestOpenExistingRejectsRetargetedCanonicalEntryBeforeConfigure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("final-entry symlink replacement requires elevated Windows privileges")
	}

	ctx := context.Background()
	root := t.TempDir()
	realPath := filepath.Join(root, "owned.db")
	alternatePath := filepath.Join(root, "alternate.db")
	createSQLiteWithJournalMode(t, ctx, realPath, "DELETE")
	createSQLiteWithJournalMode(t, ctx, alternatePath, "DELETE")

	owner, err := AcquireOwnerLock(realPath)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()
	expectedPath := owner.DatabasePath()
	if err := os.Remove(realPath); err != nil {
		t.Fatalf("remove owned database entry: %v", err)
	}
	if err := os.Symlink(alternatePath, realPath); err != nil {
		t.Fatalf("retarget owned database entry: %v", err)
	}

	db, err := OpenExisting(ctx, expectedPath)
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenExisting() database != nil, want retarget rejection")
	}
	if err == nil {
		t.Fatal("OpenExisting() error = nil, want retarget rejection")
	}
	if !strings.Contains(err.Error(), "differs from owned database path") {
		t.Fatalf("OpenExisting() error = %v, want owned-path mismatch", err)
	}

	readonly, err := OpenReadOnly(ctx, alternatePath)
	if err != nil {
		t.Fatalf("open alternate read-only: %v", err)
	}
	defer readonly.Close()
	var journalMode string
	if err := readonly.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read alternate journal mode: %v", err)
	}
	if strings.ToLower(journalMode) != "delete" {
		t.Fatalf("alternate journal mode = %q, want delete (unconfigured)", journalMode)
	}
}

func TestOpenExistingRejectsInvalidExpectedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uncleanAbsolutePath := filepath.Join(root, "child") + string(filepath.Separator) + ".." + string(filepath.Separator) + "database.db"
	for _, path := range []string{"", "relative.db", uncleanAbsolutePath} {
		db, err := OpenExisting(context.Background(), path)
		if db != nil {
			_ = db.Close()
			t.Fatalf("OpenExisting(%q) database != nil, want nil", path)
		}
		if err == nil {
			t.Fatalf("OpenExisting(%q) error = nil, want invalid path rejection", path)
		}
	}
}

func createSQLiteWithJournalMode(t *testing.T, ctx context.Context, path, mode string) {
	t.Helper()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("create SQLite database %q: %v", filepath.Base(path), err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE marker (id INTEGER PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatalf("create marker table: %v", err)
	}
	var actualMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = `+mode).Scan(&actualMode); err != nil {
		_ = db.Close()
		t.Fatalf("set journal mode: %v", err)
	}
	if !strings.EqualFold(actualMode, mode) {
		_ = db.Close()
		t.Fatalf("journal mode = %q, want %q", actualMode, mode)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close SQLite database: %v", err)
	}
}
