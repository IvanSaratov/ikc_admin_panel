package legacy

import (
	"errors"
	"strings"
	"testing"
)

func TestSheetPlanCodecRoundTripPreservesSafeStructure(t *testing.T) {
	t.Parallel()

	plan := SheetPlan{
		Name:          "А",
		Order:         1,
		Profile:       SheetA,
		HeaderRow:     3,
		HeaderMap:     map[int]Field{0: FieldEmployer, 1: FieldINN},
		ExtraNames:    map[int]string{3: "дополнительное поле"},
		secretColumns: map[int]struct{}{2: {}},
	}

	payload, err := EncodeSheetPlan(plan)
	if err != nil {
		t.Fatalf("encode sheet plan: %v", err)
	}
	for _, forbidden := range []string{"пароль", "password", "secret-value"} {
		if strings.Contains(strings.ToLower(payload), forbidden) {
			t.Fatalf("payload contains forbidden marker %q: %s", forbidden, payload)
		}
	}

	restored, err := DecodeSheetPlan("А", 1, SheetA, payload)
	if err != nil {
		t.Fatalf("decode sheet plan: %v", err)
	}
	if restored.Name != plan.Name || restored.Order != plan.Order || restored.Profile != plan.Profile || restored.HeaderRow != plan.HeaderRow {
		t.Fatalf("restored identity = %+v, want %+v", restored, plan)
	}
	if restored.HeaderMap[0] != FieldEmployer || restored.HeaderMap[1] != FieldINN || restored.ExtraNames[3] != "дополнительное поле" {
		t.Fatalf("restored mappings = fields %#v extras %#v", restored.HeaderMap, restored.ExtraNames)
	}
	if _, ok := restored.secretColumns[2]; !ok {
		t.Fatalf("secret column was not restored: %+v", restored)
	}
}

func TestSheetPlanCodecRejectsIncompatiblePayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sheet   string
		order   int
		profile SheetProfile
		payload string
	}{
		{name: "versionless", sheet: "А", order: 1, profile: SheetA, payload: `{"header_row":1,"fields":{"0":"employer"}}`},
		{name: "unknown version", sheet: "А", order: 1, profile: SheetA, payload: `{"version":2,"header_row":1,"fields":{"0":"employer"}}`},
		{name: "negative column", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"-1":"employer"}}`},
		{name: "overlapping secret", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"},"secret_columns":[0]}`},
		{name: "overlapping extra", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"},"extra_fields":{"0":"note"}}`},
		{name: "unknown field", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"unknown"}}`},
		{name: "duplicate logical field", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer","1":"employer"}}`},
		{name: "secret extra header", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"},"extra_fields":{"1":"password"}}`},
		{name: "invalid header row", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":0,"fields":{"0":"employer"}}`},
		{name: "duplicate secret", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"},"secret_columns":[1,1]}`},
		{name: "unknown property", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"},"raw_data":"forbidden"}`},
		{name: "trailing json", sheet: "А", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"}} {}`},
		{name: "empty sheet name", sheet: "", order: 1, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"}}`},
		{name: "invalid order", sheet: "А", order: 0, profile: SheetA, payload: `{"version":1,"header_row":1,"fields":{"0":"employer"}}`},
		{name: "invalid profile", sheet: "А", order: 1, profile: "X", payload: `{"version":1,"header_row":1,"fields":{"0":"employer"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeSheetPlan(test.sheet, test.order, test.profile, test.payload)
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != CodeUnsupportedWorkbook {
				t.Fatalf("error = %v, want unsupported workbook ParseError", err)
			}
			if strings.Contains(parseErr.Error(), test.payload) {
				t.Fatalf("error exposed plan payload: %v", parseErr)
			}
		})
	}
}

func TestSheetPlanCodecRejectsInvalidInMemoryPlan(t *testing.T) {
	t.Parallel()

	_, err := EncodeSheetPlan(SheetPlan{
		Name:          "А",
		Order:         1,
		Profile:       SheetA,
		HeaderRow:     1,
		HeaderMap:     map[int]Field{0: FieldEmployer},
		secretColumns: map[int]struct{}{0: {}},
	})
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Code != CodeUnsupportedWorkbook {
		t.Fatalf("error = %v, want unsupported workbook ParseError", err)
	}
}
