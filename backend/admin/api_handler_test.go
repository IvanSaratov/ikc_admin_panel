package admin_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
)

func TestAPIHandler_Session_Unauthenticated(t *testing.T) {
	t.Parallel()

	_, _, _, mount := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var body struct {
		Authenticated bool   `json:"authenticated"`
		Login         string `json:"login,omitempty"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("authenticated = true, want false")
	}
	if body.Login != "" {
		t.Fatalf("login = %q, want empty", body.Login)
	}
}

func TestAPIHandler_CSRF_ReturnsMaskedTokenAndCookie(t *testing.T) {
	t.Parallel()

	handler, sessions, _, _ := newTestHandler(t)
	csrfMiddleware, err := admin.NewCSRFMiddleware(admin.CSRFConfig{
		Key:       strings.Repeat("ab", 32),
		Plaintext: true,
	}, nil)
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}
	mounted := sessions.LoadAndSave(csrfMiddleware(http.HandlerFunc(handler.GetCSRFJSON)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.com/api/csrf", nil)
	mounted.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Token string `json:"csrf_token"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("empty CSRF token")
	}
	var foundCookie bool
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "csrf_token" {
			foundCookie = true
			if !cookie.HttpOnly {
				t.Fatal("CSRF cookie is not HttpOnly")
			}
		}
	}
	if !foundCookie {
		t.Fatal("CSRF cookie not set")
	}
}

func TestAPIHandler_LoginJSON_Success(t *testing.T) {
	t.Parallel()

	_, sm, _, mount, db := newTestHandlerWithDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"alice","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var body struct {
		Authenticated bool   `json:"authenticated"`
		Login         string `json:"login"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if !body.Authenticated {
		t.Fatalf("authenticated = false, want true")
	}
	if body.Login != "alice" {
		t.Fatalf("login = %q, want alice", body.Login)
	}

	sessionCookie := extractCookieValue(t, rec.Result().Cookies(), sm.Cookie.Name)
	loadReq := httptest.NewRequest(http.MethodGet, "/_check", nil)
	ctx, _ := sm.Load(loadReq.Context(), sessionCookie)
	if sm.GetInt64(ctx, admin.SessionKeyUserID) == 0 {
		t.Errorf("session has no user_id after JSON login")
	}
	if got := sm.GetString(ctx, admin.SessionKeyUserLogin); got != "alice" {
		t.Errorf("session user_login = %q, want alice", got)
	}

	assertActionLogged(t, db, "login.success", "alice")
}

func TestAPIHandler_LoginJSON_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, _, _, mount := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestAPIHandler_LoginJSON_InvalidCredentials(t *testing.T) {
	t.Parallel()

	_, _, _, mount, db := newTestHandlerWithDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"alice","password":"WRONG"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusUnauthorized, "invalid_credentials")
	assertActionLogged(t, db, "login.failure", "alice")
}

func TestAPIHandler_LoginJSON_RequiresJSONContentType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		contentType string
	}{
		{name: "missing"},
		{name: "wrong", contentType: "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, mount := newTestHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"alice","password":"test-password"}`))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			mount.ServeHTTP(rec, req)

			assertJSONError(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
		})
	}
}

func TestAPIHandler_LoginJSON_SessionAndLogout(t *testing.T) {
	t.Parallel()

	_, sm, _, mount := newTestHandler(t)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"login":"alice","password":"test-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mount.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("POST /api/login status = %d, want 200; body=%s", loginRec.Code, loginRec.Body.String())
	}
	sessionCookie := findCookie(t, loginRec.Result().Cookies(), sm.Cookie.Name)

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	sessionReq.AddCookie(sessionCookie)
	sessionRec := httptest.NewRecorder()
	mount.ServeHTTP(sessionRec, sessionReq)
	assertSessionJSON(t, sessionRec, true, "alice")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutRec := httptest.NewRecorder()
	mount.ServeHTTP(logoutRec, logoutReq)
	assertSessionJSON(t, logoutRec, false, "")

	nextSessionReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	for _, c := range logoutRec.Result().Cookies() {
		nextSessionReq.AddCookie(c)
	}
	nextSessionRec := httptest.NewRecorder()
	mount.ServeHTTP(nextSessionRec, nextSessionReq)
	assertSessionJSON(t, nextSessionRec, false, "")
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body.Error != wantError {
		t.Fatalf("error = %q, want %q", body.Error, wantError)
	}
}

func assertSessionJSON(t *testing.T, rec *httptest.ResponseRecorder, wantAuthenticated bool, wantLogin string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body sessionBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body.Authenticated != wantAuthenticated {
		t.Fatalf("authenticated = %v, want %v", body.Authenticated, wantAuthenticated)
	}
	if body.Login != wantLogin {
		t.Fatalf("login = %q, want %q", body.Login, wantLogin)
	}
}

func assertActionLogged(t *testing.T, db *sql.DB, action, actor string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM action_log WHERE action = ? AND actor = ?
	`, action, actor).Scan(&count); err != nil {
		t.Fatalf("query action_log: %v", err)
	}
	if count == 0 {
		t.Fatalf("no action_log row for action=%q actor=%q", action, actor)
	}
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not found", name)
	return nil
}

type sessionBody struct {
	Authenticated bool   `json:"authenticated"`
	Login         string `json:"login,omitempty"`
}
