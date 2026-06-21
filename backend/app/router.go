package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

func NewRouter(database *sql.DB) http.Handler {
	router := chi.NewRouter()
	queries := storagedb.New(database)

	programHandler := programs.NewHandler(queries)
	employerHandler := employers.NewHandler(queries)
	peopleHandler := people.NewHandler(queries)

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/programs", http.StatusSeeOther)
	})
	router.Get("/programs", programHandler.List)
	router.Post("/programs/groups", programHandler.CreateGroup)
	router.Post("/programs", programHandler.CreateProgram)
	router.Get("/employers", employerHandler.List)
	router.Post("/employers", employerHandler.Create)
	router.Get("/workers", peopleHandler.List)
	router.Post("/workers", peopleHandler.CreateWorker)
	router.Post("/workers/assignments", peopleHandler.AssignEmployer)

	return router
}
