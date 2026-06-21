package people

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

var ErrValidation = errors.New("validation")

type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	values := make([]string, 0, len(e))
	for _, value := range e {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, " ")
}

func (e FieldErrors) Is(target error) bool { return target == ErrValidation }

type WorkerForm struct {
	LastName   string
	FirstName  string
	MiddleName string
	SNILS      string
	Email      string
	BirthDate  string
}

type AssignmentForm struct {
	WorkerID        int64
	EmployerID      int64
	CurrentPosition string
}

type Service struct {
	queries *storagedb.Queries
	audit   *audit.Service
	now     func() time.Time
}

// NewService constructs a people.Service. The audit dependency is optional;
// pass nil in tests that exercise validation only.
func NewService(queries *storagedb.Queries, auditSvc *audit.Service) *Service {
	return &Service{
		queries: queries,
		audit:   auditSvc,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ListWorkers(ctx context.Context) ([]storagedb.Worker, error) {
	return s.queries.ListWorkers(ctx)
}

func (s *Service) GetWorker(ctx context.Context, id int64) (storagedb.Worker, error) {
	return s.queries.GetWorkerByID(ctx, id)
}

func (s *Service) ListAssignments(ctx context.Context, workerID int64) ([]storagedb.WorkerEmployer, error) {
	return s.queries.ListWorkerEmployersForWorker(ctx, workerID)
}

func (s *Service) GetAssignment(ctx context.Context, id int64) (storagedb.WorkerEmployer, error) {
	return s.queries.GetWorkerEmployer(ctx, id)
}

func (s *Service) CreateWorker(ctx context.Context, form WorkerForm) (storagedb.Worker, error) {
	form.LastName = strings.TrimSpace(form.LastName)
	form.FirstName = strings.TrimSpace(form.FirstName)
	form.MiddleName = strings.TrimSpace(form.MiddleName)
	form.SNILS = strings.TrimSpace(form.SNILS)
	form.Email = strings.TrimSpace(form.Email)
	form.BirthDate = strings.TrimSpace(form.BirthDate)
	normalizedSNILS := normalizeDigits(form.SNILS)

	errs := FieldErrors{}
	if form.LastName == "" {
		errs["last_name"] = "Укажите фамилию."
	}
	if form.FirstName == "" {
		errs["first_name"] = "Укажите имя."
	}
	if normalizedSNILS == "" {
		errs["snils"] = "Укажите СНИЛС."
	}
	if form.Email == "" {
		errs["email"] = "Укажите email."
	}
	if len(errs) > 0 {
		return storagedb.Worker{}, errs
	}

	timestamp := s.timestamp()
	worker, err := s.queries.CreateWorker(ctx, storagedb.CreateWorkerParams{
		LastName:        form.LastName,
		FirstName:       form.FirstName,
		MiddleName:      nullableString(form.MiddleName),
		Snils:           form.SNILS,
		SnilsNormalized: normalizedSNILS,
		Email:           form.Email,
		BirthDate:       nullableString(form.BirthDate),
		CreatedAt:       timestamp,
		UpdatedAt:       timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Worker{}, FieldErrors{"snils": "Слушатель с таким СНИЛС уже существует."}
		}
		return storagedb.Worker{}, fmt.Errorf("create worker: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "worker",
		EntityID:   sql.NullInt64{Int64: worker.ID, Valid: true},
		Details: map[string]any{
			"last_name":  worker.LastName,
			"first_name": worker.FirstName,
			"snils":      worker.SnilsNormalized,
			"email":      worker.Email,
		},
	})
	return worker, nil
}

func (s *Service) UpdateWorker(ctx context.Context, id int64, form WorkerForm) (storagedb.Worker, error) {
	form.LastName = strings.TrimSpace(form.LastName)
	form.FirstName = strings.TrimSpace(form.FirstName)
	form.MiddleName = strings.TrimSpace(form.MiddleName)
	form.SNILS = strings.TrimSpace(form.SNILS)
	form.Email = strings.TrimSpace(form.Email)
	form.BirthDate = strings.TrimSpace(form.BirthDate)
	normalizedSNILS := normalizeDigits(form.SNILS)

	errs := FieldErrors{}
	if form.LastName == "" {
		errs["last_name"] = "Укажите фамилию."
	}
	if form.FirstName == "" {
		errs["first_name"] = "Укажите имя."
	}
	if normalizedSNILS == "" {
		errs["snils"] = "Укажите СНИЛС."
	}
	if form.Email == "" {
		errs["email"] = "Укажите email."
	}
	if len(errs) > 0 {
		return storagedb.Worker{}, errs
	}

	previous, err := s.queries.GetWorkerByID(ctx, id)
	if err != nil {
		return storagedb.Worker{}, fmt.Errorf("get worker: %w", err)
	}

	updated, err := s.queries.UpdateWorker(ctx, storagedb.UpdateWorkerParams{
		LastName:        form.LastName,
		FirstName:       form.FirstName,
		MiddleName:      nullableString(form.MiddleName),
		Snils:           form.SNILS,
		SnilsNormalized: normalizedSNILS,
		Email:           form.Email,
		BirthDate:       nullableString(form.BirthDate),
		UpdatedAt:       s.timestamp(),
		ID:              id,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Worker{}, FieldErrors{"snils": "Слушатель с таким СНИЛС уже существует."}
		}
		return storagedb.Worker{}, fmt.Errorf("update worker: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "worker",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": workerSnapshot(previous),
			"to":   workerSnapshot(updated),
		},
	})
	return updated, nil
}

func (s *Service) AssignEmployer(ctx context.Context, form AssignmentForm) (storagedb.WorkerEmployer, error) {
	form.CurrentPosition = strings.TrimSpace(form.CurrentPosition)

	errs := FieldErrors{}
	if form.WorkerID <= 0 {
		errs["worker_id"] = "Выберите слушателя."
	}
	if form.EmployerID <= 0 {
		errs["employer_id"] = "Выберите работодателя."
	}
	if form.CurrentPosition == "" {
		errs["current_position"] = "Укажите должность."
	}
	if len(errs) > 0 {
		return storagedb.WorkerEmployer{}, errs
	}

	timestamp := s.timestamp()
	assignment, err := s.queries.CreateWorkerEmployer(ctx, storagedb.CreateWorkerEmployerParams{
		WorkerID:        form.WorkerID,
		EmployerID:      form.EmployerID,
		CurrentPosition: form.CurrentPosition,
		CreatedAt:       timestamp,
		UpdatedAt:       timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.WorkerEmployer{}, FieldErrors{"employer_id": "У слушателя уже есть активная связь с этим работодателем."}
		}
		return storagedb.WorkerEmployer{}, fmt.Errorf("assign employer: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "worker_employer",
		EntityID:   sql.NullInt64{Int64: assignment.ID, Valid: true},
		Details: map[string]any{
			"worker_id":        assignment.WorkerID,
			"employer_id":      assignment.EmployerID,
			"current_position": assignment.CurrentPosition,
		},
	})
	return assignment, nil
}

func (s *Service) UpdateAssignment(ctx context.Context, id int64, form AssignmentForm) (storagedb.WorkerEmployer, error) {
	form.CurrentPosition = strings.TrimSpace(form.CurrentPosition)

	errs := FieldErrors{}
	if form.EmployerID <= 0 {
		errs["employer_id"] = "Выберите работодателя."
	}
	if form.CurrentPosition == "" {
		errs["current_position"] = "Укажите должность."
	}
	if len(errs) > 0 {
		return storagedb.WorkerEmployer{}, errs
	}

	previous, err := s.queries.GetWorkerEmployer(ctx, id)
	if err != nil {
		return storagedb.WorkerEmployer{}, fmt.Errorf("get assignment: %w", err)
	}

	updated, err := s.queries.UpdateAssignment(ctx, storagedb.UpdateAssignmentParams{
		EmployerID:      form.EmployerID,
		CurrentPosition: form.CurrentPosition,
		UpdatedAt:       s.timestamp(),
		ID:              id,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.WorkerEmployer{}, FieldErrors{"employer_id": "У слушателя уже есть активная связь с этим работодателем."}
		}
		return storagedb.WorkerEmployer{}, fmt.Errorf("update assignment: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "worker_employer",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": assignmentSnapshot(previous),
			"to":   assignmentSnapshot(updated),
		},
	})
	return updated, nil
}

func (s *Service) DeactivateAssignment(ctx context.Context, id int64) (storagedb.WorkerEmployer, error) {
	previous, err := s.queries.GetWorkerEmployer(ctx, id)
	if err != nil {
		return storagedb.WorkerEmployer{}, fmt.Errorf("get assignment: %w", err)
	}
	if previous.Status == "inactive" {
		// Idempotent: no audit row when nothing changed.
		return previous, nil
	}

	updated, err := s.queries.DeactivateAssignment(ctx, storagedb.DeactivateAssignmentParams{
		UpdatedAt: s.timestamp(),
		ID:        id,
	})
	if err != nil {
		return storagedb.WorkerEmployer{}, fmt.Errorf("deactivate assignment: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "deactivate",
		EntityType: "worker_employer",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": previous.Status,
			"to":   updated.Status,
		},
	})
	return updated, nil
}

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

func normalizeDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func workerSnapshot(w storagedb.Worker) map[string]any {
	middle := ""
	if w.MiddleName.Valid {
		middle = w.MiddleName.String
	}
	birth := ""
	if w.BirthDate.Valid {
		birth = w.BirthDate.String
	}
	return map[string]any{
		"last_name":        w.LastName,
		"first_name":       w.FirstName,
		"middle_name":      middle,
		"snils_normalized": w.SnilsNormalized,
		"email":            w.Email,
		"birth_date":       birth,
	}
}

func assignmentSnapshot(a storagedb.WorkerEmployer) map[string]any {
	return map[string]any{
		"worker_id":        a.WorkerID,
		"employer_id":      a.EmployerID,
		"current_position": a.CurrentPosition,
		"status":           a.Status,
	}
}
