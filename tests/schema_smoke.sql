PRAGMA foreign_keys = ON;

.read backend/migrations/001_baseline.sql

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
  current_department,
  status,
  created_at,
  updated_at
)
VALUES (
  1, 1, 'Engineer', 'Test Department', 'active',
  '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z'
);

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
  department,
  source_reference,
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
  'Test Department',
  'request-example-1',
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
  assessment_result,
  created_at,
  updated_at
)
VALUES (
  1, 1, 'active', '2026-05-27T00:00:00Z', 'passed',
  '2026-05-27T00:00:00Z', '2026-05-27T00:00:00Z'
);

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
  profile,
  source_file_name,
  source_sha256,
  source_size_bytes,
  idempotency_key,
  uploaded_by_actor,
  received_at,
  status,
  phase,
  rows_total,
  rows_processed,
  rows_applied,
  rows_duplicate,
  rows_needs_review,
  created_at,
  updated_at
)
VALUES (
  1,
  'legacy_registry',
  'sample_request.xlsx',
  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
  2048,
  'schema-smoke-import-1',
  'operator_unidentified',
  '2026-05-27T00:00:00Z',
  'processing',
  'staging',
  2,
  1,
  0,
  0,
  0,
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO import_rows (
  id,
  import_id,
  sheet_name,
  row_number,
  raw_data,
  created_at
)
VALUES (
  1,
  1,
  'А',
  1,
  '{"name":"Aliyev Murad","snils":"123-456-789 00"}',
  '2026-05-27T00:00:00Z'
);

INSERT INTO import_sheets (
  import_id,
  sheet_name,
  sheet_order,
  sheet_profile,
  header_map,
  rows_found,
  rows_staged,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  'А',
  1,
  'А',
  '{"A":"organization"}',
  1,
  1,
  'staged',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO legacy_import_rows (
  import_row_id,
  source_fingerprint,
  extra_fields,
  status,
  created_at,
  updated_at
)
VALUES (
  1,
  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  '{}',
  'needs_review',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO import_row_issues (
  import_row_id,
  field,
  code,
  severity,
  message,
  created_at,
  updated_at
)
VALUES (
  1,
  'program',
  'unknown_program',
  'blocking',
  'Synthetic program requires mapping',
  '2026-05-27T00:00:00Z',
  '2026-05-27T00:00:00Z'
);

INSERT INTO program_aliases (
  profile,
  sheet_profile,
  alias_normalized,
  program_id,
  created_at,
  updated_at
)
VALUES (
  'legacy_registry',
  'А',
  'program a-1',
  1,
  '2026-05-27T00:00:00Z',
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
