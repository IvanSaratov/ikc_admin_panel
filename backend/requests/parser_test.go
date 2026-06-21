package requests

import (
	"bytes"
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildXLSX creates a simple single-sheet XLSX file in memory with the
// given header row and data rows. The sheet is named "Заявка" so the
// parser picks it up via pickSheet(). Used by the parser tests to
// avoid committing binary fixtures to the repo.
func buildXLSX(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()

	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Заявка"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}

	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatalf("header cell: %v", err)
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			t.Fatalf("set header: %v", err)
		}
	}
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				t.Fatalf("cell: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				t.Fatalf("set value: %v", err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

func TestParseXLSX_ValidGoldenFixture(t *testing.T) {
	t.Parallel()

	data := buildXLSX(t,
		[]string{"ФИО", "СНИЛС", "Email", "Должность", "Коды программ"},
		[][]string{
			{"Иванов Иван Иванович", "123-456-789 00", "ivanov@example.com", "Инженер", "A-1, A-2"},
			{"Петров Петр", "98765432100", "petrov@example.com", "Менеджер", "B-3"},
		},
	)

	rows, err := ParseXLSX(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	first := rows[0]
	if first.RowNumber != 2 {
		t.Errorf("row number = %d, want 2 (first data row)", first.RowNumber)
	}
	if first.RawFullName != "Иванов Иван Иванович" {
		t.Errorf("full name = %q", first.RawFullName)
	}
	if first.RawSNILS != "123-456-789 00" {
		t.Errorf("snils = %q", first.RawSNILS)
	}
	if first.RawEmail != "ivanov@example.com" {
		t.Errorf("email = %q", first.RawEmail)
	}
	if first.RawPosition != "Инженер" {
		t.Errorf("position = %q", first.RawPosition)
	}
	if len(first.RawPrograms) != 2 || first.RawPrograms[0] != "A-1" || first.RawPrograms[1] != "A-2" {
		t.Errorf("programs = %v, want [A-1 A-2]", first.RawPrograms)
	}

	second := rows[1]
	if second.RowNumber != 3 {
		t.Errorf("row number = %d, want 3", second.RowNumber)
	}
	if len(second.RawPrograms) != 1 || second.RawPrograms[0] != "B-3" {
		t.Errorf("programs = %v, want [B-3]", second.RawPrograms)
	}
}

func TestParseXLSX_EmptyFileErrors(t *testing.T) {
	t.Parallel()

	// A workbook with the required header but no data rows.
	data := buildXLSX(t,
		[]string{"ФИО", "СНИЛС", "Email", "Коды программ"},
		nil,
	)

	_, err := ParseXLSX(data)
	if !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("err = %v, want ErrEmptyFile", err)
	}
}

func TestParseXLSX_MissingRequiredColumnsErrors(t *testing.T) {
	t.Parallel()

	// Drop the Email column -> header is rejected.
	data := buildXLSX(t,
		[]string{"ФИО", "СНИЛС", "Должность", "Коды программ"},
		[][]string{
			{"Иванов Иван", "12345678900", "Инженер", "A-1"},
		},
	)

	_, err := ParseXLSX(data)
	if !errors.Is(err, ErrMissingColumns) {
		t.Fatalf("err = %v, want ErrMissingColumns", err)
	}
}

func TestParseXLSX_TrailingEmptyRowsSkipped(t *testing.T) {
	t.Parallel()

	data := buildXLSX(t,
		[]string{"ФИО", "СНИЛС", "Email", "Коды программ"},
		[][]string{
			{"Иванов Иван", "12345678900", "ivanov@example.com", "A-1"},
			{"", "", "", ""},
			{"", "", "", ""},
		},
	)

	rows, err := ParseXLSX(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (empty rows should be skipped)", len(rows))
	}
}

func TestParseXLSX_PicksFirstSheetAsFallback(t *testing.T) {
	t.Parallel()

	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Лист1"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	headers := []string{"ФИО", "СНИЛС", "Email", "Коды программ"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	row := []string{"Иванов Иван", "12345678900", "ivanov@example.com", "A-1"}
	for c, v := range row {
		cell, _ := excelize.CoordinatesToCellName(c+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	rows, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
}
