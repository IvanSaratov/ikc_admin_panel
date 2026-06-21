-- name: ListProtocols :many
-- All protocols, newest first. Used by /protocols list view.
SELECT id, program_group_id, status, training_start_date, training_end_date,
       protocol_date, sequence_year, protocol_month, annual_sequence_number,
       protocol_number, fixed_at, created_at, updated_at, protocol_suffix
FROM protocols
ORDER BY created_at DESC, id DESC;

-- name: ListProtocolsByGroup :many
SELECT id, program_group_id, status, training_start_date, training_end_date,
       protocol_date, sequence_year, protocol_month, annual_sequence_number,
       protocol_number, fixed_at, created_at, updated_at, protocol_suffix
FROM protocols
WHERE program_group_id = ?
ORDER BY created_at DESC, id DESC;

-- name: ListProtocolsByStatus :many
SELECT id, program_group_id, status, training_start_date, training_end_date,
       protocol_date, sequence_year, protocol_month, annual_sequence_number,
       protocol_number, fixed_at, created_at, updated_at, protocol_suffix
FROM protocols
WHERE status = ?
ORDER BY created_at DESC, id DESC;

-- name: GetProtocolByID :one
SELECT id, program_group_id, status, training_start_date, training_end_date,
       protocol_date, sequence_year, protocol_month, annual_sequence_number,
       protocol_number, fixed_at, created_at, updated_at, protocol_suffix
FROM protocols
WHERE id = ?;

-- name: CreateProtocol :one
-- Insert a brand-new protocol in 'draft' status. Only program_group_id is
-- required; everything else is NULL until Fix is called.
INSERT INTO protocols (
  program_group_id,
  status,
  created_at,
  updated_at
)
VALUES (?, 'draft', ?, ?)
RETURNING id, program_group_id, status, training_start_date, training_end_date,
          protocol_date, sequence_year, protocol_month, annual_sequence_number,
          protocol_number, fixed_at, created_at, updated_at, protocol_suffix;

-- name: FixProtocol :one
-- Mark a draft protocol as fixed and stamp dates + number atomically.
-- The service is responsible for computing sequence_year / protocol_month /
-- annual_sequence_number / protocol_number / fixed_at from the request and
-- the current MAX sequence for (program_group_id, sequence_year).
UPDATE protocols
SET status = 'fixed',
    training_start_date = ?,
    training_end_date = ?,
    protocol_date = ?,
    sequence_year = ?,
    protocol_month = ?,
    annual_sequence_number = ?,
    protocol_number = ?,
    protocol_suffix = ?,
    fixed_at = ?,
    updated_at = ?
WHERE id = ? AND status = 'draft'
RETURNING id, program_group_id, status, training_start_date, training_end_date,
          protocol_date, sequence_year, protocol_month, annual_sequence_number,
          protocol_number, fixed_at, created_at, updated_at, protocol_suffix;

-- name: SetProtocolStatus :one
-- Update the lifecycle status. The service validates the transition with
-- CanTransition before calling this.
UPDATE protocols
SET status = ?, updated_at = ?
WHERE id = ?
RETURNING id, program_group_id, status, training_start_date, training_end_date,
          protocol_date, sequence_year, protocol_month, annual_sequence_number,
          protocol_number, fixed_at, created_at, updated_at, protocol_suffix;

-- name: MaxAnnualSequenceForGroupYear :one
-- Returns the highest annual_sequence_number already used for a (group, year,
-- suffix) triple. The COALESCE on protocol_suffix is mirrored on the unique
-- index in 002_schema_cleanup so the same seq can be reused across suffixes.
-- Returns NULL when no fixed protocol exists yet for that triple; the
-- service treats NULL as 0 and adds 1 to assign the next slot.
SELECT MAX(annual_sequence_number) AS max_seq
FROM protocols
WHERE program_group_id = ?
  AND sequence_year = ?
  AND COALESCE(protocol_suffix, '') = ?
  AND annual_sequence_number IS NOT NULL;
