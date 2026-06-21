package employers_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateEmployerValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	service := employers.NewService(nil, nil)
	_, err := service.Create(context.Background(), employers.Form{})
	if !errors.Is(err, employers.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestCreateEmployerNormalizesINN(t *testing.T) {
	t.Parallel()

	service := newService(t)
	employer, err := service.Create(context.Background(), employers.Form{
		INN:           " 77-00 00 00 00 ",
		CanonicalName: "ООО Ромашка",
	})
	if err != nil {
		t.Fatalf("create employer: %v", err)
	}
	if employer.InnNormalized != "7700000000" {
		t.Fatalf("normalized INN = %q, want 7700000000", employer.InnNormalized)
	}
}

func TestCreateEmployerMapsDuplicateINNToValidation(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, employers.Form{INN: "7700000000", CanonicalName: "ООО Ромашка"}); err != nil {
		t.Fatalf("create first employer: %v", err)
	}
	_, err := service.Create(ctx, employers.Form{INN: "77 00 00 00 00", CanonicalName: "ООО Дубль"})
	if !errors.Is(err, employers.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func newService(t *testing.T) *employers.Service {
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

	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	return employers.NewService(queries, auditSvc)
}

func TestUpdateEmployer_RenormalizesINN(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	employer, err := service.Create(ctx, employers.Form{INN: "7700000000", CanonicalName: "ООО Ромашка"})
	if err != nil {
		t.Fatalf("create employer: %v", err)
	}

	updated, err := service.Update(ctx, employer.ID, employers.Form{INN: " 77-11 22 33 44 ", CanonicalName: "ООО Ромашка+"})
	if err != nil {
		t.Fatalf("update employer: %v", err)
	}
	if updated.InnNormalized != "7711223344" {
		t.Errorf("normalized INN = %q, want 7711223344", updated.InnNormalized)
	}
	if updated.Inn != "77-11 22 33 44" {
		t.Errorf("raw INN = %q, want trimmed form %q", updated.Inn, "77-11 22 33 44")
	}
}

func TestUpdateEmployer_DuplicateINN_MapsToFieldError(t *testing.T) {
	t.Parallel()

	service := newService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, employers.Form{INN: "7700000000", CanonicalName: "ООО Ромашка"}); err != nil {
		t.Fatalf("create first employer: %v", err)
	}
	second, err := service.Create(ctx, employers.Form{INN: "7700999999", CanonicalName: "ООО Дубль"})
	if err != nil {
		t.Fatalf("create second employer: %v", err)
	}

	_, err = service.Update(ctx, second.ID, employers.Form{INN: "7700000000", CanonicalName: "ООО Переименование"})
	if !errors.Is(err, employers.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	var fe employers.FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("error is not FieldErrors: %v", err)
	}
	if fe["inn"] == "" {
		t.Errorf("expected inn field error, got %v", fe)
	}
}
