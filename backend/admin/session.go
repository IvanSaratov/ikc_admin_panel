package admin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
)

// EnvSessionTTL is the env var that overrides the session lifetime.
// Value is parsed as a Go time.Duration (e.g. "8h", "30m", "24h").
const EnvSessionTTL = "MINTRUD_ADMIN_SESSION_TTL"

// EnvCookieSameSite is the env var that overrides the cookie SameSite
// attribute. Allowed values: "lax" (default), "strict", "none".
const EnvCookieSameSite = "MINTRUD_ADMIN_COOKIE_SAMESITE"

// EnvCookieSecure is the env var that explicitly sets the cookie Secure
// flag. When unset, Secure is derived from EnvAppEnv (true for "prod").
const EnvCookieSecure = "MINTRUD_ADMIN_COOKIE_SECURE"

// EnvAppEnv is the application environment indicator.
// "prod" enables Secure cookies by default.
const EnvAppEnv = "MINTRUD_ADMIN_ENV"

// DefaultSessionTTL is used when EnvSessionTTL is unset.
const DefaultSessionTTL = 8 * time.Hour

// SessionConfig is the fully-resolved set of session parameters passed to
// NewSessionManager. NewSessionConfig validates explicit runtime values;
// LoadSessionConfig remains as a temporary adapter for the legacy entrypoint.
type SessionConfig struct {
	// TTL controls the absolute lifetime of a session (cookie Max-Age).
	TTL time.Duration
	// SameSite maps directly onto http.SameSite.
	SameSite http.SameSite
	// Secure sets the cookie Secure flag.
	Secure bool
}

// LoadSessionConfig reads SessionConfig from environment variables.
//
// Defaults (when env vars are unset):
//   - TTL:        8h
//   - SameSite:   http.SameSiteLaxMode
//   - Secure:     true if MINTRUD_ADMIN_ENV=prod, else false
//
// Errors are returned (not logged) so main() can surface them with full
// context. The function never falls back silently on a malformed value.
func LoadSessionConfig() (SessionConfig, error) {
	ttl := DefaultSessionTTL
	sameSite := "lax"
	secure := false

	if raw := os.Getenv(EnvSessionTTL); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return SessionConfig{}, fmt.Errorf("%s=%q: %w", EnvSessionTTL, raw, err)
		}
		ttl = d
	}

	if raw := os.Getenv(EnvCookieSameSite); raw != "" {
		sameSite = raw
	}

	switch {
	case os.Getenv(EnvCookieSecure) != "":
		resolvedSecure, err := strconv.ParseBool(os.Getenv(EnvCookieSecure))
		if err != nil {
			return SessionConfig{}, fmt.Errorf("%s=%q: %w", EnvCookieSecure, os.Getenv(EnvCookieSecure), err)
		}
		secure = resolvedSecure
	default:
		secure = os.Getenv(EnvAppEnv) == "prod"
	}

	return NewSessionConfig(ttl, sameSite, secure)
}

// NewSessionConfig validates explicit session settings and resolves the
// SameSite string into the net/http representation used by scs.
func NewSessionConfig(ttl time.Duration, sameSite string, secure bool) (SessionConfig, error) {
	if ttl <= 0 {
		return SessionConfig{}, fmt.Errorf("session TTL must be > 0, got %s", ttl)
	}
	resolvedSameSite, err := parseSameSite(strings.ToLower(strings.TrimSpace(sameSite)))
	if err != nil {
		return SessionConfig{}, err
	}
	return SessionConfig{TTL: ttl, SameSite: resolvedSameSite, Secure: secure}, nil
}

func parseSameSite(raw string) (http.SameSite, error) {
	switch raw {
	case "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return 0, fmt.Errorf("%s must be one of lax|strict|none, got %q", EnvCookieSameSite, raw)
	}
}

// NewSessionManager builds a scs.SessionManager from the provided config.
// It applies HttpOnly=true unconditionally (security baseline; scs defaults
// to true as well but we set it explicitly here to make the intent visible).
func NewSessionManager(cfg SessionConfig) *scs.SessionManager {
	sm := scs.New()
	sm.Lifetime = cfg.TTL
	sm.IdleTimeout = cfg.TTL
	sm.Cookie.Name = "ikc_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = cfg.SameSite
	sm.Cookie.Secure = cfg.Secure
	sm.Cookie.Path = "/"
	return sm
}

// ErrSessionConfigInvalid is reserved for future use; current callers
// receive a wrapped error directly from LoadSessionConfig.
var ErrSessionConfigInvalid = errors.New("invalid session config")
