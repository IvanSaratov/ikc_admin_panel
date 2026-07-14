package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"
)

const preparationDiagnosticTimeout = 2 * time.Second

// BackupCreator creates a verified pre-migration backup. A nonempty path
// returned with an error means the backup is verified but retention pruning
// failed; callers must preserve both the path and the error.
type BackupCreator interface {
	Create(context.Context, *sql.DB, int64) (string, error)
}

type backupPathValidator interface {
	validateDatabasePath(context.Context, *sql.DB) error
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
	gate     chan struct{}
}

func NewInitializer(db *sql.DB, migrator *Migrator, backups BackupCreator) *Initializer {
	return &Initializer{
		db:       db,
		migrator: migrator,
		backups:  backups,
		gate:     make(chan struct{}, 1),
	}
}

func NewEmbeddedInitializer(db *sql.DB, dbPath string) (*Initializer, error) {
	if db == nil {
		return nil, errors.New("SQLite database is required")
	}
	if dbPath == "" {
		return nil, errors.New("SQLite database path is required")
	}
	if !filepath.IsAbs(dbPath) {
		return nil, fmt.Errorf("SQLite database path must be absolute: %q", dbPath)
	}
	migrator, err := NewEmbeddedMigrator(db)
	if err != nil {
		return nil, fmt.Errorf("create embedded SQLite migrator: %w", err)
	}
	return NewInitializer(db, migrator, NewBackupManager(dbPath)), nil
}

// Prepare validates and migrates the database before application startup. The
// caller must hold the database OwnerLock for the full operation. The internal
// gate serializes calls only on this Initializer; using separate Initializers
// concurrently without the OwnerLock is unsupported.
func (i *Initializer) Prepare(ctx context.Context) (PreparationResult, error) {
	var result PreparationResult
	if err := i.validateDependencies(ctx); err != nil {
		return result, err
	}
	if err := i.acquire(ctx); err != nil {
		return result, err
	}
	defer i.release()

	if validator, ok := i.backups.(backupPathValidator); ok {
		if err := validator.validateDatabasePath(ctx, i.db); err != nil {
			return result, fmt.Errorf("validate SQLite backup source path: %w", err)
		}
	}

	identity, identityErr := InspectDatabaseIdentity(ctx, i.db)
	result.Identity = identity
	retryableBootstrap := false
	if identityErr != nil {
		if errors.Is(identityErr, ErrUnrecognizedDatabase) {
			var bootstrapErr error
			retryableBootstrap, bootstrapErr = inspectRetryableGooseBootstrap(ctx, i.db)
			if bootstrapErr != nil {
				return result, errors.Join(
					fmt.Errorf("inspect SQLite database identity before preparation: %w", identityErr),
					fmt.Errorf("inspect failed Goose bootstrap state: %w", bootstrapErr),
				)
			}
		}
		if !retryableBootstrap {
			return result, fmt.Errorf("inspect SQLite database identity before preparation: %w", identityErr)
		}
	}

	status, err := i.preparationStatus(ctx, identity, retryableBootstrap)
	if err != nil {
		return result, err
	}
	result.Before = status
	result.After = status
	result.Migration = MigrationResult{From: status.Current, To: status.Current}

	if status.Current > status.Target {
		return result, fmt.Errorf(
			"%w: current version %d, target version %d",
			ErrSchemaTooNew,
			status.Current,
			status.Target,
		)
	}
	if !identity.Fresh && !retryableBootstrap && status.Current == 0 {
		return result, fmt.Errorf(
			"%w: recognized non-fresh database has Goose version 0",
			ErrSchemaNotReady,
		)
	}
	if len(status.Pending) == 0 {
		if err := RequireCurrentSchema(identity, status); err != nil {
			return result, fmt.Errorf("validate current SQLite schema: %w", err)
		}
		if err := ForeignKeyCheck(ctx, i.db); err != nil {
			return result, fmt.Errorf("check current SQLite foreign keys: %w", err)
		}
		if err := QuickCheck(ctx, i.db); err != nil {
			return result, fmt.Errorf("quick-check current SQLite database: %w", err)
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
		return i.failAfterMutation(ctx, result, fmt.Errorf("apply SQLite migrations: %w", err))
	}
	if err := ForeignKeyCheck(ctx, i.db); err != nil {
		return i.failAfterMutation(ctx, result, fmt.Errorf("check SQLite foreign keys after migration: %w", err))
	}
	if err := QuickCheck(ctx, i.db); err != nil {
		return i.failAfterMutation(ctx, result, fmt.Errorf("quick-check SQLite database after migration: %w", err))
	}

	result.After, err = i.migrator.Status(ctx)
	if err != nil {
		return i.failAfterMutation(ctx, result, fmt.Errorf("inspect SQLite migration status after preparation: %w", err))
	}
	result.Identity, err = InspectDatabaseIdentity(ctx, i.db)
	if err != nil {
		return i.failAfterMutation(ctx, result, fmt.Errorf("inspect SQLite database identity after preparation: %w", err))
	}
	if err := RequireCurrentSchema(result.Identity, result.After); err != nil {
		return i.failAfterMutation(ctx, result, fmt.Errorf("validate prepared SQLite schema: %w", err))
	}
	return result, nil
}

func (i *Initializer) validateDependencies(ctx context.Context) error {
	if i == nil {
		return errors.New("SQLite initializer is required")
	}
	if ctx == nil {
		return errors.New("SQLite preparation context is required")
	}
	if i.db == nil {
		return errors.New("SQLite initializer database is required")
	}
	if i.migrator == nil {
		return errors.New("SQLite initializer migrator is required")
	}
	if interfaceIsNil(i.backups) {
		return errors.New("SQLite initializer backup creator is required")
	}
	if i.gate == nil {
		return errors.New("SQLite initializer preparation gate is not initialized")
	}
	return nil
}

func interfaceIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (i *Initializer) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("wait to prepare SQLite database: %w", err)
	}
	select {
	case i.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait to prepare SQLite database: %w", ctx.Err())
	}
}

func (i *Initializer) release() {
	<-i.gate
}

func (i *Initializer) preparationStatus(ctx context.Context, identity DatabaseIdentity, retryableBootstrap bool) (MigrationStatus, error) {
	if identity.Fresh {
		return i.migrator.Catalog(), nil
	}

	status, err := i.migrator.Status(ctx)
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("inspect SQLite migration status before preparation: %w", err)
	}
	if retryableBootstrap {
		catalog := i.migrator.Catalog()
		if !bootstrapStatusMatchesCatalog(status, catalog) {
			return MigrationStatus{}, fmt.Errorf(
				"%w: failed Goose bootstrap status %+v differs from migration catalog %+v",
				ErrSchemaNotReady,
				status,
				catalog,
			)
		}
		return catalog, nil
	}
	return status, nil
}

func bootstrapStatusMatchesCatalog(status, catalog MigrationStatus) bool {
	if status.Current != 0 || status.Target != catalog.Target || len(status.Pending) != len(catalog.Pending) {
		return false
	}
	for index := range status.Pending {
		if status.Pending[index] != catalog.Pending[index] {
			return false
		}
	}
	return true
}

func (i *Initializer) failAfterMutation(ctx context.Context, result PreparationResult, primary error) (PreparationResult, error) {
	diagnosticCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), preparationDiagnosticTimeout)
	defer cancel()
	if err := i.refreshAfterMutation(diagnosticCtx, &result); err != nil {
		primary = errors.Join(primary, fmt.Errorf("refresh SQLite preparation result after failure: %w", err))
	}
	return result, primary
}

func (i *Initializer) refreshAfterMutation(ctx context.Context, result *PreparationResult) error {
	identity, identityErr := InspectDatabaseIdentity(ctx, i.db)
	result.Identity = identity

	retryableBootstrap := false
	var refreshErr error
	if identityErr != nil {
		if errors.Is(identityErr, ErrUnrecognizedDatabase) {
			var bootstrapErr error
			retryableBootstrap, bootstrapErr = inspectRetryableGooseBootstrap(ctx, i.db)
			if bootstrapErr != nil {
				refreshErr = errors.Join(refreshErr, fmt.Errorf("inspect failed Goose bootstrap state: %w", bootstrapErr))
			}
		}
		if !retryableBootstrap {
			refreshErr = errors.Join(refreshErr, fmt.Errorf("inspect SQLite database identity: %w", identityErr))
		}
	}

	if identity.Fresh || retryableBootstrap {
		result.After = i.migrator.Catalog()
		return refreshErr
	}
	status, statusErr := i.migrator.Status(ctx)
	result.After = status
	if statusErr != nil {
		refreshErr = errors.Join(refreshErr, fmt.Errorf("inspect SQLite migration status: %w", statusErr))
	}
	return refreshErr
}

type gooseColumn struct {
	cid        int
	name       string
	typeName   string
	notNull    int
	defaultSQL sql.NullString
	primaryKey int
	hidden     int
}

func inspectRetryableGooseBootstrap(ctx context.Context, db *sql.DB) (retryable bool, err error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, fmt.Errorf("begin failed Goose bootstrap snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var applicationID, userObjects, applicationTables int64
	if err := tx.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		return false, fmt.Errorf("read failed Goose bootstrap application ID: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE substr(name, 1, 7) <> 'sqlite_'
	`).Scan(&userObjects); err != nil {
		return false, fmt.Errorf("count failed Goose bootstrap objects: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table'
			AND substr(name, 1, 7) <> 'sqlite_'
			AND name <> 'goose_db_version'
	`).Scan(&applicationTables); err != nil {
		return false, fmt.Errorf("count failed Goose bootstrap application tables: %w", err)
	}
	var serviceTables int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table' AND name = 'goose_db_version'
	`).Scan(&serviceTables); err != nil {
		return false, fmt.Errorf("inspect failed Goose bootstrap service table: %w", err)
	}
	if applicationID != 0 || userObjects != 1 || applicationTables != 0 || serviceTables != 1 {
		return false, nil
	}

	columns, err := tx.QueryContext(ctx, `PRAGMA table_xinfo('goose_db_version')`)
	if err != nil {
		return false, fmt.Errorf("inspect failed Goose bootstrap columns: %w", err)
	}
	var actualColumns []gooseColumn
	for columns.Next() {
		var column gooseColumn
		if err := columns.Scan(
			&column.cid,
			&column.name,
			&column.typeName,
			&column.notNull,
			&column.defaultSQL,
			&column.primaryKey,
			&column.hidden,
		); err != nil {
			_ = columns.Close()
			return false, fmt.Errorf("scan failed Goose bootstrap column: %w", err)
		}
		actualColumns = append(actualColumns, column)
	}
	if err := columns.Err(); err != nil {
		_ = columns.Close()
		return false, fmt.Errorf("iterate failed Goose bootstrap columns: %w", err)
	}
	if err := columns.Close(); err != nil {
		return false, fmt.Errorf("close failed Goose bootstrap columns: %w", err)
	}
	expectedColumns := []gooseColumn{
		{cid: 0, name: "id", typeName: "INTEGER", primaryKey: 1},
		{cid: 1, name: "version_id", typeName: "INTEGER", notNull: 1},
		{cid: 2, name: "is_applied", typeName: "INTEGER", notNull: 1},
		{cid: 3, name: "tstamp", typeName: "TIMESTAMP", defaultSQL: sql.NullString{String: "datetime('now')", Valid: true}},
	}
	if !gooseColumnsEqual(actualColumns, expectedColumns) {
		return false, nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT version_id, is_applied, typeof(version_id), typeof(is_applied)
		FROM goose_db_version
	`)
	if err != nil {
		return false, fmt.Errorf("inspect failed Goose bootstrap rows: %w", err)
	}
	rowCount := 0
	for rows.Next() {
		rowCount++
		var version, applied int64
		var versionType, appliedType string
		if err := rows.Scan(&version, &applied, &versionType, &appliedType); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan failed Goose bootstrap row: %w", err)
		}
		if version != 0 || applied != 1 || versionType != "integer" || appliedType != "integer" {
			_ = rows.Close()
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("iterate failed Goose bootstrap rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("close failed Goose bootstrap rows: %w", err)
	}
	if rowCount != 1 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit failed Goose bootstrap snapshot: %w", err)
	}
	return true, nil
}

func gooseColumnsEqual(actual, expected []gooseColumn) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
