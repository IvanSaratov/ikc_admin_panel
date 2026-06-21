package admin_test

import (
	"context"
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

// newIntegrationStack builds a router that mirrors production:
//   - scs LoadAndSave (cookie I/O) on every request
//   - gorilla/csrf on every request
//   - /login public, everything else authed via admin.RequireAuth
//
// One known user "alice" is seeded with password "test-password".
func newIntegrationStack(t *testing.T) (http.Handler, *scs.SessionManager, *audit.Service) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "int-test.db"))
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
	sm.Cookie.Name = "int_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"

	csrfKey := []byte("abcdef0123456789abcdef0123456789")
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
	h := admin.NewHandler(svc, auditSvc, sm, nil)
	admin.SetDefaultHandler(h)

	authMW := admin.RequireAuth(sm, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.GetLogin)
	mux.HandleFunc("POST /login", h.PostLogin)

	// Wrap mux with auth for /protected routes.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected-ok"))
	})
	protected.HandleFunc("POST /protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected-ok"))
	})
	protectedMux := authMW(protected)

	root := http.NewServeMux()
	root.Handle("/protected", protectedMux)
	root.Handle("/login", mux)
	root.HandleFunc("GET /logout", h.PostLogout)

	mount := csrfMW(sm.LoadAndSave(root))
	return mount, sm, auditSvc
}

func TestProtectedRoute_WithoutSession_302(t *testing.T) {
	t.Parallel()

	mount, _, _ := newIntegrationStack(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login?next=/protected" {
		t.Errorf("Location = %q, want /login?next=/protected", loc)
	}
}

func TestProtectedRoute_WithSession_200(t *testing.T) {
	t.Parallel()

	mount, sm, _ := newIntegrationStack(t)

	// Seed session via scs LoadAndSave round-trip.
	seedReq := httptest.NewRequest(http.MethodGet, "/_seed", nil)
	seedCtx, _ := sm.Load(seedReq.Context(), "")
	sm.Put(seedCtx, admin.SessionKeyUserID, int64(1))
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
		t.Fatalf("no session cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(sessionCookie)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "protected-ok") {
		t.Errorf("body missing handler output: %s", rec.Body.String())
	}
}

func TestPOST_WithoutCSRFToken_403(t *testing.T) {
	t.Parallel()

	mount, sm, _ := newIntegrationStack(t)

	seedReq := httptest.NewRequest(http.MethodGet, "/_seed", nil)
	seedCtx, _ := sm.Load(seedReq.Context(), "")
	sm.Put(seedCtx, admin.SessionKeyUserID, int64(1))
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
		t.Fatalf("no session cookie")
	}

	req := httptest.NewRequest(http.MethodPost, "/protected", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	req.Host = "example.com"
	req.Header.Set("Referer", "http://example.com/protected")
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPOST_WithCSRFToken_200(t *testing.T) {
	t.Parallel()

	mount, sm, _ := newIntegrationStack(t)

	seedReq := httptest.NewRequest(http.MethodGet, "/_seed", nil)
	seedCtx, _ := sm.Load(seedReq.Context(), "")
	sm.Put(seedCtx, admin.SessionKeyUserID, int64(1))
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
		t.Fatalf("no session cookie")
	}

	// GET /login first to get a CSRF cookie + token.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getReq.AddCookie(sessionCookie)
	getReq.Host = "example.com"
	getRec := httptest.NewRecorder()
	mount.ServeHTTP(getRec, getReq)

	csrfToken := extractFormValue(t, getRec.Body.String(), "csrf_token")

	// POST /protected with the masked CSRF token in the form body.
	// We must echo back both the session cookie and the CSRF cookie;
	// gorilla/csrf validates the masked token (form value) against
	// the unmasked token in the CSRF cookie. url.Values{}.Encode()
	// percent-encodes the '+' '/' '=' base64 characters so the form
	// parser preserves them verbatim.
	formValues := url.Values{}
	formValues.Set("csrf_token", csrfToken)
	req := httptest.NewRequest(http.MethodPost, "/protected", strings.NewReader(formValues.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			req.AddCookie(c)
		}
	}
	req.Host = "example.com"
	req.Header.Set("Referer", "http://example.com/protected")
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "protected-ok") {
		t.Errorf("body missing handler output: %s", rec.Body.String())
	}
}
