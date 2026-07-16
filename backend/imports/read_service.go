package imports

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

const (
	defaultImportPageSize = 50
	maxImportPageSize     = 200
	cursorVersion         = "v1"
)

// ErrImportNotFound is safe for HTTP 404 mapping.
var ErrImportNotFound = errors.New("import not found")

// ImportView is the complete API-safe import projection.
type ImportView struct {
	ID              int64   `json:"id"`
	Profile         string  `json:"profile"`
	SourceFileName  *string `json:"source_file_name"`
	UploadedByActor string  `json:"uploaded_by_actor"`
	ReceivedAt      string  `json:"received_at"`
	Status          string  `json:"status"`
	Phase           *string `json:"phase"`
	QueuePosition   int64   `json:"queue_position"`
	RowsTotal       int64   `json:"rows_total"`
	RowsProcessed   int64   `json:"rows_processed"`
	RowsApplied     int64   `json:"rows_applied"`
	RowsDuplicate   int64   `json:"rows_duplicate"`
	RowsNeedsReview int64   `json:"rows_needs_review"`
	ErrorCode       *string `json:"error_code"`
	ErrorDetail     *string `json:"error_detail"`
	StartedAt       *string `json:"started_at"`
	StagedAt        *string `json:"staged_at"`
	CompletedAt     *string `json:"completed_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ImportPage is one stable cursor page.
type ImportPage struct {
	Items      []ImportView `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

// ReadService lists and fetches only the API-safe import projection.
type ReadService struct {
	queries *storagedb.Queries
}

func NewReadService(queries *storagedb.Queries) *ReadService {
	return &ReadService{queries: queries}
}

func (s *ReadService) List(ctx context.Context, cursor string, limit int) (ImportPage, error) {
	if limit == 0 {
		limit = defaultImportPageSize
	}
	if limit < 1 || limit > maxImportPageSize {
		return ImportPage{}, inputServiceError("invalid import page limit")
	}
	beforeID, err := decodeImportCursor(cursor)
	if err != nil {
		return ImportPage{}, err
	}
	rows, err := s.queries.ListImportsAPI(ctx, storagedb.ListImportsAPIParams{
		BeforeID: beforeID,
		PageSize: int64(limit + 1),
	})
	if err != nil {
		return ImportPage{}, storageServiceError("list imports", err)
	}
	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	items := make([]ImportView, 0, len(rows))
	for _, row := range rows {
		items = append(items, importViewFromListRow(row))
	}
	page := ImportPage{Items: items}
	if hasNext && len(items) > 0 {
		page.NextCursor = encodeImportCursor(items[len(items)-1].ID)
	}
	return page, nil
}

func (s *ReadService) Get(ctx context.Context, id int64) (ImportView, error) {
	if id <= 0 {
		return ImportView{}, inputServiceError("invalid import ID")
	}
	row, err := s.queries.GetImportAPI(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportView{}, ErrImportNotFound
	}
	if err != nil {
		return ImportView{}, storageServiceError("get import", err)
	}
	return importViewFromGetRow(row), nil
}

func encodeImportCursor(id int64) string {
	payload := cursorVersion + ":" + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeImportCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, inputServiceError("invalid import cursor")
	}
	prefix, rawID, ok := strings.Cut(string(decoded), ":")
	if !ok || prefix != cursorVersion || rawID == "" {
		return 0, inputServiceError("invalid import cursor")
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != rawID {
		return 0, inputServiceError("invalid import cursor")
	}
	return id, nil
}

func importViewFromListRow(row storagedb.ListImportsAPIRow) ImportView {
	return ImportView{
		ID:              row.ID,
		Profile:         row.Profile,
		SourceFileName:  nullStringPointer(row.SourceFileName),
		UploadedByActor: row.UploadedByActor,
		ReceivedAt:      row.ReceivedAt,
		Status:          row.Status,
		Phase:           nullStringPointer(row.Phase),
		QueuePosition:   row.QueuePosition,
		RowsTotal:       row.RowsTotal,
		RowsProcessed:   row.RowsProcessed,
		RowsApplied:     row.RowsApplied,
		RowsDuplicate:   row.RowsDuplicate,
		RowsNeedsReview: row.RowsNeedsReview,
		ErrorCode:       nullStringPointer(row.ErrorCode),
		ErrorDetail:     nullStringPointer(row.ErrorDetail),
		StartedAt:       nullStringPointer(row.StartedAt),
		StagedAt:        nullStringPointer(row.StagedAt),
		CompletedAt:     nullStringPointer(row.CompletedAt),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func importViewFromGetRow(row storagedb.GetImportAPIRow) ImportView {
	return ImportView{
		ID:              row.ID,
		Profile:         row.Profile,
		SourceFileName:  nullStringPointer(row.SourceFileName),
		UploadedByActor: row.UploadedByActor,
		ReceivedAt:      row.ReceivedAt,
		Status:          row.Status,
		Phase:           nullStringPointer(row.Phase),
		QueuePosition:   row.QueuePosition,
		RowsTotal:       row.RowsTotal,
		RowsProcessed:   row.RowsProcessed,
		RowsApplied:     row.RowsApplied,
		RowsDuplicate:   row.RowsDuplicate,
		RowsNeedsReview: row.RowsNeedsReview,
		ErrorCode:       nullStringPointer(row.ErrorCode),
		ErrorDetail:     nullStringPointer(row.ErrorDetail),
		StartedAt:       nullStringPointer(row.StartedAt),
		StagedAt:        nullStringPointer(row.StagedAt),
		CompletedAt:     nullStringPointer(row.CompletedAt),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
