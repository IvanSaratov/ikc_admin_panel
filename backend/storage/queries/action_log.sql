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
-- actor/action/entity_type + a created_at range. Each non-empty filter
-- narrows the result; an empty filter value is ignored (no constraint).
-- Ordering is newest-first so the audit UI shows recent activity first;
-- ties on created_at are broken by id DESC for stable pagination.
SELECT id, actor, action, entity_type, entity_id, details, created_at
FROM action_log
WHERE (sqlc.narg('actor') IS NULL OR actor = sqlc.narg('actor'))
  AND (sqlc.narg('action') IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('entity_type') IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('created_from') IS NULL OR created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to') IS NULL OR created_at <= sqlc.narg('created_to'))
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?;

-- name: CountActionLogsFiltered :one
-- D4 audit UI: count rows that match the same filter set as
-- ListActionLogsFiltered. Computed in a single query so the UI can
-- render "Page N of M" without pulling the full result set.
SELECT COUNT(*) AS total
FROM action_log
WHERE (sqlc.narg('actor') IS NULL OR actor = sqlc.narg('actor'))
  AND (sqlc.narg('action') IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('entity_type') IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('created_from') IS NULL OR created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to') IS NULL OR created_at <= sqlc.narg('created_to'));
