package programs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateProgramGroupValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	service := programs.NewService(nil)
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
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mintrud-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return programs.NewService(storagedb.New(database))
}
