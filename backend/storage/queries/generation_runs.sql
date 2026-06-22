-- name: CreateGenerationRun :one
-- Insert a new generation_runs row. status is one of 'success' / 'failed'
-- / 'stale' (the schema CHECK constraint). file_name and error_message
-- are nullable.
INSERT INTO generation_runs (
  protocol_id,
  type,
  status,
  file_name,
  generated_at,
  error_message,
  created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, protocol_id, type, status, file_name, generated_at, error_message, created_at;

-- name: GetGenerationRun :one
-- Look up a generation_runs row by id. Used by the download handler to
-- serve a previously generated file. Returns sql.ErrNoRows when missing.
SELECT id, protocol_id, type, status, file_name, generated_at, error_message, created_at
FROM generation_runs
WHERE id = ?;

-- name: ListGenerationRunsForProtocol :many
-- Recent generations for a single protocol, newest first. Used by the
-- detail view to show the "last downloads" list.
SELECT id, protocol_id, type, status, file_name, generated_at, error_message, created_at
FROM generation_runs
WHERE protocol_id = ?
ORDER BY generated_at DESC, id DESC;