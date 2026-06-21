package requests

import (
	"errors"
	"testing"
)

func TestNormalizeFullName_TwoPartsOK(t *testing.T) {
	t.Parallel()
	last, first, middle, err := NormalizeFullName("Иванов Иван")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if last != "Иванов" || first != "Иван" || middle != "" {
		t.Fatalf("got (%q, %q, %q), want (Иванов, Иван, \"\")", last, first, middle)
	}
}

func TestNormalizeFullName_ThreePartsOK(t *testing.T) {
	t.Parallel()
	last, first, middle, err := NormalizeFullName("Иванов Иван Иванович")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if last != "Иванов" || first != "Иван" || middle != "Иванович" {
		t.Fatalf("got (%q, %q, %q)", last, first, middle)
	}
}

func TestNormalizeFullName_CollapsesWhitespace(t *testing.T) {
	t.Parallel()
	last, first, middle, err := NormalizeFullName("  Петров   Петр   Сергеевич  ")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if last != "Петров" || first != "Петр" || middle != "Сергеевич" {
		t.Fatalf("got (%q, %q, %q)", last, first, middle)
	}
}

func TestNormalizeFullName_EmptyErrors(t *testing.T) {
	t.Parallel()
	_, _, _, err := NormalizeFullName("")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
	var ne *NormalizeError
	if !errors.As(err, &ne) {
		t.Fatalf("error is not *NormalizeError: %v", err)
	}
	if ne.Fields["full_name"] == "" {
		t.Errorf("expected full_name field error, got %v", ne.Fields)
	}
}

func TestNormalizeFullName_SinglePartErrors(t *testing.T) {
	t.Parallel()
	_, _, _, err := NormalizeFullName("Одиночка")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeSNILS_FormatsAndDigitOnly(t *testing.T) {
	t.Parallel()
	formatted, digits, err := NormalizeSNILS("123-456-789 00")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if formatted != "123-456-789 00" {
		t.Errorf("formatted = %q, want 123-456-789 00", formatted)
	}
	if digits != "12345678900" {
		t.Errorf("digits = %q, want 12345678900", digits)
	}
}

func TestNormalizeSNILS_StripsSeparators(t *testing.T) {
	t.Parallel()
	_, digits, err := NormalizeSNILS("123 456 789 00")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if digits != "12345678900" {
		t.Errorf("digits = %q, want 12345678900", digits)
	}
}

func TestNormalizeSNILS_TooShortErrors(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeSNILS("12345")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeSNILS_TooLongErrors(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeSNILS("1234567890012")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeSNILS_EmptyErrors(t *testing.T) {
	t.Parallel()
	_, _, err := NormalizeSNILS("")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeEmail_Lowercases(t *testing.T) {
	t.Parallel()
	got, err := NormalizeEmail("  User@Example.COM  ")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "user@example.com" {
		t.Errorf("got %q, want user@example.com", got)
	}
}

func TestNormalizeEmail_InvalidFormatErrors(t *testing.T) {
	t.Parallel()
	_, err := NormalizeEmail("not-an-email")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeEmail_EmptyErrors(t *testing.T) {
	t.Parallel()
	_, err := NormalizeEmail("")
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
}

func TestNormalizeProgramCode_UppercasesAndTrims(t *testing.T) {
	t.Parallel()
	if got := NormalizeProgramCode("  a-1 "); got != "A-1" {
		t.Errorf("got %q, want A-1", got)
	}
}

func TestNormalizeRow_AggregatesErrors(t *testing.T) {
	t.Parallel()
	_, err := NormalizeRow(ParsedRow{
		RawFullName: "Одиночка", // only one part -> error
		RawSNILS:    "",
		RawEmail:    "bad",
	})
	if !errors.Is(err, ErrNormalization) {
		t.Fatalf("err = %v, want ErrNormalization", err)
	}
	var ne *NormalizeError
	if !errors.As(err, &ne) {
		t.Fatalf("error is not *NormalizeError: %v", err)
	}
	for _, key := range []string{"full_name", "snils", "email"} {
		if ne.Fields[key] == "" {
			t.Errorf("expected field error for %q, got %v", key, ne.Fields)
		}
	}
}

func TestNormalizeRow_OK(t *testing.T) {
	t.Parallel()
	n, err := NormalizeRow(ParsedRow{
		RawFullName: "Иванов Иван Иванович",
		RawSNILS:    "123-456-789 00",
		RawEmail:    "Ivanov@Example.com",
		RawPosition: " Инженер ",
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if n.LastName != "Иванов" || n.FirstName != "Иван" || n.MiddleName != "Иванович" {
		t.Errorf("FIO mismatch: %+v", n)
	}
	if n.SNILSDigits != "12345678900" {
		t.Errorf("digits = %q, want 12345678900", n.SNILSDigits)
	}
	if n.Email != "ivanov@example.com" {
		t.Errorf("email = %q, want ivanov@example.com", n.Email)
	}
	if n.Position != "Инженер" {
		t.Errorf("position = %q, want Инженер", n.Position)
	}
}
