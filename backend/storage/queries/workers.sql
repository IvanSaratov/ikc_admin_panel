-- name: ListWorkers :many
SELECT id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at
FROM workers
ORDER BY last_name, first_name, middle_name;

-- name: SearchWorkers :many
SELECT id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at
FROM workers
WHERE last_name LIKE ? OR first_name LIKE ? OR snils_normalized LIKE ? OR email LIKE ?
ORDER BY last_name, first_name, middle_name;

-- name: GetWorker :one
SELECT id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at
FROM workers
WHERE id = ?;

-- name: GetWorkerByNormalizedSNILS :one
SELECT id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at
FROM workers
WHERE snils_normalized = ?;

-- name: CreateWorker :one
INSERT INTO workers (
  last_name,
  first_name,
  middle_name,
  snils,
  snils_normalized,
  email,
  birth_date,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at;

-- name: UpdateWorker :one
UPDATE workers
SET last_name = ?,
    first_name = ?,
    middle_name = ?,
    snils = ?,
    snils_normalized = ?,
    email = ?,
    birth_date = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, created_at, updated_at;
