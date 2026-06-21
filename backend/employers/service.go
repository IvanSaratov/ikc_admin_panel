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
	now     func() time.Time
}

func NewService(queries *storagedb.Queries) *Service {
	return &Service{
		queries: queries,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) List(ctx context.Context) ([]storagedb.Employer, error) {
	return s.queries.ListEmployers(ctx)
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

	_ = s.log(ctx, "employer.created", "employers", employer.ID)
	return employer, nil
}

func (s *Service) timestamp() string {
	return s.now().Format(time.RFC3339)
}

func (s *Service) log(ctx context.Context, action string, entityType string, entityID int64) error {
	if s.queries == nil {
		return nil
	}
	_, err := s.queries.CreateActionLog(ctx, storagedb.CreateActionLogParams{
		Actor:      "operator_unidentified",
		Action:     action,
		EntityType: entityType,
		EntityID:   sql.NullInt64{Int64: entityID, Valid: true},
		CreatedAt:  s.timestamp(),
	})
	return err
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
