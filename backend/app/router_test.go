package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
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
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/gorilla/csrf"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logBuf),
		zapcore.InfoLevel,
	))

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

func TestImportAPIRequestLogContainsResponseTraceID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logBuf),
		zapcore.InfoLevel,
	))

	router, _ := newTestRouterWithDBAndLog(t, logger)
	cookies := testLoginPOST(t, router)
	recorder := authedGET(t, router, "/api/imports?cursor=private-cursor&limit=10", cookies)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/imports status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	responseTraceID := recorder.Header().Get("X-Trace-ID")
	if responseTraceID == "" {
		t.Fatal("X-Trace-ID response header is empty")
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
		if entry["method"] == http.MethodGet && entry["path"] == "/api/imports" {
			requestLog = entry
			break
		}
	}
	if requestLog == nil {
		t.Fatalf("GET /api/imports log entry not found in: %s", logBuf.String())
	}
	if requestLog["trace_id"] != responseTraceID {
		t.Fatalf("trace_id = %v, want %q", requestLog["trace_id"], responseTraceID)
	}
	if requestLog["actor"] != "admin" {
		t.Fatalf("actor = %v, want admin", requestLog["actor"])
	}
	for _, forbidden := range []string{"private-cursor", "cursor", "limit"} {
		if strings.Contains(logBuf.String(), forbidden) {
			t.Fatalf("log contains forbidden value %q: %s", forbidden, logBuf.String())
		}
	}
}

func TestUnauthenticatedImportAPIRequestIsLoggedWithTraceID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logBuf),
		zapcore.InfoLevel,
	))

	router, _ := newTestRouterWithDBAndLog(t, logger)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/imports", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/imports status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	responseTraceID := recorder.Header().Get("X-Trace-ID")
	if responseTraceID == "" {
		t.Fatal("X-Trace-ID response header is empty")
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
		if entry["method"] == http.MethodGet && entry["path"] == "/api/imports" {
			requestLog = entry
			break
		}
	}
	if requestLog == nil {
		t.Fatalf("GET /api/imports log entry not found in: %s", logBuf.String())
	}
	if requestLog["status"] != float64(http.StatusUnauthorized) {
		t.Fatalf("status = %v, want 401", requestLog["status"])
	}
	if requestLog["trace_id"] != responseTraceID {
		t.Fatalf("trace_id = %v, want %q", requestLog["trace_id"], responseTraceID)
	}
	if _, exists := requestLog["actor"]; exists {
		t.Fatalf("unauthenticated request log contains actor: %#v", requestLog)
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

func TestImportAPIRequiresJSONAuthentication(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/imports", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "application/problem+json") {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestImportAPIRolesCSRFAndUploadFlow(t *testing.T) {
	router, database := newTestRouterWithDBAndLog(t, zap.NewNop())
	queries := storagedb.New(database)
	now := time.Now().UTC().Format(time.RFC3339)
	hash, err := bcrypt.GenerateFromPassword([]byte("operator-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash operator password: %v", err)
	}
	if _, err := queries.CreateUser(context.Background(), storagedb.CreateUserParams{
		Login:        "operator",
		PasswordHash: string(hash),
		Role:         "operator",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create operator: %v", err)
	}

	operatorCookies := testLoginCredentials(t, router, "operator", "operator-password")
	operatorList := authedGET(t, router, "/api/imports", operatorCookies)
	if operatorList.Code != http.StatusOK {
		t.Fatalf("operator GET status = %d, body=%s", operatorList.Code, operatorList.Body.String())
	}
	operatorUpload := importUploadRequest(t, syntheticLegacyWorkbook(t, "operator"))
	addCookies(operatorUpload, operatorCookies)
	operatorRecorder := httptest.NewRecorder()
	router.ServeHTTP(operatorRecorder, operatorUpload)
	if operatorRecorder.Code != http.StatusForbidden || !strings.Contains(operatorRecorder.Body.String(), `"code":"forbidden"`) {
		t.Fatalf("operator POST status = %d, body=%s", operatorRecorder.Code, operatorRecorder.Body.String())
	}

	adminCookies := testLoginPOST(t, router)
	withoutToken := importUploadRequest(t, syntheticLegacyWorkbook(t, "without-token"))
	addCookies(withoutToken, adminCookies)
	withoutTokenRecorder := httptest.NewRecorder()
	router.ServeHTTP(withoutTokenRecorder, withoutToken)
	if withoutTokenRecorder.Code != http.StatusForbidden || !strings.Contains(withoutTokenRecorder.Body.String(), `"code":"csrf_failed"`) {
		t.Fatalf("missing CSRF status = %d, body=%s", withoutTokenRecorder.Code, withoutTokenRecorder.Body.String())
	}

	csrfRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/csrf", nil)
	addCookies(csrfRequest, adminCookies)
	csrfRecorder := httptest.NewRecorder()
	router.ServeHTTP(csrfRecorder, csrfRequest)
	if csrfRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/csrf status = %d, body=%s", csrfRecorder.Code, csrfRecorder.Body.String())
	}
	var csrfResponse struct {
		Token string `json:"csrf_token"`
	}
	if err := json.NewDecoder(csrfRecorder.Body).Decode(&csrfResponse); err != nil {
		t.Fatalf("decode CSRF response: %v", err)
	}
	if csrfResponse.Token == "" {
		t.Fatal("empty CSRF token")
	}

	validUpload := importUploadRequest(t, syntheticLegacyWorkbook(t, "first"))
	validUpload.Header.Set("X-CSRF-Token", csrfResponse.Token)
	validUpload.Header.Set("Referer", "http://example.com/api/imports")
	validUpload.Host = "example.com"
	addCookies(validUpload, adminCookies)
	addCookies(validUpload, csrfRecorder.Result().Cookies())
	validRecorder := httptest.NewRecorder()
	router.ServeHTTP(validRecorder, validUpload)
	if validRecorder.Code != http.StatusAccepted {
		t.Fatalf("valid upload status = %d, body=%s", validRecorder.Code, validRecorder.Body.String())
	}
	var firstResponse struct {
		QueuePosition int64 `json:"queue_position"`
	}
	if err := json.NewDecoder(validRecorder.Body).Decode(&firstResponse); err != nil {
		t.Fatalf("decode first upload: %v", err)
	}
	if firstResponse.QueuePosition != 1 {
		t.Fatalf("first queue position = %d, want 1", firstResponse.QueuePosition)
	}

	secondUpload := importUploadRequest(t, syntheticLegacyWorkbook(t, "second"))
	secondUpload.Header.Set("Idempotency-Key", "router-upload-2")
	secondUpload.Header.Set("X-CSRF-Token", csrfResponse.Token)
	secondUpload.Header.Set("Referer", "http://example.com/api/imports")
	secondUpload.Host = "example.com"
	addCookies(secondUpload, adminCookies)
	addCookies(secondUpload, csrfRecorder.Result().Cookies())
	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, secondUpload)
	if secondRecorder.Code != http.StatusAccepted {
		t.Fatalf("second upload status = %d, body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var secondResponse struct {
		QueuePosition int64 `json:"queue_position"`
	}
	if err := json.NewDecoder(secondRecorder.Body).Decode(&secondResponse); err != nil {
		t.Fatalf("decode second upload: %v", err)
	}
	if secondResponse.QueuePosition != 2 {
		t.Fatalf("second queue position = %d, want 2", secondResponse.QueuePosition)
	}
	active, err := queries.CountActiveImports(context.Background())
	if err != nil {
		t.Fatalf("count active imports: %v", err)
	}
	if active != 2 {
		t.Fatalf("active imports = %d, want 2", active)
	}
}

func TestImportAPINotFoundAndMethodNotAllowedReturnProblems(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFrontendMode(t, app.FrontendDisabled)
	cookies := testLoginPOST(t, router)
	tests := []struct {
		method string
		path   string
		status int
		code   string
	}{
		{method: http.MethodGet, path: "/api/imports/1/extra", status: http.StatusNotFound, code: "not_found"},
		{method: http.MethodPut, path: "/api/imports/1", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		addCookies(request, cookies)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
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

func newTestRouterWithDBAndLog(t *testing.T, logger *zap.Logger) (http.Handler, *sql.DB) {
	return newTestRouterWithFrontend(t, logger, app.FrontendConfig{
		Mode: app.FrontendEmbedded,
		Assets: fstest.MapFS{
			"index.html":    {Data: []byte(`<!doctype html><html><head><title>IKC Expert</title><script type="module" src="/assets/app.js"></script></head><body><div id="root"></div></body></html>`)},
			"assets/app.js": {Data: []byte(`console.log("spa test entrypoint");`)},
		},
	})
}

func newTestRouterWithFrontendMode(t *testing.T, mode app.FrontendMode) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithFrontend(t, nil, app.FrontendConfig{Mode: mode})
	return router
}

func newTestRouterWithFrontend(t *testing.T, logger *zap.Logger, frontend app.FrontendConfig) (http.Handler, *sql.DB) {
	return newTestRouterWithFrontendAndLoginRate(t, logger, frontend, nil)
}

func newTestRouterWithLoginRate(t *testing.T, loginRate *admin.RateLimiter) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithFrontendAndLoginRate(t, nil, app.FrontendConfig{Mode: app.FrontendDisabled}, loginRate)
	return router
}

func newTestRouterWithFrontendAndLoginRate(t *testing.T, logger *zap.Logger, frontend app.FrontendConfig, loginRate *admin.RateLimiter) (http.Handler, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "ikc-test.db"))
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
	queries := storagedb.New(db)
	fileStore, err := imports.NewLocalFileStore(filepath.Join(t.TempDir(), "imports"))
	if err != nil {
		t.Fatalf("create import file store: %v", err)
	}
	importService, err := imports.NewService(db, queries, audit.NewService(queries), fileStore, imports.DefaultConfig())
	if err != nil {
		t.Fatalf("create import service: %v", err)
	}

	// Wire session manager with sane test defaults.
	sessions := admin.NewSessionManager(admin.SessionConfig{
		TTL:      8 * time.Hour,
		SameSite: 0,
		Secure:   false,
	})

	// CSRF with a fixed key so test runs are deterministic.
	csrfMW, err := admin.NewCSRFMiddleware(admin.CSRFConfig{
		Key:       strings.Repeat("ab", 32),
		Plaintext: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("create CSRF middleware: %v", err)
	}

	return app.NewRouter(app.Deps{
		Database:      db,
		Sessions:      sessions,
		CSRF:          csrfMW,
		LoginRate:     loginRate,
		Log:           logger,
		Frontend:      frontend,
		ImportService: importService,
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
	return testLoginCredentials(t, router, "admin", "test-password")
}

func testLoginCredentials(t *testing.T, router http.Handler, login, password string) []*http.Cookie {
	t.Helper()

	body, err := json.Marshal(map[string]string{"login": login, "password": password})
	if err != nil {
		t.Fatalf("marshal login request: %v", err)
	}
	postReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/login status = %d, want 200; body=%s", postRec.Code, postRec.Body.String())
	}
	return postRec.Result().Cookies()
}

func addCookies(request *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func importUploadRequest(t *testing.T, workbook []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "legacy.xlsx")
	if err != nil {
		t.Fatalf("create upload part: %v", err)
	}
	if _, err := part.Write(workbook); err != nil {
		t.Fatalf("write upload part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close upload body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://example.com/api/imports/legacy", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Idempotency-Key", "router-upload-1")
	return request
}

func syntheticLegacyWorkbook(t *testing.T, discriminator string) []byte {
	t.Helper()
	workbook := excelize.NewFile()
	t.Cleanup(func() { _ = workbook.Close() })
	profiles := []string{"А", "Б", "В", "П", "С"}
	if err := workbook.SetSheetName("Sheet1", profiles[0]); err != nil {
		t.Fatalf("rename workbook sheet: %v", err)
	}
	headers := []string{
		"Организация", "ИНН", "Ф.И.О.", "Профессия", "Подразделение", "СНИЛС",
		"Номер протокола", "Дата протокола", "Оценка", "Период обучения",
		"Номер в реестре", "Организация/филиал/№ заявки", "Email", "Логин", "Пароль",
	}
	row := []any{
		"Test Organization " + discriminator, "7700000000", "Testov Test Testovich", "Tester",
		"Test Department", "12345678900", "TEST-" + discriminator, "2026-07-16", "passed",
		"01.07.2026-16.07.2026", "registry-test-1", "branch-test-1",
		"worker@example.test", "test-user", "synthetic-password",
	}
	for index, profile := range profiles {
		if index > 0 {
			if _, err := workbook.NewSheet(profile); err != nil {
				t.Fatalf("create sheet %s: %v", profile, err)
			}
		}
		profileHeaders := append([]string(nil), headers...)
		profileRow := append([]any(nil), row...)
		if profile == "В" {
			profileHeaders = append(profileHeaders, "Программа обучения")
			profileRow = append(profileRow, "Test Program")
		}
		for column, value := range profileHeaders {
			cell, _ := excelize.CoordinatesToCellName(column+1, 1)
			if err := workbook.SetCellValue(profile, cell, value); err != nil {
				t.Fatalf("set header: %v", err)
			}
		}
		for column, value := range profileRow {
			cell, _ := excelize.CoordinatesToCellName(column+1, 2)
			if err := workbook.SetCellValue(profile, cell, value); err != nil {
				t.Fatalf("set row: %v", err)
			}
		}
	}
	var output bytes.Buffer
	if err := workbook.Write(&output); err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	return output.Bytes()
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
