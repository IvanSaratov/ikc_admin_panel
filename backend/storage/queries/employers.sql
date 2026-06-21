-- name: ListEmployers :many
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at
FROM employers
ORDER BY canonical_name;

-- name: SearchEmployers :many
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at
FROM employers
WHERE canonical_name LIKE ? OR inn_normalized LIKE ?
ORDER BY canonical_name;

-- name: GetEmployer :one
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at
FROM employers
WHERE id = ?;

-- name: GetEmployerByNormalizedINN :one
SELECT id, inn, inn_normalized, canonical_name, created_at, updated_at
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
RETURNING id, inn, inn_normalized, canonical_name, created_at, updated_at;

-- name: UpdateEmployer :one
UPDATE employers
SET inn = ?,
    inn_normalized = ?,
    canonical_name = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, inn, inn_normalized, canonical_name, created_at, updated_at;
