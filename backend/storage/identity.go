package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const SQLiteApplicationID int64 = 0x494B4341

var (
	ErrUnrecognizedDatabase = errors.New("database is not recognized as an IKC application database")
	ErrSchemaNotReady       = errors.New("database schema is not ready for use")
)

type DatabaseIdentity struct {
	Fresh               bool
	ApplicationID       int64
	UserObjects         int64
	ApplicationTables   int64
	HasMigrationHistory bool
}

func InspectDatabaseIdentity(ctx context.Context, db *sql.DB) (DatabaseIdentity, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DatabaseIdentity{}, fmt.Errorf("begin SQLite identity snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var identity DatabaseIdentity
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&identity.ApplicationID); err != nil {
		return DatabaseIdentity{}, fmt.Errorf("read SQLite application ID: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE substr(name, 1, 7) <> 'sqlite_'
	`).Scan(&identity.UserObjects); err != nil {
		return DatabaseIdentity{}, fmt.Errorf("count SQLite user objects: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table'
			AND substr(name, 1, 7) <> 'sqlite_'
			AND name <> 'goose_db_version'
	`).Scan(&identity.ApplicationTables); err != nil {
		return DatabaseIdentity{}, fmt.Errorf("count SQLite application tables: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sqlite_schema
			WHERE type = 'table' AND name = 'goose_db_version'
		)
	`).Scan(&identity.HasMigrationHistory); err != nil {
		return DatabaseIdentity{}, fmt.Errorf("inspect SQLite migration history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DatabaseIdentity{}, fmt.Errorf("commit SQLite identity snapshot: %w", err)
	}

	identity.Fresh = identity.ApplicationID == 0 && identity.UserObjects == 0
	if identity.Fresh {
		return identity, nil
	}
	if identity.ApplicationID != SQLiteApplicationID {
		return identity, fmt.Errorf(
			"%w: application ID %d, user objects %d",
			ErrUnrecognizedDatabase,
			identity.ApplicationID,
			identity.UserObjects,
		)
	}
	if identity.ApplicationTables == 0 || !identity.HasMigrationHistory {
		return identity, fmt.Errorf(
			"%w: application ID %d, user objects %d, application tables %d, migration history %t",
			ErrSchemaNotReady,
			identity.ApplicationID,
			identity.UserObjects,
			identity.ApplicationTables,
			identity.HasMigrationHistory,
		)
	}
	return identity, nil
}

func RequireCurrentSchema(identity DatabaseIdentity, status MigrationStatus) error {
	if identity.Fresh || identity.ApplicationID != SQLiteApplicationID || identity.ApplicationTables == 0 || !identity.HasMigrationHistory {
		return fmt.Errorf(
			"%w: fresh %t, application ID %d, user objects %d, application tables %d, migration history %t",
			ErrSchemaNotReady,
			identity.Fresh,
			identity.ApplicationID,
			identity.UserObjects,
			identity.ApplicationTables,
			identity.HasMigrationHistory,
		)
	}
	if status.Current > status.Target {
		return fmt.Errorf(
			"%w: current version %d, target version %d",
			ErrSchemaTooNew,
			status.Current,
			status.Target,
		)
	}
	if status.Current != status.Target || len(status.Pending) != 0 {
		return fmt.Errorf(
			"%w: current version %d, target version %d, pending migrations %d",
			ErrSchemaNotReady,
			status.Current,
			status.Target,
			len(status.Pending),
		)
	}
	return nil
}
