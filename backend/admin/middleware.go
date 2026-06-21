package admin

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// SessionKeyUserID is the scs session key under which we store the
// authenticated user's primary key.
const SessionKeyUserID = "user_id"

// SessionKeyUserLogin is the scs session key under which we store the
// authenticated user's login. Storing the login alongside the PK lets
// downstream code (audit, shell layout) avoid a DB round-trip on every
// authenticated request.
const SessionKeyUserLogin = "user_login"

// ContextKeyUserLogin is the request-context key under which RequireAuth
// publishes the authenticated user's login. audit.Service uses
// ActorFromContext to read this when RecordInput.Actor is empty.
//
// The value is a plain string so any caller can read it without an extra
// import. We deliberately do NOT expose the whole User record on the
// context — keeping the surface narrow makes the auth model auditable.
type ctxKey int

const ctxKeyUserLogin ctxKey = iota

// ActorFromContext returns the login of the authenticated user attached
// to the request context by RequireAuth, or "" if no such user exists.
//
// audit.Service.Record calls this when RecordInput.Actor is empty so
// that, after F3, every audit row written from an authenticated handler
// automatically carries the operator's login (instead of the historical
// "operator_unidentified" default).
func ActorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserLogin).(string); ok {
		return v
	}
	return ""
}

// UserLoginFromContext is an alias for ActorFromContext. Some callers
// (e.g. the shell layout) find "UserLoginFromContext" more readable than
// "ActorFromContext"; both return the same string.
func UserLoginFromContext(ctx context.Context) string {
	return ActorFromContext(ctx)
}

// RequireAuth returns a middleware that gates access to authenticated
// routes. Its signature is frozen by the plan (section 0.2).
//
// Behaviour:
//   - If the session has no user_id → redirect to /login (preserving
//     the requested URL as ?next=...). Status code is 303 See Other so
//     the browser follows with a GET.
//   - If the session has user_id → publish the login (also stored in
//     the session) on the request context and call the next handler.
//
// The session manager is responsible for cookie I/O; this middleware
// only inspects the session payload. We do NOT hit the DB here: the
// session already carries everything we need for routing decisions and
// audit attribution. A disabled-user check belongs at login time
// (Service.Authenticate) — a session issued to an active user remains
// valid for its TTL.
func RequireAuth(sm *scs.SessionManager, log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			rawID := sm.GetInt64(ctx, SessionKeyUserID)
			if rawID == 0 {
				redirectToLogin(w, r)
				return
			}

			login := sm.GetString(ctx, SessionKeyUserLogin)
			if login == "" {
				// Defensive: someone Put user_id without user_login
				// (e.g. a stale cookie from a previous deploy).
				// Treat as unauthenticated.
				log.Warn("auth: session has user_id but no user_login; redirecting to login")
				_ = sm.Destroy(ctx)
				redirectToLogin(w, r)
				return
			}

			ctx = context.WithValue(ctx, ctxKeyUserLogin, login)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := "/login"
	if r.URL != nil && r.URL.Path != "" && r.URL.Path != "/login" {
		target = "/login?next=" + r.URL.RequestURI()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
