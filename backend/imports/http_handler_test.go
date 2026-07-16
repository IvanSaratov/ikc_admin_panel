package imports_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type fakeLegacyEnqueuer struct {
	result imports.EnqueueResult
	err    error
	input  imports.EnqueueInput
	body   []byte
}

func (f *fakeLegacyEnqueuer) EnqueueLegacy(_ context.Context, input imports.EnqueueInput) (imports.EnqueueResult, error) {
	f.input = input
	body, err := io.ReadAll(input.Body)
	f.body = body
	if err != nil {
		return imports.EnqueueResult{}, err
	}
	return f.result, f.err
}

type fakeImportReader struct {
	page      imports.ImportPage
	view      imports.ImportView
	listErr   error
	getErr    error
	cursor    string
	limit     int
	requested int64
}

func (f *fakeImportReader) List(_ context.Context, cursor string, limit int) (imports.ImportPage, error) {
	f.cursor, f.limit = cursor, limit
	return f.page, f.listErr
}

func (f *fakeImportReader) Get(_ context.Context, id int64) (imports.ImportView, error) {
	f.requested = id
	return f.view, f.getErr
}

func newHTTPHandler(t *testing.T, enqueuer imports.LegacyEnqueuer, reader imports.ImportReader, limits legacy.Limits) *imports.HTTPHandler {
	t.Helper()
	handler, err := imports.NewHTTPHandler(enqueuer, reader, limits, zap.NewNop())
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler
}

func multipartRequest(t *testing.T, parts func(*multipart.Writer)) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	parts(writer)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/imports/legacy", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func addFilePart(t *testing.T, writer *multipart.Writer, field, name string, body []byte) {
	t.Helper()
	part, err := writer.CreateFormFile(field, name)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write file part: %v", err)
	}
}

func serveUpload(t *testing.T, handler *imports.HTTPHandler, request *http.Request, actor string) *httptest.ResponseRecorder {
	t.Helper()
	if actor != "" {
		request = request.WithContext(audit.WithActor(request.Context(), actor))
	}
	recorder := httptest.NewRecorder()
	api.TraceMiddleware(http.HandlerFunc(handler.UploadLegacy)).ServeHTTP(recorder, request)
	return recorder
}

func decodeProblem(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v; body=%s", err, recorder.Body.String())
	}
	return body
}

func TestHTTPUploadLegacyReturnsAcceptedResource(t *testing.T) {
	t.Parallel()

	enqueuer := &fakeLegacyEnqueuer{result: imports.EnqueueResult{
		Import: storagedb.Import{
			ID:     42,
			Status: "queued",
			Phase:  sql.NullString{},
		},
		QueuePosition: 2,
	}}
	handler := newHTTPHandler(t, enqueuer, &fakeImportReader{}, legacy.DefaultLimits())
	request := multipartRequest(t, func(writer *multipart.Writer) {
		addFilePart(t, writer, "file", "../industrial.xlsx", []byte("xlsx-body"))
	})
	request.Header.Set("Idempotency-Key", "ui-request-1")
	recorder := serveUpload(t, handler, request, "admin-user")

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Location") != "/api/imports/42" {
		t.Fatalf("Location = %q", recorder.Header().Get("Location"))
	}
	if enqueuer.input.OriginalFileName != "industrial.xlsx" || enqueuer.input.IdempotencyKey != "ui-request-1" || enqueuer.input.Actor != "admin-user" {
		t.Fatalf("enqueue input = %+v", enqueuer.input)
	}
	if string(enqueuer.body) != "xlsx-body" {
		t.Fatalf("streamed body = %q", enqueuer.body)
	}
	var response struct {
		ID            int64   `json:"id"`
		Status        string  `json:"status"`
		Phase         *string `json:"phase"`
		QueuePosition int64   `json:"queue_position"`
		Reused        bool    `json:"reused"`
		StatusURL     string  `json:"status_url"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != 42 || response.Status != "queued" || response.Phase != nil || response.QueuePosition != 2 || response.Reused || response.StatusURL != "/api/imports/42" {
		t.Fatalf("response = %+v", response)
	}
}

func TestHTTPUploadLegacyReturnsOKForTerminalIdempotentReuse(t *testing.T) {
	t.Parallel()

	enqueuer := &fakeLegacyEnqueuer{result: imports.EnqueueResult{
		Import: storagedb.Import{ID: 7, Status: "completed"},
		Reused: true,
	}}
	handler := newHTTPHandler(t, enqueuer, &fakeImportReader{}, legacy.DefaultLimits())
	request := multipartRequest(t, func(writer *multipart.Writer) {
		addFilePart(t, writer, "file", "legacy.xlsx", []byte("ignored-by-real-service"))
	})
	recorder := serveUpload(t, handler, request, "admin-user")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPUploadLegacyRejectsInvalidMultipart(t *testing.T) {
	t.Parallel()

	limits := legacy.DefaultLimits()
	tests := []struct {
		name    string
		request func(*testing.T) *http.Request
	}{
		{name: "wrong media type", request: func(t *testing.T) *http.Request {
			request := httptest.NewRequest(http.MethodPost, "/api/imports/legacy", strings.NewReader("body"))
			request.Header.Set("Content-Type", "application/octet-stream")
			return request
		}},
		{name: "no part", request: func(t *testing.T) *http.Request {
			return multipartRequest(t, func(*multipart.Writer) {})
		}},
		{name: "wrong part name", request: func(t *testing.T) *http.Request {
			return multipartRequest(t, func(writer *multipart.Writer) { addFilePart(t, writer, "upload", "legacy.xlsx", []byte("x")) })
		}},
		{name: "empty filename", request: func(t *testing.T) *http.Request {
			return multipartRequest(t, func(writer *multipart.Writer) {
				part, err := writer.CreateFormField("file")
				if err != nil {
					t.Fatal(err)
				}
				_, _ = part.Write([]byte("x"))
			})
		}},
		{name: "extra field", request: func(t *testing.T) *http.Request {
			return multipartRequest(t, func(writer *multipart.Writer) {
				addFilePart(t, writer, "file", "legacy.xlsx", []byte("x"))
				_ = writer.WriteField("note", "forbidden")
			})
		}},
		{name: "second file", request: func(t *testing.T) *http.Request {
			return multipartRequest(t, func(writer *multipart.Writer) {
				addFilePart(t, writer, "file", "one.xlsx", []byte("one"))
				addFilePart(t, writer, "file", "two.xlsx", []byte("two"))
			})
		}},
		{name: "malformed boundary", request: func(t *testing.T) *http.Request {
			request := httptest.NewRequest(http.MethodPost, "/api/imports/legacy", strings.NewReader("broken"))
			request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
			return request
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enqueuer := &fakeLegacyEnqueuer{}
			handler := newHTTPHandler(t, enqueuer, &fakeImportReader{}, limits)
			recorder := serveUpload(t, handler, test.request(t), "admin-user")
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
			if body := decodeProblem(t, recorder); body["code"] != "invalid_input" {
				t.Fatalf("problem = %#v", body)
			}
		})
	}
}

func TestHTTPUploadLegacyRejectsBodyOverHTTPBoundary(t *testing.T) {
	t.Parallel()

	limits := legacy.DefaultLimits()
	limits.MaxFileBytes = 8
	handler := newHTTPHandler(t, &fakeLegacyEnqueuer{}, &fakeImportReader{}, limits)
	request := multipartRequest(t, func(writer *multipart.Writer) {
		addFilePart(t, writer, "file", "legacy.xlsx", bytes.Repeat([]byte("x"), (1<<20)+9))
	})
	recorder := serveUpload(t, handler, request, "admin-user")
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}
	if body := decodeProblem(t, recorder); body["code"] != "file_too_large" {
		t.Fatalf("problem = %#v", body)
	}
}

func TestHTTPUploadLegacyMapsDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		existingID int64
	}{
		{name: "invalid", err: &imports.ServiceError{Code: imports.CodeInvalidInput, Err: errors.New("SENSITIVE-PATH")}, wantStatus: 400, wantCode: "invalid_input"},
		{name: "too large", err: &imports.ServiceError{Code: imports.CodeFileTooLarge}, wantStatus: 413, wantCode: "file_too_large"},
		{name: "not xlsx", err: &imports.ServiceError{Code: imports.CodeNotXLSX}, wantStatus: 415, wantCode: "not_xlsx"},
		{name: "unsupported", err: &imports.ServiceError{Code: imports.CodeUnsupportedWorkbook}, wantStatus: 422, wantCode: "unsupported_workbook"},
		{name: "duplicate", err: &imports.ServiceError{Code: imports.CodeDuplicateFile, ExistingImportID: 8}, wantStatus: 409, wantCode: "duplicate_file", existingID: 8},
		{name: "idempotency", err: &imports.ServiceError{Code: imports.CodeIdempotencyConflict, ExistingImportID: 9}, wantStatus: 409, wantCode: "idempotency_conflict", existingID: 9},
		{name: "queue full", err: &imports.ServiceError{Code: imports.CodeQueueFull}, wantStatus: 429, wantCode: "queue_full"},
		{name: "storage", err: &imports.ServiceError{Code: imports.CodeStorageUnavailable, Err: errors.New("SENSITIVE-DB-PATH")}, wantStatus: 503, wantCode: "storage_unavailable"},
		{name: "internal", err: &imports.ServiceError{Code: imports.CodeInternal}, wantStatus: 500, wantCode: "internal_error"},
		{name: "unknown", err: errors.New("SENSITIVE-UNKNOWN"), wantStatus: 500, wantCode: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enqueuer := &fakeLegacyEnqueuer{err: test.err}
			handler := newHTTPHandler(t, enqueuer, &fakeImportReader{}, legacy.DefaultLimits())
			request := multipartRequest(t, func(writer *multipart.Writer) {
				addFilePart(t, writer, "file", "legacy.xlsx", []byte("body"))
			})
			recorder := serveUpload(t, handler, request, "admin-user")
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			body := decodeProblem(t, recorder)
			if body["code"] != test.wantCode {
				t.Fatalf("problem = %#v", body)
			}
			if test.existingID > 0 && body["existing_import_id"] != float64(test.existingID) {
				t.Fatalf("existing_import_id = %#v", body["existing_import_id"])
			}
			for _, forbidden := range []string{"SENSITIVE-PATH", "SENSITIVE-DB-PATH", "SENSITIVE-UNKNOWN"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("problem exposed internal error: %s", recorder.Body.String())
				}
			}
			if test.wantStatus == http.StatusTooManyRequests && recorder.Header().Get("Retry-After") != "5" {
				t.Fatalf("Retry-After = %q", recorder.Header().Get("Retry-After"))
			}
		})
	}
}

func TestHTTPListAndGetUseReadService(t *testing.T) {
	t.Parallel()

	fileName := "safe.xlsx"
	view := imports.ImportView{ID: 12, Profile: "legacy_registry", SourceFileName: &fileName, Status: "queued", QueuePosition: 1}
	reader := &fakeImportReader{page: imports.ImportPage{Items: []imports.ImportView{view}, NextCursor: "next"}, view: view}
	handler := newHTTPHandler(t, &fakeLegacyEnqueuer{}, reader, legacy.DefaultLimits())

	listRequest := httptest.NewRequest(http.MethodGet, "/api/imports?cursor=cursor-value&limit=25", nil)
	listRecorder := httptest.NewRecorder()
	handler.List(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || reader.cursor != "cursor-value" || reader.limit != 25 {
		t.Fatalf("list status=%d cursor=%q limit=%d body=%s", listRecorder.Code, reader.cursor, reader.limit, listRecorder.Body.String())
	}
	if strings.Contains(listRecorder.Body.String(), "source_sha256") || !strings.Contains(listRecorder.Body.String(), "next_cursor") {
		t.Fatalf("unsafe or incomplete list body: %s", listRecorder.Body.String())
	}

	router := chi.NewRouter()
	router.Get("/api/imports/{id}", handler.Get)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/api/imports/12", nil))
	if getRecorder.Code != http.StatusOK || reader.requested != 12 {
		t.Fatalf("get status=%d requested=%d body=%s", getRecorder.Code, reader.requested, getRecorder.Body.String())
	}
}

func TestHTTPReadValidationAndNotFoundProblems(t *testing.T) {
	t.Parallel()

	reader := &fakeImportReader{getErr: imports.ErrImportNotFound}
	handler := newHTTPHandler(t, &fakeLegacyEnqueuer{}, reader, legacy.DefaultLimits())
	router := chi.NewRouter()
	router.Use(api.TraceMiddleware)
	router.Get("/api/imports/{id}", handler.Get)

	for _, path := range []string{"/api/imports/0", "/api/imports/not-a-number", "/api/imports/999"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		want := http.StatusBadRequest
		if path == "/api/imports/999" {
			want = http.StatusNotFound
		}
		if recorder.Code != want {
			t.Fatalf("%s status = %d, want %d; body=%s", path, recorder.Code, want, recorder.Body.String())
		}
	}

	listRecorder := httptest.NewRecorder()
	handler.List(listRecorder, httptest.NewRequest(http.MethodGet, "/api/imports?limit=bad", nil))
	if listRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, body=%s", listRecorder.Code, listRecorder.Body.String())
	}
}
