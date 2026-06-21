package requests

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode"
)

// Normalized row carries the cleaned-up fields used by the persistence
// layer. The raw input fields (RawFullName, RawSNILS, ...) stay on the
// originating ParsedRow so the operator can still see what came in from
// the file. Only the *Normalized fields are written into the DB.
type NormalizedRow struct {
	LastName    string
	FirstName   string
	MiddleName  string
	SNILS       string // original (trimmed) format, e.g. "123-456-789 00"
	SNILSDigits string // digits-only, e.g. "12345678900"
	Email       string
	Position    string
}

// NormalizeError is returned when one or more fields fail validation.
// Field-level errors are mapped in Fields so the UI can show them next
// to the input.
type NormalizeError struct {
	Fields map[string]string
}

func (e *NormalizeError) Error() string {
	if len(e.Fields) == 0 {
		return "normalization failed"
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	return "normalization failed: " + strings.Join(keys, ",")
}

func (e *NormalizeError) Is(target error) bool {
	return target == ErrNormalization
}

// ErrNormalization is the sentinel for FieldErrors-style comparisons.
var ErrNormalization = errors.New("normalization")

// NormalizeFullName splits a raw full name string into last/first/middle
// components. Accepts:
//   - "Last First Middle"  (3 parts)
//   - "Last First"         (2 parts)
//
// Each part is trimmed. If the input has fewer than 2 non-empty parts,
// a NormalizeError is returned with field="full_name".
func NormalizeFullName(raw string) (last, first, middle string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", "", &NormalizeError{Fields: map[string]string{
			"full_name": "Укажите ФИО.",
		}}
	}

	// Collapse internal runs of whitespace so "Иванов   Иван" still
	// splits correctly without producing empty tokens.
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", "", "", &NormalizeError{Fields: map[string]string{
			"full_name": "ФИО должно содержать как минимум фамилию и имя.",
		}}
	}

	last = fields[0]
	first = fields[1]
	if len(fields) >= 3 {
		middle = strings.Join(fields[2:], " ")
	}
	return last, first, middle, nil
}

// NormalizeSNILS accepts an arbitrary SNILS string (with dashes, spaces,
// "control-digits", etc.) and returns:
//   - formatted: XXX-XXX-XXX YY where possible
//   - digitsOnly: 11 digits, or "" if the input is empty/invalid
//   - err: nil if formatted; non-nil if the digits don't form a valid SNILS.
//
// A SNILS is valid iff it has exactly 11 digits. We do not implement the
// checksum-99/101 mod rule because Mintrud's existing XLSX imports
// contain pre-validated values; rejecting them on checksum would create
// unnecessary friction for the operator.
func NormalizeSNILS(raw string) (formatted, digitsOnly string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", &NormalizeError{Fields: map[string]string{
			"snils": "Укажите СНИЛС.",
		}}
	}

	digitsOnly = digits(trimmed)
	if len(digitsOnly) != 11 {
		return "", "", &NormalizeError{Fields: map[string]string{
			"snils": "СНИЛС должен содержать 11 цифр.",
		}}
	}

	// Best-effort formatting: XXX-XXX-XXX YY. If something goes wrong
	// (shouldn't, given the digit-only length) we return the digits
	// string instead so the caller never has to deal with empty output.
	formatted = fmt.Sprintf("%s-%s-%s %s",
		digitsOnly[0:3], digitsOnly[3:6], digitsOnly[6:9], digitsOnly[9:11])
	return formatted, digitsOnly, nil
}

// NormalizeEmail trims, lowercases, and validates an email address.
// Returns the canonical form on success or a NormalizeError on failure.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", &NormalizeError{Fields: map[string]string{
			"email": "Укажите email.",
		}}
	}
	lowered := strings.ToLower(trimmed)

	addr, err := mail.ParseAddress(lowered)
	if err != nil || addr.Address != lowered {
		return "", &NormalizeError{Fields: map[string]string{
			"email": "Некорректный формат email.",
		}}
	}
	return lowered, nil
}

// NormalizeProgramCode uppercases + trims a program code. Empty
// programs are silently dropped by the parser; this helper is used to
// canonicalize whatever survives that filter.
func NormalizeProgramCode(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

// NormalizePosition trims a position string. Empty values are returned
// as-is so the caller can decide whether to reject them.
func NormalizePosition(raw string) string {
	return strings.TrimSpace(raw)
}

// NormalizeRow runs all the per-field normalizers and aggregates any
// per-field errors into a single NormalizeError.
func NormalizeRow(p ParsedRow) (NormalizedRow, error) {
	last, first, middle, nameErr := NormalizeFullName(p.RawFullName)
	snilsFmt, snilsDigits, snilsErr := NormalizeSNILS(p.RawSNILS)
	email, emailErr := NormalizeEmail(p.RawEmail)
	position := NormalizePosition(p.RawPosition)

	if nameErr != nil || snilsErr != nil || emailErr != nil {
		fields := map[string]string{}
		for _, e := range []error{nameErr, snilsErr, emailErr} {
			var ne *NormalizeError
			if errors.As(e, &ne) {
				for k, v := range ne.Fields {
					fields[k] = v
				}
			}
		}
		return NormalizedRow{}, &NormalizeError{Fields: fields}
	}

	return NormalizedRow{
		LastName:    last,
		FirstName:   first,
		MiddleName:  middle,
		SNILS:       snilsFmt,
		SNILSDigits: snilsDigits,
		Email:       email,
		Position:    position,
	}, nil
}

// digits returns only the unicode-digit runes in s. We use unicode.IsDigit
// rather than `r >= '0' && r <= '9'` so that Cyrillic full-width digits
// or other locales are handled consistently — in practice the XLSX files
// Mintrud sends use ASCII digits, but the cost of being lenient is zero.
func digits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
