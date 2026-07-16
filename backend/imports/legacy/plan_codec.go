package legacy

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
)

const storedSheetPlanVersion = 1

type storedSheetPlan struct {
	Version       int            `json:"version"`
	HeaderRow     int64          `json:"header_row"`
	Fields        map[int]Field  `json:"fields"`
	ExtraFields   map[int]string `json:"extra_fields,omitempty"`
	SecretColumns []int          `json:"secret_columns,omitempty"`
}

// EncodeSheetPlan serializes only structural parser metadata. Secret column
// positions are preserved, while their headers and cell values are omitted.
func EncodeSheetPlan(plan SheetPlan) (string, error) {
	stored := storedSheetPlan{
		Version:       storedSheetPlanVersion,
		HeaderRow:     plan.HeaderRow,
		Fields:        copyFieldMap(plan.HeaderMap),
		ExtraFields:   copyStringMap(plan.ExtraNames),
		SecretColumns: make([]int, 0, len(plan.secretColumns)),
	}
	for column := range plan.secretColumns {
		stored.SecretColumns = append(stored.SecretColumns, column)
	}
	sort.Ints(stored.SecretColumns)
	if err := validateStoredSheetPlan(plan.Name, plan.Order, plan.Profile, stored); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return "", invalidStoredPlan("sheet plan cannot be encoded", err)
	}
	return string(encoded), nil
}

// DecodeSheetPlan restores a parser plan from the exact supported wire
// version. Versionless intermediate payloads are rejected explicitly.
func DecodeSheetPlan(name string, order int, profile SheetProfile, payload string) (SheetPlan, error) {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	var stored storedSheetPlan
	if err := decoder.Decode(&stored); err != nil {
		return SheetPlan{}, invalidStoredPlan("sheet plan cannot be decoded", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SheetPlan{}, invalidStoredPlan("sheet plan has trailing data", err)
	}
	if err := validateStoredSheetPlan(name, order, profile, stored); err != nil {
		return SheetPlan{}, err
	}

	secretColumns := make(map[int]struct{}, len(stored.SecretColumns))
	for _, column := range stored.SecretColumns {
		secretColumns[column] = struct{}{}
	}
	if len(secretColumns) == 0 {
		secretColumns = nil
	}
	return SheetPlan{
		Name:          name,
		Order:         order,
		Profile:       profile,
		HeaderRow:     stored.HeaderRow,
		HeaderMap:     copyFieldMap(stored.Fields),
		ExtraNames:    copyStringMap(stored.ExtraFields),
		secretColumns: secretColumns,
	}, nil
}

func validateStoredSheetPlan(name string, order int, profile SheetProfile, stored storedSheetPlan) error {
	resolvedProfile, knownProfile := profileForSheet(name)
	if strings.TrimSpace(name) == "" || order <= 0 || !knownProfile || resolvedProfile != profile {
		return invalidStoredPlan("sheet plan identity is invalid", nil)
	}
	if stored.Version != storedSheetPlanVersion {
		return invalidStoredPlan("sheet plan version is unsupported", nil)
	}
	if stored.HeaderRow <= 0 || len(stored.Fields) == 0 {
		return invalidStoredPlan("sheet plan structure is invalid", nil)
	}

	occupied := make(map[int]struct{}, len(stored.Fields)+len(stored.ExtraFields)+len(stored.SecretColumns))
	assignedFields := make(map[Field]struct{}, len(stored.Fields))
	for column, field := range stored.Fields {
		if column < 0 || !validStoredField(field) {
			return invalidStoredPlan("sheet plan field mapping is invalid", nil)
		}
		if _, duplicate := occupied[column]; duplicate {
			return invalidStoredPlan("sheet plan column is assigned more than once", nil)
		}
		if _, duplicate := assignedFields[field]; duplicate {
			return invalidStoredPlan("sheet plan field is assigned more than once", nil)
		}
		occupied[column] = struct{}{}
		assignedFields[field] = struct{}{}
	}
	for column, name := range stored.ExtraFields {
		if column < 0 || strings.TrimSpace(name) == "" || normalizeLabel(name) != name || isSecretHeader(name) {
			return invalidStoredPlan("sheet plan extra mapping is invalid", nil)
		}
		if _, duplicate := occupied[column]; duplicate {
			return invalidStoredPlan("sheet plan column is assigned more than once", nil)
		}
		occupied[column] = struct{}{}
	}
	for _, column := range stored.SecretColumns {
		if column < 0 {
			return invalidStoredPlan("sheet plan secret mapping is invalid", nil)
		}
		if _, duplicate := occupied[column]; duplicate {
			return invalidStoredPlan("sheet plan column is assigned more than once", nil)
		}
		occupied[column] = struct{}{}
	}
	return nil
}

func validStoredField(field Field) bool {
	switch field {
	case FieldEmployer,
		FieldINN,
		FieldFullName,
		FieldPosition,
		FieldDepartment,
		FieldSNILS,
		FieldProgram,
		FieldTrainingPeriod,
		FieldProtocolNumber,
		FieldProtocolDate,
		FieldAssessment,
		FieldRegistryNumber,
		FieldSourceRef,
		FieldMoodleEmail,
		FieldMoodleUsername:
		return true
	default:
		return false
	}
}

func copyFieldMap(source map[int]Field) map[int]Field {
	if len(source) == 0 {
		return nil
	}
	copy := make(map[int]Field, len(source))
	for column, field := range source {
		copy[column] = field
	}
	return copy
}

func copyStringMap(source map[int]string) map[int]string {
	if len(source) == 0 {
		return nil
	}
	copy := make(map[int]string, len(source))
	for column, value := range source {
		copy[column] = value
	}
	return copy
}

func invalidStoredPlan(detail string, err error) *ParseError {
	return &ParseError{
		Code:   CodeUnsupportedWorkbook,
		Detail: detail,
		Err:    err,
	}
}
