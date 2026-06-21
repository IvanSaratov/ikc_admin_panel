-- Constraint snippets for tests/run_schema_tests.sh.
-- Each block is extracted by marker and executed against a fresh DB.

-- name: duplicate_snils
INSERT INTO workers (
  last_name,
  first_name,
  middle_name,
  snils,
  snils_normalized,
  email,
  birth_date,
  created_at,
  updated_at
)
VALUES (
  'Petrov',
  'Petr',
  NULL,
  '123-456-789 00',
  '12345678900',
  'duplicate@example.test',
  NULL,
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: duplicate_normalized_snils
INSERT INTO workers (
  last_name,
  first_name,
  middle_name,
  snils,
  snils_normalized,
  email,
  birth_date,
  created_at,
  updated_at
)
VALUES (
  'Petrov',
  'Petr',
  NULL,
  '12345678900',
  '12345678900',
  'duplicate-normalized@example.test',
  NULL,
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: worker_email_required
INSERT INTO workers (
  last_name,
  first_name,
  middle_name,
  snils,
  snils_normalized,
  email,
  birth_date,
  created_at,
  updated_at
)
VALUES (
  'Noemail',
  'Worker',
  NULL,
  '987-654-321 00',
  '98765432100',
  NULL,
  NULL,
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: duplicate_normalized_inn
INSERT INTO employers (
  inn,
  inn_normalized,
  canonical_name,
  created_at,
  updated_at
)
VALUES (
  '7700000000',
  '7700000000',
  'Duplicate Employer',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: fixed_protocol_requires_number
INSERT INTO program_groups (code, name, status, created_at, updated_at)
VALUES ('B', 'Direction B', 'active', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');

INSERT INTO protocols (
  program_group_id,
  status,
  training_start_date,
  training_end_date,
  protocol_date,
  created_at,
  updated_at
)
VALUES (
  2,
  'fixed',
  '2026-05-11',
  '2026-05-15',
  '2026-05-15',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: duplicate_protocol_sequence
INSERT INTO protocols (
  program_group_id,
  status,
  training_start_date,
  training_end_date,
  protocol_date,
  sequence_year,
  protocol_month,
  annual_sequence_number,
  protocol_number,
  fixed_at,
  created_at,
  updated_at
)
VALUES (
  1,
  'fixed',
  '2026-06-01',
  '2026-06-05',
  '2026-06-05',
  2026,
  6,
  14,
  '2606A14',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: duplicate_active_protocol_participant
INSERT INTO protocol_participants (
  protocol_id,
  training_record_id,
  status,
  created_at,
  updated_at
)
VALUES (1, 1, 'active', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');

-- name: test_imports_fk_rejects_orphan_source_import_id
-- The schema_smoke seeds employer_id=1 and protocol/training records;
-- create a fresh client_request whose source_import_id points to an
-- import_id that does not exist. The FK on client_requests.source_import_id
-- must reject the row.
INSERT INTO client_requests (
  employer_id,
  received_date,
  source_type,
  source_import_id,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  '2026-05-27',
  'xlsx',
  9999,
  'review',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: test_protocols_unique_group_year_seq_suffix
-- schema_smoke inserts protocol id=1 (program_group_id=1, year=2026,
-- seq=14, suffix=NULL) and protocol id=2 (same group/year/seq, suffix='2').
-- A second protocol with the same (group, year, seq) and suffix=NULL must
-- be rejected by ux_protocols_group_year_seq_suffix.
INSERT INTO protocols (
  program_group_id,
  status,
  training_start_date,
  training_end_date,
  protocol_date,
  sequence_year,
  protocol_month,
  annual_sequence_number,
  protocol_number,
  fixed_at,
  created_at,
  updated_at
)
VALUES (
  1,
  'fixed',
  '2026-06-10',
  '2026-06-15',
  '2026-06-15',
  2026,
  6,
  14,
  '2606A14',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- name: test_protocols_different_suffix_allowed
-- Positive control: same (group, year, seq) as the existing fixed
-- protocols in schema_smoke, but with a fresh suffix '3' that no other
-- protocol uses. Must succeed.
INSERT INTO protocols (
  program_group_id,
  status,
  training_start_date,
  training_end_date,
  protocol_date,
  sequence_year,
  protocol_month,
  annual_sequence_number,
  protocol_suffix,
  protocol_number,
  fixed_at,
  created_at,
  updated_at
)
VALUES (
  1,
  'fixed',
  '2026-07-01',
  '2026-07-05',
  '2026-07-05',
  2026,
  7,
  14,
  '3',
  '2607A14/3',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);
