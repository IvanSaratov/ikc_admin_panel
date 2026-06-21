package requests

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ParsedRow is one row from the XLSX, with no normalization applied.
// All fields are taken verbatim from the spreadsheet. The downstream
// Normalize step turns these into DB-ready values.
type ParsedRow struct {
	RowNumber   int64    // 1-indexed spreadsheet row, matches request_rows.row_number
	RawFullName string   // column "ФИО"
	RawSNILS    string   // column "СНИЛС"
	RawEmail    string   // column "Email"
	RawPosition string   // column "Должность"
	RawPrograms []string // column "Коды программ" — comma/semicolon-separated
}

// ErrEmptyFile / ErrMissingSheet / ErrMissingColumns are the three
// terminal errors the parser can return. Wrapping them lets callers
// distinguish "operator uploaded garbage" from "infrastructure broke".
var (
	ErrEmptyFile      = errors.New("xlsx file contains no rows")
	ErrMissingSheet   = errors.New("xlsx file is missing the 'Заявка' sheet")
	ErrMissingColumns = errors.New("xlsx header row is missing required columns")
)

// Header layout: column index (0-based) -> field name. Headers in the
// spreadsheet are matched case-insensitively, with a small set of
// accepted aliases for each required column. Adding a new alias is a
// no-op for existing files and the cost of being friendly to variations.
type headerIndex struct {
	fullName int
	snils    int
	email    int
	position int
	programs int
}

// parserColumns maps header aliases (lower-cased) to the field they
// represent. Anything not present here is ignored.
var parserColumns = map[string]fieldKind{
	"фио":           fieldFullName,
	"снилс":         fieldSNILS,
	"email":         fieldEmail,
	"e-mail":        fieldEmail,
	"должность":     fieldPosition,
	"коды программ": fieldPrograms,
	"программы":     fieldPrograms,
	"program codes": fieldPrograms,
	"programs":      fieldPrograms,
}

type fieldKind int

const (
	fieldUnknown fieldKind = iota
	fieldFullName
	fieldSNILS
	fieldEmail
	fieldPosition
	fieldPrograms
)

// ParseXLSX reads an XLSX file (as raw bytes) and returns the parsed
// rows in the same order they appear in the spreadsheet. It stops at
// the first fully-empty row so trailing blanks don't pollute the
// staging table.
//
// The function is intentionally synchronous and small — Mintrud's
// request XLSX files are <500 rows, so we don't need streaming.
func ParseXLSX(data []byte) ([]ParsedRow, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheet := pickSheet(f)
	if sheet == "" {
		return nil, ErrMissingSheet
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, ErrEmptyFile
	}

	header := buildHeaderIndex(rows[0])
	if header == nil {
		return nil, ErrMissingColumns
	}

	out := make([]ParsedRow, 0, len(rows)-1)
	for i, r := range rows[1:] {
		// Trim trailing blanks so "row contains "" everywhere" doesn't
		// produce thousands of phantom empty rows.
		if isRowEmpty(r) {
			continue
		}
		pr := ParsedRow{
			RowNumber:   int64(i + 2), // +1 for 1-indexed, +1 for header
			RawFullName: cellAt(r, header.fullName),
			RawSNILS:    cellAt(r, header.snils),
			RawEmail:    cellAt(r, header.email),
			RawPosition: cellAt(r, header.position),
			RawPrograms: splitPrograms(cellAt(r, header.programs)),
		}
		out = append(out, pr)
	}
	if len(out) == 0 {
		return nil, ErrEmptyFile
	}
	return out, nil
}

// pickSheet returns the first sheet name that looks like a request
// ("Заявка") or, failing that, the first sheet in the file. Returning
// the first sheet is a forgiving fallback for files where operators
// renamed the sheet.
func pickSheet(f *excelize.File) string {
	for _, name := range f.GetSheetList() {
		if strings.EqualFold(name, "Заявка") {
			return name
		}
	}
	list := f.GetSheetList()
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// buildHeaderIndex scans the first row for known column aliases. All
// required columns must be present; otherwise the header is rejected
// so the operator gets a clear error rather than mysterious empty rows.
func buildHeaderIndex(headerRow []string) *headerIndex {
	idx := &headerIndex{
		fullName: -1, snils: -1, email: -1, position: -1, programs: -1,
	}
	for i, raw := range headerRow {
		key := strings.ToLower(strings.TrimSpace(raw))
		kind, ok := parserColumns[key]
		if !ok {
			continue
		}
		switch kind {
		case fieldFullName:
			idx.fullName = i
		case fieldSNILS:
			idx.snils = i
		case fieldEmail:
			idx.email = i
		case fieldPosition:
			idx.position = i
		case fieldPrograms:
			idx.programs = i
		}
	}
	// Required: ФИО + СНИЛС + Email + Программы. Должность is optional.
	if idx.fullName == -1 || idx.snils == -1 || idx.email == -1 || idx.programs == -1 {
		return nil
	}
	return idx
}

func cellAt(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

func isRowEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// splitPrograms accepts "A-1, A-2", "A-1;A-2", or "A-1 A-2" and returns
// the codes, trimmed and uppercased. Empty codes are filtered out so
// trailing commas don't produce phantom empty program codes.
func splitPrograms(raw string) []string {
	if raw == "" {
		return nil
	}
	// Replace common separators with a single one.
	normalized := strings.NewReplacer(",", " ", ";", " ", "\n", " ", "\t", " ").Replace(raw)
	parts := strings.Fields(normalized)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		cleaned := NormalizeProgramCode(p)
		if cleaned == "" {
			continue
		}
		out = append(out, cleaned)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
