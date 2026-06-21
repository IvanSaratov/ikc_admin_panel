package admin

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin/views"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/alexedwards/scs/v2"
)

// loginErrorMsg is intentionally generic so we never leak whether the
// account exists, is disabled, or simply has the wrong password.
const loginErrorMsg = "Invalid login or password"

// Handler bundles the dependencies LoginHandler / LogoutHandler need.
//
// It is intentionally thin: the heavy lifting is in Service
// (authentication) and scs.SessionManager (cookie + session data). The
// handler is responsible for HTTP shape: parse form, render templ form,
// audit login success/failure, redirect on success.
type Handler struct {
	service  *Service
	audit    *audit.Service
	sessions *scs.SessionManager
	log      *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(service *Service, auditSvc *audit.Service, sessions *scs.SessionManager, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		service:  service,
		audit:    auditSvc,
		sessions: sessions,
		log:      log,
	}
}

// LoginHandler handles both GET (render form) and POST (authenticate +
// start session). It satisfies the frozen plan signature:
//
//	func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request)
//
// by being a thin wrapper that invokes the Handler's method. We cannot
// put this directly on Service because Service must not import
// audit/views/net/http dependencies — only authentication logic.
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// Bridge to the real implementation. We construct a Handler on the fly
	// here because tests inject richer handlers directly; in production
	// the router wires the Handler with all dependencies.
	h := defaultHandler
	if h == nil {
		http.Error(w, "admin handler not initialized", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodGet {
		h.GetLogin(w, r)
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

// GetLogin renders the login form. GET /login and the redirect target
// after a failed authentication both land here.
func (h *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := views.LoginForm(r, next, "").Render(r.Context(), w); err != nil {
		h.log.Error("render login form", slog.String("err", err.Error()))
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// PostLogin authenticates the user, starts a session, audits the
// outcome, and redirects on success.
func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginWithError(w, r, "", "Could not parse form")
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
			h.log.Error("audit login failure", slog.String("err", auditErr.Error()))
		}
		h.renderLoginWithError(w, r, next, loginErrorMsg)
		return
	}

	// Successful authentication: rotate the session ID before storing
	// any user-derived data so a pre-login fixation cannot survive login.
	if err := h.sessions.RenewToken(r.Context()); err != nil {
		h.log.Error("renew session token", slog.String("err", err.Error()))
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
		h.log.Error("audit login success", slog.String("err", auditErr.Error()))
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
		h.log.Error("destroy session", slog.String("err", err.Error()))
	}

	if login != "" {
		auditCtx := audit.WithActor(r.Context(), login)
		if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
			Action:     "logout",
			EntityType: "session",
			Actor:      login,
		}); auditErr != nil {
			h.log.Error("audit logout", slog.String("err", auditErr.Error()))
		}
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) renderLoginWithError(w http.ResponseWriter, r *http.Request, next string, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK) // re-render the form, not a 403/500
	if err := views.LoginForm(r, next, msg).Render(r.Context(), w); err != nil {
		h.log.Error("render login form (error)", slog.String("err", err.Error()))
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
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
