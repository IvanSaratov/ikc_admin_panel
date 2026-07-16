package app_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
)

func TestNewServerUsesExplicitConfiguration(t *testing.T) {
	server, err := app.NewServer(app.ServerConfig{
		Addr: ":0",
		Sessions: admin.NewSessionManager(admin.SessionConfig{
			TTL: time.Hour, SameSite: http.SameSiteLaxMode, Secure: false,
		}),
		CSRF:     func(next http.Handler) http.Handler { return next },
		Frontend: app.FrontendConfig{Mode: app.FrontendDisabled},
	}, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server == nil {
		t.Fatal("nil server")
	}
}

func TestNewServerCreatesPrivateImportUploadDirectory(t *testing.T) {
	database, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	uploadRoot := filepath.Join(t.TempDir(), "private", "imports")
	server, err := app.NewServer(app.ServerConfig{
		Addr:            ":0",
		Sessions:        admin.NewSessionManager(admin.SessionConfig{TTL: time.Hour}),
		CSRF:            func(next http.Handler) http.Handler { return next },
		Frontend:        app.FrontendConfig{Mode: app.FrontendDisabled},
		ImportUploadDir: uploadRoot,
	}, database, zap.NewNop())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server == nil {
		t.Fatal("nil server")
	}
	info, err := os.Stat(uploadRoot)
	if err != nil {
		t.Fatalf("stat upload root: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("upload root mode = %04o, want 0700", info.Mode().Perm())
	}
}

func TestNewServerRejectsMissingOrSymlinkImportUploadDirectory(t *testing.T) {
	database, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	baseConfig := app.ServerConfig{
		Addr:     ":0",
		Sessions: admin.NewSessionManager(admin.SessionConfig{TTL: time.Hour}),
		CSRF:     func(next http.Handler) http.Handler { return next },
		Frontend: app.FrontendConfig{Mode: app.FrontendDisabled},
	}
	if _, err := app.NewServer(baseConfig, database, zap.NewNop()); err == nil {
		t.Fatal("server accepted missing import upload directory")
	}
	realRoot := filepath.Join(t.TempDir(), "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatalf("create real root: %v", err)
	}
	symlinkRoot := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realRoot, symlinkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	baseConfig.ImportUploadDir = symlinkRoot
	if _, err := app.NewServer(baseConfig, database, zap.NewNop()); err == nil {
		t.Fatal("server accepted symlink import upload directory")
	}
}
