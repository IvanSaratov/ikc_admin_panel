#!/usr/bin/env sh
set -eu

run_success() {
  name="$1"
  sql_file="$2"
  db_file="/tmp/mintrud_${name}.db"

  rm -f "$db_file"
  sqlite3 "$db_file" < "$sql_file"
  echo "PASS success: $name"
}

run_expected_failure() {
  name="$1"
  db_file="/tmp/mintrud_${name}.db"
  snippet_file="/tmp/mintrud_${name}.sql"

  rm -f "$db_file"
  awk -v target="$name" '
    $0 == "-- name: " target { emit = 1; next }
    /^-- name: / && emit { exit }
    emit { print }
  ' tests/schema_constraints.sql > "$snippet_file"

  if ! test -s "$snippet_file"; then
    echo "FAIL missing constraint snippet: $name"
    exit 1
  fi

  sqlite3 "$db_file" < tests/schema_smoke.sql

  if sqlite3 "$db_file" < "$snippet_file" >"/tmp/mintrud_${name}.out" 2>"/tmp/mintrud_${name}.err"; then
    echo "FAIL expected constraint failure: $name"
    cat "/tmp/mintrud_${name}.out"
    cat "/tmp/mintrud_${name}.err" >&2
    exit 1
  fi

  if ! grep -Eq "(UNIQUE|CHECK|FOREIGN KEY) constraint failed" "/tmp/mintrud_${name}.err"; then
    echo "FAIL unexpected error for: $name"
    cat "/tmp/mintrud_${name}.err" >&2
    exit 1
  fi

  echo "PASS expected failure: $name"
}

run_success "schema_smoke" "tests/schema_smoke.sql"
run_expected_failure "duplicate_snils"
run_expected_failure "duplicate_normalized_snils"
run_expected_failure "duplicate_normalized_inn"
run_expected_failure "fixed_protocol_requires_number"
run_expected_failure "duplicate_protocol_sequence"
run_expected_failure "duplicate_active_protocol_participant"
