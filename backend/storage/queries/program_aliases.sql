-- name: CreateProgramAlias :one
INSERT INTO program_aliases (
  profile,
  sheet_profile,
  alias_normalized,
  program_id,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetProgramByAlias :one
SELECT programs.*
FROM program_aliases
JOIN programs ON programs.id = program_aliases.program_id
WHERE program_aliases.profile = ?
  AND program_aliases.sheet_profile = ?
  AND program_aliases.alias_normalized = ?
LIMIT 1;

-- name: ListProgramAliases :many
SELECT *
FROM program_aliases
WHERE profile = ?
ORDER BY sheet_profile ASC, alias_normalized ASC, id ASC;
