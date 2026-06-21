package programs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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
	audit   *audit.Service
	now     func() time.Time
}

// NewService constructs a programs.Service. The audit dependency is optional
// (pass nil in unit tests that only exercise validation paths); when nil,
// mutations do not produce action_log rows.
func NewService(queries *storagedb.Queries, auditSvc *audit.Service) *Service {
	return &Service{
		queries: queries,
		audit:   auditSvc,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ListGroups(ctx context.Context) ([]storagedb.ProgramGroup, error) {
	return s.queries.ListProgramGroups(ctx)
}

func (s *Service) ListPrograms(ctx context.Context) ([]storagedb.Program, error) {
	return s.queries.ListPrograms(ctx)
}

func (s *Service) GetGroup(ctx context.Context, id int64) (storagedb.ProgramGroup, error) {
	return s.queries.GetGroupByID(ctx, id)
}

func (s *Service) GetProgram(ctx context.Context, id int64) (storagedb.Program, error) {
	return s.queries.GetProgramByID(ctx, id)
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

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "program_group",
		EntityID:   sql.NullInt64{Int64: group.ID, Valid: true},
		Details: map[string]any{
			"code": group.Code,
			"name": group.Name,
		},
	})
	return group, nil
}

func (s *Service) UpdateGroup(ctx context.Context, id int64, form GroupForm) (storagedb.ProgramGroup, error) {
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

	previous, err := s.queries.GetGroupByID(ctx, id)
	if err != nil {
		return storagedb.ProgramGroup{}, fmt.Errorf("get program group: %w", err)
	}

	updated, err := s.queries.UpdateGroup(ctx, storagedb.UpdateGroupParams{
		Code:      form.Code,
		Name:      form.Name,
		UpdatedAt: s.timestamp(),
		ID:        id,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.ProgramGroup{}, FieldErrors{"code": "Группа с таким кодом уже существует."}
		}
		return storagedb.ProgramGroup{}, fmt.Errorf("update program group: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "program_group",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": map[string]any{"code": previous.Code, "name": previous.Name, "status": previous.Status},
			"to":   map[string]any{"code": updated.Code, "name": updated.Name, "status": updated.Status},
		},
	})
	return updated, nil
}

func (s *Service) DeactivateGroup(ctx context.Context, id int64) (storagedb.ProgramGroup, error) {
	previous, err := s.queries.GetGroupByID(ctx, id)
	if err != nil {
		return storagedb.ProgramGroup{}, fmt.Errorf("get program group: %w", err)
	}
	if previous.Status == "inactive" {
		// Idempotent: no audit row when nothing changed.
		return previous, nil
	}

	updated, err := s.queries.DeactivateGroup(ctx, storagedb.DeactivateGroupParams{
		UpdatedAt: s.timestamp(),
		ID:        id,
	})
	if err != nil {
		return storagedb.ProgramGroup{}, fmt.Errorf("deactivate program group: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "deactivate",
		EntityType: "program_group",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": previous.Status,
			"to":   updated.Status,
		},
	})
	return updated, nil
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

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "program",
		EntityID:   sql.NullInt64{Int64: program.ID, Valid: true},
		Details: map[string]any{
			"code":             program.Code,
			"name":             program.Name,
			"program_group_id": program.ProgramGroupID,
			"default_hours":    program.DefaultHours,
		},
	})
	return program, nil
}

func (s *Service) UpdateProgram(ctx context.Context, id int64, form ProgramForm) (storagedb.Program, error) {
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

	previous, err := s.queries.GetProgramByID(ctx, id)
	if err != nil {
		return storagedb.Program{}, fmt.Errorf("get program: %w", err)
	}

	group, err := s.queries.GetGroupByID(ctx, form.ProgramGroupID)
	if err != nil {
		return storagedb.Program{}, FieldErrors{"program_group_id": "Выберите существующую группу программ."}
	}
	if group.Status != "active" {
		return storagedb.Program{}, FieldErrors{"program_group_id": "Группа программ неактивна."}
	}

	moodleCourseID := sql.NullString{String: form.MoodleCourseID, Valid: form.MoodleCourseID != ""}
	updated, err := s.queries.UpdateProgram(ctx, storagedb.UpdateProgramParams{
		ProgramGroupID: form.ProgramGroupID,
		Code:           form.Code,
		Name:           form.Name,
		DefaultHours:   form.DefaultHours,
		MoodleCourseID: moodleCourseID,
		UpdatedAt:      s.timestamp(),
		ID:             id,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			return storagedb.Program{}, FieldErrors{"code": "В этой группе уже есть программа с таким кодом."}
		}
		return storagedb.Program{}, fmt.Errorf("update program: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "program",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": programSnapshot(previous),
			"to":   programSnapshot(updated),
		},
	})
	return updated, nil
}

func (s *Service) DeactivateProgram(ctx context.Context, id int64) (storagedb.Program, error) {
	previous, err := s.queries.GetProgramByID(ctx, id)
	if err != nil {
		return storagedb.Program{}, fmt.Errorf("get program: %w", err)
	}
	if previous.Status == "inactive" {
		return previous, nil
	}

	updated, err := s.queries.DeactivateProgram(ctx, storagedb.DeactivateProgramParams{
		UpdatedAt: s.timestamp(),
		ID:        id,
	})
	if err != nil {
		return storagedb.Program{}, fmt.Errorf("deactivate program: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "deactivate",
		EntityType: "program",
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

func programSnapshot(p storagedb.Program) map[string]any {
	moodle := ""
	if p.MoodleCourseID.Valid {
		moodle = p.MoodleCourseID.String
	}
	return map[string]any{
		"code":             p.Code,
		"name":             p.Name,
		"program_group_id": p.ProgramGroupID,
		"default_hours":    p.DefaultHours,
		"moodle_course_id": moodle,
		"status":           p.Status,
	}
}
