package runner

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func configureCommandDatabase(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "commands.db")
}

func testRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Environment: "dev", LogLevel: "error"}
}

func runDatabaseTestCommand(ctx context.Context, action string, databasePath string, stdout io.Writer) error {
	return RunDatabase(ctx, DatabaseAction(action), DatabaseConfig{
		Runtime:      testRuntimeConfig(),
		DatabasePath: databasePath,
	}, stdout)
}

func TestRunDatabaseMigrateThenStatus(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	var migrateOut bytes.Buffer
	if err := runDatabaseTestCommand(ctx, "migrate", dbPath, &migrateOut); err != nil {
		t.Fatalf("db migrate: %v", err)
	}
	if !strings.Contains(migrateOut.String(), "current=1 target=1 pending=0") {
		t.Fatalf("migrate output = %q", migrateOut.String())
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database stat: %v", err)
	}
	assertCommandDatabaseJournalMode(t, ctx, dbPath, "wal")

	var statusOut bytes.Buffer
	if err := runDatabaseTestCommand(ctx, "status", dbPath, &statusOut); err != nil {
		t.Fatalf("db status: %v", err)
	}
	if !strings.Contains(statusOut.String(), "current=1 target=1 pending=0") {
		t.Fatalf("status output = %q", statusOut.String())
	}
}

func TestRunDatabaseMigrateRejectsForeignSQLiteWithoutMutation(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	createUnconfiguredCommandSQLite(t, ctx, dbPath)
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read foreign database before migrate: %v", err)
	}

	err = runDatabaseTestCommand(ctx, "migrate", dbPath, &bytes.Buffer{})
	if !errors.Is(err, storage.ErrUnrecognizedDatabase) {
		t.Fatalf("db migrate error = %v, want ErrUnrecognizedDatabase", err)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read foreign database after migrate: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("foreign database bytes changed during rejected migration")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, statErr := os.Stat(dbPath + suffix); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("SQLite artifact %q stat error = %v, want not exist", suffix, statErr)
		}
	}
	assertCommandDatabaseJournalMode(t, ctx, dbPath, "delete")

	readonly, err := storage.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatalf("open foreign database read-only: %v", err)
	}
	defer readonly.Close()
	var gooseTables int
	if err := readonly.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE name = 'goose_db_version'`).Scan(&gooseTables); err != nil {
		t.Fatalf("inspect foreign schema: %v", err)
	}
	if gooseTables != 0 {
		t.Fatalf("goose table count = %d, want no schema mutation", gooseTables)
	}
}

func TestRunDatabaseExistingMaintenanceRejectsTooNewBeforeConfiguration(t *testing.T) {
	for _, action := range []string{"verify", "backup"} {
		t.Run(action, func(t *testing.T) {
			dbPath := configureCommandDatabase(t)
			ctx := context.Background()
			createTooNewCommandSQLite(t, ctx, dbPath)
			before, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("read too-new database before command: %v", err)
			}

			err = runDatabaseTestCommand(ctx, action, dbPath, &bytes.Buffer{})
			if !errors.Is(err, storage.ErrSchemaTooNew) {
				t.Fatalf("db %s error = %v, want ErrSchemaTooNew", action, err)
			}
			after, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("read too-new database after command: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("too-new database bytes changed during rejected maintenance command")
			}
			for _, suffix := range []string{".lock", "-wal", "-shm"} {
				if _, statErr := os.Stat(dbPath + suffix); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("SQLite artifact %q stat error = %v, want not exist", suffix, statErr)
				}
			}
			assertCommandDatabaseJournalMode(t, ctx, dbPath, "delete")

			readonly, err := storage.OpenReadOnly(ctx, dbPath)
			if err != nil {
				t.Fatalf("open too-new database read-only: %v", err)
			}
			defer readonly.Close()
			var noteColumns int
			if err := readonly.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('marker') WHERE name = 'note'`).Scan(&noteColumns); err != nil {
				t.Fatalf("inspect too-new schema: %v", err)
			}
			if noteColumns != 1 {
				t.Fatalf("note column count = %d, want preserved too-new schema", noteColumns)
			}
		})
	}
}

func TestRunDatabaseExistingMaintenanceRejectsGooseBootstrapBeforeConfiguration(t *testing.T) {
	for _, action := range []string{"verify", "backup"} {
		t.Run(action, func(t *testing.T) {
			dbPath := configureCommandDatabase(t)
			ctx := context.Background()
			createFailedGooseBootstrapCommandSQLite(t, ctx, dbPath)
			before, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("read Goose bootstrap before command: %v", err)
			}

			err = runDatabaseTestCommand(ctx, action, dbPath, &bytes.Buffer{})
			if !errors.Is(err, storage.ErrSchemaNotReady) {
				t.Fatalf("db %s error = %v, want ErrSchemaNotReady", action, err)
			}
			after, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("read Goose bootstrap after command: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("Goose bootstrap bytes changed during rejected maintenance command")
			}
			for _, suffix := range []string{".lock", "-wal", "-shm"} {
				if _, statErr := os.Stat(dbPath + suffix); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("SQLite artifact %q stat error = %v, want not exist", suffix, statErr)
				}
			}
			assertCommandDatabaseJournalMode(t, ctx, dbPath, "delete")
		})
	}
}

func createTooNewCommandSQLite(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw too-new SQLite fixture: %v", err)
	}
	newerFS := fstest.MapFS{
		"001_base.sql": &fstest.MapFile{Data: []byte(
			"-- +goose Up\nPRAGMA application_id = 0x494B4341;\n" +
				"CREATE TABLE marker (id INTEGER PRIMARY KEY);\n",
		)},
		"002_newer.sql": &fstest.MapFile{Data: []byte(
			"-- +goose Up\nALTER TABLE marker ADD COLUMN note TEXT;\n",
		)},
	}
	migrator, err := storage.NewMigrator(db, newerFS)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create too-new migrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("migrate too-new fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close too-new fixture: %v", err)
	}
}

func createFailedGooseBootstrapCommandSQLite(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw Goose bootstrap fixture: %v", err)
	}
	failingFS := fstest.MapFS{
		"001_fail.sql": &fstest.MapFile{Data: []byte(
			"-- +goose Up\nINSERT INTO missing_bootstrap_table(value) VALUES ('fail');\n",
		)},
	}
	migrator, err := storage.NewMigrator(db, failingFS)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create failing Goose migrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err == nil {
		_ = db.Close()
		t.Fatal("failing Goose migration error = nil")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close Goose bootstrap fixture: %v", err)
	}
}

func createUnconfiguredCommandSQLite(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw SQLite fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE foreign_marker (id INTEGER PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatalf("create foreign marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw SQLite fixture: %v", err)
	}
}

func assertCommandDatabaseJournalMode(t *testing.T, ctx context.Context, path, want string) {
	t.Helper()
	db, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("open database read-only: %v", err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if strings.ToLower(mode) != strings.ToLower(want) {
		t.Fatalf("journal mode = %q, want %q", mode, want)
	}
}

func TestRunDatabaseStatusDoesNotCreateMissingDatabase(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	var out bytes.Buffer
	if err := runDatabaseTestCommand(context.Background(), "status", dbPath, &out); err != nil {
		t.Fatalf("db status: %v", err)
	}
	if !strings.Contains(out.String(), "current=0 target=1 pending=1") {
		t.Fatalf("status output = %q", out.String())
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database stat error = %v, want not-exist", err)
	}
}

func TestRunDatabaseStatusDoesNotModifyEmptyDatabaseFile(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create empty database file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close empty database file: %v", err)
	}

	var out bytes.Buffer
	if err := runDatabaseTestCommand(context.Background(), "status", dbPath, &out); err != nil {
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

func TestRunDatabaseExistingDatabaseActionsDoNotCreateMissingArtifacts(t *testing.T) {
	for _, action := range []string{"verify", "backup"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			dbPath := filepath.Join(root, "missing-parent", "commands.db")

			err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
			if !errors.Is(err, storage.ErrSchemaNotReady) {
				t.Fatalf("db %s error = %v, want ErrSchemaNotReady", action, err)
			}
			for _, path := range []string{filepath.Dir(dbPath), dbPath, dbPath + ".lock"} {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("artifact %q stat error = %v, want not exist", path, statErr)
				}
			}
		})
	}
}

func TestRunDatabaseExistingDatabaseActionsDoNotLockEmptyOrInvalidFiles(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		content []byte
		wantErr error
	}{
		{name: "empty", wantErr: storage.ErrSchemaNotReady},
		{name: "not-sqlite", content: []byte("this is not a SQLite database"), wantErr: storage.ErrUnrecognizedDatabase},
	} {
		for _, action := range []string{"verify", "backup"} {
			t.Run(fixture.name+"/"+action, func(t *testing.T) {
				dbPath := configureCommandDatabase(t)
				if err := os.WriteFile(dbPath, fixture.content, 0o600); err != nil {
					t.Fatalf("create fixture: %v", err)
				}

				err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
				if !errors.Is(err, fixture.wantErr) {
					t.Fatalf("db %s error = %v, want %v", action, err, fixture.wantErr)
				}
				if _, statErr := os.Lstat(dbPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("lock file stat error = %v, want not exist", statErr)
				}
				content, readErr := os.ReadFile(dbPath)
				if readErr != nil {
					t.Fatalf("read fixture after command: %v", readErr)
				}
				if !bytes.Equal(content, fixture.content) {
					t.Fatalf("fixture content changed")
				}
			})
		}
	}
}

func TestRunDatabaseExistingDatabaseActionsDoNotLockForeignSQLiteFiles(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		configure func(*testing.T, string)
		wantErr   error
	}{
		{
			name: "fresh-sqlite",
			configure: func(t *testing.T, path string) {
				db, err := storage.Open(context.Background(), path)
				if err != nil {
					t.Fatalf("open fresh SQLite fixture: %v", err)
				}
				if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
					t.Fatalf("materialize fresh SQLite fixture: %v", err)
				}
				if err := db.Close(); err != nil {
					t.Fatalf("close fresh SQLite fixture: %v", err)
				}
			},
			wantErr: storage.ErrSchemaNotReady,
		},
		{
			name: "foreign-sqlite",
			configure: func(t *testing.T, path string) {
				db, err := storage.Open(context.Background(), path)
				if err != nil {
					t.Fatalf("open foreign SQLite fixture: %v", err)
				}
				if _, err := db.Exec(`CREATE TABLE foreign_data (id INTEGER PRIMARY KEY)`); err != nil {
					t.Fatalf("create foreign SQLite fixture: %v", err)
				}
				if err := db.Close(); err != nil {
					t.Fatalf("close foreign SQLite fixture: %v", err)
				}
			},
			wantErr: storage.ErrUnrecognizedDatabase,
		},
	} {
		for _, action := range []string{"verify", "backup"} {
			t.Run(fixture.name+"/"+action, func(t *testing.T) {
				dbPath := configureCommandDatabase(t)
				fixture.configure(t, dbPath)

				err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
				if !errors.Is(err, fixture.wantErr) {
					t.Fatalf("db %s error = %v, want %v", action, err, fixture.wantErr)
				}
				if _, statErr := os.Lstat(dbPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("lock file stat error = %v, want not exist", statErr)
				}
			})
		}
	}
}

func TestRunDatabaseStatusRejectsDanglingSymlinkAndNonRegularPath(t *testing.T) {
	t.Run("dangling-symlink", func(t *testing.T) {
		root := t.TempDir()
		dbPath := filepath.Join(root, "alias.db")
		if err := os.Symlink(filepath.Join(root, "missing.db"), dbPath); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		for _, action := range []string{"status", "verify", "backup"} {
			err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("db %s error = nil, want dangling symlink rejection", action)
			}
		}
		if _, statErr := os.Lstat(dbPath); statErr != nil {
			t.Fatalf("dangling symlink changed: %v", statErr)
		}
		if _, statErr := os.Lstat(dbPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("alias lock stat error = %v, want not exist", statErr)
		}
	})

	t.Run("directory", func(t *testing.T) {
		dbPath := configureCommandDatabase(t)
		if err := os.Mkdir(dbPath, 0o700); err != nil {
			t.Fatalf("create directory fixture: %v", err)
		}
		for _, action := range []string{"status", "verify", "backup"} {
			err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("db %s error = nil, want non-regular path rejection", action)
			}
		}
		if _, statErr := os.Lstat(dbPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("directory lock stat error = %v, want not exist", statErr)
		}
	})
}

func TestRunDatabaseBackupUsesCanonicalDatabasePathThroughSymlink(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.db")
	if err := runDatabaseTestCommand(context.Background(), "migrate", realPath, &bytes.Buffer{}); err != nil {
		t.Fatalf("migrate real database: %v", err)
	}
	if err := os.Remove(realPath + ".lock"); err != nil {
		t.Fatalf("remove initial lock file: %v", err)
	}

	aliasPath := filepath.Join(root, "alias.db")
	if err := os.Symlink(realPath, aliasPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var out bytes.Buffer
	if err := runDatabaseTestCommand(context.Background(), "backup", aliasPath, &out); err != nil {
		t.Fatalf("backup through symlink: %v", err)
	}
	backupPath := strings.TrimPrefix(strings.TrimSpace(out.String()), "backup=")
	if !strings.HasPrefix(filepath.Base(backupPath), "real.db.manual.") {
		t.Fatalf("backup path = %q, want canonical real database basename", backupPath)
	}
	if _, err := os.Lstat(aliasPath + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alias lock stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(realPath + ".lock"); err != nil {
		t.Fatalf("canonical lock stat: %v", err)
	}
}

func TestRunDatabaseVerifyAndBackup(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	if err := runDatabaseTestCommand(ctx, "migrate", dbPath, &bytes.Buffer{}); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	var verifyOut bytes.Buffer
	if err := runDatabaseTestCommand(ctx, "verify", dbPath, &verifyOut); err != nil {
		t.Fatalf("db verify: %v", err)
	}
	if !strings.Contains(verifyOut.String(), "verification=ok") {
		t.Fatalf("verify output = %q", verifyOut.String())
	}

	var backupOut bytes.Buffer
	if err := runDatabaseTestCommand(ctx, "backup", dbPath, &backupOut); err != nil {
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

func TestRunDatabaseVerifyRejectsDatabaseWithPendingSchema(t *testing.T) {
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
	err = runDatabaseTestCommand(ctx, "verify", dbPath, &bytes.Buffer{})
	if !errors.Is(err, storage.ErrSchemaNotReady) {
		t.Fatalf("db verify error = %v, want ErrSchemaNotReady", err)
	}
}

func TestRunDatabaseMutatingDatabaseCommandsRespectOwnerLock(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	if err := runDatabaseTestCommand(context.Background(), "migrate", dbPath, &bytes.Buffer{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()

	for _, action := range []string{"migrate", "verify", "backup"} {
		err := runDatabaseTestCommand(context.Background(), action, dbPath, &bytes.Buffer{})
		if !errors.Is(err, storage.ErrDatabaseInUse) {
			t.Fatalf("db %s error = %v, want ErrDatabaseInUse", action, err)
		}
	}
}

func TestRunDatabaseStatusIsAllowedWhileOwnerLockIsHeld(t *testing.T) {
	dbPath := configureCommandDatabase(t)
	ctx := context.Background()
	if err := runDatabaseTestCommand(ctx, "migrate", dbPath, &bytes.Buffer{}); err != nil {
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
	if err := runDatabaseTestCommand(ctx, "status", dbPath, &out); err != nil {
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
