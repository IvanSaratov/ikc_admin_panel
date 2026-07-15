package requests_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/requests"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/xuri/excelize/v2"
)

// buildTestXLSX is a tiny helper that writes an XLSX with the standard
// Mintrud request header + the supplied data rows. Used everywhere a
// service test needs "an XLSX I can hand to ImportRows".
func buildTestXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Заявка"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	headers := []string{"ФИО", "СНИЛС", "Email", "Должность", "Коды программ"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

// newService opens a fresh SQLite db, applies migrations, and returns
// a fully-wired requests.Service. Mirrors backend/employers.newService.
func newService(t *testing.T) (*requests.Service, *storagedb.Queries) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "ikc-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	auditSvc.SetClock(func() time.Time { return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC) })
	svc := requests.NewService(queries, auditSvc)
	svc.SetDB(database)
	return svc, queries
}

// seedEmployerAndProgram inserts an employer (canonical, with INN) and
// one program (A-1) so ApplyRow has somewhere to point.
func seedEmployerAndProgram(t *testing.T, queries *storagedb.Queries) (int64, int64) {
	t.Helper()
	now := "2026-06-22T12:00:00Z"
	emp, err := queries.CreateEmployer(context.Background(), storagedb.CreateEmployerParams{
		Inn: "7700000000", InnNormalized: "7700000000", CanonicalName: "ООО Ромашка",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed employer: %v", err)
	}
	group, err := queries.CreateProgramGroup(context.Background(), storagedb.CreateProgramGroupParams{
		Code: "G1", Name: "Group 1", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed program_group: %v", err)
	}
	prog, err := queries.CreateProgram(context.Background(), storagedb.CreateProgramParams{
		ProgramGroupID: group.ID, Code: "A-1", Name: "Program A-1", DefaultHours: 16,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed program: %v", err)
	}
	return emp.ID, prog.ID
}

// validRow returns a single-row XLSX bytes for the supplied FIO/SNILS/etc.
// Program defaults to A-1.
func validRow(fio, snils, email, position string) [][]string {
	return [][]string{
		{fio, snils, email, position, "A-1"},
	}
}

func TestCreateRequest_Persists(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{
		EmployerID:   empID,
		ReceivedDate: "2026-06-15",
		Notes:        "manual test",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if req.ID == 0 {
		t.Errorf("request id = 0")
	}
	if req.Status != "review" {
		t.Errorf("status = %q, want review", req.Status)
	}
	if req.EmployerID != empID {
		t.Errorf("employer_id = %d, want %d", req.EmployerID, empID)
	}

	// Audit entry was recorded.
	logs, err := q.ListActionLogsByEntity(ctx, storagedb.ListActionLogsByEntityParams{
		EntityType: "client_request",
		EntityID:   sql.NullInt64{Int64: req.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(logs) == 0 {
		t.Errorf("expected at least one audit row for create")
	}
}

func TestCreateRequest_UnknownEmployer_Errors(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	_, err := svc.CreateRequest(context.Background(), requests.CreateRequestInput{
		EmployerID:   99999,
		ReceivedDate: "2026-06-15",
	})
	var fe requests.FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if fe["employer_id"] == "" {
		t.Errorf("expected employer_id field error, got %v", fe)
	}
}

func TestImportRows_CreatesRequestRowsAndItems(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, progID := seedEmployerAndProgram(t, q)

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{
		EmployerID: empID, ReceivedDate: "2026-06-15",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	xlsx := buildTestXLSX(t, [][]string{
		{"Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер", "A-1"},
		{"Петров Петр", "98765432100", "petrov@example.com", "Менеджер", "A-1"},
	})
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "test.xlsx")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(res.Rows))
	}
	if res.Rows[0].Status != requests.RowStatusParsed {
		t.Errorf("row 0 status = %q, want parsed", res.Rows[0].Status)
	}

	items, err := q.ListRequestTrainingItems(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].ProgramID != progID {
		t.Errorf("program_id = %d, want %d", items[0].ProgramID, progID)
	}
	if items[0].Status != requests.ItemStatusValid {
		t.Errorf("item status = %q, want valid", items[0].Status)
	}

	// source_import_id was back-linked.
	got, err := q.GetClientRequest(ctx, req.ID)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if !got.SourceImportID.Valid {
		t.Errorf("source_import_id was not set after ImportRows")
	}
	importRows, err := q.ListImportRows(ctx, got.SourceImportID.Int64)
	if err != nil {
		t.Fatalf("list import rows: %v", err)
	}
	if len(importRows) != 2 {
		t.Fatalf("import rows = %d, want 2", len(importRows))
	}
	if importRows[0].SheetName != "Заявка" || importRows[1].SheetName != "Заявка" {
		t.Fatalf("import row sheets = %q, %q; want Заявка", importRows[0].SheetName, importRows[1].SheetName)
	}
}

func TestImportRows_InvalidRowStagedAsInvalid(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)
	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Bad SNILS (only 5 digits) + bad email + single-name FIO.
	xlsx := buildTestXLSX(t, [][]string{
		{"Одиночка", "12345", "not-an-email", "Тест", "A-1"},
	})
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(res.Rows))
	}
	if res.Rows[0].Status != requests.RowStatusInvalid {
		t.Errorf("status = %q, want invalid", res.Rows[0].Status)
	}
	if !res.Rows[0].ErrorSummary.Valid {
		t.Errorf("error_summary not set on invalid row")
	}
}

func TestApplyRow_NewWorker_NewAssignment_NewTrainingRecord(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	xlsx := buildTestXLSX(t, validRow("Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер"))
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	r, err := svc.ApplyRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if r.Worker.ID == 0 {
		t.Errorf("worker id = 0")
	}
	if r.WorkerEmployer.ID == 0 {
		t.Errorf("assignment id = 0")
	}
	if r.TrainingRecord.ID == 0 {
		t.Errorf("training_record id = 0")
	}
	if r.RequestRow.Status != requests.RowStatusApplied {
		t.Errorf("row status = %q, want applied", r.RequestRow.Status)
	}
	if !r.Created {
		t.Errorf("Created = false, want true on first apply")
	}
	if r.Duplicate {
		t.Errorf("Duplicate = true on fresh apply")
	}

	// Verify in DB.
	w, err := q.GetWorkerByNormalizedSNILS(ctx, "12345678900")
	if err != nil {
		t.Fatalf("get worker by snils: %v", err)
	}
	if w.ID != r.Worker.ID {
		t.Errorf("worker id mismatch")
	}
	tr, err := q.GetTrainingRecord(ctx, r.TrainingRecord.ID)
	if err != nil {
		t.Fatalf("get training record: %v", err)
	}
	if tr.Status != "active" {
		t.Errorf("training_record status = %q, want active", tr.Status)
	}
}

func TestApplyRow_ExistingWorker_NewAssignment(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)

	// Pre-create the worker via the employers/people services so
	// GetWorkerByNormalizedSNILS finds it on apply.
	now := "2026-06-22T12:00:00Z"
	existing, err := q.CreateWorker(ctx, storagedb.CreateWorkerParams{
		LastName: "Иванов", FirstName: "Иван", MiddleName: sql.NullString{},
		Snils: "12345678900", SnilsNormalized: "12345678900",
		Email: "ivanov@example.com", BirthDate: sql.NullString{},
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("pre-create worker: %v", err)
	}

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	xlsx := buildTestXLSX(t, validRow("Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер"))
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	r, err := svc.ApplyRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if r.Worker.ID != existing.ID {
		t.Errorf("worker.id = %d, want %d (existing)", r.Worker.ID, existing.ID)
	}
	if r.WorkerEmployer.WorkerID != existing.ID {
		t.Errorf("assignment worker_id = %d, want %d", r.WorkerEmployer.WorkerID, existing.ID)
	}
	if r.TrainingRecord.ID == 0 {
		t.Errorf("training_record not created")
	}
}

func TestApplyRow_DuplicateTrainingRecord_MarksDuplicate(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, progID := seedEmployerAndProgram(t, q)

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	xlsx := buildTestXLSX(t, validRow("Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер"))
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	first, err := svc.ApplyRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if first.TrainingRecord.ID == 0 {
		t.Fatalf("first apply produced no training_record")
	}

	// Manually mark the row back to pending so we can re-apply.
	if _, err := q.UpdateRequestRowStatus(ctx, storagedb.UpdateRequestRowStatusParams{
		Status: requests.RowStatusParsed, ErrorSummary: sql.NullString{},
		UpdatedAt: nowStr(), ID: res.Rows[0].ID,
	}); err != nil {
		t.Fatalf("reset row: %v", err)
	}

	r, err := svc.ApplyRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !r.Duplicate {
		t.Errorf("Duplicate = false, want true")
	}
	if r.RequestRow.Status != requests.RowStatusApplied {
		t.Errorf("row status = %q, want applied", r.RequestRow.Status)
	}

	// Item was marked duplicate.
	items, err := q.ListRequestTrainingItems(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].Status != requests.ItemStatusDuplicate {
		t.Errorf("item status = %q, want duplicate", items[0].Status)
	}

	_ = progID
}

func TestApplyRow_InvalidEmail_MarksInvalid(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)

	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Sanitize the email at the DB level: stage with a valid email,
	// then mutate the row to a bad one. ApplyRow re-validates and
	// should mark the row invalid.
	xlsx := buildTestXLSX(t, validRow("Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер"))
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := q.UpdateRequestRowParsed(ctx, storagedb.UpdateRequestRowParsedParams{
		RawFullName:      sql.NullString{String: "Иванов Иван Иванович", Valid: true},
		ParsedLastName:   sql.NullString{String: "Иванов", Valid: true},
		ParsedFirstName:  sql.NullString{String: "Иван", Valid: true},
		ParsedMiddleName: sql.NullString{String: "Иванович", Valid: true},
		ParsedSnils:      sql.NullString{String: "12345678900", Valid: true},
		ParsedEmail:      sql.NullString{String: "totally-not-an-email", Valid: true},
		ParsedPosition:   sql.NullString{String: "Инженер", Valid: true},
		Status:           requests.RowStatusParsed,
		ErrorSummary:     sql.NullString{},
		UpdatedAt:        nowStr(),
		ID:               res.Rows[0].ID,
	}); err != nil {
		t.Fatalf("mutate row: %v", err)
	}

	r, err := svc.ApplyRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !r.Invalid {
		t.Errorf("Invalid = false, want true")
	}
	if r.RequestRow.Status != requests.RowStatusInvalid {
		t.Errorf("row status = %q, want invalid", r.RequestRow.Status)
	}
}

func TestSkipRow_TransitionsStatus(t *testing.T) {
	t.Parallel()
	svc, q := newService(t)
	ctx := context.Background()
	empID, _ := seedEmployerAndProgram(t, q)
	req, err := svc.CreateRequest(ctx, requests.CreateRequestInput{EmployerID: empID, ReceivedDate: "2026-06-15"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	xlsx := buildTestXLSX(t, validRow("Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер"))
	res, err := svc.ImportRows(ctx, req.ID, xlsx, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	r, err := svc.SkipRow(ctx, res.Rows[0].ID)
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if r.Status != requests.RowStatusSkipped {
		t.Errorf("status = %q, want skipped", r.Status)
	}
}

// nowStr returns the same fixed timestamp the service uses.
func nowStr() string { return "2026-06-22T12:00:00Z" }

// guard against unused-import warnings if a future edit removes one.
var _ = employers.NewService
