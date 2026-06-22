package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

// TestService_LoginHandler_GET covers the frozen-signature Service method
// (section 0.2 of the Core MVP plan). The bridge in handler.go dispatches
// to either GetLogin (GET) or PostLogin (POST) on the registered default
// handler. We exercise both verbs to lock the dispatch behaviour.
func TestService_LoginHandler_GET(t *testing.T) {
	SetDefaultHandler(newFrozenTestHandler(t))
	t.Cleanup(func() { SetDefaultHandler(nil) })

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	(&Service{}).LoginHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Sign in") {
		t.Errorf("GET dispatch did not reach GetLogin (form not rendered): %s", rec.Body.String())
	}
}

func TestService_LoginHandler_POST(t *testing.T) {
	SetDefaultHandler(newFrozenTestHandler(t))
	t.Cleanup(func() { SetDefaultHandler(nil) })

	// Empty form-encoded body. PostLogin parses it, calls
	// Service.Authenticate("", "") which returns ErrInvalidCredentials,
	// and re-renders the form with the generic error message.
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	(&Service{}).LoginHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render with error)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid login or password") {
		t.Errorf("POST dispatch did not reach PostLogin (no error msg in body): %s", rec.Body.String())
	}
}

// TestService_LogoutHandler verifies the LogoutHandler bridge delegates
// to PostLogout on the registered handler.
func TestService_LogoutHandler(t *testing.T) {
	h := newFrozenTestHandler(t)
	SetDefaultHandler(h)
	t.Cleanup(func() { SetDefaultHandler(nil) })

	// Wrap with scs LoadAndSave so PostLogout can call sessions.GetString
	// and sessions.Destroy against a live context.
	wrapped := h.sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		(&Service{}).LogoutHandler(w, r)
	}))

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

// TestService_LoginHandler_NoDefault verifies that with no handler set
// the bridge returns 500 (rather than panicking). The whole point of
// the singleton is to surface a "handler not initialised" error loudly
// at request time.
func TestService_LoginHandler_NoDefault(t *testing.T) {
	prev := defaultHandler
	defaultHandler = nil
	t.Cleanup(func() { defaultHandler = prev })

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	(&Service{}).LoginHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestService_LogoutHandler_NoDefault(t *testing.T) {
	prev := defaultHandler
	defaultHandler = nil
	t.Cleanup(func() { defaultHandler = prev })

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	(&Service{}).LogoutHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// newFrozenTestHandler builds a real *Handler with the minimum deps
// the frozen-signature bridges need: a real Service (for PostLogin),
// a real scs.SessionManager (for PostLogout), a real audit.Service
// (for failure auditing), and a seeded user. Reuses the same pattern
// as handler_test.go's newTestHandler but lives in the internal test
// package so it can call SetDefaultHandler.
func newFrozenTestHandler(t *testing.T) *Handler {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "frozen-test.db"))
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
	sm.Cookie.Name = "frozen_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"

	auditSvc := audit.NewService(queries)
	svc := NewService(queries)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(svc, auditSvc, sm, logger)
}
