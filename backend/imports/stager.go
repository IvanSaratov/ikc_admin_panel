package imports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// Stager verifies and copies one claimed workbook into import staging tables.
type Stager struct {
	database *sql.DB
	queries  *storagedb.Queries
	files    FileStore
	config   StagingConfig
	now      func() time.Time
}

func NewStager(
	database *sql.DB,
	queries *storagedb.Queries,
	files FileStore,
	config StagingConfig,
) (*Stager, error) {
	if database == nil {
		return nil, fmt.Errorf("imports staging database is required")
	}
	if queries == nil {
		return nil, fmt.Errorf("imports staging queries are required")
	}
	if files == nil {
		return nil, fmt.Errorf("imports staging file store is required")
	}
	if config.BatchSize <= 0 {
		return nil, fmt.Errorf("imports staging batch size must be positive")
	}
	if config.LeaseDuration <= 0 {
		return nil, fmt.Errorf("imports staging lease duration must be positive")
	}
	if !validLegacyLimits(config.LegacyLimits) {
		return nil, fmt.Errorf("imports staging workbook limits must be positive")
	}
	return &Stager{
		database: database,
		queries:  queries,
		files:    files,
		config:   config,
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// SetClock replaces the UTC clock used for lease and cleanup timestamps.
func (s *Stager) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Stage validates one claimed import before performing staging work.
func (s *Stager) Stage(ctx context.Context, claimed storagedb.Import) (StageOutcome, error) {
	if err := ctx.Err(); err != nil {
		return StageOutcome{}, err
	}
	if err := validateStagingImport(claimed); err != nil {
		return StageOutcome{}, err
	}
	if claimed.StagedAt.Valid {
		return s.cleanupStagedSource(ctx, claimed)
	}
	expiresAt, _ := time.Parse(time.RFC3339, claimed.TempFileExpiresAt.String)
	if !expiresAt.After(s.now().UTC()) {
		return StageOutcome{}, &StageError{Code: CodeSourceFileUnavailable}
	}
	path, err := s.files.Path(claimed.TempFileToken.String)
	if err != nil {
		return StageOutcome{}, &StageError{Code: CodeSourceFileUnavailable, Retryable: true, Err: err}
	}
	if err := verifyStagingSource(ctx, path, claimed.SourceSizeBytes.Int64, claimed.SourceSha256.String); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return StageOutcome{}, err
		}
		if errors.Is(err, errStagingSourceMismatch) {
			return StageOutcome{}, &StageError{Code: CodeSourceFileMismatch, Err: err}
		}
		return StageOutcome{}, &StageError{Code: CodeSourceFileUnavailable, Retryable: true, Err: err}
	}

	sheets, plan, err := s.loadStagingSheets(ctx, claimed)
	if err != nil {
		return StageOutcome{}, err
	}
	now := s.now().UTC()
	claimed, err = s.queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		Now:            nullableTime(now),
		LeaseExpiresAt: nullableTime(now.Add(s.config.LeaseDuration)),
		ImportID:       claimed.ID,
		LeaseOwner:     claimed.LeaseOwner,
	})
	if err != nil {
		return StageOutcome{}, classifyStagingMutation(err)
	}

	total := claimed.RowsTotal
	var current *stagedSheet
	pending := make([]pendingStageRow, 0, s.config.BatchSize)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		updatedTotal, err := s.flushBatch(ctx, claimed, current, pending, total)
		if err != nil {
			return err
		}
		total = updatedTotal
		pending = pending[:0]
		return nil
	}

	_, err = legacy.Parse(ctx, path, plan, s.config.LegacyLimits, func(ctx context.Context, source legacy.SourceRow) error {
		sheet := sheets[source.SheetName]
		if sheet == nil {
			return &StageError{Code: CodeStagingPlanIncompatible, Sheet: source.SheetName, Row: source.RowNumber}
		}
		if current != nil && current != sheet {
			if err := flush(); err != nil {
				return err
			}
		}
		current = sheet
		sheet.seen++
		if sheet.seen <= sheet.row.RowsStaged {
			return nil
		}
		rawJSON, err := json.Marshal(source)
		if err != nil {
			return &StageError{Code: CodeStagingFailed, Sheet: source.SheetName, Row: source.RowNumber, Err: err}
		}
		pending = append(pending, pendingStageRow{source: source, rawJSON: string(rawJSON)})
		if len(pending) >= s.config.BatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return StageOutcome{}, classifyParseFailure(err)
	}
	if err := flush(); err != nil {
		return StageOutcome{}, err
	}

	ordered := orderedStagingSheets(sheets)
	for _, sheet := range ordered {
		if sheet.seen < sheet.row.RowsStaged || sheet.committed != sheet.seen {
			return StageOutcome{}, &StageError{Code: CodeStagingCheckpointCorrupt, Sheet: sheet.row.SheetName}
		}
		now = s.now().UTC()
		if _, err := s.queries.CompleteImportSheetStaging(ctx, storagedb.CompleteImportSheetStagingParams{
			RowsFound:  sheet.seen,
			RowsStaged: sheet.committed,
			UpdatedAt:  now.Format(time.RFC3339),
			SheetID:    sheet.row.ID,
			ImportID:   claimed.ID,
			LeaseOwner: claimed.LeaseOwner,
			Now:        nullableTime(now),
		}); err != nil {
			return StageOutcome{}, classifyStagingMutation(err)
		}
	}

	now = s.now().UTC()
	completed, err := s.queries.CompleteImportStaging(ctx, storagedb.CompleteImportStagingParams{
		RowsTotal:      total,
		Now:            nullableTime(now),
		LeaseExpiresAt: nullableTime(now.Add(s.config.LeaseDuration)),
		ImportID:       claimed.ID,
		LeaseOwner:     claimed.LeaseOwner,
	})
	if err != nil {
		return StageOutcome{}, classifyStagingMutation(err)
	}
	outcome, err := s.cleanupStagedSource(ctx, completed)
	if err != nil {
		return StageOutcome{}, err
	}
	outcome.SheetsStaged = len(ordered)
	return outcome, nil
}

func (s *Stager) cleanupStagedSource(ctx context.Context, claimed storagedb.Import) (StageOutcome, error) {
	if claimed.TempFileToken.Valid {
		if err := s.files.Delete(claimed.TempFileToken.String); err != nil {
			return StageOutcome{}, &StageError{Code: CodeSourceCleanupFailed, Retryable: true, Err: err}
		}
		if _, err := s.queries.ClearImportTempFile(ctx, storagedb.ClearImportTempFileParams{
			Now:        s.now().UTC().Format(time.RFC3339),
			ImportID:   claimed.ID,
			LeaseOwner: claimed.LeaseOwner,
		}); err != nil {
			return StageOutcome{}, classifyStagingMutation(err)
		}
	}
	return StageOutcome{ImportID: claimed.ID, RowsStaged: claimed.RowsTotal}, nil
}

type stagedSheet struct {
	row       storagedb.ImportSheet
	seen      int64
	committed int64
}

type pendingStageRow struct {
	source  legacy.SourceRow
	rawJSON string
}

func (s *Stager) loadStagingSheets(
	ctx context.Context,
	claimed storagedb.Import,
) (map[string]*stagedSheet, legacy.WorkbookPlan, error) {
	rows, err := s.queries.ListImportSheets(ctx, claimed.ID)
	if err != nil {
		return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingStorage, Retryable: true, Err: err}
	}
	if len(rows) == 0 {
		return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingPlanIncompatible}
	}
	sheets := make(map[string]*stagedSheet, len(rows))
	plan := legacy.WorkbookPlan{Sheets: make([]legacy.SheetPlan, 0, len(rows))}
	var stagedTotal int64
	for index, row := range rows {
		if row.SheetOrder != int64(index+1) || row.RowsFound < row.RowsStaged || row.RowsStaged < 0 {
			return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingCheckpointCorrupt, Sheet: row.SheetName}
		}
		switch row.Status {
		case "pending":
			if row.RowsFound != 0 || row.RowsStaged != 0 {
				return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingCheckpointCorrupt, Sheet: row.SheetName}
			}
		case "parsing":
		case "staged":
			if row.RowsFound != row.RowsStaged {
				return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingCheckpointCorrupt, Sheet: row.SheetName}
			}
		default:
			return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingCheckpointCorrupt, Sheet: row.SheetName}
		}
		if _, duplicate := sheets[row.SheetName]; duplicate {
			return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingPlanIncompatible}
		}
		decoded, err := legacy.DecodeSheetPlan(
			row.SheetName,
			int(row.SheetOrder),
			legacy.SheetProfile(row.SheetProfile),
			row.HeaderMap,
		)
		if err != nil {
			return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingPlanIncompatible, Sheet: row.SheetName, Err: err}
		}
		sheet := &stagedSheet{row: row, committed: row.RowsStaged}
		sheets[row.SheetName] = sheet
		plan.Sheets = append(plan.Sheets, decoded)
		stagedTotal += row.RowsStaged
	}
	if stagedTotal != claimed.RowsTotal {
		return nil, legacy.WorkbookPlan{}, &StageError{Code: CodeStagingCheckpointCorrupt}
	}
	return sheets, plan, nil
}

func orderedStagingSheets(sheets map[string]*stagedSheet) []*stagedSheet {
	ordered := make([]*stagedSheet, len(sheets))
	for _, sheet := range sheets {
		ordered[sheet.row.SheetOrder-1] = sheet
	}
	return ordered
}

func (s *Stager) flushBatch(
	ctx context.Context,
	claimed storagedb.Import,
	sheet *stagedSheet,
	pending []pendingStageRow,
	total int64,
) (updatedTotal int64, returnErr error) {
	if sheet == nil || len(pending) == 0 {
		return total, nil
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return total, &StageError{Code: CodeStagingStorage, Retryable: true, Err: err}
	}
	defer func() {
		if returnErr != nil {
			_ = tx.Rollback()
		}
	}()
	queries := s.queries.WithTx(tx)
	now := s.now().UTC()
	updatedTotal = total + int64(len(pending))
	if _, err := queries.UpdateImportStagingProgress(ctx, storagedb.UpdateImportStagingProgressParams{
		RowsTotal:      updatedTotal,
		Now:            nullableTime(now),
		LeaseExpiresAt: nullableTime(now.Add(s.config.LeaseDuration)),
		ImportID:       claimed.ID,
		LeaseOwner:     claimed.LeaseOwner,
	}); err != nil {
		return total, classifyStagingMutation(err)
	}
	for _, pendingRow := range pending {
		created, err := queries.CreateImportRow(ctx, storagedb.CreateImportRowParams{
			ImportID:  claimed.ID,
			SheetName: pendingRow.source.SheetName,
			RowNumber: pendingRow.source.RowNumber,
			RawData:   pendingRow.rawJSON,
			CreatedAt: now.Format(time.RFC3339),
		})
		if err != nil {
			_, coordinateErr := queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
				ImportID:  claimed.ID,
				SheetName: pendingRow.source.SheetName,
				RowNumber: pendingRow.source.RowNumber,
			})
			if coordinateErr == nil {
				return total, &StageError{
					Code:  CodeStagingCheckpointCorrupt,
					Sheet: pendingRow.source.SheetName,
					Row:   pendingRow.source.RowNumber,
					Err:   err,
				}
			}
			return total, &StageError{
				Code: CodeStagingStorage, Retryable: true,
				Sheet: pendingRow.source.SheetName, Row: pendingRow.source.RowNumber, Err: err,
			}
		}
		extraFields := "{}"
		if len(pendingRow.source.ExtraFields) > 0 {
			encoded, err := json.Marshal(pendingRow.source.ExtraFields)
			if err != nil {
				return total, &StageError{Code: CodeStagingFailed, Sheet: pendingRow.source.SheetName, Row: pendingRow.source.RowNumber, Err: err}
			}
			extraFields = string(encoded)
		}
		if _, err := queries.CreateLegacyImportRow(ctx, storagedb.CreateLegacyImportRowParams{
			ImportRowID:       created.ID,
			SourceFingerprint: pendingRow.source.SourceFingerprintSHA256,
			ExtraFields:       extraFields,
			CreatedAt:         now.Format(time.RFC3339),
			UpdatedAt:         now.Format(time.RFC3339),
		}); err != nil {
			return total, &StageError{
				Code: CodeStagingStorage, Retryable: true,
				Sheet: pendingRow.source.SheetName, Row: pendingRow.source.RowNumber, Err: err,
			}
		}
	}
	newSheetTotal := sheet.committed + int64(len(pending))
	if _, err := queries.UpdateImportSheetStagingProgress(ctx, storagedb.UpdateImportSheetStagingProgressParams{
		RowsFound:  sheet.seen,
		RowsStaged: newSheetTotal,
		UpdatedAt:  now.Format(time.RFC3339),
		SheetID:    sheet.row.ID,
		ImportID:   claimed.ID,
		LeaseOwner: claimed.LeaseOwner,
		Now:        nullableTime(now),
	}); err != nil {
		return total, classifyStagingMutation(err)
	}
	if err := tx.Commit(); err != nil {
		return total, &StageError{Code: CodeStagingStorage, Retryable: true, Err: err}
	}
	sheet.committed = newSheetTotal
	return updatedTotal, nil
}

func nullableTime(value time.Time) sql.NullString {
	return sql.NullString{String: value.UTC().Format(time.RFC3339), Valid: true}
}

func classifyStagingMutation(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &StageError{Code: CodeStageLeaseLost, Retryable: true, Err: err}
	}
	return &StageError{Code: CodeStagingStorage, Retryable: true, Err: err}
}

func classifyParseFailure(err error) error {
	var stageErr *StageError
	if errors.As(err, &stageErr) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var parseErr *legacy.ParseError
	if errors.As(err, &parseErr) {
		return &StageError{
			Code:  CodeStagingFailed,
			Sheet: parseErr.Sheet,
			Row:   parseErr.Row,
			Err:   err,
		}
	}
	return &StageError{Code: CodeStagingFailed, Retryable: true, Err: err}
}

var errStagingSourceMismatch = errors.New("staging source mismatch")

func verifyStagingSource(ctx context.Context, path string, wantSize int64, wantSHA string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	digest := sha256.New()
	buffer := make([]byte, 32*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			size += int64(read)
			if _, err := digest.Write(buffer[:read]); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	if size != wantSize || !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), wantSHA) {
		return errStagingSourceMismatch
	}
	return nil
}

func validateStagingImport(claimed storagedb.Import) error {
	invalid := func() error {
		return &StageError{Code: CodeStageInvalidInput}
	}
	if claimed.ID <= 0 || claimed.Status != "processing" || claimed.Profile != legacyProfile {
		return invalid()
	}
	if !claimed.LeaseOwner.Valid || strings.TrimSpace(claimed.LeaseOwner.String) == "" ||
		!claimed.LeaseExpiresAt.Valid {
		return invalid()
	}
	if !claimed.SourceSha256.Valid || strings.TrimSpace(claimed.SourceSha256.String) == "" ||
		!claimed.SourceSizeBytes.Valid || claimed.SourceSizeBytes.Int64 <= 0 {
		return invalid()
	}
	if len(claimed.SourceSha256.String) != sha256.Size*2 {
		return invalid()
	}
	if _, err := hex.DecodeString(claimed.SourceSha256.String); err != nil {
		return invalid()
	}
	if _, err := time.Parse(time.RFC3339, claimed.LeaseExpiresAt.String); err != nil {
		return invalid()
	}
	if claimed.StagedAt.Valid {
		if !claimed.Phase.Valid || claimed.Phase.String != "validating" ||
			claimed.TempFileToken.Valid != claimed.TempFileExpiresAt.Valid {
			return invalid()
		}
		if claimed.TempFileToken.Valid {
			if strings.TrimSpace(claimed.TempFileToken.String) == "" || !validFileToken(claimed.TempFileToken.String) {
				return invalid()
			}
			if _, err := time.Parse(time.RFC3339, claimed.TempFileExpiresAt.String); err != nil {
				return invalid()
			}
		}
		return nil
	}
	if !claimed.Phase.Valid || (claimed.Phase.String != "parsing" && claimed.Phase.String != "staging") {
		return invalid()
	}
	if !claimed.TempFileToken.Valid || strings.TrimSpace(claimed.TempFileToken.String) == "" ||
		!validFileToken(claimed.TempFileToken.String) || !claimed.TempFileExpiresAt.Valid {
		return invalid()
	}
	if _, err := time.Parse(time.RFC3339, claimed.TempFileExpiresAt.String); err != nil {
		return invalid()
	}
	return nil
}

func validLegacyLimits(limits legacy.Limits) bool {
	return limits.MaxFileBytes > 0 && limits.MaxUncompressedBytes > 0 &&
		limits.MaxZIPEntries > 0 && limits.MaxSheets > 0 && limits.MaxRows > 0 &&
		limits.MaxCells > 0 && limits.MaxCellBytes > 0 && limits.MaxHeaderRows > 0
}
