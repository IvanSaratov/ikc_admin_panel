-- +goose Up

-- 003: status columns + actor CHECK relax.
-- (Originally planned as 002b, but goose parses only integer filename
-- prefixes, so the file was renumbered.)
--
-- F1 left DeactivateEmployer as a stub (no status column on employers/workers);
-- F3 needs to write real operator logins into action_log.actor but the existing
-- CHECK is IN ('system', 'import', 'operator_unidentified') which would reject
-- any real login. This migration is the pre-req for both:
--   * adds status to employers + workers (default 'active') so Deactivate can
--     flip a real soft-delete flag, and
--   * rebuilds action_log with a permissive actor CHECK that still bounds
--     length but no longer constrains the value set.

ALTER TABLE employers
  ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK (status IN ('active', 'inactive'));

ALTER TABLE workers
  ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK (status IN ('active', 'inactive'));

-- SQLite has no ALTER TABLE DROP CONSTRAINT, so the only way to relax
-- action_log.actor is the 12-step table rebuild (documented in 002). The
-- existing rows are copied verbatim; only the CHECK on actor changes.
CREATE TABLE action_log_new (
  id INTEGER PRIMARY KEY,
  actor TEXT NOT NULL
    CHECK (length(actor) > 0 AND length(actor) <= 200),
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id INTEGER,
  details TEXT,
  created_at TEXT NOT NULL
);

INSERT INTO action_log_new (id, actor, action, entity_type, entity_id, details, created_at)
SELECT id, actor, action, entity_type, entity_id, details, created_at
FROM action_log;

DROP TABLE action_log;

ALTER TABLE action_log_new RENAME TO action_log;

CREATE INDEX ix_action_log_created_at ON action_log (created_at);
CREATE INDEX ix_action_log_entity ON action_log (entity_type, entity_id);
