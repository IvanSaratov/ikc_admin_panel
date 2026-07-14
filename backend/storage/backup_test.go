package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupManagerCreatesVerifiedSnapshot(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	manager.now = func() time.Time {
		return time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	}

	backupPath, err := manager.Create(ctx, db, 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	backup, err := OpenReadOnly(ctx, backupPath)
	if err != nil {
		t.Fatalf("OpenReadOnly(backup) error = %v", err)
	}
	defer backup.Close()

	var note string
	if err := backup.QueryRowContext(ctx, `SELECT body FROM notes`).Scan(&note); err != nil {
		t.Fatalf("read backup notes row: %v", err)
	}
	if note != "preserved" {
		t.Fatalf("backup note = %q, want preserved", note)
	}

	var applicationID int64
	if err := backup.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatalf("read backup application ID: %v", err)
	}
	if applicationID != SQLiteApplicationID {
		t.Fatalf("backup application ID = %#x, want %#x", applicationID, SQLiteApplicationID)
	}
}

func TestBackupManagerRetainsNewestAutomaticSnapshots(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	baseTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	currentTime := baseTime
	manager.now = func() time.Time { return currentTime }

	paths := make([]string, 0, 4)
	for version := int64(1); version <= 4; version++ {
		currentTime = baseTime.Add(time.Duration(version-1) * time.Minute)
		path, err := manager.Create(ctx, db, version)
		if err != nil {
			t.Fatalf("Create(version %d) error = %v", version, err)
		}
		paths = append(paths, path)
	}

	entries, err := os.ReadDir(manager.backupDir)
	if err != nil {
		t.Fatalf("ReadDir(backups) error = %v", err)
	}
	if len(entries) != defaultBackupRetention {
		t.Fatalf("backup directory entries = %d, want %d", len(entries), defaultBackupRetention)
	}
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest backup os.Stat() error = %v, want not exist", err)
	}
	for _, path := range paths[1:] {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("newest backup %q os.Stat() error = %v", path, err)
		}
	}
}

func TestBackupManagerKeepsManualSnapshotsDuringAutomaticRetention(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	baseTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	currentTime := baseTime
	manager.now = func() time.Time { return currentTime }

	manualPath, err := manager.CreateManual(ctx, db, 1)
	if err != nil {
		t.Fatalf("CreateManual() error = %v", err)
	}
	if !strings.Contains(filepath.Base(manualPath), ".manual.") {
		t.Fatalf("manual backup filename = %q, want .manual. marker", filepath.Base(manualPath))
	}

	for version := int64(1); version <= 4; version++ {
		currentTime = baseTime.Add(time.Duration(version) * time.Minute)
		if _, err := manager.Create(ctx, db, version); err != nil {
			t.Fatalf("Create(version %d) error = %v", version, err)
		}
	}

	if _, err := os.Stat(manualPath); err != nil {
		t.Fatalf("manual backup os.Stat() error = %v", err)
	}
	entries, err := os.ReadDir(manager.backupDir)
	if err != nil {
		t.Fatalf("ReadDir(backups) error = %v", err)
	}
	wantEntries := defaultBackupRetention + 1
	if len(entries) != wantEntries {
		t.Fatalf("backup directory entries = %d, want %d", len(entries), wantEntries)
	}
}

func newBackupTestDatabase(t *testing.T) (context.Context, string, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(source) error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `CREATE TABLE notes (body TEXT NOT NULL)`); err != nil {
		t.Fatalf("create notes table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO notes (body) VALUES ('preserved')`); err != nil {
		t.Fatalf("insert notes fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA application_id = 0x494B4341`); err != nil {
		t.Fatalf("set SQLite application ID: %v", err)
	}

	return ctx, path, db
}
