PRAGMA foreign_keys = ON;

.read migrations/001_initial_schema.sql

INSERT INTO program_groups (code, name, status, created_at, updated_at)
VALUES ('A', 'Direction A', 'active', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');

INSERT INTO programs (
  program_group_id,
  code,
  name,
  default_hours,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  'A-1',
  'Program A-1',
  40,
  'active',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO employers (inn, inn_normalized, canonical_name, created_at, updated_at)
VALUES (
  '7700000000',
  '7700000000',
  'Test Employer',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

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
  'Aliyev',
  'Murad',
  'Mamed ogly',
  '123-456-789 00',
  '12345678900',
  NULL,
  NULL,
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO worker_employers (
  worker_id,
  employer_id,
  current_position,
  status,
  created_at,
  updated_at
)
VALUES (1, 1, 'Engineer', 'active', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');

INSERT INTO client_requests (
  employer_id,
  received_date,
  source_type,
  status,
  created_at,
  updated_at
)
VALUES (1, '2026-05-27', 'manual', 'review', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');

INSERT INTO training_records (
  worker_employer_id,
  program_id,
  client_request_id,
  position,
  hours,
  requires_mintrud_test,
  moodle_status,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  1,
  1,
  'Engineer',
  40,
  0,
  'pending',
  'active',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

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
  '2026-05-11',
  '2026-05-15',
  '2026-05-15',
  2026,
  5,
  14,
  '2605A14',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO protocol_participants (
  protocol_id,
  training_record_id,
  status,
  requires_mintrud_test_confirmed_at,
  created_at,
  updated_at
)
VALUES (1, 1, 'active', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z');
