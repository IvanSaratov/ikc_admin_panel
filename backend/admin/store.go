// Package admin implements authentication (F3): scs sessions, gorilla/csrf,
// bcrypt-hashed credentials, and login/logout handlers.
//
// Store is a thin wrapper over the sqlc-generated user queries. Keeping the
// indirection here lets Service/Handler code stay storage-agnostic and gives
// us a single place to add future user-management calls (e.g. password reset,
// disable, role change) without leaking sqlc types into request handlers.
package admin

import (
	"context"
	"database/sql"
	"errors"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// ErrUserNotFound is returned by Store lookups when no row matches.
// Callers should map this to a generic "invalid credentials" response so
// user enumeration is not possible.
var ErrUserNotFound = errors.New("user not found")

// Store wraps sqlc-generated user queries.
//
// It is safe for concurrent use as long as the underlying *storagedb.Queries
// is. All methods accept a context for cancellation/timeout propagation.
type Store struct {
	queries *storagedb.Queries
}

// NewStore constructs a Store backed by the provided queries.
func NewStore(q *storagedb.Queries) *Store {
	return &Store{queries: q}
}

// GetByLogin fetches a user row by login.
//
// Returns ErrUserNotFound (not a raw sql.ErrNoRows) so callers can branch on
// a single sentinel without importing database/sql.
func (s *Store) GetByLogin(ctx context.Context, login string) (storagedb.User, error) {
	user, err := s.queries.GetUserByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storagedb.User{}, ErrUserNotFound
		}
		return storagedb.User{}, err
	}
	return user, nil
}

// GetByID fetches a user row by primary key. Used by RequireAuth middleware
// to hydrate the user record from the session-stored user ID.
func (s *Store) GetByID(ctx context.Context, id int64) (storagedb.User, error) {
	user, err := s.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storagedb.User{}, ErrUserNotFound
		}
		return storagedb.User{}, err
	}
	return user, nil
}

// Create inserts a new user row and returns the persisted record (with id
// and timestamps populated).
func (s *Store) Create(ctx context.Context, arg storagedb.CreateUserParams) (storagedb.User, error) {
	return s.queries.CreateUser(ctx, arg)
}
