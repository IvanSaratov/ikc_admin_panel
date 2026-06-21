-- name: CreateImport :one
INSERT INTO imports (
  source_type,
  source_file_name,
  source_sha256,
  uploaded_by_actor,
  received_at,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportByID :one
SELECT *
FROM imports
WHERE id = ?
LIMIT 1;

-- name: ListImports :many
SELECT *
FROM imports
ORDER BY received_at DESC, id DESC;

-- name: UpdateImportStatus :exec
UPDATE imports
SET status = ?,
    updated_at = ?
WHERE id = ?;

-- name: CreateImportRow :one
INSERT INTO import_rows (
  import_id,
  row_number,
  raw_data,
  created_at
)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListImportRows :many
SELECT *
FROM import_rows
WHERE import_id = ?
ORDER BY row_number ASC, id ASC;

-- name: GetImportRowByID :one
SELECT *
FROM import_rows
WHERE id = ?
LIMIT 1;
