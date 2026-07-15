package legacy

import (
	"archive/zip"
	"context"
	"math"
	"os"
	pathpkg "path"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Preflight validates the XLSX container and discovers the five supported
// sheet/header layouts without retaining business rows.
func Preflight(ctx context.Context, filePath string, limits Limits) (WorkbookPlan, error) {
	if err := ctx.Err(); err != nil {
		return WorkbookPlan{}, err
	}
	if err := validateArchive(filePath, limits); err != nil {
		return WorkbookPlan{}, err
	}

	workbook, err := excelize.OpenFile(filePath)
	if err != nil {
		return WorkbookPlan{}, codedWrap(CodeNotXLSX, "XLSX workbook cannot be opened", err)
	}
	defer workbook.Close()

	sheetNames := workbook.GetSheetList()
	if len(sheetNames) > limits.MaxSheets {
		return WorkbookPlan{}, coded(CodeLimitExceeded, "sheet count exceeds limit")
	}

	seenProfiles := make(map[SheetProfile]struct{}, 5)
	plan := WorkbookPlan{Sheets: make([]SheetPlan, 0, 5)}
	var rowsSeen, cellsSeen int64
	for sheetIndex, sheetName := range sheetNames {
		if err := ctx.Err(); err != nil {
			return WorkbookPlan{}, err
		}
		profile, supported := profileForSheet(sheetName)
		if supported {
			if _, duplicate := seenProfiles[profile]; duplicate {
				return WorkbookPlan{}, &ParseError{
					Code:   CodeUnsupportedWorkbook,
					Sheet:  sheetName,
					Detail: "duplicate normalized sheet name",
				}
			}
			seenProfiles[profile] = struct{}{}
		}

		rows, err := workbook.Rows(sheetName)
		if err != nil {
			return WorkbookPlan{}, sheetReadError(sheetName, err)
		}
		var discovered *SheetPlan
		hasData := false
		hasAnyValue := false
		rowNumber := int64(0)
		for rows.Next() {
			rowNumber++
			rowsSeen++
			if err := ctx.Err(); err != nil {
				_ = rows.Close()
				return WorkbookPlan{}, err
			}
			if rowsSeen > limits.MaxRows {
				_ = rows.Close()
				return WorkbookPlan{}, limitError(sheetName, rowNumber, "row count exceeds limit")
			}
			columns, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return WorkbookPlan{}, rowReadError(sheetName, rowNumber, err)
			}
			cellsSeen += int64(len(columns))
			if cellsSeen > limits.MaxCells {
				_ = rows.Close()
				return WorkbookPlan{}, limitError(sheetName, rowNumber, "cell count exceeds limit")
			}
			if anyCellTooLong(columns, limits.MaxCellBytes) {
				_ = rows.Close()
				return WorkbookPlan{}, limitError(sheetName, rowNumber, "cell length exceeds limit")
			}
			if emptyCells(columns) {
				continue
			}
			hasAnyValue = true
			if !supported {
				continue
			}
			if discovered == nil && rowNumber <= int64(limits.MaxHeaderRows) {
				candidate, candidateErr := buildSheetPlan(profile, sheetName, sheetIndex+1, rowNumber, columns)
				if candidateErr == nil {
					discovered = &candidate
					continue
				}
				var parseErr *ParseError
				if !asParseError(candidateErr, &parseErr) || parseErr.Code != CodeMissingColumns {
					_ = rows.Close()
					return WorkbookPlan{}, candidateErr
				}
				continue
			}
			if discovered != nil && rowNumber > discovered.HeaderRow {
				if firstUnclassifiedNonEmpty(*discovered, columns) >= 0 {
					_ = rows.Close()
					return WorkbookPlan{}, &ParseError{
						Code:   CodeUnsupportedWorkbook,
						Sheet:  sheetName,
						Row:    rowNumber,
						Detail: "non-empty column has no recognized header",
					}
				}
				hasData = true
			}
		}
		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return WorkbookPlan{}, sheetReadError(sheetName, err)
		}
		if err := rows.Close(); err != nil {
			return WorkbookPlan{}, sheetReadError(sheetName, err)
		}

		if !supported {
			if hasAnyValue {
				return WorkbookPlan{}, &ParseError{
					Code:   CodeUnknownSheet,
					Sheet:  sheetName,
					Detail: "unknown non-empty sheet",
				}
			}
			continue
		}
		if discovered == nil {
			return WorkbookPlan{}, &ParseError{
				Code:   CodeMissingColumns,
				Sheet:  sheetName,
				Detail: "required columns were not recognized",
			}
		}
		if !hasData {
			return WorkbookPlan{}, &ParseError{
				Code:   CodeUnsupportedWorkbook,
				Sheet:  sheetName,
				Detail: "required sheet contains no data rows",
			}
		}
		plan.Sheets = append(plan.Sheets, *discovered)
	}

	for _, required := range []SheetProfile{SheetA, SheetB, SheetV, SheetP, SheetS} {
		if _, ok := seenProfiles[required]; !ok {
			return WorkbookPlan{}, &ParseError{
				Code:   CodeMissingSheet,
				Sheet:  string(required),
				Detail: "required sheet is missing",
			}
		}
	}
	return plan, nil
}

func validateArchive(filePath string, limits Limits) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return codedWrap(CodeReadFailed, "source file cannot be read", err)
	}
	if info.Size() > limits.MaxFileBytes {
		return coded(CodeWorkbookTooLarge, "file size exceeds limit")
	}

	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return codedWrap(CodeNotXLSX, "file is not a readable ZIP/XLSX", err)
	}
	defer reader.Close()
	if len(reader.File) > limits.MaxZIPEntries {
		return coded(CodeLimitExceeded, "ZIP entry count exceeds limit")
	}

	requiredParts := map[string]bool{
		"[Content_Types].xml": false,
		"xl/workbook.xml":     false,
	}
	var totalUncompressed uint64
	for _, entry := range reader.File {
		normalized := strings.ReplaceAll(entry.Name, "\\", "/")
		trimmed := strings.TrimSuffix(normalized, "/")
		cleaned := pathpkg.Clean(trimmed)
		if normalized != entry.Name || cleaned == "." || cleaned != trimmed || strings.HasPrefix(cleaned, "../") || pathpkg.IsAbs(cleaned) || strings.Contains(cleaned, ":") {
			return coded(CodeNotXLSX, "ZIP contains an unsafe path")
		}
		if entry.Flags&0x1 != 0 {
			return coded(CodeNotXLSX, "encrypted XLSX is not supported")
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return coded(CodeNotXLSX, "ZIP symbolic links are not supported")
		}
		if entry.Name == "xl/vbaProject.bin" {
			return coded(CodeNotXLSX, "macro-enabled workbooks are not supported")
		}
		if entry.UncompressedSize64 > math.MaxUint64-totalUncompressed {
			return coded(CodeLimitExceeded, "uncompressed ZIP size overflow")
		}
		totalUncompressed += entry.UncompressedSize64
		if totalUncompressed > limits.MaxUncompressedBytes {
			return coded(CodeLimitExceeded, "uncompressed workbook exceeds limit")
		}
		if _, required := requiredParts[entry.Name]; required {
			requiredParts[entry.Name] = true
		}
	}
	if !requiredParts["[Content_Types].xml"] || !requiredParts["xl/workbook.xml"] {
		return coded(CodeNotXLSX, "ZIP does not contain XLSX workbook parts")
	}
	return nil
}

func emptyCells(cells []string) bool {
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func anyCellTooLong(cells []string, maxBytes int) bool {
	for _, cell := range cells {
		if len(cell) > maxBytes {
			return true
		}
	}
	return false
}

func firstUnclassifiedNonEmpty(sheet SheetPlan, cells []string) int {
	for column, cell := range cells {
		if strings.TrimSpace(cell) == "" {
			continue
		}
		if _, ok := sheet.HeaderMap[column]; ok {
			continue
		}
		if _, ok := sheet.ExtraNames[column]; ok {
			continue
		}
		if _, ok := sheet.secretColumns[column]; ok {
			continue
		}
		return column
	}
	return -1
}

func asParseError(err error, target **ParseError) bool {
	parseErr, ok := err.(*ParseError)
	if !ok {
		return false
	}
	*target = parseErr
	return true
}

func sheetReadError(sheet string, err error) *ParseError {
	return &ParseError{Code: CodeReadFailed, Sheet: sheet, Detail: "sheet cannot be read", Err: err}
}

func rowReadError(sheet string, row int64, err error) *ParseError {
	return &ParseError{Code: CodeReadFailed, Sheet: sheet, Row: row, Detail: "row cannot be read", Err: err}
}

func limitError(sheet string, row int64, detail string) *ParseError {
	return &ParseError{Code: CodeLimitExceeded, Sheet: sheet, Row: row, Detail: detail}
}
