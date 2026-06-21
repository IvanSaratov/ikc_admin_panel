package employers

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

type Form struct {
	INN           string
	CanonicalName string
}

type Service struct {
	queries *storagedb.Queries
	audit   *audit.Service
	now     func() time.Time
}

// NewService constructs an employers.Service. The audit dependency is
// optional; pass nil in tests that exercise validation only.
func NewService(queries *storagedb.Queries, auditSvc *audit.Service) *Service {
	return &Service{
		queries: queries,
		audit:   auditSvc,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) List(ctx context.Context) ([]storagedb.Employer, error) {
	return s.queries.ListEmployers(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (storagedb.Employer, error) {
	return s.queries.GetEmployerByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, form Form) (storagedb.Employer, error) {
	form.INN = strings.TrimSpace(form.INN)
	form.CanonicalName = strings.TrimSpace(form.CanonicalName)
	normalizedINN := normalizeDigits(form.INN)

	errs := FieldErrors{}
	if normalizedINN == "" {
		errs["inn"] = "Укажите ИНН."
	}
	if form.CanonicalName == "" {
		errs["canonical_name"] = "Укажите название работодателя."
	}
	if len(errs) > 0 {
		return storagedb.Employer{}, errs
	}

	timestamp := s.timestamp()
	employer, err := s.queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn:           form.INN,
		InnNormalized: normalizedINN,
		CanonicalName: form.CanonicalName,
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Employer{}, FieldErrors{"inn": "Работодатель с таким ИНН уже существует."}
		}
		return storagedb.Employer{}, fmt.Errorf("create employer: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "employer",
		EntityID:   sql.NullInt64{Int64: employer.ID, Valid: true},
		Details: map[string]any{
			"inn":            employer.Inn,
			"canonical_name": employer.CanonicalName,
		},
	})
	return employer, nil
}

func (s *Service) Update(ctx context.Context, id int64, form Form) (storagedb.Employer, error) {
	form.INN = strings.TrimSpace(form.INN)
	form.CanonicalName = strings.TrimSpace(form.CanonicalName)
	normalizedINN := normalizeDigits(form.INN)

	errs := FieldErrors{}
	if normalizedINN == "" {
		errs["inn"] = "Укажите ИНН."
	}
	if form.CanonicalName == "" {
		errs["canonical_name"] = "Укажите название работодателя."
	}
	if len(errs) > 0 {
		return storagedb.Employer{}, errs
	}

	previous, err := s.queries.GetEmployerByID(ctx, id)
	if err != nil {
		return storagedb.Employer{}, fmt.Errorf("get employer: %w", err)
	}

	updated, err := s.queries.UpdateEmployer(ctx, storagedb.UpdateEmployerParams{
		Inn:           form.INN,
		InnNormalized: normalizedINN,
		CanonicalName: form.CanonicalName,
		UpdatedAt:     s.timestamp(),
		ID:            id,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Employer{}, FieldErrors{"inn": "Работодатель с таким ИНН уже существует."}
		}
		return storagedb.Employer{}, fmt.Errorf("update employer: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "employer",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": employerSnapshot(previous),
			"to":   employerSnapshot(updated),
		},
	})
	return updated, nil
}

// Deactivate is a placeholder: employers has no `status` column on MVP schema,
// so the query only bumps updated_at. The audit row is still recorded so the
// event is visible in the action_log.
func (s *Service) Deactivate(ctx context.Context, id int64) (storagedb.Employer, error) {
	previous, err := s.queries.GetEmployerByID(ctx, id)
	if err != nil {
		return storagedb.Employer{}, fmt.Errorf("get employer: %w", err)
	}

	updated, err := s.queries.DeactivateEmployer(ctx, storagedb.DeactivateEmployerParams{
		UpdatedAt: s.timestamp(),
		ID:        id,
	})
	if err != nil {
		return storagedb.Employer{}, fmt.Errorf("deactivate employer: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "deactivate",
		EntityType: "employer",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": previous.UpdatedAt,
			"to":   updated.UpdatedAt,
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

func normalizeDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func employerSnapshot(e storagedb.Employer) map[string]any {
	return map[string]any{
		"inn":            e.Inn,
		"inn_normalized": e.InnNormalized,
		"canonical_name": e.CanonicalName,
		"updated_at":     e.UpdatedAt,
	}
}
