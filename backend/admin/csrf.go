package admin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/gorilla/csrf"
	"go.uber.org/zap"
)

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

// CSRFConfig is the explicit set of values required to build CSRF middleware.
type CSRFConfig struct {
	Key            string
	TrustedOrigins []string
	Plaintext      bool
}

// NewCSRFMiddleware builds CSRF middleware exclusively from explicit config.
func NewCSRFMiddleware(config CSRFConfig, log *zap.Logger) (func(http.Handler) http.Handler, error) {
	if log == nil {
		log = zap.NewNop()
	}
	key, err := resolveCSRFKey(config.Key, log)
	if err != nil {
		return nil, err
	}

	opts := []csrf.Option{
		csrf.Secure(false),
		csrf.HttpOnly(true),
		csrf.FieldName(csrfFieldName),
		csrf.RequestHeader(csrfRequestHeader),
		csrf.CookieName(csrfCookieName),
		csrf.Path("/"),
	}
	if len(config.TrustedOrigins) > 0 {
		opts = append(opts, csrf.TrustedOrigins(config.TrustedOrigins))
	}
	protected := csrf.Protect(key, opts...)
	if !config.Plaintext {
		return protected, nil
	}
	return func(next http.Handler) http.Handler {
		wrapped := protected(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped.ServeHTTP(w, csrf.PlaintextHTTPRequest(r))
		})
	}, nil
}

func resolveCSRFKey(raw string, log *zap.Logger) ([]byte, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if raw != "" {
		// Prefer hex decoding; fall back to raw bytes if not valid hex.
		if decoded, err := hex.DecodeString(raw); err == nil {
			if len(decoded) != csrfKeyLength {
				return nil, fmt.Errorf("CSRF key hex value must decode to %d bytes, got %d", csrfKeyLength, len(decoded))
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
	log.Warn("IKC_SERVER_CSRF_KEY is unset; generated an ephemeral per-process CSRF key")
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
