-- name: ListEmployers :many
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at, status
FROM employers
ORDER BY canonical_name;

-- name: SearchEmployers :many
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at, status
FROM employers
WHERE canonical_name LIKE ? OR inn_normalized LIKE ?
ORDER BY canonical_name;

-- name: GetEmployer :one
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at, status
FROM employers
WHERE id = ?;

-- name: GetEmployerByNormalizedINN :one
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at, status
FROM employers
WHERE inn_normalized = ?;

-- name: CreateEmployer :one
INSERT INTO employers (
  inn,
  inn_normalized,
  canonical_name,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?)
RETURNING id, inn, inn_normalized, canonical_name, created_at, updated_at, status;

-- name: UpdateEmployer :one
UPDATE employers
SET inn = ?,
    inn_normalized = ?,
    canonical_name = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, inn, inn_normalized, canonical_name, created_at, updated_at, status;

-- name: GetEmployerByID :one
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at, status
FROM employers
WHERE id = ?;

-- name: DeactivateEmployer :one
-- After 003 added `status` to employers, this flips the soft-delete flag
-- and records the transition in the audit log. Idempotent: a no-op when
-- status is already 'inactive' (no rows updated).
UPDATE employers
SET status = 'inactive', updated_at = ?
WHERE id = ? AND status = 'active'
RETURNING id, inn, inn_normalized, canonical_name, created_at, updated_at, status;