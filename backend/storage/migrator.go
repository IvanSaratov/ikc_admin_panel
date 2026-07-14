package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/migrations"
	"github.com/pressly/goose/v3"
)

var ErrSchemaTooNew = errors.New("database schema is newer than this binary")

type MigrationInfo struct {
	Version int64
	Name    string
}

type MigrationStatus struct {
	Current int64
	Target  int64
	Pending []MigrationInfo
}

type AppliedMigration struct {
	MigrationInfo
	Duration time.Duration
}

type MigrationResult struct {
	From    int64
	To      int64
	Applied []AppliedMigration
}

type MigrationFailure struct {
	From    int64
	Target  int64
	To      int64
	Applied []AppliedMigration
	Failed  AppliedMigration
	Err     error
}

func (e *MigrationFailure) Error() string {
	return fmt.Sprintf(
		"migration %d (%s) failed from version %d toward target %d; current version %d: %v",
		e.Failed.Version,
		e.Failed.Name,
		e.From,
		e.Target,
		e.To,
		e.Err,
	)
}

func (e *MigrationFailure) Unwrap() error {
	return e.Err
}

type Migrator struct {
	provider *goose.Provider
}

func NewMigrator(db *sql.DB, fsys fs.FS) (*Migrator, error) {
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		db,
		fsys,
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return &Migrator{provider: provider}, nil
}

func NewEmbeddedMigrator(db *sql.DB) (*Migrator, error) {
	return NewMigrator(db, migrations.FS)
}

func (m *Migrator) Catalog() MigrationStatus {
	sources := m.provider.ListSources()
	pending := make([]MigrationInfo, 0, len(sources))
	for _, source := range sources {
		pending = append(pending, migrationInfo(source))
	}

	var target int64
	if len(sources) > 0 {
		target = sources[len(sources)-1].Version
	}
	return MigrationStatus{Target: target, Pending: pending}
}

func EmbeddedMigrationCatalog() (MigrationStatus, error) {
	db, err := sql.Open(sqliteDriverName, ":memory:")
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("open in-memory migration catalog database: %w", err)
	}
	defer db.Close()

	migrator, err := NewEmbeddedMigrator(db)
	if err != nil {
		return MigrationStatus{}, err
	}
	return migrator.Catalog(), nil
}

func (m *Migrator) Status(ctx context.Context) (MigrationStatus, error) {
	current, target, err := m.provider.GetVersions(ctx)
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("get migration versions: %w", err)
	}

	status := MigrationStatus{Current: current, Target: target}
	if current > target {
		return status, nil
	}

	migrations, err := m.provider.Status(ctx)
	if err != nil {
		return MigrationStatus{}, fmt.Errorf("get migration status: %w", err)
	}
	for _, migration := range migrations {
		if migration.State == goose.StatePending {
			status.Pending = append(status.Pending, migrationInfo(migration.Source))
		}
	}
	return status, nil
}

func (m *Migrator) Up(ctx context.Context) (MigrationResult, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return MigrationResult{}, err
	}
	if status.Current > status.Target {
		result := MigrationResult{From: status.Current, To: status.Current}
		return result, fmt.Errorf(
			"%w: current version %d, target version %d",
			ErrSchemaTooNew,
			status.Current,
			status.Target,
		)
	}

	results, err := m.provider.Up(ctx)
	if err != nil {
		var partial *goose.PartialError
		if !errors.As(err, &partial) {
			return MigrationResult{}, fmt.Errorf("apply migrations: %w", err)
		}

		to, versionErr := m.provider.GetDBVersion(ctx)
		if versionErr != nil {
			return MigrationResult{}, fmt.Errorf("get database version after partial migration: %w", versionErr)
		}
		applied := appliedMigrations(partial.Applied)
		failure := &MigrationFailure{
			From:    status.Current,
			Target:  status.Target,
			To:      to,
			Applied: applied,
			Failed:  appliedMigration(partial.Failed),
			Err:     partial.Err,
		}
		return MigrationResult{From: status.Current, To: to, Applied: applied}, failure
	}

	applied := appliedMigrations(results)
	to, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("get database version after migrations: %w", err)
	}
	return MigrationResult{From: status.Current, To: to, Applied: applied}, nil
}

func migrationInfo(source *goose.Source) MigrationInfo {
	return MigrationInfo{
		Version: source.Version,
		Name:    filepath.Base(source.Path),
	}
}

func appliedMigration(result *goose.MigrationResult) AppliedMigration {
	return AppliedMigration{
		MigrationInfo: migrationInfo(result.Source),
		Duration:      result.Duration,
	}
}

func appliedMigrations(results []*goose.MigrationResult) []AppliedMigration {
	applied := make([]AppliedMigration, 0, len(results))
	for _, result := range results {
		applied = append(applied, appliedMigration(result))
	}
	return applied
}

func MigrateContext(ctx context.Context, db *sql.DB) error {
	migrator, err := NewEmbeddedMigrator(db)
	if err != nil {
		return err
	}
	_, err = migrator.Up(ctx)
	return err
}

func Migrate(db *sql.DB) error {
	return MigrateContext(context.Background(), db)
}
