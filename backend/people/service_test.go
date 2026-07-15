package people_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCreateWorkerRequiresEmail(t *testing.T) {
	t.Parallel()

	service := people.NewService(nil, nil)
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
	return people.NewService(queries, auditSvc), queries
}

func TestUpdateWorker_RenormalizesSNILS(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
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

	updated, err := service.UpdateWorker(ctx, worker.ID, people.WorkerForm{
		LastName:  "Петров",
		FirstName: "Петр",
		SNILS:     "987 654 321 00",
		Email:     "worker@example.test",
	})
	if err != nil {
		t.Fatalf("update worker: %v", err)
	}
	if updated.SnilsNormalized != "98765432100" {
		t.Errorf("normalized SNILS = %q, want 98765432100", updated.SnilsNormalized)
	}
}

func TestDeactivateAssignment_AllowsReassignLater(t *testing.T) {
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

	now := time.Now().UTC().Format(time.RFC3339)
	emp, err := queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn:           "7700000000",
		InnNormalized: "7700000000",
		CanonicalName: "ООО Ромашка",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed employer: %v", err)
	}

	first, err := service.AssignEmployer(ctx, people.AssignmentForm{
		WorkerID:        worker.ID,
		EmployerID:      emp.ID,
		CurrentPosition: "Инженер",
	})
	if err != nil {
		t.Fatalf("first assign: %v", err)
	}

	if _, err := service.DeactivateAssignment(ctx, first.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// The unique index on (worker_id, employer_id) WHERE status='active'
	// should no longer block a fresh assignment for the same pair.
	second, err := service.AssignEmployer(ctx, people.AssignmentForm{
		WorkerID:        worker.ID,
		EmployerID:      emp.ID,
		CurrentPosition: "Старший инженер",
	})
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if second.Status != "active" {
		t.Errorf("status = %q, want active", second.Status)
	}
	if second.ID == first.ID {
		t.Errorf("reassignment reused old id %d, expected new row", first.ID)
	}
}

func TestGetWorkerDetail_IncludesAssignments(t *testing.T) {
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
	now := time.Now().UTC().Format(time.RFC3339)
	emp, err := queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn:           "7700000000",
		InnNormalized: "7700000000",
		CanonicalName: "ООО Ромашка",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed employer: %v", err)
	}
	second, err := queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn:           "7700999999",
		InnNormalized: "7700999999",
		CanonicalName: "ООО Дубль",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed second employer: %v", err)
	}
	if _, err := service.AssignEmployer(ctx, people.AssignmentForm{
		WorkerID:        worker.ID,
		EmployerID:      emp.ID,
		CurrentPosition: "Инженер",
	}); err != nil {
		t.Fatalf("assign first: %v", err)
	}
	if _, err := service.AssignEmployer(ctx, people.AssignmentForm{
		WorkerID:        worker.ID,
		EmployerID:      second.ID,
		CurrentPosition: "Аналитик",
	}); err != nil {
		t.Fatalf("assign second: %v", err)
	}

	workerDetail, err := service.GetWorker(ctx, worker.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if workerDetail.ID != worker.ID {
		t.Errorf("worker id = %d, want %d", workerDetail.ID, worker.ID)
	}

	assignments, err := service.ListAssignments(ctx, worker.ID)
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("assignments = %d, want 2", len(assignments))
	}

	positions := make(map[string]bool)
	for _, a := range assignments {
		positions[a.CurrentPosition] = true
	}
	if !positions["Инженер"] || !positions["Аналитик"] {
		t.Errorf("missing expected position(s): %v", positions)
	}
}
