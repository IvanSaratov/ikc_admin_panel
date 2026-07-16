package imports_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func openDatabase(t *testing.T) *sql.DB {
	t.Helper()

	database, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "imports.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return database
}

func TestClaimNextImportKeepsSingleActiveLease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	queries := storagedb.New(database)
	now := "2026-07-15T12:00:00Z"
	for _, key := range []string{"queue-1", "queue-2"} {
		if _, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
			Profile:           "legacy_registry",
			IdempotencyKey:    sql.NullString{String: key, Valid: true},
			UploadedByActor:   "admin",
			ReceivedAt:        now,
			Status:            "queued",
			Phase:             sql.NullString{},
			TempFileToken:     sql.NullString{},
			TempFileExpiresAt: sql.NullString{},
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			t.Fatalf("create import %s: %v", key, err)
		}
	}

	first, err := queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-1", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-15T12:01:00Z", Valid: true},
		Now:            sql.NullString{String: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("claim first import: %v", err)
	}
	if first.ID != 1 || first.Attempt != 1 || first.Status != "processing" {
		t.Fatalf("first claim = %+v", first)
	}

	_, err = queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-2", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-15T12:01:00Z", Valid: true},
		Now:            sql.NullString{String: now, Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second claim error = %v, want sql.ErrNoRows while first lease is active", err)
	}
}

func TestStagingLeaseOwnsPhaseProgressAndCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	queries := storagedb.New(database)
	now := sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true}
	leaseExpiry := sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true}
	_, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
		Profile:           "legacy_registry",
		SourceSha256:      sql.NullString{String: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Valid: true},
		SourceSizeBytes:   sql.NullInt64{Int64: 1024, Valid: true},
		UploadedByActor:   "test-admin",
		ReceivedAt:        now.String,
		Status:            "queued",
		TempFileToken:     sql.NullString{String: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Valid: true},
		TempFileExpiresAt: sql.NullString{String: "2026-07-17T12:00:00Z", Valid: true},
		CreatedAt:         now.String,
		UpdatedAt:         now.String,
	})
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	claimed, err := queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}

	_, err = queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-b", Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign owner start error = %v, want sql.ErrNoRows", err)
	}
	expiredNow := sql.NullString{String: "2026-07-16T12:03:00Z", Valid: true}
	_, err = queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:05:00Z", Valid: true},
		Now:            expiredNow,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired owner renewed staging lease: %v", err)
	}
	staging, err := queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if err != nil {
		t.Fatalf("start staging: %v", err)
	}
	if staging.Phase.String != "staging" || staging.RowsTotal != 0 {
		t.Fatalf("staging state = %+v", staging)
	}

	progress, err := queries.UpdateImportStagingProgress(ctx, storagedb.UpdateImportStagingProgressParams{
		RowsTotal:      3,
		Now:            now,
		LeaseExpiresAt: leaseExpiry,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if err != nil {
		t.Fatalf("update staging progress: %v", err)
	}
	if progress.RowsTotal != 3 {
		t.Fatalf("rows total = %d, want 3", progress.RowsTotal)
	}
	_, err = queries.UpdateImportStagingProgress(ctx, storagedb.UpdateImportStagingProgressParams{
		RowsTotal:      2,
		Now:            now,
		LeaseExpiresAt: leaseExpiry,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("decreasing progress error = %v, want sql.ErrNoRows", err)
	}

	completed, err := queries.CompleteImportStaging(ctx, storagedb.CompleteImportStagingParams{
		RowsTotal:      3,
		Now:            now,
		LeaseExpiresAt: leaseExpiry,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if err != nil {
		t.Fatalf("complete staging: %v", err)
	}
	if completed.Phase.String != "validating" || !completed.StagedAt.Valid || !completed.TempFileToken.Valid {
		t.Fatalf("completed staging state = %+v", completed)
	}
	_, err = queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("validating import restarted staging: %v", err)
	}
	_, err = queries.ClearImportTempFile(ctx, storagedb.ClearImportTempFileParams{
		Now:        now.String,
		ImportID:   claimed.ID,
		LeaseOwner: sql.NullString{String: "lease-b", Valid: true},
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign owner cleanup error = %v, want sql.ErrNoRows", err)
	}
	cleaned, err := queries.ClearImportTempFile(ctx, storagedb.ClearImportTempFileParams{
		Now:        now.String,
		ImportID:   claimed.ID,
		LeaseOwner: sql.NullString{String: "lease-a", Valid: true},
	})
	if err != nil {
		t.Fatalf("clear import temp file: %v", err)
	}
	if cleaned.TempFileToken.Valid || cleaned.TempFileExpiresAt.Valid {
		t.Fatalf("temporary source metadata survived cleanup: %+v", cleaned)
	}
}

func TestStagingCheckpointTransactionRollsBackWhenLeaseIsLost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	queries := storagedb.New(database)
	now := sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true}
	leaseExpiry := sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true}
	created, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
		Profile:         "legacy_registry",
		UploadedByActor: "test-admin",
		ReceivedAt:      now.String,
		Status:          "queued",
		CreatedAt:       now.String,
		UpdatedAt:       now.String,
	})
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	sheet, err := queries.CreateImportSheet(ctx, storagedb.CreateImportSheetParams{
		ImportID:     created.ID,
		SheetName:    "А",
		SheetOrder:   1,
		SheetProfile: "А",
		HeaderMap:    `{"version":1}`,
		CreatedAt:    now.String,
		UpdatedAt:    now.String,
	})
	if err != nil {
		t.Fatalf("create import sheet: %v", err)
	}
	claimed, err := queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	if _, err := queries.StartImportStaging(ctx, storagedb.StartImportStagingParams{
		LeaseExpiresAt: leaseExpiry,
		Now:            now,
		ImportID:       claimed.ID,
		LeaseOwner:     sql.NullString{String: "lease-a", Valid: true},
	}); err != nil {
		t.Fatalf("start staging: %v", err)
	}

	writeBatch := func(owner string) error {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		txQueries := queries.WithTx(tx)
		row, err := txQueries.CreateImportRow(ctx, storagedb.CreateImportRowParams{
			ImportID:  claimed.ID,
			SheetName: "А",
			RowNumber: 2,
			RawData:   `{"safe":"synthetic"}`,
			CreatedAt: now.String,
		})
		if err != nil {
			return err
		}
		if _, err := txQueries.CreateLegacyImportRow(ctx, storagedb.CreateLegacyImportRowParams{
			ImportRowID:       row.ID,
			SourceFingerprint: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			ExtraFields:       `{}`,
			CreatedAt:         now.String,
			UpdatedAt:         now.String,
		}); err != nil {
			return err
		}
		if _, err := txQueries.UpdateImportSheetStagingProgress(ctx, storagedb.UpdateImportSheetStagingProgressParams{
			RowsFound:  1,
			RowsStaged: 1,
			UpdatedAt:  now.String,
			SheetID:    sheet.ID,
			ImportID:   claimed.ID,
			LeaseOwner: sql.NullString{String: owner, Valid: true},
			Now:        now,
		}); err != nil {
			return err
		}
		if _, err := txQueries.UpdateImportStagingProgress(ctx, storagedb.UpdateImportStagingProgressParams{
			RowsTotal:      1,
			Now:            now,
			LeaseExpiresAt: leaseExpiry,
			ImportID:       claimed.ID,
			LeaseOwner:     sql.NullString{String: owner, Valid: true},
		}); err != nil {
			return err
		}
		return tx.Commit()
	}

	if err := writeBatch("lease-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign-owner batch error = %v, want sql.ErrNoRows", err)
	}
	_, err = queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
		ImportID:  claimed.ID,
		SheetName: "А",
		RowNumber: 2,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back row lookup error = %v, want sql.ErrNoRows", err)
	}

	if err := writeBatch("lease-a"); err != nil {
		t.Fatalf("owned batch: %v", err)
	}
	row, err := queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
		ImportID:  claimed.ID,
		SheetName: "А",
		RowNumber: 2,
	})
	if err != nil {
		t.Fatalf("get committed import row: %v", err)
	}
	if _, err := queries.GetLegacyImportRow(ctx, row.ID); err != nil {
		t.Fatalf("get committed legacy row: %v", err)
	}
	updatedSheet, err := queries.GetImportSheet(ctx, storagedb.GetImportSheetParams{ImportID: claimed.ID, SheetName: "А"})
	if err != nil {
		t.Fatalf("get updated sheet: %v", err)
	}
	if updatedSheet.RowsStaged != 1 || updatedSheet.Status != "parsing" {
		t.Fatalf("sheet checkpoint = %+v", updatedSheet)
	}
	_, err = queries.UpdateImportSheetStagingProgress(ctx, storagedb.UpdateImportSheetStagingProgressParams{
		RowsFound:  1,
		RowsStaged: 1,
		UpdatedAt:  now.String,
		SheetID:    sheet.ID,
		ImportID:   claimed.ID,
		LeaseOwner: sql.NullString{String: "lease-b", Valid: true},
		Now:        now,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign owner changed sheet checkpoint: %v", err)
	}
	_, err = queries.UpdateImportSheetStagingProgress(ctx, storagedb.UpdateImportSheetStagingProgressParams{
		RowsFound:  0,
		RowsStaged: 1,
		UpdatedAt:  now.String,
		SheetID:    sheet.ID,
		ImportID:   claimed.ID,
		LeaseOwner: sql.NullString{String: "lease-a", Valid: true},
		Now:        now,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("decreasing sheet progress error = %v, want sql.ErrNoRows", err)
	}
	stagedSheet, err := queries.CompleteImportSheetStaging(ctx, storagedb.CompleteImportSheetStagingParams{
		RowsFound:  1,
		RowsStaged: 1,
		UpdatedAt:  now.String,
		SheetID:    sheet.ID,
		ImportID:   claimed.ID,
		LeaseOwner: sql.NullString{String: "lease-a", Valid: true},
		Now:        now,
	})
	if err != nil {
		t.Fatalf("complete sheet staging: %v", err)
	}
	if stagedSheet.Status != "staged" || stagedSheet.RowsStaged != 1 {
		t.Fatalf("completed sheet checkpoint = %+v", stagedSheet)
	}
}

func TestGeneratedQueriesPersistImportSheet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	now := "2026-07-15T12:00:00Z"
	result, err := database.ExecContext(ctx, `
		INSERT INTO imports (
			profile, uploaded_by_actor, received_at, status, created_at, updated_at
		)
		VALUES ('legacy_registry', 'admin', ?, 'queued', ?, ?)
	`, now, now, now)
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	importID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("import id: %v", err)
	}

	queries := storagedb.New(database)
	sheet, err := queries.CreateImportSheet(ctx, storagedb.CreateImportSheetParams{
		ImportID:     importID,
		SheetName:    "А",
		SheetOrder:   1,
		SheetProfile: "А",
		HeaderMap:    `{"A":"organization"}`,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create import sheet: %v", err)
	}
	if sheet.ImportID != importID || sheet.SheetName != "А" || sheet.Status != "pending" {
		t.Fatalf("created sheet = %+v", sheet)
	}
}

func TestGeneratedQueriesPersistLegacyRowIssueAndAlias(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	now := "2026-07-15T12:00:00Z"

	result, err := database.ExecContext(ctx, `
		INSERT INTO imports (
			profile, uploaded_by_actor, received_at, status, created_at, updated_at
		)
		VALUES ('legacy_registry', 'admin', ?, 'queued', ?, ?)
	`, now, now, now)
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	importID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("import id: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO import_sheets (
			import_id, sheet_name, sheet_order, sheet_profile, header_map,
			status, created_at, updated_at
		)
		VALUES (?, 'А', 1, 'А', '{}', 'staged', ?, ?)
	`, importID, now, now); err != nil {
		t.Fatalf("create sheet: %v", err)
	}
	rowResult, err := database.ExecContext(ctx, `
		INSERT INTO import_rows (import_id, sheet_name, row_number, raw_data, created_at)
		VALUES (?, 'А', 2, '{}', ?)
	`, importID, now)
	if err != nil {
		t.Fatalf("create import row: %v", err)
	}
	rowID, err := rowResult.LastInsertId()
	if err != nil {
		t.Fatalf("import row id: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES ('A', 'Test Group A', 'active', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("create program group: %v", err)
	}
	programResult, err := database.ExecContext(ctx, `
		INSERT INTO programs (
			program_group_id, code, name, default_hours, status, created_at, updated_at
		)
		VALUES (1, 'A-1', 'Test Program A', 16, 'active', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("create program: %v", err)
	}
	programID, err := programResult.LastInsertId()
	if err != nil {
		t.Fatalf("program id: %v", err)
	}

	queries := storagedb.New(database)
	legacyRow, err := queries.CreateLegacyImportRow(ctx, storagedb.CreateLegacyImportRowParams{
		ImportRowID:       rowID,
		SourceFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExtraFields:       `{}`,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("create legacy row: %v", err)
	}
	issue, err := queries.CreateImportRowIssue(ctx, storagedb.CreateImportRowIssueParams{
		ImportRowID: rowID,
		Field:       "program",
		Code:        "unknown_program",
		Severity:    "blocking",
		Message:     "Program requires mapping",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	alias, err := queries.CreateProgramAlias(ctx, storagedb.CreateProgramAliasParams{
		Profile:         "legacy_registry",
		SheetProfile:    "А",
		AliasNormalized: "program a",
		ProgramID:       programID,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}

	if legacyRow.Status != "staged" || issue.Code != "unknown_program" || alias.ProgramID != programID {
		t.Fatalf("persisted values = row %+v, issue %+v, alias %+v", legacyRow, issue, alias)
	}
}

func TestLegacyRowsUseCursorPagination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	queries := storagedb.New(database)
	now := "2026-07-15T12:00:00Z"
	created, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
		Profile:           "legacy_registry",
		UploadedByActor:   "admin",
		ReceivedAt:        now,
		Status:            "queued",
		Phase:             sql.NullString{},
		TempFileToken:     sql.NullString{},
		TempFileExpiresAt: sql.NullString{},
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	if _, err := queries.CreateImportSheet(ctx, storagedb.CreateImportSheetParams{
		ImportID:     created.ID,
		SheetName:    "А",
		SheetOrder:   1,
		SheetProfile: "А",
		HeaderMap:    `{}`,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create sheet: %v", err)
	}
	for rowNumber := int64(1); rowNumber <= 55; rowNumber++ {
		row, err := queries.CreateImportRow(ctx, storagedb.CreateImportRowParams{
			ImportID:  created.ID,
			SheetName: "А",
			RowNumber: rowNumber,
			RawData:   `{}`,
			CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("create row %d: %v", rowNumber, err)
		}
		if _, err := queries.CreateLegacyImportRow(ctx, storagedb.CreateLegacyImportRowParams{
			ImportRowID:       row.ID,
			SourceFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ExtraFields:       `{}`,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			t.Fatalf("create legacy row %d: %v", rowNumber, err)
		}
	}

	first, err := queries.ListAllLegacyImportRowsPage(ctx, storagedb.ListAllLegacyImportRowsPageParams{
		ImportID:        created.ID,
		AfterSheetOrder: 0,
		AfterRowNumber:  0,
		AfterID:         0,
		PageSize:        50,
	})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(first) != 50 || first[0].RowNumber != 1 || first[49].RowNumber != 50 {
		t.Fatalf("first page length/range = %d, %d..%d", len(first), first[0].RowNumber, first[len(first)-1].RowNumber)
	}
	last := first[len(first)-1]
	second, err := queries.ListAllLegacyImportRowsPage(ctx, storagedb.ListAllLegacyImportRowsPageParams{
		ImportID:        created.ID,
		AfterSheetOrder: last.SheetOrder,
		AfterRowNumber:  last.RowNumber,
		AfterID:         last.ID,
		PageSize:        50,
	})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if len(second) != 5 || second[0].RowNumber != 51 || second[4].RowNumber != 55 {
		t.Fatalf("second page length/range = %d, %d..%d", len(second), second[0].RowNumber, second[len(second)-1].RowNumber)
	}
}

func TestBaselineSupportsLegacyImportStagingGraph(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	now := "2026-07-15T12:00:00Z"

	result, err := database.ExecContext(ctx, `
		INSERT INTO imports (
			profile, source_file_name, source_sha256, source_size_bytes,
			idempotency_key, uploaded_by_actor, received_at, status,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?)
	`,
		"legacy_registry",
		"legacy.xlsx",
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		int64(2048),
		"legacy-import-1",
		"admin",
		now,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	importID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("import id: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO import_sheets (
			import_id, sheet_name, sheet_order, sheet_profile, header_map,
			rows_found, rows_staged, status, created_at, updated_at
		)
		VALUES (?, 'А', 1, 'А', '{}', 1, 1, 'staged', ?, ?)
	`, importID, now, now); err != nil {
		t.Fatalf("create import sheet: %v", err)
	}

	rowResult, err := database.ExecContext(ctx, `
		INSERT INTO import_rows (import_id, sheet_name, row_number, raw_data, created_at)
		VALUES (?, 'А', 2, '{"full_name":"Testov Test"}', ?)
	`, importID, now)
	if err != nil {
		t.Fatalf("create import row: %v", err)
	}
	rowID, err := rowResult.LastInsertId()
	if err != nil {
		t.Fatalf("import row id: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO legacy_import_rows (
			import_row_id, source_fingerprint, employer_name, inn_normalized,
			last_name, first_name, snils_normalized, email_normalized,
			position, department, program_text, protocol_number, protocol_date,
			assessment_result, mintrud_registry_number, source_reference,
			moodle_username, moodle_email, extra_fields, status, version,
			created_at, updated_at
		)
		VALUES (
			?, ?, 'Test Organization', '7700000000', 'Testov', 'Test',
			'12345678900', 'worker@example.test', 'Tester', 'QA', 'Program A',
			'A-1', '2026-07-01', 'passed', 'registry-test-1', 'request-test-1',
			'test-user', 'worker@example.test', '{}', 'needs_review', 1, ?, ?
		)
	`, rowID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, now); err != nil {
		t.Fatalf("create legacy row: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO import_row_issues (
			import_row_id, field, code, severity, message, created_at, updated_at
		)
		VALUES (?, 'program', 'unknown_program', 'blocking', 'Program requires mapping', ?, ?)
	`, rowID, now, now); err != nil {
		t.Fatalf("create import row issue: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO program_groups (code, name, status, created_at, updated_at)
		VALUES ('A', 'Test Group A', 'active', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("create program group: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO programs (
			program_group_id, code, name, default_hours, status, created_at, updated_at
		)
		VALUES (1, 'A-1', 'Test Program A', 16, 'active', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("create program: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO program_aliases (
			profile, sheet_profile, alias_normalized, program_id, created_at, updated_at
		)
		VALUES ('legacy_registry', 'А', 'program a', 1, ?, ?)
	`, now, now); err != nil {
		t.Fatalf("create program alias: %v", err)
	}

	var status string
	var issueCount int64
	if err := database.QueryRowContext(ctx, `
		SELECT legacy_import_rows.status, COUNT(import_row_issues.id)
		FROM legacy_import_rows
		JOIN import_row_issues ON import_row_issues.import_row_id = legacy_import_rows.import_row_id
		WHERE legacy_import_rows.import_row_id = ?
		GROUP BY legacy_import_rows.status
	`, rowID).Scan(&status, &issueCount); err != nil {
		t.Fatalf("read staging graph: %v", err)
	}
	if status != "needs_review" || issueCount != 1 {
		t.Fatalf("staging graph = status %q, issues %d; want needs_review, 1", status, issueCount)
	}
}
