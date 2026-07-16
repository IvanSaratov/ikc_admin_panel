package api

import (
	"encoding/json"
	"net/http"
)

// Problem contains only stable, client-safe problem metadata.
type Problem struct {
	Status           int
	Code             string
	Detail           string
	ExistingImportID int64
}

type problemDocument struct {
	Status           int    `json:"status"`
	Code             string `json:"code"`
	Detail           string `json:"detail"`
	TraceID          string `json:"trace_id"`
	ExistingImportID int64  `json:"existing_import_id,omitempty"`
}

// WriteProblem writes an RFC 9457 media type with the project's stable fields.
func WriteProblem(w http.ResponseWriter, r *http.Request, problem Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	traceID := ""
	if r != nil {
		traceID = TraceID(r.Context())
	}
	_ = json.NewEncoder(w).Encode(problemDocument{
		Status:           problem.Status,
		Code:             problem.Code,
		Detail:           problem.Detail,
		TraceID:          traceID,
		ExistingImportID: problem.ExistingImportID,
	})
}

// WriteJSON writes a JSON response using the shared content type.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
