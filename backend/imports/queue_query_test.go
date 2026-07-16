package imports_test

import (
	"context"
	"testing"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestCountActiveImportsIncludesOnlyQueuedAndProcessing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openDatabase(t)
	now := "2026-07-16T12:00:00Z"
	for _, status := range []string{
		"queued",
		"processing",
		"completed",
		"completed_with_issues",
		"failed",
		"cancelled",
	} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO imports (
				profile, uploaded_by_actor, received_at, status, created_at, updated_at
			)
			VALUES ('legacy_registry', 'test-admin', ?, ?, ?, ?)
		`, now, status, now, now); err != nil {
			t.Fatalf("create %s import: %v", status, err)
		}
	}

	count, err := storagedb.New(database).CountActiveImports(ctx)
	if err != nil {
		t.Fatalf("count active imports: %v", err)
	}
	if count != 2 {
		t.Fatalf("active imports = %d, want 2", count)
	}
}
