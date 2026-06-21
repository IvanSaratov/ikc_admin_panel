package app

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	"github.com/IvanSaratov/ikc_admin_panel/backend/protocols"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// Deps bundles the runtime dependencies NewRouter needs to wire auth
// (session manager + CSRF middleware) into every request. Construct it
// once in main.go and pass it to NewRouter.
type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       *slog.Logger
}

// NewRouter wires the application router with F3 auth baseline:
//   - scs LoadAndSave (cookie I/O) on every request
//   - CSRF middleware on every request (login form included)
//   - /login GET/POST public
//   - everything else inside admin.RequireAuth
//
// The split keeps the security baseline (tech-stack.md:122-135) global
// — there's no route that can accidentally skip auth or CSRF.
func NewRouter(deps Deps) http.Handler {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	if deps.Sessions == nil {
		// Tests that don't care about sessions construct their own Deps
		// without Sessions. Fall back to a no-op loader so router setup
		// doesn't panic in those cases.
		deps.Sessions = scs.New()
	}
	if deps.CSRF == nil {
		// Provide a no-op CSRF middleware so legacy tests (which don't
		// know about CSRF yet) keep working. Production main.go must
		// always set Deps.CSRF.
		deps.CSRF = func(next http.Handler) http.Handler { return next }
	}

	router := chi.NewRouter()
	queries := storagedb.New(deps.Database)
	auditSvc := audit.NewService(queries)

	adminSvc := admin.NewService(queries)
	adminStore := admin.NewStore(queries)
	adminHandler := admin.NewHandler(adminSvc, auditSvc, deps.Sessions, deps.Log)
	admin.SetDefaultHandler(adminHandler)

	// Wire actor propagation: RequireAuth publishes the login on the
	// request context via audit.WithActor so downstream audit.Record
	// calls attribute the row to the operator. We do this by wrapping
	// the inner handler with a small adapter that injects the actor.
	authMiddleware := admin.RequireAuth(deps.Sessions, deps.Log)
	withActor := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if login := admin.UserLoginFromContext(r.Context()); login != "" {
				r = r.WithContext(audit.WithActor(r.Context(), login))
			}
			next.ServeHTTP(w, r)
		})
	}

	// scs LoadAndSave + CSRF on every request (including /login).
	router.Use(deps.Sessions.LoadAndSave)
	router.Use(deps.CSRF)

	// Public auth endpoints. csrfField in the login form makes the POST
	// safe; RequireAuth is intentionally NOT applied here so unauth'd
	// users can reach the form.
	//
	// The login rate limiter sits BETWEEN csrf and the PostLogin handler
	// so brute-force POSTs are rejected before the bcrypt path. The
	// middleware is a no-op on every other route; see its docs.
	router.Get("/login", adminHandler.GetLogin)
	if deps.LoginRate != nil {
		router.With(admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, auditSvc)).
			Post("/login", adminHandler.PostLogin)
	} else {
		router.Post("/login", adminHandler.PostLogin)
	}

	// Protected group: everything else goes through auth.
	router.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(withActor)

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/programs", http.StatusSeeOther)
		})
		// Logout is POST-only (with CSRF) so a third-party <img> tag or
		// link prefetch cannot trigger it. See shell.templ for the form.
		r.Post("/logout", adminHandler.PostLogout)

		programHandler := programs.NewHandler(queries, auditSvc)
		employerHandler := employers.NewHandler(queries, auditSvc)
		peopleHandler := people.NewHandler(queries, auditSvc)
		protocolHandler := protocols.NewHandler(queries, deps.Database, auditSvc)

		// Programs: groups.
		r.Get("/programs", programHandler.List)
		r.Post("/programs/groups", programHandler.CreateGroup)
		r.Get("/programs/groups/{id}/edit", programHandler.EditGroup)
		r.Post("/programs/groups/{id}/edit", programHandler.EditGroup)
		r.Post("/programs/groups/{id}/deactivate", programHandler.DeactivateGroup)

		// Programs: programs.
		r.Post("/programs", programHandler.CreateProgram)
		r.Get("/programs/{id}/edit", programHandler.EditProgram)
		r.Post("/programs/{id}/edit", programHandler.EditProgram)
		r.Post("/programs/{id}/deactivate", programHandler.DeactivateProgram)

		// Employers.
		r.Get("/employers", employerHandler.List)
		r.Post("/employers", employerHandler.Create)
		r.Get("/employers/{id}", employerHandler.Detail)
		r.Get("/employers/{id}/edit", employerHandler.Edit)
		r.Post("/employers/{id}", employerHandler.Edit)
		r.Post("/employers/{id}/deactivate", employerHandler.Deactivate)

		// Workers + assignments.
		r.Get("/workers", peopleHandler.List)
		r.Post("/workers", peopleHandler.CreateWorker)
		r.Get("/workers/{id}", peopleHandler.Detail)
		r.Get("/workers/{id}/edit", peopleHandler.Edit)
		r.Post("/workers/{id}", peopleHandler.Edit)
		r.Post("/workers/assignments", peopleHandler.AssignEmployer)
		r.Post("/workers/assignments/{id}/deactivate", peopleHandler.DeactivateAssignment)

		// Protocols (D2): list, create, detail, fix, transition, participants.
		// Every POST goes through CSRF + RequireAuth (the outer Group enforces
		// both). The /protocols/{id}/participants/{pid} route uses a form
		// field `_method=delete` so HTML forms (which only emit GET/POST)
		// can soft-delete a participant.
		r.Get("/protocols", protocolHandler.List)
		r.Post("/protocols", protocolHandler.Create)
		r.Get("/protocols/{id}", protocolHandler.Detail)
		r.Get("/protocols/{id}/fix", protocolHandler.Fix)
		r.Post("/protocols/{id}/fix", protocolHandler.Fix)
		r.Post("/protocols/{id}/participants", protocolHandler.AddParticipant)
		r.Post("/protocols/{id}/participants/{pid}", protocolHandler.RemoveParticipant)
		r.Post("/protocols/{id}/transition", protocolHandler.Transition)
	})

	// adminStore is referenced so the import is used; it's needed by
	// future slices (e.g. user management) but kept here so that the
	// package compiles cleanly. Remove this line once store is consumed
	// by a handler.
	_ = adminStore

	return router
}
