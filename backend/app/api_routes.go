package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func registerAPIRoutes(router chi.Router, deps Deps, c *container) {
	router.Route("/api", func(r chi.Router) {
		r.Get("/session", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"authenticated":true}`))
		})
	})
}
