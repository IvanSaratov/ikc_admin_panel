package runner

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type shutdownTestServer struct {
	shutdownErr error
	closeErr    error
	closed      bool
	onClose     func()
}

func resolvedServeTestConfig(databasePath, bootstrapPassword string) ResolvedServeConfig {
	return ResolvedServeConfig{
		Address:           "127.0.0.1:0",
		DatabasePath:      databasePath,
		BootstrapPassword: bootstrapPassword,
		Session: admin.SessionConfig{
			TTL:      8 * time.Hour,
			SameSite: http.SameSiteLaxMode,
		},
		CSRF: admin.CSRFConfig{Key: strings.Repeat("ab", 32)},
		Frontend: app.FrontendConfig{
			Mode: app.FrontendDisabled,
		},
	}
}

func TestFrontendAssetsDirPrefersRepositoryRootLayout(t *testing.T) {
	root := t.TempDir()
	distDir := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("create frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), nil, 0o644); err != nil {
		t.Fatalf("create frontend index: %v", err)
	}
	t.Chdir(root)

	if got := frontendAssetsDir(); got != filepath.Join("frontend", "dist") {
		t.Errorf("frontendAssetsDir() = %q, want %q", got, filepath.Join("frontend", "dist"))
	}
}

func TestFrontendAssetsDirFallsBackFromBackendModule(t *testing.T) {
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	distDir := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("create backend directory: %v", err)
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("create frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), nil, 0o644); err != nil {
		t.Fatalf("create frontend index: %v", err)
	}
	t.Chdir(backendDir)

	if got := frontendAssetsDir(); got != filepath.Join("..", "frontend", "dist") {
		t.Errorf("frontendAssetsDir() = %q, want %q", got, filepath.Join("..", "frontend", "dist"))
	}
}

func (server *shutdownTestServer) Shutdown(context.Context) error {
	return server.shutdownErr
}

func (server *shutdownTestServer) Close() error {
	server.closed = true
	if server.onClose != nil {
		server.onClose()
	}
	return server.closeErr
}

func TestShutdownServerForcesCloseAndWaitsAfterGracefulFailure(t *testing.T) {
	shutdownErr := errors.New("synthetic shutdown timeout")
	closeErr := errors.New("synthetic forced-close warning")
	serveErr := make(chan error, 1)
	server := &shutdownTestServer{
		shutdownErr: shutdownErr,
		closeErr:    closeErr,
		onClose: func() {
			serveErr <- http.ErrServerClosed
		},
	}

	err := shutdownServer(server, serveErr, time.Millisecond)
	if !errors.Is(err, shutdownErr) {
		t.Fatalf("shutdownServer error = %v, want shutdown error", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("shutdownServer error = %v, want forced-close error", err)
	}
	if !server.closed {
		t.Fatal("shutdownServer did not force close after graceful failure")
	}
	select {
	case leftover := <-serveErr:
		t.Fatalf("shutdownServer did not wait for serve result: %v", leftover)
	default:
	}
}

func TestRunServeRejectsTooNewSchemaBeforeStartingHTTP(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "too-new.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	newerFS := fstest.MapFS{
		"001_base.sql": &fstest.MapFile{Data: []byte(
			"-- +goose Up\nPRAGMA application_id = 0x494B4341;\n" +
				"CREATE TABLE marker (id INTEGER PRIMARY KEY);\n",
		)},
		"002_newer.sql": &fstest.MapFile{Data: []byte(
			"-- +goose Up\nALTER TABLE marker ADD COLUMN note TEXT;\n",
		)},
	}
	migrator, err := storage.NewMigrator(db, newerFS)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("migrate too-new database: %v", err)
	}
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = DELETE`).Scan(&journalMode); err != nil {
		t.Fatalf("set too-new journal mode: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	core, observed := observer.New(zap.InfoLevel)
	err = runServe(
		ctx,
		resolvedServeTestConfig(dbPath, "synthetic-test-password"),
		zap.New(core),
	)
	if !errors.Is(err, storage.ErrSchemaTooNew) {
		t.Fatalf("runServe error = %v, want ErrSchemaTooNew", err)
	}
	if observed.FilterMessage("Mintrud Admin listening").Len() != 0 {
		t.Fatal("too-new startup reached HTTP listening stage")
	}
	assertCommandDatabaseJournalMode(t, ctx, dbPath, "delete")
	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("owner lock remains held after failed startup: %v", err)
	}
	owner.Close()
}

func TestRunServeHoldsOwnerLockUntilShutdown(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "serve.db")
	ctx, cancel := context.WithCancel(context.Background())
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	result := make(chan error, 1)
	go func() {
		result <- runServe(
			ctx,
			resolvedServeTestConfig(dbPath, "synthetic-test-password"),
			logger,
		)
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	lockObserved := false
	for !lockObserved || observed.FilterMessage("Mintrud Admin listening").Len() == 0 {
		if !lockObserved {
			probe, err := storage.AcquireOwnerLock(dbPath)
			switch {
			case errors.Is(err, storage.ErrDatabaseInUse):
				lockObserved = true
			case err != nil:
				cancel()
				t.Fatalf("probe owner lock: %v", err)
			default:
				probe.Close()
			}
		}
		select {
		case err := <-result:
			cancel()
			t.Fatalf("runServe returned before lock observation: %v", err)
		case <-deadline.C:
			cancel()
			t.Fatal("timed out waiting for server owner lock")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runServe shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
	after, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("acquire owner after shutdown: %v", err)
	}
	after.Close()
	assertCommandDatabaseJournalMode(t, context.Background(), dbPath, "wal")
}

func TestRunServeReleasesOwnerLockWhenBootstrapFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing-bootstrap.db")

	err := runServe(
		context.Background(),
		resolvedServeTestConfig(dbPath, ""),
		nil,
	)
	if !errors.Is(err, admin.ErrBootstrapPasswordMissing) {
		t.Fatalf("runServe error = %v, want ErrBootstrapPasswordMissing", err)
	}

	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("owner lock remains held after bootstrap failure: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close owner lock: %v", err)
	}
}

func TestRunServeLogsChecksPassedForCurrentSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "current.db")
	if err := RunDatabase(context.Background(), DatabaseMigrate, DatabaseConfig{
		Runtime:      testRuntimeConfig(),
		DatabasePath: dbPath,
	}, io.Discard); err != nil {
		t.Fatalf("migrate current database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	core, observed := observer.New(zap.InfoLevel)
	result := make(chan error, 1)
	go func() {
		result <- runServe(
			ctx,
			resolvedServeTestConfig(dbPath, "synthetic-test-password"),
			zap.New(core),
		)
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for observed.FilterMessage("Mintrud Admin listening").Len() == 0 {
		select {
		case err := <-result:
			t.Fatalf("runServe returned before listening: %v", err)
		case <-deadline.C:
			t.Fatal("timed out waiting for server startup")
		case <-time.After(10 * time.Millisecond):
		}
	}
	ready := observed.FilterMessage("database ready").All()
	if len(ready) != 1 {
		t.Fatalf("database ready log count = %d, want 1", len(ready))
	}
	if got := ready[0].ContextMap()["post_migration_checks"]; got != "passed" {
		t.Fatalf("post_migration_checks = %v, want passed", got)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runServe shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}
