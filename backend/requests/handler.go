package requests

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/requests/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

// Handler wires the requests.Service into HTTP routes. All routes are
// expected to be mounted inside the protected group in app/router.go
// (so RequireAuth and CSRF are inherited).
type Handler struct {
	service *Service
	queries *storagedb.Queries
	audit   *audit.Service
	log     logrus.FieldLogger
}

// NewHandler constructs a requests.Handler. db is wired into the
// service so ApplyRow can use storage.WithTx.
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

// Service exposes the underlying *Service so callers (notably
// app/router.go) can wire the *sql.DB handle into ApplyRow's tx.
func (h *Handler) Service() *Service { return h.service }

// List renders GET /requests with an optional ?status= filter.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	items, err := h.service.ListRequests(r.Context(), status)
	if err != nil {
		h.log.WithError(err).Error("list requests")
		http.Error(w, "list requests: "+err.Error(), http.StatusInternalServerError)
		return
	}
	views.List(r, items, status).Render(r.Context(), w)
}

// NewRequestForm renders GET /requests/new — the upload form. The form
// needs the employer list so the operator can attach the request to
// one.
func (h *Handler) NewRequestForm(w http.ResponseWriter, r *http.Request) {
	employers, err := h.queries.ListEmployers(r.Context())
	if err != nil {
		h.log.WithError(err).Error("list employers for new request")
		http.Error(w, "list employers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	views.NewRequest(r, employers, nil, "").Render(r.Context(), w)
}

// Upload handles POST /requests/new — multipart upload with XLSX.
//
// On success: redirect to /requests/{id}.
// On validation error: re-render the form with errors.
// On bad upload: 4xx with a clear operator-visible message.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	// Cap upload size BEFORE we read anything so a malicious client
	// can't pin memory.
	r.Body = http.MaxBytesReader(w, r.Body, ReadXLSXMaxBytes)
	if err := r.ParseMultipartForm(ReadXLSXMaxBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "upload too large (max 10MB)", http.StatusRequestEntityTooLarge)
			return
		}
		h.renderUploadError(w, r, err.Error(), "")
		return
	}

	employerIDRaw := strings.TrimSpace(r.FormValue("employer_id"))
	employerID, err := strconv.ParseInt(employerIDRaw, 10, 64)
	if err != nil || employerID <= 0 {
		h.renderUploadError(w, r, "Выберите работодателя.", employerIDRaw)
		return
	}
	received := strings.TrimSpace(r.FormValue("received_date"))
	if received == "" {
		received = time.Now().UTC().Format("2006-01-02")
	}

	file, header, err := r.FormFile("xlsx")
	if err != nil {
		h.renderUploadError(w, r, "Прикрепите XLSX-файл.", "")
		return
	}
	defer file.Close()

	if !IsAcceptableUploadContentType(header.Header.Get("Content-Type")) {
		// Fall through: the parser will still validate the bytes.
		// We only flag the content-type as suspect. Log a content
		// fingerprint (sha256 + length) instead of the raw filename or
		// Content-Type — both can carry control characters that would
		// break structured log parsing (log injection vector).
		// The hash is recomputed once the body is read; here we log
		// the header bytes alone (no body yet).
		h.log.WithFields(logrus.Fields{
			"content_type_hash": sha256Hex([]byte(header.Header.Get("Content-Type"))),
			"filename_hash":     sha256Hex([]byte(header.Filename)),
		}).Warn("upload with unusual content-type")
	}

	data, err := io.ReadAll(file)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "upload too large (max 10MB)", http.StatusRequestEntityTooLarge)
			return
		}
		h.renderUploadError(w, r, "Не удалось прочитать файл: "+err.Error(), "")
		return
	}

	req, err := h.service.CreateRequest(r.Context(), CreateRequestInput{
		EmployerID:   employerID,
		ReceivedDate: received,
		Notes:        strings.TrimSpace(r.FormValue("notes")),
		XLSXData:     data,
		XLSXFileName: header.Filename,
	})
	if err != nil {
		var fe FieldErrors
		if errors.As(err, &fe) {
			h.renderUploadError(w, r, fe.Error(), employerIDRaw)
			return
		}
		// Parser errors (empty file / missing sheet / missing columns)
		// fall through to a 400 with the raw message — the operator
		// needs to see exactly what went wrong.
		h.log.WithError(err).Warn("upload rejected")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/requests/%d", req.ID), http.StatusSeeOther)
}

// Detail renders GET /requests/{id} — request detail with rows table
// and per-row apply/skip forms.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	req, err := h.service.GetRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "request not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := h.service.ListRows(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	itemsByRow := make(map[int64][]storagedb.ListRequestTrainingItemsForRowRow, len(rows))
	for _, row := range rows {
		items, err := h.service.ListItemsForRow(r.Context(), row.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		itemsByRow[row.ID] = items
	}
	views.Detail(r, req, rows, itemsByRow).Render(r.Context(), w)
}

// ApplyRow handles POST /requests/{id}/rows/{rowID}/apply.
//
// Authorization: the row must belong to the request in the URL. Without
// this check, an authenticated operator could call
// /requests/999/rows/<real-row-in-another-request>/apply and mutate rows
// they can see but don't own. The frozen service signature
// (ctx, requestRowID) doesn't take requestID — so we enforce ownership
// here in the handler. On mismatch we return 404 (not 400) so existence
// of the row in another request is not leaked.
func (h *Handler) ApplyRow(w http.ResponseWriter, r *http.Request) {
	requestID, rowID, err := h.parseRequestRowIDs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.rowBelongsToRequest(r.Context(), requestID, rowID) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.service.ApplyRow(r.Context(), rowID); err != nil {
		h.log.WithFields(logrus.Fields{
			"request_id": requestID,
			"row_id":     rowID,
		}).WithError(err).Warn("apply row")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/requests/%d", requestID), http.StatusSeeOther)
}

// SkipRow handles POST /requests/{id}/rows/{rowID}/skip. See ApplyRow
// for the ownership-check rationale — SkipRow has the same shape.
func (h *Handler) SkipRow(w http.ResponseWriter, r *http.Request) {
	requestID, rowID, err := h.parseRequestRowIDs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.rowBelongsToRequest(r.Context(), requestID, rowID) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.service.SkipRow(r.Context(), rowID); err != nil {
		h.log.WithFields(logrus.Fields{
			"request_id": requestID,
			"row_id":     rowID,
		}).WithError(err).Warn("skip row")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/requests/%d", requestID), http.StatusSeeOther)
}

// rowBelongsToRequest loads rowID and asserts its client_request_id
// equals requestID. A missing row and a row owned by a different
// request both return false — the caller maps both to 404 so existence
// is not leaked to enumeration.
func (h *Handler) rowBelongsToRequest(ctx context.Context, requestID, rowID int64) bool {
	row, err := h.queries.GetRequestRow(ctx, rowID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			h.log.WithError(err).Error("lookup request row for authz")
		}
		return false
	}
	return row.ClientRequestID == requestID
}

// sha256Hex is a small logging helper that returns the hex sha256 of
// the input bytes. Used to fingerprint uploaded Content-Type and
// filename values without exposing attacker-controlled strings to the
// log stream.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ----- helpers -----

var errNotFound = errors.New("not found")

func (h *Handler) parseRequestRowIDs(r *http.Request) (int64, int64, error) {
	requestID, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid request id: %w", err)
	}
	rowID, err := parseInt64Param(chi.URLParam(r, "rowID"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid row id: %w", err)
	}
	return requestID, rowID, nil
}

func parseInt64Param(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

func (h *Handler) renderUploadError(w http.ResponseWriter, r *http.Request, message string, employerID string) {
	employers, err := h.queries.ListEmployers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	errMap := map[string]string{"form": message}
	_ = employerID // echoed back via the template's employer_id pre-fill below
	views.NewRequest(r, employers, errMap, employerID).Render(r.Context(), w)
}
