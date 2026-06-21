package people

import (
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/people/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type Handler struct {
	service *Service
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries) *Handler {
	return &Handler{service: NewService(queries), queries: queries}
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

	views.List(workers, employers, assignments).Render(r.Context(), w)
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
