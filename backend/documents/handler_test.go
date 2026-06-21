package documents

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/go-chi/chi/v5"
)

// handlerEnv wraps the Handler + the underlying Service + DB + queries.
// Mirrors the pattern used in protocols/handler_test.go.
type handlerEnv struct {
	handler *Handler
	env     *testEnv
}

func newHandlerEnv(t *testing.T) *handlerEnv {
	t.Helper()
	e := newTestEnv(t)
	h := NewHandler(e.queries, audit.NewService(e.queries), e.svc)
	return &handlerEnv{handler: h, env: e}
}

// post issues a POST to the supplied handler.
func (e *handlerEnv) post(t *testing.T, h http.HandlerFunc, values url.Values, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, params...)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// get issues a GET.
func (e *handlerEnv) get(t *testing.T, h http.HandlerFunc, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withURLParams(req, params...)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func withURLParams(r *http.Request, pairs ...string) *http.Request {
	if len(pairs)%2 != 0 {
		panic("withURLParams: odd number of pairs")
	}
	rctx := chi.NewRouteContext()
	for i := 0; i < len(pairs); i += 2 {
		rctx.URLParams.Add(pairs[i], pairs[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestGenerate_POST_RedirectsToDownload verifies that POST
// /protocols/{id}/generate?type=xml against a fixed protocol returns a
// 303 redirect to the download URL with the run id in the query string.
func TestGenerate_POST_RedirectsToDownload(t *testing.T) {
	t.Parallel()

	e := newHandlerEnv(t)
	protocolID, _ := e.env.seedFixedProtocol(t)

	req := httptest.NewRequest(http.MethodPost, "/?type=xml", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, "id", strconv.FormatInt(protocolID, 10))
	rr := httptest.NewRecorder()
	e.handler.Generate(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rr.Code, truncate(rr.Body.String(), 200))
	}
	loc := rr.Header().Get("Location")
	wantPrefix := fmt.Sprintf("/protocols/%d/download?run=", protocolID)
	if !strings.HasPrefix(loc, wantPrefix) {
		t.Errorf("Location = %q, want prefix %q", loc, wantPrefix)
	}
}

// TestGenerate_POST_DocxRedirectsToDownload mirrors TestGenerate_POST_RedirectsToDownload
// for the docx path.
func TestGenerate_POST_DocxRedirectsToDownload(t *testing.T) {
	t.Parallel()

	e := newHandlerEnv(t)
	protocolID, _ := e.env.seedFixedProtocol(t)

	rr := e.post(t, e.handler.Generate, url.Values{}, "id", strconv.FormatInt(protocolID, 10))
	// we re-issue with ?type=docx via a custom request
	req := httptest.NewRequest(http.MethodPost, "/?type=docx", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, "id", strconv.FormatInt(protocolID, 10))
	rr2 := httptest.NewRecorder()
	e.handler.Generate(rr2, req)
	if rr2.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr2.Code)
	}
	loc := rr2.Header().Get("Location")
	wantPrefix := fmt.Sprintf("/protocols/%d/download?run=", protocolID)
	if !strings.HasPrefix(loc, wantPrefix) {
		t.Errorf("Location = %q, want prefix %q", loc, wantPrefix)
	}
	_ = rr
}

// TestGenerate_POST_BadTypeReturns400 verifies that POST with type other
// than xml/docx returns 400.
func TestGenerate_POST_BadTypeReturns400(t *testing.T) {
	t.Parallel()

	e := newHandlerEnv(t)
	protocolID, _ := e.env.seedFixedProtocol(t)

	req := httptest.NewRequest(http.MethodPost, "/?type=pdf", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, "id", strconv.FormatInt(protocolID, 10))
	rr := httptest.NewRecorder()
	e.handler.Generate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestGenerate_POST_DraftProtocolReturns400 verifies that POST against a
// draft protocol returns 400.
func TestGenerate_POST_DraftProtocolReturns400(t *testing.T) {
	t.Parallel()

	e := newHandlerEnv(t)
	ctx := context.Background()
	now := "2026-06-22T00:00:00Z"

	res, err := e.env.db.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?)`,
		"HD-G-"+t.Name(), "HD Group "+t.Name(), now, now)
	if err != nil {
		t.Fatalf("insert program_group: %v", err)
	}
	groupID, _ := res.LastInsertId()

	pRes, err := e.env.db.ExecContext(ctx, `
		INSERT INTO protocols (program_group_id, status, created_at, updated_at)
		VALUES (?, 'draft', ?, ?)`,
		groupID, now, now)
	if err != nil {
		t.Fatalf("insert protocol: %v", err)
	}
	protocolID, _ := pRes.LastInsertId()

	req := httptest.NewRequest(http.MethodPost, "/?type=xml", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withURLParams(req, "id", strconv.FormatInt(protocolID, 10))
	rr := httptest.NewRecorder()
	e.handler.Generate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestDownload_GET_ReturnsFile verifies that GET /protocols/{id}/download
// returns the file with the correct Content-Type.
func TestDownload_GET_ReturnsFile(t *testing.T) {
	t.Parallel()

	e := newHandlerEnv(t)
	protocolID, _ := e.env.seedFixedProtocol(t)
	ctx := context.Background()

	// Generate a run first so we have a run id.
	_, run, err := e.env.svc.generateXMLWith(ctx, e.env.queries, protocolID)
	if err != nil {
		t.Fatalf("generateXMLWith: %v", err)
	}
	if run == nil {
		t.Fatalf("nil run")
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/protocols/%d/download?run=%d", protocolID, run.ID), nil)
	req = withURLParams(req, "id", strconv.FormatInt(protocolID, 10))
	rr := httptest.NewRecorder()
	e.handler.Download(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, truncate(rr.Body.String(), 200))
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/xml") {
		t.Errorf("Content-Type = %q, want application/xml", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("Content-Disposition = %q, want attachment;", cd)
	}
	if len(rr.Body.Bytes()) == 0 {
		t.Errorf("empty body")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
