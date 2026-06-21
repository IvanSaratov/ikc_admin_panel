// Package documents owns the D3 slice: generating Mintrud-compliant XML
// and DOCX files for fixed protocols, persisting one generation_runs row
// per attempt, and serving the files back to the operator.
//
// The package follows the adapter-layer convention: backend/documents/ is
// the ONLY package that imports backend/documents/legacy/. Other packages
// see only the Service + Handler types defined here plus the top-level
// GenerateXML / GenerateDOCX helpers frozen in section 0.2 of the core
// MVP plan.
package documents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// ErrInvalidType is returned when callers ask for ?type=other than xml/docx.
var ErrInvalidType = errors.New("invalid document type")

// GenerationRun is a re-export of the storagedb type so callers don't
// have to import storage/db explicitly (matches section 0.2 of the core
// MVP plan, which lists the return type as `*GenerationRun`).
type GenerationRun = storagedb.GenerationRun

// Service coordinates generation: it loads the protocol through the
// adapter, hands the resulting RegistrySet to the legacy package, and
// records every attempt in generation_runs. The handler is a thin shell
// over this type.
type Service struct {
	db      *sql.DB
	queries *storagedb.Queries
	audit   *audit.Service
	log     *slog.Logger
	now     func() time.Time
}

// NewService wires the dependencies. audit and log are optional (nil
// allowed for tests); the service still records generation_runs in
// either case because that table is the source of truth for downloads.
func NewService(db *sql.DB, queries *storagedb.Queries, auditSvc *audit.Service, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		db:      db,
		queries: queries,
		audit:   auditSvc,
		log:     log,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// timestamp returns the service's current clock value as RFC3339.
func (s *Service) timestamp() string {
	return s.now().Format(time.RFC3339)
}

// recordGenerationRun inserts a single generation_runs row. `status` is
// one of 'success' / 'failed' / 'stale' (the schema CHECK constraint).
// `errMsg` is persisted when the run failed.
//
// Returns the inserted row. The function logs and swallows storage
// errors so a missing generation_runs row never blocks a download path.
func (s *Service) recordGenerationRun(ctx context.Context, protocolID int64, docType, status, fileName, errMsg string) (storagedb.GenerationRun, error) {
	if s.queries == nil {
		return storagedb.GenerationRun{}, fmt.Errorf("nil queries in service")
	}
	var errMsgNS sql.NullString
	if errMsg != "" {
		errMsgNS = sql.NullString{String: errMsg, Valid: true}
	}
	var fileNameNS sql.NullString
	if fileName != "" {
		fileNameNS = sql.NullString{String: fileName, Valid: true}
	}
	row, err := s.queries.CreateGenerationRun(ctx, storagedb.CreateGenerationRunParams{
		ProtocolID:   protocolID,
		Type:         docType,
		Status:       status,
		FileName:     fileNameNS,
		GeneratedAt:  s.timestamp(),
		ErrorMessage: errMsgNS,
		CreatedAt:    s.timestamp(),
	})
	if err != nil {
		s.log.Error("create generation_runs row", "protocol_id", protocolID, "type", docType, "err", err)
		return storagedb.GenerationRun{}, fmt.Errorf("insert generation_run: %w", err)
	}
	return row, nil
}

// recordAudit writes an action_log entry for the generation event. The
// actor is pulled from the request context (audit.WithActor) or defaults
// to audit.DefaultActor ("operator_unidentified") — see audit.Service.
func (s *Service) recordAudit(ctx context.Context, action string, protocolID int64, details map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		Action:     action,
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: protocolID, Valid: true},
		Details:    details,
	})
}

// --- default singleton so the frozen top-level wrappers can work -------
//
// The core MVP plan (section 0.2) freezes GenerateXML/GenerateDOCX as
// top-level package functions with signature
//
//	func GenerateXML(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
//	func GenerateDOCX(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
//
// We back them with a process-wide Service that main.go initialises at
// startup. Tests can either swap the default via SetDefaultService or
// call GenerateXML/GenerateDOCX directly with their own *storagedb.Queries
// after wiring a service through SetDefaultForTesting.

var (
	defaultMu      sync.RWMutex
	defaultService *Service
)

// SetDefaultService installs the process-wide service used by the
// top-level GenerateXML / GenerateDOCX wrappers. Call from main.go right
// after Migrate succeeds and before the HTTP server starts.
func SetDefaultService(svc *Service) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultService = svc
}

// currentService returns the registered default service or an
// unconfigured one (which will then fail at the audit / db step). The
// latter path is intentional: a missing default during normal startup
// is a programming error and should fail loudly.
func currentService() *Service {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultService
}
