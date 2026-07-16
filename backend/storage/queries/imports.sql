-- name: CreateImport :one
INSERT INTO imports (
  profile,
  source_file_name,
  source_sha256,
  source_size_bytes,
  idempotency_key,
  uploaded_by_actor,
  received_at,
  status,
  phase,
  temp_file_token,
  temp_file_expires_at,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportByID :one
SELECT *
FROM imports
WHERE id = ?
LIMIT 1;

-- name: GetImportByIdempotencyKey :one
SELECT *
FROM imports
WHERE idempotency_key = ?
LIMIT 1;

-- name: FindExistingLegacyImportBySHA256 :one
SELECT *
FROM imports
WHERE profile = 'legacy_registry'
  AND source_sha256 = ?
  AND status IN ('queued', 'processing', 'completed', 'completed_with_issues')
ORDER BY id DESC
LIMIT 1;

-- name: ListImportsPage :many
SELECT *
FROM imports
WHERE id < sqlc.arg(before_id) OR sqlc.arg(before_id) = 0
ORDER BY id DESC
LIMIT sqlc.arg(page_size);

-- name: ListImportsAPI :many
SELECT
  current_import.id,
  current_import.profile,
  current_import.source_file_name,
  current_import.uploaded_by_actor,
  current_import.received_at,
  current_import.status,
  current_import.phase,
  current_import.rows_total,
  current_import.rows_processed,
  current_import.rows_applied,
  current_import.rows_duplicate,
  current_import.rows_needs_review,
  current_import.error_code,
  current_import.error_detail,
  current_import.started_at,
  current_import.staged_at,
  current_import.completed_at,
  current_import.created_at,
  current_import.updated_at,
  CASE
    WHEN current_import.status = 'queued' THEN 1 + (
      SELECT COUNT(*)
      FROM imports AS ahead
      WHERE ahead.id < current_import.id
        AND ahead.status IN ('queued', 'processing')
    )
    ELSE 0
  END AS queue_position
FROM imports AS current_import
WHERE current_import.id < sqlc.arg(before_id) OR sqlc.arg(before_id) = 0
ORDER BY current_import.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetImportAPI :one
SELECT
  current_import.id,
  current_import.profile,
  current_import.source_file_name,
  current_import.uploaded_by_actor,
  current_import.received_at,
  current_import.status,
  current_import.phase,
  current_import.rows_total,
  current_import.rows_processed,
  current_import.rows_applied,
  current_import.rows_duplicate,
  current_import.rows_needs_review,
  current_import.error_code,
  current_import.error_detail,
  current_import.started_at,
  current_import.staged_at,
  current_import.completed_at,
  current_import.created_at,
  current_import.updated_at,
  CASE
    WHEN current_import.status = 'queued' THEN 1 + (
      SELECT COUNT(*)
      FROM imports AS ahead
      WHERE ahead.id < current_import.id
        AND ahead.status IN ('queued', 'processing')
    )
    ELSE 0
  END AS queue_position
FROM imports AS current_import
WHERE current_import.id = ?
LIMIT 1;

-- name: CountImportsAhead :one
SELECT COUNT(*)
FROM imports
WHERE id < sqlc.arg(import_id)
  AND status IN ('queued', 'processing');

-- name: CountActiveImports :one
SELECT COUNT(*)
FROM imports
WHERE status IN ('queued', 'processing');

-- name: ClaimNextImport :one
UPDATE imports
SET status = 'processing',
    phase = COALESCE(phase, 'parsing'),
    lease_owner = sqlc.arg(lease_owner),
    lease_expires_at = sqlc.arg(lease_expires_at),
    heartbeat_at = sqlc.arg(now),
    attempt = attempt + 1,
    started_at = COALESCE(started_at, sqlc.arg(now)),
    updated_at = sqlc.arg(now)
WHERE id = (
  SELECT id
  FROM imports AS candidate
  WHERE (
      candidate.status = 'queued'
      OR (
        candidate.status = 'processing'
        AND (
          candidate.lease_expires_at IS NULL
          OR candidate.lease_expires_at < sqlc.arg(now)
        )
      )
    )
    AND NOT EXISTS (
      SELECT 1
      FROM imports AS active
      WHERE active.status = 'processing'
        AND active.lease_expires_at >= ?3
    )
  ORDER BY id ASC
  LIMIT 1
)
RETURNING *;

-- name: UpdateImportProgress :one
UPDATE imports
SET rows_total = ?,
    rows_processed = ?,
    rows_applied = ?,
    rows_duplicate = ?,
    rows_needs_review = ?,
    heartbeat_at = ?,
    lease_expires_at = ?,
    updated_at = ?
WHERE id = ? AND lease_owner = ?
RETURNING *;

-- name: UpdateImportState :one
UPDATE imports
SET status = ?,
    phase = ?,
    error_code = ?,
    error_detail = ?,
    staged_at = ?,
    completed_at = ?,
    lease_owner = ?,
    lease_expires_at = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: CreateImportRow :one
INSERT INTO import_rows (
  import_id,
  sheet_name,
  row_number,
  raw_data,
  created_at
)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListImportRows :many
SELECT import_rows.*
FROM import_rows
LEFT JOIN import_sheets
  ON import_sheets.import_id = import_rows.import_id
 AND import_sheets.sheet_name = import_rows.sheet_name
WHERE import_rows.import_id = ?
ORDER BY COALESCE(import_sheets.sheet_order, 0) ASC,
         import_rows.row_number ASC,
         import_rows.id ASC;

-- name: GetImportRowByID :one
SELECT *
FROM import_rows
WHERE id = ?
LIMIT 1;
