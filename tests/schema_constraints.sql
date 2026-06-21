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
