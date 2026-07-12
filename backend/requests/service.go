// Package requests implements the XLSX import staging slice (plan D1).
//
// The package owns the lifecycle of a single client request — from
// upload through row-by-row application into the canonical `workers`,
// `worker_employers`, and `training_records` tables.
//
// Layering:
//   - parser.go: bytes -> []ParsedRow  (XLSX shape only)
//   - normalizer.go: ParsedRow -> NormalizedRow (validation)
//   - state.go: allowed status transitions for request_rows / items
//   - service.go (this file): orchestration, transactions, audit
//   - handler.go: upload/download HTTP adapters
//
// The Service is the only type that touches storage.WithTx; handler.go
// calls into Service methods and stays free of SQL.
package requests

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// ErrValidation is the sentinel returned by CreateRequest when input
// fields fail validation. Field-level details live on the wrapped
// FieldErrors value (compare with errors.As).
var ErrValidation = errors.New("validation")

// FieldErrors maps form field name -> human-readable error message.
// Mirrors the people/employers pattern so existing render code can
// re-use the same render loop.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	if len(e) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e))
	for k, v := range e {
		parts = append(parts, k+": "+v)
	}
	return strings.Join(parts, "; ")
}

func (e FieldErrors) Is(target error) bool { return target == ErrValidation }

// CreateRequestInput is the operator-supplied input to CreateRequest.
// It is intentionally small: a request is always tied to an employer
// (the F1 slice) and a received date. The XLSX itself arrives later
// via ImportRows so a request can exist before the file lands.
type CreateRequestInput struct {
	EmployerID   int64
	ReceivedDate string // RFC3339 or YYYY-MM-DD; normalised by the handler
	Notes        string
	XLSXData     []byte // optional: handler passes the upload bytes here
	XLSXFileName string // optional: used for the imports row audit only
}

// ImportResult is what ImportRows hands back to the caller: the new
// client_request and the rows that landed in the staging table.
type ImportResult struct {
	Request storagedb.ClientRequest
	Rows    []storagedb.RequestRow
}

// ApplyResult is the per-row outcome of ApplyRow. It lets the HTTP
// handler render a partial without re-querying the DB and gives the
// test suite a typed value to assert on.
type ApplyResult struct {
	RequestRow     storagedb.RequestRow
	Worker         storagedb.Worker
	WorkerEmployer storagedb.WorkerEmployer
	TrainingRecord storagedb.TrainingRecord // zero when status=invalid/duplicate
	Created        bool                     // true => new worker/assignment/training_record created
	Duplicate      bool                     // true => at least one item was already active
	Invalid        bool                     // true => row failed validation after re-check
}

// Service is the orchestration layer for the requests slice. Its
// dependency surface (queries + audit) mirrors the other slices so
// the wiring in app/router.go stays uniform.
type Service struct {
	queries *storagedb.Queries
	db      *sql.DB // needed for storage.WithTx; nil in tests that don't apply rows
	audit   *audit.Service
	now     func() time.Time
}

// NewService constructs a Service. db is required for transactional
// methods (ApplyRow); tests that only exercise CreateRequest can pass
// nil and the methods will surface a clear error.
func NewService(q *storagedb.Queries, auditSvc *audit.Service) *Service {
	return &Service{
		queries: q,
		audit:   auditSvc,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// SetDB wires the database handle used for transactions. Called from
// app/router.go (where both queries and *sql.DB are in scope).
func (s *Service) SetDB(db *sql.DB) {
	s.db = db
}

// CreateRequest persists a new client_request row and, if input.XLSXData
// is non-empty, immediately stages its rows via ImportRows in the same
// transaction-free flow (ImportRows opens its own internal inserts).
func (s *Service) CreateRequest(ctx context.Context, in CreateRequestInput) (storagedb.ClientRequest, error) {
	if in.EmployerID <= 0 {
		return storagedb.ClientRequest{}, FieldErrors{"employer_id": "Выберите работодателя."}
	}
	if strings.TrimSpace(in.ReceivedDate) == "" {
		return storagedb.ClientRequest{}, FieldErrors{"received_date": "Укажите дату получения."}
	}
	if _, err := s.queries.GetEmployerByID(ctx, in.EmployerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storagedb.ClientRequest{}, FieldErrors{"employer_id": "Работодатель не найден."}
		}
		return storagedb.ClientRequest{}, fmt.Errorf("get employer: %w", err)
	}

	timestamp := s.timestamp()
	req, err := s.queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID:     in.EmployerID,
		ReceivedDate:   strings.TrimSpace(in.ReceivedDate),
		SourceType:     "xlsx",
		SourceImportID: sql.NullInt64{}, // filled in by ImportRows below
		Status:         "review",
		Notes:          nullableString(strings.TrimSpace(in.Notes)),
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	})
	if err != nil {
		return storagedb.ClientRequest{}, fmt.Errorf("create client_request: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "client_request",
		EntityID:   sql.NullInt64{Int64: req.ID, Valid: true},
		Details: map[string]any{
			"employer_id":   req.EmployerID,
			"received_date": req.ReceivedDate,
		},
	})

	if len(in.XLSXData) > 0 {
		// ImportRows updates req.SourceImportID + req.UpdatedAt, so we
		// don't need a follow-up write here.
		if _, err := s.ImportRows(ctx, req.ID, in.XLSXData, in.XLSXFileName); err != nil {
			return storagedb.ClientRequest{}, err
		}
	}
	return req, nil
}

// ImportRows records an `imports` row for the uploaded XLSX, parses +
// normalizes the file, and stages each parsed row in `request_rows`
// (with one `request_training_item` per program code). The original
// ClientRequest's source_import_id is back-linked so audit/details can
// recover the upload later.
func (s *Service) ImportRows(ctx context.Context, requestID int64, xlsxData []byte, fileName string) (ImportResult, error) {
	parsed, err := ParseXLSX(xlsxData)
	if err != nil {
		return ImportResult{}, fmt.Errorf("parse xlsx: %w", err)
	}

	// Hash the upload so audit/details can prove what bytes were
	// processed; this is the same fingerprint we store on the imports row.
	sum := sha256.Sum256(xlsxData)
	hashHex := hex.EncodeToString(sum[:])

	now := s.timestamp()
	imp, err := s.queries.CreateImport(ctx, storagedb.CreateImportParams{
		SourceType:      "xlsx",
		SourceFileName:  nullableString(fileName),
		SourceSha256:    nullableString(hashHex),
		UploadedByActor: "operator",
		ReceivedAt:      now,
		Status:          "completed",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("create import: %w", err)
	}

	// Back-link the import to the client request.
	if _, err := s.queries.SetClientRequestSourceImport(ctx, storagedb.SetClientRequestSourceImportParams{
		SourceImportID: sql.NullInt64{Int64: imp.ID, Valid: true},
		UpdatedAt:      now,
		ID:             requestID,
	}); err != nil {
		return ImportResult{}, fmt.Errorf("set source_import: %w", err)
	}

	// Keep a verbatim copy of each raw row in import_rows for audit.
	for _, p := range parsed {
		rawJSON := rawRowJSON(p)
		if _, err := s.queries.CreateImportRow(ctx, storagedb.CreateImportRowParams{
			ImportID:  imp.ID,
			RowNumber: p.RowNumber,
			RawData:   rawJSON,
			CreatedAt: now,
		}); err != nil {
			return ImportResult{}, fmt.Errorf("create import_row: %w", err)
		}
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "import",
		EntityType: "client_request",
		EntityID:   sql.NullInt64{Int64: requestID, Valid: true},
		Details: map[string]any{
			"import_id": imp.ID,
			"row_count": len(parsed),
			"sha256":    hashHex,
		},
	})

	// Stage each row.
	out := make([]storagedb.RequestRow, 0, len(parsed))
	for _, p := range parsed {
		row, err := s.stageRow(ctx, requestID, p)
		if err != nil {
			return ImportResult{}, fmt.Errorf("stage row %d: %w", p.RowNumber, err)
		}
		out = append(out, row)
	}

	return ImportResult{
		Request: mustGetClientRequest(ctx, s.queries, requestID),
		Rows:    out,
	}, nil
}

// stageRow inserts a request_row + zero or more request_training_items
// for a single parsed XLSX row. Normalize errors translate into a
// request_row.status='invalid' with error_summary set; the row is still
// stored so the operator can fix it in the UI later.
func (s *Service) stageRow(ctx context.Context, requestID int64, p ParsedRow) (storagedb.RequestRow, error) {
	now := s.timestamp()
	normalized, normErr := NormalizeRow(p)
	rawJSON := rawRowJSON(p)

	status := RowStatusParsed
	var errSummary sql.NullString
	if normErr != nil {
		status = RowStatusInvalid
		errSummary = sql.NullString{String: normErr.Error(), Valid: true}
	}

	row, err := s.queries.CreateRequestRow(ctx, storagedb.CreateRequestRowParams{
		ClientRequestID:  requestID,
		RowNumber:        p.RowNumber,
		RawData:          rawJSON,
		RawFullName:      nullableString(p.RawFullName),
		ParsedLastName:   nullableString(normalized.LastName),
		ParsedFirstName:  nullableString(normalized.FirstName),
		ParsedMiddleName: nullableString(normalized.MiddleName),
		ParsedSnils:      nullableString(normalized.SNILSDigits),
		ParsedEmail:      nullableString(normalized.Email),
		ParsedPosition:   nullableString(normalized.Position),
		Status:           status,
		ErrorSummary:     errSummary,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return storagedb.RequestRow{}, err
	}

	if normErr != nil {
		// No items to create when the row didn't normalize.
		return row, nil
	}

	for _, code := range p.RawPrograms {
		program, err := s.queries.GetProgramByCode(ctx, code)
		itemStatus := ItemStatusValid
		var itemErr sql.NullString
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				itemStatus = ItemStatusInvalid
				itemErr = sql.NullString{String: "unknown program code: " + code, Valid: true}
			} else {
				return storagedb.RequestRow{}, fmt.Errorf("get program by code: %w", err)
			}
		}
		if _, err := s.queries.CreateRequestTrainingItem(ctx, storagedb.CreateRequestTrainingItemParams{
			RequestRowID:     row.ID,
			ProgramID:        programOrZero(program, err),
			Status:           itemStatus,
			ErrorSummary:     itemErr,
			Resolution:       sql.NullString{},
			TrainingRecordID: sql.NullInt64{},
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return storagedb.RequestRow{}, fmt.Errorf("create training item: %w", err)
		}
	}

	return row, nil
}

// ApplyRow is the operator's "commit this staged row" action. It runs
// in a single SQLite transaction so a partial write can never be left
// behind. Outcomes:
//
//   - new worker (snils_normalized not seen before) + new assignment +
//     new training_records: row -> applied, audit recorded.
//   - existing worker, new assignment: row -> applied.
//   - existing active training_record for any program: that item ->
//     duplicate, row -> applied if at least one item was created.
//   - bad email after a re-check: row -> invalid.
//
// All created entities are audit-logged (entity_type carries the
// table name; entity_id the new PK). request_rows.status is moved to
// 'applied' only after all training_records are committed.
func (s *Service) ApplyRow(ctx context.Context, requestRowID int64) (ApplyResult, error) {
	if s.db == nil {
		return ApplyResult{}, errors.New("requests.Service: db handle not configured (call SetDB)")
	}

	row, err := s.queries.GetRequestRow(ctx, requestRowID)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("get request_row: %w", err)
	}
	if !CanTransitionRow(row.Status, RowStatusApplied) {
		return ApplyResult{}, fmt.Errorf("cannot apply row in status %q", row.Status)
	}

	// Defensive re-validation: by the time we apply, the operator may
	// have edited the row, but NormalizeRow gives us the canonical
	// fields. We rebuild from the parsed_* columns so we don't trust
	// the raw_* input.
	if _, err := normalizeFromRow(row); err != nil {
		// Mark invalid and audit, then return.
		s.recordAudit(ctx, audit.RecordInput{
			Action:     "reject",
			EntityType: "request_row",
			EntityID:   sql.NullInt64{Int64: row.ID, Valid: true},
			Details:    map[string]any{"reason": err.Error()},
		})
		invalid, err2 := s.queries.UpdateRequestRowStatus(ctx, storagedb.UpdateRequestRowStatusParams{
			Status:       RowStatusInvalid,
			ErrorSummary: sql.NullString{String: err.Error(), Valid: true},
			UpdatedAt:    s.timestamp(),
			ID:           row.ID,
		})
		if err2 != nil {
			return ApplyResult{}, fmt.Errorf("mark invalid: %w", err2)
		}
		return ApplyResult{RequestRow: invalid, Invalid: true}, nil
	}

	clientReq, err := s.queries.GetClientRequest(ctx, row.ClientRequestID)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("get client_request: %w", err)
	}

	items, err := s.queries.ListRequestTrainingItems(ctx, row.ID)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("list items: %w", err)
	}

	result := ApplyResult{}

	err = storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		q := s.queries.WithTx(tx)

		// 1. Find-or-create worker.
		snils := row.ParsedSnils.String
		worker, err := q.GetWorkerByNormalizedSNILS(ctx, snils)
		workerCreated := false
		if errors.Is(err, sql.ErrNoRows) {
			now := s.timestamp()
			middle := row.ParsedMiddleName
			worker, err = q.CreateWorker(ctx, storagedb.CreateWorkerParams{
				LastName:        row.ParsedLastName.String,
				FirstName:       row.ParsedFirstName.String,
				MiddleName:      middle,
				Snils:           snils,
				SnilsNormalized: snils,
				Email:           row.ParsedEmail.String,
				BirthDate:       sql.NullString{},
				CreatedAt:       now,
				UpdatedAt:       now,
			})
			if err != nil {
				return fmt.Errorf("create worker: %w", err)
			}
			workerCreated = true
		} else if err != nil {
			return fmt.Errorf("get worker by snils: %w", err)
		}

		// 2. Find-or-create active worker_employer.
		assignment, err := q.FindActiveWorkerEmployer(ctx, storagedb.FindActiveWorkerEmployerParams{
			WorkerID:   worker.ID,
			EmployerID: clientReq.EmployerID,
		})
		assignmentCreated := false
		if errors.Is(err, sql.ErrNoRows) {
			position := row.ParsedPosition.String
			if position == "" {
				position = "—"
			}
			now := s.timestamp()
			assignment, err = q.CreateWorkerEmployer(ctx, storagedb.CreateWorkerEmployerParams{
				WorkerID:        worker.ID,
				EmployerID:      clientReq.EmployerID,
				CurrentPosition: position,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
			if err != nil {
				return fmt.Errorf("create worker_employer: %w", err)
			}
			assignmentCreated = true
		} else if err != nil {
			return fmt.Errorf("find worker_employer: %w", err)
		}

		// 3. Walk items: detect duplicate / invalid, otherwise create
		// a training_record and mark the item applied.
		anyCreated := false
		anyDuplicate := false
		var lastTraining storagedb.TrainingRecord

		for _, item := range items {
			if item.ProgramID == 0 {
				// Stays invalid.
				continue
			}
			existing, err := q.FindActiveTrainingRecord(ctx, storagedb.FindActiveTrainingRecordParams{
				WorkerEmployerID: assignment.ID,
				ProgramID:        item.ProgramID,
			})
			if err == nil {
				anyDuplicate = true
				if _, err := q.UpdateRequestTrainingItemStatus(ctx, storagedb.UpdateRequestTrainingItemStatusParams{
					Status:           ItemStatusDuplicate,
					ErrorSummary:     sql.NullString{String: "training_record already active", Valid: true},
					Resolution:       sql.NullString{},
					TrainingRecordID: sql.NullInt64{Int64: existing.ID, Valid: true},
					UpdatedAt:        s.timestamp(),
					ID:               item.ID,
				}); err != nil {
					return fmt.Errorf("mark item duplicate: %w", err)
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("find active training_record: %w", err)
			}

			program, err := q.GetProgram(ctx, item.ProgramID)
			if err != nil {
				return fmt.Errorf("get program: %w", err)
			}
			now := s.timestamp()
			tr, err := q.CreateTrainingRecord(ctx, storagedb.CreateTrainingRecordParams{
				WorkerEmployerID:    assignment.ID,
				ProgramID:           item.ProgramID,
				ClientRequestID:     sql.NullInt64{Int64: clientReq.ID, Valid: true},
				Position:            assignment.CurrentPosition,
				Hours:               program.DefaultHours,
				RequiresMintrudTest: 0,
				MoodleStatus:        "not_required",
				Status:              "active",
				CreatedAt:           now,
				UpdatedAt:           now,
			})
			if err != nil {
				return fmt.Errorf("create training_record: %w", err)
			}
			if _, err := q.UpdateRequestTrainingItemStatus(ctx, storagedb.UpdateRequestTrainingItemStatusParams{
				Status:           ItemStatusApplied,
				ErrorSummary:     sql.NullString{},
				Resolution:       sql.NullString{},
				TrainingRecordID: sql.NullInt64{Int64: tr.ID, Valid: true},
				UpdatedAt:        now,
				ID:               item.ID,
			}); err != nil {
				return fmt.Errorf("mark item applied: %w", err)
			}
			lastTraining = tr
			anyCreated = true
		}

		// 4. Transition the request_row to applied (or keep parsed if
		// every item was a duplicate of an already-active record — the
		// operator still gets credit for reviewing the row, but we
		// distinguish "applied with new record" vs "applied but duplicate
		// only" via ApplyResult.Duplicate).
		newStatus := RowStatusApplied
		rowUpdated, err := q.UpdateRequestRowStatus(ctx, storagedb.UpdateRequestRowStatusParams{
			Status:       newStatus,
			ErrorSummary: sql.NullString{},
			UpdatedAt:    s.timestamp(),
			ID:           row.ID,
		})
		if err != nil {
			return fmt.Errorf("mark row applied: %w", err)
		}

		result.RequestRow = rowUpdated
		result.Worker = worker
		result.WorkerEmployer = assignment
		result.TrainingRecord = lastTraining
		result.Created = workerCreated || assignmentCreated || anyCreated
		result.Duplicate = anyDuplicate
		return nil
	})
	if err != nil {
		return ApplyResult{}, err
	}

	// Audit outside the transaction: a failed audit row must not roll
	// back the actual apply. We log every side-effect that happened.
	s.recordAudit(ctx, audit.RecordInput{
		Action:     "apply",
		EntityType: "request_row",
		EntityID:   sql.NullInt64{Int64: result.RequestRow.ID, Valid: true},
		Details: map[string]any{
			"worker_id":          result.Worker.ID,
			"worker_employer_id": result.WorkerEmployer.ID,
			"duplicate":          result.Duplicate,
			"client_request_id":  clientReq.ID,
		},
	})
	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "worker",
		EntityID:   sql.NullInt64{Int64: result.Worker.ID, Valid: true},
		Actor:      "import",
		Details: map[string]any{
			"snils_normalized": result.Worker.SnilsNormalized,
			"source":           "request_apply",
		},
	})
	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "worker_employer",
		EntityID:   sql.NullInt64{Int64: result.WorkerEmployer.ID, Valid: true},
		Actor:      "import",
		Details: map[string]any{
			"worker_id":   result.WorkerEmployer.WorkerID,
			"employer_id": result.WorkerEmployer.EmployerID,
		},
	})
	if result.TrainingRecord.ID != 0 {
		s.recordAudit(ctx, audit.RecordInput{
			Action:     "create",
			EntityType: "training_record",
			EntityID:   sql.NullInt64{Int64: result.TrainingRecord.ID, Valid: true},
			Actor:      "import",
			Details: map[string]any{
				"worker_employer_id": result.TrainingRecord.WorkerEmployerID,
				"program_id":         result.TrainingRecord.ProgramID,
			},
		})
	}
	if result.Duplicate {
		s.recordAudit(ctx, audit.RecordInput{
			Action:     "reject",
			EntityType: "request_row",
			EntityID:   sql.NullInt64{Int64: result.RequestRow.ID, Valid: true},
			Details:    map[string]any{"reason": "duplicate active training_record"},
		})
	}

	return result, nil
}

// SkipRow transitions a staged row to 'skipped'. Used by the
// /skip POST route.
func (s *Service) SkipRow(ctx context.Context, requestRowID int64) (storagedb.RequestRow, error) {
	row, err := s.queries.GetRequestRow(ctx, requestRowID)
	if err != nil {
		return storagedb.RequestRow{}, fmt.Errorf("get request_row: %w", err)
	}
	if !CanTransitionRow(row.Status, RowStatusSkipped) {
		return storagedb.RequestRow{}, fmt.Errorf("cannot skip row in status %q", row.Status)
	}
	updated, err := s.queries.UpdateRequestRowStatus(ctx, storagedb.UpdateRequestRowStatusParams{
		Status:       RowStatusSkipped,
		ErrorSummary: sql.NullString{},
		UpdatedAt:    s.timestamp(),
		ID:           row.ID,
	})
	if err != nil {
		return storagedb.RequestRow{}, fmt.Errorf("update row status: %w", err)
	}
	s.recordAudit(ctx, audit.RecordInput{
		Action:     "skip",
		EntityType: "request_row",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
	})
	return updated, nil
}

// ListRequests returns all client_requests, optionally filtered by status.
func (s *Service) ListRequests(ctx context.Context, status string) ([]storagedb.ClientRequest, error) {
	if status == "" {
		return s.queries.ListClientRequests(ctx)
	}
	return s.queries.ListClientRequestsByStatus(ctx, status)
}

// GetRequest returns a single client_request by id.
func (s *Service) GetRequest(ctx context.Context, id int64) (storagedb.ClientRequest, error) {
	return s.queries.GetClientRequest(ctx, id)
}

// ListRows returns all request_rows for a given client_request.
func (s *Service) ListRows(ctx context.Context, requestID int64) ([]storagedb.RequestRow, error) {
	return s.queries.ListRequestRows(ctx, requestID)
}

// ListItemsForRow returns the items (with joined program info) for a row.
func (s *Service) ListItemsForRow(ctx context.Context, rowID int64) ([]storagedb.ListRequestTrainingItemsForRowRow, error) {
	return s.queries.ListRequestTrainingItemsForRow(ctx, rowID)
}

// ----- internal helpers -----

func (s *Service) timestamp() string {
	return s.now().Format(time.RFC3339)
}

func (s *Service) recordAudit(ctx context.Context, in audit.RecordInput) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, in)
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// mustGetClientRequest re-reads the request after ImportRows updated its
// source_import_id so the caller sees the final row.
func mustGetClientRequest(ctx context.Context, q *storagedb.Queries, id int64) storagedb.ClientRequest {
	r, err := q.GetClientRequest(ctx, id)
	if err != nil {
		return storagedb.ClientRequest{}
	}
	return r
}

// normalizeFromRow re-runs NormalizeRow against the parsed_* columns
// of a stored request_row. Used by ApplyRow as a final guard.
func normalizeFromRow(row storagedb.RequestRow) (NormalizedRow, error) {
	raw := ParsedRow{
		RowNumber:   row.RowNumber,
		RawFullName: joinFIO(row.ParsedLastName.String, row.ParsedFirstName.String, row.ParsedMiddleName.String),
		RawSNILS:    row.ParsedSnils.String,
		RawEmail:    row.ParsedEmail.String,
		RawPosition: row.ParsedPosition.String,
	}
	return NormalizeRow(raw)
}

func joinFIO(last, first, middle string) string {
	parts := []string{}
	if last != "" {
		parts = append(parts, last)
	}
	if first != "" {
		parts = append(parts, first)
	}
	if middle != "" {
		parts = append(parts, middle)
	}
	return strings.Join(parts, " ")
}

// programOrZero returns the program id when present, else 0, so the
// request_training_item row records a placeholder we can detect later.
func programOrZero(p storagedb.Program, err error) int64 {
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	return p.ID
}

// rawRowJSON is a best-effort JSON-ish serialisation. We deliberately
// avoid encoding/json to keep import_rows cheap; the value is for
// audit/debug only and is never rendered into the UI.
func rawRowJSON(p ParsedRow) string {
	var b strings.Builder
	b.WriteString("{")
	b.WriteString(`"row_number":`)
	b.WriteString(fmt.Sprintf("%d", p.RowNumber))
	b.WriteString(`,"raw_full_name":"`)
	b.WriteString(escapeJSON(p.RawFullName))
	b.WriteString(`","raw_snils":"`)
	b.WriteString(escapeJSON(p.RawSNILS))
	b.WriteString(`","raw_email":"`)
	b.WriteString(escapeJSON(p.RawEmail))
	b.WriteString(`","raw_position":"`)
	b.WriteString(escapeJSON(p.RawPosition))
	b.WriteString(`","raw_programs":[`)
	for i, code := range p.RawPrograms {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"`)
		b.WriteString(escapeJSON(code))
		b.WriteString(`"`)
	}
	b.WriteString("]}")
	return b.String()
}

func escapeJSON(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

// ReadXLSXMaxBytes is the upload cap. Handlers call this via
// http.MaxBytesReader to bound memory use. 10 MB matches Mintrud's
// realistic worst case (a few thousand rows).
const ReadXLSXMaxBytes = 10 * 1024 * 1024

// MimeXLSX is the canonical Content-Type for an XLSX upload.
const MimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// IsAcceptableUploadContentType returns true when the Content-Type
// header looks like an XLSX upload (or a fallback browsers sometimes
// send). We accept application/octet-stream so we don't reject well-
// formed files when curl sends the wrong type.
func IsAcceptableUploadContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return false
	}
	if strings.HasPrefix(ct, MimeXLSX) {
		return true
	}
	if strings.HasPrefix(ct, "application/octet-stream") {
		return true
	}
	if strings.HasPrefix(ct, "application/zip") {
		return true
	}
	return false
}

// MaxBytesResponseWriter is a tiny helper that returns the canonical
// 413 + plain-text body when the cap is hit. Used by the upload
// handler so a too-big file gets a clear operator-visible error.
func MaxBytesResponseWriter(w http.ResponseWriter) {
	http.Error(w, "upload too large (max 10MB)", http.StatusRequestEntityTooLarge)
}
