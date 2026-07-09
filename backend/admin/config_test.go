package admin

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
)

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
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", hex64)

	key, err := resolveCSRFKey()
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
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "not-actually-hex-zzz")

	key, err := resolveCSRFKey()
	if err != nil {
		t.Fatalf("resolveCSRFKey raw fallback: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("raw-fallback key len = %d, want 32 (padded/truncated)", len(key))
	}
}

// TestResolveCSRFKey_GeneratesEphemeral verifies the fallback path: when
// the env var is unset, resolveCSRFKey returns a 32-byte random key
// (still meeting the gorilla/csrf length contract).
func TestResolveCSRFKey_GeneratesEphemeral(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "")

	key, err := resolveCSRFKey()
	if err != nil {
		t.Fatalf("resolveCSRFKey fallback: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("ephemeral key len = %d, want 32", len(key))
	}
}

// TestLoadCSRF_ReturnsMiddleware locks the public surface: LoadCSRF must
// always return a non-nil middleware (so main.go can plug it straight
// into app.Deps.CSRF).
func TestLoadCSRF_ReturnsMiddleware(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "")

	mw, err := LoadCSRF()
	if err != nil {
		t.Fatalf("LoadCSRF: %v", err)
	}
	if mw == nil {
		t.Fatal("LoadCSRF returned nil middleware")
	}

	// The middleware must be callable — wrap a trivial handler and call it.
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("wrapped handler was not invoked")
	}
}

func TestLoadCSRFWithLogger_UsesProvidedLogger(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "")

	var out bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&out)
	logger.SetFormatter(&logrus.JSONFormatter{})

	if _, err := LoadCSRFWithLogger(logger); err != nil {
		t.Fatalf("LoadCSRFWithLogger: %v", err)
	}

	logOutput := out.String()
	if !strings.Contains(logOutput, "MINTRUD_ADMIN_CSRF_KEY is unset") {
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

// TestLoadSessionConfig verifies the env-driven config loader returns
// sensible defaults and respects MINTRUD_ADMIN_SESSION_TTL.
func TestLoadSessionConfig(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_SESSION_TTL", "")
	t.Setenv("MINTRUD_ADMIN_COOKIE_SECURE", "")
	t.Setenv("MINTRUD_ADMIN_COOKIE_SAMESITE", "")

	cfg, err := LoadSessionConfig()
	if err != nil {
		t.Fatalf("LoadSessionConfig default: %v", err)
	}
	if cfg.TTL <= 0 {
		t.Errorf("TTL = %v, want positive default", cfg.TTL)
	}
	if cfg.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite default = %d, want SameSiteLaxMode", cfg.SameSite)
	}
}

// TestSplitCSV locks the comma-separated env parser used for
// MINTRUD_ADMIN_TRUSTED_ORIGINS (and any future list-valued env).
// Empty string → nil (signals "unset"), whitespace around items is
// trimmed, empty entries are dropped.
func TestSplitCSV(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty yields nil", "", nil},
		{"single entry", "http://localhost:8081", []string{"http://localhost:8081"}},
		{"multiple entries trimmed", " a , b ,c ", []string{"a", "b", "c"}},
		{"drops empty segments", "a,,b, ,c", []string{"a", "b", "c"}},
		{"trailing comma tolerated", "a,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCSV(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitCSV(%q) len = %d, want %d (%v)", tc.in, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestTruthy covers every recognised form plus a few that should be
// rejected. Used by LoadCSRF to read MINTRUD_ADMIN_PLAINTEXT_CSRF.
func TestTruthy(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false}, {"0", false}, {"false", false}, {"no", false},
		{"off", false}, {"garbage", false},
		{"1", true}, {"true", true}, {"True", true}, {"TRUE", true},
		{"t", true}, {"yes", true}, {"y", true}, {"on", true},
		{"  true  ", true}, // trimming
	}
	for _, tc := range cases {
		if got := truthy(tc.in); got != tc.want {
			t.Errorf("truthy(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestLoadCSRF_PlaintextFlag проверяет, что локальный HTTP-режим не отключает
// проверку CSRF token. Plaintext mode должен только пометить request безопасным
// для referer-проверок gorilla/csrf, а не обходить csrf.Protect.
func TestLoadCSRF_PlaintextFlag(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "")
	t.Setenv(EnvPlaintextCSRF, "true")

	mw, err := LoadCSRF()
	if err != nil {
		t.Fatalf("LoadCSRF: %v", err)
	}
	if mw == nil {
		t.Fatal("LoadCSRF returned nil middleware")
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

// TestLoadCSRF_PlaintextFlag_AllowsValidHTTPReferer проверяет позитивный
// локальный HTTP flow: валидный token и HTTP Referer должны пройти, когда
// MINTRUD_ADMIN_PLAINTEXT_CSRF явно включен.
func TestLoadCSRF_PlaintextFlag_AllowsValidHTTPReferer(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", strings.Repeat("ab", 32))
	t.Setenv(EnvPlaintextCSRF, "true")

	mw, err := LoadCSRF()
	if err != nil {
		t.Fatalf("LoadCSRF: %v", err)
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

// TestLoadCSRF_TrustedOrigins verifies the env var is wired into the
// csrf.TrustedOrigins option when non-empty. The middleware still has
// to run a real request to exercise the check end-to-end, but the
// load path must succeed and return a non-nil middleware.
func TestLoadCSRF_TrustedOrigins(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_CSRF_KEY", "")
	t.Setenv(EnvPlaintextCSRF, "")
	t.Setenv(EnvTrustedOrigins, "http://localhost:8081, http://example.com")

	mw, err := LoadCSRF()
	if err != nil {
		t.Fatalf("LoadCSRF with TrustedOrigins: %v", err)
	}
	if mw == nil {
		t.Fatal("LoadCSRF returned nil middleware")
	}
}
