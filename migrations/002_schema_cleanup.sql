-- +goose Up
-- Migration 002: schema cleanup for slice F2.
--
-- Goals:
--   1. Add `imports` and `import_rows` tables for XLSX/manual request imports.
--   2. Convert `client_requests.source_import_id` from a plain INTEGER column
--      into a proper foreign key referencing `imports(id)` with
--      `ON DELETE SET NULL` (current value preserved on delete, request stays).
--   3. Add `protocols.protocol_suffix` (nullable TEXT, e.g. '1', '2', '3') to
--      allow multiple fixed protocols to share the same (group, year, seq).
--   4. Replace the existing unique index on protocols with a stricter one that
--      includes the suffix.
--   5. Document status vocabularies for `request_rows.status` and
--      `protocols.status` (no vocabulary changes — workflow already correct).
--
-- Design notes (SQLite quirks):
--   * SQLite cannot `ALTER TABLE ... ADD CONSTRAINT FOREIGN KEY` against an
--     existing column. We use the standard 12-step procedure: build a
--     `client_requests_new` with the FK, copy data over, drop the old table,
--     rename, then recreate the indexes (which are dropped automatically with
--     the table).
--   * SQLite's default NULL semantics in UNIQUE indexes treat NULLs as
--     distinct. We want a single "no suffix" protocol per (group, year, seq),
--     so the new index uses `COALESCE(protocol_suffix, '')` to make NULL
--     collapse to a single key value. The original `WHERE annual_sequence_number
--     IS NOT NULL` clause is preserved so draft protocols are excluded.
--   * No DOWN section: the schema tests load migrations via goose Up only,
--     and per the design decision (plan section F2 step 7) we don't ship a
--     Down until we have an explicit reverse path.

-- Step 1: imports metadata table for XLSX/manual/other sources.
CREATE TABLE imports (
  id INTEGER PRIMARY KEY,
  -- Mirrors the `client_requests.source_type` vocabulary exactly so a
  -- request row can carry its source via `source_import_id` without coercion.
  source_type TEXT NOT NULL
    CHECK (source_type IN ('xlsx', 'manual', 'other')),
  source_file_name TEXT,
  source_sha256 TEXT,
  uploaded_by_actor TEXT NOT NULL,
  received_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'received'
    CHECK (status IN ('received', 'processing', 'completed', 'failed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX ix_imports_status
  ON imports (status);

CREATE INDEX ix_imports_received_at
  ON imports (received_at);

-- Step 2: raw rows captured from each import (one row per source-data row).
CREATE TABLE import_rows (
  id INTEGER PRIMARY KEY,
  import_id INTEGER NOT NULL,
  row_number INTEGER NOT NULL CHECK (row_number > 0),
  raw_data TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (import_id) REFERENCES imports (id)
);

CREATE INDEX ix_import_rows_import_id
  ON import_rows (import_id);

CREATE UNIQUE INDEX ux_import_rows_import_row_number
  ON import_rows (import_id, row_number);

-- Step 3: rebuild client_requests with a proper FK to imports(id).
-- 12-step procedure:
--   (a) create new table with full schema + FK,
--   (b) copy all rows,
--   (c) drop old table (this drops its indexes too),
--   (d) rename new -> old name,
--   (e) recreate the original indexes.
-- The CHECK constraints and DEFAULT values are preserved verbatim.
CREATE TABLE client_requests_new (
  id INTEGER PRIMARY KEY,
  employer_id INTEGER NOT NULL,
  received_date TEXT NOT NULL,
  source_type TEXT NOT NULL
    CHECK (source_type IN ('xlsx', 'manual', 'other')),
  source_import_id INTEGER
    REFERENCES imports (id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'review'
    CHECK (status IN ('review', 'completed', 'cancelled')),
  notes TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (employer_id) REFERENCES employers (id)
);

INSERT INTO client_requests_new (
  id, employer_id, received_date, source_type, source_import_id,
  status, notes, created_at, updated_at
)
SELECT
  id, employer_id, received_date, source_type, source_import_id,
  status, notes, created_at, updated_at
FROM client_requests;

DROP TABLE client_requests;

ALTER TABLE client_requests_new RENAME TO client_requests;

-- Step 3e: recreate the indexes that were dropped with the original table.
CREATE INDEX ix_client_requests_employer_id
  ON client_requests (employer_id);

CREATE INDEX ix_client_requests_status
  ON client_requests (status);

CREATE INDEX ix_client_requests_received_date
  ON client_requests (received_date);

-- Step 4: add `protocol_suffix` column (NULL allowed).
-- Examples: '1', '2', '3' for parallel protocols in the same annual sequence.
-- NULL means "single protocol with no suffix variant".
ALTER TABLE protocols ADD COLUMN protocol_suffix TEXT;

-- Step 5: replace the old uniqueness index.
-- Old: `ux_protocols_group_year_sequence` on (program_group_id,
--      sequence_year, annual_sequence_number) WHERE annual_sequence_number
--      IS NOT NULL.
-- New: `ux_protocols_group_year_seq_suffix` adds COALESCE(protocol_suffix, '')
-- so a NULL suffix is treated as a single value (empty string), preventing
-- accidental multiple "no-suffix" protocols for the same (group, year, seq).
DROP INDEX ux_protocols_group_year_sequence;

CREATE UNIQUE INDEX ux_protocols_group_year_seq_suffix
  ON protocols (
    program_group_id,
    sequence_year,
    annual_sequence_number,
    COALESCE(protocol_suffix, '')
  )
  WHERE annual_sequence_number IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Status vocabulary documentation (no schema change; comments only).
--
-- request_rows.status:
--   pending  - row inserted, not yet parsed.
--   parsed   - raw fields split into parsed_* columns; awaiting normalization.
--   invalid  - normalization or validation failed; row stays for operator review.
--   applied  - row was successfully turned into a worker/assignment/training_record.
--   skipped  - operator explicitly skipped (e.g. duplicate intentionally left out).
--
-- protocols.status (linear lifecycle, `cancelled` reachable from any state):
--   draft            - protocol created; no dates/number assigned yet.
--   fixed            - dates and number assigned; ready for XML export.
--   xml_uploaded     - XML payload sent to Минтруд external system.
--   registry_entered - Минтруд returned a registry number for participants.
--   generated        - DOCX protocol generated locally from the fixed data.
--   completed        - lifecycle finished; no further transitions.
--   cancelled        - terminal; reached from any non-terminal status.
-- ---------------------------------------------------------------------------
