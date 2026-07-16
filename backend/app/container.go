package app

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/documents"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type container struct {
	auditSvc        *audit.Service
	adminHandler    *admin.Handler
	documentHandler *documents.Handler
	importHandler   *imports.HTTPHandler
	requireAuth     func(http.Handler) http.Handler
	requireAPIAuth  func(http.Handler) http.Handler
}

func newContainer(deps Deps) *container {
	queries := storagedb.New(deps.Database)
	auditSvc := audit.NewService(queries)

	// Сервис документов остается единственной оберткой над старым генератором
	// XML/DOCX и регистрируется как значение по умолчанию для внешних вызовов.
	documentSvc := documents.NewService(deps.Database, queries, auditSvc, deps.Log)
	documents.SetDefaultService(documentSvc)

	adminSvc := admin.NewService(queries)
	adminHandler := admin.NewHandler(adminSvc, auditSvc, deps.Sessions, deps.Log)
	admin.SetDefaultHandler(adminHandler)

	var importHandler *imports.HTTPHandler
	if deps.ImportService != nil {
		var err error
		importHandler, err = imports.NewHTTPHandler(
			deps.ImportService,
			imports.NewReadService(queries),
			imports.DefaultConfig().LegacyLimits,
			deps.Log,
		)
		if err != nil {
			panic("construct import HTTP handler: " + err.Error())
		}
	}

	return &container{
		auditSvc:        auditSvc,
		adminHandler:    adminHandler,
		documentHandler: documents.NewHandler(queries, auditSvc, documentSvc),
		importHandler:   importHandler,
		requireAuth:     admin.RequireAuth(deps.Sessions, deps.Log),
		requireAPIAuth:  admin.RequireAPIAuth(deps.Sessions, admin.NewStore(queries), deps.Log),
	}
}
