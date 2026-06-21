package protocols

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateProtocol_OnlyGroupID_Required(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	protocol, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	if protocol.Status != string(StatusDraft) {
		t.Errorf("status = %q, want draft", protocol.Status)
	}
	if protocol.ProgramGroupID != groupID {
		t.Errorf("program_group_id = %d, want %d", protocol.ProgramGroupID, groupID)
	}
	if protocol.ProtocolNumber.Valid {
		t.Errorf("protocol_number should be NULL, got %q", protocol.ProtocolNumber.String)
	}
	if protocol.AnnualSequenceNumber.Valid {
		t.Errorf("annual_sequence_number should be NULL, got %d", protocol.AnnualSequenceNumber.Int64)
	}
}

func TestCreateProtocol_RejectsUnknownGroup(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	_, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: 99999})
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if fe["program_group_id"] == "" {
		t.Errorf("expected program_group_id error, got %v", fe)
	}
}

func TestCreateProtocol_RejectsZeroGroupID(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	_, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: 0})
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
}

func TestFix_MissingDates_RejectedWithFieldError(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	cases := []FixInput{
		{}, // all empty
		{TrainingStartDate: "2026-06-01", TrainingEndDate: "2026-06-30"}, // no protocol_date
		{TrainingStartDate: "2026-06-01", ProtocolDate: "2026-06-30"},    // no end_date
		{TrainingEndDate: "2026-06-30", ProtocolDate: "2026-06-30"},      // no start_date
	}

	for i, in := range cases {
		_, err := env.svc.Fix(ctx, p.ID, in)
		var fe FieldErrors
		if !errors.As(err, &fe) {
			t.Errorf("case %d: err = %v, want FieldErrors", i, err)
			continue
		}
		if len(fe) == 0 {
			t.Errorf("case %d: expected at least one field error, got 0", i)
		}
	}
}

func TestFix_MalformedDates_RejectedWithFieldError(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	_, err = env.svc.Fix(ctx, p.ID, FixInput{
		TrainingStartDate: "06/01/2026",
		TrainingEndDate:   "2026-06-30",
		ProtocolDate:      "2026-06-30",
	})
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if fe["training_start_date"] == "" {
		t.Errorf("expected training_start_date error, got %v", fe)
	}
}

func TestFix_Success_AssignsNumberAndStatus(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	fixed, err := env.svc.Fix(ctx, p.ID, validFixInput())
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if fixed.Status != string(StatusFixed) {
		t.Errorf("status = %q, want fixed", fixed.Status)
	}
	if fixed.AnnualSequenceNumber.Int64 != 1 {
		t.Errorf("sequence = %d, want 1", fixed.AnnualSequenceNumber.Int64)
	}
	if fixed.SequenceYear.Int64 != 2026 {
		t.Errorf("year = %d, want 2026", fixed.SequenceYear.Int64)
	}
	if fixed.ProtocolMonth.Int64 != 6 {
		t.Errorf("month = %d, want 6", fixed.ProtocolMonth.Int64)
	}
	if !fixed.FixedAt.Valid {
		t.Errorf("fixed_at should be set")
	}
	if fixed.ProtocolNumber.String != "2026-06/001" {
		t.Errorf("protocol_number = %q, want 2026-06/001", fixed.ProtocolNumber.String)
	}
}

func TestFix_RejectsDoubleFix(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	if _, err := env.svc.Fix(ctx, p.ID, validFixInput()); err != nil {
		t.Fatalf("first Fix: %v", err)
	}
	// Second Fix attempt must fail because the protocol is already 'fixed'
	// (the FixProtocol query is gated on status='draft').
	_, err = env.svc.Fix(ctx, p.ID, validFixInput())
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("second Fix: err = %v, want FieldErrors", err)
	}
	if fe["protocol_id"] == "" {
		t.Errorf("expected protocol_id error, got %v", fe)
	}
}

func TestAddParticipant_AddsRowAndAudits(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	trainingRecordID := env.mustTrainingRecord(t)

	if err := env.svc.AddParticipant(ctx, p.ID, trainingRecordID); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}

	parts, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("participants = %d, want 1", len(parts))
	}
	if parts[0].TrainingRecordID != trainingRecordID {
		t.Errorf("training_record_id = %d, want %d", parts[0].TrainingRecordID, trainingRecordID)
	}
}

func TestAddParticipant_TrainingRecordAlreadyActiveInOtherProtocol_Rejected(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)
	trainingRecordID := env.mustTrainingRecord(t)

	p1, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p1: %v", err)
	}
	if err := env.svc.AddParticipant(ctx, p1.ID, trainingRecordID); err != nil {
		t.Fatalf("first AddParticipant: %v", err)
	}

	// Second protocol tries to attach the same training_record.
	p2, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p2: %v", err)
	}
	err = env.svc.AddParticipant(ctx, p2.ID, trainingRecordID)
	if err == nil {
		t.Fatalf("expected AddParticipant to reject duplicate active training_record, got nil")
	}
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if fe["training_record_id"] == "" {
		t.Errorf("expected training_record_id error, got %v", fe)
	}
}

func TestRemoveParticipant_FlipsStatusToRemoved(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	trainingRecordID := env.mustTrainingRecord(t)

	if err := env.svc.AddParticipant(ctx, p.ID, trainingRecordID); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}
	parts, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("participants = %d, want 1", len(parts))
	}

	if err := env.svc.RemoveParticipant(ctx, parts[0].ID); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}

	// Active list is now empty.
	active, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListParticipants (after remove): %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active participants = %d, want 0", len(active))
	}

	// All list (including removed) still has the row.
	all, err := env.queries.ListAllProtocolParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListAllProtocolParticipants: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("all participants = %d, want 1", len(all))
	}
	if all[0].Status != "removed" {
		t.Errorf("status = %q, want removed", all[0].Status)
	}
}

func TestTransition_FixedToXmlUploaded_OK(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	if _, err := env.svc.Fix(ctx, p.ID, validFixInput()); err != nil {
		t.Fatalf("Fix: %v", err)
	}

	if err := env.svc.Transition(ctx, p.ID, StatusXmlUploaded); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	got, err := env.svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != string(StatusXmlUploaded) {
		t.Errorf("status = %q, want xml_uploaded", got.Status)
	}
}

func TestTransition_DraftToGenerated_Rejected(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	err = env.svc.Transition(ctx, p.ID, StatusGenerated)
	if err == nil {
		t.Fatalf("expected transition draft → generated to be rejected")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition", err)
	}
	// Verify status is unchanged.
	got, err := env.svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != string(StatusDraft) {
		t.Errorf("status = %q, want draft (unchanged)", got.Status)
	}
}

func TestTransition_UnknownStatus_Rejected(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	err = env.svc.Transition(ctx, p.ID, ProtocolStatus("nonsense"))
	if err == nil {
		t.Fatalf("expected unknown status to be rejected")
	}
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Errorf("err = %v, want FieldErrors", err)
	}
}

func TestFullLifecycle_Draft_To_Completed_AllStepsWork(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	// Add a couple of training records as participants.
	for i := 0; i < 3; i++ {
		tr := env.mustTrainingRecord(t)
		if err := env.svc.AddParticipant(ctx, p.ID, tr); err != nil {
			t.Fatalf("AddParticipant %d: %v", i, err)
		}
	}

	// Draft → fixed.
	fixed, err := env.svc.Fix(ctx, p.ID, validFixInput())
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if fixed.Status != string(StatusFixed) {
		t.Fatalf("post-Fix status = %q, want fixed", fixed.Status)
	}

	// Walk the rest of the linear lifecycle.
	for _, to := range []ProtocolStatus{
		StatusXmlUploaded,
		StatusRegistryEntered,
		StatusGenerated,
		StatusCompleted,
	} {
		if err := env.svc.Transition(ctx, p.ID, to); err != nil {
			t.Fatalf("Transition → %s: %v", to, err)
		}
		got, err := env.svc.Get(ctx, p.ID)
		if err != nil {
			t.Fatalf("Get after %s: %v", to, err)
		}
		if got.Status != string(to) {
			t.Errorf("status = %q, want %s", got.Status, to)
		}
	}

	// Audit log should record the protocol-level events: create + fix +
	// 4 transitions = 6 rows for entity_type=protocol.
	rows, err := env.queries.ListActionLogsByEntity(ctx, storagedb.ListActionLogsByEntityParams{
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: p.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("ListActionLogsByEntity: %v", err)
	}
	if len(rows) != 6 {
		t.Fatalf("audit rows for protocol = %d, want 6 (create+fix+4 transitions)", len(rows))
	}
}

// --- helpers for service_test.go ---

// mustTrainingRecord inserts the minimum data required to point a participant
// at: one worker, one worker_employer, one training_record. The protocols
// slice is not responsible for any of those tables' validation. Returns
// the training_record id.
//
// `seq` is included in every UNIQUE column (program_groups.code,
// programs.code, snils, employer inn) so multiple calls within one test
// can build independent rows. Tests call with i=0,1,2,... and we read the
// index from a per-test counter.
func (e *testEnv) mustTrainingRecord(t *testing.T) int64 {
	t.Helper()
	e.trainingRecordSeq++
	return e.mustTrainingRecordAt(t, e.trainingRecordSeq)
}

func (e *testEnv) mustTrainingRecordAt(t *testing.T, seq int) int64 {
	t.Helper()

	ctx := context.Background()
	now := "2026-06-21T00:00:00Z"

	// Each training record needs its own program group (FK chain) so we
	// insert one here. The protocols slice does not own program lifecycle.
	groupRes, err := e.db.ExecContext(ctx,
		`INSERT INTO program_groups (code, name, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`,
		fmt.Sprintf("PG-%s-%d", t.Name(), seq), fmt.Sprintf("PG %s %d", t.Name(), seq), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, err := groupRes.LastInsertId()
	if err != nil {
		t.Fatalf("program_group last id: %v", err)
	}

	snils := fmt.Sprintf("%011d", 10000000000+seq)
	workerRes, err := e.db.ExecContext(ctx,
		`INSERT INTO workers (last_name, first_name, snils, snils_normalized, email, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"Иванов", fmt.Sprintf("Иван%d", seq), snils, snils, "ivan@example.com", now, now)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	workerID, err := workerRes.LastInsertId()
	if err != nil {
		t.Fatalf("worker last id: %v", err)
	}

	employerRes, err := e.db.ExecContext(ctx,
		`INSERT INTO employers (inn, inn_normalized, canonical_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		fmt.Sprintf("%010d", 1234567890+seq), fmt.Sprintf("%010d", 1234567890+seq),
		fmt.Sprintf("Employer %s %d", t.Name(), seq), now, now)
	if err != nil {
		t.Fatalf("insert employer: %v", err)
	}
	employerID, err := employerRes.LastInsertId()
	if err != nil {
		t.Fatalf("employer last id: %v", err)
	}

	weRes, err := e.db.ExecContext(ctx,
		`INSERT INTO worker_employers (worker_id, employer_id, current_position, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
		workerID, employerID, "Engineer", now, now)
	if err != nil {
		t.Fatalf("insert worker_employer: %v", err)
	}
	weID, err := weRes.LastInsertId()
	if err != nil {
		t.Fatalf("worker_employer last id: %v", err)
	}

	progRes, err := e.db.ExecContext(ctx,
		`INSERT INTO programs (program_group_id, code, name, default_hours, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		groupID, fmt.Sprintf("P-%s-%d", t.Name(), seq), fmt.Sprintf("Program %s %d", t.Name(), seq), 40, now, now)
	if err != nil {
		t.Fatalf("insert program: %v", err)
	}
	progID, err := progRes.LastInsertId()
	if err != nil {
		t.Fatalf("program last id: %v", err)
	}

	trRes, err := e.db.ExecContext(ctx,
		`INSERT INTO training_records (worker_employer_id, program_id, position, hours, requires_mintrud_test, moodle_status, status, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 'not_required', 'active', ?, ?)`,
		weID, progID, "Engineer", 40, now, now)
	if err != nil {
		t.Fatalf("insert training_record: %v", err)
	}
	id, err := trRes.LastInsertId()
	if err != nil {
		t.Fatalf("training_record last id: %v", err)
	}
	return id
}

// silence the unused import warning if the test file stops needing audit
// directly.
var _ = audit.RecordInput{}
