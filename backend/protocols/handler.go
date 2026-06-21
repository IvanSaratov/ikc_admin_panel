package protocols

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/protocols/views"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

// Handler is the HTTP layer for the protocols slice. It stays thin — every
// non-trivial decision happens in Service. The training-records lookup is
// done here because the protocols slice does not own the people slice; the
// handler is the right place to compose across slices.
type Handler struct {
	svc     *Service
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries, database *sql.DB, auditSvc *audit.Service) *Handler {
	return &Handler{
		svc:     NewService(queries, database, auditSvc),
		queries: queries,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	protocolsList, err := h.svc.List(ctx)
	if err != nil {
		http.Error(w, "list protocols", http.StatusInternalServerError)
		return
	}
	groups, err := h.queries.ListProgramGroups(ctx)
	if err != nil {
		http.Error(w, "list groups", http.StatusInternalServerError)
		return
	}
	lookup := h.groupNameLookup(ctx)

	views.List(r, protocolsList, groups, lookup).Render(ctx, w)
}

// Create handles POST /protocols — creates a new draft protocol attached
// to the program group selected in the form.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(r.FormValue("program_group_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid program_group_id", http.StatusBadRequest)
		return
	}
	_, err = h.svc.CreateProtocol(r.Context(), CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/protocols", http.StatusSeeOther)
}

// Detail renders /protocols/{id} with participants, transition buttons,
// and the "fix" CTA when status is draft.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	p, err := h.svc.Get(ctx, id)
	if err != nil {
		http.Error(w, "protocol not found", http.StatusNotFound)
		return
	}
	participants, err := h.svc.ListParticipants(ctx, id)
	if err != nil {
		http.Error(w, "list participants", http.StatusInternalServerError)
		return
	}
	group, err := h.queries.GetGroupByID(ctx, p.ProgramGroupID)
	if err != nil {
		http.Error(w, "get group", http.StatusInternalServerError)
		return
	}

	trainingRecords := h.loadTrainingRecords(ctx, participants)

	views.Detail(r, p, participants, group.Name, trainingRecords, transitionTargets(p.Status)).Render(ctx, w)
}

// Fix handles GET (form) and POST (action) for /protocols/{id}/fix.
func (h *Handler) Fix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := h.svc.Get(ctx, id)
		if err != nil {
			http.Error(w, "protocol not found", http.StatusNotFound)
			return
		}
		if p.Status != string(StatusDraft) {
			http.Redirect(w, r, "/protocols/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
			return
		}
		views.FixForm(r, p).Render(ctx, w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form", http.StatusBadRequest)
			return
		}
		_, err := h.svc.Fix(ctx, id, FixInput{
			TrainingStartDate: r.FormValue("training_start_date"),
			TrainingEndDate:   r.FormValue("training_end_date"),
			ProtocolDate:      r.FormValue("protocol_date"),
			ProtocolSuffix:    r.FormValue("protocol_suffix"),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/protocols/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// AddParticipant handles POST /protocols/{id}/participants.
func (h *Handler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	protocolID, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	trID, err := strconv.ParseInt(r.FormValue("training_record_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid training_record_id", http.StatusBadRequest)
		return
	}
	if err := h.svc.AddParticipant(r.Context(), protocolID, trID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/protocols/"+strconv.FormatInt(protocolID, 10), http.StatusSeeOther)
}

// RemoveParticipant handles POST /protocols/{id}/participants/{pid}
// with the `_method=delete` form-field trick (chi has no DELETE shortcut
// for HTML forms; we keep the verb visible so the operator can audit the
// network log).
func (h *Handler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	participantID, err := parseInt64Param(chi.URLParam(r, "pid"))
	if err != nil {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}
	if err := h.svc.RemoveParticipant(r.Context(), participantID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	protocolID, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/protocols/"+strconv.FormatInt(protocolID, 10), http.StatusSeeOther)
}

// Transition handles POST /protocols/{id}/transition?to=...
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	to := ProtocolStatus(r.FormValue("to"))
	if !to.IsValid() {
		http.Error(w, "invalid to status", http.StatusBadRequest)
		return
	}
	if err := h.svc.Transition(r.Context(), id, to); err != nil {
		// 422 makes more sense than 400 for "valid input, rejected by state
		// machine", but the existing slices use 400 for everything; we keep
		// the convention so operators don't need to learn two error codes.
		if errors.Is(err, ErrInvalidTransition) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/protocols/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// transitionTargets returns the legal next statuses for a given current
// status — used by the detail view to render the right set of buttons.
// Returns view-layer TransitionTarget pairs (status + Russian label) so
// the views package can stay free of an import on this one.
func transitionTargets(currentStatus string) []views.TransitionTarget {
	current := ProtocolStatus(currentStatus)
	var targets []views.TransitionTarget
	for _, candidate := range AllStatuses {
		if candidate == current {
			continue
		}
		if CanTransition(current, candidate) {
			targets = append(targets, views.TransitionTarget{
				Status: string(candidate),
				Label:  transitionLabel(candidate),
			})
		}
	}
	return targets
}

// transitionLabel returns a Russian label for each transition target.
func transitionLabel(to ProtocolStatus) string {
	switch to {
	case StatusFixed:
		return "Зафиксировать"
	case StatusXmlUploaded:
		return "XML отправлен"
	case StatusRegistryEntered:
		return "Реестр получен"
	case StatusGenerated:
		return "DOCX готов"
	case StatusCompleted:
		return "Завершить"
	case StatusCancelled:
		return "Отменить"
	}
	return string(to)
}

// loadTrainingRecords fetches each training_record referenced by the
// participants so the detail view can show program/position info.
// Returns a map keyed by training_record_id; missing rows map to zero-value.
func (h *Handler) loadTrainingRecords(ctx context.Context, participants []storagedb.ProtocolParticipant) map[int64]storagedb.TrainingRecord {
	out := make(map[int64]storagedb.TrainingRecord, len(participants))
	for _, p := range participants {
		if _, ok := out[p.TrainingRecordID]; ok {
			continue
		}
		tr, err := h.queries.GetTrainingRecord(ctx, p.TrainingRecordID)
		if err != nil {
			continue // missing training record renders as zero-value
		}
		out[p.TrainingRecordID] = tr
	}
	return out
}

// groupNameLookup returns a closure that maps a program_group_id to its
// human-readable name (cached per-request by the handler).
func (h *Handler) groupNameLookup(ctx context.Context) func(int64) string {
	groups, err := h.queries.ListProgramGroups(ctx)
	if err != nil {
		return func(int64) string { return "" }
	}
	byID := make(map[int64]string, len(groups))
	for _, g := range groups {
		byID[g.ID] = g.Code + " — " + g.Name
	}
	return func(id int64) string {
		if name, ok := byID[id]; ok {
			return name
		}
		return strconv.FormatInt(id, 10)
	}
}

func parseInt64Param(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

// MapSQLiteError is re-exported so the handler file can decide HTTP status
// codes for storage-level constraint violations without importing storage.
func mapSQLite(err error) error { return storage.MapSQLiteError(err) }
