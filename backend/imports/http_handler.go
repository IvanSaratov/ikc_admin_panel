package imports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/api"
	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/imports/legacy"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const multipartOverheadBytes int64 = 1 << 20

var errExtraMultipartPart = errors.New("extra multipart part")

// LegacyEnqueuer is the transport boundary implemented by Service.
type LegacyEnqueuer interface {
	EnqueueLegacy(context.Context, EnqueueInput) (EnqueueResult, error)
}

// ImportReader is the transport boundary implemented by ReadService.
type ImportReader interface {
	List(context.Context, string, int) (ImportPage, error)
	Get(context.Context, int64) (ImportView, error)
}

// HTTPHandler exposes legacy enqueue and safe import projections.
type HTTPHandler struct {
	enqueuer    LegacyEnqueuer
	reader      ImportReader
	maxBodySize int64
	log         *zap.Logger
}

func NewHTTPHandler(
	enqueuer LegacyEnqueuer,
	reader ImportReader,
	limits legacy.Limits,
	log *zap.Logger,
) (*HTTPHandler, error) {
	if enqueuer == nil || reader == nil {
		return nil, fmt.Errorf("import HTTP services are required")
	}
	if limits.MaxFileBytes <= 0 || limits.MaxFileBytes > math.MaxInt64-multipartOverheadBytes {
		return nil, fmt.Errorf("valid import file limit is required")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &HTTPHandler{
		enqueuer:    enqueuer,
		reader:      reader,
		maxBodySize: limits.MaxFileBytes + multipartOverheadBytes,
		log:         log,
	}, nil
}

// UploadLegacy streams exactly one multipart file into the enqueue service.
func (h *HTTPHandler) UploadLegacy(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		h.writeError(w, r, inputServiceError("multipart form-data is required"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	reader, err := r.MultipartReader()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "file" || part.FileName() == "" {
		h.writeError(w, r, inputServiceError("exactly one file part is required"))
		return
	}

	result, err := h.enqueuer.EnqueueLegacy(r.Context(), EnqueueInput{
		OriginalFileName: part.FileName(),
		IdempotencyKey:   r.Header.Get("Idempotency-Key"),
		Actor:            audit.ActorFromContext(r.Context()),
		Body: &singleMultipartFile{
			part:   part,
			reader: reader,
		},
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	statusURL := "/api/imports/" + strconv.FormatInt(result.Import.ID, 10)
	w.Header().Set("Location", statusURL)
	status := http.StatusAccepted
	if result.Reused && terminalImportStatus(result.Import.Status) {
		status = http.StatusOK
	}
	api.WriteJSON(w, status, uploadResponse{
		ID:            result.Import.ID,
		Status:        result.Import.Status,
		Phase:         nullStringPointer(result.Import.Phase),
		QueuePosition: result.QueuePosition,
		Reused:        result.Reused,
		StatusURL:     statusURL,
	})
}

// List returns one safe cursor page.
func (h *HTTPHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeError(w, r, inputServiceError("invalid import page limit"))
			return
		}
		limit = parsed
	}
	page, err := h.reader.List(r.Context(), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, page)
}

// Get returns one safe import projection.
func (h *HTTPHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.writeError(w, r, inputServiceError("invalid import ID"))
		return
	}
	view, err := h.reader.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, view)
}

type uploadResponse struct {
	ID            int64   `json:"id"`
	Status        string  `json:"status"`
	Phase         *string `json:"phase"`
	QueuePosition int64   `json:"queue_position"`
	Reused        bool    `json:"reused"`
	StatusURL     string  `json:"status_url"`
}

type singleMultipartFile struct {
	part    *multipart.Part
	reader  *multipart.Reader
	checked bool
}

func (r *singleMultipartFile) Read(buffer []byte) (int, error) {
	read, err := r.part.Read(buffer)
	if !errors.Is(err, io.EOF) || r.checked {
		return read, err
	}
	r.checked = true
	next, nextErr := r.reader.NextPart()
	if next != nil {
		_ = next.Close()
	}
	if errors.Is(nextErr, io.EOF) {
		return read, io.EOF
	}
	if nextErr != nil {
		return read, nextErr
	}
	return read, errExtraMultipartPart
}

func terminalImportStatus(status string) bool {
	switch status {
	case "completed", "completed_with_issues", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func (h *HTTPHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	problem := problemForImportError(err)
	if problem.Status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "5")
	}
	if problem.Status >= http.StatusInternalServerError {
		h.log.Error("import API request failed",
			zap.String("trace_id", api.TraceID(r.Context())),
			zap.String("code", problem.Code),
		)
	}
	api.WriteProblem(w, r, problem)
}

func problemForImportError(err error) api.Problem {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return api.Problem{Status: http.StatusRequestEntityTooLarge, Code: string(CodeFileTooLarge), Detail: "Файл превышает допустимый размер"}
	}
	if errors.Is(err, errExtraMultipartPart) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return api.Problem{Status: http.StatusBadRequest, Code: string(CodeInvalidInput), Detail: "Некорректный multipart запрос"}
	}
	if errors.Is(err, ErrImportNotFound) {
		return api.Problem{Status: http.StatusNotFound, Code: "import_not_found", Detail: "Импорт не найден"}
	}
	var serviceError *ServiceError
	if errors.As(err, &serviceError) {
		problem := api.Problem{Code: string(serviceError.Code), ExistingImportID: serviceError.ExistingImportID}
		switch serviceError.Code {
		case CodeInvalidInput:
			problem.Status, problem.Detail = http.StatusBadRequest, "Некорректный запрос"
		case CodeFileTooLarge:
			problem.Status, problem.Detail = http.StatusRequestEntityTooLarge, "Файл превышает допустимый размер"
		case CodeNotXLSX:
			problem.Status, problem.Detail = http.StatusUnsupportedMediaType, "Файл не является XLSX"
		case CodeUnsupportedWorkbook:
			problem.Status, problem.Detail = http.StatusUnprocessableEntity, "Структура книги не поддерживается"
		case CodeDuplicateFile:
			problem.Status, problem.Detail = http.StatusConflict, "Файл уже был загружен"
		case CodeIdempotencyConflict:
			problem.Status, problem.Detail = http.StatusConflict, "Ключ запроса принадлежит другому импорту"
		case CodeQueueFull:
			problem.Status, problem.Detail = http.StatusTooManyRequests, "Очередь импортов заполнена"
		case CodeStorageUnavailable:
			problem.Status, problem.Detail = http.StatusServiceUnavailable, "Сервис временно недоступен"
		case CodeInternal:
			problem.Status, problem.Detail = http.StatusInternalServerError, "Внутренняя ошибка сервера"
		default:
			problem.Status, problem.Code, problem.Detail = http.StatusInternalServerError, string(CodeInternal), "Внутренняя ошибка сервера"
		}
		return problem
	}
	return api.Problem{Status: http.StatusInternalServerError, Code: string(CodeInternal), Detail: "Внутренняя ошибка сервера"}
}

var _ LegacyEnqueuer = (*Service)(nil)
var _ ImportReader = (*ReadService)(nil)
