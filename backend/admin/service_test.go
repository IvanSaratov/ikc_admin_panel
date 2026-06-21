package admin_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"golang.org/x/crypto/bcrypt"
)

// newTestService wires an admin.Service + storage.Store against a
// fresh in-memory-style SQLite DB. The admin user is seeded with a
// known bcrypt password so authentication tests have something to
// verify against.
func newTestService(t *testing.T) (*admin.Service, context.Context) {
	t.Helper()

	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "admin-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	queries := storagedb.New(db)
	hash, err := bcrypt.GenerateFromPassword([]byte("known-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login:        "alice",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	return admin.NewService(queries), ctx
}

func TestAuthenticate_ValidCredentials(t *testing.T) {
	t.Parallel()

	svc, ctx := newTestService(t)

	user, err := svc.Authenticate(ctx, "alice", "known-password")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.Login != "alice" {
		t.Errorf("login = %q, want alice", user.Login)
	}
	if user.Status != "active" {
		t.Errorf("status = %q, want active", user.Status)
	}
}

func TestAuthenticate_WrongPassword_Errors(t *testing.T) {
	t.Parallel()

	svc, ctx := newTestService(t)

	_, err := svc.Authenticate(ctx, "alice", "WRONG")
	if err == nil {
		t.Fatalf("expected error for wrong password")
	}
	if err != admin.ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticate_DisabledUser_Errors(t *testing.T) {
	t.Parallel()

	_, ctx := newTestService(t)

	// Mark alice disabled.
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "admin-disabled.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	queries := storagedb.New(db)
	hash, _ := bcrypt.GenerateFromPassword([]byte("known-password"), bcrypt.MinCost)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := queries.CreateUser(ctx, storagedb.CreateUserParams{
		Login:        "alice",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "disabled",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svcDisabled := admin.NewService(queries)
	_, err = svcDisabled.Authenticate(ctx, "alice", "known-password")
	if err == nil {
		t.Fatalf("expected error for disabled user")
	}
	// The plan says "DisabledUser_Errors" — we use ErrInvalidCredentials
	// as the public-facing error so the caller cannot distinguish
	// "wrong password" from "disabled account". The internal flow is
	// validated separately by inspecting audit reasons.
	if err != admin.ErrInvalidCredentials && err != admin.ErrUserDisabled {
		t.Fatalf("err = %v, want ErrInvalidCredentials or ErrUserDisabled", err)
	}
}

func TestAuthenticate_UnknownUser_Errors(t *testing.T) {
	t.Parallel()

	svc, ctx := newTestService(t)
	_, err := svc.Authenticate(ctx, "ghost", "any")
	if err != admin.ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticate_EmptyInputs_Errors(t *testing.T) {
	t.Parallel()

	svc, ctx := newTestService(t)
	if _, err := svc.Authenticate(ctx, "", ""); err != admin.ErrInvalidCredentials {
		t.Fatalf("empty inputs: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Authenticate(ctx, "alice", ""); err != admin.ErrInvalidCredentials {
		t.Fatalf("empty password: err = %v, want ErrInvalidCredentials", err)
	}
}
