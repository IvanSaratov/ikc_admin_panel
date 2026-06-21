package admin

import (
	"context"
	"errors"
	"fmt"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned by Authenticate when the supplied
// login/password pair does not match an active user.
//
// It is intentionally broad: it covers both "user does not exist" and
// "wrong password" so the login form cannot be used for user enumeration.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrUserDisabled is returned by Authenticate when the matched user has
// status='disabled'. Callers should map this to the same UI message as
// ErrInvalidCredentials to avoid leaking account state to outsiders.
var ErrUserDisabled = errors.New("user disabled")

// Service is the entry point for authentication.
//
// It is intentionally small: it only knows how to compare a bcrypt hash
// against a presented password. Session lifecycle is owned by scs (see
// session.go); CSRF is owned by gorilla/csrf (see csrf.go); HTTP handlers
// live in handler.go and use SessionManager + Service together.
type Service struct {
	store *Store
}

// NewService constructs an authentication Service.
//
// Mirrors the frozen contract in section 0.2 of the implementation plan.
func NewService(q *storagedb.Queries) *Service {
	return &Service{store: NewStore(q)}
}

// User is an alias for the sqlc-generated User row so callers don't have
// to import backend/storage/db just to reference the return type.
type User = storagedb.User

// Authenticate looks up the user by login and verifies the bcrypt password
// hash. It rejects disabled users with ErrUserDisabled and unknown/wrong
// credentials with ErrInvalidCredentials so the caller can render a single
// "login failed" UI without leaking which case fired.
func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error) {
	if login == "" || password == "" {
		return User{}, ErrInvalidCredentials
	}

	user, err := s.store.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("lookup user: %w", err)
	}

	if user.Status != "active" {
		return User{}, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	return user, nil
}
