package imports_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
)

func TestDefaultConfigMatchesQueueContract(t *testing.T) {
	t.Parallel()

	got := imports.DefaultConfig()
	if got.ActiveQueueLimit != 5 || got.FileTTL != 24*time.Hour {
		t.Fatalf("config = %+v", got)
	}
	if got.LegacyLimits != legacy.DefaultLimits() {
		t.Fatalf("legacy limits = %+v", got.LegacyLimits)
	}
}

func TestServiceErrorDoesNotExposeCause(t *testing.T) {
	t.Parallel()

	err := &imports.ServiceError{
		Code:   imports.CodeStorageUnavailable,
		Detail: "temporary storage unavailable",
		Err:    errors.New("SENSITIVE-INTERNAL-PATH"),
	}
	if strings.Contains(err.Error(), "SENSITIVE-INTERNAL-PATH") {
		t.Fatal("service error exposed internal cause")
	}
	if !errors.Is(err, err.Err) {
		t.Fatal("service error does not unwrap its cause")
	}
}

func TestServiceErrorIncludesExistingImportIDWithoutCause(t *testing.T) {
	t.Parallel()

	err := &imports.ServiceError{
		Code:             imports.CodeDuplicateFile,
		Detail:           "file was already imported",
		ExistingImportID: 42,
		Err:              errors.New("private database detail"),
	}
	message := err.Error()
	if !strings.Contains(message, "existing_import_id=42") || strings.Contains(message, "private database detail") {
		t.Fatalf("safe error message = %q", message)
	}
}
