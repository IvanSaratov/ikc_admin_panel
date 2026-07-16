package imports_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestGeneratedImportAPIQueriesReturnSafeProjectionAndQueuePositions(t *testing.T) {
	t.Parallel()

	database := openDatabase(t)
	queries := storagedb.New(database)
	ctx := context.Background()
	now := "2026-07-16T12:00:00Z"
	statuses := []string{"queued", "processing", "completed", "queued"}
	for index, status := range statuses {
		if _, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
			Profile:         "legacy_registry",
			SourceFileName:  sql.NullString{String: "safe.xlsx", Valid: true},
			SourceSha256:    sql.NullString{String: string(rune('a'+index)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Valid: true},
			IdempotencyKey:  sql.NullString{String: "read-query-" + string(rune('a'+index)), Valid: true},
			UploadedByActor: "test-admin",
			ReceivedAt:      now,
			Status:          status,
			CreatedAt:       now,
			UpdatedAt:       now,
		}); err != nil {
			t.Fatalf("create %s import: %v", status, err)
		}
	}

	rows, err := queries.ListImportsAPI(ctx, storagedb.ListImportsAPIParams{
		BeforeID: 0,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("list imports API: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("row count = %d, want 4", len(rows))
	}
	if rows[0].ID != 4 || rows[0].QueuePosition != 3 {
		t.Fatalf("newest queued row = %+v", rows[0])
	}
	if rows[1].ID != 3 || rows[1].QueuePosition != 0 {
		t.Fatalf("terminal row = %+v", rows[1])
	}
	if rows[2].ID != 2 || rows[2].QueuePosition != 0 {
		t.Fatalf("processing row = %+v", rows[2])
	}
	if rows[3].ID != 1 || rows[3].QueuePosition != 1 {
		t.Fatalf("oldest queued row = %+v", rows[3])
	}

	detail, err := queries.GetImportAPI(ctx, 4)
	if err != nil {
		t.Fatalf("get import API: %v", err)
	}
	if detail.ID != rows[0].ID || detail.QueuePosition != rows[0].QueuePosition {
		t.Fatalf("detail = %+v, list row = %+v", detail, rows[0])
	}

	rowType := reflect.TypeOf(rows[0])
	for _, forbidden := range []string{"SourceSha256", "IdempotencyKey", "TempFileToken", "TempFileExpiresAt", "LeaseOwner"} {
		if _, exists := rowType.FieldByName(forbidden); exists {
			t.Errorf("safe API projection exposes %s", forbidden)
		}
	}
}
