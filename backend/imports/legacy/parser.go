package legacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/xuri/excelize/v2"
)

// RowSink accepts one cleaned row. Returning an error stops Parse immediately.
type RowSink func(context.Context, SourceRow) error

// Parse streams the sheets described by plan and emits cleaned business rows.
// It does not calculate formulas and never copies secret-like columns.
func Parse(ctx context.Context, filePath string, plan WorkbookPlan, limits Limits, sink RowSink) (ParseStats, error) {
	if err := ctx.Err(); err != nil {
		return ParseStats{}, err
	}
	if sink == nil {
		return ParseStats{}, coded(CodeReadFailed, "row sink is required")
	}

	workbook, err := excelize.OpenFile(filePath)
	if err != nil {
		return ParseStats{}, codedWrap(CodeReadFailed, "workbook cannot be opened", err)
	}
	defer workbook.Close()

	var stats ParseStats
	for _, sheet := range plan.Sheets {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		rows, err := workbook.Rows(sheet.Name)
		if err != nil {
			return stats, sheetReadError(sheet.Name, err)
		}
		rowNumber := int64(0)
		for rows.Next() {
			rowNumber++
			if err := ctx.Err(); err != nil {
				_ = rows.Close()
				return stats, err
			}
			columns, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return stats, rowReadError(sheet.Name, rowNumber, err)
			}
			stats.Cells += int64(len(columns))
			if stats.Cells > limits.MaxCells {
				_ = rows.Close()
				return stats, limitError(sheet.Name, rowNumber, "cell count exceeds limit")
			}
			if anyCellTooLong(columns, limits.MaxCellBytes) {
				_ = rows.Close()
				return stats, limitError(sheet.Name, rowNumber, "cell length exceeds limit")
			}
			if rowNumber <= sheet.HeaderRow || emptyCells(columns) {
				continue
			}
			if firstUnclassifiedNonEmpty(sheet, columns) >= 0 {
				_ = rows.Close()
				return stats, &ParseError{
					Code:   CodeUnsupportedWorkbook,
					Sheet:  sheet.Name,
					Row:    rowNumber,
					Detail: "non-empty column is missing from parser plan",
				}
			}

			source, err := extractSourceRow(sheet, rowNumber, columns)
			if err != nil {
				_ = rows.Close()
				return stats, err
			}
			if sourceRowEmpty(source) {
				continue
			}
			stats.Rows++
			if stats.Rows > limits.MaxRows {
				_ = rows.Close()
				return stats, limitError(sheet.Name, rowNumber, "row count exceeds limit")
			}
			if err := sink(ctx, source); err != nil {
				_ = rows.Close()
				return stats, err
			}
		}
		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return stats, sheetReadError(sheet.Name, err)
		}
		if err := rows.Close(); err != nil {
			return stats, sheetReadError(sheet.Name, err)
		}
	}
	return stats, nil
}

func extractSourceRow(sheet SheetPlan, rowNumber int64, columns []string) (SourceRow, error) {
	row := SourceRow{
		SheetName:    sheet.Name,
		SheetProfile: sheet.Profile,
		RowNumber:    rowNumber,
	}
	for column, rawValue := range columns {
		value := strings.TrimSpace(rawValue)
		if field, ok := sheet.HeaderMap[column]; ok {
			setSourceField(&row, field, value)
			continue
		}
		extraName, ok := sheet.ExtraNames[column]
		if !ok || value == "" || isSecretHeader(extraName) {
			continue
		}
		if row.ExtraFields == nil {
			row.ExtraFields = make(map[string]string)
		}
		row.ExtraFields[normalizeLabel(extraName)] = value
	}

	fingerprint, err := fingerprintSourceRow(row)
	if err != nil {
		return SourceRow{}, &ParseError{
			Code:   CodeReadFailed,
			Sheet:  sheet.Name,
			Row:    rowNumber,
			Detail: "cleaned row fingerprint cannot be calculated",
			Err:    err,
		}
	}
	row.SourceFingerprintSHA256 = fingerprint
	return row, nil
}

func setSourceField(row *SourceRow, field Field, value string) {
	switch field {
	case FieldEmployer:
		row.EmployerName = value
	case FieldINN:
		row.INN = value
	case FieldFullName:
		row.FullName = value
	case FieldPosition:
		row.Position = value
	case FieldDepartment:
		row.Department = value
	case FieldSNILS:
		row.SNILS = value
	case FieldProgram:
		row.ProgramText = value
	case FieldTrainingPeriod:
		row.TrainingPeriod = value
	case FieldProtocolNumber:
		row.ProtocolNumber = value
	case FieldProtocolDate:
		row.ProtocolDate = value
	case FieldAssessment:
		row.AssessmentResult = value
	case FieldRegistryNumber:
		row.MintrudRegistryNumber = value
	case FieldSourceRef:
		row.SourceReference = value
	case FieldMoodleEmail:
		row.MoodleEmail = value
	case FieldMoodleUsername:
		row.MoodleUsername = value
	}
}

func sourceRowEmpty(row SourceRow) bool {
	return row.EmployerName == "" &&
		row.INN == "" &&
		row.FullName == "" &&
		row.Position == "" &&
		row.Department == "" &&
		row.SNILS == "" &&
		row.ProgramText == "" &&
		row.TrainingPeriod == "" &&
		row.ProtocolNumber == "" &&
		row.ProtocolDate == "" &&
		row.AssessmentResult == "" &&
		row.MintrudRegistryNumber == "" &&
		row.SourceReference == "" &&
		row.MoodleEmail == "" &&
		row.MoodleUsername == "" &&
		len(row.ExtraFields) == 0
}

func fingerprintSourceRow(row SourceRow) (string, error) {
	row.SheetName = ""
	row.RowNumber = 0
	row.SourceFingerprintSHA256 = ""
	data, err := json.Marshal(row)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
