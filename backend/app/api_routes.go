package app

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/go-chi/chi/v5"
)

func registerAPIRoutes(router chi.Router, deps Deps, c *container) {
	router.Route("/api", func(r chi.Router) {
		r.Get("/session", c.adminHandler.GetSessionJSON)
		if deps.LoginRate != nil {
			r.With(admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, c.auditSvc)).
				Post("/login", c.adminHandler.PostLoginJSON)
		} else {
			r.Post("/login", c.adminHandler.PostLoginJSON)
		}
		r.Post("/logout", c.adminHandler.PostLogoutJSON)

		if c.importHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(api.TraceMiddleware)
				r.Use(requestLogger(deps.Log))
				r.Use(c.requireAPIAuth)
				r.Use(captureRequestLogActor)
				r.With(admin.RequireAPIRoles("admin", "operator"), deps.CSRF).
					Get("/csrf", c.adminHandler.GetCSRFJSON)
			})

			r.Route("/imports", func(r chi.Router) {
				r.Use(api.TraceMiddleware)
				r.Use(requestLogger(deps.Log))
				r.Use(c.requireAPIAuth)
				r.Use(captureRequestLogActor)
				r.With(admin.RequireAPIRoles("admin", "operator")).Get("/", c.importHandler.List)
				r.With(admin.RequireAPIRoles("admin", "operator")).Get("/{id}", c.importHandler.Get)
				r.With(admin.RequireAPIRoles("admin"), deps.CSRF).Post("/legacy", c.importHandler.UploadLegacy)
				r.NotFound(func(w http.ResponseWriter, request *http.Request) {
					api.WriteProblem(w, request, api.Problem{
						Status: http.StatusNotFound,
						Code:   "not_found",
						Detail: "API маршрут не найден",
					})
				})
				r.MethodNotAllowed(func(w http.ResponseWriter, request *http.Request) {
					api.WriteProblem(w, request, api.Problem{
						Status: http.StatusMethodNotAllowed,
						Code:   "method_not_allowed",
						Detail: "HTTP метод не поддерживается",
					})
				})
			})
		}
	})
}
