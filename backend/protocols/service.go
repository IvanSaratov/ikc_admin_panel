package protocols

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// ErrValidation is the sentinel returned (via errors.Is / errors.As) when a
// caller passed bad input. FieldErrors implements Is(target) so callers can
// branch on this even when the concrete type is FieldErrors.
var ErrValidation = errors.New("validation")

// FieldErrors is a per-field validation error map. Keys are field names
// (snake_case to match the request form values); values are operator-facing
// Russian messages.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	values := make([]string, 0, len(e))
	for _, value := range e {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, " ")
}

func (e FieldErrors) Is(target error) bool { return target == ErrValidation }

// CreateProtocolInput captures the only field required to create a protocol:
// the program group it belongs to. Dates and the number are filled in later
// by Fix.
type CreateProtocolInput struct {
	ProgramGroupID int64
}

// FixInput carries the data needed to assign a protocol number: training and
// protocol dates plus an optional suffix. ProtocolSuffix is the empty string
// (NOT null) when the caller wants a single, non-suffixed protocol.
type FixInput struct {
	TrainingStartDate string
	TrainingEndDate   string
	ProtocolDate      string
	ProtocolSuffix    string
}

// Service is the D2 protocols slice. It is constructed once and shared by
// the HTTP handler and tests. All write paths go through the audit service
// (when configured) so the action_log captures every state change.
type Service struct {
	queries *storagedb.Queries
	db      *sql.DB
	audit   *audit.Service
	now     func() time.Time
}

// NewService wires the dependencies. `audit` is optional — nil is allowed for
// tests that only exercise validation paths and do not care about action_log.
func NewService(queries *storagedb.Queries, database *sql.DB, auditSvc *audit.Service) *Service {
	return &Service{
		queries: queries,
		db:      database,
		audit:   auditSvc,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// CreateProtocol inserts a new draft protocol attached to the given program
// group. Only program_group_id is required; the schema explicitly allows all
// other fields to be NULL until Fix runs.
func (s *Service) CreateProtocol(ctx context.Context, in CreateProtocolInput) (storagedb.Protocol, error) {
	if in.ProgramGroupID <= 0 {
		return storagedb.Protocol{}, FieldErrors{"program_group_id": "Выберите группу программ."}
	}

	// Validate the group actually exists and is active; otherwise the FK would
	// reject the INSERT (which would surface as an opaque conflict error).
	if _, err := s.queries.GetGroupByID(ctx, in.ProgramGroupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storagedb.Protocol{}, FieldErrors{"program_group_id": "Выбранная группа программ не найдена."}
		}
		return storagedb.Protocol{}, fmt.Errorf("get program group: %w", err)
	}

	timestamp := s.timestamp()
	protocol, err := s.queries.CreateProtocol(ctx, storagedb.CreateProtocolParams{
		ProgramGroupID: in.ProgramGroupID,
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	})
	if err != nil {
		return storagedb.Protocol{}, fmt.Errorf("create protocol: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "create",
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: protocol.ID, Valid: true},
		Details: map[string]any{
			"program_group_id": protocol.ProgramGroupID,
			"status":           protocol.Status,
		},
	})
	return protocol, nil
}

// AddParticipant links a training_record to a protocol. The unique-active
// constraint on protocol_participants(training_record_id) WHERE status='active'
// ensures a training_record can appear in at most one ACTIVE protocol at a
// time. We enforce it explicitly here so callers see a clean field error
// instead of a SQLite "UNIQUE constraint failed" message.
//
// Protocol must exist; we look it up first so the error is meaningful.
func (s *Service) AddParticipant(ctx context.Context, protocolID, trainingRecordID int64) error {
	errs := FieldErrors{}
	if protocolID <= 0 {
		errs["protocol_id"] = "Некорректный идентификатор протокола."
	}
	if trainingRecordID <= 0 {
		errs["training_record_id"] = "Некорректный идентификатор записи обучения."
	}
	if len(errs) > 0 {
		return errs
	}

	if _, err := s.queries.GetProtocolByID(ctx, protocolID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FieldErrors{"protocol_id": "Протокол не найден."}
		}
		return fmt.Errorf("get protocol: %w", err)
	}

	// Enforce the unique-active rule at service level BEFORE attempting the
	// INSERT, so the operator sees a clear error tied to a specific field
	// rather than a generic conflict.
	if existing, err := s.queries.GetActiveParticipantForTrainingRecord(ctx, trainingRecordID); err == nil {
		return FieldErrors{
			"training_record_id": fmt.Sprintf(
				"Запись обучения уже добавлена в активный протокол №%d.",
				existing.ProtocolID,
			),
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup active participant: %w", err)
	}

	timestamp := s.timestamp()
	participant, err := s.queries.CreateProtocolParticipant(ctx, storagedb.CreateProtocolParticipantParams{
		ProtocolID:       protocolID,
		TrainingRecordID: trainingRecordID,
		CreatedAt:        timestamp,
		UpdatedAt:        timestamp,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			// Race: another concurrent request added the same training_record
			// between our check and our insert. Re-fetch to give the operator
			// the same friendly message.
			return FieldErrors{
				"training_record_id": "Запись обучения уже добавлена в другой активный протокол.",
			}
		}
		return fmt.Errorf("add participant: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "add_participant",
		EntityType: "protocol_participant",
		EntityID:   sql.NullInt64{Int64: participant.ID, Valid: true},
		Details: map[string]any{
			"protocol_id":        protocolID,
			"training_record_id": trainingRecordID,
		},
	})
	return nil
}

// RemoveParticipant soft-deletes a participant by flipping its status to
// 'removed'. The audit row records which training_record was removed from
// which protocol. Idempotent: removing a non-active row returns nil without
// writing a duplicate audit row.
func (s *Service) RemoveParticipant(ctx context.Context, participantID int64) error {
	if participantID <= 0 {
		return FieldErrors{"participant_id": "Некорректный идентификатор участника."}
	}

	previous, err := s.queries.GetProtocolParticipantByID(ctx, participantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FieldErrors{"participant_id": "Участник не найден."}
		}
		return fmt.Errorf("get participant: %w", err)
	}
	if previous.Status == "removed" {
		return nil
	}

	removed, err := s.queries.MarkParticipantRemoved(ctx, storagedb.MarkParticipantRemovedParams{
		UpdatedAt: s.timestamp(),
		ID:        participantID,
	})
	if err != nil {
		return fmt.Errorf("remove participant: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "remove_participant",
		EntityType: "protocol_participant",
		EntityID:   sql.NullInt64{Int64: removed.ID, Valid: true},
		Details: map[string]any{
			"protocol_id":        removed.ProtocolID,
			"training_record_id": removed.TrainingRecordID,
		},
	})
	return nil
}

// Fix assigns the protocol number and freezes the lifecycle state from
// 'draft' to 'fixed'. Must run inside a transaction so the sequence lookup
// and the status flip are atomic — otherwise two concurrent Fix calls could
// race for the same (program_group_id, sequence_year, annual_sequence_number,
// suffix) slot and one would fail the unique index.
func (s *Service) Fix(ctx context.Context, protocolID int64, in FixInput) (storagedb.Protocol, error) {
	startDate, endDate, protocolDate, suffix, errs := s.validateFixInput(in)
	if len(errs) > 0 {
		return storagedb.Protocol{}, errs
	}

	protocolYear, protocolMonth, err := ParseISODate(protocolDate)
	if err != nil {
		return storagedb.Protocol{}, FieldErrors{"protocol_date": err.Error()}
	}
	year64 := int64(protocolYear)
	month64 := int64(protocolMonth)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storagedb.Protocol{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// All queries we issue from here on must use the same tx. We re-create
	// a thin Queries facade bound to the transaction.
	txQueries := s.queries.WithTx(tx)

	existing, err := txQueries.GetProtocolByID(ctx, protocolID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storagedb.Protocol{}, FieldErrors{"protocol_id": "Протокол не найден."}
		}
		return storagedb.Protocol{}, fmt.Errorf("get protocol: %w", err)
	}
	if existing.Status != string(StatusDraft) {
		return storagedb.Protocol{}, FieldErrors{
			"protocol_id": fmt.Sprintf("Фиксация возможна только из статуса «draft», сейчас «%s».", existing.Status),
		}
	}

	// nextSequenceLocked runs against the transaction-bound facade.
	next, err := nextSequenceLocked(ctx, txQueries, existing.ProgramGroupID, year64)
	if err != nil {
		return storagedb.Protocol{}, err
	}

	fixed, err := txQueries.FixProtocol(ctx, storagedb.FixProtocolParams{
		TrainingStartDate:    sql.NullString{String: startDate, Valid: true},
		TrainingEndDate:      sql.NullString{String: endDate, Valid: true},
		ProtocolDate:         sql.NullString{String: protocolDate, Valid: true},
		SequenceYear:         sql.NullInt64{Int64: year64, Valid: true},
		ProtocolMonth:        sql.NullInt64{Int64: month64, Valid: true},
		AnnualSequenceNumber: sql.NullInt64{Int64: next, Valid: true},
		ProtocolNumber:       sql.NullString{String: FormatProtocolNumber(protocolYear, protocolMonth, next, suffix), Valid: true},
		ProtocolSuffix:       suffixNull(suffix),
		FixedAt:              sql.NullString{String: s.timestamp(), Valid: true},
		UpdatedAt:            s.timestamp(),
		ID:                   protocolID,
	})
	if err != nil {
		if errors.Is(storage.MapSQLiteError(err), storage.ErrConflict) {
			// Re-run triggered by a concurrent fix that took our slot; surface
			// a clear field error so the operator can retry.
			return storagedb.Protocol{}, FieldErrors{
				"protocol_number": "Номер уже занят, попробуйте ещё раз.",
			}
		}
		return storagedb.Protocol{}, fmt.Errorf("fix protocol: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return storagedb.Protocol{}, fmt.Errorf("commit fix: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "fix",
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: fixed.ID, Valid: true},
		Details: map[string]any{
			"protocol_number":        fixed.ProtocolNumber.String,
			"sequence_year":          fixed.SequenceYear.Int64,
			"protocol_month":         fixed.ProtocolMonth.Int64,
			"annual_sequence_number": fixed.AnnualSequenceNumber.Int64,
			"protocol_suffix":        suffixOrEmpty(fixed.ProtocolSuffix),
			"training_start_date":    fixed.TrainingStartDate.String,
			"training_end_date":      fixed.TrainingEndDate.String,
			"protocol_date":          fixed.ProtocolDate.String,
		},
	})
	return fixed, nil
}

// Transition moves a protocol from its current status to `to`. The state
// machine in state.go is the only source of truth for allowed moves.
//
// `from` is read inside the same transaction so a concurrent Fix or
// Transition cannot move the protocol under us.
func (s *Service) Transition(ctx context.Context, protocolID int64, to ProtocolStatus) error {
	if !to.IsValid() {
		return FieldErrors{"to": "Неизвестный статус протокола."}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txQueries := s.queries.WithTx(tx)

	existing, err := txQueries.GetProtocolByID(ctx, protocolID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FieldErrors{"protocol_id": "Протокол не найден."}
		}
		return fmt.Errorf("get protocol: %w", err)
	}

	from := ProtocolStatus(existing.Status)
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}

	updated, err := txQueries.SetProtocolStatus(ctx, storagedb.SetProtocolStatusParams{
		Status:    string(to),
		UpdatedAt: s.timestamp(),
		ID:        protocolID,
	})
	if err != nil {
		return fmt.Errorf("transition protocol: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transition: %w", err)
	}

	s.recordAudit(ctx, audit.RecordInput{
		Action:     "transition",
		EntityType: "protocol",
		EntityID:   sql.NullInt64{Int64: updated.ID, Valid: true},
		Details: map[string]any{
			"from": string(from),
			"to":   string(to),
		},
	})
	return nil
}

// validateFixInput does the cross-field validation of the dates + suffix
// BEFORE we open the transaction. ISO-date format is checked here; the
// year/month split into year/month fields is performed in Fix using
// ParseISODate.
func (s *Service) validateFixInput(in FixInput) (start, end, protoDate, suffix string, errs FieldErrors) {
	start = strings.TrimSpace(in.TrainingStartDate)
	end = strings.TrimSpace(in.TrainingEndDate)
	protoDate = strings.TrimSpace(in.ProtocolDate)

	errs = FieldErrors{}
	if start == "" {
		errs["training_start_date"] = "Укажите дату начала обучения."
	} else if _, _, err := ParseISODate(start); err != nil {
		errs["training_start_date"] = err.Error()
	}
	if end == "" {
		errs["training_end_date"] = "Укажите дату окончания обучения."
	} else if _, _, err := ParseISODate(end); err != nil {
		errs["training_end_date"] = err.Error()
	}
	if protoDate == "" {
		errs["protocol_date"] = "Укажите дату протокола."
	} else if _, _, err := ParseISODate(protoDate); err != nil {
		errs["protocol_date"] = err.Error()
	}
	normalized, err := NormalizeSuffix(in.ProtocolSuffix)
	if err != nil {
		errs["protocol_suffix"] = err.Error()
	}
	suffix = normalized
	return
}

// List returns all protocols, ordered by created_at DESC. The list view is
// expected to filter client-side; this matches the small expected data set.
func (s *Service) List(ctx context.Context) ([]storagedb.Protocol, error) {
	return s.queries.ListProtocols(ctx)
}

// Get returns a single protocol by id. Returns sql.ErrNoRows when missing.
func (s *Service) Get(ctx context.Context, id int64) (storagedb.Protocol, error) {
	return s.queries.GetProtocolByID(ctx, id)
}

// ListParticipants returns the active participants of a protocol, in insertion
// order. The detail view uses this; the handler joins the training_record
// rows separately when it has the queries it needs.
func (s *Service) ListParticipants(ctx context.Context, protocolID int64) ([]storagedb.ProtocolParticipant, error) {
	return s.queries.ListProtocolParticipants(ctx, protocolID)
}

func (s *Service) timestamp() string {
	return s.now().Format(time.RFC3339)
}

func (s *Service) recordAudit(ctx context.Context, in audit.RecordInput) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, in)
}

func suffixNull(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func suffixOrEmpty(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
