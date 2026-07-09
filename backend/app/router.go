package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

// Deps собирает зависимости времени выполнения для авторизации, CSRF и логов.
type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       logrus.FieldLogger
}

// NewRouter собирает роутер с общей базой авторизации: Sessions.LoadAndSave,
// затем CSRF для каждого запроса, затем регистрация публичных и защищенных URL.
func NewRouter(deps Deps) http.Handler {
	if deps.Log == nil {
		deps.Log = logrus.StandardLogger()
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
	router.Use(deps.CSRF)

	registerRoutes(router, deps, newContainer(deps))

	return router
}
