package protocols

import (
	"database/sql"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// Handler is kept as the wiring point for protocol dependencies. The
// server-rendered protocol pages and form endpoints were removed in Stage 5;
// protocol behavior lives in Service and JSON/API/document handlers.
type Handler struct {
	svc     *Service
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries, database *sql.DB, auditSvc *audit.Service) *Handler {
	return &Handler{
		svc:     NewService(queries, database, auditSvc),
		queries: queries,
	}
}
