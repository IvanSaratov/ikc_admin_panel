-- name: GetTrainingRecord :one
-- Returns a single training record by id, used by the protocols slice
-- to enrich the participant table with program/position info. Kept in
-- its own file (rather than people.sql) so the protocols slice can
-- reach into the storage layer without taking a dependency on the
-- people service.
SELECT id, worker_employer_id, program_id, client_request_id, position, hours,
       requires_mintrud_test, moodle_status, moodle_error, moodle_enrolled_at,
       status, created_at, updated_at
FROM training_records
WHERE id = ?;
