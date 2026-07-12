package protocols

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowAPI_ReturnsJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteWorkflowJSON(w, WorkflowResponse{
			ProtocolID: 1,
			Number:     "2605А15",
			Employer:   "Тестовый работодатель",
			Stages: []WorkflowStageResponse{
				{ID: "xml", Label: "XML", State: "active"},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/protocols/1/workflow", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"number":"2605А15"`) {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}
