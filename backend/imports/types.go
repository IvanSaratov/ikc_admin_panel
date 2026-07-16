package imports

import (
	"context"
	"io"
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
