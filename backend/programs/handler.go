package programs

import (
	"net/http"
	"strconv"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/programs/views"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(queries *storagedb.Queries, auditSvc *audit.Service) *Handler {
	return &Handler{service: NewService(queries, auditSvc)}
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

func (h *Handler) EditGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		group, err := h.service.GetGroup(r.Context(), id)
		if err != nil {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}
		views.EditGroup(group).Render(r.Context(), w)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form", http.StatusBadRequest)
			return
		}
		form := GroupForm{Code: r.FormValue("code"), Name: r.FormValue("name")}
		if _, err := h.service.UpdateGroup(r.Context(), id, form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/programs", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) DeactivateGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.service.DeactivateGroup(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func (h *Handler) EditProgram(w http.ResponseWriter, r *http.Request) {
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		program, err := h.service.GetProgram(r.Context(), id)
		if err != nil {
			http.Error(w, "program not found", http.StatusNotFound)
			return
		}
		groups, err := h.service.ListGroups(r.Context())
		if err != nil {
			http.Error(w, "list groups", http.StatusInternalServerError)
			return
		}
		views.EditProgram(program, groups).Render(r.Context(), w)
	case http.MethodPost:
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
		form := ProgramForm{
			ProgramGroupID: groupID,
			Code:           r.FormValue("code"),
			Name:           r.FormValue("name"),
			DefaultHours:   defaultHours,
			MoodleCourseID: r.FormValue("moodle_course_id"),
		}
		if _, err := h.service.UpdateProgram(r.Context(), id, form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/programs", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) DeactivateProgram(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	id, err := parseInt64Param(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.service.DeactivateProgram(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/programs", http.StatusSeeOther)
}

func parseInt64Param(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}
