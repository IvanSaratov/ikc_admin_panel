package documents

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// testEnv bundles a fresh in-memory SQLite DB, the queries facade and a
// default Service wired through SetDefaultService. Each test gets its
// own DB so failures don't bleed across cases.
type testEnv struct {
	db      *sql.DB
	queries *storagedb.Queries
	svc     *Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "documents-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	svc := NewService(database, queries, auditSvc, nil)
	// Install as the package-level default for the duration of the test.
	// We intentionally do NOT reset it on cleanup because tests run in
	// parallel and a nil-out from one would break sibling tests. Each
	// test installs its own service before exercising the top-level
	// GenerateXML / GenerateDOCX functions.
	SetDefaultService(svc)
	return &testEnv{db: database, queries: queries, svc: svc}
}

// seedFixedProtocol inserts the chain required for a fixed protocol with
// a single participant: program_group + program + worker + employer +
// worker_employer + training_record. Returns the protocol id and the
// training_record id so callers can attach the participant.
func (e *testEnv) seedFixedProtocol(t *testing.T) (protocolID, trID int64) {
	t.Helper()
	ctx := context.Background()
	now := "2026-06-22T00:00:00Z"

	res, err := e.db.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		"DOC-G-"+t.Name(), "DOC Group "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, _ := res.LastInsertId()

	progRes, err := e.db.ExecContext(ctx, `
		INSERT INTO programs (program_group_id, code, name, default_hours, status, created_at, updated_at)
		VALUES (?, ?, ?, 40, 'active', ?, ?)`,
		groupID, "DOC-P-"+t.Name(), "DOC Program "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert program: %v", err)
	}
	programID, _ := progRes.LastInsertId()

	snils := "12345678901"
	workerRes, err := e.db.ExecContext(ctx, `
		INSERT INTO workers (last_name, first_name, middle_name, snils, snils_normalized, email, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"Иванов", "Иван", "Иванович", snils, snils, "ivan@example.test", now, now)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	workerID, _ := workerRes.LastInsertId()

	empRes, err := e.db.ExecContext(ctx, `
		INSERT INTO employers (inn, inn_normalized, canonical_name, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`,
		"7701234567", "7701234567", "DOC Employer "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert employer: %v", err)
	}
	employerID, _ := empRes.LastInsertId()

	weRes, err := e.db.ExecContext(ctx, `
		INSERT INTO worker_employers (worker_id, employer_id, current_position, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`,
		workerID, employerID, "Engineer", now, now)
	if err != nil {
		t.Fatalf("insert worker_employer: %v", err)
	}
	weID, _ := weRes.LastInsertId()

	trRes, err := e.db.ExecContext(ctx, `
		INSERT INTO training_records (worker_employer_id, program_id, position, hours, requires_mintrud_test, moodle_status, status, created_at, updated_at)
		VALUES (?, ?, ?, 40, 0, 'not_required', 'active', ?, ?)`,
		weID, programID, "Engineer", now, now)
	if err != nil {
		t.Fatalf("insert training_record: %v", err)
	}
	trID, _ = trRes.LastInsertId()

	// Create the protocol in fixed state with a complete number stamp.
	protoRes, err := e.db.ExecContext(ctx, `
		INSERT INTO protocols (program_group_id, status, training_start_date, training_end_date, protocol_date,
		    sequence_year, protocol_month, annual_sequence_number, protocol_number, protocol_suffix, fixed_at, created_at, updated_at)
		VALUES (?, 'fixed', ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		groupID, "2026-06-01", "2026-06-30", "2026-06-30",
		int64(2026), int64(6), int64(1), "2026-06/001", now, now, now)
	if err != nil {
		t.Fatalf("insert protocol: %v", err)
	}
	protocolID, _ = protoRes.LastInsertId()

	// Attach the training record as an active participant.
	_, err = e.db.ExecContext(ctx, `
		INSERT INTO protocol_participants (protocol_id, training_record_id, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		protocolID, trID, now, now)
	if err != nil {
		t.Fatalf("insert protocol_participant: %v", err)
	}
	return protocolID, trID
}
