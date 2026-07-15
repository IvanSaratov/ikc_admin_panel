package programs_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateProgramGroupValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	service := programs.NewService(nil, nil)
	_, err := service.CreateGroup(context.Background(), programs.GroupForm{})
	if !errors.Is(err, programs.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestCreateProgramGroupMapsDuplicateCodeToValidation(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()

	if _, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"}); err != nil {
		t.Fatalf("create first group: %v", err)
	}
	_, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Duplicate Group A"})
	if !errors.Is(err, programs.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestCreateProgramRequiresPositiveHours(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	_, err = service.CreateProgram(ctx, programs.ProgramForm{
		ProgramGroupID: group.ID,
		Code:           "A-1",
		Name:           "Program A-1",
		DefaultHours:   0,
	})
	if !errors.Is(err, programs.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestCreateProgramMapsDuplicateCodeWithinGroupToValidation(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	form := programs.ProgramForm{
		ProgramGroupID: group.ID,
		Code:           "A-1",
		Name:           "Program A-1",
		DefaultHours:   40,
	}
	if _, err := service.CreateProgram(ctx, form); err != nil {
		t.Fatalf("create first program: %v", err)
	}
	_, err = service.CreateProgram(ctx, form)
	if !errors.Is(err, programs.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func newService(t *testing.T) *programs.Service {
	t.Helper()

	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "ikc-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	return programs.NewService(queries, auditSvc)
}

// newServiceWithQueries is like newService but also returns the *sql.DB so
// tests can inspect tables directly without going through sqlc.
func newServiceWithQueries(t *testing.T) (*programs.Service, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "ikc-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	return programs.NewService(queries, auditSvc), database
}

func TestUpdateGroup_NormalizesFields(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "  Initial  "})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	updated, err := service.UpdateGroup(ctx, group.ID, programs.GroupForm{Code: "A", Name: "  Renamed  "})
	if err != nil {
		t.Fatalf("update group: %v", err)
	}
	if updated.Code != "A" {
		t.Errorf("code = %q, want A", updated.Code)
	}
	if updated.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed (trimmed)", updated.Name)
	}
}

func TestDeactivateGroup_DoesNotHardDelete(t *testing.T) {
	t.Parallel()

	service, database := newServiceWithQueries(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	if _, err := service.DeactivateGroup(ctx, group.ID); err != nil {
		t.Fatalf("deactivate group: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM program_groups WHERE id = ?`, group.ID).Scan(&count); err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if count != 1 {
		t.Errorf("groups remaining = %d, want 1 (soft delete)", count)
	}

	updated, err := service.GetGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if updated.Status != "inactive" {
		t.Errorf("status = %q, want inactive", updated.Status)
	}
}

func TestUpdateProgram_RejectsInvalidGroup(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	program, err := service.CreateProgram(ctx, programs.ProgramForm{
		ProgramGroupID: group.ID,
		Code:           "A-1",
		Name:           "P",
		DefaultHours:   40,
	})
	if err != nil {
		t.Fatalf("create program: %v", err)
	}

	_, err = service.UpdateProgram(ctx, program.ID, programs.ProgramForm{
		ProgramGroupID: 9999, // non-existent group
		Code:           "A-1",
		Name:           "P",
		DefaultHours:   40,
	})
	if !errors.Is(err, programs.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestDeactivateProgram_Idempotent(t *testing.T) {
	t.Parallel()

	service, database := newServiceWithQueries(t)
	ctx := context.Background()
	group, err := service.CreateGroup(ctx, programs.GroupForm{Code: "A", Name: "Group A"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	program, err := service.CreateProgram(ctx, programs.ProgramForm{
		ProgramGroupID: group.ID,
		Code:           "A-1",
		Name:           "P",
		DefaultHours:   40,
	})
	if err != nil {
		t.Fatalf("create program: %v", err)
	}

	if _, err := service.DeactivateProgram(ctx, program.ID); err != nil {
		t.Fatalf("first deactivate: %v", err)
	}

	// Second call must not error.
	updated, err := service.DeactivateProgram(ctx, program.ID)
	if err != nil {
		t.Fatalf("second deactivate: %v", err)
	}
	if updated.Status != "inactive" {
		t.Errorf("status = %q, want inactive", updated.Status)
	}

	// Only one deactivate audit row should be present for this program.
	var count int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM action_log
		WHERE entity_type = 'program' AND entity_id = ? AND action = 'deactivate'
	`, program.ID).Scan(&count); err != nil {
		t.Fatalf("count action_log: %v", err)
	}
	if count != 1 {
		t.Errorf("deactivate audit rows = %d, want 1 (idempotent)", count)
	}
}
