package app

import (
	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/go-chi/chi/v5"
)

func registerRoutes(router chi.Router, deps Deps, c *container) {
	if deps.LoginRate != nil {
		router.With(requestLogger(deps.Log), admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, c.auditSvc)).
			Post("/login", c.adminHandler.PostLogin)
	} else {
		router.With(requestLogger(deps.Log)).Post("/login", c.adminHandler.PostLogin)
	}

	// Защищенная группа: все остальные маршруты проходят авторизацию, actor и лог запроса.
	router.Group(func(r chi.Router) {
		r.Use(c.requireAuth)
		r.Use(withActor)
		r.Use(requestLogger(deps.Log))

		// Выход принимает только POST с CSRF, чтобы внешняя ссылка или
		// предзагрузка не могли завершить сессию.
		r.Post("/logout", c.adminHandler.PostLogout)

		// Документы: генерация и скачивание.
		r.Post("/protocols/{id}/generate", c.documentHandler.Generate)
		r.Get("/protocols/{id}/download", c.documentHandler.Download)
	})
}
