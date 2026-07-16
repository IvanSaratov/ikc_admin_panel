package imports_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/xuri/excelize/v2"
)

var serviceFixtureHeaders = []string{
	"Организация",
	"ИНН",
	"Ф.И.О.",
	"Профессия",
	"Подразделение",
	"СНИЛС",
	"Номер протокола",
	"Дата протокола",
	"Оценка",
	"Период обучения",
	"Номер в реестре",
	"Организация/филиал/№ заявки",
	"Email",
	"Логин",
	"Пароль",
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("body must not be read")
}

type serviceFixture struct {
	service *imports.Service
	db      *sql.DB
	root    string
	queries *storagedb.Queries
}

func newServiceFixture(t *testing.T, config imports.Config) serviceFixture {
	t.Helper()

	database := openDatabase(t)
	root := filepath.Join(t.TempDir(), "uploads")
	files, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	queries := storagedb.New(database)
	service, err := imports.NewService(database, queries, nil, files, config)
	if err != nil {
		t.Fatalf("create import service: %v", err)
	}
	service.SetClock(func() time.Time {
		return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	})
	return serviceFixture{service: service, db: database, root: root, queries: queries}
}

func syntheticWorkbook(t *testing.T, discriminator string) []byte {
	t.Helper()

	workbook := excelize.NewFile()
	t.Cleanup(func() { _ = workbook.Close() })
	profiles := []string{"А", "Б", "В", "П", "С"}
	if err := workbook.SetSheetName("Sheet1", profiles[0]); err != nil {
		t.Fatalf("rename first sheet: %v", err)
	}
	for index, profile := range profiles {
		if index > 0 {
			if _, err := workbook.NewSheet(profile); err != nil {
				t.Fatalf("create sheet %s: %v", profile, err)
			}
		}
		headers := append([]string(nil), serviceFixtureHeaders...)
		row := []any{
			"Test Organization " + discriminator,
			"7700000000",
			"Testov Test Testovich",
			"Tester",
			"Test Department",
			"12345678900",
			"TEST-" + discriminator,
			"2026-07-16",
			"passed",
			"01.07.2026-16.07.2026",
			"registry-test-" + discriminator,
			"branch-test-" + discriminator,
			"worker@example.test",
			"test-user",
			"synthetic-password",
		}
		if profile == "В" {
			headers = append(headers, "Программа обучения")
			row = append(row, "Test Program")
		}
		for column, header := range headers {
			cell, err := excelize.CoordinatesToCellName(column+1, 1)
			if err != nil {
				t.Fatalf("header coordinates: %v", err)
			}
			if err := workbook.SetCellValue(profile, cell, header); err != nil {
				t.Fatalf("set header: %v", err)
			}
		}
		for column, value := range row {
			cell, err := excelize.CoordinatesToCellName(column+1, 2)
			if err != nil {
				t.Fatalf("row coordinates: %v", err)
			}
			if err := workbook.SetCellValue(profile, cell, value); err != nil {
				t.Fatalf("set row: %v", err)
			}
		}
	}

	var output bytes.Buffer
	if err := workbook.Write(&output); err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	return output.Bytes()
}

func unsupportedWorkbook(t *testing.T) []byte {
	t.Helper()

	workbook := excelize.NewFile()
	t.Cleanup(func() { _ = workbook.Close() })
	if err := workbook.SetSheetName("Sheet1", "А"); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for column, header := range serviceFixtureHeaders {
		cell, err := excelize.CoordinatesToCellName(column+1, 1)
		if err != nil {
			t.Fatalf("header coordinates: %v", err)
		}
		if err := workbook.SetCellValue("А", cell, header); err != nil {
			t.Fatalf("set header: %v", err)
		}
	}
	var output bytes.Buffer
	if err := workbook.Write(&output); err != nil {
		t.Fatalf("write unsupported workbook: %v", err)
	}
	return output.Bytes()
}

func enqueue(t *testing.T, service *imports.Service, key string, body []byte) (imports.EnqueueResult, error) {
	t.Helper()
	return service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
		OriginalFileName: "legacy.xlsx",
		IdempotencyKey:   key,
		Actor:            "test-admin",
		Body:             bytes.NewReader(body),
	})
}

func TestEnqueueLegacyValidatesBeforeWriting(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	tests := []struct {
		name  string
		input imports.EnqueueInput
	}{
		{name: "missing body", input: imports.EnqueueInput{Actor: "test-admin"}},
		{name: "missing actor", input: imports.EnqueueInput{Body: strings.NewReader("unused")}},
		{
			name: "invalid idempotency key before body read",
			input: imports.EnqueueInput{
				Actor:          "test-admin",
				IdempotencyKey: "bad\nkey",
				Body:           panicReader{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixture.service.EnqueueLegacy(context.Background(), test.input)
			assertImportErrorCode(t, err, imports.CodeInvalidInput)
		})
	}
	assertDirectoryEmpty(t, fixture.root)
}

func TestEnqueueLegacyPersistsQueuedImportAndSafeSheetPlans(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	result, err := fixture.service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
		OriginalFileName: "../private\\industrial.xlsx",
		IdempotencyKey:   "request-1",
		Actor:            "test-admin",
		Body:             bytes.NewReader(syntheticWorkbook(t, "one")),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if result.Reused || result.QueuePosition != 1 || result.Import.Status != "queued" {
		t.Fatalf("enqueue result = %+v", result)
	}
	if result.Import.SourceFileName.String != "industrial.xlsx" {
		t.Fatalf("stored source name = %q", result.Import.SourceFileName.String)
	}
	if result.Import.SourceSha256.String == "" || result.Import.SourceSizeBytes.Int64 == 0 {
		t.Fatalf("missing source metadata: %+v", result.Import)
	}
	if result.Import.TempFileToken.String == "" {
		t.Fatal("missing opaque temporary file token")
	}
	if _, err := os.Stat(filepath.Join(fixture.root, result.Import.TempFileToken.String+".upload")); err != nil {
		t.Fatalf("stored upload: %v", err)
	}

	sheets, err := fixture.queries.ListImportSheets(context.Background(), result.Import.ID)
	if err != nil {
		t.Fatalf("list import sheets: %v", err)
	}
	if len(sheets) != 5 {
		t.Fatalf("sheet count = %d, want 5", len(sheets))
	}
	workbookPlan := legacy.WorkbookPlan{Sheets: make([]legacy.SheetPlan, 0, len(sheets))}
	for _, sheet := range sheets {
		plan, err := legacy.DecodeSheetPlan(
			sheet.SheetName,
			int(sheet.SheetOrder),
			legacy.SheetProfile(sheet.SheetProfile),
			sheet.HeaderMap,
		)
		if err != nil {
			t.Fatalf("decode sheet plan %s: %v", sheet.SheetName, err)
		}
		if plan.HeaderRow != 1 || len(plan.HeaderMap) < 14 || plan.HeaderMap[0] != legacy.FieldEmployer {
			t.Fatalf("unsafe or incomplete sheet plan %s: %+v", sheet.SheetName, plan)
		}
		workbookPlan.Sheets = append(workbookPlan.Sheets, plan)
		if strings.Contains(strings.ToLower(sheet.HeaderMap), "password") || strings.Contains(sheet.HeaderMap, "Пароль") {
			t.Fatalf("secret header persisted for sheet %s", sheet.SheetName)
		}
		for _, forbidden := range []string{"synthetic-password", "Test Organization one", "Testov Test Testovich"} {
			if strings.Contains(sheet.HeaderMap, forbidden) {
				t.Fatalf("row data persisted in sheet plan %s", sheet.SheetName)
			}
		}
	}

	var restoredRows []legacy.SourceRow
	_, err = legacy.Parse(
		context.Background(),
		filepath.Join(fixture.root, result.Import.TempFileToken.String+".upload"),
		workbookPlan,
		legacy.DefaultLimits(),
		func(_ context.Context, row legacy.SourceRow) error {
			restoredRows = append(restoredRows, row)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("parse with restored sheet plans: %v", err)
	}
	restoredJSON, err := json.Marshal(restoredRows)
	if err != nil {
		t.Fatalf("encode restored rows: %v", err)
	}
	if strings.Contains(strings.ToLower(string(restoredJSON)), "synthetic-password") {
		t.Fatalf("restored sheet plans exposed password: %s", restoredJSON)
	}
}

func TestEnqueueLegacyRejectsInvalidAndOversizedFilesWithoutResidue(t *testing.T) {
	t.Parallel()

	config := imports.DefaultConfig()
	config.LegacyLimits.MaxFileBytes = 64
	fixture := newServiceFixture(t, config)
	_, err := enqueue(t, fixture.service, "oversized", bytes.Repeat([]byte("x"), 65))
	assertImportErrorCode(t, err, imports.CodeFileTooLarge)
	assertDirectoryEmpty(t, fixture.root)

	config.LegacyLimits.MaxFileBytes = 1 << 20
	fixture = newServiceFixture(t, config)
	_, err = enqueue(t, fixture.service, "invalid-xlsx", []byte("not a workbook"))
	assertImportErrorCode(t, err, imports.CodeNotXLSX)
	assertDirectoryEmpty(t, fixture.root)

	fixture = newServiceFixture(t, imports.DefaultConfig())
	_, err = enqueue(t, fixture.service, "unsupported", unsupportedWorkbook(t))
	assertImportErrorCode(t, err, imports.CodeUnsupportedWorkbook)
	assertDirectoryEmpty(t, fixture.root)
}

func TestEnqueueLegacyCleansStoredFileWhenDatabaseFails(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	if err := fixture.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	_, err := fixture.service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
		OriginalFileName: "legacy.xlsx",
		Actor:            "test-admin",
		Body:             bytes.NewReader(syntheticWorkbook(t, "db-failure")),
	})
	assertImportErrorCode(t, err, imports.CodeStorageUnavailable)
	assertDirectoryEmpty(t, fixture.root)
}

func TestEnqueueLegacyIdempotencyDoesNotReadSecondBody(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	first, err := enqueue(t, fixture.service, "same-request", syntheticWorkbook(t, "same"))
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := fixture.service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
		OriginalFileName: "ignored.xlsx",
		IdempotencyKey:   "same-request",
		Actor:            "test-admin",
		Body:             panicReader{},
	})
	if err != nil {
		t.Fatalf("idempotent enqueue: %v", err)
	}
	if !second.Reused || second.Import.ID != first.Import.ID || second.QueuePosition != 1 {
		t.Fatalf("idempotent result = %+v, first = %+v", second, first)
	}
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("read upload directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stored files = %d, want 1", len(entries))
	}
}

func TestEnqueueLegacyRejectsIdempotencyKeyOwnedByOtherProfileWithoutReadingBody(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	now := "2026-07-16T12:00:00Z"
	existing, err := fixture.queries.CreateImport(context.Background(), storagedb.CreateImportParams{
		Profile:         "client_request",
		IdempotencyKey:  sql.NullString{String: "shared-key", Valid: true},
		UploadedByActor: "test-admin",
		ReceivedAt:      now,
		Status:          "queued",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create client request import: %v", err)
	}
	_, err = fixture.service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
		OriginalFileName: "ignored.xlsx",
		IdempotencyKey:   "shared-key",
		Actor:            "test-admin",
		Body:             panicReader{},
	})
	var serviceErr *imports.ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != imports.CodeIdempotencyConflict || serviceErr.ExistingImportID != existing.ID {
		t.Fatalf("idempotency conflict = %#v", err)
	}
	assertDirectoryEmpty(t, fixture.root)
}

func TestEnqueueLegacyRejectsActiveDuplicateSHA(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	body := syntheticWorkbook(t, "duplicate")
	first, err := enqueue(t, fixture.service, "request-a", body)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	_, err = enqueue(t, fixture.service, "request-b", body)
	var serviceErr *imports.ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != imports.CodeDuplicateFile || serviceErr.ExistingImportID != first.Import.ID {
		t.Fatalf("duplicate error = %#v", err)
	}
	entries, readErr := os.ReadDir(fixture.root)
	if readErr != nil {
		t.Fatalf("read upload directory: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("stored files = %d, want 1", len(entries))
	}
}

func TestEnqueueLegacyAllowsSHAAfterFailedImport(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	body := syntheticWorkbook(t, "retry")
	first, err := enqueue(t, fixture.service, "failed-request", body)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := fixture.db.ExecContext(context.Background(), `
		UPDATE imports SET status = 'failed', updated_at = ? WHERE id = ?
	`, "2026-07-16T12:01:00Z", first.Import.ID); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	second, err := enqueue(t, fixture.service, "retry-request", body)
	if err != nil {
		t.Fatalf("retry enqueue: %v", err)
	}
	if second.Import.ID == first.Import.ID || second.QueuePosition != 1 {
		t.Fatalf("retry result = %+v", second)
	}
}

func TestEnqueueLegacyUsesBoundedFIFOQueue(t *testing.T) {
	t.Parallel()

	config := imports.DefaultConfig()
	config.ActiveQueueLimit = 2
	fixture := newServiceFixture(t, config)
	first, err := enqueue(t, fixture.service, "queue-1", syntheticWorkbook(t, "queue-1"))
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := enqueue(t, fixture.service, "queue-2", syntheticWorkbook(t, "queue-2"))
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.QueuePosition != 1 || second.QueuePosition != 2 {
		t.Fatalf("queue positions = %d, %d", first.QueuePosition, second.QueuePosition)
	}
	_, err = enqueue(t, fixture.service, "queue-3", syntheticWorkbook(t, "queue-3"))
	assertImportErrorCode(t, err, imports.CodeQueueFull)

	claimed, err := fixture.queries.ClaimNextImport(context.Background(), storagedb.ClaimNextImportParams{
		LeaseOwner:     sql.NullString{String: "test-worker", Valid: true},
		LeaseExpiresAt: sql.NullString{String: "2026-07-16T12:05:00Z", Valid: true},
		Now:            sql.NullString{String: "2026-07-16T12:00:00Z", Valid: true},
	})
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed.ID != first.Import.ID {
		t.Fatalf("claimed import = %d, want first %d", claimed.ID, first.Import.ID)
	}
}

func TestEnqueueLegacyConcurrentSameKeyCreatesOneImportAndFile(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, imports.DefaultConfig())
	body := syntheticWorkbook(t, "concurrent")
	start := make(chan struct{})
	results := make(chan imports.EnqueueResult, 2)
	errorsChannel := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, err := enqueue(t, fixture.service, "concurrent-key", body)
			results <- result
			errorsChannel <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent enqueue: %v", err)
		}
	}
	var created, reused int
	var importID int64
	for result := range results {
		if result.Reused {
			reused++
		} else {
			created++
		}
		if importID == 0 {
			importID = result.Import.ID
		} else if result.Import.ID != importID {
			t.Fatalf("different import IDs: %d and %d", importID, result.Import.ID)
		}
	}
	if created != 1 || reused != 1 {
		t.Fatalf("created = %d, reused = %d", created, reused)
	}
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("read upload directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stored files = %d, want 1", len(entries))
	}
}

func TestEnqueueLegacyConcurrentUploadsDoNotExceedQueueLimit(t *testing.T) {
	t.Parallel()

	config := imports.DefaultConfig()
	config.ActiveQueueLimit = 1
	fixture := newServiceFixture(t, config)
	bodies := [][]byte{
		syntheticWorkbook(t, "capacity-a"),
		syntheticWorkbook(t, "capacity-b"),
	}
	start := make(chan struct{})
	errorsChannel := make(chan error, 2)
	var workers sync.WaitGroup
	for index := range bodies {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			_, err := fixture.service.EnqueueLegacy(context.Background(), imports.EnqueueInput{
				OriginalFileName: "legacy.xlsx",
				IdempotencyKey:   fmt.Sprintf("capacity-%d", index),
				Actor:            "test-admin",
				Body:             bytes.NewReader(bodies[index]),
			})
			errorsChannel <- err
		}(index)
	}
	close(start)
	workers.Wait()
	close(errorsChannel)

	var succeeded, queueFull int
	for err := range errorsChannel {
		if err == nil {
			succeeded++
			continue
		}
		var serviceErr *imports.ServiceError
		if errors.As(err, &serviceErr) && serviceErr.Code == imports.CodeQueueFull {
			queueFull++
			continue
		}
		t.Fatalf("unexpected concurrent enqueue error: %v", err)
	}
	if succeeded != 1 || queueFull != 1 {
		t.Fatalf("succeeded = %d, queue full = %d", succeeded, queueFull)
	}
	active, err := fixture.queries.CountActiveImports(context.Background())
	if err != nil {
		t.Fatalf("count active imports: %v", err)
	}
	if active != 1 {
		t.Fatalf("active imports = %d, want 1", active)
	}
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("read upload directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stored files = %d, want 1", len(entries))
	}
}

func TestEnqueueLegacyWritesSafeBestEffortAuditRecord(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	root := filepath.Join(t.TempDir(), "uploads")
	files, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}
	service, err := imports.NewService(database, queries, audit.NewService(queries), files, imports.DefaultConfig())
	if err != nil {
		t.Fatalf("create import service: %v", err)
	}
	result, err := enqueue(t, service, "audit-request", syntheticWorkbook(t, "audit"))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	logs, err := queries.ListActionLogsByEntity(context.Background(), storagedb.ListActionLogsByEntityParams{
		EntityType: "import",
		EntityID:   sql.NullInt64{Int64: result.Import.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("list action logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Actor != "test-admin" || logs[0].Action != "enqueue" {
		t.Fatalf("audit logs = %+v", logs)
	}
	for _, forbidden := range []string{"legacy.xlsx", result.Import.SourceSha256.String, result.Import.TempFileToken.String} {
		if forbidden != "" && strings.Contains(logs[0].Details.String, forbidden) {
			t.Fatalf("audit details contain forbidden upload metadata")
		}
	}
}

var _ io.Reader = panicReader{}
