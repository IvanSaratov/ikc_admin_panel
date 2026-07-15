package legacy

import (
	"strings"
	"unicode"
)

var headerAliases = map[string]Field{
	"организация":              FieldEmployer,
	"наименование организации": FieldEmployer,
	"работодатель":             FieldEmployer,
	"инн":                      FieldINN,
	"ф и о":                    FieldFullName,
	"фио":                      FieldFullName,
	"фамилия имя отчество":     FieldFullName,
	"профессия":                FieldPosition,
	"должность":                FieldPosition,
	"профессия должность":      FieldPosition,
	"подразделение":            FieldDepartment,
	"снилс":                    FieldSNILS,
	"программа":                FieldProgram,
	"программа обучения":       FieldProgram,
	"наименование программы":   FieldProgram,
	"обучение по оказанию первой помощи пострадавшим": FieldProgram,
	"оказание первой помощи пострадавшим":             FieldProgram,
	"первая помощь": FieldProgram,
	"использование применение средств индивидуальной защиты": FieldProgram,
	"средства индивидуальной защиты":                         FieldProgram,
	"сиз":             FieldProgram,
	"номер протокола": FieldProtocolNumber,
	"номер протокола проверки знаний": FieldProtocolNumber,
	"протокол":                                 FieldProtocolNumber,
	"дата протокола":                           FieldProtocolDate,
	"оценка":                                   FieldAssessment,
	"оценка в протоколе":                       FieldAssessment,
	"результат":                                FieldAssessment,
	"результат проверки знаний":                FieldAssessment,
	"период обучения":                          FieldTrainingPeriod,
	"сроки обучения":                           FieldTrainingPeriod,
	"номер в реестре":                          FieldRegistryNumber,
	"номер в реестре минтруда":                 FieldRegistryNumber,
	"номер в реестре обученных лиц":            FieldRegistryNumber,
	"номер записи в реестре":                   FieldRegistryNumber,
	"организация филиал номер заявки":          FieldSourceRef,
	"организация филилал номер заявки":         FieldSourceRef,
	"организация филиал заявка":                FieldSourceRef,
	"филиал заявка":                            FieldSourceRef,
	"номер заявки":                             FieldSourceRef,
	"email":                                    FieldMoodleEmail,
	"e mail":                                   FieldMoodleEmail,
	"электронная почта":                        FieldMoodleEmail,
	"электронная почта moodle":                 FieldMoodleEmail,
	"e mail для открытия доступа на уч портал": FieldMoodleEmail,
	"логин":    FieldMoodleUsername,
	"username": FieldMoodleUsername,
}

var profileHeaderAliases = map[SheetProfile]map[string]Field{
	SheetP: {
		"01 00 00": FieldProtocolDate,
		"0":        FieldAssessment,
	},
}

var commonRequiredFields = []Field{
	FieldEmployer,
	FieldINN,
	FieldFullName,
	FieldPosition,
	FieldSNILS,
	FieldProtocolNumber,
	FieldProtocolDate,
	FieldAssessment,
	FieldTrainingPeriod,
	FieldRegistryNumber,
	FieldSourceRef,
}

func normalizeLabel(value string) string {
	value = strings.ReplaceAll(value, "№", " номер ")
	var b strings.Builder
	needSpace := false
	for _, r := range strings.ToLower(value) {
		if r == 'ё' {
			r = 'е'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if needSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			needSpace = false
			continue
		}
		needSpace = true
	}
	return b.String()
}

func profileForSheet(name string) (SheetProfile, bool) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case string(SheetA):
		return SheetA, true
	case string(SheetB):
		return SheetB, true
	case string(SheetV):
		return SheetV, true
	case string(SheetP):
		return SheetP, true
	case string(SheetS):
		return SheetS, true
	default:
		return "", false
	}
}

func isSecretHeader(header string) bool {
	normalized := normalizeLabel(header)
	if normalized == "" {
		return false
	}
	if normalized == "api key" || normalized == "ключ доступа" {
		return true
	}
	for _, word := range strings.Fields(normalized) {
		switch word {
		case "пароль", "password", "passwd", "secret", "token":
			return true
		}
	}
	return false
}

func buildSheetPlan(profile SheetProfile, name string, order int, headerRow int64, headers []string) (SheetPlan, error) {
	plan := SheetPlan{
		Name:          name,
		Order:         order,
		Profile:       profile,
		HeaderRow:     headerRow,
		HeaderMap:     make(map[int]Field),
		ExtraNames:    make(map[int]string),
		secretColumns: make(map[int]struct{}),
	}
	seen := make(map[Field]struct{})
	for index, rawHeader := range headers {
		normalized := normalizeLabel(rawHeader)
		if normalized == "" {
			continue
		}
		if isSecretHeader(normalized) {
			plan.secretColumns[index] = struct{}{}
			continue
		}
		field, recognized := fieldForHeader(profile, normalized)
		if !recognized {
			plan.ExtraNames[index] = normalized
			continue
		}
		if _, duplicate := seen[field]; duplicate {
			return SheetPlan{}, &ParseError{
				Code:   CodeUnsupportedWorkbook,
				Sheet:  name,
				Row:    headerRow,
				Detail: "duplicate logical column",
			}
		}
		seen[field] = struct{}{}
		plan.HeaderMap[index] = field
	}

	required := append([]Field(nil), commonRequiredFields...)
	if profile == SheetV {
		required = append(required, FieldProgram)
	}
	for _, field := range required {
		if _, ok := seen[field]; !ok {
			return SheetPlan{}, &ParseError{
				Code:   CodeMissingColumns,
				Sheet:  name,
				Row:    headerRow,
				Detail: "required columns were not recognized",
			}
		}
	}
	if len(plan.ExtraNames) == 0 {
		plan.ExtraNames = nil
	}
	if len(plan.secretColumns) == 0 {
		plan.secretColumns = nil
	}
	return plan, nil
}

func fieldForHeader(profile SheetProfile, normalized string) (Field, bool) {
	if field, ok := headerAliases[normalized]; ok {
		return field, true
	}
	field, ok := profileHeaderAliases[profile][normalized]
	return field, ok
}
