package legacy

import (
	"errors"
	"reflect"
	"testing"
)

var syntheticCommonHeaders = []string{
	"Организация",
	"ИНН",
	"Ф.И.О.",
	"Профессия",
	"Подразделение",
	"СНИЛС",
	"Номер протокола",
	"Дата протокола",
	"Оценка",
	"Период обучения",
	"Номер в реестре",
	"Организация/филиал/№ заявки",
	"Email",
	"Логин",
	"Пароль",
}

func TestNormalizeLabel(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"  Ф.И.О.\n":   "ф и о",
		"E-mail":       "e mail",
		"№  протокола": "номер протокола",
		"Организация / филиал / № заявки":     "организация филиал номер заявки",
		"Результат\tпроверки—знаний":          "результат проверки знаний",
		"  Электронная\u00a0почта (Moodle)  ": "электронная почта moodle",
	}
	for input, want := range tests {
		if got := normalizeLabel(input); got != want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProfileForSheetNormalizesWhitespaceAndCase(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]SheetProfile{
		" А ": SheetA,
		"б":   SheetB,
		"В":   SheetV,
		" п ": SheetP,
		"с":   SheetS,
	} {
		got, ok := profileForSheet(input)
		if !ok || got != want {
			t.Errorf("profileForSheet(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestBuildSheetPlanMatchesIndustrialAliases(t *testing.T) {
	t.Parallel()

	headers := append([]string(nil), syntheticCommonHeaders...)
	headers[0] = "Наименование организации"
	headers[2] = "ФИО"
	headers[8] = "Результат проверки знаний"
	headers[10] = "Номер записи в реестре"
	headers[12] = "Электронная почта (Moodle)"

	plan, err := buildSheetPlan(SheetA, "А", 1, 3, headers)
	if err != nil {
		t.Fatalf("build sheet plan: %v", err)
	}
	if plan.HeaderRow != 3 || plan.Profile != SheetA || plan.Order != 1 {
		t.Fatalf("plan metadata = %+v", plan)
	}
	want := map[int]Field{
		0: FieldEmployer, 1: FieldINN, 2: FieldFullName, 3: FieldPosition,
		4: FieldDepartment, 5: FieldSNILS, 6: FieldProtocolNumber,
		7: FieldProtocolDate, 8: FieldAssessment, 9: FieldTrainingPeriod,
		10: FieldRegistryNumber, 11: FieldSourceRef, 12: FieldMoodleEmail,
		13: FieldMoodleUsername,
	}
	if !reflect.DeepEqual(plan.HeaderMap, want) {
		t.Fatalf("header map = %#v, want %#v", plan.HeaderMap, want)
	}
	if _, exists := plan.ExtraNames[14]; exists {
		t.Fatal("password column was retained as an extra field")
	}
}

func TestBuildSheetPlanMatchesProductionWorkbookAliases(t *testing.T) {
	t.Parallel()

	headers := append([]string(nil), syntheticCommonHeaders...)
	headers[8] = "Оценка в протоколе"
	headers[10] = "Номер в реестре Минтруда"
	headers[11] = "Организация/филилал/№ заявки"
	headers[12] = "E-mail для открытия доступа на уч. портал"
	plan, err := buildSheetPlan(SheetA, "А", 1, 1, headers)
	if err != nil {
		t.Fatalf("build production alias plan: %v", err)
	}
	for column, want := range map[int]Field{
		8: FieldAssessment, 10: FieldRegistryNumber,
		11: FieldSourceRef, 12: FieldMoodleEmail,
	} {
		if plan.HeaderMap[column] != want {
			t.Errorf("column %d = %q, want %q", column+1, plan.HeaderMap[column], want)
		}
	}
}

func TestBuildSheetPlanConfinesMalformedProductionHeadersToProfileP(t *testing.T) {
	t.Parallel()

	headers := append([]string(nil), syntheticCommonHeaders...)
	headers[7] = "01:00:00"
	headers[8] = "0"
	headers = append(headers, "Оказание первой помощи пострадавшим")
	plan, err := buildSheetPlan(SheetP, "П", 4, 1, headers)
	if err != nil {
		t.Fatalf("build production profile P plan: %v", err)
	}
	if plan.HeaderMap[7] != FieldProtocolDate || plan.HeaderMap[8] != FieldAssessment || plan.HeaderMap[len(headers)-1] != FieldProgram {
		t.Fatalf("profile P mappings = %#v", plan.HeaderMap)
	}

	if _, err := buildSheetPlan(SheetA, "А", 1, 1, headers); err == nil {
		t.Fatal("malformed profile P headers were accepted for profile A")
	}
}

func TestBuildSheetPlanRejectsMissingCommonCore(t *testing.T) {
	t.Parallel()

	headers := append([]string(nil), syntheticCommonHeaders...)
	headers[5] = ""
	_, err := buildSheetPlan(SheetA, "А", 1, 1, headers)
	assertParseErrorCode(t, err, CodeMissingColumns)
}

func TestSheetVRequiresExplicitProgramColumn(t *testing.T) {
	t.Parallel()

	_, err := buildSheetPlan(SheetV, "В", 3, 1, syntheticCommonHeaders)
	assertParseErrorCode(t, err, CodeMissingColumns)

	headers := append([]string(nil), syntheticCommonHeaders...)
	headers = append(headers, "Программа обучения")
	plan, err := buildSheetPlan(SheetV, "В", 3, 1, headers)
	if err != nil {
		t.Fatalf("build sheet V plan: %v", err)
	}
	if plan.HeaderMap[len(headers)-1] != FieldProgram {
		t.Fatalf("program column mapping = %q", plan.HeaderMap[len(headers)-1])
	}
}

func TestProfileSpecificProgramHeadersAreRecognized(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		profile SheetProfile
		header  string
	}{
		{SheetP, "Обучение по оказанию первой помощи пострадавшим"},
		{SheetS, "Использование (применение) средств индивидуальной защиты"},
	} {
		headers := append(append([]string(nil), syntheticCommonHeaders...), tc.header)
		plan, err := buildSheetPlan(tc.profile, string(tc.profile), 1, 1, headers)
		if err != nil {
			t.Fatalf("build profile %s plan: %v", tc.profile, err)
		}
		if plan.HeaderMap[len(headers)-1] != FieldProgram {
			t.Errorf("profile %s program mapping = %q", tc.profile, plan.HeaderMap[len(headers)-1])
		}
	}
}

func TestSecretHeadersAreNeverExtraFields(t *testing.T) {
	t.Parallel()

	for _, secretHeader := range []string{"Пароль", "Password", "passwd", "Access token", "API key", "Ключ доступа"} {
		headers := append(append([]string(nil), syntheticCommonHeaders[:14]...), secretHeader)
		plan, err := buildSheetPlan(SheetA, "А", 1, 1, headers)
		if err != nil {
			t.Fatalf("header %q: %v", secretHeader, err)
		}
		if _, exists := plan.ExtraNames[len(headers)-1]; exists {
			t.Errorf("secret header %q retained as extra", secretHeader)
		}
	}
}

func TestUnknownNonSecretHeaderBecomesExtraField(t *testing.T) {
	t.Parallel()

	headers := append(append([]string(nil), syntheticCommonHeaders...), "Внутренний комментарий")
	plan, err := buildSheetPlan(SheetA, "А", 1, 1, headers)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.ExtraNames[len(headers)-1]; got != "внутренний комментарий" {
		t.Fatalf("extra field name = %q", got)
	}
}

func TestBuildSheetPlanRejectsDuplicateLogicalColumn(t *testing.T) {
	t.Parallel()

	headers := append(append([]string(nil), syntheticCommonHeaders...), "ФИО")
	_, err := buildSheetPlan(SheetA, "А", 1, 1, headers)
	assertParseErrorCode(t, err, CodeUnsupportedWorkbook)
}

func assertParseErrorCode(t *testing.T, err error, want ErrorCode) *ParseError {
	t.Helper()
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want *ParseError", err)
	}
	if parseErr.Code != want {
		t.Fatalf("error code = %q, want %q", parseErr.Code, want)
	}
	return parseErr
}
