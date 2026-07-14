package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitializerPreparesFreshDatabaseWithoutBackup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigrator(t, db, true)
	backups := NewBackupManager(dbPath)

	result, err := NewInitializer(db, migrator, backups).Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	assertMigrationStatus(t, result.Before, 0, 2, []MigrationInfo{
		{Version: 1, Name: "001_items.sql"},
		{Version: 2, Name: "002_item_status.sql"},
	})
	assertMigrationStatus(t, result.After, 2, 2, nil)
	assertMigrationResult(t, result.Migration, 0, 2, []int64{1, 2})
	if result.BackupPath != "" {
		t.Fatalf("Prepare() BackupPath = %q, want empty", result.BackupPath)
	}
	if result.Identity.Fresh || result.Identity.ApplicationID != SQLiteApplicationID {
		t.Fatalf("Prepare() Identity = %+v, want recognized migrated database", result.Identity)
	}
	if _, err := os.Stat(backups.backupDir); !os.IsNotExist(err) {
		t.Fatalf("backup directory os.Stat() error = %v, want not exist", err)
	}
}

func TestInitializerBacksUpPopulatedDatabaseBeforeUpgrade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO items (name) VALUES ('preserved')`); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	v2 := newInitializerTestMigrator(t, db, true)
	backups := NewBackupManager(dbPath)

	result, err := NewInitializer(db, v2, backups).Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	assertMigrationStatus(t, result.Before, 1, 2, []MigrationInfo{{Version: 2, Name: "002_item_status.sql"}})
	assertMigrationStatus(t, result.After, 2, 2, nil)
	if result.BackupPath == "" {
		t.Fatal("Prepare() BackupPath is empty, want verified pre-migration backup")
	}
	assertInitializerBackup(t, ctx, result.BackupPath)

	var name, status string
	if err := db.QueryRowContext(ctx, `SELECT name, status FROM items`).Scan(&name, &status); err != nil {
		t.Fatalf("read upgraded item: %v", err)
	}
	if name != "preserved" || status != "active" {
		t.Fatalf("upgraded item = (%q, %q), want (preserved, active)", name, status)
	}
}

func TestInitializerStopsBeforeMigrationWhenBackupFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	diskFull := errors.New("disk full")
	v2 := newInitializerTestMigrator(t, db, true)

	result, err := NewInitializer(db, v2, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		return "", diskFull
	})).Prepare(ctx)
	if !errors.Is(err, diskFull) {
		t.Fatalf("Prepare() error = %v, want wrapped disk-full error", err)
	}
	assertMigrationStatus(t, result.Before, 1, 2, []MigrationInfo{{Version: 2, Name: "002_item_status.sql"}})
	assertMigrationStatus(t, result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_item_status.sql"}})
	assertInitializerSchemaVersion(t, ctx, db, 1)
}

func TestInitializerRejectsUnmarkedLegacyDatabaseBeforeBackupOrGooseMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO items (name) VALUES ('legacy')`); err != nil {
		t.Fatalf("insert legacy item: %v", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA application_id = 0`); err != nil {
		t.Fatalf("clear application ID: %v", err)
	}
	backups := NewBackupManager(dbPath)

	_, err := NewInitializer(db, v1, backups).Prepare(ctx)
	if !errors.Is(err, ErrUnrecognizedDatabase) {
		t.Fatalf("Prepare() error = %v, want ErrUnrecognizedDatabase", err)
	}
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM items`).Scan(&name); err != nil {
		t.Fatalf("read legacy item: %v", err)
	}
	if name != "legacy" {
		t.Fatalf("legacy item = %q, want preserved", name)
	}
	if _, err := os.Stat(backups.backupDir); !os.IsNotExist(err) {
		t.Fatalf("backup directory os.Stat() error = %v, want not exist", err)
	}
}

func TestInitializerRejectsPopulatedUnversionedDatabaseWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	if _, err := db.ExecContext(ctx, `CREATE TABLE legacy_items (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO legacy_items (value) VALUES ('preserved')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	migrator := newInitializerTestMigrator(t, db, true)
	backups := NewBackupManager(dbPath)

	_, err := NewInitializer(db, migrator, backups).Prepare(ctx)
	if !errors.Is(err, ErrUnrecognizedDatabase) {
		t.Fatalf("Prepare() error = %v, want ErrUnrecognizedDatabase", err)
	}
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM legacy_items`).Scan(&value); err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if value != "preserved" {
		t.Fatalf("legacy value = %q, want preserved", value)
	}
	var historyTables int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'table' AND name = 'goose_db_version'
	`).Scan(&historyTables); err != nil {
		t.Fatalf("inspect migration history: %v", err)
	}
	if historyTables != 0 {
		t.Fatalf("goose_db_version table count = %d, want 0", historyTables)
	}
	if _, err := os.Stat(backups.backupDir); !os.IsNotExist(err) {
		t.Fatalf("backup directory os.Stat() error = %v, want not exist", err)
	}
}

func TestInitializerPreservesVerifiedBackupPathOnPruneFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO items (name) VALUES ('preserved')`); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	backups := NewBackupManager(dbPath)
	for attempt := 1; attempt <= defaultBackupRetention; attempt++ {
		if _, err := backups.Create(ctx, db, 1); err != nil {
			t.Fatalf("seed backup %d: %v", attempt, err)
		}
	}
	pruneFailure := errors.New("prune failed")
	backups.remove = func(string) error { return pruneFailure }
	v2 := newInitializerTestMigrator(t, db, true)

	result, err := NewInitializer(db, v2, backups).Prepare(ctx)
	if !errors.Is(err, pruneFailure) {
		t.Fatalf("Prepare() error = %v, want wrapped prune failure", err)
	}
	if result.BackupPath == "" {
		t.Fatal("Prepare() BackupPath is empty after verified backup and prune failure")
	}
	assertInitializerBackup(t, ctx, result.BackupPath)
	assertMigrationStatus(t, result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_item_status.sql"}})
	assertInitializerSchemaVersion(t, ctx, db, 1)
}

type backupCreatorFunc func(context.Context, *sql.DB, int64) (string, error)

func (f backupCreatorFunc) Create(ctx context.Context, db *sql.DB, version int64) (string, error) {
	return f(ctx, db, version)
}

func openInitializerTestDatabase(t *testing.T, ctx context.Context) (string, *sql.DB) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "initializer.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return path, db
}

func newInitializerTestMigrator(t *testing.T, db *sql.DB, includeSecond bool) *Migrator {
	t.Helper()

	migrator, err := NewMigrator(db, migrationFS(includeSecond))
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	return migrator
}

func assertInitializerSchemaVersion(t *testing.T, ctx context.Context, db *sql.DB, want int64) {
	t.Helper()

	version, err := readGooseSchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("readGooseSchemaVersion() error = %v", err)
	}
	if version != want {
		t.Fatalf("schema version = %d, want %d", version, want)
	}
}

func assertInitializerBackup(t *testing.T, ctx context.Context, path string) {
	t.Helper()

	backup, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly(backup) error = %v", err)
	}
	defer backup.Close()
	if err := QuickCheck(ctx, backup); err != nil {
		t.Fatalf("backup QuickCheck() error = %v", err)
	}
	assertInitializerSchemaVersion(t, ctx, backup, 1)
	var name string
	if err := backup.QueryRowContext(ctx, `SELECT name FROM items`).Scan(&name); err != nil {
		t.Fatalf("read backup item: %v", err)
	}
	if name != "preserved" {
		t.Fatalf("backup item = %q, want preserved", name)
	}
}
