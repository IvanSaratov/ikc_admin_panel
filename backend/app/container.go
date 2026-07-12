package app

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/documents"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type container struct {
	auditSvc        *audit.Service
	adminHandler    *admin.Handler
	documentHandler *documents.Handler
	requireAuth     func(http.Handler) http.Handler
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

	return &container{
		auditSvc:        auditSvc,
		adminHandler:    adminHandler,
		documentHandler: documents.NewHandler(queries, auditSvc, documentSvc),
		requireAuth:     admin.RequireAuth(deps.Sessions, deps.Log),
	}
}
