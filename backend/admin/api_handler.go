package admin

import (
	"database/sql"
	"encoding/json"
	"mime"
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"go.uber.org/zap"
)

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Login         string `json:"login,omitempty"`
}

// GetSessionJSON reports the current session state for the React app.
func (h *Handler) GetSessionJSON(w http.ResponseWriter, r *http.Request) {
	login := h.sessions.GetString(r.Context(), SessionKeyUserLogin)
	if login == "" {
		writeJSON(w, http.StatusOK, sessionResponse{Authenticated: false})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, Login: login})
}

// PostLoginJSON authenticates JSON credentials and starts an scs session.
func (h *Handler) PostLoginJSON(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported_media_type"})
		return
	}

	var input loginRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	user, err := h.service.Authenticate(r.Context(), input.Login, input.Password)
	if err != nil {
		auditLogin := input.Login
		if auditLogin == "" {
			auditLogin = "empty"
		}
		auditCtx := audit.WithActor(r.Context(), auditLogin)
		if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
			Action:     "login.failure",
			EntityType: "session",
			Actor:      auditLogin,
			Details:    map[string]any{"reason": errReason(err)},
		}); auditErr != nil {
			h.log.Error("audit login failure", zap.Error(auditErr))
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}

	if err := h.sessions.RenewToken(r.Context()); err != nil {
		h.log.Error("renew session token", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session_error"})
		return
	}
	h.sessions.Put(r.Context(), SessionKeyUserID, user.ID)
	h.sessions.Put(r.Context(), SessionKeyUserLogin, user.Login)

	auditCtx := audit.WithActor(r.Context(), user.Login)
	if auditErr := h.audit.Record(auditCtx, audit.RecordInput{
		Action:     "login.success",
		EntityType: "session",
		Actor:      user.Login,
		EntityID:   sql.NullInt64{Int64: user.ID, Valid: true},
	}); auditErr != nil {
		h.log.Error("audit login success", zap.Error(auditErr))
	}

	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, Login: user.Login})
}

// PostLogoutJSON destroys the current session for the React app.
func (h *Handler) PostLogoutJSON(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.Destroy(r.Context()); err != nil {
		h.log.Error("destroy session", zap.Error(err))
	}
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: false})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
