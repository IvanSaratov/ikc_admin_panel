package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/alexedwards/scs/v2"
	"go.uber.org/zap"
)

type apiIdentityKey struct{}

// APIIdentity is the minimal current database identity exposed to API handlers.
type APIIdentity struct {
	ID    int64
	Login string
	Role  string
}

// APIIdentityFromContext returns the authenticated API identity.
func APIIdentityFromContext(ctx context.Context) (APIIdentity, bool) {
	identity, ok := ctx.Value(apiIdentityKey{}).(APIIdentity)
	return identity, ok
}

// RequireAPIAuth authenticates JSON API requests without HTML redirects and
// reloads user status and role from the database on every request.
func RequireAPIAuth(
	sessions *scs.SessionManager,
	store *Store,
	log *zap.Logger,
) func(http.Handler) http.Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessions == nil || store == nil {
				log.Error("API authentication is unavailable")
				writeAPIAuthProblem(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Сервис временно недоступен")
				return
			}
			userID := sessions.GetInt64(r.Context(), SessionKeyUserID)
			if userID <= 0 {
				writeAPIAuthProblem(w, r, http.StatusUnauthorized, "unauthorized", "Требуется аутентификация")
				return
			}
			user, err := store.GetByID(r.Context(), userID)
			if err != nil {
				if errors.Is(err, ErrUserNotFound) {
					_ = sessions.Destroy(r.Context())
					writeAPIAuthProblem(w, r, http.StatusUnauthorized, "unauthorized", "Требуется аутентификация")
					return
				}
				log.Error("API user lookup failed", zap.String("trace_id", api.TraceID(r.Context())))
				writeAPIAuthProblem(w, r, http.StatusServiceUnavailable, "storage_unavailable", "Сервис временно недоступен")
				return
			}
			if user.Status != "active" {
				_ = sessions.Destroy(r.Context())
				writeAPIAuthProblem(w, r, http.StatusUnauthorized, "unauthorized", "Требуется аутентификация")
				return
			}

			identity := APIIdentity{ID: user.ID, Login: user.Login, Role: user.Role}
			ctx := context.WithValue(r.Context(), apiIdentityKey{}, identity)
			ctx = audit.WithActor(ctx, user.Login)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAPIRoles permits only identities whose current role is listed.
func RequireAPIRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := APIIdentityFromContext(r.Context())
			if !ok {
				writeAPIAuthProblem(w, r, http.StatusUnauthorized, "unauthorized", "Требуется аутентификация")
				return
			}
			if _, ok := allowed[identity.Role]; !ok {
				writeAPIAuthProblem(w, r, http.StatusForbidden, "forbidden", "Недостаточно прав")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAPIAuthProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	api.WriteProblem(w, r, api.Problem{Status: status, Code: code, Detail: detail})
}
