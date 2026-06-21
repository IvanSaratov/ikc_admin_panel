-- name: ListPrograms :many
SELECT id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at
FROM programs
ORDER BY code;

-- name: ListProgramsByGroup :many
SELECT id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at
FROM programs
WHERE program_group_id = ?
ORDER BY code;

-- name: GetProgram :one
SELECT id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at
FROM programs
WHERE id = ?;

-- name: CreateProgram :one
INSERT INTO programs (
  program_group_id,
  code,
  name,
  default_hours,
  moodle_course_id,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, 'active', ?, ?)
RETURNING id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at;

-- name: UpdateProgram :one
UPDATE programs
SET program_group_id = ?,
    code = ?,
    name = ?,
    default_hours = ?,
    moodle_course_id = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at;

-- name: SetProgramStatus :one
UPDATE programs
SET status = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at;
