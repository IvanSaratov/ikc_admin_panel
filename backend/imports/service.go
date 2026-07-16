package imports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

const (
	legacyProfile     = "legacy_registry"
	maxActorBytes     = 200
	maxIdempotencyKey = 200
	maxFileNameBytes  = 255
)

// Service validates legacy workbooks and atomically places them in the local
// persistent FIFO queue. File parsing and application are worker concerns.
type Service struct {
	database *sql.DB
	queries  *storagedb.Queries
	audit    *audit.Service
	files    FileStore
	config   Config
	now      func() time.Time
	queueMu  sync.Mutex
}

// NewService constructs an import enqueue service. The queue limit is enforced
// transactionally by one service instance, matching the application's local
// single-process SQLite deployment.
func NewService(
	database *sql.DB,
	queries *storagedb.Queries,
	auditService *audit.Service,
	files FileStore,
	config Config,
) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("imports database is required")
	}
	if queries == nil {
		return nil, fmt.Errorf("imports queries are required")
	}
	if files == nil {
		return nil, fmt.Errorf("imports file store is required")
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	return &Service{
		database: database,
		queries:  queries,
		audit:    auditService,
		files:    files,
		config:   config,
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// SetClock replaces the UTC clock used for durable timestamps. Intended for
// deterministic tests.
func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// EnqueueLegacy validates metadata, stores and preflights the workbook, then
// creates the queued import and its safe structural sheet plans in one DB
// transaction. Any failure before commit removes the newly stored file.
func (s *Service) EnqueueLegacy(ctx context.Context, input EnqueueInput) (EnqueueResult, error) {
	validated, err := validateEnqueueInput(input)
	if err != nil {
		return EnqueueResult{}, err
	}

	if validated.idempotencyKey != "" {
		existing, err := s.queries.GetImportByIdempotencyKey(ctx, nullableString(validated.idempotencyKey))
		switch {
		case err == nil:
			return s.reuseResult(ctx, existing)
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return EnqueueResult{}, storageServiceError("check idempotency key", err)
		}
	}

	stored, err := s.files.Save(ctx, input.Body, s.config.LegacyLimits.MaxFileBytes)
	if err != nil {
		return EnqueueResult{}, normalizeServiceError(err)
	}
	keepFile := false
	defer func() {
		if !keepFile {
			_ = s.files.Delete(stored.Token)
		}
	}()

	storedPath, err := s.files.Path(stored.Token)
	if err != nil {
		return EnqueueResult{}, storageServiceError("open stored upload", err)
	}
	plan, err := legacy.Preflight(ctx, storedPath, s.config.LegacyLimits)
	if err != nil {
		return EnqueueResult{}, mapPreflightError(err)
	}

	result, err := s.insertQueuedImport(ctx, validated, stored, plan)
	if err != nil {
		return EnqueueResult{}, err
	}
	keepFile = !result.Reused
	if !result.Reused {
		s.recordEnqueueAudit(ctx, result, len(plan.Sheets), stored.Size, validated.actor)
	}
	return result, nil
}

type validatedEnqueueInput struct {
	fileName       string
	idempotencyKey string
	actor          string
}

func validateEnqueueInput(input EnqueueInput) (validatedEnqueueInput, error) {
	if input.Body == nil {
		return validatedEnqueueInput{}, inputServiceError("file body is required")
	}
	actor := strings.TrimSpace(input.Actor)
	if actor == "" || len(actor) > maxActorBytes || containsControl(actor) {
		return validatedEnqueueInput{}, inputServiceError("valid actor is required")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if len(key) > maxIdempotencyKey || containsControl(key) {
		return validatedEnqueueInput{}, inputServiceError("invalid idempotency key")
	}
	return validatedEnqueueInput{
		fileName:       sanitizeFileName(input.OriginalFileName),
		idempotencyKey: key,
		actor:          actor,
	}, nil
}

func validateConfig(config Config) error {
	limits := config.LegacyLimits
	if config.ActiveQueueLimit <= 0 {
		return fmt.Errorf("active import queue limit must be positive")
	}
	if config.FileTTL <= 0 {
		return fmt.Errorf("import file TTL must be positive")
	}
	if limits.MaxFileBytes <= 0 || limits.MaxUncompressedBytes == 0 ||
		limits.MaxZIPEntries <= 0 || limits.MaxSheets <= 0 || limits.MaxRows <= 0 ||
		limits.MaxCells <= 0 || limits.MaxCellBytes <= 0 || limits.MaxHeaderRows <= 0 {
		return fmt.Errorf("legacy workbook limits must be positive")
	}
	return nil
}

func (s *Service) insertQueuedImport(
	ctx context.Context,
	input validatedEnqueueInput,
	stored StoredFile,
	plan legacy.WorkbookPlan,
) (result EnqueueResult, returnErr error) {
	// This short critical section serializes capacity checks and inserts. SQLite
	// constraints remain the durable backstop for duplicate keys and hashes.
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return EnqueueResult{}, storageServiceError("begin import transaction", err)
	}
	defer func() {
		if returnErr != nil {
			_ = tx.Rollback()
		}
	}()
	queries := s.queries.WithTx(tx)

	if input.idempotencyKey != "" {
		existing, err := queries.GetImportByIdempotencyKey(ctx, nullableString(input.idempotencyKey))
		switch {
		case err == nil:
			if err := tx.Rollback(); err != nil {
				return EnqueueResult{}, storageServiceError("rollback reused import", err)
			}
			return s.reuseResult(ctx, existing)
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return EnqueueResult{}, storageServiceError("recheck idempotency key", err)
		}
	}

	existing, err := queries.FindExistingLegacyImportBySHA256(ctx, nullableString(stored.SHA256))
	switch {
	case err == nil:
		return EnqueueResult{}, &ServiceError{
			Code:             CodeDuplicateFile,
			Detail:           "workbook was already imported",
			ExistingImportID: existing.ID,
		}
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return EnqueueResult{}, storageServiceError("check workbook digest", err)
	}

	active, err := queries.CountActiveImports(ctx)
	if err != nil {
		return EnqueueResult{}, storageServiceError("count active imports", err)
	}
	if active >= s.config.ActiveQueueLimit {
		return EnqueueResult{}, &ServiceError{Code: CodeQueueFull, Detail: "import queue is full"}
	}

	now := s.now().UTC()
	nowText := now.Format(time.RFC3339)
	created, err := queries.CreateImport(ctx, storagedb.CreateImportParams{
		Profile:           legacyProfile,
		SourceFileName:    nullableString(input.fileName),
		SourceSha256:      nullableString(stored.SHA256),
		SourceSizeBytes:   sql.NullInt64{Int64: stored.Size, Valid: true},
		IdempotencyKey:    nullableString(input.idempotencyKey),
		UploadedByActor:   input.actor,
		ReceivedAt:        nowText,
		Status:            "queued",
		TempFileToken:     nullableString(stored.Token),
		TempFileExpiresAt: nullableString(now.Add(s.config.FileTTL).Format(time.RFC3339)),
		CreatedAt:         nowText,
		UpdatedAt:         nowText,
	})
	if err != nil {
		return EnqueueResult{}, storageServiceError("create queued import", err)
	}

	for _, sheet := range plan.Sheets {
		headerMap, err := legacy.EncodeSheetPlan(sheet)
		if err != nil {
			return EnqueueResult{}, &ServiceError{Code: CodeInternal, Detail: "encode workbook structure", Err: err}
		}
		if _, err := queries.CreateImportSheet(ctx, storagedb.CreateImportSheetParams{
			ImportID:     created.ID,
			SheetName:    sheet.Name,
			SheetOrder:   int64(sheet.Order),
			SheetProfile: string(sheet.Profile),
			HeaderMap:    headerMap,
			CreatedAt:    nowText,
			UpdatedAt:    nowText,
		}); err != nil {
			return EnqueueResult{}, storageServiceError("create import sheet", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return EnqueueResult{}, storageServiceError("commit queued import", err)
	}
	return EnqueueResult{
		Import:        created,
		QueuePosition: active + 1,
	}, nil
}

func (s *Service) reuseResult(ctx context.Context, existing storagedb.Import) (EnqueueResult, error) {
	if existing.Profile != legacyProfile {
		return EnqueueResult{}, &ServiceError{
			Code:             CodeIdempotencyConflict,
			Detail:           "idempotency key belongs to another import profile",
			ExistingImportID: existing.ID,
		}
	}
	position, err := s.queuePosition(ctx, existing)
	if err != nil {
		return EnqueueResult{}, err
	}
	return EnqueueResult{Import: existing, QueuePosition: position, Reused: true}, nil
}

func (s *Service) queuePosition(ctx context.Context, existing storagedb.Import) (int64, error) {
	if existing.Status != "queued" {
		return 0, nil
	}
	ahead, err := s.queries.CountImportsAhead(ctx, existing.ID)
	if err != nil {
		return 0, storageServiceError("calculate queue position", err)
	}
	return ahead + 1, nil
}

func (s *Service) recordEnqueueAudit(
	ctx context.Context,
	result EnqueueResult,
	sheetCount int,
	size int64,
	actor string,
) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		Action:     "enqueue",
		EntityType: "import",
		EntityID:   sql.NullInt64{Int64: result.Import.ID, Valid: true},
		Actor:      actor,
		Details: map[string]any{
			"profile":        legacyProfile,
			"size_bytes":     size,
			"sheet_count":    sheetCount,
			"queue_position": result.QueuePosition,
		},
	})
}

func mapPreflightError(err error) error {
	var parseErr *legacy.ParseError
	if !errors.As(err, &parseErr) {
		return &ServiceError{Code: CodeInternal, Detail: "inspect workbook", Err: err}
	}
	switch parseErr.Code {
	case legacy.CodeNotXLSX:
		return &ServiceError{Code: CodeNotXLSX, Detail: "file is not a valid XLSX workbook", Err: err}
	case legacy.CodeWorkbookTooLarge:
		return &ServiceError{Code: CodeFileTooLarge, Detail: "workbook exceeds configured limits", Err: err}
	case legacy.CodeUnsupportedWorkbook,
		legacy.CodeMissingSheet,
		legacy.CodeUnknownSheet,
		legacy.CodeMissingColumns,
		legacy.CodeLimitExceeded:
		return &ServiceError{Code: CodeUnsupportedWorkbook, Detail: "unsupported workbook structure", Err: err}
	case legacy.CodeReadFailed:
		return &ServiceError{Code: CodeStorageUnavailable, Detail: "cannot inspect stored workbook", Err: err}
	default:
		return &ServiceError{Code: CodeInternal, Detail: "inspect workbook", Err: err}
	}
}

func normalizeServiceError(err error) error {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return err
	}
	return storageServiceError("store upload", err)
}

func inputServiceError(detail string) *ServiceError {
	return &ServiceError{Code: CodeInvalidInput, Detail: detail}
}

func storageServiceError(detail string, err error) *ServiceError {
	return &ServiceError{Code: CodeStorageUnavailable, Detail: detail, Err: err}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func sanitizeFileName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "upload.xlsx"
	}
	if len(value) <= maxFileNameBytes {
		return value
	}
	var truncated strings.Builder
	for _, r := range value {
		if truncated.Len()+utf8.RuneLen(r) > maxFileNameBytes {
			break
		}
		truncated.WriteRune(r)
	}
	if truncated.Len() == 0 {
		return "upload.xlsx"
	}
	return truncated.String()
}
