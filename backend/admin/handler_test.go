package admin_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/csrf"
	"golang.org/x/crypto/bcrypt"
)

// newTestHandler is the shared fixture for handler-level tests:
//   - real scs.SessionManager
//   - real gorilla/csrf middleware with a fixed test key
//   - real audit.Service
//   - real admin.Handler
//
// It also seeds a known user so the login flow has something to verify.
func newTestHandler(t *testing.T) (*admin.Handler, *scs.SessionManager, *audit.Service, http.Handler) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "handler-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	queries := storagedb.New(db)
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login:        "alice",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sm := scs.New()
	sm.Lifetime = time.Hour
	sm.Cookie.Name = "handler_session"
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

	auditSvc := audit.NewService(queries)
	svc := admin.NewService(queries)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := admin.NewHandler(svc, auditSvc, sm, logger)
	admin.SetDefaultHandler(h)

	// Build a router that mirrors app.NewRouter: scs LoadAndSave + CSRF
	// globally, then /login public, everything else authed.
	router := http.NewServeMux()
	router.HandleFunc("GET /login", h.GetLogin)
	router.HandleFunc("POST /login", h.PostLogin)

	mount := csrfMW(sm.LoadAndSave(router))

	return h, sm, auditSvc, mount
}

func TestLogin_GET_RendersForm(t *testing.T) {
	t.Parallel()

	_, _, _, mount := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sign in") {
		t.Errorf("body missing 'Sign in': %s", body)
	}
	if !strings.Contains(body, `name="csrf_token"`) {
		t.Errorf("body missing csrf field: %s", body)
	}
	if !strings.Contains(body, `name="login"`) {
		t.Errorf("body missing login field: %s", body)
	}
}

func TestLogin_POST_Success_Redirects(t *testing.T) {
	t.Parallel()

	_, sm, _, mount := newTestHandler(t)

	// Fetch login form to acquire a CSRF token + cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	mount.ServeHTTP(getRec, getReq)

	cookies := getRec.Result().Cookies()
	body := getRec.Body.String()
	formToken := extractFormValue(t, body, "csrf_token")

	// POST login. Use url.Values{}.Encode() to percent-encode the
	// '+', '/', '=' base64 characters in the masked CSRF token so
	// the form parser preserves them verbatim.
	formValues := url.Values{}
	formValues.Set("login", "alice")
	formValues.Set("password", "test-password")
	formValues.Set("csrf_token", formToken)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(formValues.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com/login")
	req.Host = "example.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}

	// Now the session should have user_id; load and check.
	loadReq := httptest.NewRequest(http.MethodGet, "/_check", nil)
	var sessionCookieValue string
	for _, c := range rec.Result().Cookies() {
		loadReq.AddCookie(c)
		if c.Name == sm.Cookie.Name {
			sessionCookieValue = c.Value
		}
	}
	ctx, _ := sm.Load(loadReq.Context(), sessionCookieValue)
	if sm.GetInt64(ctx, admin.SessionKeyUserID) == 0 {
		t.Errorf("session has no user_id after login")
	}
	if got := sm.GetString(ctx, admin.SessionKeyUserLogin); got != "alice" {
		t.Errorf("session user_login = %q, want alice", got)
	}
}

func TestLogin_POST_InvalidCredentials_RerendersForm(t *testing.T) {
	t.Parallel()

	_, _, _, mount := newTestHandler(t)

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	mount.ServeHTTP(getRec, getReq)

	cookies := getRec.Result().Cookies()
	body := getRec.Body.String()
	formToken := extractFormValue(t, body, "csrf_token")

	formValues := url.Values{}
	formValues.Set("login", "alice")
	formValues.Set("password", "WRONG")
	formValues.Set("csrf_token", formToken)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(formValues.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com/login")
	req.Host = "example.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render)", rec.Code)
	}
	body2 := rec.Body.String()
	if !strings.Contains(body2, "Invalid login or password") {
		t.Errorf("body missing error msg: %s", body2)
	}
	if !strings.Contains(body2, `name="csrf_token"`) {
		t.Errorf("body missing csrf field on re-render: %s", body2)
	}
}

func TestLogout_GET_DestroysSession_Redirects(t *testing.T) {
	t.Parallel()

	h, sm, _, mount := newTestHandler(t)

	// Seed session via scs LoadAndSave round-trip.
	seedReq := httptest.NewRequest(http.MethodGet, "/_seed", nil)
	seedCtx, _ := sm.Load(seedReq.Context(), "")
	sm.Put(seedCtx, admin.SessionKeyUserID, 1)
	sm.Put(seedCtx, admin.SessionKeyUserLogin, "alice")
	seedReq = seedReq.WithContext(seedCtx)
	seedRec := httptest.NewRecorder()
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(seedRec, seedReq)

	var sessionCookie *http.Cookie
	for _, c := range seedRec.Result().Cookies() {
		if c.Name == sm.Cookie.Name {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("no session cookie issued")
	}

	// Call PostLogout through scs LoadAndSave so the request context
	// has session data attached (otherwise scs panics on GetString).
	logoutReq := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.Header.Set("Referer", "http://example.com/programs")
	logoutReq.Host = "example.com"
	logoutRec := httptest.NewRecorder()
	wrapped := sm.LoadAndSave(http.HandlerFunc(h.PostLogout))
	wrapped.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", logoutRec.Code)
	}
	if loc := logoutRec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}

	// After destroy, GetInt64 should return 0.
	if id := sm.GetInt64(seedCtx, admin.SessionKeyUserID); id != 0 {
		t.Errorf("user_id after destroy = %d, want 0", id)
	}

	_ = mount
}

func extractCookieValue(t *testing.T, cookies []*http.Cookie, name string) string {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	t.Fatalf("no cookie %s", name)
	return ""
}

func extractFormValue(t *testing.T, body, name string) string {
	t.Helper()
	idx := strings.Index(body, `name="`+name+`"`)
	if idx < 0 {
		t.Fatalf("form field %q not in body", name)
	}
	rest := body[idx:]
	valIdx := strings.Index(rest, `value="`)
	if valIdx < 0 {
		t.Fatalf("form field %q has no value", name)
	}
	rest = rest[valIdx+len(`value="`):]
	end := strings.Index(rest, `"`)
	return rest[:end]
}

func TestIsSafeRedirect(t *testing.T) {
	t.Skip("moved to internal_test.go (package admin) — handler_test.go is external and cannot reach unexported isSafeRedirect")
}

func TestErrUserDisabledIsErrInvalidCredentials(t *testing.T) {
	t.Skip("moved to internal_test.go (package admin) — handler_test.go is external and cannot reach unexported sentinels")
}
