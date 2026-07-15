package legacy

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

type fixtureSheet struct {
	Name      string
	HeaderRow int
	Headers   []string
	Rows      [][]any
}

func syntheticLegacySheets() []fixtureSheet {
	sheets := make([]fixtureSheet, 0, 5)
	for _, profile := range []SheetProfile{SheetA, SheetB, SheetV, SheetP, SheetS} {
		headers := append([]string(nil), syntheticCommonHeaders...)
		row := []any{
			"Test Organization", "7700000000", "Testov Test Testovich",
			"Tester", "Test Department", "12345678900", "TEST-1",
			"2026-07-16", "passed", "01.07.2026-16.07.2026",
			"registry-test-1", "branch-test-1", "worker@example.test",
			"test-user", "synthetic-password",
		}
		if profile == SheetV {
			headers = append(headers, "Программа обучения")
			row = append(row, "Test Program")
		}
		sheets = append(sheets, fixtureSheet{
			Name:      string(profile),
			HeaderRow: 1,
			Headers:   headers,
			Rows:      [][]any{row},
		})
	}
	return sheets
}

func writeWorkbook(t *testing.T, sheets []fixtureSheet) string {
	t.Helper()

	workbook := excelize.NewFile()
	t.Cleanup(func() { _ = workbook.Close() })
	if len(sheets) == 0 {
		path := filepath.Join(t.TempDir(), "fixture.xlsx")
		if err := workbook.SaveAs(path); err != nil {
			t.Fatalf("save empty workbook: %v", err)
		}
		return path
	}

	if err := workbook.SetSheetName("Sheet1", sheets[0].Name); err != nil {
		t.Fatalf("rename first sheet: %v", err)
	}
	for index, sheet := range sheets {
		if index > 0 {
			if _, err := workbook.NewSheet(sheet.Name); err != nil {
				t.Fatalf("create sheet %q: %v", sheet.Name, err)
			}
		}
		headerRow := sheet.HeaderRow
		if headerRow == 0 {
			headerRow = 1
		}
		for column, header := range sheet.Headers {
			cell, err := excelize.CoordinatesToCellName(column+1, headerRow)
			if err != nil {
				t.Fatalf("header cell: %v", err)
			}
			if err := workbook.SetCellValue(sheet.Name, cell, header); err != nil {
				t.Fatalf("set header: %v", err)
			}
		}
		for rowIndex, row := range sheet.Rows {
			for column, value := range row {
				cell, err := excelize.CoordinatesToCellName(column+1, headerRow+rowIndex+1)
				if err != nil {
					t.Fatalf("data cell: %v", err)
				}
				if err := workbook.SetCellValue(sheet.Name, cell, value); err != nil {
					t.Fatalf("set data: %v", err)
				}
			}
		}
	}

	path := filepath.Join(t.TempDir(), "fixture.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatalf("save workbook: %v", err)
	}
	return path
}

func writeZIP(t *testing.T, entries map[string][]byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
	return path
}
