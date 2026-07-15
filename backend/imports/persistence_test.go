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
