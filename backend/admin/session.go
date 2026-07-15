package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
)

// DefaultSessionTTL is the default session lifetime used by the CLI.
const DefaultSessionTTL = 8 * time.Hour

// SessionConfig is the fully-resolved set of session parameters passed to
// NewSessionManager. NewSessionConfig validates explicit runtime values.
type SessionConfig struct {
	// TTL controls the absolute lifetime of a session (cookie Max-Age).
	TTL time.Duration
	// SameSite maps directly onto http.SameSite.
	SameSite http.SameSite
	// Secure sets the cookie Secure flag.
	Secure bool
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
		return 0, fmt.Errorf("cookie same-site must be one of lax|strict|none, got %q", raw)
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
