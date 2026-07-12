package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Deps собирает зависимости времени выполнения для авторизации, CSRF и логов.
type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       *zap.Logger
	Frontend  FrontendConfig
}

// NewRouter собирает роутер с общей базой авторизации: Sessions.LoadAndSave
// применяется ко всем запросам, JSON API регистрируется без CSRF, а legacy
// POST form endpoints остаются под CSRF для обратной совместимости.
func NewRouter(deps Deps) http.Handler {
	if deps.Log == nil {
		deps.Log = zap.NewNop()
	}
	if deps.Sessions == nil {
		// Тесты, которым не важны сессии, могут передать Deps без Sessions.
		deps.Sessions = scs.New()
	}
	if deps.CSRF == nil {
		// Пустой CSRF сохраняет совместимость тестов; боевой код задает прослойку.
		deps.CSRF = func(next http.Handler) http.Handler { return next }
	}

	router := chi.NewRouter()
	router.Use(deps.Sessions.LoadAndSave)

	container := newContainer(deps)
	registerAPIRoutes(router, deps, container)
	router.Group(func(r chi.Router) {
		r.Use(deps.CSRF)
		registerRoutes(r, deps, container)
	})
	registerFrontendRoutes(router, deps.Frontend)

	return router
}
