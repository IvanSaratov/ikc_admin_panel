package admin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/csrf"
	"go.uber.org/zap"
)

// EnvCSRFKey is the env var that supplies the CSRF authKey.
//
// The value MUST decode to 32 bytes. If it is a hex string it is decoded;
// otherwise the raw bytes of the string are used and will be truncated /
// padded to 32 bytes — so a hex-encoded 32-byte value is the recommended
// representation.
const EnvCSRFKey = "MINTRUD_ADMIN_CSRF_KEY"

// EnvTrustedOrigins is a comma-separated list of origins the CSRF
// middleware will accept on HTTP deployments. gorilla/csrf v1.7+
// treats any request without a plaintext context marker as HTTPS and
// rejects HTTP Referer/Origin headers as a downgrade attack; the only
// way to make the middleware accept HTTP requests in production
// (where csrf.PlaintextHTTPRequest is not appropriate) is to list
// each expected origin here. Behind a TLS-terminating reverse proxy,
// the proxy's https://host is what the middleware sees and no
// override is needed.
const EnvTrustedOrigins = "MINTRUD_ADMIN_TRUSTED_ORIGINS"

// EnvPlaintextCSRF, when set to "true" / "1" / "yes", wraps every
// request through csrf.PlaintextHTTPRequest so the middleware
// accepts HTTP referers / origins. This is INTENDED FOR HTTP LOCAL
// DEVELOPMENT ONLY — never set it in production behind a reverse
// proxy that terminates TLS, because the flag tells gorilla/csrf
// to skip the HTTPS-only referer-origin checks that defend against
// HTTP MITM downgrade attacks. In a normal HTTPS deployment the
// reverse proxy terminates TLS and the Go process only ever sees
// https:// requests, so this flag is unnecessary.
const EnvPlaintextCSRF = "MINTRUD_ADMIN_PLAINTEXT_CSRF"

// csrfKeyLength is the required length for the CSRF auth key.
const csrfKeyLength = 32

// csrfCookieName is the cookie name used by the CSRF middleware.
// Exposed so handler tests can read the token via the documented header.
const csrfCookieName = "csrf_token"

// csrfFieldName is the legacy form field name configured on the CSRF
// middleware. JSON clients use csrfRequestHeader instead.
const csrfFieldName = "csrf_token"

// csrfRequestHeader is the request header that carries the CSRF token for
// non-form clients (JSON APIs, fetch()). Mirrors gorilla/csrf default.
const csrfRequestHeader = "X-CSRF-Token"

// LoadCSRF сохраняет прежний публичный контракт и использует стандартный logger.
func LoadCSRF() (func(http.Handler) http.Handler, error) {
	return LoadCSRFWithLogger(zap.L())
}

// LoadCSRFWithLogger собирает CSRF middleware из env-настроек и пишет
// предупреждения через переданный runtime logger без значений секретов.
func LoadCSRFWithLogger(log *zap.Logger) (func(http.Handler) http.Handler, error) {
	if log == nil {
		log = zap.NewNop()
	}

	key, err := resolveCSRFKeyWithLogger(log)
	if err != nil {
		return nil, err
	}

	opts := []csrf.Option{
		csrf.Secure(false), // dev: allow HTTP. prod must terminate TLS in front.
		csrf.HttpOnly(true),
		csrf.FieldName(csrfFieldName),
		csrf.RequestHeader(csrfRequestHeader),
		csrf.CookieName(csrfCookieName),
		csrf.Path("/"),
	}
	if origins := splitCSV(os.Getenv(EnvTrustedOrigins)); len(origins) > 0 {
		opts = append(opts, csrf.TrustedOrigins(origins))
	}

	mw := csrf.Protect(key, opts...)

	// HTTP deployments: gorilla/csrf v1.7+ rejects HTTP Referer headers
	// as a downgrade attack unless PlaintextHTTPContextKey is set on
	// the request. csrf.PlaintextHTTPRequest is the documented way to
	// mark a request as plaintext; wrapping it as a middleware applies
	// the marker to every request. Only enable via the explicit env
	// flag — see EnvPlaintextCSRF for why this is dev-only.
	if truthy(os.Getenv(EnvPlaintextCSRF)) {
		return func(next http.Handler) http.Handler {
			protected := mw(next)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = csrf.PlaintextHTTPRequest(r)
				protected.ServeHTTP(w, r)
			})
		}, nil
	}

	return mw, nil
}

// truthy reports whether s looks like an enabled boolean. Recognised
// forms: "1", "t", "true", "yes", "y", "on" (case-insensitive). Used
// for opt-in feature flags read from env.
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	}
	return false
}

// splitCSV returns the non-empty trimmed entries from a comma-separated
// string. Used for env-driven list config (TrustedOrigins today, more
// later). Empty input yields a nil slice so callers can use len() == 0
// to mean "unset".
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func resolveCSRFKey() ([]byte, error) {
	return resolveCSRFKeyWithLogger(zap.L())
}

func resolveCSRFKeyWithLogger(log *zap.Logger) ([]byte, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if raw := os.Getenv(EnvCSRFKey); raw != "" {
		// Prefer hex decoding; fall back to raw bytes if not valid hex.
		if decoded, err := hex.DecodeString(raw); err == nil {
			if len(decoded) != csrfKeyLength {
				return nil, fmt.Errorf("%s hex value must decode to %d bytes, got %d", EnvCSRFKey, csrfKeyLength, len(decoded))
			}
			return decoded, nil
		}

		// Raw-string fallback: pad/truncate to 32 bytes. Operators who
		// want a real 32-byte key can pass it hex-encoded.
		key := padOrTruncate([]byte(raw), csrfKeyLength)
		return key, nil
	}

	// Missing env: generate per-process key with a WARN log line so
	// operators notice the session will not survive restart.
	buf := make([]byte, csrfKeyLength)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate csrf key: %w", err)
	}
	log.Warn(
		"MINTRUD_ADMIN_CSRF_KEY is unset; generated an ephemeral per-process CSRF key. " +
			"Tokens will be invalidated on every restart. Set MINTRUD_ADMIN_CSRF_KEY to a stable 32-byte hex value.",
	)
	return buf, nil
}

func padOrTruncate(in []byte, n int) []byte {
	if len(in) == n {
		return in
	}
	out := make([]byte, n)
	copy(out, in)
	return out
}
