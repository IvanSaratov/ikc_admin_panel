-- name: CreateClientRequest :one
INSERT INTO client_requests (
  employer_id,
  received_date,
  source_type,
  source_import_id,
  status,
  notes,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at;

-- name: GetClientRequest :one
SELECT id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at
FROM client_requests
WHERE id = ?;

-- name: ListClientRequests :many
SELECT id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at
FROM client_requests
ORDER BY received_date DESC, id DESC;

-- name: ListClientRequestsByStatus :many
SELECT id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at
FROM client_requests
WHERE status = ?
ORDER BY received_date DESC, id DESC;

-- name: UpdateClientRequestStatus :one
UPDATE client_requests
SET status = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at;

-- name: SetClientRequestSourceImport :one
UPDATE client_requests
SET source_import_id = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at;