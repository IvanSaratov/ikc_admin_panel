package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	core, observed := observer.New(zap.InfoLevel)
	err = runServe(
		ctx,
		runtimeConfig{Addr: "127.0.0.1:0", DBPath: dbPath},
		zap.New(core),
	)
	if !errors.Is(err, storage.ErrSchemaTooNew) {
		t.Fatalf("runServe error = %v, want ErrSchemaTooNew", err)
	}
	if observed.FilterMessage("Mintrud Admin listening").Len() != 0 {
		t.Fatal("too-new startup reached HTTP listening stage")
	}
	owner, err := storage.AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("owner lock remains held after failed startup: %v", err)
	}
	owner.Close()
}

func TestRunServeHoldsOwnerLockUntilShutdown(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "serve.db")
	t.Setenv("MINTRUD_ADMIN_BOOTSTRAP_PASSWORD", "synthetic-test-password")
	t.Setenv("MINTRUD_ADMIN_FRONTEND", "disabled")
	ctx, cancel := context.WithCancel(context.Background())
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	result := make(chan error, 1)
	go func() {
		result <- runServe(
			ctx,
			runtimeConfig{Addr: "127.0.0.1:0", DBPath: dbPath},
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
}

func TestRunServeReleasesOwnerLockWhenBootstrapFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing-bootstrap.db")
	t.Setenv("MINTRUD_ADMIN_BOOTSTRAP_PASSWORD", "")
	t.Setenv("MINTRUD_ADMIN_FRONTEND", "disabled")

	err := runServe(
		context.Background(),
		runtimeConfig{Addr: "127.0.0.1:0", DBPath: dbPath},
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
