package app

import (
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
)

type container struct {
	auditSvc        *audit.Service
	adminHandler    *admin.Handler
	programHandler  *programs.Handler
	employerHandler *employers.Handler
	peopleHandler   *people.Handler
	auditHandler    *audit.Handler
	protocolHandler *protocols.Handler
	requestHandler  *requests.Handler
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

	requestHandler := requests.NewHandler(queries, auditSvc, deps.Log)
	requestHandler.Service().SetDB(deps.Database)

	return &container{
		auditSvc:        auditSvc,
		adminHandler:    adminHandler,
		programHandler:  programs.NewHandler(queries, auditSvc),
		employerHandler: employers.NewHandler(queries, auditSvc),
		peopleHandler:   people.NewHandler(queries, auditSvc),
		auditHandler:    audit.NewHandler(queries),
		protocolHandler: protocols.NewHandler(queries, deps.Database, auditSvc),
		requestHandler:  requestHandler,
		documentHandler: documents.NewHandler(queries, auditSvc, documentSvc),
		requireAuth:     admin.RequireAuth(deps.Sessions, deps.Log),
	}
}
