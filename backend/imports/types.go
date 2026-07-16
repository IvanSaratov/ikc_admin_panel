package imports

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// Config controls queue capacity, temporary file lifetime and workbook limits.
type Config struct {
	ActiveQueueLimit int64
	FileTTL          time.Duration
	LegacyLimits     legacy.Limits
}

// DefaultConfig returns the agreed local single-worker import settings.
func DefaultConfig() Config {
	return Config{
		ActiveQueueLimit: 5,
		FileTTL:          24 * time.Hour,
		LegacyLimits:     legacy.DefaultLimits(),
	}
}

// EnqueueInput is the validated boundary used later by the multipart handler.
type EnqueueInput struct {
	OriginalFileName string
	IdempotencyKey   string
	Actor            string
	Body             io.Reader
}

// EnqueueResult describes a newly queued or idempotently reused job.
type EnqueueResult struct {
	Import        storagedb.Import
	QueuePosition int64
	Reused        bool
}

// StoredFile is an opaque private upload produced by FileStore.
type StoredFile struct {
	Token  string
	Path   string
	SHA256 string
	Size   int64
}

// FileStore owns temporary upload persistence independently from queue state.
type FileStore interface {
	Save(context.Context, io.Reader, int64) (StoredFile, error)
	Path(string) (string, error)
	Delete(string) error
}

// StagingConfig controls crash-safe workbook staging.
type StagingConfig struct {
	BatchSize     int
	LeaseDuration time.Duration
	LegacyLimits  legacy.Limits
}

// DefaultStagingConfig returns bounded single-worker staging defaults.
func DefaultStagingConfig() StagingConfig {
	return StagingConfig{
		BatchSize:     250,
		LeaseDuration: 2 * time.Minute,
		LegacyLimits:  legacy.DefaultLimits(),
	}
}

// StageOutcome summarizes durable rows produced by one staging run.
type StageOutcome struct {
	ImportID     int64
	RowsStaged   int64
	SheetsStaged int
}

// StageErrorCode is a stable worker error classification.
type StageErrorCode string

const (
	CodeStageInvalidInput        StageErrorCode = "staging_invalid_input"
	CodeStageLeaseLost           StageErrorCode = "staging_lease_lost"
	CodeSourceFileUnavailable    StageErrorCode = "source_file_unavailable"
	CodeSourceFileMismatch       StageErrorCode = "source_file_mismatch"
	CodeStagingPlanIncompatible  StageErrorCode = "staging_plan_incompatible"
	CodeStagingCheckpointCorrupt StageErrorCode = "staging_checkpoint_corrupt"
	CodeStagingFailed            StageErrorCode = "staging_failed"
	CodeSourceCleanupFailed      StageErrorCode = "source_cleanup_failed"
	CodeStagingStorage           StageErrorCode = "staging_storage_unavailable"
)

// StageError exposes only fixed staging metadata. Its cause remains available
// through errors.Is/errors.As but is intentionally omitted from Error().
type StageError struct {
	Code      StageErrorCode
	Retryable bool
	Sheet     string
	Row       int64
	Err       error
}

func (e *StageError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{string(e.Code), fmt.Sprintf("retryable=%t", e.Retryable)}
	if e.Sheet != "" {
		parts = append(parts, "sheet="+e.Sheet)
	}
	if e.Row > 0 {
		parts = append(parts, fmt.Sprintf("row=%d", e.Row))
	}
	return strings.Join(parts, ": ")
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
