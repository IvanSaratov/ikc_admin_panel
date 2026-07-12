package app

import (
	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
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
	})
}
