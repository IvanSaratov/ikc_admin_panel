package legacy

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestPreflightAcceptsFiveSheetLegacyWorkbook(t *testing.T) {
	t.Parallel()

	plan, err := Preflight(context.Background(), writeWorkbook(t, syntheticLegacySheets()), DefaultLimits())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if len(plan.Sheets) != 5 {
		t.Fatalf("sheet count = %d, want 5", len(plan.Sheets))
	}
	for index, want := range []SheetProfile{SheetA, SheetB, SheetV, SheetP, SheetS} {
		if plan.Sheets[index].Profile != want || plan.Sheets[index].Order != index+1 {
			t.Errorf("sheet %d = %+v, want profile %q order %d", index, plan.Sheets[index], want, index+1)
		}
	}
}

func TestPreflightFindsHeaderWithinFirstTwentyRows(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].HeaderRow = 3
	plan, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if plan.Sheets[0].HeaderRow != 3 {
		t.Fatalf("header row = %d, want 3", plan.Sheets[0].HeaderRow)
	}
}

func TestPreflightRejectsNonZIPAsCodeNotXLSX(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/not-xlsx"
	if err := os.WriteFile(path, []byte("not an xlsx"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Preflight(context.Background(), path, DefaultLimits())
	assertParseErrorCode(t, err, CodeNotXLSX)
}

func TestPreflightRejectsZIPWithoutWorkbookParts(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{"safe.txt": []byte("safe")})
	_, err := Preflight(context.Background(), path, DefaultLimits())
	assertParseErrorCode(t, err, CodeNotXLSX)
}

func TestPreflightRejectsEncryptedZIPEntry(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{
		"[Content_Types].xml": []byte("types"),
		"xl/workbook.xml":     []byte("workbook"),
	})
	markFirstZIPEntryEncrypted(t, path)
	_, err := Preflight(context.Background(), path, DefaultLimits())
	assertParseErrorCode(t, err, CodeNotXLSX)
}

func TestPreflightRejectsUnsafeZIPPath(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{
		"[Content_Types].xml": []byte("types"),
		"xl/workbook.xml":     []byte("workbook"),
		"../escaped.xml":      []byte("unsafe"),
	})
	_, err := Preflight(context.Background(), path, DefaultLimits())
	assertParseErrorCode(t, err, CodeNotXLSX)
}

func TestPreflightRejectsFileSizeOverLimit(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	limits := DefaultLimits()
	limits.MaxFileBytes = 1
	_, err := Preflight(context.Background(), path, limits)
	assertParseErrorCode(t, err, CodeWorkbookTooLarge)
}

func TestPreflightRejectsUncompressedSizeOverLimit(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{
		"[Content_Types].xml": bytes.Repeat([]byte("x"), 128),
		"xl/workbook.xml":     bytes.Repeat([]byte("y"), 128),
	})
	limits := DefaultLimits()
	limits.MaxUncompressedBytes = 64
	_, err := Preflight(context.Background(), path, limits)
	assertParseErrorCode(t, err, CodeLimitExceeded)
}

func TestPreflightRejectsTooManyZIPEntries(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{
		"[Content_Types].xml": []byte("types"),
		"xl/workbook.xml":     []byte("workbook"),
	})
	limits := DefaultLimits()
	limits.MaxZIPEntries = 1
	_, err := Preflight(context.Background(), path, limits)
	assertParseErrorCode(t, err, CodeLimitExceeded)
}

func TestPreflightRejectsMacroEnabledWorkbook(t *testing.T) {
	t.Parallel()

	path := writeZIP(t, map[string][]byte{
		"[Content_Types].xml": []byte("types"),
		"xl/workbook.xml":     []byte("workbook"),
		"xl/vbaProject.bin":   []byte("macro"),
	})
	_, err := Preflight(context.Background(), path, DefaultLimits())
	assertParseErrorCode(t, err, CodeNotXLSX)
}

func TestPreflightRejectsMissingRequiredSheet(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	path := writeWorkbook(t, sheets[:len(sheets)-1])
	_, err := Preflight(context.Background(), path, DefaultLimits())
	parseErr := assertParseErrorCode(t, err, CodeMissingSheet)
	if parseErr.Sheet != string(SheetS) {
		t.Fatalf("missing sheet = %q, want %q", parseErr.Sheet, SheetS)
	}
}

func TestPreflightRejectsUnknownNonEmptySheet(t *testing.T) {
	t.Parallel()

	sheets := append(syntheticLegacySheets(), fixtureSheet{
		Name:      "Unknown",
		HeaderRow: 1,
		Headers:   []string{"Header"},
		Rows:      [][]any{{"value"}},
	})
	_, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	assertParseErrorCode(t, err, CodeUnknownSheet)
}

func TestPreflightAllowsUnknownEmptySheet(t *testing.T) {
	t.Parallel()

	sheets := append(syntheticLegacySheets(), fixtureSheet{Name: "Notes"})
	plan, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if len(plan.Sheets) != 5 {
		t.Fatalf("supported sheet count = %d, want 5", len(plan.Sheets))
	}
}

func TestPreflightRejectsDuplicateNormalizedSheetName(t *testing.T) {
	t.Parallel()

	sheets := append(syntheticLegacySheets(), fixtureSheet{
		Name:      " а ",
		HeaderRow: 1,
		Headers:   syntheticCommonHeaders,
		Rows:      [][]any{{"duplicate"}},
	})
	_, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	assertParseErrorCode(t, err, CodeUnsupportedWorkbook)
}

func TestPreflightRejectsMissingColumnsWithoutEchoingHeader(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].Headers[5] = "SENSITIVE-HEADER-SENTINEL"
	_, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	parseErr := assertParseErrorCode(t, err, CodeMissingColumns)
	if bytes.Contains([]byte(parseErr.Error()), []byte("SENSITIVE-HEADER-SENTINEL")) {
		t.Fatal("error echoed workbook header")
	}
}

func TestPreflightRejectsNonEmptyColumnWithoutHeader(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].Headers = append(sheets[0].Headers, "")
	sheets[0].Rows[0] = append(sheets[0].Rows[0], "UNMAPPED-CELL-SENTINEL")
	_, err := Preflight(context.Background(), writeWorkbook(t, sheets), DefaultLimits())
	parseErr := assertParseErrorCode(t, err, CodeUnsupportedWorkbook)
	if strings.Contains(parseErr.Error(), "UNMAPPED-CELL-SENTINEL") {
		t.Fatal("error echoed an unmapped cell value")
	}
}

func TestPreflightHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Preflight(ctx, writeWorkbook(t, syntheticLegacySheets()), DefaultLimits())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func markFirstZIPEntryEncrypted(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, signature := range []struct {
		value  []byte
		offset int
	}{
		{[]byte{'P', 'K', 3, 4}, 6},
		{[]byte{'P', 'K', 1, 2}, 8},
	} {
		index := bytes.Index(data, signature.value)
		if index < 0 {
			t.Fatalf("ZIP signature %v not found", signature.value)
		}
		flags := binary.LittleEndian.Uint16(data[index+signature.offset:])
		binary.LittleEndian.PutUint16(data[index+signature.offset:], flags|1)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
