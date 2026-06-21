package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

func NewRouter(database *sql.DB) http.Handler {
	router := chi.NewRouter()
	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)

	programHandler := programs.NewHandler(queries, auditSvc)
	employerHandler := employers.NewHandler(queries, auditSvc)
	peopleHandler := people.NewHandler(queries, auditSvc)

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/programs", http.StatusSeeOther)
	})

	// Programs: groups.
	router.Get("/programs", programHandler.List)
	router.Post("/programs/groups", programHandler.CreateGroup)
	router.Get("/programs/groups/{id}/edit", programHandler.EditGroup)
	router.Post("/programs/groups/{id}/edit", programHandler.EditGroup)
	router.Post("/programs/groups/{id}/deactivate", programHandler.DeactivateGroup)

	// Programs: programs.
	router.Post("/programs", programHandler.CreateProgram)
	router.Get("/programs/{id}/edit", programHandler.EditProgram)
	router.Post("/programs/{id}/edit", programHandler.EditProgram)
	router.Post("/programs/{id}/deactivate", programHandler.DeactivateProgram)

	// Employers.
	router.Get("/employers", employerHandler.List)
	router.Post("/employers", employerHandler.Create)
	router.Get("/employers/{id}", employerHandler.Detail)
	router.Get("/employers/{id}/edit", employerHandler.Edit)
	router.Post("/employers/{id}", employerHandler.Edit)
	router.Post("/employers/{id}/deactivate", employerHandler.Deactivate)

	// Workers + assignments.
	router.Get("/workers", peopleHandler.List)
	router.Post("/workers", peopleHandler.CreateWorker)
	router.Get("/workers/{id}", peopleHandler.Detail)
	router.Get("/workers/{id}/edit", peopleHandler.Edit)
	router.Post("/workers/{id}", peopleHandler.Edit)
	router.Post("/workers/assignments", peopleHandler.AssignEmployer)
	router.Post("/workers/assignments/{id}/deactivate", peopleHandler.DeactivateAssignment)

	return router
}
