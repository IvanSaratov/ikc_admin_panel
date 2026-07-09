package documents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy"
	"github.com/IvanSaratov/ikc_admin_panel/backend/documents/legacy/models"
	"github.com/IvanSaratov/ikc_admin_panel/backend/protocols"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/sirupsen/logrus"
)

// ErrProtocolNotFixed is returned when callers ask to generate a document
// for a protocol that has not yet been Fix()ed (status='draft') or has
// been Cancelled. The Service translates this into a 400 Bad Request.
var ErrProtocolNotFixed = errors.New("protocol is not in a state that permits document generation")

// timeTypeKey returns the Russian category code (А/Б/В/П/С) used by the
// legacy DOCX pipeline as a key into models.TIME_TO_TYPE. The mintrud
// curriculum programs are mapped by their default_hours: 40 -> А (общие
// вопросы), 16 -> Б/В (специальные / повышенной опасности), 8 -> П/С
// (первая помощь / СИЗ). When hours are something else we fall back to А
// to keep the call deterministic and avoid an unknown-key lookup panic in
// the legacy code.
func timeTypeKey(hours int64) string {
	switch {
	case hours >= 40:
		return "А"
	case hours >= 16:
		return "Б"
	case hours >= 8:
		return "П"
	}
	return "А"
}

// renderRegistrySet loads everything the legacy XML/DOCX pipeline needs
// from our DB and translates it into a *legacy.models.RegistrySet.
//
// The mapping is intentionally narrow:
//   - Worker = training_record -> worker_employer -> worker
//   - Organization = the employer attached to that worker_employer
//   - Test = protocol_date / protocol_number + the program's title
//   - IsPassedAttr is hardcoded "true" (legacy always emits it; our schema
//     filters non-passed training records out at the SQL level — only
//     status='active' rows reach the generator).
//
// Returns ErrProtocolNotFixed when the protocol is in draft (cannot
// generate yet) or cancelled. Returns sql.ErrNoRows when the protocol
// itself does not exist (callers should map this to 404).
//
// The function is the single place that touches legacy domain types from
// DB-loaded rows. It is exported so handler/service tests can target it
// directly with a controlled DB fixture.
func renderRegistrySet(ctx context.Context, queries *storagedb.Queries, protocolID int64) (*models.RegistrySet, error) {
	protocol, err := queries.GetProtocolByID(ctx, protocolID)
	if err != nil {
		// Pass sql.ErrNoRows through unchanged so the service can return 404.
		return nil, err
	}

	// Only fixed or later states are eligible to generate. 'cancelled' and
	// 'draft' are rejected; everything in between (xml_uploaded,
	// registry_entered, generated, completed) is allowed — generation is
	// idempotent and operators routinely re-generate after small edits.
	status := protocols.ProtocolStatus(protocol.Status)
	if status == protocols.StatusDraft || status == protocols.StatusCancelled {
		return nil, fmt.Errorf("%w: status=%s", ErrProtocolNotFixed, protocol.Status)
	}

	participants, err := queries.ListProtocolParticipants(ctx, protocolID)
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}

	// Load the protocol's program_group name to drive the LegacyRecord
	// organisation fallback. The Mintrud schema asks for two org blocks
	// (Worker.Employer* + Organization) — for our purposes both reference
	// the same employer, since worker_employer links each training record
	// to exactly one employer.
	group, err := queries.GetProgramGroup(ctx, protocol.ProgramGroupID)
	if err != nil {
		return nil, fmt.Errorf("get program group: %w", err)
	}

	records := make([]*models.RegistryRecord, 0, len(participants))
	for _, part := range participants {
		tr, err := queries.GetTrainingRecord(ctx, part.TrainingRecordID)
		if err != nil {
			return nil, fmt.Errorf("get training record %d: %w", part.TrainingRecordID, err)
		}

		we, err := queries.GetWorkerEmployer(ctx, tr.WorkerEmployerID)
		if err != nil {
			return nil, fmt.Errorf("get worker_employer %d: %w", tr.WorkerEmployerID, err)
		}

		worker, err := queries.GetWorker(ctx, we.WorkerID)
		if err != nil {
			return nil, fmt.Errorf("get worker %d: %w", we.WorkerID, err)
		}

		employer, err := queries.GetEmployer(ctx, we.EmployerID)
		if err != nil {
			return nil, fmt.Errorf("get employer %d: %w", we.EmployerID, err)
		}

		program, err := queries.GetProgram(ctx, tr.ProgramID)
		if err != nil {
			return nil, fmt.Errorf("get program %d: %w", tr.ProgramID, err)
		}

		// Dates: protocol_date / training_start / training_end are stored as
		// 'YYYY-MM-DD' (the format written by ProtocolService.Fix). We parse
		// them as UTC midnight so the resulting time.Time is deterministic
		// across runs (and stable in the golden test fixtures).
		testDate, err := parseISODateOrZero(protocol.ProtocolDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse protocol date %q: %w", protocol.ProtocolDate.String, err)
		}
		eduStart, err := parseISODateOrZero(protocol.TrainingStartDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse training start date %q: %w", protocol.TrainingStartDate.String, err)
		}
		eduEnd, err := parseISODateOrZero(protocol.TrainingEndDate.String)
		if err != nil {
			return nil, fmt.Errorf("parse training end date %q: %w", protocol.TrainingEndDate.String, err)
		}

		middleName := ""
		if worker.MiddleName.Valid {
			middleName = worker.MiddleName.String
		}

		// legacy.isPassed always emits "true"; we honour the same contract
		// and rely on the SQL filter at the participant level (only
		// status='active' rows reach us) to exclude failed trainees.
		_ = group // group kept in scope so future migrations can stamp it on each record

		records = append(records, &models.RegistryRecord{
			Worker: &models.Worker{
				LastName:       worker.LastName,
				FirstName:      worker.FirstName,
				MiddleName:     middleName,
				Snils:          worker.Snils,
				Position:       tr.Position,
				EmployerInn:    employer.Inn,
				EmployerTitle:  employer.CanonicalName,
				IsForeignSnils: "",
				ForeignSnils:   "",
				Citizenship:    "",
			},
			Organization: &models.Organization{
				Inn:   employer.Inn,
				Title: employer.CanonicalName,
			},
			Test: &models.Test{
				IsPassedAttr:       "true",
				LearnProgramIdAttr: program.Code,
				Date:               testDate,
				ProtocolNumber:     protocol.ProtocolNumber.String,
				LearnProgramTitle:  program.Name,
				EducationStart:     eduStart,
				EducationEnd:       eduEnd,
			},
		})
	}

	return &models.RegistrySet{RegistryRecord: records}, nil
}

// parseISODateOrZero returns time.Time{IsZero=true} for empty input (so a
// missing date renders as <Tag></Tag> instead of crashing) and otherwise
// parses 'YYYY-MM-DD' as UTC midnight.
func parseISODateOrZero(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", raw)
}

// Ensure model imports are referenced even when callers only use the
// return type. (Prevents "imported and not used" regressions if the file
// is later trimmed.)
var (
	_ = sql.NullInt64{}
	_ = legacy.GenerateXML
)

// legacyCreateDocx — единственная точка, через которую backend/documents
// вызывает старый DOCX pipeline. Держим ее здесь, а не в docx.go, чтобы
// правило adapter-layer оставалось явным: legacy/* трогают только этот файл и xml.go.
//
// Logrus adapter из backend/documents/legacy/logadapter.go оборачивает runtime
// logger приложения без раскрытия остальной logging-архитектуры legacy-коду.
func legacyCreateDocx(set *models.RegistrySet, template []byte, timeType string, log logrus.FieldLogger) ([][]byte, error) {
	if set == nil {
		return nil, fmt.Errorf("legacyCreateDocx: nil registry set")
	}
	if len(template) == 0 {
		return nil, fmt.Errorf("legacyCreateDocx: empty template")
	}
	return legacy.CreateDocx(set, string(template), timeType, legacy.NewLogrusAdapterWithLogger(log))
}
