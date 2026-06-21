-- name: GetUserByLogin :one
SELECT id, login, password_hash, role, status, created_at, updated_at
FROM users
WHERE login = ?;

-- name: GetUser :one
SELECT id, login, password_hash, role, status, created_at, updated_at
FROM users
WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (
  login,
  password_hash,
  role,
  status,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, login, password_hash, role, status, created_at, updated_at;