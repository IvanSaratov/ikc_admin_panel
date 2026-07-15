package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestParseStreamsRowsInWorkbookOrder(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	var rows []SourceRow
	stats, err := Parse(context.Background(), path, plan, DefaultLimits(), func(_ context.Context, row SourceRow) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stats.Rows != 5 || len(rows) != 5 {
		t.Fatalf("rows = %d/%d, want 5", stats.Rows, len(rows))
	}
	for index, want := range []SheetProfile{SheetA, SheetB, SheetV, SheetP, SheetS} {
		if rows[index].SheetProfile != want || rows[index].RowNumber != 2 {
			t.Errorf("row %d = profile %q row %d; want %q row 2", index, rows[index].SheetProfile, rows[index].RowNumber, want)
		}
	}
}

func TestParseSkipsEmptyRowsWithoutStoppingSheet(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	first := append([]any(nil), sheets[0].Rows[0]...)
	second := append([]any(nil), first...)
	second[2] = "Second Synthetic Worker"
	sheets[0].Rows = [][]any{first, {}, second}
	path := writeWorkbook(t, sheets)
	plan := mustPreflight(t, path)
	var sheetARows []SourceRow
	_, err := Parse(context.Background(), path, plan, DefaultLimits(), func(_ context.Context, row SourceRow) error {
		if row.SheetProfile == SheetA {
			sheetARows = append(sheetARows, row)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sheetARows) != 2 || sheetARows[0].RowNumber != 2 || sheetARows[1].RowNumber != 4 {
		t.Fatalf("sheet A rows = %+v, want spreadsheet rows 2 and 4", sheetARows)
	}
}

func TestParsePreservesDisplayedINNAndSNILS(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	mutateWorkbook(t, path, func(workbook *excelize.File) {
		innFormat := "0000000000"
		snilsFormat := "00000000000"
		innStyle, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: &innFormat})
		if err != nil {
			t.Fatal(err)
		}
		snilsStyle, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: &snilsFormat})
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellValue("А", "B2", 123456789); err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellStyle("А", "B2", "B2", innStyle); err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellValue("А", "F2", 1234567890); err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellStyle("А", "F2", "F2", snilsStyle); err != nil {
			t.Fatal(err)
		}
	})

	row := firstParsedRow(t, path)
	if row.INN != "0123456789" || row.SNILS != "01234567890" {
		t.Fatalf("displayed identifiers = INN %q, SNILS %q", row.INN, row.SNILS)
	}
}

func TestParsePreservesDisplayedExcelDate(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	mutateWorkbook(t, path, func(workbook *excelize.File) {
		dateFormat := "dd.mm.yyyy"
		style, err := workbook.NewStyle(&excelize.Style{CustomNumFmt: &dateFormat})
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellValue("А", "H2", time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetCellStyle("А", "H2", "H2", style); err != nil {
			t.Fatal(err)
		}
	})

	if got := firstParsedRow(t, path).ProtocolDate; got != "16.07.2026" {
		t.Fatalf("displayed protocol date = %q, want 16.07.2026", got)
	}
}

func TestParsePreservesUnknownNonSecretColumns(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].Headers = append(sheets[0].Headers, "Внутренний комментарий")
	sheets[0].Rows[0] = append(sheets[0].Rows[0], "Synthetic note")
	row := firstParsedRow(t, writeWorkbook(t, sheets))
	if row.ExtraFields["внутренний комментарий"] != "Synthetic note" {
		t.Fatalf("extra fields = %#v", row.ExtraFields)
	}
}

func TestParseDropsPasswordAndSecretLikeColumns(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].Rows[0][14] = "DO-NOT-PERSIST-SECRET"
	sheets[0].Headers = append(sheets[0].Headers, "Access token")
	sheets[0].Rows[0] = append(sheets[0].Rows[0], "SECOND-SECRET-SENTINEL")
	row := firstParsedRow(t, writeWorkbook(t, sheets))
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{
		"do-not-persist-secret", "second-secret-sentinel", "password", "пароль", "access token",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("serialized row contains forbidden marker %q", forbidden)
		}
	}
}

func TestParseDefensivelyDropsSecretExtraFromTamperedPlan(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	if plan.Sheets[0].ExtraNames == nil {
		plan.Sheets[0].ExtraNames = make(map[int]string)
	}
	plan.Sheets[0].ExtraNames[14] = "password"
	row := firstParsedRowWithPlan(t, path, plan)
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "synthetic-password") {
		t.Fatal("tampered plan exposed a secret column")
	}
}

func TestParseRejectsNonEmptyColumnMissingFromPlan(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	delete(plan.Sheets[0].HeaderMap, 3)
	_, err := Parse(context.Background(), path, plan, DefaultLimits(), func(context.Context, SourceRow) error { return nil })
	parseErr := assertParseErrorCode(t, err, CodeUnsupportedWorkbook)
	if strings.Contains(parseErr.Error(), "Tester") {
		t.Fatal("error echoed an unmapped cell value")
	}
}

func TestParseRejectsCellLongerThanLimitIncludingSecretCell(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	sheets[0].Rows[0][14] = strings.Repeat("s", 100)
	path := writeWorkbook(t, sheets)
	plan := mustPreflight(t, path)
	limits := DefaultLimits()
	limits.MaxCellBytes = 40
	_, err := Parse(context.Background(), path, plan, limits, func(context.Context, SourceRow) error { return nil })
	assertParseErrorCode(t, err, CodeLimitExceeded)
}

func TestParseSkipsRowWhoseOnlyValueIsSecret(t *testing.T) {
	t.Parallel()

	sheets := syntheticLegacySheets()
	secretOnly := make([]any, len(sheets[0].Headers))
	secretOnly[14] = "SECRET-ONLY-ROW"
	sheets[0].Rows = append([][]any{secretOnly}, sheets[0].Rows...)
	path := writeWorkbook(t, sheets)
	plan := mustPreflight(t, path)
	var sheetARows []SourceRow
	_, err := Parse(context.Background(), path, plan, DefaultLimits(), func(_ context.Context, row SourceRow) error {
		if row.SheetProfile == SheetA {
			sheetARows = append(sheetARows, row)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sheetARows) != 1 || sheetARows[0].RowNumber != 3 {
		t.Fatalf("sheet A rows = %+v, want only cleaned row 3", sheetARows)
	}
}

func TestParseRejectsRowLimit(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	limits := DefaultLimits()
	limits.MaxRows = 1
	_, err := Parse(context.Background(), path, plan, limits, func(context.Context, SourceRow) error { return nil })
	assertParseErrorCode(t, err, CodeLimitExceeded)
}

func TestParseRejectsCellLimit(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	limits := DefaultLimits()
	limits.MaxCells = 20
	_, err := Parse(context.Background(), path, plan, limits, func(context.Context, SourceRow) error { return nil })
	assertParseErrorCode(t, err, CodeLimitExceeded)
}

func TestParseStopsImmediatelyWhenSinkFails(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	sentinel := errors.New("sink failed")
	calls := 0
	_, err := Parse(context.Background(), path, plan, DefaultLimits(), func(context.Context, SourceRow) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("sink result = error %v, calls %d", err, calls)
	}
}

func TestParseHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	plan := mustPreflight(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Parse(ctx, path, plan, DefaultLimits(), func(context.Context, SourceRow) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestParseRejectsNilSink(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	_, err := Parse(context.Background(), path, mustPreflight(t, path), DefaultLimits(), nil)
	assertParseErrorCode(t, err, CodeReadFailed)
}

func TestParseFingerprintIsStableForCleanedRow(t *testing.T) {
	t.Parallel()

	path := writeWorkbook(t, syntheticLegacySheets())
	first := firstParsedRow(t, path)
	second := firstParsedRow(t, path)
	if first.SourceFingerprintSHA256 == "" || first.SourceFingerprintSHA256 != second.SourceFingerprintSHA256 {
		t.Fatalf("fingerprints = %q and %q", first.SourceFingerprintSHA256, second.SourceFingerprintSHA256)
	}

	sheets := syntheticLegacySheets()
	sheets[0].Rows[0][3] = "Changed Synthetic Position"
	changed := firstParsedRow(t, writeWorkbook(t, sheets))
	if changed.SourceFingerprintSHA256 == first.SourceFingerprintSHA256 {
		t.Fatal("business field change did not change source fingerprint")
	}
}

func mustPreflight(t *testing.T, path string) WorkbookPlan {
	t.Helper()
	plan, err := Preflight(context.Background(), path, DefaultLimits())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	return plan
}

func firstParsedRow(t *testing.T, path string) SourceRow {
	t.Helper()
	return firstParsedRowWithPlan(t, path, mustPreflight(t, path))
}

func firstParsedRowWithPlan(t *testing.T, path string, plan WorkbookPlan) SourceRow {
	t.Helper()
	var first SourceRow
	errStop := errors.New("first row collected")
	_, err := Parse(context.Background(), path, plan, DefaultLimits(), func(_ context.Context, row SourceRow) error {
		first = row
		return errStop
	})
	if !errors.Is(err, errStop) {
		t.Fatalf("parse first row: %v", err)
	}
	return first
}

func mutateWorkbook(t *testing.T, path string, mutate func(*excelize.File)) {
	t.Helper()
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(workbook)
	if err := workbook.Save(); err != nil {
		_ = workbook.Close()
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
}
