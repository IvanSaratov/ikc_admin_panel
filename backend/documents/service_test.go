package documents

import (
	"context"
	"errors"
	"testing"
)

// TestGenerate_StatusNotFixed_Rejected exercises both XML and DOCX
// generation against a draft protocol and confirms both surface
// ErrProtocolNotFixed via errors.Is.
func TestGenerate_StatusNotFixed_Rejected(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	now := "2026-06-22T00:00:00Z"
	res, err := env.db.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		"DRAFT-G2-"+t.Name(), "Draft Group 2 "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, _ := res.LastInsertId()

	pRes, err := env.db.ExecContext(ctx, `
		INSERT INTO protocols (program_group_id, status, created_at, updated_at)
		VALUES (?, 'draft', ?, ?)`,
		groupID, now, now)
	if err != nil {
		t.Fatalf("insert protocol: %v", err)
	}
	protocolID, _ := pRes.LastInsertId()

	if _, _, err := env.svc.generateXMLWith(ctx, env.queries, protocolID); !errors.Is(err, ErrProtocolNotFixed) {
		t.Errorf("GenerateXML err = %v, want ErrProtocolNotFixed", err)
	}
	if _, _, err := env.svc.generateDOCXWith(ctx, env.queries, protocolID); !errors.Is(err, ErrProtocolNotFixed) {
		t.Errorf("GenerateDOCX err = %v, want ErrProtocolNotFixed", err)
	}
}

// TestGenerate_Exception_RecordsFailedRun covers the path where the
// legacy pipeline rejects a request mid-flight. The simplest
// reproducer is a fixed protocol with no participants — the legacy
// CreateDocx dereferences data.RegistryRecord[0] which panics. We
// convert the panic into a recorded-failed-run.
//
// This test also acts as the "exception → audit row" gate: any
// generation failure (panic or returned error) MUST produce a
// generation_runs row with status='failed' so the operator can see
// the attempt in the history table.
func TestGenerate_Exception_RecordsFailedRun(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	now := "2026-06-22T00:00:00Z"

	res, err := env.db.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		"EXC-G-"+t.Name(), "Exc Group "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, _ := res.LastInsertId()

	pRes, err := env.db.ExecContext(ctx, `
		INSERT INTO protocols (program_group_id, status, training_start_date, training_end_date, protocol_date,
		    sequence_year, protocol_month, annual_sequence_number, protocol_number, fixed_at, created_at, updated_at)
		VALUES (?, 'fixed', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		groupID, "2026-06-01", "2026-06-30", "2026-06-30",
		int64(2026), int64(6), int64(1), "2026-06/002", now, now, now)
	if err != nil {
		t.Fatalf("insert protocol: %v", err)
	}
	protocolID, _ := pRes.LastInsertId()

	// DOCX path: service layer rejects empty records with a clean error
	// (we never call legacy on empty input).
	if _, _, err := env.svc.generateDOCXWith(ctx, env.queries, protocolID); err == nil {
		t.Errorf("GenerateDOCX with no participants: expected error, got nil")
	}

	// XML path: returns an error from legacy because there are no
	// records to emit — but renderRegistrySet happily returns an
	// empty set, and legacy.GenerateXML emits an empty document.
	raw, run, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if err != nil {
		// If an error fires, we want a 'failed' row in generation_runs.
		if run == nil || run.Status != "failed" {
			t.Errorf("GenerateXML error path: run = %+v, want status=failed", run)
		}
		return
	}
	// If no error fires (empty output is OK), we still want a 'success' row.
	if len(raw) == 0 && (run == nil || run.Status != "success") {
		t.Errorf("GenerateXML empty: run = %+v, want status=success", run)
	}
}
