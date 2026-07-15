package legacy

import (
	"fmt"
	"strings"
)

// ErrorCode is safe for API mapping and does not contain workbook data.
type ErrorCode string

const (
	CodeNotXLSX             ErrorCode = "not_xlsx"
	CodeWorkbookTooLarge    ErrorCode = "workbook_too_large"
	CodeUnsupportedWorkbook ErrorCode = "unsupported_workbook"
	CodeMissingSheet        ErrorCode = "missing_sheet"
	CodeUnknownSheet        ErrorCode = "unknown_sheet"
	CodeMissingColumns      ErrorCode = "missing_columns"
	CodeLimitExceeded       ErrorCode = "limit_exceeded"
	CodeReadFailed          ErrorCode = "read_failed"
)

// ParseError reports only safe structural coordinates. Err is available for
// errors.Is/errors.As but is intentionally omitted from Error to avoid leaking
// cell contents through nested parser messages.
type ParseError struct {
	Code   ErrorCode
	Sheet  string
	Row    int64
	Detail string
	Err    error
}

func (e *ParseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{string(e.Code)}
	if e.Sheet != "" {
		parts = append(parts, fmt.Sprintf("sheet=%q", e.Sheet))
	}
	if e.Row > 0 {
		parts = append(parts, fmt.Sprintf("row=%d", e.Row))
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	return strings.Join(parts, ": ")
}

func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func coded(code ErrorCode, detail string) *ParseError {
	return &ParseError{Code: code, Detail: detail}
}

func codedWrap(code ErrorCode, detail string, err error) *ParseError {
	return &ParseError{Code: code, Detail: detail, Err: err}
}
