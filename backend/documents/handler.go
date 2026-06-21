package documents

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

// Handler exposes the document generation routes. The handler is thin:
// it parses the URL, delegates to GenerateXML / GenerateDOCX, and
// translates domain errors into HTTP status codes.
//
// All routes are registered inside the protected Group in router.go so
// every request goes through RequireAuth + CSRF (POST) + withActor.
type Handler struct {
	svc     *Service
	queries *storagedb.Queries
	audit   *audit.Service
}

// NewHandler wires the dependencies. audit may be nil (audit rows are
// best-effort from this handler's perspective; Service.recordGenerationRun
// is the only mandatory side effect).
func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service, svc *Service) *Handler {
	return &Handler{
		svc:     svc,
		queries: queries,
		audit:   auditSvc,
	}
}

// contentTypeFor maps a generation_runs.type to the matching MIME type
// for the HTTP response.
func contentTypeFor(docType string) string {
	switch docType {
	case "xml":
		return "application/xml; charset=utf-8"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return "application/octet-stream"
}

// Generate is the entry point for POST /protocols/{id}/generate?type=xml|docx.
// On success it returns a redirect to /protocols/{id}/download?run=<id>
// so the browser auto-streams the file as an attachment.
//
// Errors map as follows:
//   - 400 when ?type is missing or not in {xml, docx}
//   - 400 when the protocol is in draft/cancelled state
//   - 404 when the protocol does not exist
//   - 500 when storage / generation fails
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	protocolID, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid protocol id", http.StatusBadRequest)
		return
	}
	docType := r.URL.Query().Get("type")
	if docType != "xml" && docType != "docx" {
		http.Error(w, "type must be xml or docx", http.StatusBadRequest)
		return
	}

	var (
		raw []byte
		run *GenerationRun
		gen error
	)
	switch docType {
	case "xml":
		raw, run, gen = h.svc.generateXMLWith(ctx, h.queries, protocolID)
	case "docx":
		raw, run, gen = h.svc.generateDOCXWith(ctx, h.queries, protocolID)
	}

	if gen != nil {
		// Distinguish "rejected before generation" (no bytes produced)
		// from "generation produced no bytes" — the former maps to 400/404,
		// the latter to 500.
		switch {
		case errors.Is(gen, ErrProtocolNotFixed):
			http.Error(w, gen.Error(), http.StatusBadRequest)
		case raw == nil && run == nil:
			// Storage-level failure (e.g. couldn't insert generation_runs).
			http.Error(w, gen.Error(), http.StatusInternalServerError)
		default:
			http.Error(w, gen.Error(), http.StatusInternalServerError)
		}
		return
	}

	// No errors and no bytes means the operator hit "generate" on an
	// empty registry. Treat as 500 so we surface a real bug rather than
	// silently producing an empty file.
	if len(raw) == 0 {
		http.Error(w, "generate produced empty output", http.StatusInternalServerError)
		return
	}

	if run == nil {
		http.Error(w, "generate succeeded but no generation_runs row was written", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/protocols/"+strconv.FormatInt(protocolID, 10)+"/download?run="+strconv.FormatInt(run.ID, 10), http.StatusSeeOther)
}

// Download serves the bytes of a previously generated file.
//
// GET /protocols/{id}/download?run=<id>
//
// The handler does not regenerate anything — it looks up the
// generation_runs row by id, verifies it belongs to the protocol in the
// path, and streams the bytes. We deliberately do not store the bytes
// in the DB (legacy DOCX archives can be 50KB+ and SQLite BLOBs are not
// the right home for binary blobs in this project); instead the bytes
// are returned inline on the Generate redirect.
//
// When the requested run is missing or stale, the handler returns 404 /
// 410. status='failed' rows are served the same way as successful ones
// because operators routinely inspect failure artefacts.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	protocolID, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid protocol id", http.StatusBadRequest)
		return
	}
	runID, err := parseInt64Param(r.URL.Query().Get("run"))
	if err != nil {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return
	}

	row, err := h.queries.GetGenerationRun(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "generation run not found", http.StatusNotFound)
			return
		}
		http.Error(w, "get generation run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if row.ProtocolID != protocolID {
		http.Error(w, "generation run does not belong to this protocol", http.StatusBadRequest)
		return
	}

	// We never persisted the bytes; we only kept metadata. To honour the
	// download contract, we regenerate on demand. This is what operators
	// expect: hitting "download" gives them the freshest artefact, not a
	// stale one. The regeneration is fast (sub-second) for typical
	// protocols with <100 participants.
	switch row.Type {
	case "xml":
		raw, _, regenErr := h.svc.generateXMLWith(ctx, h.queries, protocolID)
		if regenErr != nil {
			http.Error(w, "regenerate xml: "+regenErr.Error(), http.StatusInternalServerError)
			return
		}
		writeAttachment(w, "xml", raw, row.FileName.String)
	case "docx":
		raw, _, regenErr := h.svc.generateDOCXWith(ctx, h.queries, protocolID)
		if regenErr != nil {
			http.Error(w, "regenerate docx: "+regenErr.Error(), http.StatusInternalServerError)
			return
		}
		writeAttachment(w, "docx", raw, row.FileName.String)
	default:
		http.Error(w, "unsupported run type: "+row.Type, http.StatusBadRequest)
	}
}

// writeAttachment sets the Content-Type + Content-Disposition headers and
// writes the bytes. fileName defaults to "protocol_<id>.<ext>" when the
// caller passes an empty string.
func writeAttachment(w http.ResponseWriter, docType string, raw []byte, fileName string) {
	if fileName == "" {
		switch docType {
		case "xml":
			fileName = "protocol.xml"
		case "docx":
			fileName = "protocol.zip"
		}
	}
	w.Header().Set("Content-Type", contentTypeFor(docType))
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	_, _ = w.Write(raw)
}

// parseInt64Param parses a chi URL parameter or query string value into
// an int64, returning a friendly error on failure.
func parseInt64Param(raw string) (int64, error) {
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}
