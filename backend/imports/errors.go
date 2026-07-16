// Package imports implements asynchronous import orchestration.
package imports

import (
	"fmt"
	"strings"
)

// ErrorCode is a stable domain code that the HTTP layer maps to a response.
type ErrorCode string

const (
	CodeInvalidInput        ErrorCode = "invalid_input"
	CodeFileTooLarge        ErrorCode = "file_too_large"
	CodeNotXLSX             ErrorCode = "not_xlsx"
	CodeUnsupportedWorkbook ErrorCode = "unsupported_workbook"
	CodeDuplicateFile       ErrorCode = "duplicate_file"
	CodeQueueFull           ErrorCode = "queue_full"
	CodeIdempotencyConflict ErrorCode = "idempotency_conflict"
	CodeStorageUnavailable  ErrorCode = "storage_unavailable"
	CodeInternal            ErrorCode = "internal_error"
)

// ServiceError exposes only safe fixed metadata. Err remains available for
// errors.Is/errors.As but is intentionally omitted from Error().
type ServiceError struct {
	Code             ErrorCode
	Detail           string
	ExistingImportID int64
	Err              error
}

func (e *ServiceError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{string(e.Code)}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if e.ExistingImportID > 0 {
		parts = append(parts, fmt.Sprintf("existing_import_id=%d", e.ExistingImportID))
	}
	return strings.Join(parts, ": ")
}

func (e *ServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
