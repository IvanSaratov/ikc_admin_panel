package admin_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

type apiAuthFixture struct {
	database *sql.DB
	sessions *scs.SessionManager
	store    *admin.Store
	user     storagedb.User
}

func newAPIAuthFixture(t *testing.T, role, status string) apiAuthFixture {
	t.Helper()

	database, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "api-auth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	queries := storagedb.New(database)
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	user, err := queries.CreateUser(context.Background(), storagedb.CreateUserParams{
		Login:        "api-user",
		PasswordHash: string(hash),
		Role:         role,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return apiAuthFixture{
		database: database,
		sessions: scs.New(),
		store:    admin.NewStore(queries),
		user:     user,
	}
}

func requestWithUserSession(t *testing.T, sessions *scs.SessionManager, userID int64) *http.Request {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/api/imports", nil)
	ctx, err := sessions.Load(request.Context(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	sessions.Put(ctx, admin.SessionKeyUserID, userID)
	sessions.Put(ctx, admin.SessionKeyUserLogin, "stale-session-login")
	return request.WithContext(ctx)
}

func decodeProblemCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var problem struct {
		Code    string `json:"code"`
		TraceID string `json:"trace_id"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.TraceID == "" {
		t.Fatal("problem has no trace_id")
	}
	return problem.Code
}

func TestRequireAPIAuthRejectsMissingSessionWithProblem(t *testing.T) {
	t.Parallel()

	fixture := newAPIAuthFixture(t, "admin", "active")
	called := false
	handler := api.TraceMiddleware(fixture.sessions.LoadAndSave(
		admin.RequireAPIAuth(fixture.sessions, fixture.store, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		})),
	))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/imports", nil))
	if recorder.Code != http.StatusUnauthorized || decodeProblemCode(t, recorder) != "unauthorized" {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatal("handler called without session")
	}
}

func TestRequireAPIAuthLoadsCurrentDatabaseIdentity(t *testing.T) {
	t.Parallel()

	fixture := newAPIAuthFixture(t, "admin", "active")
	var got admin.APIIdentity
	var gotActor string
	handler := api.TraceMiddleware(admin.RequireAPIAuth(fixture.sessions, fixture.store, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = admin.APIIdentityFromContext(r.Context())
		if !ok {
			t.Error("identity missing from context")
		}
		gotActor = audit.ActorFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestWithUserSession(t, fixture.sessions, fixture.user.ID))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if got.ID != fixture.user.ID || got.Login != fixture.user.Login || got.Role != "admin" {
		t.Fatalf("identity = %+v", got)
	}
	if got.Login == "stale-session-login" || gotActor != fixture.user.Login {
		t.Fatalf("identity login = %q, audit actor = %q", got.Login, gotActor)
	}
}

func TestRequireAPIAuthRejectsDisabledAndUnknownUsers(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		status string
		id     int64
	}{
		{name: "disabled", status: "disabled"},
		{name: "unknown", status: "active", id: 9999},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAPIAuthFixture(t, "admin", test.status)
			userID := test.id
			if userID == 0 {
				userID = fixture.user.ID
			}
			handler := api.TraceMiddleware(admin.RequireAPIAuth(fixture.sessions, fixture.store, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler called for invalid user")
			})))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, requestWithUserSession(t, fixture.sessions, userID))
			if recorder.Code != http.StatusUnauthorized || decodeProblemCode(t, recorder) != "unauthorized" {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRequireAPIAuthMapsDatabaseFailureToServiceUnavailable(t *testing.T) {
	t.Parallel()

	fixture := newAPIAuthFixture(t, "admin", "active")
	if err := fixture.database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	handler := api.TraceMiddleware(admin.RequireAPIAuth(fixture.sessions, fixture.store, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called while database is unavailable")
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestWithUserSession(t, fixture.sessions, fixture.user.ID))
	if recorder.Code != http.StatusServiceUnavailable || decodeProblemCode(t, recorder) != "storage_unavailable" {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequireAPIRolesUsesCurrentIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		role       string
		allowed    []string
		wantStatus int
		wantCode   string
	}{
		{name: "admin upload", role: "admin", allowed: []string{"admin"}, wantStatus: http.StatusNoContent},
		{name: "operator upload", role: "operator", allowed: []string{"admin"}, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "operator read", role: "operator", allowed: []string{"admin", "operator"}, wantStatus: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAPIAuthFixture(t, test.role, "active")
			handler := api.TraceMiddleware(admin.RequireAPIAuth(fixture.sessions, fixture.store, nil)(
				admin.RequireAPIRoles(test.allowed...)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				})),
			))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, requestWithUserSession(t, fixture.sessions, fixture.user.ID))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantCode != "" && decodeProblemCode(t, recorder) != test.wantCode {
				t.Fatalf("problem body = %s", recorder.Body.String())
			}
		})
	}
}
