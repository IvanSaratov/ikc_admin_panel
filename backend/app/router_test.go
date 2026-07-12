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

func TestFrontendRouteFallsBackToSPAIndex(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/programs", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<div id="root"></div>`) {
		t.Fatalf("body does not contain SPA root: %s", body)
	}
	if !strings.Contains(body, `<script type="module" src="/assets/app.js"></script>`) {
		t.Fatalf("body does not contain SPA asset entrypoint: %s", body)
	}
}

func TestFrontendLoginHeadFallsBackToSPAIndex(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodHead, "/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD /login status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("HEAD /login Content-Type = %q, want text/html", ct)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD /login body length = %d, want 0", rec.Body.Len())
	}
}

func TestRequestLoggingMiddlewareWritesSafeFields(t *testing.T) {
	var logBuf bytes.Buffer
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(&logBuf)

	router, _ := newTestRouterWithDBAndLog(t, logger)
	cookies := testLoginPOST(t, router)

	rec := authedGET(t, router, "/protocols/1/download?run=1&csrf_token=test-password", cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /protocols/1/download status = %d, want 404", rec.Code)
	}

	var requestLog map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logBuf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		if entry["method"] == http.MethodGet && entry["path"] == "/protocols/1/download" {
			requestLog = entry
			break
		}
	}
	if requestLog == nil {
		t.Fatalf("GET /protocols/1/download log entry not found in: %s", logBuf.String())
	}
	if requestLog["status"] != float64(http.StatusNotFound) {
		t.Fatalf("status = %v, want 404", requestLog["status"])
	}
	if requestLog["actor"] != "admin" {
		t.Fatalf("actor = %v, want admin", requestLog["actor"])
	}
	if _, ok := requestLog["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing from log: %#v", requestLog)
	}
	if _, ok := requestLog["remote_ip"]; !ok {
		t.Fatalf("remote_ip missing from log: %#v", requestLog)
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
	return newTestRouterWithFrontend(t, logger, app.FrontendConfig{
		Mode: app.FrontendEmbedded,
		Assets: fstest.MapFS{
			"index.html":    {Data: []byte(`<!doctype html><html><head><title>IKC Expert Mintrud Admin</title><script type="module" src="/assets/app.js"></script></head><body><div id="root"></div></body></html>`)},
			"assets/app.js": {Data: []byte(`console.log("spa test entrypoint");`)},
		},
	})
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

// testLoginPOST performs a real JSON login and returns the session cookies
// that subsequent authenticated requests should reuse.
func testLoginPOST(t *testing.T, router http.Handler) []*http.Cookie {
	t.Helper()

	postReq := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"admin","password":"test-password"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/login status = %d, want 200; body=%s", postRec.Code, postRec.Body.String())
	}
	return postRec.Result().Cookies()
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
