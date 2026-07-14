package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
)

type databaseOpenPolicy uint8

const (
	databaseMayCreate databaseOpenPolicy = iota
	databaseMustExist
)

type databaseFileState uint8

const (
	databaseFileMissing databaseFileState = iota
	databaseFileEmpty
	databaseFileSQLite
)

const sqliteFileHeader = "SQLite format 3\x00"

func writeEmbeddedCatalogStatus(out io.Writer) error {
	catalog, err := storage.EmbeddedMigrationCatalog()
	if err != nil {
		return err
	}
	return writeMigrationStatus(out, catalog)
}

func runStatusCommand(ctx context.Context, config runtimeConfig, stdout io.Writer) error {
	state, err := inspectDatabaseFile(config.DBPath)
	if err != nil {
		return err
	}
	if state == databaseFileMissing || state == databaseFileEmpty {
		return writeEmbeddedCatalogStatus(stdout)
	}

	db, err := storage.OpenReadOnly(ctx, config.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	identity, err := storage.InspectDatabaseIdentity(ctx, db)
	if err != nil {
		return err
	}
	if identity.Fresh {
		return writeEmbeddedCatalogStatus(stdout)
	}

	migrator, err := storage.NewEmbeddedMigrator(db)
	if err != nil {
		return err
	}
	status, err := migrator.Status(ctx)
	if err != nil {
		return err
	}
	if err := writeMigrationStatus(stdout, status); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	if status.Current > status.Target {
		return fmt.Errorf(
			"%w: current=%d target=%d",
			storage.ErrSchemaTooNew,
			status.Current,
			status.Target,
		)
	}
	return nil
}

func withOwnedDatabase(
	ctx context.Context,
	path string,
	policy databaseOpenPolicy,
	fn func(*sql.DB, string) error,
) (retErr error) {
	if policy == databaseMayCreate {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	} else if err := requireExistingSQLiteDatabase(ctx, path); err != nil {
		return err
	}
	owner, err := storage.AcquireOwnerLock(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, owner.Close()) }()
	ownedPath := owner.DatabasePath()
	if policy == databaseMustExist {
		if err := requireExistingSQLiteDatabase(ctx, ownedPath); err != nil {
			return err
		}
	}

	db, err := storage.Open(ctx, ownedPath)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	return fn(db, ownedPath)
}

func requireExistingSQLiteDatabase(ctx context.Context, path string) error {
	state, err := inspectDatabaseFile(path)
	if err != nil {
		return err
	}
	if state != databaseFileSQLite {
		return fmt.Errorf("%w: existing non-empty SQLite database is required", storage.ErrSchemaNotReady)
	}
	return inspectApplicationDatabaseReadOnly(ctx, path)
}

func inspectApplicationDatabaseReadOnly(ctx context.Context, path string) (retErr error) {
	db, err := storage.OpenReadOnly(ctx, path)
	if err != nil {
		return fmt.Errorf("validate existing SQLite database: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()

	identity, err := storage.InspectDatabaseIdentity(ctx, db)
	if err != nil {
		return err
	}
	if identity.Fresh {
		return fmt.Errorf("%w: existing SQLite database has no application schema", storage.ErrSchemaNotReady)
	}
	return nil
}

func inspectDatabaseFile(path string) (databaseFileState, error) {
	if path == "" {
		return databaseFileMissing, errors.New("database path is empty")
	}
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return databaseFileMissing, nil
		}
		return databaseFileMissing, fmt.Errorf("inspect database path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return databaseFileMissing, errors.New("database path is a dangling symlink")
		}
		return databaseFileMissing, fmt.Errorf("stat database path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return databaseFileMissing, errors.New("database path is not a regular file")
	}
	if info.Size() == 0 {
		return databaseFileEmpty, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return databaseFileMissing, fmt.Errorf("open database header: %w", err)
	}
	header := make([]byte, len(sqliteFileHeader))
	readCount, readErr := io.ReadFull(file, header)
	closeErr := file.Close()
	if readErr != nil || string(header) != sqliteFileHeader {
		validationErr := fmt.Errorf(
			"%w: invalid SQLite file header (%d bytes read)",
			storage.ErrUnrecognizedDatabase,
			readCount,
		)
		if closeErr != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("close database header: %w", closeErr))
		}
		return databaseFileMissing, validationErr
	}
	if closeErr != nil {
		return databaseFileMissing, fmt.Errorf("close database header: %w", closeErr)
	}
	return databaseFileSQLite, nil
}

func writeMigrationStatus(out io.Writer, status storage.MigrationStatus) error {
	if _, err := fmt.Fprintf(
		out,
		"current=%d target=%d pending=%d\n",
		status.Current,
		status.Target,
		len(status.Pending),
	); err != nil {
		return err
	}
	for _, pending := range status.Pending {
		if _, err := fmt.Fprintf(out, "pending_migration=%d:%s\n", pending.Version, pending.Name); err != nil {
			return err
		}
	}
	return nil
}

func logAppliedMigrations(logger *zap.Logger, applied []storage.AppliedMigration) {
	if logger == nil {
		return
	}
	for _, migration := range applied {
		logger.Info(
			"database migration applied",
			zap.Int64("version", migration.Version),
			zap.String("migration", migration.Name),
			zap.Duration("duration", migration.Duration),
		)
	}
}

func logPendingMigrations(logger *zap.Logger, pending []storage.MigrationInfo) {
	if logger == nil {
		return
	}
	for _, migration := range pending {
		logger.Info(
			"database migration pending",
			zap.Int64("version", migration.Version),
			zap.String("migration", migration.Name),
		)
	}
}

func logMigrationFailure(logger *zap.Logger, err error) {
	if logger == nil {
		return
	}
	var failure *storage.MigrationFailure
	if !errors.As(err, &failure) {
		return
	}
	logger.Error(
		"database migration failed",
		zap.Int64("schema_from", failure.From),
		zap.Int64("schema_target", failure.Target),
		zap.Int64("schema_current", failure.To),
		zap.Int64("failed_version", failure.Failed.Version),
		zap.String("failed_migration", failure.Failed.Name),
		zap.Error(failure.Err),
	)
}

func logVerifiedBackup(logger *zap.Logger, databasePath string, backupPath string) {
	if logger == nil || backupPath == "" {
		return
	}
	logger.Info(
		"pre-migration backup verified",
		zap.String("database_path", databasePath),
		zap.String("backup_path", backupPath),
	)
}

func logDatabasePreparationFailure(
	logger *zap.Logger,
	databasePath string,
	result storage.PreparationResult,
	duration time.Duration,
	err error,
) {
	if logger == nil {
		return
	}
	logger.Error(
		"database preparation failed",
		zap.String("database_path", databasePath),
		zap.Int64("schema_current", result.Before.Current),
		zap.Int64("schema_target", result.Before.Target),
		zap.Int("pending_migrations", len(result.Before.Pending)),
		zap.String("backup_path", result.BackupPath),
		zap.Duration("preparation_duration", duration),
		zap.Error(err),
	)
}

func runDatabaseCommand(
	ctx context.Context,
	action string,
	config runtimeConfig,
	stdout io.Writer,
	logger *zap.Logger,
) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	if action == "status" {
		return runStatusCommand(ctx, config, stdout)
	}
	if action != "migrate" && action != "verify" && action != "backup" {
		return ErrUsage
	}

	policy := databaseMustExist
	if action == "migrate" {
		policy = databaseMayCreate
	}
	return withOwnedDatabase(ctx, config.DBPath, policy, func(db *sql.DB, ownedPath string) error {
		if action == "migrate" {
			initializer, err := storage.NewEmbeddedInitializer(db, ownedPath)
			if err != nil {
				return err
			}
			started := time.Now()
			result, err := initializer.Prepare(ctx)
			duration := time.Since(started)
			logPendingMigrations(logger, result.Before.Pending)
			logAppliedMigrations(logger, result.Migration.Applied)
			logVerifiedBackup(logger, ownedPath, result.BackupPath)
			if err != nil {
				logMigrationFailure(logger, err)
				logDatabasePreparationFailure(logger, ownedPath, result, duration, err)
				return err
			}
			return writeMigrationStatus(stdout, result.After)
		}

		identity, err := storage.InspectDatabaseIdentity(ctx, db)
		if err != nil {
			return err
		}
		if identity.Fresh {
			return storage.ErrSchemaNotReady
		}
		migrator, err := storage.NewEmbeddedMigrator(db)
		if err != nil {
			return err
		}
		status, err := migrator.Status(ctx)
		if err != nil {
			return err
		}
		if status.Current > status.Target {
			return fmt.Errorf(
				"%w: current=%d target=%d",
				storage.ErrSchemaTooNew,
				status.Current,
				status.Target,
			)
		}

		switch action {
		case "verify":
			if err := storage.RequireCurrentSchema(identity, status); err != nil {
				return err
			}
			if err := storage.IntegrityCheck(ctx, db); err != nil {
				return err
			}
			if err := storage.ForeignKeyCheck(ctx, db); err != nil {
				return err
			}
			if err := writeMigrationStatus(stdout, status); err != nil {
				return err
			}
			_, err := fmt.Fprintln(stdout, "verification=ok")
			return err
		case "backup":
			if err := storage.QuickCheck(ctx, db); err != nil {
				return err
			}
			path, err := storage.NewBackupManager(ownedPath).CreateManual(ctx, db, status.Current)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "backup=%s\n", path)
			return err
		default:
			return ErrUsage
		}
	})
}
