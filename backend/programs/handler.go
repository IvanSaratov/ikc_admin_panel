package programs

import (
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/programs/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

type Handler struct {
	service *Service
}

func NewHandler(queries *storagedb.Queries) *Handler {
	return &Handler{service: NewService(queries)}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	groups, err := h.service.ListGroups(ctx)
	if err != nil {
		http.Error(w, "list program groups", http.StatusInternalServerError)
		return
	}
	programs, err := h.service.ListPrograms(ctx)
	if err != nil {
		http.Error(w, "list programs", http.StatusInternalServerError)
		return
	}

	views.List(groups, programs).Render(ctx, w)
}

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}

	_, err := h.service.CreateGroup(r.Context(), GroupForm{
		Code: r.FormValue("code"),
		Name: r.FormValue("name"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/programs", http.StatusSeeOther)
}

func (h *Handler) CreateProgram(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(r.FormValue("program_group_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid program_group_id", http.StatusBadRequest)
		return
	}
	defaultHours, err := strconv.ParseInt(r.FormValue("default_hours"), 10, 64)
	if err != nil {
		http.Error(w, "invalid default_hours", http.StatusBadRequest)
		return
	}

	_, err = h.service.CreateProgram(r.Context(), ProgramForm{
		ProgramGroupID: groupID,
		Code:           r.FormValue("code"),
		Name:           r.FormValue("name"),
		DefaultHours:   defaultHours,
		MoodleCourseID: r.FormValue("moodle_course_id"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/programs", http.StatusSeeOther)
}
