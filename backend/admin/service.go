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
// It is intentionally broad: it covers "user does not exist", "wrong
// password", and "user disabled" so the login form cannot be used for
// user enumeration. Callers must not branch on the underlying reason.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrUserDisabled is an alias for ErrInvalidCredentials. It exists so
// historical call sites that distinguish "disabled" for audit reasons
// still type-check, but at the value level it is the same sentinel so
// `errors.Is(err, ErrInvalidCredentials)` succeeds for all three cases.
var ErrUserDisabled = ErrInvalidCredentials

// dummyHash is a precomputed bcrypt hash used to equalize response
// timing on the "user not found" branch. Without this, an attacker can
// enumerate valid logins by observing that bcrypt.CompareHashAndPassword
// takes ~100ms for existing users but returns immediately for unknown
// ones. The hash itself is throwaway — no password ever validates
// against it.
var dummyHash = mustGenerateDummyHash()

func mustGenerateDummyHash() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-equalize-timing"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("admin: generate dummy bcrypt hash: %v", err))
	}
	return hash
}

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
//
// PasswordHash is exposed only so the credential check inside
// Authenticate can reach it. Callers MUST NOT serialize User to JSON,
// logs, or templates — use the session-stored login/ID instead.
type User = storagedb.User

// Authenticate looks up the user by login and verifies the bcrypt password
// hash. All failure modes collapse to ErrInvalidCredentials so the caller
// renders a single "login failed" UI without leaking which case fired.
func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error) {
	if login == "" || password == "" {
		// Run a no-op bcrypt compare to keep timing consistent with the
		// other failure branches.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, ErrInvalidCredentials
	}

	user, err := s.store.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Equalize timing with the "wrong password" branch: a real
			// bcrypt compare against a throwaway hash. This costs ~100ms
			// per request, but it removes the enumeration signal.
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("lookup user: %w", err)
	}

	if user.Status != "active" {
		// Still verify against the real hash so timing matches the
		// "wrong password" branch. We discard the result — disabled
		// users always fail.
		_ = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		return User{}, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	return user, nil
}
