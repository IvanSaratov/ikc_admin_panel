package employers_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

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

	return employers.NewService(storagedb.New(database), nil)
}
