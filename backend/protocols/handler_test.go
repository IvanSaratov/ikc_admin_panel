package protocols

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

// handlerEnv bundles everything the handler tests need: the handler, the
// raw DB, the queries facade, and the service. Bypasses the router so we
// don't have to wire CSRF/auth for these handler-level tests.
type handlerEnv struct {
	handler *Handler
	db      *sql.DB
	queries *storagedb.Queries
	svc     *Service
	// trainingRecordSeq is incremented for each mustTrainingRecord call so
	// repeated calls in the same test produce UNIQUE-compatible rows.
	trainingRecordSeq int
}

func newHandlerEnv(t *testing.T) *handlerEnv {
	t.Helper()

	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mintrud-handler-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	svc := NewService(queries, database, auditSvc)
	return &handlerEnv{
		handler: NewHandler(queries, database, auditSvc),
		db:      database,
		queries: queries,
		svc:     svc,
	}
}

// postForm issues a POST with the given values to the supplied handler.
// URL params are passed as alternating key/value pairs so chi.URLParam
// works inside the handler.
func (e *handlerEnv) postForm(t *testing.T, h http.HandlerFunc, values url.Values, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, params...)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// get issues a GET to the supplied handler.
func (e *handlerEnv) get(t *testing.T, h http.HandlerFunc, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withURLParams(req, params...)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// withURLParams injects chi URL params into the request context so handlers
// that call chi.URLParam work without a real router. Pairs are key1, value1,
// key2, value2, ...
func withURLParams(r *http.Request, pairs ...string) *http.Request {
	if len(pairs)%2 != 0 {
		panic("withURLParams: odd number of pairs")
	}
	rctx := chi.NewRouteContext()
	for i := 0; i < len(pairs); i += 2 {
		rctx.URLParams.Add(pairs[i], pairs[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestList_GET_RendersProtocols(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	// Empty list still renders the page.
	rr := env.get(t, env.handler.List)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Протоколы") {
		t.Errorf("body missing 'Протоколы' heading, got: %s", truncate(rr.Body.String(), 200))
	}

	// After creating a protocol the list includes the link.
	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	rr = env.get(t, env.handler.List)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), fmt.Sprintf("/protocols/%d", p.ID)) {
		t.Errorf("body missing link to protocol %d", p.ID)
	}
}

func TestDetail_GET_ShowsParticipants(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	tr := env.mustTrainingRecord(t)
	if err := env.svc.AddParticipant(ctx, p.ID, tr); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}

	rr := env.get(t, env.handler.Detail, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, fmt.Sprintf("draft #%d", p.ID)) {
		t.Errorf("body missing draft heading, got: %s", truncate(body, 200))
	}
	if !strings.Contains(body, fmt.Sprint(tr)) {
		t.Errorf("body missing training_record %d", tr)
	}
}

func TestFix_GET_Form_Only_If_Not_Fixed(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	// Draft: GET /fix returns the form.
	rr := env.get(t, env.handler.Fix, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusOK {
		t.Fatalf("draft fix GET: status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Фиксация протокола") {
		t.Errorf("body missing FixForm heading")
	}

	// Fixed: GET /fix redirects to detail.
	if _, err := env.svc.Fix(ctx, p.ID, validFixInput()); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	rr = env.get(t, env.handler.Fix, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("fixed fix GET: status = %d, want 303 (redirect to detail)", rr.Code)
	}
	if rr.Header().Get("Location") != fmt.Sprintf("/protocols/%d", p.ID) {
		t.Errorf("Location = %q, want /protocols/%d", rr.Header().Get("Location"), p.ID)
	}
}

func TestFix_POST_RedirectsToDetail(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	form := url.Values{}
	form.Set("training_start_date", "2026-06-01")
	form.Set("training_end_date", "2026-06-30")
	form.Set("protocol_date", "2026-06-30")
	form.Set("protocol_suffix", "")

	rr := env.postForm(t, env.handler.Fix, form, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rr.Code, truncate(rr.Body.String(), 300))
	}
	if rr.Header().Get("Location") != fmt.Sprintf("/protocols/%d", p.ID) {
		t.Errorf("Location = %q, want /protocols/%d", rr.Header().Get("Location"), p.ID)
	}

	got, err := env.svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "fixed" {
		t.Errorf("status = %q, want fixed", got.Status)
	}
}

func TestFix_POST_MissingFields_Returns400(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	rr := env.postForm(t, env.handler.Fix, url.Values{}, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing fields: status = %d, want 400", rr.Code)
	}
}

func TestAddParticipant_POST_UpdatesList(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	tr := env.mustTrainingRecord(t)

	form := url.Values{}
	form.Set("training_record_id", fmt.Sprint(tr))
	rr := env.postForm(t, env.handler.AddParticipant, form, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rr.Code, truncate(rr.Body.String(), 300))
	}

	parts, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(parts) != 1 || parts[0].TrainingRecordID != tr {
		t.Errorf("participants = %+v, want one row for tr=%d", parts, tr)
	}
}

func TestAddParticipant_POST_DuplicateReturns400(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	tr := env.mustTrainingRecord(t)

	form := url.Values{}
	form.Set("training_record_id", fmt.Sprint(tr))
	if rr := env.postForm(t, env.handler.AddParticipant, form, "id", fmt.Sprint(p.ID)); rr.Code != http.StatusSeeOther {
		t.Fatalf("first AddParticipant status = %d, want 303", rr.Code)
	}
	if rr := env.postForm(t, env.handler.AddParticipant, form, "id", fmt.Sprint(p.ID)); rr.Code != http.StatusBadRequest {
		t.Errorf("duplicate AddParticipant status = %d, want 400", rr.Code)
	}
}

func TestRemoveParticipant_POST_SoftDeletes(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	tr := env.mustTrainingRecord(t)
	if err := env.svc.AddParticipant(ctx, p.ID, tr); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}
	parts, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil || len(parts) != 1 {
		t.Fatalf("ListParticipants: %v len=%d", err, len(parts))
	}

	form := url.Values{}
	form.Set("_method", "delete")
	rr := env.postForm(t, env.handler.RemoveParticipant, form,
		"id", fmt.Sprint(p.ID), "pid", fmt.Sprint(parts[0].ID))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}

	active, err := env.svc.ListParticipants(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active participants = %d, want 0", len(active))
	}
}

func TestTransition_POST_ChangesStatus(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	if _, err := env.svc.Fix(ctx, p.ID, validFixInput()); err != nil {
		t.Fatalf("Fix: %v", err)
	}

	form := url.Values{}
	form.Set("to", "xml_uploaded")
	rr := env.postForm(t, env.handler.Transition, form, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rr.Code, truncate(rr.Body.String(), 300))
	}

	got, err := env.svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "xml_uploaded" {
		t.Errorf("status = %q, want xml_uploaded", got.Status)
	}
}

func TestTransition_POST_InvalidReturns400(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}

	form := url.Values{}
	form.Set("to", "generated") // invalid: draft → generated
	rr := env.postForm(t, env.handler.Transition, form, "id", fmt.Sprint(p.ID))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid transition)", rr.Code)
	}
}

func TestCreate_POST_RedirectsToList(t *testing.T) {
	t.Parallel()

	env := newHandlerEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	form := url.Values{}
	form.Set("program_group_id", fmt.Sprint(groupID))
	rr := env.postForm(t, env.handler.Create, form)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rr.Code, truncate(rr.Body.String(), 300))
	}

	all, err := env.svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("protocols = %d, want 1", len(all))
	}
}

// mustGroup inserts an active program group for handler tests.
func (e *handlerEnv) mustGroup(t *testing.T) int64 {
	t.Helper()

	ctx := context.Background()
	now := "2026-06-21T00:00:00Z"
	res, err := e.db.ExecContext(ctx,
		`INSERT INTO program_groups (code, name, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`,
		"HG-"+t.Name(), "Group "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// mustTrainingRecord inserts a full chain of worker → worker_employer →
// training_record so AddParticipant tests have a real FK target.
func (e *handlerEnv) mustTrainingRecord(t *testing.T) int64 {
	t.Helper()
	e.trainingRecordSeq++
	return e.mustTrainingRecordAt(t, e.trainingRecordSeq)
}

func (e *handlerEnv) mustTrainingRecordAt(t *testing.T, seq int) int64 {
	t.Helper()

	ctx := context.Background()
	now := "2026-06-21T00:00:00Z"

	groupRes, err := e.db.ExecContext(ctx,
		`INSERT INTO program_groups (code, name, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`,
		fmt.Sprintf("HPG-%s-%d", t.Name(), seq), fmt.Sprintf("HPG %s %d", t.Name(), seq), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, err := groupRes.LastInsertId()
	if err != nil {
		t.Fatalf("program_group last id: %v", err)
	}

	snils := fmt.Sprintf("%011d", 20000000000+seq)
	workerRes, err := e.db.ExecContext(ctx,
		`INSERT INTO workers (last_name, first_name, snils, snils_normalized, email, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"Петров", fmt.Sprintf("Пётр%d", seq), snils, snils, "petr@example.com", now, now)
	if err != nil {
		t.Fatalf("insert worker: %v", err)
	}
	workerID, err := workerRes.LastInsertId()
	if err != nil {
		t.Fatalf("worker last id: %v", err)
	}

	employerRes, err := e.db.ExecContext(ctx,
		`INSERT INTO employers (inn, inn_normalized, canonical_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		fmt.Sprintf("%010d", 9000000000+seq), fmt.Sprintf("%010d", 9000000000+seq),
		fmt.Sprintf("HEmployer %s %d", t.Name(), seq), now, now)
	if err != nil {
		t.Fatalf("insert employer: %v", err)
	}
	employerID, err := employerRes.LastInsertId()
	if err != nil {
		t.Fatalf("employer last id: %v", err)
	}

	weRes, err := e.db.ExecContext(ctx,
		`INSERT INTO worker_employers (worker_id, employer_id, current_position, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
		workerID, employerID, "Engineer", now, now)
	if err != nil {
		t.Fatalf("insert worker_employer: %v", err)
	}
	weID, err := weRes.LastInsertId()
	if err != nil {
		t.Fatalf("worker_employer last id: %v", err)
	}

	progRes, err := e.db.ExecContext(ctx,
		`INSERT INTO programs (program_group_id, code, name, default_hours, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		groupID, fmt.Sprintf("HP-%s-%d", t.Name(), seq), fmt.Sprintf("HProgram %s %d", t.Name(), seq), 40, now, now)
	if err != nil {
		t.Fatalf("insert program: %v", err)
	}
	progID, err := progRes.LastInsertId()
	if err != nil {
		t.Fatalf("program last id: %v", err)
	}

	trRes, err := e.db.ExecContext(ctx,
		`INSERT INTO training_records (worker_employer_id, program_id, position, hours, requires_mintrud_test, moodle_status, status, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 'not_required', 'active', ?, ?)`,
		weID, progID, "Engineer", 40, now, now)
	if err != nil {
		t.Fatalf("insert training_record: %v", err)
	}
	id, err := trRes.LastInsertId()
	if err != nil {
		t.Fatalf("training_record last id: %v", err)
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
