package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

// newTestMiddleware wires a real scs.SessionManager + Store + middleware
// against a fresh DB. Used by RequireAuth tests.
func newTestMiddleware(t *testing.T) (*scs.SessionManager, *admin.Store, admin.User) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mw-test.db"))
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
	user, err := queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login:        "alice",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	sm := scs.New()
	sm.Lifetime = time.Hour
	sm.Cookie.Name = "test_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"

	store := admin.NewStore(queries)
	return sm, store, user
}

func TestRequireAuth_NoSession_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	sm, store, _ := newTestMiddleware(t)
	mw := admin.RequireAuth(sm, nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// Mount with scs.LoadAndSave so the session manager has a chance to
	// initialise an empty session before RequireAuth queries it.
	mounted := sm.LoadAndSave(handler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	mounted.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login?next=/protected" {
		t.Errorf("Location = %q, want /login?next=/protected", loc)
	}
	if called {
		t.Errorf("inner handler called despite missing session")
	}

	_ = store
}

func TestRequireAuth_WithSession_CallsNext(t *testing.T) {
	t.Parallel()

	sm, _, user := newTestMiddleware(t)
	mw := admin.RequireAuth(sm, nil)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := admin.UserLoginFromContext(r.Context()); got != user.Login {
			t.Errorf("login from context = %q, want %q", got, user.Login)
		}
	}))

	// Mount scs LoadAndSave so the session is properly persisted and
	// the response cookie carries the real session token.
	mounted := sm.LoadAndSave(handler)

	// First request: seed the session by routing through LoadAndSave.
	seedReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	seedCtx, _ := sm.Load(seedReq.Context(), "")
	sm.Put(seedCtx, admin.SessionKeyUserID, user.ID)
	sm.Put(seedCtx, admin.SessionKeyUserLogin, user.Login)
	seedReq = seedReq.WithContext(seedCtx)
	seedRec := httptest.NewRecorder()
	mounted.ServeHTTP(seedRec, seedReq)

	// Even though RequireAuth redirected (no cookie yet), scs commits
	// the new session and Set-Cookie appears in seedRec.
	var sessionCookie *http.Cookie
	for _, c := range seedRec.Result().Cookies() {
		if c.Name == sm.Cookie.Name {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("scs did not issue a session cookie")
	}

	// Second request: send the session cookie back.
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	mounted.ServeHTTP(rec, req)

	if !called {
		t.Errorf("inner handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
