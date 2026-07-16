package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
)

func TestWriteProblemSerializesOnlySafeFields(t *testing.T) {
	t.Parallel()

	handler := api.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.WriteProblem(w, r, api.Problem{
			Status:           http.StatusConflict,
			Code:             "duplicate_file",
			Detail:           "Файл уже загружен",
			ExistingImportID: 42,
		})
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/imports/legacy", nil))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body["status"] != float64(http.StatusConflict) || body["code"] != "duplicate_file" || body["detail"] != "Файл уже загружен" {
		t.Fatalf("problem body = %#v", body)
	}
	if body["existing_import_id"] != float64(42) {
		t.Fatalf("existing_import_id = %#v", body["existing_import_id"])
	}
	trace, ok := body["trace_id"].(string)
	if !ok || trace == "" || trace != recorder.Header().Get("X-Trace-ID") {
		t.Fatalf("trace_id = %#v", body["trace_id"])
	}
	for _, forbidden := range []string{"error", "stack", "path"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("problem exposes forbidden field %q", forbidden)
		}
	}
}

func TestWriteProblemOmitsZeroExistingImportID(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/imports/404", nil)
	api.WriteProblem(recorder, request, api.Problem{
		Status: http.StatusNotFound,
		Code:   "import_not_found",
		Detail: "Импорт не найден",
	})
	if strings.Contains(recorder.Body.String(), "existing_import_id") {
		t.Fatalf("zero existing import ID was serialized: %s", recorder.Body.String())
	}
}

func TestWriteJSONUsesJSONContentType(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	api.WriteJSON(recorder, http.StatusAccepted, map[string]int{"id": 7})
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"id":7}` {
		t.Fatalf("body = %s", got)
	}
}
