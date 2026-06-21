package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdminLogin is the canonical login for the bootstrap admin user.
// Centralised so both the seed step and tests reference the same constant.
const BootstrapAdminLogin = "admin"

// BootstrapAdminRole is the role assigned to the bootstrap admin user.
// Operator-level accounts (non-admin) must be created out-of-band by an
// existing admin; this slice does not provide a self-service sign-up flow.
const BootstrapAdminRole = "admin"

// ErrBootstrapPasswordMissing is returned by EnsureBootstrapAdmin when the
// DB has no admin user AND the env var MINTRUD_ADMIN_BOOTSTRAP_PASSWORD
// is empty. We refuse to start rather than autogenerate or default —
// auto-generated passwords get logged, lost, and reused; defaults are a
// security hole. The operator must explicitly set the env var.
var ErrBootstrapPasswordMissing = errors.New(
	"no admin user exists in DB and MINTRUD_ADMIN_BOOTSTRAP_PASSWORD env is not set; " +
		"refusing to start. Set the env to a strong password to bootstrap the initial admin account.",
)

// EnsureBootstrapAdmin guarantees the database has an 'admin' user.
//
// Behaviour:
//   - If a user with login == BootstrapAdminLogin already exists → no-op,
//     the env var is ignored (does NOT allow resetting the password via
//     env — that requires a separate reset command).
//   - If no admin user exists and the env var is empty → returns
//     ErrBootstrapPasswordMissing so main() can surface a clear error.
//   - If no admin user exists and the env var is set → bcrypt-hashes it
//     and inserts a row with role='admin', status='active'.
//
// The function never logs the password value (only the fact that it
// was used to create the bootstrap account).
func EnsureBootstrapAdmin(ctx context.Context, store *Store, queries *storagedb.Queries, bootstrapPassword string) error {
	if bootstrapPassword == "" {
		// Even if an admin exists, we still want to verify it for clarity.
		_, err := store.GetByLogin(ctx, BootstrapAdminLogin)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrUserNotFound) {
			return fmt.Errorf("check bootstrap admin: %w", err)
		}
		return ErrBootstrapPasswordMissing
	}

	_, err := store.GetByLogin(ctx, BootstrapAdminLogin)
	if err == nil {
		// Admin already exists; ignore env (do NOT allow password reset
		// through this path).
		return nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return fmt.Errorf("check bootstrap admin: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrapPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login:        BootstrapAdminLogin,
		PasswordHash: string(hash),
		Role:         BootstrapAdminRole,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("insert bootstrap admin: %w", err)
	}

	return nil
}
