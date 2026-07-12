package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

func TestProgramsPageReturnsOperatorShell(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/programs", cookies)
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
	cookies := testLoginPOST(t, router)

	rec := authedPOST(t, router, "/programs/groups", "code=A&name="+url.QueryEscape("Охрана труда"), cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/programs", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "Охрана труда") {
		t.Fatalf("body does not contain created group: %s", body)
	}
}

func TestCreateProgramRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	authedPOST(t, router, "/programs/groups", "code=A&name="+url.QueryEscape("Охрана труда"), cookies)

	rec := authedPOST(t, router, "/programs",
		"program_group_id=1&code=A-1&name="+url.QueryEscape("Общие вопросы охраны труда")+"&default_hours=40",
		cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/programs", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "Общие вопросы охраны труда") {
		t.Fatalf("body does not contain created program: %s", body)
	}
}

func TestCreateEmployerRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedPOST(t, router, "/employers", "inn=7700000000&canonical_name="+url.QueryEscape("ООО Ромашка"), cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/employers", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "ООО Ромашка") {
		t.Fatalf("body does not contain created employer: %s", body)
	}
}

func TestCreateWorkerRedirectsAndPersists(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedPOST(t, router, "/workers",
		"last_name="+url.QueryEscape("Петров")+"&first_name="+url.QueryEscape("Петр")+
			"&snils=123-456-789+00&email=worker@example.test", cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/workers", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "Петров") {
		t.Fatalf("body does not contain created worker: %s", body)
	}
}

func TestAssignEmployerRedirectsAndShowsAssignment(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	authedPOST(t, router, "/employers", "inn=7700000000&canonical_name="+url.QueryEscape("ООО Ромашка"), cookies)

	authedPOST(t, router, "/workers",
		"last_name="+url.QueryEscape("Петров")+"&first_name="+url.QueryEscape("Петр")+
			"&snils=123-456-789+00&email=worker@example.test", cookies)

	rec := authedPOST(t, router, "/workers/assignments",
		"worker_id=1&employer_id=1&current_position="+url.QueryEscape("Инженер"), cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/workers", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "Инженер") || !strings.Contains(body, "ООО Ромашка") {
		t.Fatalf("body does not contain assignment details: %s", body)
	}
}

func TestValidationResponseIncludesFieldMessage(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedPOST(t, router, "/employers", "canonical_name="+url.QueryEscape("ООО Без ИНН"), cookies)
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
func seedGroup(t *testing.T, router http.Handler, cookies []*http.Cookie) {
	t.Helper()
	authedPOST(t, router, "/programs/groups",
		"code=A&name="+url.QueryEscape("Охрана труда"), cookies)
}

func TestEdit_GET_ReturnsForm_200(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)
	seedGroup(t, router, cookies)

	rec := authedGET(t, router, "/programs/groups/1/edit", cookies)
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
	cookies := testLoginPOST(t, router)
	seedGroup(t, router, cookies)

	rec := authedPOST(t, router, "/programs/groups/1/edit",
		"code=A&name="+url.QueryEscape("Renamed group"), cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/programs", cookies)
	if !strings.Contains(rec.Body.String(), "Renamed group") {
		t.Fatalf("updated name not visible: %s", rec.Body.String())
	}
}

func TestDeactivate_POST_ChangesStatus_Redirects(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)
	seedGroup(t, router, cookies)

	rec := authedPOST(t, router, "/programs/groups/1/deactivate", "", cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}

	rec = authedGET(t, router, "/programs", cookies)
	if !strings.Contains(rec.Body.String(), "inactive") {
		t.Fatalf("status not reflected on list: %s", rec.Body.String())
	}
}

func TestDetail_GET_Returns200_WithChildren(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	authedPOST(t, router, "/employers", "inn=7700000000&canonical_name="+url.QueryEscape("ООО Ромашка"), cookies)
	authedPOST(t, router, "/workers",
		"last_name="+url.QueryEscape("Петров")+"&first_name="+url.QueryEscape("Петр")+
			"&snils=123-456-789+00&email=worker@example.test", cookies)
	authedPOST(t, router, "/workers/assignments",
		"worker_id=1&employer_id=1&current_position="+url.QueryEscape("Инженер"), cookies)

	rec := authedGET(t, router, "/employers/1", cookies)
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

	rec = authedGET(t, router, "/workers/1", cookies)
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
	cookies := testLoginPOST(t, router)

	authedPOST(t, router, "/employers", "inn=7700000000&canonical_name="+url.QueryEscape("ООО Ромашка"), cookies)
	authedPOST(t, router, "/employers/1", "inn=7700000000&canonical_name="+url.QueryEscape("ООО Ромашка+"), cookies)
	authedPOST(t, router, "/employers/1/deactivate", "", cookies)

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

func TestRequestLoggingMiddlewareWritesSafeFields(t *testing.T) {
	var logBuf bytes.Buffer
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(&logBuf)

	router, _ := newTestRouterWithDBAndLog(t, logger)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/programs?csrf_token=test-password", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /programs status = %d, want 200", rec.Code)
	}

	var programsLog map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logBuf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		if entry["method"] == http.MethodGet && entry["path"] == "/programs" {
			programsLog = entry
			break
		}
	}
	if programsLog == nil {
		t.Fatalf("GET /programs log entry not found in: %s", logBuf.String())
	}
	if programsLog["status"] != float64(http.StatusOK) {
		t.Fatalf("status = %v, want 200", programsLog["status"])
	}
	if programsLog["actor"] != "admin" {
		t.Fatalf("actor = %v, want admin", programsLog["actor"])
	}
	if _, ok := programsLog["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing from log: %#v", programsLog)
	}
	if _, ok := programsLog["remote_ip"]; !ok {
		t.Fatalf("remote_ip missing from log: %#v", programsLog)
	}

	logs := logBuf.String()
	for _, forbidden := range []string{"test-password", "csrf_token"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("log contains forbidden value %q: %s", forbidden, logs)
		}
	}
}

func TestFrontendDisabled_DoesNotServeSPA(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)

	req := httptest.NewRequest(http.MethodGet, "/protocols/1/workflow", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("disabled frontend served SPA HTML")
	}
}

func TestFrontendEmbedded_ServesSPA(t *testing.T) {
	t.Parallel()

	router, _ := newTestRouterWithFrontend(t, nil, app.FrontendConfig{
		Mode: app.FrontendEmbedded,
		Assets: fstest.MapFS{
			"index.html": {Data: []byte("<!doctype html><title>React</title>")},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protocols/1/workflow", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("body does not contain SPA HTML: %s", rec.Body.String())
	}
}

func TestSessionAPI_ReturnsJSON(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"authenticated":false}` {
		t.Fatalf("body = %s", body)
	}
}

func TestAPILogin_WorksWithoutCSRFToken(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"admin","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool   `json:"authenticated"`
		Login         string `json:"login"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if !body.Authenticated || body.Login != "admin" {
		t.Fatalf("body = %+v, want authenticated admin", body)
	}
}

func TestLegacyLogin_RequiresCSRFToken(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)

	form := url.Values{}
	form.Set("login", "admin")
	form.Set("password", "test-password")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com/login")
	req.Host = "example.com"
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /login status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPILogin_RateLimitedWhenConfigured(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithLoginRate(t, admin.NewRateLimiter(1, time.Hour, nil))

	first := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"admin","password":"WRONG"}`))
	first.Header.Set("Content-Type", "application/json")
	first.RemoteAddr = "203.0.113.9:1234"
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusUnauthorized {
		t.Fatalf("first POST /api/login status = %d, want 401; body=%s", firstRec.Code, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"admin","password":"WRONG"}`))
	second.Header.Set("Content-Type", "application/json")
	second.RemoteAddr = "203.0.113.9:5678"
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second POST /api/login status = %d, want 429; body=%s", secondRec.Code, secondRec.Body.String())
	}
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithDB(t)
	return router
}

// newTestRouterWithDB returns the router and the underlying *sql.DB so
// tests can inspect the action_log table directly. The router is wired
// with a real session manager and a CSRF middleware (using a fixed test
// key) so all routes behave the way they do in production.
func newTestRouterWithDB(t *testing.T) (http.Handler, *sql.DB) {
	return newTestRouterWithDBAndLog(t, nil)
}

func newTestRouterWithDBAndLog(t *testing.T, logger logrus.FieldLogger) (http.Handler, *sql.DB) {
	return newTestRouterWithFrontend(t, logger, app.FrontendConfig{})
}

func newTestRouterWithFrontendMode(t *testing.T, mode app.FrontendMode) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithFrontend(t, nil, app.FrontendConfig{Mode: mode})
	return router
}

func newTestRouterWithFrontend(t *testing.T, logger logrus.FieldLogger, frontend app.FrontendConfig) (http.Handler, *sql.DB) {
	return newTestRouterWithFrontendAndLoginRate(t, logger, frontend, nil)
}

func newTestRouterWithLoginRate(t *testing.T, loginRate *admin.RateLimiter) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithFrontendAndLoginRate(t, nil, app.FrontendConfig{Mode: app.FrontendDisabled}, loginRate)
	return router
}

func newTestRouterWithFrontendAndLoginRate(t *testing.T, logger logrus.FieldLogger, frontend app.FrontendConfig, loginRate *admin.RateLimiter) (http.Handler, *sql.DB) {
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

	if err := seedAdminUser(t, db); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// Wire session manager with sane test defaults.
	sessions := admin.NewSessionManager(admin.SessionConfig{
		TTL:      8 * time.Hour,
		SameSite: 0,
		Secure:   false,
	})

	// CSRF with a fixed key so test runs are deterministic.
	csrfKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	csrfMW := csrf.Protect(csrfKey,
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.FieldName("csrf_token"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.CookieName("csrf_token"),
		csrf.Path("/"),
	)

	return app.NewRouter(app.Deps{
		Database:  db,
		Sessions:  sessions,
		CSRF:      csrfMW,
		LoginRate: loginRate,
		Log:       logger,
		Frontend:  frontend,
	}), db
}

// seedAdminUser inserts a known admin user into the freshly-migrated
// test database. bcrypt.MinCost keeps seeding fast across thousands of
// tests.
func seedAdminUser(t *testing.T, db *sql.DB) error {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	queries := storagedb.New(db)
	_, err = queries.CreateUser(context.Background(), storagedb.CreateUserParams{
		Login:        "admin",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") {
			return err
		}
	}
	_ = admin.BootstrapAdminLogin
	return nil
}

// testLoginPOST performs a real login round-trip and returns the cookies
// (session + CSRF) that subsequent authenticated requests should reuse.
func testLoginPOST(t *testing.T, router http.Handler) []*http.Cookie {
	t.Helper()

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /login status = %d, want 200", getRec.Code)
	}

	cookies := getRec.Result().Cookies()

	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("no csrf_token cookie in response")
	}

	body := getRec.Body.String()
	idx := strings.Index(body, `name="csrf_token"`)
	if idx < 0 {
		t.Fatalf("login form has no csrf field: %s", body)
	}
	rest := body[idx:]
	valIdx := strings.Index(rest, `value="`)
	if valIdx < 0 {
		t.Fatalf("csrf field has no value: %s", rest)
	}
	rest = rest[valIdx+len(`value="`):]
	endIdx := strings.Index(rest, `"`)
	if endIdx < 0 {
		t.Fatalf("csrf value not terminated: %s", rest)
	}
	token := rest[:endIdx]

	form := url.Values{}
	form.Set("login", "admin")
	form.Set("password", "test-password")
	form.Set("csrf_token", token)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Referer", "http://example.com/login")
	postReq.Host = "example.com"
	for _, c := range cookies {
		postReq.AddCookie(c)
	}
	postReq = csrf.PlaintextHTTPRequest(postReq)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /login status = %d, want 303; body=%s", postRec.Code, postRec.Body.String())
	}

	final := append(cookies, postRec.Result().Cookies()...)
	return final
}

func authedGET(t *testing.T, router http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func authedPOST(t *testing.T, router http.Handler, path string, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	// gorilla/csrf masks the token per-request, so we must GET the page
	// that contains the form first, parse the masked token out of the
	// rendered HTML, and then POST with that exact token. The CSRF
	// cookie itself (set by the middleware) carries the unmasked base
	// token; the form field carries (OTP XOR base) for that render.
	pageRec := authedGET(t, router, formReferrerPath(path), cookies)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("GET for CSRF page: status %d body %s", pageRec.Code, pageRec.Body.String())
	}
	token := extractCSRFToken(t, pageRec.Body.String())

	form := url.Values{}
	if body != "" {
		parsed, err := url.ParseQuery(body)
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		for k, vs := range parsed {
			if k == "csrf_token" {
				continue
			}
			for _, v := range vs {
				form.Set(k, v)
			}
		}
	}
	form.Set("csrf_token", token)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com"+path)
	req.Host = "example.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// formReferrerPath maps a POST target back to the page that renders the
// form. For list-page POSTs this is the list URL; for per-row POSTs
// (deactivate / edit) we use the list URL too — same domain.
func formReferrerPath(path string) string {
	switch {
	case path == "/login":
		return "/login"
	case path == "/programs/groups":
		return "/programs"
	case path == "/programs":
		return "/programs"
	case path == "/employers":
		return "/employers"
	case path == "/workers":
		return "/workers"
	case path == "/workers/assignments":
		return "/workers"
	case strings.HasPrefix(path, "/programs/"):
		// /programs/groups/{id}/edit, /programs/groups/{id}/deactivate,
		// /programs/{id}/edit, /programs/{id}/deactivate — list page.
		return "/programs"
	case strings.HasPrefix(path, "/employers/"):
		// /employers/{id} (POST is the edit endpoint), deactivate, etc.
		return "/employers"
	case strings.HasPrefix(path, "/workers/"):
		return "/workers"
	default:
		return path
	}
}

// extractCSRFToken pulls the value attribute out of the hidden
// <input type="hidden" name="csrf_token" value="..."> field rendered
// by components.CSRFField.
func extractCSRFToken(t *testing.T, body string) string {
	t.Helper()
	idx := strings.Index(body, `name="csrf_token"`)
	if idx < 0 {
		t.Fatalf("body has no csrf_token field: %s", body)
	}
	rest := body[idx:]
	valIdx := strings.Index(rest, `value="`)
	if valIdx < 0 {
		t.Fatalf("csrf_token field has no value: %s", rest)
	}
	rest = rest[valIdx+len(`value="`):]
	endIdx := strings.Index(rest, `"`)
	if endIdx < 0 {
		t.Fatalf("csrf_token value not terminated: %s", rest)
	}
	return rest[:endIdx]
}

// _ keeps the csrf package referenced when future tests need it directly.
var _ = csrf.TemplateField
