package imports_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/xuri/excelize/v2"
)

func TestStagerConstructorValidatesDependenciesAndConfig(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	files, err := imports.NewLocalFileStore(filepath.Join(t.TempDir(), "uploads"))
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	valid := imports.DefaultStagingConfig()

	tests := []struct {
		name    string
		db      *sql.DB
		queries *storagedb.Queries
		files   imports.FileStore
		config  imports.StagingConfig
	}{
		{name: "missing database", queries: queries, files: files, config: valid},
		{name: "missing queries", db: database, files: files, config: valid},
		{name: "missing file store", db: database, queries: queries, config: valid},
		{name: "zero batch", db: database, queries: queries, files: files, config: func() imports.StagingConfig {
			config := valid
			config.BatchSize = 0
			return config
		}()},
		{name: "zero lease duration", db: database, queries: queries, files: files, config: func() imports.StagingConfig {
			config := valid
			config.LeaseDuration = 0
			return config
		}()},
		{name: "invalid workbook limits", db: database, queries: queries, files: files, config: func() imports.StagingConfig {
			config := valid
			config.LegacyLimits.MaxRows = 0
			return config
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := imports.NewStager(test.db, test.queries, test.files, test.config); err == nil {
				t.Fatal("NewStager accepted invalid configuration")
			}
		})
	}

	if _, err := imports.NewStager(database, queries, files, valid); err != nil {
		t.Fatalf("NewStager rejected defaults: %v", err)
	}
}

func TestStagerInputRejectsIncompleteOrUnownedImport(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	files, err := imports.NewLocalFileStore(filepath.Join(t.TempDir(), "uploads"))
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	stager, err := imports.NewStager(database, queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})

	valid := storagedb.Import{
		ID:                7,
		Profile:           "legacy_registry",
		SourceSha256:      sql.NullString{String: strings.Repeat("a", 64), Valid: true},
		SourceSizeBytes:   sql.NullInt64{Int64: 1024, Valid: true},
		Status:            "processing",
		Phase:             sql.NullString{String: "parsing", Valid: true},
		TempFileToken:     sql.NullString{String: strings.Repeat("b", 64), Valid: true},
		TempFileExpiresAt: sql.NullString{String: "2026-07-17T12:00:00Z", Valid: true},
		LeaseOwner:        sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt:    sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
	}

	tests := []struct {
		name   string
		mutate func(*storagedb.Import)
	}{
		{name: "zero id", mutate: func(item *storagedb.Import) { item.ID = 0 }},
		{name: "queued status", mutate: func(item *storagedb.Import) { item.Status = "queued" }},
		{name: "wrong profile", mutate: func(item *storagedb.Import) { item.Profile = "client_requests" }},
		{name: "unexpected phase", mutate: func(item *storagedb.Import) { item.Phase = sql.NullString{String: "validating", Valid: true} }},
		{name: "missing owner", mutate: func(item *storagedb.Import) { item.LeaseOwner = sql.NullString{} }},
		{name: "missing digest", mutate: func(item *storagedb.Import) { item.SourceSha256 = sql.NullString{} }},
		{name: "malformed digest", mutate: func(item *storagedb.Import) { item.SourceSha256.String = "not-a-sha256" }},
		{name: "missing size", mutate: func(item *storagedb.Import) { item.SourceSizeBytes = sql.NullInt64{} }},
		{name: "missing token", mutate: func(item *storagedb.Import) { item.TempFileToken = sql.NullString{} }},
		{name: "malformed token", mutate: func(item *storagedb.Import) { item.TempFileToken.String = "../not-a-token" }},
		{name: "missing file expiry", mutate: func(item *storagedb.Import) { item.TempFileExpiresAt = sql.NullString{} }},
		{name: "missing lease expiry", mutate: func(item *storagedb.Import) { item.LeaseExpiresAt = sql.NullString{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := valid
			test.mutate(&item)
			_, err := stager.Stage(context.Background(), item)
			assertStageErrorCode(t, err, imports.CodeStageInvalidInput)
		})
	}
}

func TestStagerInputRejectsMalformedCleanupState(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	files, err := imports.NewLocalFileStore(filepath.Join(t.TempDir(), "uploads"))
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	stager, err := imports.NewStager(database, queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	valid := stagingClaim(imports.StoredFile{
		Token: strings.Repeat("b", 64), SHA256: strings.Repeat("a", 64), Size: 1024,
	})
	valid.Phase = sql.NullString{String: "validating", Valid: true}
	valid.StagedAt = sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true}

	tests := []struct {
		name   string
		mutate func(*storagedb.Import)
	}{
		{name: "malformed lease expiry", mutate: func(item *storagedb.Import) { item.LeaseExpiresAt.String = "invalid" }},
		{name: "malformed token", mutate: func(item *storagedb.Import) { item.TempFileToken.String = "../invalid" }},
		{name: "missing paired expiry", mutate: func(item *storagedb.Import) { item.TempFileExpiresAt = sql.NullString{} }},
		{name: "malformed file expiry", mutate: func(item *storagedb.Import) { item.TempFileExpiresAt.String = "invalid" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := valid
			test.mutate(&item)
			_, err := stager.Stage(context.Background(), item)
			assertStageErrorCode(t, err, imports.CodeStageInvalidInput)
		})
	}
}

func TestStageErrorStringDoesNotExposeWrappedError(t *testing.T) {
	t.Parallel()

	secret := errors.New("token=/private/uploads/secret person@example.test")
	err := &imports.StageError{
		Code:      imports.CodeStagingFailed,
		Retryable: true,
		Sheet:     "А",
		Row:       7,
		Err:       secret,
	}
	message := err.Error()
	if !errors.Is(err, secret) {
		t.Fatal("StageError does not unwrap its cause")
	}
	for _, forbidden := range []string{"/private", "secret", "person@example.test"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("safe error contains %q: %s", forbidden, message)
		}
	}
	for _, required := range []string{"staging_failed", "retryable=true", "sheet=А", "row=7"} {
		if !strings.Contains(message, required) {
			t.Fatalf("safe error misses %q: %s", required, message)
		}
	}
}

func TestStagerRejectsExpiredOrUnavailableSource(t *testing.T) {
	t.Parallel()

	fixture := newStagerFixture(t)
	stored, err := fixture.files.Save(context.Background(), strings.NewReader("synthetic source"), 1024)
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	claimed := stagingClaim(stored)
	claimed.TempFileExpiresAt = sql.NullString{String: "2026-07-16T11:59:59Z", Valid: true}
	_, err = fixture.stager.Stage(context.Background(), claimed)
	stageErr := assertStageErrorCode(t, err, imports.CodeSourceFileUnavailable)
	if stageErr.Retryable {
		t.Fatal("expired source marked retryable")
	}

	claimed = stagingClaim(stored)
	if err := fixture.files.Delete(stored.Token); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	_, err = fixture.stager.Stage(context.Background(), claimed)
	stageErr = assertStageErrorCode(t, err, imports.CodeSourceFileUnavailable)
	if !stageErr.Retryable {
		t.Fatal("temporarily unavailable source marked permanent")
	}
}

func TestStagerRejectsSourceSizeOrDigestMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, imports.StoredFile)
	}{
		{
			name: "size",
			mutate: func(t *testing.T, stored imports.StoredFile) {
				t.Helper()
				if err := os.WriteFile(stored.Path, []byte("different-size"), 0o600); err != nil {
					t.Fatalf("replace source: %v", err)
				}
			},
		},
		{
			name: "digest with same size",
			mutate: func(t *testing.T, stored imports.StoredFile) {
				t.Helper()
				replacement := bytes.Repeat([]byte("x"), int(stored.Size))
				if err := os.WriteFile(stored.Path, replacement, 0o600); err != nil {
					t.Fatalf("replace source: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStagerFixture(t)
			stored, err := fixture.files.Save(context.Background(), strings.NewReader("original-source"), 1024)
			if err != nil {
				t.Fatalf("save source: %v", err)
			}
			test.mutate(t, stored)
			_, err = fixture.stager.Stage(context.Background(), stagingClaim(stored))
			stageErr := assertStageErrorCode(t, err, imports.CodeSourceFileMismatch)
			if stageErr.Retryable {
				t.Fatal("immutable source mismatch marked retryable")
			}
			message := stageErr.Error()
			for _, forbidden := range []string{stored.Token, stored.Path, stored.SHA256, "original-source"} {
				if strings.Contains(message, forbidden) {
					t.Fatalf("safe error contains source metadata: %s", message)
				}
			}
		})
	}
}

func TestStagerSourceVerificationHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newStagerFixture(t)
	stored, err := fixture.files.Save(context.Background(), bytes.NewReader(bytes.Repeat([]byte("z"), 1<<20)), 2<<20)
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = fixture.stager.Stage(ctx, stagingClaim(stored))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestStagerStagesWorkbookAndCleansSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	result, err := enqueue(t, fixture.service, "stage-happy-path", syntheticWorkbook(t, "stage"))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	config := imports.DefaultStagingConfig()
	config.BatchSize = 2
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, config)
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})

	outcome, err := stager.Stage(ctx, claimed)
	if err != nil {
		t.Fatalf("stage workbook: %v", err)
	}
	if outcome.ImportID != claimed.ID || outcome.RowsStaged != 5 || outcome.SheetsStaged != 5 {
		t.Fatalf("outcome = %+v", outcome)
	}
	storedImport, err := fixture.queries.GetImportByID(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get staged import: %v", err)
	}
	if storedImport.Status != "processing" || storedImport.Phase.String != "validating" ||
		!storedImport.StagedAt.Valid || storedImport.RowsTotal != 5 {
		t.Fatalf("staged state = %+v", storedImport)
	}
	if storedImport.TempFileToken.Valid || storedImport.TempFileExpiresAt.Valid {
		t.Fatalf("source metadata survived cleanup: %+v", storedImport)
	}
	if _, err := files.Path(result.Import.TempFileToken.String); err == nil {
		t.Fatal("temporary source survived successful staging")
	}

	for _, sheetName := range []string{"А", "Б", "В", "П", "С"} {
		row, err := fixture.queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
			ImportID:  claimed.ID,
			SheetName: sheetName,
			RowNumber: 2,
		})
		if err != nil {
			t.Fatalf("get row %s: %v", sheetName, err)
		}
		legacyRow, err := fixture.queries.GetLegacyImportRow(ctx, row.ID)
		if err != nil {
			t.Fatalf("get legacy row %s: %v", sheetName, err)
		}
		if legacyRow.SourceFingerprint == "" {
			t.Fatalf("row %s has empty fingerprint", sheetName)
		}
		for _, persisted := range []string{row.RawData, legacyRow.ExtraFields} {
			if strings.Contains(persisted, "synthetic-password") {
				t.Fatalf("row %s persisted password", sheetName)
			}
		}
		sheet, err := fixture.queries.GetImportSheet(ctx, storagedb.GetImportSheetParams{
			ImportID: claimed.ID, SheetName: sheetName,
		})
		if err != nil {
			t.Fatalf("get sheet %s: %v", sheetName, err)
		}
		if sheet.Status != "staged" || sheet.RowsFound != 1 || sheet.RowsStaged != 1 {
			t.Fatalf("sheet %s checkpoint = %+v", sheetName, sheet)
		}
	}
}

func TestStagerRecoversCommittedBatchAfterFailureAndNewLease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	payload := syntheticWorkbookWithRows(t, "recovery", 3)
	_, err := enqueue(t, fixture.service, "stage-recovery", payload)
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	if _, err := fixture.db.ExecContext(ctx, `
		CREATE TRIGGER fail_second_staging_batch
		BEFORE INSERT ON import_rows
		WHEN NEW.sheet_name = 'А' AND NEW.row_number = 4
		BEGIN
			SELECT RAISE(ABORT, 'synthetic staging failure');
		END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	config := imports.DefaultStagingConfig()
	config.BatchSize = 2
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, config)
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	assertStageErrorCode(t, err, imports.CodeStagingStorage)

	for _, rowNumber := range []int64{2, 3} {
		if _, err := fixture.queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
			ImportID: claimed.ID, SheetName: "А", RowNumber: rowNumber,
		}); err != nil {
			t.Fatalf("committed row %d missing: %v", rowNumber, err)
		}
	}
	if _, err := fixture.queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
		ImportID: claimed.ID, SheetName: "А", RowNumber: 4,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("failed batch row lookup = %v, want sql.ErrNoRows", err)
	}
	checkpoint, err := fixture.queries.GetImportSheet(ctx, storagedb.GetImportSheetParams{ImportID: claimed.ID, SheetName: "А"})
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if checkpoint.RowsStaged != 2 {
		t.Fatalf("checkpoint rows = %d, want 2", checkpoint.RowsStaged)
	}
	storedImport, err := fixture.queries.GetImportByID(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get interrupted import: %v", err)
	}
	if storedImport.RowsTotal != 2 || storedImport.Phase.String != "staging" {
		t.Fatalf("interrupted state = %+v", storedImport)
	}
	if _, err := fixture.db.ExecContext(ctx, `DROP TRIGGER fail_second_staging_batch`); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}

	reclaimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-b", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:05:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:03:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("reclaim import: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 3, 0, 0, time.UTC)
	})
	outcome, err := stager.Stage(ctx, reclaimed)
	if err != nil {
		t.Fatalf("resume staging: %v", err)
	}
	if outcome.RowsStaged != 15 {
		t.Fatalf("resumed rows = %d, want 15", outcome.RowsStaged)
	}
	for _, rowNumber := range []int64{2, 3, 4} {
		if _, err := fixture.queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
			ImportID: claimed.ID, SheetName: "А", RowNumber: rowNumber,
		}); err != nil {
			t.Fatalf("resumed row %d missing: %v", rowNumber, err)
		}
	}
}

func TestStagerRejectsPreexistingCoordinateWithoutCheckpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	_, err := enqueue(t, fixture.service, "stage-corrupt", syntheticWorkbook(t, "corrupt"))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	row, err := fixture.queries.CreateImportRow(ctx, storagedb.CreateImportRowParams{
		ImportID: claimed.ID, SheetName: "А", RowNumber: 2,
		RawData: `{"safe":"orphaned-checkpoint"}`, CreatedAt: "2026-07-16T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("create preexisting coordinate: %v", err)
	}
	if _, err := fixture.queries.CreateLegacyImportRow(ctx, storagedb.CreateLegacyImportRowParams{
		ImportRowID: row.ID, SourceFingerprint: strings.Repeat("c", 64), ExtraFields: "{}",
		CreatedAt: "2026-07-16T12:00:00Z", UpdatedAt: "2026-07-16T12:00:00Z",
	}); err != nil {
		t.Fatalf("create preexisting legacy row: %v", err)
	}
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	stageErr := assertStageErrorCode(t, err, imports.CodeStagingCheckpointCorrupt)
	if stageErr.Retryable {
		t.Fatal("corrupt checkpoint marked retryable")
	}
}

func TestStagerRollsBackWholeBatchWhenLeaseChangesInsideTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	_, err := enqueue(t, fixture.service, "stage-lost-lease", syntheticWorkbookWithRows(t, "lease", 2))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	if _, err := fixture.db.ExecContext(ctx, `
		CREATE TRIGGER change_staging_owner
		AFTER INSERT ON import_rows
		WHEN NEW.sheet_name = 'А' AND NEW.row_number = 2
		BEGIN
			UPDATE imports SET lease_owner = 'worker-b' WHERE id = NEW.import_id;
		END
	`); err != nil {
		t.Fatalf("create lease trigger: %v", err)
	}
	config := imports.DefaultStagingConfig()
	config.BatchSize = 2
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, config)
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	assertStageErrorCode(t, err, imports.CodeStageLeaseLost)
	if _, err := fixture.queries.GetImportRowByCoordinate(ctx, storagedb.GetImportRowByCoordinateParams{
		ImportID: claimed.ID, SheetName: "А", RowNumber: 2,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("lost-lease batch row lookup = %v, want sql.ErrNoRows", err)
	}
	storedImport, err := fixture.queries.GetImportByID(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get import after rollback: %v", err)
	}
	if storedImport.RowsTotal != 0 || storedImport.LeaseOwner.String != "worker-a" {
		t.Fatalf("lost-lease batch was not fully rolled back: %+v", storedImport)
	}
}

func TestStagerRetriesCleanupWithoutReopeningWorkbook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	result, err := enqueue(t, fixture.service, "stage-cleanup", syntheticWorkbook(t, "cleanup"))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	failingFiles := &failOnceDeleteStore{FileStore: files}
	stager, err := imports.NewStager(fixture.db, fixture.queries, failingFiles, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	assertStageErrorCode(t, err, imports.CodeSourceCleanupFailed)
	staged, err := fixture.queries.GetImportByID(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get import after cleanup failure: %v", err)
	}
	if staged.Phase.String != "validating" || !staged.StagedAt.Valid || !staged.TempFileToken.Valid {
		t.Fatalf("cleanup failure state = %+v", staged)
	}
	path, err := files.Path(result.Import.TempFileToken.String)
	if err != nil {
		t.Fatalf("resolve source before cleanup retry: %v", err)
	}
	if err := os.WriteFile(path, []byte("not an xlsx and wrong digest"), 0o600); err != nil {
		t.Fatalf("corrupt source before cleanup retry: %v", err)
	}
	if err := files.Delete(result.Import.TempFileToken.String); err != nil {
		t.Fatalf("remove source before cleanup retry: %v", err)
	}
	outcome, err := stager.Stage(ctx, staged)
	if err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if outcome.ImportID != claimed.ID || outcome.RowsStaged != 5 {
		t.Fatalf("cleanup outcome = %+v", outcome)
	}
	cleaned, err := fixture.queries.GetImportByID(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get cleaned import: %v", err)
	}
	if cleaned.TempFileToken.Valid || cleaned.TempFileExpiresAt.Valid {
		t.Fatalf("cleanup retry retained source metadata: %+v", cleaned)
	}
}

func TestStagerRejectsIncompatibleStoredPlanWithoutExposingIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	_, err := enqueue(t, fixture.service, "stage-plan", syntheticWorkbook(t, "plan"))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	const sensitivePlan = `{"version":2,"private":"person@example.test"}`
	if _, err := fixture.db.ExecContext(ctx, `UPDATE import_sheets SET header_map = ? WHERE import_id = ? AND sheet_name = 'А'`, sensitivePlan, claimed.ID); err != nil {
		t.Fatalf("corrupt stored plan: %v", err)
	}
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	stageErr := assertStageErrorCode(t, err, imports.CodeStagingPlanIncompatible)
	if stageErr.Retryable || strings.Contains(stageErr.Error(), "person@example.test") {
		t.Fatalf("unsafe plan error: %s", stageErr.Error())
	}
}

func TestStagerRejectsFailedSheetCheckpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	_, err := enqueue(t, fixture.service, "stage-failed-sheet", syntheticWorkbook(t, "failed-sheet"))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	if _, err := fixture.db.ExecContext(ctx, `UPDATE import_sheets SET status = 'failed' WHERE import_id = ? AND sheet_name = 'А'`, claimed.ID); err != nil {
		t.Fatalf("mark sheet failed: %v", err)
	}
	stager, err := imports.NewStager(fixture.db, fixture.queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	assertStageErrorCode(t, err, imports.CodeStagingCheckpointCorrupt)
}

func TestStagerCancellationKeepsOnlyCommittedBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture := newServiceFixture(t, imports.DefaultConfig())
	files := openFixtureFileStore(t, fixture)
	_, err := enqueue(t, fixture.service, "stage-cancel", syntheticWorkbookWithRows(t, "cancel", 1100))
	if err != nil {
		t.Fatalf("enqueue workbook: %v", err)
	}
	claimed, err := fixture.queries.ClaimNextImport(ctx, storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim import: %v", err)
	}
	const batchSize = 1000
	observed := make(chan error, 1)
	watchingFiles := &cancelAtCheckpointStore{
		FileStore: files,
		database:  fixture.db,
		importID:  claimed.ID,
		rows:      batchSize,
		cancel:    cancel,
		observed:  observed,
	}
	config := imports.DefaultStagingConfig()
	config.BatchSize = batchSize
	stager, err := imports.NewStager(fixture.db, fixture.queries, watchingFiles, config)
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	_, err = stager.Stage(ctx, claimed)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stage error = %v, want context.Canceled", err)
	}
	if monitorErr := <-observed; monitorErr != nil {
		t.Fatalf("observe committed checkpoint: %v", monitorErr)
	}
	storedImport, err := fixture.queries.GetImportByID(context.Background(), claimed.ID)
	if err != nil {
		t.Fatalf("get cancelled import: %v", err)
	}
	if storedImport.RowsTotal != batchSize || storedImport.Phase.String != "staging" || !storedImport.TempFileToken.Valid {
		t.Fatalf("cancelled state = %+v", storedImport)
	}
	sheet, err := fixture.queries.GetImportSheet(context.Background(), storagedb.GetImportSheetParams{
		ImportID: claimed.ID, SheetName: "А",
	})
	if err != nil {
		t.Fatalf("get cancelled sheet: %v", err)
	}
	if sheet.RowsStaged != batchSize {
		t.Fatalf("cancelled checkpoint = %+v", sheet)
	}
	if _, err := fixture.queries.GetImportRowByCoordinate(context.Background(), storagedb.GetImportRowByCoordinateParams{
		ImportID: claimed.ID, SheetName: "А", RowNumber: batchSize + 1,
	}); err != nil {
		t.Fatalf("last committed row missing: %v", err)
	}
	if _, err := fixture.queries.GetImportRowByCoordinate(context.Background(), storagedb.GetImportRowByCoordinateParams{
		ImportID: claimed.ID, SheetName: "А", RowNumber: batchSize + 2,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("uncommitted row lookup = %v, want sql.ErrNoRows", err)
	}
}

type failOnceDeleteStore struct {
	imports.FileStore
	mu     sync.Mutex
	failed bool
}

type cancelAtCheckpointStore struct {
	imports.FileStore
	database *sql.DB
	importID int64
	rows     int64
	cancel   context.CancelFunc
	observed chan<- error
	once     sync.Once
}

func (s *cancelAtCheckpointStore) Path(token string) (string, error) {
	path, err := s.FileStore.Path(token)
	if err != nil {
		return "", err
	}
	s.once.Do(func() {
		go func() {
			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-deadline.C:
					s.observed <- errors.New("timed out waiting for staging checkpoint")
					return
				case <-ticker.C:
					var rows int64
					err := s.database.QueryRow(`SELECT rows_total FROM imports WHERE id = ?`, s.importID).Scan(&rows)
					if err != nil {
						s.observed <- err
						return
					}
					if rows >= s.rows {
						s.cancel()
						s.observed <- nil
						return
					}
				}
			}
		}()
	})
	return path, nil
}

func (s *failOnceDeleteStore) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return errors.New("synthetic cleanup failure")
	}
	return s.FileStore.Delete(token)
}

func syntheticWorkbookWithRows(t *testing.T, discriminator string, rowsPerSheet int) []byte {
	t.Helper()
	payload := syntheticWorkbook(t, discriminator)
	workbook, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("open synthetic workbook: %v", err)
	}
	defer workbook.Close()
	for _, sheetName := range []string{"А", "Б", "В", "П", "С"} {
		rows, err := workbook.GetRows(sheetName)
		if err != nil || len(rows) < 2 {
			t.Fatalf("read synthetic sheet %s: %v", sheetName, err)
		}
		for rowNumber := 3; rowNumber <= rowsPerSheet+1; rowNumber++ {
			for column, value := range rows[1] {
				if column == 0 {
					value += " row " + strconv.Itoa(rowNumber)
				}
				cell, err := excelize.CoordinatesToCellName(column+1, rowNumber)
				if err != nil {
					t.Fatalf("row coordinates: %v", err)
				}
				if err := workbook.SetCellValue(sheetName, cell, value); err != nil {
					t.Fatalf("set synthetic row: %v", err)
				}
			}
		}
	}
	var output bytes.Buffer
	if err := workbook.Write(&output); err != nil {
		t.Fatalf("write synthetic workbook: %v", err)
	}
	return output.Bytes()
}

type stagerFixture struct {
	stager   *imports.Stager
	files    *imports.LocalFileStore
	database *sql.DB
	queries  *storagedb.Queries
}

func newStagerFixture(t *testing.T) stagerFixture {
	t.Helper()
	database := openDatabase(t)
	queries := storagedb.New(database)
	files, err := imports.NewLocalFileStore(filepath.Join(t.TempDir(), "uploads"))
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	stager, err := imports.NewStager(database, queries, files, imports.DefaultStagingConfig())
	if err != nil {
		t.Fatalf("create stager: %v", err)
	}
	stager.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	return stagerFixture{stager: stager, files: files, database: database, queries: queries}
}

func openFixtureFileStore(t *testing.T, fixture serviceFixture) *imports.LocalFileStore {
	t.Helper()
	files, err := imports.NewLocalFileStore(fixture.root)
	if err != nil {
		t.Fatalf("open fixture file store: %v", err)
	}
	return files
}

func stagingClaim(stored imports.StoredFile) storagedb.Import {
	return storagedb.Import{
		ID:                7,
		Profile:           "legacy_registry",
		SourceSha256:      sql.NullString{String: stored.SHA256, Valid: true},
		SourceSizeBytes:   sql.NullInt64{Int64: stored.Size, Valid: true},
		Status:            "processing",
		Phase:             sql.NullString{String: "parsing", Valid: true},
		TempFileToken:     sql.NullString{String: stored.Token, Valid: true},
		TempFileExpiresAt: sql.NullString{String: "2026-07-17T12:00:00Z", Valid: true},
		LeaseOwner:        sql.NullString{String: "worker-a", Valid: true},
		LeaseExpiresAt:    sql.NullString{String: "2026-07-16T12:02:00Z", Valid: true},
	}
}

func assertStageErrorCode(t *testing.T, err error, code imports.StageErrorCode) *imports.StageError {
	t.Helper()
	var stageErr *imports.StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("error = %v, want StageError", err)
	}
	if stageErr.Code != code {
		t.Fatalf("code = %q, want %q", stageErr.Code, code)
	}
	return stageErr
}
