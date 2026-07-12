-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE program_groups (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_program_groups_code
  ON program_groups (code);

CREATE TABLE programs (
  id INTEGER PRIMARY KEY,
  program_group_id INTEGER NOT NULL,
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  default_hours INTEGER NOT NULL CHECK (default_hours > 0),
  moodle_course_id TEXT,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (program_group_id) REFERENCES program_groups (id)
);

CREATE UNIQUE INDEX ux_programs_group_code
  ON programs (program_group_id, code);

CREATE INDEX ix_programs_group_id
  ON programs (program_group_id);

CREATE TABLE employers (
  id INTEGER PRIMARY KEY,
  inn TEXT NOT NULL,
  inn_normalized TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_employers_inn_normalized
  ON employers (inn_normalized);

CREATE TABLE workers (
  id INTEGER PRIMARY KEY,
  last_name TEXT NOT NULL,
  first_name TEXT NOT NULL,
  middle_name TEXT,
  snils TEXT NOT NULL,
  snils_normalized TEXT NOT NULL,
  email TEXT NOT NULL,
  birth_date TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_workers_snils_normalized
  ON workers (snils_normalized);

CREATE INDEX ix_workers_name
  ON workers (last_name, first_name, middle_name);

CREATE TABLE worker_employers (
  id INTEGER PRIMARY KEY,
  worker_id INTEGER NOT NULL,
  employer_id INTEGER NOT NULL,
  current_position TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (worker_id) REFERENCES workers (id),
  FOREIGN KEY (employer_id) REFERENCES employers (id)
);

CREATE UNIQUE INDEX ux_worker_employers_active_pair
  ON worker_employers (worker_id, employer_id)
  WHERE status = 'active';

CREATE INDEX ix_worker_employers_worker_id
  ON worker_employers (worker_id);

CREATE INDEX ix_worker_employers_employer_id
  ON worker_employers (employer_id);

CREATE TABLE client_requests (
  id INTEGER PRIMARY KEY,
  employer_id INTEGER NOT NULL,
  received_date TEXT NOT NULL,
  source_type TEXT NOT NULL
    CHECK (source_type IN ('xlsx', 'manual', 'other')),
  source_import_id INTEGER,
  status TEXT NOT NULL DEFAULT 'review'
    CHECK (status IN ('review', 'completed', 'cancelled')),
  notes TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (employer_id) REFERENCES employers (id)
);

CREATE INDEX ix_client_requests_employer_id
  ON client_requests (employer_id);

CREATE INDEX ix_client_requests_status
  ON client_requests (status);

CREATE INDEX ix_client_requests_received_date
  ON client_requests (received_date);

CREATE TABLE request_rows (
  id INTEGER PRIMARY KEY,
  client_request_id INTEGER NOT NULL,
  row_number INTEGER NOT NULL CHECK (row_number > 0),
  raw_data TEXT NOT NULL,
  raw_full_name TEXT,
  parsed_last_name TEXT,
  parsed_first_name TEXT,
  parsed_middle_name TEXT,
  parsed_snils TEXT,
  parsed_email TEXT,
  parsed_position TEXT,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'parsed', 'invalid', 'applied', 'skipped')),
  error_summary TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (client_request_id) REFERENCES client_requests (id)
);

CREATE UNIQUE INDEX ux_request_rows_request_row_number
  ON request_rows (client_request_id, row_number);

CREATE INDEX ix_request_rows_client_request_id
  ON request_rows (client_request_id);

CREATE INDEX ix_request_rows_status
  ON request_rows (status);

CREATE TABLE training_records (
  id INTEGER PRIMARY KEY,
  worker_employer_id INTEGER NOT NULL,
  program_id INTEGER NOT NULL,
  client_request_id INTEGER,
  position TEXT NOT NULL,
  hours INTEGER NOT NULL CHECK (hours > 0),
  requires_mintrud_test INTEGER NOT NULL DEFAULT 0
    CHECK (requires_mintrud_test IN (0, 1)),
  moodle_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (moodle_status IN ('not_required', 'pending', 'enrolled', 'failed')),
  moodle_error TEXT,
  moodle_enrolled_at TEXT,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'cancelled')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (worker_employer_id) REFERENCES worker_employers (id),
  FOREIGN KEY (program_id) REFERENCES programs (id),
  FOREIGN KEY (client_request_id) REFERENCES client_requests (id)
);

CREATE INDEX ix_training_records_worker_employer_id
  ON training_records (worker_employer_id);

CREATE INDEX ix_training_records_program_id
  ON training_records (program_id);

CREATE INDEX ix_training_records_client_request_id
  ON training_records (client_request_id);

CREATE INDEX ix_training_records_status
  ON training_records (status);

CREATE INDEX ix_training_records_moodle_status
  ON training_records (moodle_status);

CREATE TABLE request_training_items (
  id INTEGER PRIMARY KEY,
  request_row_id INTEGER NOT NULL,
  program_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'valid', 'invalid', 'duplicate', 'conflict', 'applied', 'skipped')),
  error_summary TEXT,
  resolution TEXT
    CHECK (resolution IS NULL OR resolution IN ('skip_duplicate', 'link_existing', 'create_repeat')),
  training_record_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (request_row_id) REFERENCES request_rows (id),
  FOREIGN KEY (program_id) REFERENCES programs (id),
  FOREIGN KEY (training_record_id) REFERENCES training_records (id)
);

CREATE INDEX ix_request_training_items_row_id
  ON request_training_items (request_row_id);

CREATE INDEX ix_request_training_items_program_id
  ON request_training_items (program_id);

CREATE INDEX ix_request_training_items_status
  ON request_training_items (status);

CREATE INDEX ix_request_training_items_training_record_id
  ON request_training_items (training_record_id);

CREATE TABLE moodle_accounts (
  id INTEGER PRIMARY KEY,
  worker_id INTEGER NOT NULL,
  moodle_user_id TEXT NOT NULL,
  username TEXT NOT NULL,
  email TEXT,
  idnumber TEXT,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive', 'failed')),
  synced_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (worker_id) REFERENCES workers (id)
);

CREATE UNIQUE INDEX ux_moodle_accounts_moodle_user_id
  ON moodle_accounts (moodle_user_id);

CREATE UNIQUE INDEX ux_moodle_accounts_username
  ON moodle_accounts (username);

CREATE INDEX ix_moodle_accounts_worker_id
  ON moodle_accounts (worker_id);

CREATE TABLE protocols (
  id INTEGER PRIMARY KEY,
  program_group_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft', 'fixed', 'xml_uploaded', 'registry_entered', 'generated', 'completed', 'cancelled')),
  training_start_date TEXT,
  training_end_date TEXT,
  protocol_date TEXT,
  sequence_year INTEGER
    CHECK (sequence_year IS NULL OR sequence_year >= 2000),
  protocol_month INTEGER
    CHECK (protocol_month IS NULL OR protocol_month BETWEEN 1 AND 12),
  annual_sequence_number INTEGER
    CHECK (annual_sequence_number IS NULL OR annual_sequence_number > 0),
  protocol_number TEXT,
  fixed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (
    protocol_number IS NULL
    OR (
      sequence_year IS NOT NULL
      AND protocol_month IS NOT NULL
      AND annual_sequence_number IS NOT NULL
      AND fixed_at IS NOT NULL
    )
  ),
  CHECK (
    status NOT IN ('fixed', 'xml_uploaded', 'registry_entered', 'generated', 'completed')
    OR (
      training_start_date IS NOT NULL
      AND training_end_date IS NOT NULL
      AND protocol_date IS NOT NULL
      AND sequence_year IS NOT NULL
      AND protocol_month IS NOT NULL
      AND annual_sequence_number IS NOT NULL
      AND protocol_number IS NOT NULL
      AND fixed_at IS NOT NULL
    )
  ),
  FOREIGN KEY (program_group_id) REFERENCES program_groups (id)
);

CREATE INDEX ix_protocols_program_group_id
  ON protocols (program_group_id);

CREATE INDEX ix_protocols_status
  ON protocols (status);

CREATE INDEX ix_protocols_training_start_date
  ON protocols (training_start_date);

CREATE UNIQUE INDEX ux_protocols_group_year_sequence
  ON protocols (program_group_id, sequence_year, annual_sequence_number)
  WHERE annual_sequence_number IS NOT NULL;

CREATE UNIQUE INDEX ux_protocols_protocol_number
  ON protocols (protocol_number)
  WHERE protocol_number IS NOT NULL;

CREATE TABLE protocol_participants (
  id INTEGER PRIMARY KEY,
  protocol_id INTEGER NOT NULL,
  training_record_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'removed')),
  requires_mintrud_test_confirmed_at TEXT,
  mintrud_registry_number TEXT,
  mintrud_registry_entered_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (protocol_id) REFERENCES protocols (id),
  FOREIGN KEY (training_record_id) REFERENCES training_records (id)
);

CREATE INDEX ix_protocol_participants_protocol_id
  ON protocol_participants (protocol_id);

CREATE INDEX ix_protocol_participants_training_record_id
  ON protocol_participants (training_record_id);

CREATE INDEX ix_protocol_participants_status
  ON protocol_participants (status);

CREATE UNIQUE INDEX ux_protocol_participants_active_training_record
  ON protocol_participants (training_record_id)
  WHERE status = 'active';

CREATE TABLE generation_runs (
  id INTEGER PRIMARY KEY,
  protocol_id INTEGER NOT NULL,
  type TEXT NOT NULL
    CHECK (type IN ('xml', 'docx', 'moodle_credentials', 'xlsx_export')),
  status TEXT NOT NULL
    CHECK (status IN ('success', 'failed', 'stale')),
  file_name TEXT,
  generated_at TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (protocol_id) REFERENCES protocols (id)
);

CREATE INDEX ix_generation_runs_protocol_id
  ON generation_runs (protocol_id);

CREATE INDEX ix_generation_runs_type
  ON generation_runs (type);

CREATE INDEX ix_generation_runs_status
  ON generation_runs (status);

CREATE INDEX ix_generation_runs_generated_at
  ON generation_runs (generated_at);

CREATE TABLE action_log (
  id INTEGER PRIMARY KEY,
  actor TEXT NOT NULL
    CHECK (actor IN ('system', 'import', 'operator_unidentified')),
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id INTEGER,
  details TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX ix_action_log_created_at
  ON action_log (created_at);

CREATE INDEX ix_action_log_entity
  ON action_log (entity_type, entity_id);
