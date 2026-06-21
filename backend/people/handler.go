package people

import (
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/people/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service) *Handler {
	return &Handler{service: NewService(queries, auditSvc), queries: queries}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	workers, err := h.service.ListWorkers(r.Context())
	if err != nil {
		http.Error(w, "list workers", http.StatusInternalServerError)
		return
	}
	employers, err := h.queries.ListEmployers(r.Context())
	if err != nil {
		http.Error(w, "list employers", http.StatusInternalServerError)
		return
	}
	assignments, err := h.queries.ListWorkerEmployerAssignments(r.Context())
	if err != nil {
		http.Error(w, "list assignments", http.StatusInternalServerError)
		return
	}

	views.List(r, workers, employers, assignments).Render(r.Context(), w)
}

func (h *Handler) CreateWorker(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}

	_, err := h.service.CreateWorker(r.Context(), WorkerForm{
		LastName:   r.FormValue("last_name"),
		FirstName:  r.FormValue("first_name"),
		MiddleName: r.FormValue("middle_name"),
		SNILS:      r.FormValue("snils"),
		Email:      r.FormValue("email"),
		BirthDate:  r.FormValue("birth_date"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/workers", http.StatusSeeOther)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	worker, err := h.service.GetWorker(r.Context(), id)
	if err != nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}
	assignments, err := h.service.ListAssignments(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	employers, err := h.queries.ListEmployers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views.Detail(r, worker, assignments, employers).Render(r.Context(), w)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		worker, err := h.service.GetWorker(r.Context(), id)
		if err != nil {
			http.Error(w, "worker not found", http.StatusNotFound)
			return
		}
		views.Edit(r, worker).Render(r.Context(), w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form", http.StatusBadRequest)
			return
		}
		form := WorkerForm{
			LastName:   r.FormValue("last_name"),
			FirstName:  r.FormValue("first_name"),
			MiddleName: r.FormValue("middle_name"),
			SNILS:      r.FormValue("snils"),
			Email:      r.FormValue("email"),
			BirthDate:  r.FormValue("birth_date"),
		}
		if _, err := h.service.UpdateWorker(r.Context(), id, form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/workers", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) AssignEmployer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	workerID, err := strconv.ParseInt(r.FormValue("worker_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid worker_id", http.StatusBadRequest)
		return
	}
	employerID, err := strconv.ParseInt(r.FormValue("employer_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid employer_id", http.StatusBadRequest)
		return
	}

	_, err = h.service.AssignEmployer(r.Context(), AssignmentForm{
		WorkerID:        workerID,
		EmployerID:      employerID,
		CurrentPosition: r.FormValue("current_position"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/workers", http.StatusSeeOther)
}

func (h *Handler) DeactivateAssignment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.service.DeactivateAssignment(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/workers", http.StatusSeeOther)
}

func parseInt64Param(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}
