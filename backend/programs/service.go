package programs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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

func (e FieldErrors) Is(target error) bool {
	return target == ErrValidation
}

type GroupForm struct {
	Code string
	Name string
}

type ProgramForm struct {
	ProgramGroupID int64
	Code           string
	Name           string
	DefaultHours   int64
	MoodleCourseID string
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

func (s *Service) ListGroups(ctx context.Context) ([]storagedb.ProgramGroup, error) {
	return s.queries.ListProgramGroups(ctx)
}

func (s *Service) ListPrograms(ctx context.Context) ([]storagedb.Program, error) {
	return s.queries.ListPrograms(ctx)
}

func (s *Service) CreateGroup(ctx context.Context, form GroupForm) (storagedb.ProgramGroup, error) {
	form.Code = strings.TrimSpace(form.Code)
	form.Name = strings.TrimSpace(form.Name)

	errs := FieldErrors{}
	if form.Code == "" {
		errs["code"] = "Укажите код группы."
	}
	if form.Name == "" {
		errs["name"] = "Укажите название группы."
	}
	if len(errs) > 0 {
		return storagedb.ProgramGroup{}, errs
	}

	timestamp := s.timestamp()
	group, err := s.queries.CreateProgramGroup(ctx, storagedb.CreateProgramGroupParams{
		Code:      form.Code,
		Name:      form.Name,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.ProgramGroup{}, FieldErrors{"code": "Группа с таким кодом уже существует."}
		}
		return storagedb.ProgramGroup{}, fmt.Errorf("create program group: %w", err)
	}

	_ = s.log(ctx, "program_group.created", "program_groups", group.ID)
	return group, nil
}

func (s *Service) CreateProgram(ctx context.Context, form ProgramForm) (storagedb.Program, error) {
	form.Code = strings.TrimSpace(form.Code)
	form.Name = strings.TrimSpace(form.Name)
	form.MoodleCourseID = strings.TrimSpace(form.MoodleCourseID)

	errs := FieldErrors{}
	if form.ProgramGroupID <= 0 {
		errs["program_group_id"] = "Выберите группу программ."
	}
	if form.Code == "" {
		errs["code"] = "Укажите код программы."
	}
	if form.Name == "" {
		errs["name"] = "Укажите название программы."
	}
	if form.DefaultHours <= 0 {
		errs["default_hours"] = "Часы должны быть больше нуля."
	}
	if len(errs) > 0 {
		return storagedb.Program{}, errs
	}

	moodleCourseID := sql.NullString{String: form.MoodleCourseID, Valid: form.MoodleCourseID != ""}
	timestamp := s.timestamp()
	program, err := s.queries.CreateProgram(ctx, storagedb.CreateProgramParams{
		ProgramGroupID: form.ProgramGroupID,
		Code:           form.Code,
		Name:           form.Name,
		DefaultHours:   form.DefaultHours,
		MoodleCourseID: moodleCourseID,
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Program{}, FieldErrors{"code": "В этой группе уже есть программа с таким кодом."}
		}
		return storagedb.Program{}, fmt.Errorf("create program: %w", err)
	}

	_ = s.log(ctx, "program.created", "programs", program.ID)
	return program, nil
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
