package audit_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/gorilla/csrf"
	"golang.org/x/crypto/bcrypt"
)

// openAndMigrate creates a fresh SQLite DB at path and applies all
// embedded migrations. Used by the handler tests so each test gets an
// isolated action_log table to seed.
func openAndMigrate(path string) error {
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		return err
	}
	defer db.Close()
	return storage.Migrate(db)
}

// openDB opens an already-migrated DB at path and registers a cleanup
// hook that closes it when the test ends. Returns the raw *sql.DB so
// callers can seed rows through their own queries / inserts.
func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// routerWithDB wires the real app.NewRouter around an open DB. The
// router has scs LoadAndSave + CSRF + auth + audit handler — the full
// production stack. Tests log in via loginRoundTrip and then make
// authedGet calls.
func routerWithDB(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	sessions := admin.NewSessionManager(admin.SessionConfig{
		TTL:      8 * time.Hour,
		SameSite: 0,
		Secure:   false,
	})
	csrfKey := []byte("0123456789abcdef0123456789abcdef")
	csrfMW := csrf.Protect(csrfKey,
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.FieldName("csrf_token"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.CookieName("csrf_token"),
		csrf.Path("/"),
	)

	queries := storagedb.New(db)
	auditSvc := audit.NewService(queries)
	adminSvc := admin.NewService(queries)
	adminHandler := admin.NewHandler(adminSvc, auditSvc, sessions, nil)
	admin.SetDefaultHandler(adminHandler)

	return app.NewRouter(app.Deps{
		Database: db,
		Sessions: sessions,
		CSRF:     csrfMW,
	})
}

// bcryptGenerateHash produces a low-cost bcrypt hash for the given
// password. The audit handler tests only need the password to round-
// trip through LoginHandler; they never compare against a real
// stolen password, so bcrypt.MinCost is acceptable here.
func bcryptGenerateHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// csrfPlainHTTPRequest wraps the csrf.PlaintextHTTPRequest helper.
// We re-export it under a short name so the test body stays readable.
func csrfPlainHTTPRequest(r *http.Request) *http.Request {
	return csrf.PlaintextHTTPRequest(r)
}

// seedAdminUser inserts the admin login used by all audit handler
// tests. Called from the fixtures that build a router.
func seedAdminUser(t *testing.T, db *sql.DB) {
	t.Helper()
	hash, err := bcryptGenerateHash("test-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	queries := storagedb.New(db)
	if _, err := queries.CreateUser(context.Background(), storagedb.CreateUserParams{
		Login:        "admin",
		PasswordHash: hash,
		Role:         "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
}

// pathInTempDir is a tiny helper to keep the temp-file names readable.
// It is used in the few places where the test file would otherwise
// compose an awkward filepath.Join inline.
func pathInTempDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// loginRoundTrip performs a real /login POST against the router and
// returns the cookies (session + CSRF) that subsequent authenticated
// requests should reuse. Mirrors the same pattern used in
// app/router_test.go but lives here so the audit tests stay
// self-contained.
func loginRoundTrip(t *testing.T, router http.Handler) []*http.Cookie {
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
		t.Fatalf("no csrf_token cookie issued")
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

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://example.com/login")
	req.Host = "example.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req = csrf.PlaintextHTTPRequest(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /login status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	final := append(cookies, rec.Result().Cookies()...)
	return final
}

// findCookie is a tiny helper that returns the named cookie or nil.
// Kept here (rather than inlined) because loginRoundTrip used to share
// it before seedSessionCookies was removed.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
