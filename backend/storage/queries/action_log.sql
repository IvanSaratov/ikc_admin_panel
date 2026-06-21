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

-- name: ListActionLogsFiltered :many
-- D4 audit UI: list action_log rows matching the optional filters in
-- actor/action/entity_type + a created_at range. Each filter parameter
-- is a plain string; an empty string disables that filter. We use
-- `length(filter) = 0 OR col <op> filter` to short-circuit when the
-- filter is empty so the WHERE clause drops out without a NULL path.
-- Ordering is newest-first; ties broken by id DESC for stable pagination.
SELECT id, actor, action, entity_type, entity_id, details, created_at
FROM action_log
WHERE (length(sqlc.arg('actor')) = 0 OR actor = sqlc.arg('actor'))
  AND (length(sqlc.arg('action')) = 0 OR action = sqlc.arg('action'))
  AND (length(sqlc.arg('entity_type')) = 0 OR entity_type = sqlc.arg('entity_type'))
  AND (length(sqlc.arg('created_from')) = 0 OR created_at >= sqlc.arg('created_from'))
  AND (length(sqlc.arg('created_to')) = 0 OR created_at <= sqlc.arg('created_to'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountActionLogsFiltered :one
-- D4 audit UI: count rows that match the same filter set as
-- ListActionLogsFiltered. One round-trip so the UI can render "Page N
-- of M" without pulling the full result set. Same filter idiom.
SELECT COUNT(*) AS total
FROM action_log
WHERE (length(sqlc.arg('actor')) = 0 OR actor = sqlc.arg('actor'))
  AND (length(sqlc.arg('action')) = 0 OR action = sqlc.arg('action'))
  AND (length(sqlc.arg('entity_type')) = 0 OR entity_type = sqlc.arg('entity_type'))
  AND (length(sqlc.arg('created_from')) = 0 OR created_at >= sqlc.arg('created_from'))
  AND (length(sqlc.arg('created_to')) = 0 OR created_at <= sqlc.arg('created_to'));
