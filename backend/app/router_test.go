package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
)

func TestProgramsPageReturnsOperatorShell(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Programs") {
		t.Fatalf("body does not contain Programs: %s", body)
	}
	if !strings.Contains(body, "Mintrud Admin") {
		t.Fatalf("body does not contain shell title: %s", body)
	}
}

func TestCreateProgramGroupRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	form := url.Values{}
	form.Set("code", "A")
	form.Set("name", "Охрана труда")

	req := httptest.NewRequest(http.MethodPost, "/programs/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Охрана труда") {
		t.Fatalf("body does not contain created group: %s", body)
	}
}

func TestCreateProgramRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	groupForm := url.Values{}
	groupForm.Set("code", "A")
	groupForm.Set("name", "Охрана труда")
	req := httptest.NewRequest(http.MethodPost, "/programs/groups", strings.NewReader(groupForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	form := url.Values{}
	form.Set("program_group_id", "1")
	form.Set("code", "A-1")
	form.Set("name", "Общие вопросы охраны труда")
	form.Set("default_hours", "40")

	req = httptest.NewRequest(http.MethodPost, "/programs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Общие вопросы охраны труда") {
		t.Fatalf("body does not contain created program: %s", body)
	}
}

func TestCreateEmployerRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	form := url.Values{}
	form.Set("inn", "7700000000")
	form.Set("canonical_name", "ООО Ромашка")

	req := httptest.NewRequest(http.MethodPost, "/employers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/employers", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ООО Ромашка") {
		t.Fatalf("body does not contain created employer: %s", body)
	}
}

func TestCreateWorkerRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	form := url.Values{}
	form.Set("last_name", "Петров")
	form.Set("first_name", "Петр")
	form.Set("snils", "123-456-789 00")
	form.Set("email", "worker@example.test")

	req := httptest.NewRequest(http.MethodPost, "/workers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/workers", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Петров") {
		t.Fatalf("body does not contain created worker: %s", body)
	}
}

func TestAssignEmployerRedirectsAndShowsAssignment(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	employerForm := url.Values{}
	employerForm.Set("inn", "7700000000")
	employerForm.Set("canonical_name", "ООО Ромашка")
	req := httptest.NewRequest(http.MethodPost, "/employers", strings.NewReader(employerForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	workerForm := url.Values{}
	workerForm.Set("last_name", "Петров")
	workerForm.Set("first_name", "Петр")
	workerForm.Set("snils", "123-456-789 00")
	workerForm.Set("email", "worker@example.test")
	req = httptest.NewRequest(http.MethodPost, "/workers", strings.NewReader(workerForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	assignmentForm := url.Values{}
	assignmentForm.Set("worker_id", "1")
	assignmentForm.Set("employer_id", "1")
	assignmentForm.Set("current_position", "Инженер")
	req = httptest.NewRequest(http.MethodPost, "/workers/assignments", strings.NewReader(assignmentForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/workers", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Инженер") || !strings.Contains(body, "ООО Ромашка") {
		t.Fatalf("body does not contain assignment details: %s", body)
	}
}

func TestValidationResponseIncludesFieldMessage(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	form := url.Values{}
	form.Set("canonical_name", "ООО Без ИНН")

	req := httptest.NewRequest(http.MethodPost, "/employers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Укажите ИНН") {
		t.Fatalf("body does not contain field validation message: %s", body)
	}
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mintrud-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return app.NewRouter(db)
}
