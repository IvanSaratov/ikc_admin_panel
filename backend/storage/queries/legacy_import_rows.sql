-- name: CreateLegacyImportRow :one
INSERT INTO legacy_import_rows (
  import_row_id,
  source_fingerprint,
  extra_fields,
  created_at,
  updated_at
)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLegacyImportRow :one
SELECT *
FROM legacy_import_rows
WHERE import_row_id = ?
LIMIT 1;

-- name: UpdateLegacyImportRowNormalized :one
UPDATE legacy_import_rows
SET employer_name = ?,
    inn_normalized = ?,
    last_name = ?,
    first_name = ?,
    middle_name = ?,
    snils_normalized = ?,
    email_normalized = ?,
    position = ?,
    department = ?,
    program_text = ?,
    training_start_date = ?,
    training_end_date = ?,
    protocol_number = ?,
    protocol_date = ?,
    assessment_result = ?,
    mintrud_registry_number = ?,
    source_reference = ?,
    moodle_username = ?,
    moodle_email = ?,
    extra_fields = ?,
    status = ?,
    version = version + 1,
    updated_at = ?
WHERE import_row_id = ? AND version = ?
RETURNING *;

-- name: UpdateLegacyImportRowLinks :one
UPDATE legacy_import_rows
SET employer_id = ?,
    worker_id = ?,
    worker_employer_id = ?,
    program_id = ?,
    client_request_id = ?,
    training_record_id = ?,
    protocol_id = ?,
    protocol_participant_id = ?,
    moodle_account_id = ?,
    status = ?,
    version = version + 1,
    updated_at = ?
WHERE import_row_id = ? AND version = ?
RETURNING *;

-- name: ListAllLegacyImportRowsPage :many
SELECT
  import_rows.id,
  import_rows.import_id,
  import_rows.sheet_name,
  import_rows.row_number,
  import_rows.raw_data,
  import_rows.created_at,
  import_sheets.sheet_order,
  legacy_import_rows.*
FROM import_rows
JOIN import_sheets
  ON import_sheets.import_id = import_rows.import_id
 AND import_sheets.sheet_name = import_rows.sheet_name
JOIN legacy_import_rows
  ON legacy_import_rows.import_row_id = import_rows.id
WHERE import_rows.import_id = sqlc.arg(import_id)
  AND (
    import_sheets.sheet_order > sqlc.arg(after_sheet_order)
    OR (
      import_sheets.sheet_order = sqlc.arg(after_sheet_order)
      AND import_rows.row_number > sqlc.arg(after_row_number)
    )
    OR (
      import_sheets.sheet_order = sqlc.arg(after_sheet_order)
      AND import_rows.row_number = sqlc.arg(after_row_number)
      AND import_rows.id > sqlc.arg(after_id)
    )
  )
ORDER BY import_sheets.sheet_order ASC, import_rows.row_number ASC, import_rows.id ASC
LIMIT sqlc.arg(page_size);

-- name: ListLegacyImportRowsByStatusPage :many
SELECT
  import_rows.id,
  import_rows.import_id,
  import_rows.sheet_name,
  import_rows.row_number,
  import_rows.raw_data,
  import_rows.created_at,
  import_sheets.sheet_order,
  legacy_import_rows.*
FROM import_rows
JOIN import_sheets
  ON import_sheets.import_id = import_rows.import_id
 AND import_sheets.sheet_name = import_rows.sheet_name
JOIN legacy_import_rows
  ON legacy_import_rows.import_row_id = import_rows.id
WHERE import_rows.import_id = sqlc.arg(import_id)
  AND legacy_import_rows.status = sqlc.arg(status_filter)
  AND (
    import_sheets.sheet_order > sqlc.arg(after_sheet_order)
    OR (
      import_sheets.sheet_order = sqlc.arg(after_sheet_order)
      AND import_rows.row_number > sqlc.arg(after_row_number)
    )
    OR (
      import_sheets.sheet_order = sqlc.arg(after_sheet_order)
      AND import_rows.row_number = sqlc.arg(after_row_number)
      AND import_rows.id > sqlc.arg(after_id)
    )
  )
ORDER BY import_sheets.sheet_order ASC, import_rows.row_number ASC, import_rows.id ASC
LIMIT sqlc.arg(page_size);
