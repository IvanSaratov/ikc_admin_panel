package employers

import (
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type Handler struct {
	service *Service
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service) *Handler {
	return &Handler{service: NewService(queries, auditSvc), queries: queries}
}
