package legacy_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
)

func TestIndustrialWorkbookCompatibility(t *testing.T) {
	path := os.Getenv("IKC_TEST_LEGACY_XLSX")
	if path == "" {
		t.Skip("IKC_TEST_LEGACY_XLSX is not set")
	}

	limits := legacy.DefaultLimits()
	plan, err := legacy.Preflight(context.Background(), path, limits)
	if err != nil {
		t.Fatalf("preflight industrial workbook: %v", err)
	}
	if len(plan.Sheets) != 5 {
		t.Fatalf("sheet count = %d, want 5", len(plan.Sheets))
	}
	wantProfiles := map[legacy.SheetProfile]bool{
		legacy.SheetA: false,
		legacy.SheetB: false,
		legacy.SheetV: false,
		legacy.SheetP: false,
		legacy.SheetS: false,
	}
	for _, sheet := range plan.Sheets {
		wantProfiles[sheet.Profile] = true
	}
	for profile, found := range wantProfiles {
		if !found {
			t.Fatalf("required profile %q was not recognized", profile)
		}
	}

	stats, err := legacy.Parse(context.Background(), path, plan, limits, func(_ context.Context, row legacy.SourceRow) error {
		if row.SourceFingerprintSHA256 == "" {
			return errors.New("empty source fingerprint")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parse industrial workbook: %v", err)
	}
	if stats.Rows == 0 {
		t.Fatal("industrial workbook produced no rows")
	}
}
