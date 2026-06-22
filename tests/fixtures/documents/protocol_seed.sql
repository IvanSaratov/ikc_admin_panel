-- protocol_seed.sql: canonical seed for D3 golden fixtures.
-- Mirrors the inline seeding in backend/documents/testenv_test.go so the
-- fixture-based tests can re-use the same shape.
--
-- Run AFTER migrations are applied. The script uses current_timestamp
-- values for created_at / updated_at / fixed_at; tests that need a
-- deterministic timestamp should override those.
--
-- Usage from a SQL shell:
--   sqlite3 /path/to/test.db < tests/fixtures/documents/protocol_seed.sql
--
-- After running this, the seed produces:
--   - program_group id=1
--   - program id=1
--   - worker id=1
--   - employer id=1
--   - worker_employer id=1
--   - training_record id=1
--   - protocol id=1, status='fixed', protocol_number='2026-06/001'
--   - protocol_participant id=1 (active, linking protocol 1 <-> tr 1)

INSERT INTO program_groups (id, code, name, status, created_at, updated_at)
VALUES (1, 'D3-GOLDEN-G', 'D3 Golden Group', 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO programs (id, program_group_id, code, name, default_hours, status, created_at, updated_at)
VALUES (1, 1, 'D3-GOLDEN-P', 'D3 Golden Program', 40, 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO workers (id, last_name, first_name, middle_name, snils, snils_normalized, email, created_at, updated_at)
VALUES (1, 'Иванов', 'Иван', 'Иванович', '123-456-789 01', '12345678901', 'ivan@example.test', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO employers (id, inn, inn_normalized, canonical_name, status, created_at, updated_at)
VALUES (1, '7701234567', '7701234567', 'D3 Golden Employer', 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO worker_employers (id, worker_id, employer_id, current_position, status, created_at, updated_at)
VALUES (1, 1, 1, 'Engineer', 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO training_records (id, worker_employer_id, program_id, position, hours, requires_mintrud_test, moodle_status, status, created_at, updated_at)
VALUES (1, 1, 1, 'Engineer', 40, 0, 'not_required', 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO protocols (id, program_group_id, status, training_start_date, training_end_date, protocol_date,
    sequence_year, protocol_month, annual_sequence_number, protocol_number, fixed_at, created_at, updated_at)
VALUES (1, 1, 'fixed', '2026-06-01', '2026-06-30', '2026-06-30',
    2026, 6, 1, '2026-06/001', '2026-06-22T00:00:00Z',
    '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');

INSERT INTO protocol_participants (id, protocol_id, training_record_id, status, created_at, updated_at)
VALUES (1, 1, 1, 'active', '2026-06-22T00:00:00Z', '2026-06-22T00:00:00Z');