package app_test

import (
	"context"
	"database/sql"
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

// seedGroup posts a program group creation form so subsequent tests have
// something with id=1 to edit/deactivate.
func seedGroup(t *testing.T, router http.Handler) {
	t.Helper()
	form := url.Values{}
	form.Set("code", "A")
	form.Set("name", "Охрана труда")
	req := httptest.NewRequest(http.MethodPost, "/programs/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)
}

func TestEdit_GET_ReturnsForm_200(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	seedGroup(t, router)

	req := httptest.NewRequest(http.MethodGet, "/programs/groups/1/edit", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Edit program group") {
		t.Errorf("body missing edit heading: %s", body)
	}
	if !strings.Contains(body, `value="A"`) {
		t.Errorf("body missing existing code value: %s", body)
	}
}

func TestEdit_POST_UpdatesRecord_Redirects(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	seedGroup(t, router)

	form := url.Values{}
	form.Set("code", "A")
	form.Set("name", "Renamed group")
	req := httptest.NewRequest(http.MethodPost, "/programs/groups/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Renamed group") {
		t.Fatalf("updated name not visible: %s", rec.Body.String())
	}
}

func TestDeactivate_POST_ChangesStatus_Redirects(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	seedGroup(t, router)

	req := httptest.NewRequest(http.MethodPost, "/programs/groups/1/deactivate", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	// Edit form should still render (group exists), and the list should now
	// show "inactive".
	req = httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "inactive") {
		t.Fatalf("status not reflected on list: %s", rec.Body.String())
	}
}

func TestDetail_GET_Returns200_WithChildren(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	// Create employer.
	form := url.Values{}
	form.Set("inn", "7700000000")
	form.Set("canonical_name", "ООО Ромашка")
	req := httptest.NewRequest(http.MethodPost, "/employers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// Create worker.
	workerForm := url.Values{}
	workerForm.Set("last_name", "Петров")
	workerForm.Set("first_name", "Петр")
	workerForm.Set("snils", "123-456-789 00")
	workerForm.Set("email", "worker@example.test")
	req = httptest.NewRequest(http.MethodPost, "/workers", strings.NewReader(workerForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// Assign.
	assign := url.Values{}
	assign.Set("worker_id", "1")
	assign.Set("employer_id", "1")
	assign.Set("current_position", "Инженер")
	req = httptest.NewRequest(http.MethodPost, "/workers/assignments", strings.NewReader(assign.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// Now hit the employer detail page.
	req = httptest.NewRequest(http.MethodGet, "/employers/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ООО Ромашка") {
		t.Errorf("missing employer name on detail: %s", body)
	}
	if !strings.Contains(body, "Worker assignments") {
		t.Errorf("missing assignments section: %s", body)
	}

	// Worker detail should also have 200 + employer card.
	req = httptest.NewRequest(http.MethodGet, "/workers/1", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("worker detail status = %d, want 200", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Петров") {
		t.Errorf("missing worker surname on detail: %s", body)
	}
	if !strings.Contains(body, "Инженер") {
		t.Errorf("missing assignment on detail: %s", body)
	}
}

func TestMutation_AlwaysWritesAudit(t *testing.T) {
	t.Parallel()

	router, database := newTestRouterWithDB(t)

	// Create an employer through the router.
	form := url.Values{}
	form.Set("inn", "7700000000")
	form.Set("canonical_name", "ООО Ромашка")
	req := httptest.NewRequest(http.MethodPost, "/employers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// Update it.
	form = url.Values{}
	form.Set("inn", "7700000000")
	form.Set("canonical_name", "ООО Ромашка+")
	req = httptest.NewRequest(http.MethodPost, "/employers/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// Deactivate it.
	req = httptest.NewRequest(http.MethodPost, "/employers/1/deactivate", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(httptest.NewRecorder(), req)

	rows, err := database.QueryContext(context.Background(), `
		SELECT action FROM action_log
		WHERE entity_type = 'employer' AND entity_id = 1
		ORDER BY id
	`)
	if err != nil {
		t.Fatalf("query action_log: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[action]++
	}
	for _, expected := range []string{"create", "update", "deactivate"} {
		if got[expected] == 0 {
			t.Errorf("expected audit row for action %q, got %v", expected, got)
		}
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

// newTestRouterWithDB returns the router and the underlying *sql.DB so
// tests can inspect the action_log table directly.
func newTestRouterWithDB(t *testing.T) (http.Handler, *sql.DB) {
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

	return app.NewRouter(db), db
}
