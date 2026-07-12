-- +goose Up

-- 004: auth foundation — users table for F3 login/logout.
-- scs cookie-based sessions live entirely in the session cookie, so no
-- separate sessions table is needed for MVP.

CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  login TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL
    CHECK (role IN ('operator', 'admin')),
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'disabled')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_users_login
  ON users (login);