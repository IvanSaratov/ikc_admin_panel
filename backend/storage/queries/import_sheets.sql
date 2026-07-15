-- name: CreateImportSheet :one
INSERT INTO import_sheets (
  import_id,
  sheet_name,
  sheet_order,
  sheet_profile,
  header_map,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportSheet :one
SELECT *
FROM import_sheets
WHERE import_id = ? AND sheet_name = ?
LIMIT 1;

-- name: ListImportSheets :many
SELECT *
FROM import_sheets
WHERE import_id = ?
ORDER BY sheet_order ASC, id ASC;

-- name: UpdateImportSheetProgress :one
UPDATE import_sheets
SET rows_found = ?,
    rows_staged = ?,
    status = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;
