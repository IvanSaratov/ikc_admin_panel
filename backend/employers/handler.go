package employers

import (
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/employers/views"
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
	items, err := h.service.List(r.Context())
	if err != nil {
		http.Error(w, "list employers", http.StatusInternalServerError)
		return
	}

	views.List(items).Render(r.Context(), w)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}

	_, err := h.service.Create(r.Context(), Form{
		INN:           r.FormValue("inn"),
		CanonicalName: r.FormValue("canonical_name"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/employers", http.StatusSeeOther)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	employer, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "employer not found", http.StatusNotFound)
		return
	}
	assignments, err := h.queries.ListWorkersForEmployer(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views.Detail(employer, assignments).Render(r.Context(), w)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		employer, err := h.service.Get(r.Context(), id)
		if err != nil {
			http.Error(w, "employer not found", http.StatusNotFound)
			return
		}
		views.Edit(employer).Render(r.Context(), w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form", http.StatusBadRequest)
			return
		}
		form := Form{INN: r.FormValue("inn"), CanonicalName: r.FormValue("canonical_name")}
		if _, err := h.service.Update(r.Context(), id, form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/employers", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) Deactivate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.service.Deactivate(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/employers", http.StatusSeeOther)
}

func parseInt64Param(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}
