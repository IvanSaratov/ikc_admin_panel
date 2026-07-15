-- +goose Up
PRAGMA application_id = 0x494B4341;
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
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive'))
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
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive'))
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
  current_department TEXT,
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

CREATE TABLE imports (
  id INTEGER PRIMARY KEY,
  profile TEXT NOT NULL
    CHECK (profile IN ('legacy_registry', 'client_request')),
  source_file_name TEXT,
  source_sha256 TEXT
    CHECK (
      source_sha256 IS NULL
      OR (
        length(source_sha256) = 64
        AND source_sha256 NOT GLOB '*[^0-9a-f]*'
      )
    ),
  source_size_bytes INTEGER
    CHECK (source_size_bytes IS NULL OR source_size_bytes >= 0),
  idempotency_key TEXT,
  uploaded_by_actor TEXT NOT NULL,
  received_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued'
    CHECK (
      status IN (
        'queued',
        'processing',
        'completed',
        'completed_with_issues',
        'failed',
        'cancelled'
      )
    ),
  phase TEXT
    CHECK (
      phase IS NULL
      OR phase IN ('parsing', 'staging', 'validating', 'applying', 'finalizing')
    ),
  temp_file_token TEXT,
  temp_file_expires_at TEXT,
  lease_owner TEXT,
  lease_expires_at TEXT,
  heartbeat_at TEXT,
  attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  rows_total INTEGER NOT NULL DEFAULT 0 CHECK (rows_total >= 0),
  rows_processed INTEGER NOT NULL DEFAULT 0 CHECK (rows_processed >= 0),
  rows_applied INTEGER NOT NULL DEFAULT 0 CHECK (rows_applied >= 0),
  rows_duplicate INTEGER NOT NULL DEFAULT 0 CHECK (rows_duplicate >= 0),
  rows_needs_review INTEGER NOT NULL DEFAULT 0 CHECK (rows_needs_review >= 0),
  error_code TEXT,
  error_detail TEXT,
  started_at TEXT,
  staged_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX ix_imports_status
  ON imports (status);

CREATE INDEX ix_imports_received_at
  ON imports (received_at);

CREATE UNIQUE INDEX ux_imports_idempotency_key
  ON imports (idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX ux_imports_active_legacy_sha256
  ON imports (profile, source_sha256)
  WHERE profile = 'legacy_registry'
    AND source_sha256 IS NOT NULL
    AND status IN ('queued', 'processing', 'completed', 'completed_with_issues');

CREATE TABLE import_rows (
  id INTEGER PRIMARY KEY,
  import_id INTEGER NOT NULL,
  sheet_name TEXT NOT NULL,
  row_number INTEGER NOT NULL CHECK (row_number > 0),
  raw_data TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (import_id) REFERENCES imports (id) ON DELETE CASCADE
);

CREATE INDEX ix_import_rows_import_id
  ON import_rows (import_id);

CREATE UNIQUE INDEX ux_import_rows_import_sheet_row_number
  ON import_rows (import_id, sheet_name, row_number);

CREATE TABLE import_sheets (
  id INTEGER PRIMARY KEY,
  import_id INTEGER NOT NULL,
  sheet_name TEXT NOT NULL,
  sheet_order INTEGER NOT NULL CHECK (sheet_order > 0),
  sheet_profile TEXT NOT NULL,
  header_map TEXT NOT NULL DEFAULT '{}',
  rows_found INTEGER NOT NULL DEFAULT 0 CHECK (rows_found >= 0),
  rows_staged INTEGER NOT NULL DEFAULT 0 CHECK (rows_staged >= 0),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'parsing', 'staged', 'failed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (import_id) REFERENCES imports (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX ux_import_sheets_import_name
  ON import_sheets (import_id, sheet_name);

CREATE UNIQUE INDEX ux_import_sheets_import_order
  ON import_sheets (import_id, sheet_order);

CREATE INDEX ix_import_sheets_import_id
  ON import_sheets (import_id);

CREATE TABLE legacy_import_rows (
  import_row_id INTEGER PRIMARY KEY,
  source_fingerprint TEXT NOT NULL
    CHECK (
      length(source_fingerprint) = 64
      AND source_fingerprint NOT GLOB '*[^0-9a-f]*'
    ),
  employer_name TEXT,
  inn_normalized TEXT,
  last_name TEXT,
  first_name TEXT,
  middle_name TEXT,
  snils_normalized TEXT,
  email_normalized TEXT,
  position TEXT,
  department TEXT,
  program_text TEXT,
  training_start_date TEXT,
  training_end_date TEXT,
  protocol_number TEXT,
  protocol_date TEXT,
  assessment_result TEXT,
  mintrud_registry_number TEXT,
  source_reference TEXT,
  moodle_username TEXT,
  moodle_email TEXT,
  extra_fields TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'staged'
    CHECK (
      status IN (
        'staged',
        'valid',
        'applying',
        'applied',
        'duplicate',
        'needs_review',
        'skipped'
      )
    ),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  employer_id INTEGER,
  worker_id INTEGER,
  worker_employer_id INTEGER,
  program_id INTEGER,
  client_request_id INTEGER,
  training_record_id INTEGER,
  protocol_id INTEGER,
  protocol_participant_id INTEGER,
  moodle_account_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (import_row_id) REFERENCES import_rows (id) ON DELETE CASCADE,
  FOREIGN KEY (employer_id) REFERENCES employers (id),
  FOREIGN KEY (worker_id) REFERENCES workers (id),
  FOREIGN KEY (worker_employer_id) REFERENCES worker_employers (id),
  FOREIGN KEY (program_id) REFERENCES programs (id),
  FOREIGN KEY (client_request_id) REFERENCES client_requests (id),
  FOREIGN KEY (training_record_id) REFERENCES training_records (id),
  FOREIGN KEY (protocol_id) REFERENCES protocols (id),
  FOREIGN KEY (protocol_participant_id) REFERENCES protocol_participants (id),
  FOREIGN KEY (moodle_account_id) REFERENCES moodle_accounts (id)
);

CREATE INDEX ix_legacy_import_rows_status
  ON legacy_import_rows (status);

CREATE INDEX ix_legacy_import_rows_inn
  ON legacy_import_rows (inn_normalized);

CREATE INDEX ix_legacy_import_rows_snils
  ON legacy_import_rows (snils_normalized);

CREATE INDEX ix_legacy_import_rows_protocol
  ON legacy_import_rows (protocol_number, protocol_date);

CREATE TABLE import_row_issues (
  id INTEGER PRIMARY KEY,
  import_row_id INTEGER NOT NULL,
  field TEXT NOT NULL,
  code TEXT NOT NULL,
  severity TEXT NOT NULL
    CHECK (severity IN ('warning', 'blocking')),
  message TEXT NOT NULL,
  resolution TEXT,
  resolved_by_actor TEXT,
  resolved_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (import_row_id) REFERENCES import_rows (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX ux_import_row_issues_row_field_code
  ON import_row_issues (import_row_id, field, code);

CREATE INDEX ix_import_row_issues_row_id
  ON import_row_issues (import_row_id);

CREATE INDEX ix_import_row_issues_code
  ON import_row_issues (code);

CREATE TABLE program_aliases (
  id INTEGER PRIMARY KEY,
  profile TEXT NOT NULL
    CHECK (profile IN ('legacy_registry', 'client_request')),
  sheet_profile TEXT NOT NULL,
  alias_normalized TEXT NOT NULL,
  program_id INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (program_id) REFERENCES programs (id)
);

CREATE UNIQUE INDEX ux_program_aliases_profile_sheet_alias
  ON program_aliases (profile, sheet_profile, alias_normalized);

CREATE INDEX ix_program_aliases_program_id
  ON program_aliases (program_id);

CREATE TABLE client_requests (
  id INTEGER PRIMARY KEY,
  employer_id INTEGER NOT NULL,
  received_date TEXT NOT NULL,
  source_type TEXT NOT NULL
    CHECK (source_type IN ('xlsx', 'manual', 'other')),
  source_import_id INTEGER REFERENCES imports (id) ON DELETE SET NULL,
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
  department TEXT,
  source_reference TEXT,
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
  protocol_suffix TEXT,
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

CREATE UNIQUE INDEX ux_protocols_group_year_seq_suffix
  ON protocols (
    program_group_id,
    sequence_year,
    annual_sequence_number,
    COALESCE(protocol_suffix, '')
  )
  WHERE annual_sequence_number IS NOT NULL;

CREATE UNIQUE INDEX ux_protocols_group_protocol_number
  ON protocols (program_group_id, protocol_number)
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
  assessment_result TEXT,
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
    CHECK (length(actor) > 0 AND length(actor) <= 200),
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
