package requests

import (
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/sirupsen/logrus"
)

// Handler wires the requests.Service into the application container. The
// old server-rendered request pages and form endpoints were removed in
// Stage 5; request behavior remains in Service.
type Handler struct {
	service *Service
	queries *storagedb.Queries
	audit   *audit.Service
	log     logrus.FieldLogger
}

// NewHandler constructs a requests.Handler. db is wired separately into
// Service by app/container.go so ApplyRow can use storage.WithTx.
func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service, log logrus.FieldLogger) *Handler {
	if log == nil {
		log = logrus.StandardLogger()
	}
	svc := NewService(queries, auditSvc)
	return &Handler{
		service: svc,
		queries: queries,
		audit:   auditSvc,
		log:     log,
	}
}

// Service exposes the underlying *Service so callers can wire the *sql.DB
// handle into ApplyRow's tx support.
func (h *Handler) Service() *Service { return h.service }
