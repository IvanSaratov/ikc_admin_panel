-- name: CreateRequestRow :one
INSERT INTO request_rows (
  client_request_id,
  row_number,
  raw_data,
  raw_full_name,
  parsed_last_name,
  parsed_first_name,
  parsed_middle_name,
  parsed_snils,
  parsed_email,
  parsed_position,
  status,
  error_summary,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, client_request_id, row_number, raw_data, raw_full_name, parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position, status, error_summary, created_at, updated_at;

-- name: GetRequestRow :one
SELECT id, client_request_id, row_number, raw_data, raw_full_name, parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position, status, error_summary, created_at, updated_at
FROM request_rows
WHERE id = ?;

-- name: ListRequestRows :many
SELECT id, client_request_id, row_number, raw_data, raw_full_name, parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position, status, error_summary, created_at, updated_at
FROM request_rows
WHERE client_request_id = ?
ORDER BY row_number ASC, id ASC;

-- name: UpdateRequestRowParsed :one
UPDATE request_rows
SET raw_full_name = ?,
    parsed_last_name = ?,
    parsed_first_name = ?,
    parsed_middle_name = ?,
    parsed_snils = ?,
    parsed_email = ?,
    parsed_position = ?,
    status = ?,
    error_summary = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, client_request_id, row_number, raw_data, raw_full_name, parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position, status, error_summary, created_at, updated_at;

-- name: UpdateRequestRowStatus :one
UPDATE request_rows
SET status = ?,
    error_summary = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, client_request_id, row_number, raw_data, raw_full_name, parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position, status, error_summary, created_at, updated_at;