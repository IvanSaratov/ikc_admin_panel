package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDatabaseChecksAcceptValidMigratedDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "valid.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := MigrateContext(ctx, db); err != nil {
		t.Fatalf("MigrateContext() error = %v", err)
	}
	identity, err := InspectDatabaseIdentity(ctx, db)
	if err != nil {
		t.Fatalf("InspectDatabaseIdentity() error = %v", err)
	}
	migrator, err := NewEmbeddedMigrator(db)
	if err != nil {
		t.Fatalf("NewEmbeddedMigrator() error = %v", err)
	}
	status, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if err := RequireCurrentSchema(identity, status); err != nil {
		t.Fatalf("RequireCurrentSchema() error = %v", err)
	}

	checks := []struct {
		name  string
		check func(context.Context, *sql.DB) error
	}{
		{name: "quick", check: QuickCheck},
		{name: "integrity", check: IntegrityCheck},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			if err := check.check(ctx, db); err != nil {
				t.Fatalf("check error = %v", err)
			}
		})
	}
	if err := ForeignKeyCheck(ctx, db); err != nil {
		t.Fatalf("ForeignKeyCheck() error = %v", err)
	}
}

func TestForeignKeyCheckRejectsViolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "foreign-key.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	statements := []string{
		`CREATE TABLE parents (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parents(id))`,
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO children (id, parent_id) VALUES (1, 999)`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("execute %q: %v", statement, err)
		}
	}

	if err := ForeignKeyCheck(ctx, db); err == nil {
		t.Fatal("ForeignKeyCheck() error = nil, want violation")
	}
}
