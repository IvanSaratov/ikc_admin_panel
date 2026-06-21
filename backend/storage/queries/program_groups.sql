-- name: ListProgramGroups :many
SELECT id, code, name, status, created_at, updated_at
FROM program_groups
ORDER BY code;

-- name: GetProgramGroup :one
SELECT id, code, name, status, created_at, updated_at
FROM program_groups
WHERE id = ?;

-- name: CreateProgramGroup :one
INSERT INTO program_groups (
  code,
  name,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, 'active', ?, ?)
RETURNING id, code, name, status, created_at, updated_at;

-- name: UpdateProgramGroup :one
UPDATE program_groups
SET code = ?,
    name = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, code, name, status, created_at, updated_at;

-- name: SetProgramGroupStatus :one
UPDATE program_groups
SET status = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, code, name, status, created_at, updated_at;
