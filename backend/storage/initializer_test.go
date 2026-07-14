package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
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

func TestInitializerReportsConfigurationFailureAfterIdentityAndStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "readonly-current.db")
	raw, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	migrator := newInitializerTestMigrator(t, raw, false)
	if _, err := migrator.Up(ctx); err != nil {
		_ = raw.Close()
		t.Fatalf("migrate current fixture: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close current fixture: %v", err)
	}

	readonly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("open current fixture read-only: %v", err)
	}
	defer readonly.Close()
	readonlyMigrator := newInitializerTestMigrator(t, readonly, false)
	initializer := NewInitializer(readonly, readonlyMigrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		t.Fatal("backup called for current schema")
		return "", nil
	}))
	for attempt := 1; attempt <= 2; attempt++ {
		result, err := initializer.Prepare(ctx)
		if err == nil || !strings.Contains(err.Error(), "configure SQLite database for preparation") {
			t.Fatalf("Prepare(attempt %d) error = %v, want configuration failure", attempt, err)
		}
		assertMigrationStatus(t, result.Before, 1, 1, nil)
		assertMigrationStatus(t, result.After, 1, 1, nil)
		if result.Identity.ApplicationID != SQLiteApplicationID || !result.Identity.HasMigrationHistory {
			t.Fatalf("Prepare(attempt %d) Identity = %+v, want recognized current database", attempt, result.Identity)
		}
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
	assertInitializerBackup(t, ctx, result.BackupPath, "preserved")

	var name, status string
	if err := db.QueryRowContext(ctx, `SELECT name, status FROM items`).Scan(&name, &status); err != nil {
		t.Fatalf("read upgraded item: %v", err)
	}
	if name != "preserved" || status != "active" {
		t.Fatalf("upgraded item = (%q, %q), want (preserved, active)", name, status)
	}
}

func TestInitializerReturnsRecoveryBackupOnFailedUpgrade(t *testing.T) {
	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO items (name) VALUES ('protected')`); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	brokenFS := migrationFS(false)
	brokenFS["002_broken.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up
INSERT INTO missing_table(value) VALUES ('broken');
`)}
	broken := newInitializerTestMigratorFS(t, db, brokenFS)
	result, err := NewInitializer(db, broken, NewBackupManager(dbPath)).Prepare(ctx)
	var failure *MigrationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Prepare() error = %v, want *MigrationFailure", err)
	}
	if result.BackupPath == "" {
		t.Fatal("Prepare() BackupPath is empty after failed upgrade")
	}
	assertInitializerBackup(t, ctx, result.BackupPath, "protected")
	assertMigrationStatus(t, result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_broken.sql"}})
	assertInitializerSchemaVersion(t, ctx, db, 1)
	var protectedName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM items WHERE id = 1`).Scan(&protectedName); err != nil {
		t.Fatalf("read item after failed upgrade: %v", err)
	}
	if protectedName != "protected" {
		t.Fatalf("item after failed upgrade = %q, want protected", protectedName)
	}
}

func TestPreMigrationBackupRestoresPreviousSchemaAndData(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "restore.db")
	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("v1 Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO items (name) VALUES ('restore-me')`); err != nil {
		_ = db.Close()
		t.Fatalf("seed item: %v", err)
	}
	v2 := newInitializerTestMigrator(t, db, true)
	prepared, err := NewInitializer(db, v2, NewBackupManager(dbPath)).Prepare(ctx)
	if err != nil {
		_ = db.Close()
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close upgraded database: %v", err)
	}

	backupBytes, err := os.ReadFile(prepared.BackupPath)
	if err != nil {
		t.Fatalf("read recovery backup: %v", err)
	}
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove upgraded database file %s: %v", filepath.Base(path), err)
		}
	}
	if err := os.WriteFile(dbPath, backupBytes, 0o600); err != nil {
		t.Fatalf("restore recovery backup: %v", err)
	}

	restored, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	restoredMigrator := newInitializerTestMigrator(t, restored, false)
	status, err := restoredMigrator.Status(ctx)
	if err != nil {
		t.Fatalf("restored Status() error = %v", err)
	}
	assertMigrationStatus(t, status, 1, 1, nil)
	var name string
	if err := restored.QueryRowContext(ctx, `SELECT name FROM items WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("read restored item: %v", err)
	}
	if name != "restore-me" {
		t.Fatalf("restored item name = %q, want restore-me", name)
	}
	var upgradedColumns int
	if err := restored.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'status'`,
	).Scan(&upgradedColumns); err != nil {
		t.Fatalf("inspect restored items schema: %v", err)
	}
	if upgradedColumns != 0 {
		t.Fatalf("restored status column count = %d, want 0", upgradedColumns)
	}
	if err := QuickCheck(ctx, restored); err != nil {
		t.Fatalf("QuickCheck(restored) error = %v", err)
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
	assertInitializerBackup(t, ctx, result.BackupPath, "preserved")
	assertMigrationStatus(t, result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_item_status.sql"}})
	assertInitializerSchemaVersion(t, ctx, db, 1)
}

func TestInitializerRetriesExactGooseBootstrapAfterFailedBaseline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	failing := newInitializerTestMigratorFS(t, db, initializerFailedBaselineFS())
	backups := backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		t.Fatal("backup called for version-zero preparation")
		return "", nil
	})

	first, err := NewInitializer(db, failing, backups).Prepare(ctx)
	var failure *MigrationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("first Prepare() error = %v, want *MigrationFailure", err)
	}
	assertMigrationResult(t, first.Migration, 0, 0, nil)
	assertOnlyGooseBootstrapObjects(t, ctx, db)
	assertNoTable(t, ctx, db, "failed_items")

	valid := newInitializerTestMigrator(t, db, false)
	second, err := NewInitializer(db, valid, backups).Prepare(ctx)
	if err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	assertMigrationStatus(t, second.Before, 0, 1, []MigrationInfo{{Version: 1, Name: "001_items.sql"}})
	assertMigrationStatus(t, second.After, 1, 1, nil)
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&rows); err != nil {
		t.Fatalf("count retried items: %v", err)
	}
	if rows != 0 {
		t.Fatalf("retried item rows = %d, want 0", rows)
	}
	assertNoTable(t, ctx, db, "failed_items")
}

func TestInitializerRejectsForgedGooseBootstrapShells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup []string
	}{
		{
			name: "wrong column shape",
			setup: []string{
				`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id TEXT NOT NULL, is_applied INTEGER NOT NULL, tstamp TIMESTAMP DEFAULT (datetime('now')))`,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
			},
		},
		{
			name: "extra user object",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
				`CREATE INDEX forged_goose_index ON goose_db_version(version_id)`,
			},
		},
		{
			name: "unapplied version zero",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 0)`,
			},
		},
		{
			name:  "missing history row",
			setup: []string{gooseBootstrapTableSQL},
		},
		{
			name: "nonzero version",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, 1)`,
			},
		},
		{
			name: "application ID already marked",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
				`PRAGMA application_id = 0x494B4341`,
			},
		},
		{
			name: "extra history row",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
			},
		},
		{
			name: "application table",
			setup: []string{
				gooseBootstrapTableSQL,
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
				`CREATE TABLE forged_items (id INTEGER PRIMARY KEY)`,
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			_, db := openInitializerTestDatabase(t, ctx)
			for _, statement := range test.setup {
				if _, err := db.ExecContext(ctx, statement); err != nil {
					t.Fatalf("setup %q: %v", statement, err)
				}
			}
			migrator := newInitializerTestMigrator(t, db, false)
			_, err := NewInitializer(db, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
				t.Fatal("backup called for forged bootstrap shell")
				return "", nil
			})).Prepare(ctx)
			if !errors.Is(err, ErrUnrecognizedDatabase) && !errors.Is(err, ErrSchemaNotReady) {
				t.Fatalf("Prepare() error = %v, want unrecognized or not-ready database", err)
			}
			assertNoTable(t, ctx, db, "items")
		})
	}
}

func TestInitializerChecksForeignKeysOnEveryCurrentRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigrator(t, db, false)
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE parents (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create parents: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents(id))`); err != nil {
		t.Fatalf("create children: %v", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO children (id, parent_id) VALUES (1, 999)`); err != nil {
		t.Fatalf("insert violating child: %v", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	initializer := NewInitializer(db, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		t.Fatal("backup called without pending migrations")
		return "", nil
	}))

	for attempt := 1; attempt <= 2; attempt++ {
		result, err := initializer.Prepare(ctx)
		if err == nil || !strings.Contains(err.Error(), "foreign key") {
			t.Fatalf("Prepare(attempt %d) error = %v, want foreign-key violation", attempt, err)
		}
		assertMigrationResult(t, result.Migration, 1, 1, nil)
	}
}

func TestInitializerRefreshesResultAfterPartialMigrationFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigratorFS(t, db, initializerPartialFailureFS())

	result, err := NewInitializer(db, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		t.Fatal("backup called for version-zero preparation")
		return "", nil
	})).Prepare(ctx)
	var failure *MigrationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Prepare() error = %v, want *MigrationFailure", err)
	}
	if failure.Err == nil || !errors.Is(err, failure.Err) {
		t.Fatalf("Prepare() error = %v, want root cause %v", err, failure.Err)
	}
	assertMigrationResult(t, result.Migration, 0, 1, []int64{1})
	assertMigrationStatus(t, result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_duplicate.sql"}})
	if result.Identity.ApplicationID != SQLiteApplicationID || result.Identity.ApplicationTables == 0 || !result.Identity.HasMigrationHistory {
		t.Fatalf("Prepare() Identity = %+v, want committed recognized v1 database", result.Identity)
	}
}

func TestInitializerRefreshesPartialResultAfterContextCancellation(t *testing.T) {
	ctx := context.Background()
	path, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigratorFS(t, db, initializerCanceledPartialFailureFS())
	observer, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(observer) error = %v", err)
	}
	t.Cleanup(func() { _ = observer.Close() })

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		result PreparationResult
		err    error
	}
	outcomes := make(chan outcome, 1)
	go func() {
		result, err := NewInitializer(db, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
			t.Error("backup called for version-zero preparation")
			return "", nil
		})).Prepare(cancelCtx)
		outcomes <- outcome{result: result, err: err}
	}()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var version int64
		err := observer.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version_id), 0)
			FROM goose_db_version
			WHERE is_applied = 1
		`).Scan(&version)
		if err == nil && version == 1 {
			cancel()
			break
		}
		select {
		case completed := <-outcomes:
			t.Fatalf("Prepare() completed before cancellation: result = %+v, error = %v", completed.result, completed.err)
		case <-deadline.C:
			cancel()
			t.Fatal("timed out waiting for migration 1 to commit")
		case <-poll.C:
		}
	}

	select {
	case completed := <-outcomes:
		var failure *MigrationFailure
		if !errors.As(completed.err, &failure) {
			t.Fatalf("Prepare() error = %v, want *MigrationFailure", completed.err)
		}
		if !errors.Is(completed.err, context.Canceled) && !errors.Is(completed.err, context.DeadlineExceeded) {
			t.Fatalf("Prepare() error = %v, want cancellation root cause", completed.err)
		}
		assertMigrationResult(t, completed.result.Migration, 0, 1, []int64{1})
		assertMigrationStatus(t, completed.result.After, 1, 2, []MigrationInfo{{Version: 2, Name: "002_slow.sql"}})
		if completed.result.Identity.ApplicationID != SQLiteApplicationID {
			t.Fatalf("Prepare() Identity = %+v, want detached refresh of committed v1", completed.result.Identity)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for canceled preparation")
	}
}

func TestInitializerCurrentDatabaseReportsNoOpMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigrator(t, db, false)
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	result, err := NewInitializer(db, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) {
		t.Fatal("backup called without pending migrations")
		return "", nil
	})).Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	assertMigrationResult(t, result.Migration, 1, 1, nil)
}

func TestInitializerRejectsInvalidDependenciesWithoutPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	migrator := newInitializerTestMigrator(t, db, true)
	var typedNil *BackupManager
	tests := []struct {
		name        string
		initializer *Initializer
	}{
		{name: "nil receiver", initializer: nil},
		{name: "nil database", initializer: NewInitializer(nil, migrator, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) { return "", nil }))},
		{name: "nil migrator", initializer: NewInitializer(db, nil, backupCreatorFunc(func(context.Context, *sql.DB, int64) (string, error) { return "", nil }))},
		{name: "nil backups", initializer: NewInitializer(db, migrator, nil)},
		{name: "typed nil backups", initializer: NewInitializer(db, migrator, typedNil)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("Prepare() panic = %v, want validation error", recovered)
				}
			}()
			if _, err := test.initializer.Prepare(ctx); err == nil {
				t.Fatal("Prepare() error = nil, want invalid dependency error")
			}
		})
	}
}

func TestNewEmbeddedInitializerValidatesDatabasePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	tests := []struct {
		name string
		db   *sql.DB
		path string
	}{
		{name: "nil database", db: nil, path: dbPath},
		{name: "empty path", db: db, path: ""},
		{name: "relative path", db: db, path: "initializer.db"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewEmbeddedInitializer(test.db, test.path); err == nil {
				t.Fatal("NewEmbeddedInitializer() error = nil, want validation error")
			}
		})
	}
}

func TestInitializerRejectsMismatchedBackupPathBeforeFreshMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath, db := openInitializerTestDatabase(t, ctx)
	otherPath := filepath.Join(filepath.Dir(dbPath), "other.db")
	tests := []struct {
		name string
		new  func() (*Initializer, error)
	}{
		{
			name: "backup manager",
			new: func() (*Initializer, error) {
				return NewInitializer(db, newInitializerTestMigrator(t, db, true), NewBackupManager(otherPath)), nil
			},
		},
		{
			name: "embedded initializer",
			new: func() (*Initializer, error) {
				return NewEmbeddedInitializer(db, otherPath)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			initializer, err := test.new()
			if err != nil {
				t.Fatalf("construct initializer: %v", err)
			}
			if _, err := initializer.Prepare(ctx); err == nil || !strings.Contains(err.Error(), "differs") {
				t.Fatalf("Prepare() error = %v, want mismatched database path", err)
			}
			var objects int
			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM sqlite_schema
				WHERE substr(name, 1, 7) <> 'sqlite_'
			`).Scan(&objects); err != nil {
				t.Fatalf("count user objects: %v", err)
			}
			if objects != 0 {
				t.Fatalf("user objects after rejected path = %d, want 0", objects)
			}
		})
	}
}

func TestInitializerCanceledWaiterDoesNotEnterPreparation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, db := openInitializerTestDatabase(t, ctx)
	v1 := newInitializerTestMigrator(t, db, false)
	if _, err := v1.Up(ctx); err != nil {
		t.Fatalf("v1 Up() error = %v", err)
	}
	v2 := newInitializerTestMigrator(t, db, true)
	blocker := &blockingBackupCreator{
		entered:       make(chan struct{}),
		secondEntered: make(chan struct{}),
		release:       make(chan struct{}),
		err:           errors.New("stop first preparation after gate test"),
	}
	defer func() {
		select {
		case <-blocker.release:
		default:
			close(blocker.release)
		}
	}()
	initializer := NewInitializer(db, v2, blocker)
	firstDone := make(chan error, 1)
	go func() {
		_, err := initializer.Prepare(ctx)
		firstDone <- err
	}()
	select {
	case <-blocker.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first preparation did not enter backup")
	}

	waitCtx, cancel := context.WithCancel(ctx)
	waiterDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := initializer.Prepare(waitCtx)
		waiterDone <- err
	}()
	<-started
	enteredPreparation := false
	select {
	case <-blocker.secondEntered:
		enteredPreparation = true
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-waiterDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting Prepare() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiting Prepare() did not return promptly")
	}
	if calls := blocker.calls.Load(); calls != 1 {
		t.Fatalf("backup calls before release = %d, want 1", calls)
	}
	if enteredPreparation {
		t.Fatal("waiting Prepare() entered backup before cancellation")
	}
	close(blocker.release)
	if err := <-firstDone; !errors.Is(err, blocker.err) {
		t.Fatalf("first Prepare() error = %v, want blocker error", err)
	}
}

const gooseBootstrapTableSQL = `CREATE TABLE goose_db_version (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	version_id INTEGER NOT NULL,
	is_applied INTEGER NOT NULL,
	tstamp TIMESTAMP DEFAULT (datetime('now'))
)`

type blockingBackupCreator struct {
	entered       chan struct{}
	secondEntered chan struct{}
	release       chan struct{}
	err           error
	calls         atomic.Int32
}

func (b *blockingBackupCreator) Create(context.Context, *sql.DB, int64) (string, error) {
	if b.calls.Add(1) == 1 {
		close(b.entered)
		<-b.release
		return "", b.err
	}
	close(b.secondEntered)
	return "", errors.New("concurrent preparation entered backup")
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

	return newInitializerTestMigratorFS(t, db, migrationFS(includeSecond))
}

func newInitializerTestMigratorFS(t *testing.T, db *sql.DB, fsys fstest.MapFS) *Migrator {
	t.Helper()

	migrator, err := NewMigrator(db, fsys)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	return migrator
}

func initializerFailedBaselineFS() fstest.MapFS {
	return fstest.MapFS{
		"001_failed.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
PRAGMA application_id = 0x494B4341;
CREATE TABLE failed_items (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
THIS IS INVALID SQL;

-- +goose Down
DROP TABLE failed_items;
`)},
	}
}

func initializerPartialFailureFS() fstest.MapFS {
	return fstest.MapFS{
		"001_items.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
PRAGMA application_id = 0x494B4341;
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- +goose Down
DROP TABLE items;
`)},
		"002_duplicate.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE items (id INTEGER PRIMARY KEY);

-- +goose Down
`)},
	}
}

func initializerCanceledPartialFailureFS() fstest.MapFS {
	return fstest.MapFS{
		"001_items.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
PRAGMA application_id = 0x494B4341;
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- +goose Down
DROP TABLE items;
`)},
		"002_slow.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE slow AS
WITH RECURSIVE cnt(x) AS (
    VALUES(0)
    UNION ALL
    SELECT x+1 FROM cnt WHERE x < 1000000
)
SELECT sum(x) AS n FROM cnt;

-- +goose Down
DROP TABLE slow;
`)},
	}
}

func assertOnlyGooseBootstrapObjects(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT type, name
		FROM sqlite_schema
		WHERE substr(name, 1, 7) <> 'sqlite_'
		ORDER BY type, name
	`)
	if err != nil {
		t.Fatalf("query bootstrap objects: %v", err)
	}
	defer rows.Close()
	type object struct {
		typeName string
		name     string
	}
	var objects []object
	for rows.Next() {
		var got object
		if err := rows.Scan(&got.typeName, &got.name); err != nil {
			t.Fatalf("scan bootstrap object: %v", err)
		}
		objects = append(objects, got)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate bootstrap objects: %v", err)
	}
	if len(objects) != 1 || objects[0] != (object{typeName: "table", name: "goose_db_version"}) {
		t.Fatalf("bootstrap objects = %+v, want only goose_db_version table", objects)
	}
	var version, applied, rowsCount int64
	if err := db.QueryRowContext(ctx, `
		SELECT MIN(version_id), MIN(is_applied), COUNT(*)
		FROM goose_db_version
	`).Scan(&version, &applied, &rowsCount); err != nil {
		t.Fatalf("read bootstrap row: %v", err)
	}
	if version != 0 || applied != 1 || rowsCount != 1 {
		t.Fatalf("bootstrap row = version %d, applied %d, count %d; want 0, 1, 1", version, applied, rowsCount)
	}
}

func assertNoTable(t *testing.T, ctx context.Context, db *sql.DB, name string) {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'table' AND name = ?
	`, name).Scan(&count); err != nil {
		t.Fatalf("inspect table %q: %v", name, err)
	}
	if count != 0 {
		t.Fatalf("table %q count = %d, want 0", name, count)
	}
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

func assertInitializerBackup(t *testing.T, ctx context.Context, path, wantName string) {
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
	if name != wantName {
		t.Fatalf("backup item = %q, want %q", name, wantName)
	}
}
