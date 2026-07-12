package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/alexedwards/scs/v2"
	"github.com/sirupsen/logrus"
)

// loginErrorMsg is intentionally generic so we never leak whether the
// account exists, is disabled, or simply has the wrong password.
const loginErrorMsg = "Invalid login or password"

// Handler bundles the dependencies LoginHandler / LogoutHandler need.
//
// It is intentionally thin: the heavy lifting is in Service
// (authentication) and scs.SessionManager (cookie + session data). The
// handler is responsible for HTTP shape: parse login requests, audit
// success/failure, and return the appropriate redirect or JSON response.
type Handler struct {
	service  *Service
	audit    *audit.Service
	sessions *scs.SessionManager
	log      logrus.FieldLogger
}

// NewHandler constructs a Handler.
func NewHandler(service *Service, auditSvc *audit.Service, sessions *scs.SessionManager, log logrus.FieldLogger) *Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &Handler{
		service:  service,
		audit:    auditSvc,
		sessions: sessions,
		log:      log,
	}
}

// LoginHandler handles legacy POST form authentication. It satisfies the
// frozen plan signature:
//
//	func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request)
//
// by being a thin wrapper that invokes the Handler's method. We cannot
// put this directly on Service because Service must not import
// audit/net/http dependencies — only authentication logic.
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// Bridge to the real implementation. We construct a Handler on the fly
	// here because tests inject richer handlers directly; in production
	// the router wires the Handler with all dependencies.
	h := defaultHandler
	if h == nil {
		http.Error(w, "admin handler not initialized", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.PostLogin(w, r)
}

// LogoutHandler satisfies the frozen plan signature.
//
//	func (s *Service) LogoutHandler(w http.ResponseWriter, r *http.Request)
func (s *Service) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h := defaultHandler
	if h == nil {
		http.Error(w, "admin handler not initialized", http.StatusInternalServerError)
		return
	}
	h.PostLogout(w, r)
}

// SetDefaultHandler registers the handler that the frozen-signature
// LoginHandler/LogoutHandler will delegate to. Called by main.go (and
// by tests that need to inject a custom handler).
func SetDefaultHandler(h *Handler) {
	defaultHandler = h
}

// defaultHandler is the singleton consulted by Service.LoginHandler /
// LogoutHandler. We use a singleton rather than stashing the handler on
// Service because Service's constructor signature is frozen.
var defaultHandler *Handler

// PostLogin authenticates the user, starts a session, audits the
// outcome, and redirects on success.
func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}

	login := r.FormValue("login")
	password := r.FormValue("password")
	next := r.FormValue("next")

	user, err := h.service.Authenticate(r.Context(), login, password)
	if err != nil {
		// Audit every failure with the attempted login (or "empty" if
		// the form was blank) so brute-force attempts are visible.
		auditLogin := login
		if auditLogin == "" {
			auditLogin = "empty"
		}
		actor := auditLogin
		auditCtx := audit.WithActor(r.Context(), actor)
		if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
			Action:     "login.failure",
			EntityType: "session",
			Actor:      actor,
			Details:    map[string]any{"reason": errReason(err)},
		}); auditErr != nil {
			h.log.WithError(auditErr).Error("audit login failure")
		}
		http.Error(w, loginErrorMsg, http.StatusUnauthorized)
		return
	}

	// Successful authentication: rotate the session ID before storing
	// any user-derived data so a pre-login fixation cannot survive login.
	if err := h.sessions.RenewToken(r.Context()); err != nil {
		h.log.WithError(err).Error("renew session token")
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	h.sessions.Put(r.Context(), SessionKeyUserID, user.ID)
	h.sessions.Put(r.Context(), SessionKeyUserLogin, user.Login)

	auditCtx := audit.WithActor(r.Context(), user.Login)
	if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
		Action:     "login.success",
		EntityType: "session",
		Actor:      user.Login,
		EntityID:   sql.NullInt64{Int64: user.ID, Valid: true},
	}); auditErr != nil {
		h.log.WithError(auditErr).Error("audit login success")
	}

	redirect := next
	if redirect == "" || !isSafeRedirect(redirect) {
		redirect = "/programs"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// PostLogout destroys the session and redirects to /login. We accept
// GET (per the plan's router sketch) and POST for symmetry with form
// submissions; both behave identically.
func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	login := h.sessions.GetString(r.Context(), SessionKeyUserLogin)

	if err := h.sessions.Destroy(r.Context()); err != nil {
		h.log.WithError(err).Error("destroy session")
	}

	if login != "" {
		auditCtx := audit.WithActor(r.Context(), login)
		if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
			Action:     "logout",
			EntityType: "session",
			Actor:      login,
		}); auditErr != nil {
			h.log.WithError(auditErr).Error("audit logout")
		}
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func errReason(err error) string {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, ErrUserDisabled):
		return "user_disabled"
	default:
		return "internal"
	}
}

// isSafeRedirect prevents open-redirects: only same-host relative paths
// are allowed, and we reject anything that net/url parses as absolute,
// contains a backslash, or carries percent-encoded slashes that decode
// to a host-relative scheme like `//evil.com`.
func isSafeRedirect(target string) bool {
	if target == "" || strings.ContainsAny(target, "\\") {
		return false
	}
	lower := strings.ToLower(target)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return false
	}
	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || u.Host != "" || u.Scheme != "" {
		return false
	}
	return strings.HasPrefix(u.Path, "/")
}
