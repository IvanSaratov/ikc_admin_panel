package audit

import storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"

// Handler is retained as the app container wiring point. The old
// server-rendered audit page was removed in Stage 5; audit recording and
// storage queries remain in Service and sqlc-generated code.
type Handler struct {
	queries *storagedb.Queries
}

func NewHandler(queries *storagedb.Queries) *Handler {
	return &Handler{queries: queries}
}
