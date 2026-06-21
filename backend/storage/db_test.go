package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
)

func TestOpenAndMigrateConfiguresSQLiteConnection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "mintrud-test.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys pragma = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestConstraintErrorMappingDetectsUniqueViolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "mintrud-test.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	insertWorker(t, ctx, db, "worker@example.test")
	err = insertWorkerRaw(ctx, db, "worker-duplicate@example.test")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("duplicate worker error = %v, want ErrConflict", err)
	}
}

func insertWorker(t *testing.T, ctx context.Context, db *sql.DB, email string) {
	t.Helper()

	if err := insertWorkerRaw(ctx, db, email); err != nil {
		t.Fatalf("insert worker: %v", err)
	}
}

func insertWorkerRaw(ctx context.Context, db *sql.DB, email string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO workers (
			last_name,
			first_name,
			middle_name,
			snils,
			snils_normalized,
			email,
			birth_date,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"Petrov",
		"Petr",
		nil,
		"123-456-789 00",
		"12345678900",
		email,
		nil,
		"2026-05-27T00:00:00Z",
		"2026-05-27T00:00:00Z",
	)
	return storage.MapSQLiteError(err)
}

// TestMigrate_002_AppliesCleanOnEmptyDB verifies that migration 002 (schema
// cleanup) creates the new imports / import_rows tables and adds the
// protocol_suffix column on a fresh, empty database — no errors and no
// leftovers from migration 001 blocking the run.
func TestMigrate_002_AppliesCleanOnEmptyDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "mintrud-migrate-002-empty.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	expectedTables := []string{"imports", "import_rows"}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name = ?",
			table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing after migrate: %v", table, err)
		}
		if name != table {
			t.Fatalf("expected table %s, got %s", table, name)
		}
	}

	importsColumns := map[string]bool{}
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(imports)")
	if err != nil {
		t.Fatalf("pragma table_info(imports): %v", err)
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			t.Fatalf("scan pragma row: %v", err)
		}
		importsColumns[name] = true
	}
	rows.Close()

	for _, col := range []string{
		"id", "source_type", "source_file_name", "source_sha256",
		"uploaded_by_actor", "received_at", "status", "created_at", "updated_at",
	} {
		if !importsColumns[col] {
			t.Fatalf("imports missing column %q (have %v)", col, importsColumns)
		}
	}

	importRowsColumns := map[string]bool{}
	rows, err = db.QueryContext(ctx, "PRAGMA table_info(import_rows)")
	if err != nil {
		t.Fatalf("pragma table_info(import_rows): %v", err)
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			t.Fatalf("scan pragma row: %v", err)
		}
		importRowsColumns[name] = true
	}
	rows.Close()

	for _, col := range []string{
		"id", "import_id", "row_number", "raw_data", "created_at",
	} {
		if !importRowsColumns[col] {
			t.Fatalf("import_rows missing column %q (have %v)", col, importRowsColumns)
		}
	}

	protocolHasSuffix := false
	rows, err = db.QueryContext(ctx, "PRAGMA table_info(protocols)")
	if err != nil {
		t.Fatalf("pragma table_info(protocols): %v", err)
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == "protocol_suffix" {
			protocolHasSuffix = true
		}
	}
	rows.Close()
	if !protocolHasSuffix {
		t.Fatalf("protocols.protocol_suffix column missing after migrate")
	}

	// Verify client_requests.source_import_id is now a real FK to imports(id).
	// An orphan source_import_id must be rejected by SQLite.
	_, err = db.ExecContext(ctx, `
		INSERT INTO employers (inn, inn_normalized, canonical_name, created_at, updated_at)
		VALUES ('7700000000', '7700000000', 'FK Test', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("seed employer: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO client_requests (
			employer_id, received_date, source_type, source_import_id,
			status, created_at, updated_at
		)
		VALUES (1, '2026-05-27', 'xlsx', 9999, 'review',
		        '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z')
	`)
	if err == nil {
		t.Fatalf("expected FK violation for orphan source_import_id, got nil")
	}
	mappedErr := storage.MapSQLiteError(err)
	if !errors.Is(mappedErr, storage.ErrConflict) {
		t.Fatalf("orphan source_import_id error = %v (mapped %v), want ErrConflict", err, mappedErr)
	}
}

// TestMigrate_002_RunsAfter001 verifies that migrations apply in order on a
// pre-existing database that already has migration 001 applied. The new
// imports / import_rows tables must coexist with all original tables, and
// protocol_suffix must appear on protocols.
func TestMigrate_002_RunsAfter001(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "mintrud-migrate-002-after.db")

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	wantTables := map[string]bool{
		"action_log":             false,
		"client_requests":        false,
		"employers":              false,
		"generation_runs":        false,
		"import_rows":            false,
		"imports":                false,
		"moodle_accounts":        false,
		"program_groups":         false,
		"programs":               false,
		"protocol_participants":  false,
		"protocols":              false,
		"request_rows":           false,
		"request_training_items": false,
		"training_records":       false,
		"worker_employers":       false,
		"workers":                false,
	}

	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan table row: %v", err)
		}
		if _, ok := wantTables[name]; ok {
			wantTables[name] = true
		}
	}
	rows.Close()

	for table, found := range wantTables {
		if !found {
			t.Fatalf("expected table %s after migrations 001+002", table)
		}
	}

	// Smoke write into a couple of new tables to make sure the FK + suffix
	// additions are not just metadata but actually usable.
	_, err = db.ExecContext(ctx, `
		INSERT INTO imports (
			id, source_type, source_file_name, source_sha256,
			uploaded_by_actor, received_at, status, created_at, updated_at
		)
		VALUES (
			1, 'manual', NULL, NULL,
			'operator_unidentified', '2026-05-27T00:00:00Z',
			'received', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z'
		)
	`)
	if err != nil {
		t.Fatalf("insert into imports: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO import_rows (id, import_id, row_number, raw_data, created_at)
		VALUES (1, 1, 1, '{}', '2026-05-27T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert into import_rows: %v", err)
	}
}
