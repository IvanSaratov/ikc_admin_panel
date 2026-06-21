package audit_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestRecord_PersistsAllFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newQueries(t)
	service := audit.NewService(queries.Queries)

	entityID := sql.NullInt64{Int64: 42, Valid: true}

	err := service.Record(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "program_group",
		Actor:      "operator_unidentified",
		EntityID:   entityID,
		Details: map[string]any{
			"code": "A",
			"name": "Охрана труда",
		},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	var row storagedb.ActionLog
	err = queries.DB.QueryRowContext(ctx, `
		SELECT id, actor, action, entity_type, entity_id, details, created_at
		FROM action_log
	`).Scan(
		&row.ID,
		&row.Actor,
		&row.Action,
		&row.EntityType,
		&row.EntityID,
		&row.Details,
		&row.CreatedAt,
	)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if row.Actor != "operator_unidentified" {
		t.Errorf("actor = %q, want operator_unidentified", row.Actor)
	}
	if row.Action != "create" {
		t.Errorf("action = %q, want create", row.Action)
	}
	if row.EntityType != "program_group" {
		t.Errorf("entity_type = %q, want program_group", row.EntityType)
	}
	if !row.EntityID.Valid || row.EntityID.Int64 != 42 {
		t.Errorf("entity_id = %+v, want valid 42", row.EntityID)
	}
	if !row.Details.Valid {
		t.Fatalf("details is NULL")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(row.Details.String), &decoded); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	if decoded["code"] != "A" {
		t.Errorf("details.code = %v, want A", decoded["code"])
	}
	if decoded["name"] != "Охрана труда" {
		t.Errorf("details.name = %v, want Охрана труда", decoded["name"])
	}
}

func TestRecord_NilEntityID_Allowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newQueries(t)
	service := audit.NewService(queries.Queries)

	err := service.Record(ctx, audit.RecordInput{
		Action:     "login",
		EntityType: "session",
		Actor:      "system",
		EntityID:   sql.NullInt64{}, // explicit zero value, !Valid
		Details: map[string]any{
			"ip": "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	var entityID sql.NullInt64
	var details sql.NullString
	err = queries.DB.QueryRowContext(ctx, `
		SELECT entity_id, details
		FROM action_log
	`).Scan(&entityID, &details)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if entityID.Valid {
		t.Errorf("entity_id.Valid = true, want false (nil)")
	}
	if !details.Valid {
		t.Errorf("details.Valid = false, want true")
	}
}

func TestRecord_DetailsJSONSerialized(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newQueries(t)
	service := audit.NewService(queries.Queries)

	err := service.Record(ctx, audit.RecordInput{
		Action:     "update",
		EntityType: "employer",
		Actor:      "operator_unidentified",
		EntityID:   sql.NullInt64{Int64: 7, Valid: true},
		Details: map[string]any{
			"from": map[string]any{
				"inn": "7700000000",
			},
			"to": map[string]any{
				"inn": "7700111111",
			},
		},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	var details sql.NullString
	err = queries.DB.QueryRowContext(ctx, `SELECT details FROM action_log`).Scan(&details)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !details.Valid {
		t.Fatalf("details is NULL")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(details.String), &decoded); err != nil {
		t.Fatalf("decode details as JSON: %v", err)
	}

	from, ok := decoded["from"].(map[string]any)
	if !ok {
		t.Fatalf("details.from is not an object: %T", decoded["from"])
	}
	if from["inn"] != "7700000000" {
		t.Errorf("details.from.inn = %v, want 7700000000", from["inn"])
	}

	to, ok := decoded["to"].(map[string]any)
	if !ok {
		t.Fatalf("details.to is not an object: %T", decoded["to"])
	}
	if to["inn"] != "7700111111" {
		t.Errorf("details.to.inn = %v, want 7700111111", to["inn"])
	}
}

func TestRecord_DefaultsActorWhenEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newQueries(t)
	service := audit.NewService(queries.Queries)

	err := service.Record(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "program",
		EntityID:   sql.NullInt64{Int64: 1, Valid: true},
		// Actor intentionally empty.
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	var actor string
	err = queries.DB.QueryRowContext(ctx, `SELECT actor FROM action_log`).Scan(&actor)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if actor != "operator_unidentified" {
		t.Errorf("actor = %q, want operator_unidentified", actor)
	}
}

// queriesWithDB exposes the underlying *sql.DB so tests can read back rows
// without coupling to generated query methods.
type queriesWithDB struct {
	*storagedb.Queries
	DB *sql.DB
}

func newQueries(t *testing.T) *queriesWithDB {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "mintrud-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return &queriesWithDB{
		Queries: storagedb.New(db),
		DB:      db,
	}
}
