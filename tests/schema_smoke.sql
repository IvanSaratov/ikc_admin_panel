PRAGMA foreign_keys = ON;

.read migrations/001_initial_schema.sql
.read migrations/002_schema_cleanup.sql

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
  'student@example.test',
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

-- Smoke row exercising the new `protocols.protocol_suffix` column.
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
  '2026-05-11',
  '2026-05-15',
  '2026-05-15',
  2026,
  5,
  14,
  '2',
  '2605A14/2',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

-- Smoke rows exercising the new `imports` / `import_rows` tables.
INSERT INTO imports (
  id,
  source_type,
  source_file_name,
  source_sha256,
  uploaded_by_actor,
  received_at,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  'xlsx',
  'sample_request.xlsx',
  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
  'operator_unidentified',
  '2026-05-27T00:00:00Z',
  'completed',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO import_rows (
  id,
  import_id,
  row_number,
  raw_data,
  created_at
)
VALUES (
  1,
  1,
  1,
  '{"name":"Aliyev Murad","snils":"123-456-789 00"}',
  '2026-05-27T00:00:00Z'
);

INSERT INTO action_log (
  actor,
  action,
  entity_type,
  entity_id,
  details,
  created_at
)
VALUES (
  'operator_unidentified',
  'worker.created',
  'workers',
  1,
  '{"source":"schema_smoke"}',
  '2026-05-27T00:00:00Z'
);
