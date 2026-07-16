package imports_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func seedReadableImports(t *testing.T, queries *storagedb.Queries, count int) {
	t.Helper()
	now := "2026-07-16T12:00:00Z"
	for index := 1; index <= count; index++ {
		if _, err := queries.CreateImport(context.Background(), storagedb.CreateImportParams{
			Profile:         "legacy_registry",
			SourceFileName:  sql.NullString{String: fmt.Sprintf("fixture-%d.xlsx", index), Valid: true},
			IdempotencyKey:  sql.NullString{String: fmt.Sprintf("read-service-%d", index), Valid: true},
			UploadedByActor: "test-admin",
			ReceivedAt:      now,
			Status:          "queued",
			CreatedAt:       now,
			UpdatedAt:       now,
		}); err != nil {
			t.Fatalf("create import %d: %v", index, err)
		}
	}
}

func TestReadServiceUsesDefaultLimitAndStableCursor(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	seedReadableImports(t, queries, 55)
	service := imports.NewReadService(queries)

	first, err := service.List(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(first.Items) != 50 || first.NextCursor == "" {
		t.Fatalf("first page items = %d, cursor = %q", len(first.Items), first.NextCursor)
	}
	if first.Items[0].ID != 55 || first.Items[49].ID != 6 {
		t.Fatalf("first page bounds = %d..%d", first.Items[0].ID, first.Items[49].ID)
	}

	second, err := service.List(context.Background(), first.NextCursor, 0)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if len(second.Items) != 5 || second.NextCursor != "" {
		t.Fatalf("second page items = %d, cursor = %q", len(second.Items), second.NextCursor)
	}
	if second.Items[0].ID != 5 || second.Items[4].ID != 1 {
		t.Fatalf("second page bounds = %d..%d", second.Items[0].ID, second.Items[4].ID)
	}
}

func TestReadServiceRejectsInvalidCursorAndLimits(t *testing.T) {
	t.Parallel()

	service := imports.NewReadService(storagedb.New(openDatabase(t)))
	tests := []struct {
		name   string
		cursor string
		limit  int
	}{
		{name: "invalid base64", cursor: "%%%"},
		{name: "wrong version", cursor: "djI6MQ"},
		{name: "zero id", cursor: "djE6MA"},
		{name: "negative id", cursor: "djE6LTE"},
		{name: "trailing data", cursor: "djE6MSA"},
		{name: "negative limit", limit: -1},
		{name: "over max limit", limit: 201},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.List(context.Background(), test.cursor, test.limit)
			assertImportErrorCode(t, err, imports.CodeInvalidInput)
		})
	}
}

func TestReadServiceGetReturnsSafeViewAndNotFound(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	seedReadableImports(t, queries, 1)
	service := imports.NewReadService(queries)

	view, err := service.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("get import: %v", err)
	}
	if view.ID != 1 || view.SourceFileName == nil || *view.SourceFileName != "fixture-1.xlsx" || view.UploadedByActor != "test-admin" {
		t.Fatalf("view = %+v", view)
	}
	if _, err := service.Get(context.Background(), 0); err == nil {
		t.Fatal("nonpositive import ID accepted")
	} else {
		assertImportErrorCode(t, err, imports.CodeInvalidInput)
	}
	if _, err := service.Get(context.Background(), 999); !errors.Is(err, imports.ErrImportNotFound) {
		t.Fatalf("missing import error = %v", err)
	}
}

func TestReadServiceMapsDatabaseFailureToStorageUnavailable(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	service := imports.NewReadService(storagedb.New(database))
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	_, err := service.List(context.Background(), "", 50)
	assertImportErrorCode(t, err, imports.CodeStorageUnavailable)
}
