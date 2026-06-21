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
