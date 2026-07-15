package legacy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
)

func TestDefaultLimitsMatchImportContract(t *testing.T) {
	t.Parallel()

	got := legacy.DefaultLimits()
	if got.MaxFileBytes != 50<<20 || got.MaxUncompressedBytes != 256<<20 {
		t.Fatalf("byte limits = %+v", got)
	}
	if got.MaxZIPEntries != 4096 || got.MaxSheets != 32 || got.MaxRows != 200_000 || got.MaxCells != 5_000_000 || got.MaxCellBytes != 32<<10 || got.MaxHeaderRows != 20 {
		t.Fatalf("workbook limits = %+v", got)
	}
}

func TestSourceRowJSONContractHasNoSecretField(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(legacy.SourceRow{SheetName: "А", RowNumber: 2})
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{"password", "пароль", "secret", "token"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("source row JSON contains %q", forbidden)
		}
	}
}
