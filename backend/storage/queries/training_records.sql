-- name: CreateTrainingRecord :one
INSERT INTO training_records (
  worker_employer_id,
  program_id,
  client_request_id,
  position,
  hours,
  requires_mintrud_test,
  moodle_status,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, worker_employer_id, program_id, client_request_id, position, hours, requires_mintrud_test, moodle_status, moodle_error, moodle_enrolled_at, status, created_at, updated_at;

-- name: GetTrainingRecord :one
SELECT id, worker_employer_id, program_id, client_request_id, position, hours, requires_mintrud_test, moodle_status, moodle_error, moodle_enrolled_at, status, created_at, updated_at
FROM training_records
WHERE id = ?;

-- name: FindActiveTrainingRecord :one
-- Used by ApplyRow to detect duplicates: same worker_employer + program +
-- status='active' is treated as "already enrolled", which forces the row
-- into the 'duplicate' state instead of creating a second training_record.
SELECT id, worker_employer_id, program_id, client_request_id, position, hours, requires_mintrud_test, moodle_status, moodle_error, moodle_enrolled_at, status, created_at, updated_at
FROM training_records
WHERE worker_employer_id = ?
  AND program_id = ?
  AND status = 'active'
LIMIT 1;