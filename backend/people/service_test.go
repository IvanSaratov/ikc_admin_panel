package people_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateWorkerRequiresEmail(t *testing.T) {
	t.Parallel()

	service := people.NewService(nil)
	_, err := service.CreateWorker(context.Background(), people.WorkerForm{
		LastName:  "Петров",
		FirstName: "Петр",
		SNILS:     "123-456-789 00",
	})
	if !errors.Is(err, people.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestCreateWorkerNormalizesSNILS(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
	worker, err := service.CreateWorker(context.Background(), people.WorkerForm{
		LastName:  "Петров",
		FirstName: "Петр",
		SNILS:     "123-456-789 00",
		Email:     "worker@example.test",
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	if worker.SnilsNormalized != "12345678900" {
		t.Fatalf("normalized SNILS = %q, want 12345678900", worker.SnilsNormalized)
	}
}

func TestCreateWorkerMapsDuplicateSNILSToValidation(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
	ctx := context.Background()
	form := people.WorkerForm{
		LastName:  "Петров",
		FirstName: "Петр",
		SNILS:     "123-456-789 00",
		Email:     "worker@example.test",
	}
	if _, err := service.CreateWorker(ctx, form); err != nil {
		t.Fatalf("create first worker: %v", err)
	}
	form.Email = "worker2@example.test"
	_, err := service.CreateWorker(ctx, form)
	if !errors.Is(err, people.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestAssignEmployerMapsDuplicateActivePairToValidation(t *testing.T) {
	t.Parallel()

	service, queries := newService(t)
	ctx := context.Background()

	worker, err := service.CreateWorker(ctx, people.WorkerForm{
		LastName:  "Петров",
		FirstName: "Петр",
		SNILS:     "123-456-789 00",
		Email:     "worker@example.test",
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	employer, err := queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn:           "7700000000",
		InnNormalized: "7700000000",
		CanonicalName: "ООО Ромашка",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create employer: %v", err)
	}

	form := people.AssignmentForm{
		WorkerID:        worker.ID,
		EmployerID:      employer.ID,
		CurrentPosition: "Инженер",
	}
	if _, err := service.AssignEmployer(ctx, form); err != nil {
		t.Fatalf("assign employer: %v", err)
	}
	_, err = service.AssignEmployer(ctx, form)
	if !errors.Is(err, people.ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func newService(t *testing.T) (*people.Service, *storagedb.Queries) {
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
	return people.NewService(queries), queries
}
