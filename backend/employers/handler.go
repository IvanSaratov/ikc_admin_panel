package employers

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/employers/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type Handler struct {
	service *Service
}

func NewHandler(queries *storagedb.Queries) *Handler {
	return &Handler{service: NewService(queries)}
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
