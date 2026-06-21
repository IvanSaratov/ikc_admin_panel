package audit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// HXRequestHeader is the header HTMX sends on every AJAX request. When
// it is "true" the handler returns only the table fragment instead of
// the full shell so HTMX's outerHTML swap can drop the result into the
// #audit-table-wrap target without nesting a second shell layout.
const HXRequestHeader = "HX-Request"

// PageSize is the number of action_log rows rendered per page. The D4
// plan fixes this at 50; tests assert pagination on the same constant
// so the value lives in one place.
const PageSize = 50

// Handler exposes the read-only audit UI.
//
// The audit slice is intentionally one-way: this handler never writes
// to action_log — that responsibility belongs to the audit.Service.Record
// path used by every mutating handler (programs, employers, people,
// protocols, etc.). Adding a write method here would create a second
// code path that bypasses Service.Record's actor-resolution logic.
type Handler struct {
	queries *storagedb.Queries
}

// NewHandler wires the audit UI to the same *storagedb.Queries that
// audit.Service.Record uses for writes. The handler needs no Service
// reference because it never records; it only reads.
func NewHandler(queries *storagedb.Queries) *Handler {
	return &Handler{queries: queries}
}

// List renders the audit log table for the current filters and page.
//
// Filter parameters (all optional, exact match except for created_at):
//   - actor        (string)   exact match on action_log.actor
//   - action       (string)   exact match on action_log.action
//   - entity_type  (string)   exact match on action_log.entity_type
//   - created_from (RFC3339)  inclusive lower bound on created_at
//   - created_to   (RFC3339)  inclusive upper bound on created_at
//   - page         (int > 0)  1-based page number, default 1
//
// Filter values are passed straight through to sqlc-generated queries;
// the SQL uses parameter binding (? placeholders) so user input never
// reaches the query string.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	actor := q.Get("actor")
	action := q.Get("action")
	entityType := q.Get("entity_type")
	createdFrom := q.Get("created_from")
	createdTo := q.Get("created_to")

	page := parsePage(q.Get("page"))
	offset := (page - 1) * PageSize

	rows, err := h.queries.ListActionLogsFiltered(ctx, storagedb.ListActionLogsFilteredParams{
		Actor:       nullIfEmpty(actor),
		Action:      nullIfEmpty(action),
		EntityType:  nullIfEmpty(entityType),
		CreatedFrom: nullIfEmpty(createdFrom),
		CreatedTo:   nullIfEmpty(createdTo),
		Lim:         PageSize,
		Off:         offset,
	})
	if err != nil {
		http.Error(w, "list action_log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	total, err := h.queries.CountActionLogsFiltered(ctx, storagedb.CountActionLogsFilteredParams{
		Actor:       nullIfEmpty(actor),
		Action:      nullIfEmpty(action),
		EntityType:  nullIfEmpty(entityType),
		CreatedFrom: nullIfEmpty(createdFrom),
		CreatedTo:   nullIfEmpty(createdTo),
	})
	if err != nil {
		http.Error(w, "count action_log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Resolve the login here rather than in the templ file to avoid an
	// import cycle: admin/handler.go already imports audit, so the
	// audit/views package must not import admin. The login is published
	// on the request context by router.go's withActor middleware (which
	// runs immediately after admin.RequireAuth in the protected group)
	// via audit.WithActor; reading it through audit.ActorFromContext
	// avoids any admin dependency from this handler.
	login := ActorFromContext(ctx)

	pageData := views.Page{
		Rows:       rows,
		Total:      total,
		Page:       page,
		PageSize:   PageSize,
		Actor:      actor,
		Action:     action,
		EntityType: entityType,
		From:       createdFrom,
		To:         createdTo,
	}

	// HTMX requests get only the inner fragment so the swap does not
	// embed a second shell layout inside the page. The non-HTMX path
	// renders the full page (filter form + table + shell).
	if r.Header.Get(HXRequestHeader) == "true" {
		// For HTMX outerHTML swap into #audit-table-wrap the response
		// must itself BE the wrapper div so the new content lands
		// inside the same id (replacing the old one).
		writeTableWrap(w, views.TableFragment(r, pageData, login))
		return
	}
	views.List(r, pageData, login).Render(ctx, w)
}

// writeTableWrap emits a minimal #audit-table-wrap wrapper around the
// given fragment. Used only on the HTMX response path so the swap
// target stays consistent with the initial render.
func writeTableWrap(w http.ResponseWriter, inner interface {
	Render(context.Context, io.Writer) error
}) {
	// Render inner to a buffer first, then write the wrapper + inner.
	var buf bytes.Buffer
	if err := inner.Render(context.Background(), &buf); err != nil {
		http.Error(w, "render fragment: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<div id="audit-table-wrap">`)
	_, _ = w.Write(buf.Bytes())
	_, _ = io.WriteString(w, `</div>`)
}

// parsePage coerces the ?page= query param to a positive 1-based page
// number. Invalid / missing values collapse to 1 so a malicious or
// stale link can never produce a negative offset.
func parsePage(raw string) int64 {
	if raw == "" {
		return 1
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// nullIfEmpty normalises an empty filter string into the SQL sentinel
// "" — the WHERE clause in ListActionLogsFiltered / CountActionLogsFiltered
// uses `length(filter) = 0 OR col <op> filter` so an empty string drops
// the constraint. Returning the original string when non-empty
// preserves exact-match semantics for `actor = ?` / `entity_type = ?`.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return ""
	}
	return s
}
