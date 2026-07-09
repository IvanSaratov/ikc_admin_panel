package app

import (
	"database/sql"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/documents"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs"
	"github.com/IvanSaratov/ikc_admin_panel/backend/protocols"
	"github.com/IvanSaratov/ikc_admin_panel/backend/requests"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

// Deps bundles the runtime dependencies NewRouter needs to wire auth
// (session manager + CSRF middleware) into every request. Construct it
// once in main.go and pass it to NewRouter.
type Deps struct {
	Database  *sql.DB
	Sessions  *scs.SessionManager
	CSRF      func(http.Handler) http.Handler
	LoginRate *admin.RateLimiter
	Log       logrus.FieldLogger
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
		deps.Log = logrus.StandardLogger()
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

	// D3: the documents Service is the only one that wraps the legacy
	// XML/DOCX generator. It also backs the top-level GenerateXML /
	// GenerateDOCX functions called by the handler. We register it as
	// the process-wide default so callers outside this package (e.g.
	// tests) can still produce documents.
	documentSvc := documents.NewService(deps.Database, queries, auditSvc, deps.Log)
	documents.SetDefaultService(documentSvc)

	adminSvc := admin.NewService(queries)
	adminStore := admin.NewStore(queries)
	adminHandler := admin.NewHandler(adminSvc, auditSvc, deps.Sessions, deps.Log)
	admin.SetDefaultHandler(adminHandler)

	authMiddleware := admin.RequireAuth(deps.Sessions, deps.Log)

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
	router.With(requestLogger(deps.Log)).Get("/login", adminHandler.GetLogin)
	if deps.LoginRate != nil {
		router.With(requestLogger(deps.Log), admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, auditSvc)).
			Post("/login", adminHandler.PostLogin)
	} else {
		router.With(requestLogger(deps.Log)).Post("/login", adminHandler.PostLogin)
	}

	// Protected group: everything else goes through auth.
	router.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(withActor)
		r.Use(requestLogger(deps.Log))

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/programs", http.StatusSeeOther)
		})
		// Logout is POST-only (with CSRF) so a third-party <img> tag or
		// link prefetch cannot trigger it. See shell.templ for the form.
		r.Post("/logout", adminHandler.PostLogout)

		programHandler := programs.NewHandler(queries, auditSvc)
		employerHandler := employers.NewHandler(queries, auditSvc)
		peopleHandler := people.NewHandler(queries, auditSvc)
		auditHandler := audit.NewHandler(queries)
		protocolHandler := protocols.NewHandler(queries, deps.Database, auditSvc)
		requestHandler := requests.NewHandler(queries, auditSvc, deps.Log)
		requestHandler.Service().SetDB(deps.Database)
		documentHandler := documents.NewHandler(queries, auditSvc, documentSvc)

		// Audit log viewer (D4). Read-only — does not write to
		// action_log. Mutations go through audit.Service.Record from
		// every other handler in this group.
		r.Get("/audit", auditHandler.List)

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

		// Documents (D3): generate + download. The POST endpoints are
		// CSRF-protected by the outer Group; the GET download is a plain
		// file stream.
		r.Post("/protocols/{id}/generate", documentHandler.Generate)
		r.Get("/protocols/{id}/download", documentHandler.Download)

		// Requests (XLSX upload + staging).
		r.Get("/requests", requestHandler.List)
		r.Get("/requests/new", requestHandler.NewRequestForm)
		r.Post("/requests/new", requestHandler.Upload)
		r.Get("/requests/{id}", requestHandler.Detail)
		r.Post("/requests/{id}/rows/{rowID}/apply", requestHandler.ApplyRow)
		r.Post("/requests/{id}/rows/{rowID}/skip", requestHandler.SkipRow)
	})

	// adminStore is referenced so the import is used; it's needed by
	// future slices (e.g. user management) but kept here so that the
	// package compiles cleanly. Remove this line once store is consumed
	// by a handler.
	_ = adminStore

	return router
}
