-- name: ListWorkerEmployersForWorker :many
SELECT id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at
FROM worker_employers
WHERE worker_id = ?
ORDER BY status, updated_at DESC;

-- name: ListWorkersForEmployer :many
SELECT id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at
FROM worker_employers
WHERE employer_id = ?
ORDER BY status, updated_at DESC;

-- name: ListWorkerEmployerAssignments :many
SELECT
  worker_employers.id,
  worker_employers.worker_id,
  workers.last_name AS worker_last_name,
  workers.first_name AS worker_first_name,
  worker_employers.employer_id,
  employers.canonical_name AS employer_name,
  worker_employers.current_position,
  worker_employers.current_department,
  worker_employers.status
FROM worker_employers
JOIN workers ON workers.id = worker_employers.worker_id
JOIN employers ON employers.id = worker_employers.employer_id
ORDER BY worker_employers.status, workers.last_name, workers.first_name, employers.canonical_name;

-- name: GetWorkerEmployer :one
SELECT id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at
FROM worker_employers
WHERE id = ?;

-- name: CreateWorkerEmployer :one
INSERT INTO worker_employers (
  worker_id,
  employer_id,
  current_position,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, ?, 'active', ?, ?)
RETURNING id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at;

-- name: UpdateWorkerEmployer :one
UPDATE worker_employers
SET employer_id = ?,
    current_position = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at;

-- name: SetWorkerEmployerStatus :one
UPDATE worker_employers
SET status = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at;

-- name: UpdateAssignment :one
UPDATE worker_employers
SET employer_id = ?,
    current_position = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at;

-- name: DeactivateAssignment :one
UPDATE worker_employers
SET status = 'inactive',
    updated_at = ?
WHERE id = ?
RETURNING id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at;

-- name: FindActiveWorkerEmployer :one
-- Used by ApplyRow to locate the active assignment between a worker and the
-- request's employer; falls back to creating one if none exists.
SELECT id, worker_id, employer_id, current_position, current_department, status, created_at, updated_at
FROM worker_employers
WHERE worker_id = ?
  AND employer_id = ?
  AND status = 'active'
LIMIT 1;
