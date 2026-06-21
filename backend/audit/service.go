// Package audit persists manual data changes to the action_log table.
//
// The Service is intentionally small: it only knows how to serialise
// RecordInput into a row. Business services (programs, employers, people)
// call Record after a successful write. After F3 this will be the single
// entry point for login/logout/import/protocol events as well.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// Default actor used when callers leave RecordInput.Actor empty.
const DefaultActor = "operator_unidentified"

// Service writes audit entries through sqlc-generated queries.
//
// It is safe for concurrent use as long as the underlying *sql.DB is.
type Service struct {
	queries *storagedb.Queries
	now     func() time.Time
}

// RecordInput describes a single action_log entry.
//
// Details is serialised to JSON before being persisted. Use EntityID
// with Valid=false for events that do not attach to a row (login, logout,
// import with no affected rows, etc.).
type RecordInput struct {
	Action     string         // "create"|"update"|"deactivate"|"login"|"logout"|"import"
	EntityType string         // "program_group"|"program"|"employer"|"worker"|"worker_employer"|...
	EntityID   sql.NullInt64  // optional FK to the affected row
	Actor      string         // "system"|"import"|"operator_unidentified"|<login post-auth>
	Details    map[string]any // optional JSON-serialised context
}

// NewService constructs an audit Service backed by the provided queries.
func NewService(q *storagedb.Queries) *Service {
	return &Service{
		queries: q,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Record inserts a single action_log entry.
//
// On an empty Actor the entry is stored with DefaultActor
// ("operator_unidentified") so the column's NOT NULL/CHECK constraint is
// satisfied even before F3 wires in real login sessions.
//
// Returns any underlying storage error verbatim so callers can decide
// whether audit failures should bubble up to the user.
func (s *Service) Record(ctx context.Context, in RecordInput) error {
	actor := in.Actor
	if actor == "" {
		actor = DefaultActor
	}

	details, err := marshalDetails(in.Details)
	if err != nil {
		return err
	}

	_, err = s.queries.CreateActionLog(ctx, storagedb.CreateActionLogParams{
		Actor:      actor,
		Action:     in.Action,
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		Details:    details,
		CreatedAt:  s.now().Format(time.RFC3339),
	})
	return err
}

// SetClock replaces the time source. Intended for tests.
func (s *Service) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	s.now = now
}

func marshalDetails(details map[string]any) (sql.NullString, error) {
	if len(details) == 0 {
		return sql.NullString{}, nil
	}

	payload, err := json.Marshal(details)
	if err != nil {
		return sql.NullString{}, err
	}

	return sql.NullString{String: string(payload), Valid: true}, nil
}
