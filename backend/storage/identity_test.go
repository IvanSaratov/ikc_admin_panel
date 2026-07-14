package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestInspectDatabaseIdentityDistinguishesDatabaseKinds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("fresh", func(t *testing.T) {
		db, err := Open(ctx, filepath.Join(t.TempDir(), "fresh.db"))
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db.Close()

		identity, err := InspectDatabaseIdentity(ctx, db)
		if err != nil {
			t.Fatalf("InspectDatabaseIdentity() error = %v", err)
		}
		if !identity.Fresh || identity.ApplicationID != 0 || identity.UserObjects != 0 || identity.HasMigrationHistory {
			t.Fatalf("identity = %+v, want fresh empty database", identity)
		}
	})

	t.Run("recognized", func(t *testing.T) {
		db, err := Open(ctx, filepath.Join(t.TempDir(), "recognized.db"))
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db.Close()

		migrator, err := NewMigrator(db, migrationFS(false))
		if err != nil {
			t.Fatalf("NewMigrator() error = %v", err)
		}
		if _, err := migrator.Up(ctx); err != nil {
			t.Fatalf("Up() error = %v", err)
		}

		identity, err := InspectDatabaseIdentity(ctx, db)
		if err != nil {
			t.Fatalf("InspectDatabaseIdentity() error = %v", err)
		}
		if identity.Fresh || identity.ApplicationID != SQLiteApplicationID || identity.UserObjects == 0 || !identity.HasMigrationHistory {
			t.Fatalf("identity = %+v, want recognized migrated database", identity)
		}
	})

	t.Run("unmarked legacy", func(t *testing.T) {
		db, err := Open(ctx, filepath.Join(t.TempDir(), "legacy.db"))
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db.Close()
		if _, err := db.ExecContext(ctx, `CREATE TABLE legacy_items (id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatalf("create legacy table: %v", err)
		}

		_, err = InspectDatabaseIdentity(ctx, db)
		if !errors.Is(err, ErrUnrecognizedDatabase) {
			t.Fatalf("InspectDatabaseIdentity() error = %v, want ErrUnrecognizedDatabase", err)
		}
	})
}

func TestRequireCurrentSchemaRejectsUnreadyVersions(t *testing.T) {
	t.Parallel()

	recognized := DatabaseIdentity{
		ApplicationID:       SQLiteApplicationID,
		UserObjects:         2,
		HasMigrationHistory: true,
	}
	tests := []struct {
		name   string
		status MigrationStatus
		want   error
	}{
		{name: "pending", status: MigrationStatus{Current: 1, Target: 2, Pending: []MigrationInfo{{Version: 2, Name: "002.sql"}}}, want: ErrSchemaNotReady},
		{name: "too new", status: MigrationStatus{Current: 3, Target: 2}, want: ErrSchemaTooNew},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := RequireCurrentSchema(recognized, test.status)
			if !errors.Is(err, test.want) {
				t.Fatalf("RequireCurrentSchema() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRequireCurrentSchemaRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	status := MigrationStatus{Current: 2, Target: 2}
	tests := []struct {
		name     string
		identity DatabaseIdentity
	}{
		{name: "fresh", identity: DatabaseIdentity{Fresh: true}},
		{name: "wrong application id", identity: DatabaseIdentity{ApplicationID: 7, UserObjects: 2, HasMigrationHistory: true}},
		{name: "missing migration history", identity: DatabaseIdentity{ApplicationID: SQLiteApplicationID, UserObjects: 1}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := RequireCurrentSchema(test.identity, status)
			if !errors.Is(err, ErrSchemaNotReady) {
				t.Fatalf("RequireCurrentSchema() error = %v, want ErrSchemaNotReady", err)
			}
		})
	}
}
