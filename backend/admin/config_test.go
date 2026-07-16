package admin

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/gorilla/csrf"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewSessionConfigValidatesAndResolvesValues(t *testing.T) {
	cfg, err := NewSessionConfig(12*time.Hour, "strict", true)
	if err != nil {
		t.Fatalf("NewSessionConfig: %v", err)
	}
	if cfg.TTL != 12*time.Hour || cfg.SameSite != http.SameSiteStrictMode || !cfg.Secure {
		t.Fatalf("config = %#v", cfg)
	}
	if _, err := NewSessionConfig(0, "lax", false); err == nil {
		t.Fatal("zero TTL accepted")
	}
	if _, err := NewSessionConfig(time.Hour, "invalid", false); err == nil {
		t.Fatal("invalid SameSite accepted")
	}
}

func TestNewCSRFMiddlewareUsesExplicitConfig(t *testing.T) {
	middleware, err := NewCSRFMiddleware(CSRFConfig{
		Key:            strings.Repeat("ab", 32),
		TrustedOrigins: []string{"localhost:8081"},
		Plaintext:      true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}
	if middleware == nil {
		t.Fatal("nil middleware")
	}
}

func TestNewCSRFMiddlewareDoesNotReadLegacyEnvironment(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "invalid-environment-value")
	_, err := NewCSRFMiddleware(CSRFConfig{Key: strings.Repeat("ab", 32)}, zap.NewNop())
	if err != nil {
		t.Fatalf("explicit config was overridden by environment: %v", err)
	}
}

// TestPadOrTruncate locks the CSRF key-length normaliser: input of the
// target length is returned as-is, too-short input is right-padded with
// 0x00 bytes, and too-long input is truncated. CSRF keys must be exactly
// 32 bytes per the gorilla/csrf contract.
func TestPadOrTruncate(t *testing.T) {

	cases := []struct {
		name string
		in   []byte
		n    int
		want []byte
	}{
		{"exact length passes through", []byte{1, 2, 3, 4}, 4, []byte{1, 2, 3, 4}},
		{"short input is right-padded with zeros", []byte{1, 2}, 4, []byte{1, 2, 0, 0}},
		{"long input is truncated", []byte{1, 2, 3, 4, 5, 6}, 4, []byte{1, 2, 3, 4}},
		{"zero-length target yields empty", []byte{1, 2}, 0, []byte{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := padOrTruncate(tc.in, tc.n)
			if len(got) != len(tc.want) {
				t.Fatalf("len(padOrTruncate(%v, %d)) = %d, want %d", tc.in, tc.n, len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("byte %d: got %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestResolveCSRFKey_ValidHex verifies that a 64-char hex string decodes
// to the canonical 32-byte key.
func TestResolveCSRFKey_ValidHex(t *testing.T) {

	hex64 := strings.Repeat("ab", 32) // 64 hex chars

	key, err := resolveCSRFKey(hex64, zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCSRFKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key len = %d, want 32", len(key))
	}
	if key[0] != 0xab || key[31] != 0xab {
		t.Errorf("key bytes not as expected: %x", key)
	}
	// Round-trip: re-encoding should give back the hex string.
	if got := hex.EncodeToString(key); got != hex64 {
		t.Errorf("hex round-trip = %q, want %q", got, hex64)
	}
}

// TestResolveCSRFKey_InvalidHex verifies the documented fallback:
// a non-hex string is treated as raw bytes and padded/truncated to the
// canonical 32-byte key length. This is by design so operators can pass
// a short human-readable passphrase without converting it to hex.
func TestResolveCSRFKey_InvalidHex(t *testing.T) {
	key, err := resolveCSRFKey("not-actually-hex-zzz", zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCSRFKey raw fallback: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("raw-fallback key len = %d, want 32 (padded/truncated)", len(key))
	}
}

// TestResolveCSRFKey_GeneratesEphemeral verifies the fallback path: when
// the explicit key is empty, resolveCSRFKey returns a 32-byte random key
// (still meeting the gorilla/csrf length contract).
func TestResolveCSRFKey_GeneratesEphemeral(t *testing.T) {
	key, err := resolveCSRFKey("", zap.NewNop())
	if err != nil {
		t.Fatalf("resolveCSRFKey fallback: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("ephemeral key len = %d, want 32", len(key))
	}
}

func TestNewCSRFMiddlewareUsesProvidedLogger(t *testing.T) {
	var out bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&out),
		zapcore.InfoLevel,
	))

	if _, err := NewCSRFMiddleware(CSRFConfig{}, logger); err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}

	logOutput := out.String()
	if !strings.Contains(logOutput, "IKC_SERVER_CSRF_KEY is unset") {
		t.Fatalf("log output missing csrf warning: %s", logOutput)
	}
	if strings.Contains(logOutput, "csrf_token") {
		t.Fatalf("log output contains csrf token name: %s", logOutput)
	}
}

// TestParseSameSite verifies all three accepted values plus the default
// (error) branch.
func TestParseSameSite(t *testing.T) {

	cases := []struct {
		in      string
		want    http.SameSite
		wantErr bool
	}{
		{"lax", http.SameSiteLaxMode, false},
		{"strict", http.SameSiteStrictMode, false},
		{"none", http.SameSiteNoneMode, false},
		{"", 0, true},
		{"garbage", 0, true},
	}
	for _, tc := range cases {
		got, err := parseSameSite(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseSameSite(%q) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseSameSite(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNewSessionManagerUsesCanonicalCookieName(t *testing.T) {
	sm := NewSessionManager(SessionConfig{TTL: time.Hour})
	if sm.Cookie.Name != "ikc_session" {
		t.Fatalf("cookie name = %q, want %q", sm.Cookie.Name, "ikc_session")
	}
}

// TestNewCSRFMiddleware_PlaintextFlag проверяет, что локальный HTTP-режим не отключает
// проверку CSRF token. Plaintext mode должен только пометить request безопасным
// для referer-проверок gorilla/csrf, а не обходить csrf.Protect.
func TestNewCSRFMiddleware_PlaintextFlag(t *testing.T) {
	mw, err := NewCSRFMiddleware(CSRFConfig{Plaintext: true}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}
	if mw == nil {
		t.Fatal("NewCSRFMiddleware returned nil middleware")
	}

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if called {
		t.Error("plaintext wrapper bypassed CSRF protection")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestNewCSRFMiddleware_PlaintextFlag_AllowsValidHTTPReferer проверяет позитивный
// локальный HTTP flow: валидный token и HTTP Referer должны пройти, когда
// plaintext mode явно включен.
func TestNewCSRFMiddleware_PlaintextFlag_AllowsValidHTTPReferer(t *testing.T) {
	mw, err := NewCSRFMiddleware(CSRFConfig{
		Key:       strings.Repeat("ab", 32),
		Plaintext: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}

	postCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(csrf.Token(r)))
			return
		}
		postCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "http://example.com/form", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRec.Code)
	}
	token := getRec.Body.String()
	if token == "" {
		t.Fatal("empty csrf token")
	}

	form := url.Values{}
	form.Set("csrf_token", token)
	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/form", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Referer", "http://example.com/form")
	for _, cookie := range getRec.Result().Cookies() {
		postReq.AddCookie(cookie)
	}

	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if !postCalled {
		t.Fatalf("valid plaintext CSRF POST did not reach handler, status=%d", postRec.Code)
	}
	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204", postRec.Code)
	}
}

// TestNewCSRFMiddleware_TrustedOrigins verifies explicit origins are wired into the
// csrf.TrustedOrigins option when non-empty. The middleware still has
// to run a real request to exercise the check end-to-end, but the
// load path must succeed and return a non-nil middleware.
func TestNewCSRFMiddleware_TrustedOrigins(t *testing.T) {
	mw, err := NewCSRFMiddleware(CSRFConfig{
		TrustedOrigins: []string{"http://localhost:8081", "http://example.com"},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware with TrustedOrigins: %v", err)
	}
	if mw == nil {
		t.Fatal("NewCSRFMiddleware returned nil middleware")
	}
}

func TestNewCSRFMiddlewareReturnsProblemJSONForAPIRequests(t *testing.T) {
	mw, err := NewCSRFMiddleware(CSRFConfig{
		Key:       strings.Repeat("ab", 32),
		Plaintext: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}
	handler := api.TraceMiddleware(mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request without CSRF token reached handler")
	})))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://example.com/api/imports/legacy", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	body := recorder.Body.String()
	for _, required := range []string{`"code":"csrf_failed"`, `"trace_id":"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("problem body %q does not contain %q", body, required)
		}
	}
}

func TestNewCSRFMiddlewareKeepsLegacyFailureNonJSON(t *testing.T) {
	mw, err := NewCSRFMiddleware(CSRFConfig{
		Key:       strings.Repeat("ab", 32),
		Plaintext: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewCSRFMiddleware: %v", err)
	}
	handler := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request without CSRF token reached handler")
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://example.com/login", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "json") {
		t.Fatalf("legacy failure became JSON: %q", recorder.Header().Get("Content-Type"))
	}
}
