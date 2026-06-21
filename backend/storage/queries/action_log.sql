-- name: CreateActionLog :one
INSERT INTO action_log (
  actor,
  action,
  entity_type,
  entity_id,
  details,
  created_at
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, actor, action, entity_type, entity_id, details, created_at;

-- name: ListActionLogsByEntity :many
-- Test helper: returns all action_log rows for a given entity, ordered by
-- insertion (id ASC). Used by service tests to verify audit trails.
SELECT id, actor, action, entity_type, entity_id, details, created_at
FROM action_log
WHERE entity_type = ? AND entity_id = ?
ORDER BY id ASC;
