-- name: CreateRequestTrainingItem :one
INSERT INTO request_training_items (
  request_row_id,
  program_id,
  status,
  error_summary,
  resolution,
  training_record_id,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, request_row_id, program_id, status, error_summary, resolution, training_record_id, created_at, updated_at;

-- name: GetRequestTrainingItem :one
SELECT id, request_row_id, program_id, status, error_summary, resolution, training_record_id, created_at, updated_at
FROM request_training_items
WHERE id = ?;

-- name: ListRequestTrainingItems :many
SELECT id, request_row_id, program_id, status, error_summary, resolution, training_record_id, created_at, updated_at
FROM request_training_items
WHERE request_row_id = ?
ORDER BY id ASC;

-- name: UpdateRequestTrainingItemStatus :one
UPDATE request_training_items
SET status = ?,
    error_summary = ?,
    resolution = ?,
    training_record_id = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, request_row_id, program_id, status, error_summary, resolution, training_record_id, created_at, updated_at;

-- name: ListRequestTrainingItemsForRow :many
SELECT rti.id, rti.request_row_id, rti.program_id, rti.status, rti.error_summary, rti.resolution, rti.training_record_id, rti.created_at, rti.updated_at,
       p.code AS program_code, p.name AS program_name, p.default_hours AS program_default_hours
FROM request_training_items rti
JOIN programs p ON p.id = rti.program_id
WHERE rti.request_row_id = ?
ORDER BY rti.id ASC;