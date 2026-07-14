package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

	backupPath, err := manager.Create(ctx, db, 1)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.Contains(filepath.Base(backupPath), "source.db.pre-migration.20260714T123000.000000000Z.s000001.v000001.db") {
		t.Fatalf("backup filename = %q, want owned automatic filename grammar", filepath.Base(backupPath))
	}
	assertVerifiedBackup(t, ctx, backupPath, 1)
}

func TestBackupManagerReservesDistinctDestinationsWithoutOverwriting(t *testing.T) {
	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	fixedTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	manager.now = func() time.Time { return fixedTime }

	if err := os.MkdirAll(manager.backupDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(backup directory) error = %v", err)
	}
	preexistingPath := filepath.Join(manager.backupDir, "source.db.pre-migration.20260714T123000.000000000Z.s000001.v000001.db")
	preexistingContents := []byte("administrator-owned collision")
	if err := os.WriteFile(preexistingPath, preexistingContents, 0o600); err != nil {
		t.Fatalf("WriteFile(preexisting) error = %v", err)
	}

	type result struct {
		path string
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			path, err := manager.Create(ctx, db, 1)
			results <- result{path: path, err: err}
		}()
	}
	close(start)

	paths := make(map[string]struct{}, 2)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent Create() error = %v", result.err)
		}
		if _, duplicate := paths[result.path]; duplicate {
			t.Fatalf("concurrent Create() returned duplicate path %q", result.path)
		}
		paths[result.path] = struct{}{}
		assertVerifiedBackup(t, ctx, result.path, 1)
	}

	gotContents, err := os.ReadFile(preexistingPath)
	if err != nil {
		t.Fatalf("ReadFile(preexisting) error = %v", err)
	}
	if string(gotContents) != string(preexistingContents) {
		t.Fatalf("preexisting contents = %q, want %q", gotContents, preexistingContents)
	}
}

func TestBackupManagerRetentionOnlyPrunesOwnedAutomaticSnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx, sourcePath, db := newBackupTestDatabaseAt(t, dir, "foo.db")
	manager := NewBackupManager(sourcePath)
	baseTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	currentTime := baseTime
	manager.now = func() time.Time { return currentTime }

	if err := os.MkdirAll(manager.backupDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(backup directory) error = %v", err)
	}
	adminPath := filepath.Join(manager.backupDir, "foo.db.pre-migration.administrator-copy.db")
	if err := os.WriteFile(adminPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile(admin file) error = %v", err)
	}
	manualPath, err := manager.CreateManual(ctx, db, 1)
	if err != nil {
		t.Fatalf("CreateManual() error = %v", err)
	}

	automaticPaths := make([]string, 0, 4)
	for attempt := 1; attempt <= 4; attempt++ {
		currentTime = baseTime.Add(time.Duration(attempt) * time.Minute)
		path, err := manager.Create(ctx, db, 1)
		if err != nil {
			t.Fatalf("Create(attempt %d) error = %v", attempt, err)
		}
		automaticPaths = append(automaticPaths, path)
	}

	ctxOther, otherPath, otherDB := newBackupTestDatabaseAt(t, dir, "foo.sqlite")
	otherManager := NewBackupManager(otherPath)
	otherManager.now = func() time.Time { return baseTime }
	otherBackup, err := otherManager.Create(ctxOther, otherDB, 1)
	if err != nil {
		t.Fatalf("other Create() error = %v", err)
	}
	if manager.backupDir == otherManager.backupDir {
		t.Fatalf("backup directories collide at %q", manager.backupDir)
	}

	for _, path := range []string{adminPath, manualPath, otherBackup} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved path %q os.Stat() error = %v", path, err)
		}
	}
	if _, err := os.Stat(automaticPaths[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest automatic os.Stat() error = %v, want not exist", err)
	}
	for _, path := range automaticPaths[1:] {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained automatic %q os.Stat() error = %v", path, err)
		}
	}
	entries, err := os.ReadDir(manager.backupDir)
	if err != nil {
		t.Fatalf("ReadDir(backups) error = %v", err)
	}
	if len(entries) != defaultBackupRetention+2 {
		t.Fatalf("primary backup entries = %d, want 3 automatic + manual + admin", len(entries))
	}
}

func TestBackupManagerPinsNewSnapshotAcrossClockRollbackAndIdenticalTimes(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	baseTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	times := []time.Time{
		baseTime.Add(time.Hour),
		baseTime.Add(time.Hour),
		baseTime.Add(-time.Hour),
		baseTime,
	}

	paths := make([]string, 0, len(times))
	for _, backupTime := range times {
		currentTime := backupTime
		manager.now = func() time.Time { return currentTime }
		path, err := manager.Create(ctx, db, 1)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", backupTime, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("newly returned path %q os.Stat() error = %v", path, err)
		}
		paths = append(paths, path)
	}

	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest sequence os.Stat() error = %v, want not exist", err)
	}
	for _, path := range paths[1:] {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("newest sequence %q os.Stat() error = %v", path, err)
		}
	}
}

func TestBackupManagerAppliesPrivateFilesystemModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are governed by ACLs")
	}

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	if err := os.MkdirAll(manager.backupDir, 0o777); err != nil {
		t.Fatalf("MkdirAll(backup directory) error = %v", err)
	}
	if err := os.Chmod(manager.backupDir, 0o777); err != nil {
		t.Fatalf("Chmod(backup directory) error = %v", err)
	}

	backupPath, err := manager.Create(ctx, db, 1)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	dirInfo, err := os.Stat(manager.backupDir)
	if err != nil {
		t.Fatalf("Stat(backup directory) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("backup directory mode = %#o, want 0700", got)
	}
	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("Stat(backup) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("backup file mode = %#o, want 0600", got)
	}
}

func TestBackupManagerValidatesSourcePathAndSchemaVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx, sourcePath, db := newBackupTestDatabaseAt(t, dir, "source.db")
	_, otherPath, _ := newBackupTestDatabaseAt(t, dir, "other.db")

	tests := []struct {
		name    string
		manager *BackupManager
		version int64
	}{
		{name: "negative version", manager: NewBackupManager(sourcePath), version: -1},
		{name: "version differs from Goose", manager: NewBackupManager(sourcePath), version: 2},
		{name: "database handle path differs", manager: NewBackupManager(otherPath), version: 1},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			path, err := test.manager.Create(ctx, db, test.version)
			if err == nil {
				t.Fatalf("Create() = %q, nil; want validation error", path)
			}
			if path != "" {
				t.Fatalf("Create() path = %q on validation failure, want empty", path)
			}
		})
	}
}

func TestBackupManagerRejectsMismatchedPathBeforeCreatingBackupDirectory(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(filepath.Join(filepath.Dir(sourcePath), "different.db"))

	path, err := manager.Create(ctx, db, 1)
	if err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("Create() = %q, %v; want mismatched source path error", path, err)
	}
	if path != "" {
		t.Fatalf("Create() path = %q, want empty", path)
	}
	if _, err := os.Stat(manager.backupDir); !os.IsNotExist(err) {
		t.Fatalf("backup directory os.Stat() error = %v, want not exist", err)
	}
}

func TestBackupManagerReturnsVerifiedPathWithPruneError(t *testing.T) {
	t.Parallel()

	ctx, sourcePath, db := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	baseTime := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	currentTime := baseTime
	manager.now = func() time.Time { return currentTime }
	for attempt := 1; attempt <= defaultBackupRetention; attempt++ {
		currentTime = baseTime.Add(time.Duration(attempt) * time.Minute)
		if _, err := manager.Create(ctx, db, 1); err != nil {
			t.Fatalf("Create(attempt %d) error = %v", attempt, err)
		}
	}

	removeErr := errors.New("injected retention removal failure")
	manager.remove = func(string) error { return removeErr }
	currentTime = baseTime.Add(10 * time.Minute)
	backupPath, err := manager.Create(ctx, db, 1)
	if !errors.Is(err, removeErr) {
		t.Fatalf("Create() error = %v, want wrapped remove error", err)
	}
	if backupPath == "" {
		t.Fatal("Create() path is empty after verified backup and prune failure")
	}
	if _, statErr := os.Stat(backupPath); statErr != nil {
		t.Fatalf("verified backup %q os.Stat() error = %v", backupPath, statErr)
	}
}

func TestBackupReservationCleanupOnlyRemovesReservedPathAndJoinsErrors(t *testing.T) {
	t.Parallel()

	_, sourcePath, _ := newBackupTestDatabase(t)
	manager := NewBackupManager(sourcePath)
	if err := manager.prepareBackupDirectory(); err != nil {
		t.Fatalf("prepareBackupDirectory() error = %v", err)
	}
	otherPath := filepath.Join(manager.backupDir, "administrator-file.db")
	if err := os.WriteFile(otherPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile(other) error = %v", err)
	}

	cleanupErr := errors.New("injected cleanup failure")
	var removedPaths []string
	manager.remove = func(path string) error {
		removedPaths = append(removedPaths, path)
		return cleanupErr
	}
	reservation, err := manager.reserveBackup("pre-migration", time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("reserveBackup() error = %v", err)
	}
	operationErr := errors.New("backup operation failed")
	err = reservation.cleanup(operationErr)
	if !errors.Is(err, operationErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup() error = %v, want operation and cleanup errors", err)
	}
	if len(removedPaths) != 1 || removedPaths[0] != reservation.path {
		t.Fatalf("removed paths = %q, want only reserved path %q", removedPaths, reservation.path)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("other path os.Stat() error = %v", err)
	}
}

func newBackupTestDatabase(t *testing.T) (context.Context, string, *sql.DB) {
	t.Helper()
	return newBackupTestDatabaseAt(t, t.TempDir(), "source.db")
}

func newBackupTestDatabaseAt(t *testing.T, dir, name string) (context.Context, string, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(dir, name)
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", name, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := NewMigrator(db, migrationFS(false))
	if err != nil {
		t.Fatalf("NewMigrator(%s) error = %v", name, err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("Up(%s) error = %v", name, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE notes (body TEXT NOT NULL)`); err != nil {
		t.Fatalf("create notes table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO notes (body) VALUES ('preserved')`); err != nil {
		t.Fatalf("insert notes fixture: %v", err)
	}

	return ctx, path, db
}

func assertVerifiedBackup(t *testing.T, ctx context.Context, path string, wantVersion int64) {
	t.Helper()

	backup, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly(%q) error = %v", path, err)
	}
	defer backup.Close()
	if err := QuickCheck(ctx, backup); err != nil {
		t.Fatalf("QuickCheck(%q) error = %v", path, err)
	}
	identity, err := InspectDatabaseIdentity(ctx, backup)
	if err != nil {
		t.Fatalf("InspectDatabaseIdentity(%q) error = %v", path, err)
	}
	if identity.ApplicationID != SQLiteApplicationID || !identity.HasMigrationHistory || identity.ApplicationTables == 0 {
		t.Fatalf("backup identity = %+v, want recognized application database", identity)
	}
	var version int64
	if err := backup.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied = 1
	`).Scan(&version); err != nil {
		t.Fatalf("read backup Goose version: %v", err)
	}
	if version != wantVersion {
		t.Fatalf("backup Goose version = %d, want %d", version, wantVersion)
	}
	var note string
	if err := backup.QueryRowContext(ctx, `SELECT body FROM notes`).Scan(&note); err != nil {
		t.Fatalf("read backup notes row: %v", err)
	}
	if note != "preserved" {
		t.Fatalf("backup note = %q, want preserved", note)
	}
}
