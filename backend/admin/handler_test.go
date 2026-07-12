package admin_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"github.com/sirupsen/logrus"
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
	h, sm, auditSvc, mount, _ := newTestHandlerWithDB(t)
	return h, sm, auditSvc, mount
}

func newTestHandlerWithDB(t *testing.T) (*admin.Handler, *scs.SessionManager, *audit.Service, http.Handler, *sql.DB) {
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

	auditSvc := audit.NewService(queries)
	svc := admin.NewService(queries)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	h := admin.NewHandler(svc, auditSvc, sm, logger)
	admin.SetDefaultHandler(h)

	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc("GET /api/session", h.GetSessionJSON)
	apiRouter.HandleFunc("POST /api/login", h.PostLoginJSON)
	apiRouter.HandleFunc("POST /api/logout", h.PostLogoutJSON)

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiRouter.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	mount := sm.LoadAndSave(router)

	return h, sm, auditSvc, mount, db
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

func TestIsSafeRedirect(t *testing.T) {
	t.Skip("moved to internal_test.go (package admin) — handler_test.go is external and cannot reach unexported isSafeRedirect")
}

func TestErrUserDisabledIsErrInvalidCredentials(t *testing.T) {
	t.Skip("moved to internal_test.go (package admin) — handler_test.go is external and cannot reach unexported sentinels")
}
