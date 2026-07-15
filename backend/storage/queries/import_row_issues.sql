-- name: CreateImportRowIssue :one
INSERT INTO import_row_issues (
  import_row_id,
  field,
  code,
  severity,
  message,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListImportRowIssues :many
SELECT *
FROM import_row_issues
WHERE import_row_id = ?
ORDER BY severity DESC, field ASC, code ASC, id ASC;

-- name: DeleteImportRowIssues :exec
DELETE FROM import_row_issues
WHERE import_row_id = ?;

-- name: ResolveImportRowIssue :one
UPDATE import_row_issues
SET resolution = ?,
    resolved_by_actor = ?,
    resolved_at = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;
