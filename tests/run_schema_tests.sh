#!/usr/bin/env sh
set -eu

run_success() {
  name="$1"
  sql_file="$2"
  db_file="/tmp/mintrud_${name}.db"

  rm -f "$db_file"
  # Foreign-key enforcement is per-connection. Prefix the input with the
  # pragma so tests that check FK behaviour run inside a single connection
  # where foreign_keys = ON.
  cat "$sql_file" | sqlite3 "$db_file"
  echo "PASS success: $name"
}

extract_snippet() {
  name="$1"
  snippet_file="$2"

  awk -v target="$name" '
    $0 == "-- name: " target { emit = 1; next }
    /^-- name: / && emit { exit }
    emit { print }
  ' tests/schema_constraints.sql > "$snippet_file"

  if ! test -s "$snippet_file"; then
    echo "FAIL missing constraint snippet: $name"
    exit 1
  fi
}

run_expected_failure() {
  name="$1"
  db_file="/tmp/mintrud_${name}.db"
  snippet_file="/tmp/mintrud_${name}.sql"

  rm -f "$db_file"
  extract_snippet "$name" "$snippet_file"

  # Single sqlite3 invocation so PRAGMA foreign_keys = ON (set at the top
  # of schema_smoke.sql) stays active for the snippet, which is required
  # for FK-based constraint snippets to fire.
  if cat tests/schema_smoke.sql "$snippet_file" | sqlite3 "$db_file" \
       >"/tmp/mintrud_${name}.out" 2>"/tmp/mintrud_${name}.err"; then
    echo "FAIL expected constraint failure: $name"
    cat "/tmp/mintrud_${name}.out"
    cat "/tmp/mintrud_${name}.err" >&2
    exit 1
  fi

  if ! grep -Eq "(UNIQUE|CHECK|FOREIGN KEY|NOT NULL) constraint failed" "/tmp/mintrud_${name}.err"; then
    echo "FAIL unexpected error for: $name"
    cat "/tmp/mintrud_${name}.err" >&2
    exit 1
  fi

  echo "PASS expected failure: $name"
}

# Positive control: extract a named snippet and verify it executes without
# error against a fresh DB seeded with schema_smoke.sql. Used for constraint
# snippets that should be allowed (e.g. non-conflicting inserts).
run_expected_success() {
  name="$1"
  db_file="/tmp/mintrud_${name}.db"
  snippet_file="/tmp/mintrud_${name}.sql"

  rm -f "$db_file"
  extract_snippet "$name" "$snippet_file"

  if ! cat tests/schema_smoke.sql "$snippet_file" | sqlite3 "$db_file" \
         >"/tmp/mintrud_${name}.out" 2>"/tmp/mintrud_${name}.err"; then
    echo "FAIL expected positive snippet to succeed: $name"
    cat "/tmp/mintrud_${name}.out"
    cat "/tmp/mintrud_${name}.err" >&2
    exit 1
  fi

  echo "PASS expected success: $name"
}

run_success "schema_smoke" "tests/schema_smoke.sql"
run_expected_failure "duplicate_snils"
run_expected_failure "worker_email_required"
run_expected_failure "duplicate_normalized_snils"
run_expected_failure "duplicate_normalized_inn"
run_expected_failure "fixed_protocol_requires_number"
run_expected_failure "duplicate_protocol_sequence"
run_expected_failure "duplicate_active_protocol_participant"
run_expected_failure "test_imports_fk_rejects_orphan_source_import_id"
run_expected_failure "test_protocols_unique_group_year_seq_suffix"
run_expected_success "test_protocols_different_suffix_allowed"


