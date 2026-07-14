package storage

import (
	"context"
	"database/sql"
	"fmt"
)

type BackupCreator interface {
	Create(context.Context, *sql.DB, int64) (string, error)
}

type PreparationResult struct {
	Before     MigrationStatus
	After      MigrationStatus
	Migration  MigrationResult
	BackupPath string
	Identity   DatabaseIdentity
}

type Initializer struct {
	db       *sql.DB
	migrator *Migrator
	backups  BackupCreator
}

func NewInitializer(db *sql.DB, migrator *Migrator, backups BackupCreator) *Initializer {
	return &Initializer{
		db:       db,
		migrator: migrator,
		backups:  backups,
	}
}

func NewEmbeddedInitializer(db *sql.DB, dbPath string) (*Initializer, error) {
	migrator, err := NewEmbeddedMigrator(db)
	if err != nil {
		return nil, fmt.Errorf("create embedded SQLite migrator: %w", err)
	}
	return NewInitializer(db, migrator, NewBackupManager(dbPath)), nil
}

func (i *Initializer) Prepare(ctx context.Context) (PreparationResult, error) {
	identity, err := InspectDatabaseIdentity(ctx, i.db)
	result := PreparationResult{Identity: identity}
	if err != nil {
		return result, fmt.Errorf("inspect SQLite database identity before preparation: %w", err)
	}

	status, err := i.migrator.Status(ctx)
	if err != nil {
		return result, fmt.Errorf("inspect SQLite migration status before preparation: %w", err)
	}
	result.Before = status
	result.After = status

	if status.Current > status.Target {
		return result, fmt.Errorf(
			"%w: current version %d, target version %d",
			ErrSchemaTooNew,
			status.Current,
			status.Target,
		)
	}
	if !identity.Fresh && status.Current == 0 {
		return result, fmt.Errorf(
			"%w: recognized non-fresh database has Goose version 0",
			ErrSchemaNotReady,
		)
	}
	if len(status.Pending) == 0 {
		if err := RequireCurrentSchema(identity, status); err != nil {
			return result, fmt.Errorf("validate current SQLite schema: %w", err)
		}
		return result, nil
	}

	if status.Current > 0 {
		if err := QuickCheck(ctx, i.db); err != nil {
			return result, fmt.Errorf("quick-check SQLite database before migration: %w", err)
		}
		result.BackupPath, err = i.backups.Create(ctx, i.db, status.Current)
		if err != nil {
			return result, fmt.Errorf("create pre-migration SQLite backup: %w", err)
		}
	}

	result.Migration, err = i.migrator.Up(ctx)
	if err != nil {
		return result, fmt.Errorf("apply SQLite migrations: %w", err)
	}
	if err := ForeignKeyCheck(ctx, i.db); err != nil {
		return result, fmt.Errorf("check SQLite foreign keys after migration: %w", err)
	}
	if err := QuickCheck(ctx, i.db); err != nil {
		return result, fmt.Errorf("quick-check SQLite database after migration: %w", err)
	}

	result.After, err = i.migrator.Status(ctx)
	if err != nil {
		return result, fmt.Errorf("inspect SQLite migration status after preparation: %w", err)
	}
	result.Identity, err = InspectDatabaseIdentity(ctx, i.db)
	if err != nil {
		return result, fmt.Errorf("inspect SQLite database identity after preparation: %w", err)
	}
	if err := RequireCurrentSchema(result.Identity, result.After); err != nil {
		return result, fmt.Errorf("validate prepared SQLite schema: %w", err)
	}
	return result, nil
}
