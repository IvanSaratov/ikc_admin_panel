package api_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
)

func TestTraceMiddlewareCreatesPrivateUniqueTraceIDs(t *testing.T) {
	t.Parallel()

	seen := make([]string, 0, 2)
	handler := api.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, api.TraceID(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))
	pattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	responses := make([]string, 0, 2)
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/imports", nil)
		request.Header.Set("X-Trace-ID", "client-controlled-trace")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", recorder.Code)
		}
		responses = append(responses, recorder.Header().Get("X-Trace-ID"))
	}
	if len(seen) != 2 {
		t.Fatalf("handler trace count = %d, want 2", len(seen))
	}
	for index := range seen {
		if !pattern.MatchString(seen[index]) {
			t.Errorf("trace ID %q does not match lowercase hex contract", seen[index])
		}
		if responses[index] != seen[index] {
			t.Errorf("response trace = %q, context trace = %q", responses[index], seen[index])
		}
		if seen[index] == "client-controlled-trace" {
			t.Fatal("middleware trusted a client-controlled trace ID")
		}
	}
	if seen[0] == seen[1] {
		t.Fatalf("trace IDs are not unique: %q", seen[0])
	}
}
