// Package legacy parses the historical multi-sheet customer registry.
package legacy

// Limits bounds both the compressed file and the workbook content inspected
// by preflight and Parse.
type Limits struct {
	MaxFileBytes         int64
	MaxUncompressedBytes uint64
	MaxZIPEntries        int
	MaxSheets            int
	MaxRows              int64
	MaxCells             int64
	MaxCellBytes         int
	MaxHeaderRows        int
}

// DefaultLimits returns the defensive limits agreed for the legacy registry.
func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:         50 << 20,
		MaxUncompressedBytes: 256 << 20,
		MaxZIPEntries:        4096,
		MaxSheets:            32,
		MaxRows:              200_000,
		MaxCells:             5_000_000,
		MaxCellBytes:         32 << 10,
		MaxHeaderRows:        20,
	}
}

// SheetProfile identifies one of the five supported industrial sheets.
type SheetProfile string

const (
	SheetA SheetProfile = "А"
	SheetB SheetProfile = "Б"
	SheetV SheetProfile = "В"
	SheetP SheetProfile = "П"
	SheetS SheetProfile = "С"
)

// Field is a safe logical destination for a recognized workbook column.
type Field string

const (
	FieldEmployer       Field = "employer"
	FieldINN            Field = "inn"
	FieldFullName       Field = "full_name"
	FieldPosition       Field = "position"
	FieldDepartment     Field = "department"
	FieldSNILS          Field = "snils"
	FieldProgram        Field = "program"
	FieldTrainingPeriod Field = "training_period"
	FieldProtocolNumber Field = "protocol_number"
	FieldProtocolDate   Field = "protocol_date"
	FieldAssessment     Field = "assessment_result"
	FieldRegistryNumber Field = "mintrud_registry_number"
	FieldSourceRef      Field = "source_reference"
	FieldMoodleEmail    Field = "moodle_email"
	FieldMoodleUsername Field = "moodle_username"
)

// SheetPlan records the structural information discovered during preflight.
// Column indexes are zero-based; HeaderRow is one-based like Excel rows.
type SheetPlan struct {
	Name          string         `json:"name"`
	Order         int            `json:"order"`
	Profile       SheetProfile   `json:"profile"`
	HeaderRow     int64          `json:"header_row"`
	HeaderMap     map[int]Field  `json:"header_map"`
	ExtraNames    map[int]string `json:"extra_names,omitempty"`
	secretColumns map[int]struct{}
}

// WorkbookPlan is safe to persist: it contains headers and positions but no
// business cell values.
type WorkbookPlan struct {
	Sheets []SheetPlan `json:"sheets"`
}

// SourceRow is one cleaned historical row. It deliberately has no password or
// generic secret field.
type SourceRow struct {
	SheetName               string            `json:"sheet_name"`
	SheetProfile            SheetProfile      `json:"sheet_profile"`
	RowNumber               int64             `json:"row_number"`
	EmployerName            string            `json:"employer_name,omitempty"`
	INN                     string            `json:"inn,omitempty"`
	FullName                string            `json:"full_name,omitempty"`
	Position                string            `json:"position,omitempty"`
	Department              string            `json:"department,omitempty"`
	SNILS                   string            `json:"snils,omitempty"`
	ProgramText             string            `json:"program_text,omitempty"`
	TrainingPeriod          string            `json:"training_period,omitempty"`
	ProtocolNumber          string            `json:"protocol_number,omitempty"`
	ProtocolDate            string            `json:"protocol_date,omitempty"`
	AssessmentResult        string            `json:"assessment_result,omitempty"`
	MintrudRegistryNumber   string            `json:"mintrud_registry_number,omitempty"`
	SourceReference         string            `json:"source_reference,omitempty"`
	MoodleEmail             string            `json:"moodle_email,omitempty"`
	MoodleUsername          string            `json:"moodle_username,omitempty"`
	ExtraFields             map[string]string `json:"extra_fields,omitempty"`
	SourceFingerprintSHA256 string            `json:"source_fingerprint_sha256"`
}

// ParseStats describes emitted rows and all cells observed while streaming.
type ParseStats struct {
	Rows  int64
	Cells int64
}
