package admin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/csrf"
)

// EnvCSRFKey is the env var that supplies the CSRF authKey.
//
// The value MUST decode to 32 bytes. If it is a hex string it is decoded;
// otherwise the raw bytes of the string are used and will be truncated /
// padded to 32 bytes — so a hex-encoded 32-byte value is the recommended
// representation.
const EnvCSRFKey = "MINTRUD_ADMIN_CSRF_KEY"

// csrfKeyLength is the required length for the CSRF auth key.
const csrfKeyLength = 32

// csrfCookieName is the cookie name used by the CSRF middleware.
// Exposed so handler tests can read the token via the documented header.
const csrfCookieName = "csrf_token"

// csrfFieldName is the form field name the templ helper injects.
// Must match csrf.FieldName below.
const csrfFieldName = "csrf_token"

// csrfRequestHeader is the request header that carries the CSRF token for
// non-form clients (JSON APIs, fetch()). Mirrors gorilla/csrf default.
const csrfRequestHeader = "X-CSRF-Token"

// LoadCSRF returns a CSRF middleware built from environment configuration.
//
// Contract (frozen): returns (middleware, nil) on success or
// (nil, error) on misconfiguration. The middleware is intentionally
// stateful (per-process key); when MINTRUD_ADMIN_CSRF_KEY is missing a
// per-process random key is generated and a WARN log line is emitted
// (without the key value) so operators know sessions will not survive
// restart.
func LoadCSRF() (func(http.Handler) http.Handler, error) {
	key, err := resolveCSRFKey()
	if err != nil {
		return nil, err
	}

	mw := csrf.Protect(
		key,
		csrf.Secure(false), // dev: allow HTTP. prod must terminate TLS in front.
		csrf.HttpOnly(true),
		csrf.FieldName(csrfFieldName),
		csrf.RequestHeader(csrfRequestHeader),
		csrf.CookieName(csrfCookieName),
		csrf.Path("/"),
	)
	return mw, nil
}

func resolveCSRFKey() ([]byte, error) {
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
	slog.Warn(
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
