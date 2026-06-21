-- name: ListProtocolParticipants :many
-- Active participants for a protocol. Used by the detail view.
SELECT id, protocol_id, training_record_id, status,
       requires_mintrud_test_confirmed_at, mintrud_registry_number,
       mintrud_registry_entered_at, created_at, updated_at
FROM protocol_participants
WHERE protocol_id = ? AND status = 'active'
ORDER BY id ASC;

-- name: ListAllProtocolParticipants :many
-- Including removed participants (kept for audit). Used by integration tests.
SELECT id, protocol_id, training_record_id, status,
       requires_mintrud_test_confirmed_at, mintrud_registry_number,
       mintrud_registry_entered_at, created_at, updated_at
FROM protocol_participants
WHERE protocol_id = ?
ORDER BY id ASC;

-- name: GetProtocolParticipantByID :one
SELECT id, protocol_id, training_record_id, status,
       requires_mintrud_test_confirmed_at, mintrud_registry_number,
       mintrud_registry_entered_at, created_at, updated_at
FROM protocol_participants
WHERE id = ?;

-- name: GetActiveParticipantForTrainingRecord :one
-- Returns the active (non-removed) participant row for a training_record, if
-- any. Used by AddParticipant to enforce the unique-active-training-record
-- constraint at service level so the caller sees a clean validation error
-- instead of a SQLite constraint failure.
SELECT id, protocol_id, training_record_id, status,
       requires_mintrud_test_confirmed_at, mintrud_registry_number,
       mintrud_registry_entered_at, created_at, updated_at
FROM protocol_participants
WHERE training_record_id = ? AND status = 'active'
LIMIT 1;

-- name: CreateProtocolParticipant :one
INSERT INTO protocol_participants (
  protocol_id,
  training_record_id,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, 'active', ?, ?)
RETURNING id, protocol_id, training_record_id, status,
          requires_mintrud_test_confirmed_at, mintrud_registry_number,
          mintrud_registry_entered_at, created_at, updated_at;

-- name: MarkParticipantRemoved :one
-- Soft-delete: flips status to 'removed' instead of physically deleting.
UPDATE protocol_participants
SET status = 'removed', updated_at = ?
WHERE id = ? AND status = 'active'
RETURNING id, protocol_id, training_record_id, status,
          requires_mintrud_test_confirmed_at, mintrud_registry_number,
          mintrud_registry_entered_at, created_at, updated_at;
