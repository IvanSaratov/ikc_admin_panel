package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func configureCommandDatabase(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "commands.db")
	t.Setenv("MINTRUD_ADMIN_ENV", "dev")
	t.Setenv("MINTRUD_ADMIN_DB", dbPath)
	return dbPath
}

func TestRunCommandMigrateThenStatus(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	var migrateOut bytes.Buffer
	if err := runCommand(ctx, []string{"db", "migrate"}, &migrateOut, zap.NewNop()); err != nil {
		t.Fatalf("db migrate: %v", err)
	}
	if !strings.Contains(migrateOut.String(), "current=1 target=1 pending=0") {
		t.Fatalf("migrate output = %q", migrateOut.String())
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database stat: %v", err)
	}

	var statusOut bytes.Buffer
	if err := runCommand(ctx, []string{"db", "status"}, &statusOut, zap.NewNop()); err != nil {
		t.Fatalf("db status: %v", err)
	}
	if !strings.Contains(statusOut.String(), "current=1 target=1 pending=0") {
		t.Fatalf("status output = %q", statusOut.String())
	}
}

func TestRunCommandStatusDoesNotCreateMissingDatabase(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	var out bytes.Buffer
	if err := runCommand(context.Background(), []string{"db", "status"}, &out, zap.NewNop()); err != nil {
		t.Fatalf("db status: %v", err)
	}
	if !strings.Contains(out.String(), "current=0 target=1 pending=1") {
		t.Fatalf("status output = %q", out.String())
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database stat error = %v, want not-exist", err)
	}
}

func TestRunCommandStatusDoesNotModifyEmptyDatabaseFile(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create empty database file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close empty database file: %v", err)
	}

	var out bytes.Buffer
	if err := runCommand(context.Background(), []string{"db", "status"}, &out, nil); err != nil {
		t.Fatalf("db status: %v", err)
	}
	if !strings.Contains(out.String(), "current=0 target=1 pending=1") {
		t.Fatalf("status output = %q", out.String())
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat empty database file: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("database size = %d, want unchanged empty file", info.Size())
	}
}

func TestRunCommandVerifyAndBackup(t *testing.T) {
	configureCommandDatabase(t)
	ctx := context.Background()
	if err := runCommand(ctx, []string{"db", "migrate"}, &bytes.Buffer{}, zap.NewNop()); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	var verifyOut bytes.Buffer
	if err := runCommand(ctx, []string{"db", "verify"}, &verifyOut, zap.NewNop()); err != nil {
		t.Fatalf("db verify: %v", err)
	}
	if !strings.Contains(verifyOut.String(), "verification=ok") {
		t.Fatalf("verify output = %q", verifyOut.String())
	}

	var backupOut bytes.Buffer
	if err := runCommand(ctx, []string{"db", "backup"}, &backupOut, zap.NewNop()); err != nil {
		t.Fatalf("db backup: %v", err)
	}
	line := strings.TrimSpace(backupOut.String())
	if !strings.HasPrefix(line, "backup=") {
		t.Fatalf("backup output = %q", line)
	}
	backupPath := strings.TrimPrefix(line, "backup=")
	if !strings.Contains(filepath.Base(backupPath), ".manual.") {
		t.Fatalf("manual backup path has unexpected name")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup stat: %v", err)
	}
}

func TestRunCommandVerifyRejectsDatabaseWithPendingSchema(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open pending database: %v", err)
	}
	migrator, err := storage.NewEmbeddedMigrator(db)
	if err != nil {
		t.Fatalf("new embedded migrator: %v", err)
	}
	if _, err := migrator.Status(ctx); err != nil {
		t.Fatalf("initialize migration history: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		"PRAGMA application_id = 0x494B4341; CREATE TABLE pending_probe (id INTEGER PRIMARY KEY);",
	); err != nil {
		t.Fatalf("mark pending database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close pending database: %v", err)
	}
	err = runCommand(ctx, []string{"db", "verify"}, &bytes.Buffer{}, zap.NewNop())
	if !errors.Is(err, storage.ErrSchemaNotReady) {
		t.Fatalf("db verify error = %v, want ErrSchemaNotReady", err)
	}
}

func TestRunCommandMutatingDatabaseCommandsRespectOwnerLock(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()

	for _, action := range []string{"migrate", "verify", "backup"} {
		err := runCommand(context.Background(), []string{"db", action}, &bytes.Buffer{}, zap.NewNop())
		if !errors.Is(err, storage.ErrDatabaseInUse) {
			t.Fatalf("db %s error = %v, want ErrDatabaseInUse", action, err)
		}
	}
}

func TestRunCommandStatusIsAllowedWhileOwnerLockIsHeld(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	if err := runCommand(ctx, []string{"db", "migrate"}, &bytes.Buffer{}, zap.NewNop()); err != nil {
		t.Fatalf("db migrate: %v", err)
	}
	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()
	database, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open service database: %v", err)
	}
	defer database.Close()

	var out bytes.Buffer
	if err := runCommand(ctx, []string{"db", "status"}, &out, zap.NewNop()); err != nil {
		t.Fatalf("db status with owner lock: %v", err)
	}
	if !strings.Contains(out.String(), "current=1 target=1 pending=0") {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestMigrationLoggingIncludesVersionsNamesAndDuration(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	logAppliedMigrations(logger, []storage.AppliedMigration{{
		MigrationInfo: storage.MigrationInfo{Version: 2, Name: "002_upgrade.sql"},
		Duration:      125 * time.Millisecond,
	}})
	failure := &storage.MigrationFailure{
		From:   1,
		Target: 3,
		To:     2,
		Failed: storage.AppliedMigration{
			MigrationInfo: storage.MigrationInfo{Version: 3, Name: "003_broken.sql"},
		},
		Err: errors.New("synthetic migration failure"),
	}
	logMigrationFailure(logger, failure)
	logDatabasePreparationFailure(
		logger,
		"synthetic-database-path",
		storage.PreparationResult{BackupPath: "synthetic-recovery-backup.db"},
		250*time.Millisecond,
		failure,
	)

	applied := observed.FilterMessage("database migration applied").All()
	if len(applied) != 1 {
		t.Fatalf("applied log count = %d, want 1", len(applied))
	}
	appliedFields := applied[0].ContextMap()
	for _, field := range []string{"version", "migration", "duration"} {
		if _, ok := appliedFields[field]; !ok {
			t.Fatalf("applied log missing field %q: %v", field, appliedFields)
		}
	}
	failed := observed.FilterMessage("database migration failed").All()
	if len(failed) != 1 {
		t.Fatalf("failure log count = %d, want 1", len(failed))
	}
	failedFields := failed[0].ContextMap()
	for _, field := range []string{
		"schema_from", "schema_target", "schema_current",
		"failed_version", "failed_migration", "error",
	} {
		if _, ok := failedFields[field]; !ok {
			t.Fatalf("failure log missing field %q: %v", field, failedFields)
		}
	}
	preparationFailed := observed.FilterMessage("database preparation failed").All()
	if len(preparationFailed) != 1 {
		t.Fatalf("preparation failure log count = %d, want 1", len(preparationFailed))
	}
	preparationFields := preparationFailed[0].ContextMap()
	for _, field := range []string{
		"database_path", "schema_current", "schema_target",
		"pending_migrations", "backup_path", "preparation_duration", "error",
	} {
		if _, ok := preparationFields[field]; !ok {
			t.Fatalf("preparation failure log missing field %q: %v", field, preparationFields)
		}
	}
}

func TestRunCommandRejectsUnknownCommand(t *testing.T) {
	configureCommandDatabase(t)
	err := runCommand(context.Background(), []string{"unknown"}, &bytes.Buffer{}, zap.NewNop())
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("run error = %v, want ErrUsage", err)
	}
}

func TestRunCommandRejectsUnknownDatabaseCommand(t *testing.T) {
	configureCommandDatabase(t)
	err := runCommand(context.Background(), []string{"db", "unknown"}, &bytes.Buffer{}, nil)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("run error = %v, want ErrUsage", err)
	}
}

func TestRunCommandRejectsUnknownCommandBeforeLoadingRuntimeConfig(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_ENV", "prod")
	t.Setenv("MINTRUD_ADMIN_DB", filepath.Join("relative", "customer.db"))

	for _, args := range [][]string{{"unknown"}, {"db", "unknown"}} {
		err := runCommand(context.Background(), args, &bytes.Buffer{}, nil)
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("runCommand(%q) error = %v, want ErrUsage", args, err)
		}
	}
}
