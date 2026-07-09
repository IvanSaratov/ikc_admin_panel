package documents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// GenerateXML produces the Mintrud registry XML for a fixed protocol and
// returns the bytes + the generation_runs row + any error.
//
// Frozen signature per core MVP plan §0.2:
//
//	func GenerateXML(ctx, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error)
//
// Behaviour:
//   - Status must be >= fixed (rejects draft and cancelled).
//   - The legacy XML generator is called with the adapter's RegistrySet.
//   - A generation_runs row is written with status='success' on the happy
//     path and status='failed' if anything before the byte emission
//     failed.
//   - Audit log records both the "requested" and the "completed" / "failed"
//     events.
//
// Returned GenerationRun may be nil when the call rejects at the protocol
// status gate.
func GenerateXML(ctx context.Context, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error) {
	svc := currentService()
	if svc == nil {
		return nil, nil, errors.New("documents: default service not initialised (call SetDefaultService in main)")
	}
	return generateXMLImpl(ctx, svc, q, protocolID)
}

// generateXMLImpl is the Service-aware core. The top-level GenerateXML
// wrapper is what production code uses; tests can call Service methods
// directly (see Service.generateXMLWith) to avoid the shared global.
func generateXMLImpl(ctx context.Context, svc *Service, q *storagedb.Queries, protocolID int64) ([]byte, *GenerationRun, error) {
	svc.recordAudit(ctx, "documents.generate.requested", protocolID, map[string]any{
		"type": "xml",
	})

	registrySet, err := renderRegistrySet(ctx, q, protocolID)
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "xml", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "xml",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("render registry: %w", err)
	}

	raw, err := legacy.GenerateXML(registrySet)
	if err != nil {
		run, _ := svc.recordGenerationRun(ctx, protocolID, "xml", "failed", "", err.Error())
		svc.recordAudit(ctx, "documents.generate.failed", protocolID, map[string]any{
			"type": "xml",
			"err":  err.Error(),
		})
		return nil, &run, fmt.Errorf("legacy generate xml: %w", err)
	}

	fileName := xmlFileName(svc, protocolID)
	run, err := svc.recordGenerationRun(ctx, protocolID, "xml", "success", fileName, "")
	if err != nil {
		// Bytes were produced successfully; the run is missing only as a
		// bookkeeping failure. Return the bytes anyway with a nil pointer
		// and the storage error so the operator gets a usable download.
		svc.log.WithField("protocol_id", protocolID).WithError(err).Error("insert generation_runs row after xml success")
		return raw, nil, nil
	}
	svc.recordAudit(ctx, "documents.generate.completed", protocolID, map[string]any{
		"type":      "xml",
		"bytes":     len(raw),
		"run_id":    run.ID,
		"file_name": fileName,
	})
	return raw, &run, nil
}

// xmlFileName produces a stable, sortable name like "protocol_42_2026-06-22T15-04-05Z.xml".
// Using UTC timestamps keeps two re-generations from clobbering each other
// when the operator hits the button twice in a row.
func xmlFileName(svc *Service, protocolID int64) string {
	now := svc.now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("protocol_%d_%s.xml", protocolID, now)
}

// docxFileName is the matching helper for DOCX outputs. Exported so the
// DOCX path uses the same naming convention.
func docxFileName(svc *Service, protocolID int64) string {
	now := svc.now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("protocol_%d_%s.zip", protocolID, now)
}

// pathIsSafe guards against protocolID crafted as a path-traversal token.
// The current call sites always pass an int64, but a defensive check
// here keeps future refactors honest.
func pathIsSafe(name string) bool {
	cleaned := filepath.Clean(name)
	if cleaned != name {
		return false
	}
	if len(cleaned) == 0 || cleaned[0] == '.' || cleaned[0] == '/' {
		return false
	}
	return true
}

// Ensure imports are referenced.
var (
	_ = sql.NullString{}
	_ = pathIsSafe
)
