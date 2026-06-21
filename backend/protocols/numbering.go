package protocols

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// ErrInvalidDate is returned when a date string cannot be parsed as a calendar
// date (YYYY-MM-DD). The protocol schema stores dates as TEXT in ISO format;
// bad input must be rejected before we try to derive sequence_year/month.
var ErrInvalidDate = errors.New("invalid date")

// ErrInvalidSuffix is returned when the suffix is non-empty but does not match
// the allowed vocabulary. We restrict suffixes to short numeric strings ("1",
// "2", "3") to keep protocol numbers operator-readable; arbitrary text would
// risk collisions with the "/<seq>/<suffix>" format.
var ErrInvalidSuffix = errors.New("invalid protocol suffix")

// ParseISODate validates an ISO-style YYYY-MM-DD date string and returns the
// parsed year and month (1-12). Returns ErrInvalidDate with a field error
// message when the string is not a real calendar date.
//
// We do NOT use time.Parse because SQLite stores dates as TEXT and downstream
// callers may pass through any consistent format — but for the protocol
// lifecycle we want strict ISO-8601 (date only, no time component, no
// timezone surprises).
func ParseISODate(raw string) (year int, month int, err error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) != 10 || trimmed[4] != '-' || trimmed[7] != '-' {
		return 0, 0, fmt.Errorf("%w: %q must be YYYY-MM-DD", ErrInvalidDate, raw)
	}
	yearValue, err := strconv.Atoi(trimmed[0:4])
	if err != nil || yearValue < 2000 {
		return 0, 0, fmt.Errorf("%w: year must be >= 2000", ErrInvalidDate)
	}
	monthValue, err := strconv.Atoi(trimmed[5:7])
	if err != nil || monthValue < 1 || monthValue > 12 {
		return 0, 0, fmt.Errorf("%w: month must be 01..12", ErrInvalidDate)
	}
	dayValue, err := strconv.Atoi(trimmed[8:10])
	if err != nil || dayValue < 1 || dayValue > 31 {
		return 0, 0, fmt.Errorf("%w: day must be 01..31", ErrInvalidDate)
	}
	return yearValue, monthValue, nil
}

// NormalizeSuffix trims whitespace and validates the suffix vocabulary. Empty
// (after trim) means "no suffix" and returns ("", nil). Non-empty must be a
// short numeric string; the schema CHECK on protocol_suffix does not enforce
// this, so we add the check here to keep generated numbers sane.
func NormalizeSuffix(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if _, err := strconv.Atoi(trimmed); err != nil {
		return "", fmt.Errorf("%w: suffix must be a short number, got %q", ErrInvalidSuffix, trimmed)
	}
	if len(trimmed) > 8 {
		return "", fmt.Errorf("%w: suffix too long", ErrInvalidSuffix)
	}
	return trimmed, nil
}

// FormatProtocolNumber renders the canonical protocol_number string. The
// contract is "<year>-<month>/<seq_3_digit>[/<suffix>]" — month is always
// zero-padded to two digits, sequence to three digits. Suffix is appended
// only when non-empty so the no-suffix protocol number stays compact.
func FormatProtocolNumber(year, month int, seq int64, suffix string) string {
	base := fmt.Sprintf("%04d-%02d/%03d", year, month, seq)
	if suffix == "" {
		return base
	}
	return base + "/" + suffix
}

// nextSequenceLocked returns the next annual sequence number for a
// (program_group_id, sequence_year, suffix) triple. The caller is expected
// to hold a transaction so the read-then-write isn't raced. Returns 1 when
// no fixed protocol exists yet for that triple.
//
// The COALESCE on the suffix mirrors the unique index from
// 002_schema_cleanup, so an empty suffix slots in next to a "1" suffix
// without colliding (each gets its own sequence number per group/year).
//
// The MaxAnnualSequenceForGroupYear query returns interface{} because sqlc
// cannot infer the column type for a MAX() over a nullable column; we
// type-switch defensively.
func nextSequenceLocked(ctx context.Context, q *storagedb.Queries, programGroupID int64, year int64, suffix string) (int64, error) {
	max, err := q.MaxAnnualSequenceForGroupYear(ctx, storagedb.MaxAnnualSequenceForGroupYearParams{
		ProgramGroupID: programGroupID,
		SequenceYear:   sql.NullInt64{Int64: year, Valid: true},
		ProtocolSuffix: sql.NullString{String: suffix, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("read max sequence: %w", err)
	}
	switch v := max.(type) {
	case nil:
		return 1, nil
	case int64:
		return v + 1, nil
	default:
		return 0, fmt.Errorf("unexpected max sequence type %T", v)
	}
}
