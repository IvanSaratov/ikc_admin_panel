package requests_test

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/requests"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// mountTestRouter mirrors admin/handler_test.go: it builds the same
// app.NewRouter(...) the real binary uses, but with a fresh SQLite
// db in t.TempDir() and a fixed CSRF key. Returns the router + the
// queries (so tests can inspect rows directly without going through
// HTTP).
func mountTestRouter(t *testing.T) (http.Handler, *scs.SessionManager, *storagedb.Queries, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "requests-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	queries := storagedb.New(db)

	now := time.Now().UTC().Format(time.RFC3339)
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if _, err := queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login: "alice", PasswordHash: string(hash),
		Role: "admin", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := queries.CreateEmployer(ctx, storagedb.CreateEmployerParams{
		Inn: "7700000000", InnNormalized: "7700000000", CanonicalName: "ООО Ромашка",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed employer: %v", err)
	}
	group, err := queries.CreateProgramGroup(ctx, storagedb.CreateProgramGroupParams{
		Code: "G1", Name: "Group 1", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := queries.CreateProgram(ctx, storagedb.CreateProgramParams{
		ProgramGroupID: group.ID, Code: "A-1", Name: "Program A-1", DefaultHours: 16,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed program: %v", err)
	}

	sm := scs.New()
	sm.Lifetime = time.Hour
	sm.Cookie.Name = "requests_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"

	csrfKey := []byte("0123456789abcdef0123456789abcdef")
	csrfMW := csrf.Protect(csrfKey,
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.FieldName("csrf_token"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.CookieName("csrf_token"),
		csrf.Path("/"),
	)
	_ = csrfMW
	// CSRF is exercised by backend/admin/handler_test.go (the full
	// gorilla middleware dance). For requests handler tests we use a
	// no-op middleware so multipart uploads don't have to round-trip
	// the csrf cookie + masked token through every step. The handler
	// itself doesn't know about CSRF — that's the router's job.
	csrfMW = func(next http.Handler) http.Handler { return next }

	auditSvc := audit.NewService(queries)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	router := app.NewRouter(app.Deps{
		Database: db,
		Sessions: sm,
		CSRF:     csrfMW,
		Log:      logger,
	})
	_ = auditSvc
	return router, sm, queries, db
}

// authenticate performs the GET /login -> POST /login dance and
// returns just the session cookie. The router under test uses a no-op
// CSRF middleware (see mountTestRouter) so we don't need to thread
// csrf tokens through the test.
func authenticate(t *testing.T, router http.Handler, sm *scs.SessionManager) *http.Cookie {
	t.Helper()

	postValues := url.Values{}
	postValues.Set("login", "alice")
	postValues.Set("password", "test-password")
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(postValues.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Referer", "http://example.com/login")
	postReq.Host = "example.com"
	postReq = csrf.PlaintextHTTPRequest(postReq)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code >= 400 {
		t.Fatalf("login failed: status=%d body=%s", postRec.Code, postRec.Body.String())
	}

	sessionCookie := extractCookie(t, postRec.Result().Cookies(), sm.Cookie.Name)
	if sessionCookie == nil {
		t.Fatalf("no session cookie after login")
	}
	return sessionCookie
}

func extractCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not found in response (have %d cookies)", name, len(cookies))
	return nil
}

func TestList_GET_RendersRequests(t *testing.T) {
	t.Parallel()
	router, sm, _, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	req := httptest.NewRequest(http.MethodGet, "/requests", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Requests") {
		t.Errorf("expected 'Requests' heading in body, got %s", rec.Body.String())
	}
}

func TestUpload_POST_CreatesRequest_Redirects(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	xlsx := buildTestXLSX(t, [][]string{
		{"Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер", "A-1"},
	})

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("employer_id", "1")
	_ = mw.WriteField("received_date", "2026-06-15")
	_ = mw.WriteField("notes", "test")
	fw, _ := mw.CreateFormFile("xlsx", "test.xlsx")
	_, _ = fw.Write(xlsx)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/requests/new", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Host = "example.com"
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/requests/") {
		t.Fatalf("Location = %q, want /requests/N", loc)
	}

	got, err := queries.ListClientRequests(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("client_requests count = %d, want 1", len(got))
	}
}

func TestDetail_GET_RendersRows(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	ctx := context.Background()
	now := "2026-06-22T12:00:00Z"
	req, err := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-15", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed req: %v", err)
	}
	if _, err := queries.CreateRequestRow(ctx, storagedb.CreateRequestRowParams{
		ClientRequestID: req.ID, RowNumber: 2, RawData: "{}",
		RawFullName:      ns("Иванов Иван Иванович"),
		ParsedLastName:   ns("Иванов"),
		ParsedFirstName:  ns("Иван"),
		ParsedMiddleName: ns("Иванович"),
		ParsedSnils:      ns("12345678900"),
		ParsedEmail:      ns("ivanov@example.com"),
		ParsedPosition:   ns("Инженер"),
		Status:           requests.RowStatusParsed,
		CreatedAt:        now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodGet, "/requests/"+strconv.FormatInt(req.ID, 10), nil)
	httpReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Иванов") {
		t.Errorf("expected FIO in body, got %s", body)
	}
	if !strings.Contains(body, "Apply") {
		t.Errorf("expected Apply button in body")
	}
}

func TestApply_POST_TransitionsStatus(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	ctx := context.Background()
	now := "2026-06-22T12:00:00Z"
	req, err := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-15", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed req: %v", err)
	}
	if _, err := queries.CreateWorker(ctx, storagedb.CreateWorkerParams{
		LastName: "Иванов", FirstName: "Иван",
		MiddleName:      ns("Иванович"),
		Snils:           "12345678900",
		SnilsNormalized: "12345678900",
		Email:           "ivanov@example.com",
		BirthDate:       sql.NullString{},
		CreatedAt:       now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	row, err := queries.CreateRequestRow(ctx, storagedb.CreateRequestRowParams{
		ClientRequestID: req.ID, RowNumber: 2, RawData: "{}",
		RawFullName:      ns("Иванов Иван Иванович"),
		ParsedLastName:   ns("Иванов"),
		ParsedFirstName:  ns("Иван"),
		ParsedMiddleName: ns("Иванович"),
		ParsedSnils:      ns("12345678900"),
		ParsedEmail:      ns("ivanov@example.com"),
		ParsedPosition:   ns("Инженер"),
		Status:           requests.RowStatusParsed,
		CreatedAt:        now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed row: %v", err)
	}
	// mountTestRouter already seeded program "A-1" — reuse it instead
	// of re-inserting (which would violate the UNIQUE constraint on
	// program_groups.code). Find the seeded group by listing.
	groups, err := queries.ListProgramGroups(ctx)
	if err != nil || len(groups) == 0 {
		t.Fatalf("list groups: %v", err)
	}
	var groupID int64
	for _, g := range groups {
		if g.Code == "G1" {
			groupID = g.ID
			break
		}
	}
	if groupID == 0 {
		t.Fatalf("seeded group G1 not found")
	}
	prog, err := queries.CreateProgram(ctx, storagedb.CreateProgramParams{
		ProgramGroupID: groupID, Code: "A-1-apply", Name: "Program Apply", DefaultHours: 16,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed prog: %v", err)
	}
	if _, err := queries.CreateRequestTrainingItem(ctx, storagedb.CreateRequestTrainingItemParams{
		RequestRowID: row.ID, ProgramID: prog.ID,
		Status:    requests.ItemStatusValid,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	applyReq := httptest.NewRequest(http.MethodPost,
		"/requests/"+strconv.FormatInt(req.ID, 10)+"/rows/"+strconv.FormatInt(row.ID, 10)+"/apply",
		strings.NewReader(""))
	applyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyReq.Host = "example.com"
	applyReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, applyReq)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}

	got, err := queries.GetRequestRow(ctx, row.ID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if got.Status != requests.RowStatusApplied {
		t.Errorf("row status = %q, want applied", got.Status)
	}
}

func TestE2E_UploadParseApply_AllEntitiesCreated(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	xlsx := buildTestXLSX(t, [][]string{
		{"Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер", "A-1"},
		{"Петров Петр Сергеевич", "98765432100", "petrov@example.com", "Менеджер", "A-1"},
	})

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("employer_id", "1")
	_ = mw.WriteField("received_date", "2026-06-15")
	fw, _ := mw.CreateFormFile("xlsx", "e2e.xlsx")
	_, _ = fw.Write(xlsx)
	_ = mw.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/requests/new", &buf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.Host = "example.com"
	uploadReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, uploadReq)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	reqs, err := queries.ListClientRequests(ctx)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("client_requests = %d (err %v)", len(reqs), err)
	}
	clientReq := reqs[0]
	rows, err := queries.ListRequestRows(ctx, clientReq.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %d (err %v), want 2", len(rows), err)
	}

	for _, row := range rows {
		applyReq := httptest.NewRequest(http.MethodPost,
			"/requests/"+strconv.FormatInt(clientReq.ID, 10)+"/rows/"+strconv.FormatInt(row.ID, 10)+"/apply",
			strings.NewReader(""))
		applyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		applyReq.Host = "example.com"
		applyReq.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, applyReq)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("apply row %d status = %d, want 303; body=%s", row.ID, rec.Code, rec.Body.String())
		}
	}

	workers, err := queries.ListWorkers(ctx)
	if err != nil || len(workers) != 2 {
		t.Errorf("workers = %d, want 2 (err %v)", len(workers), err)
	}

	logs, err := queries.ListActionLogsByEntity(ctx, storagedb.ListActionLogsByEntityParams{
		EntityType: "client_request", EntityID: sql.NullInt64{Int64: clientReq.ID, Valid: true},
	})
	if err != nil || len(logs) == 0 {
		t.Errorf("expected at least one action_log for client_request %d (err %v)", clientReq.ID, err)
	}
}

func ns(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }

// TestApply_POST_RejectsForeignRow asserts the IDOR fix: a row owned by
// request A must not be modifiable through the URL of request B.
// Without the ownership check, any authenticated operator who knows
// a row ID could apply/skip rows in other requests.
//
// 404 (not 400) is the correct status: a foreign row is indistinguishable
// from a non-existent row to the operator — leaking 400 would confirm
// that rowID exists in some other request.
func TestApply_POST_RejectsForeignRow(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	ctx := context.Background()
	now := "2026-06-22T12:00:00Z"
	// Two distinct requests.
	reqA, err := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-15", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed reqA: %v", err)
	}
	reqB, err := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-16", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed reqB: %v", err)
	}
	// Row belongs to reqA; we will try to apply it via reqB's URL.
	row, err := queries.CreateRequestRow(ctx, storagedb.CreateRequestRowParams{
		ClientRequestID: reqA.ID, RowNumber: 1, RawData: "{}",
		RawFullName:    ns("Иванов Иван Иванович"),
		ParsedLastName: ns("Иванов"), ParsedFirstName: ns("Иван"), ParsedMiddleName: ns("Иванович"),
		ParsedSnils: ns("12345678900"), ParsedEmail: ns("ivanov@example.com"),
		ParsedPosition: ns("Инженер"),
		Status:         requests.RowStatusParsed,
		CreatedAt:      now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// Attempt: apply reqA's row through reqB's URL.
	applyReq := httptest.NewRequest(http.MethodPost,
		"/requests/"+strconv.FormatInt(reqB.ID, 10)+"/rows/"+strconv.FormatInt(row.ID, 10)+"/apply",
		strings.NewReader(""))
	applyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyReq.Host = "example.com"
	applyReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, applyReq)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (ownership mismatch must look like 'not found')", rec.Code)
	}

	// The row must remain untouched.
	got, err := queries.GetRequestRow(ctx, row.ID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if got.Status != requests.RowStatusParsed {
		t.Errorf("row status changed to %q through foreign URL — IDOR!", got.Status)
	}
}

func TestSkip_POST_RejectsForeignRow(t *testing.T) {
	t.Parallel()
	router, sm, queries, _ := mountTestRouter(t)
	sessionCookie := authenticate(t, router, sm)

	ctx := context.Background()
	now := "2026-06-22T12:00:00Z"
	reqA, _ := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-15", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	reqB, _ := queries.CreateClientRequest(ctx, storagedb.CreateClientRequestParams{
		EmployerID: 1, ReceivedDate: "2026-06-16", SourceType: "xlsx",
		SourceImportID: sql.NullInt64{}, Status: "review",
		Notes: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
	})
	row, _ := queries.CreateRequestRow(ctx, storagedb.CreateRequestRowParams{
		ClientRequestID: reqA.ID, RowNumber: 1, RawData: "{}",
		RawFullName:    ns("Петров Пётр"),
		ParsedLastName: ns("Петров"), ParsedFirstName: ns("Пётр"),
		ParsedSnils: ns("98765432100"), ParsedEmail: ns("petrov@example.com"),
		ParsedPosition: ns("Менеджер"),
		Status:         requests.RowStatusParsed,
		CreatedAt:      now, UpdatedAt: now,
	})

	skipReq := httptest.NewRequest(http.MethodPost,
		"/requests/"+strconv.FormatInt(reqB.ID, 10)+"/rows/"+strconv.FormatInt(row.ID, 10)+"/skip",
		strings.NewReader(""))
	skipReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	skipReq.Host = "example.com"
	skipReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, skipReq)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	got, _ := queries.GetRequestRow(ctx, row.ID)
	if got.Status != requests.RowStatusParsed {
		t.Errorf("row status changed to %q through foreign URL — IDOR!", got.Status)
	}
}
