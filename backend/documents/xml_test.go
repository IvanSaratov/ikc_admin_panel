package documents

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// TestGenerateXML_RecordsGenerationRun verifies that a successful XML
// generation writes a 'success' row to generation_runs and that the row
// has a non-empty file_name and no error_message.
func TestGenerateXML_RecordsGenerationRun(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	protocolID, _ := env.seedFixedProtocol(t)

	ctx := context.Background()
	raw, run, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if err != nil {
		t.Fatalf("GenerateXML: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("GenerateXML produced empty bytes")
	}
	if run == nil {
		t.Fatalf("GenerateXML returned nil GenerationRun")
	}
	if run.Status != "success" {
		t.Errorf("run.Status = %q, want success", run.Status)
	}
	if !run.FileName.Valid || !strings.HasSuffix(run.FileName.String, ".xml") {
		t.Errorf("run.FileName = %v, want .xml suffix", run.FileName)
	}
	if run.ErrorMessage.Valid {
		t.Errorf("run.ErrorMessage should be NULL, got %q", run.ErrorMessage.String)
	}
	if run.ProtocolID != protocolID {
		t.Errorf("run.ProtocolID = %d, want %d", run.ProtocolID, protocolID)
	}
	if run.Type != "xml" {
		t.Errorf("run.Type = %q, want xml", run.Type)
	}

	// Verify the row is queryable.
	runs, err := env.queries.ListGenerationRunsForProtocol(ctx, protocolID)
	if err != nil {
		t.Fatalf("ListGenerationRunsForProtocol: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("generation_runs rows = %d, want 1", len(runs))
	}
}

// TestGenerateXML_RejectsDraftProtocol verifies that GenerateXML returns
// ErrProtocolNotFixed when called against a draft protocol. The error
// must be wrapped via fmt.Errorf("%w: ...") so callers can branch on
// errors.Is.
func TestGenerateXML_RejectsDraftProtocol(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	now := "2026-06-22T00:00:00Z"
	res, err := env.db.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		"DRAFT-G-"+t.Name(), "Draft Group "+t.Name(), now, now)
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

	raw, run, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if raw != nil {
		t.Errorf("raw should be nil, got %d bytes", len(raw))
	}
	if !errors.Is(err, ErrProtocolNotFixed) {
		t.Fatalf("err = %v, want ErrProtocolNotFixed", err)
	}
	if run == nil {
		t.Fatalf("run should be a failed row so the audit trail records the attempt")
	}
	if run.Status != "failed" {
		t.Errorf("run.Status = %q, want failed", run.Status)
	}
}

// TestGenerateXML_NormalizationIsStable runs generation twice against
// the same fixture and confirms NormalizeXML yields identical bytes.
// This is the property that makes golden tests tractable (no
// millisecond-precision timestamps embedded in the output).
func TestGenerateXML_NormalizationIsStable(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	protocolID, _ := env.seedFixedProtocol(t)
	ctx := context.Background()

	raw1, _, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if err != nil {
		t.Fatalf("first GenerateXML: %v", err)
	}
	raw2, _, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if err != nil {
		t.Fatalf("second GenerateXML: %v", err)
	}
	n1 := NormalizeXML(raw1)
	n2 := NormalizeXML(raw2)
	if string(n1) != string(n2) {
		t.Fatalf("NormalizeXML not stable across runs:\n%s\n---\n%s", n1, n2)
	}
}

// TestGenerateXML_RowIncludesAuditEntry asserts that a generation writes
// at least one action_log entry tagged with the protocol id.
func TestGenerateXML_RowIncludesAuditEntry(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	protocolID, _ := env.seedFixedProtocol(t)
	ctx := context.Background()

	_, _, err := env.svc.generateXMLWith(ctx, env.queries, protocolID)
	if err != nil {
		t.Fatalf("GenerateXML: %v", err)
	}

	logs, err := env.queries.ListActionLogsByEntity(ctx, storagedb.ListActionLogsByEntityParams{
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: protocolID, Valid: true},
	})
	if err != nil {
		t.Fatalf("ListActionLogsByEntity: %v", err)
	}
	hasRequested := false
	hasCompleted := false
	for _, l := range logs {
		if l.Action == "documents.generate.requested" {
			hasRequested = true
		}
		if l.Action == "documents.generate.completed" {
			hasCompleted = true
		}
	}
	if !hasRequested {
		t.Errorf("missing action_log entry for documents.generate.requested; got %d rows", len(logs))
	}
	if !hasCompleted {
		t.Errorf("missing action_log entry for documents.generate.completed; got %d rows", len(logs))
	}
}
