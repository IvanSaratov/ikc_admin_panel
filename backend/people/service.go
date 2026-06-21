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
	now     func() time.Time
}

func NewService(queries *storagedb.Queries) *Service {
	return &Service{
		queries: queries,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ListWorkers(ctx context.Context) ([]storagedb.Worker, error) {
	return s.queries.ListWorkers(ctx)
}

func (s *Service) ListAssignments(ctx context.Context, workerID int64) ([]storagedb.WorkerEmployer, error) {
	return s.queries.ListWorkerEmployersForWorker(ctx, workerID)
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

	_ = s.log(ctx, "worker.created", "workers", worker.ID)
	return worker, nil
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

	_ = s.log(ctx, "worker_employer.created", "worker_employers", assignment.ID)
	return assignment, nil
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
