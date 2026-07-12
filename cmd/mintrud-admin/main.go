package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/platform/logging"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"github.com/sirupsen/logrus"
)

func main() {
	logger, err := newLoggerFromEnv(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL: invalid log configuration")
		os.Exit(1)
	}

	if err := run(logger); err != nil {
		logger.Fatal(err)
	}
}

func run(logger logrus.FieldLogger) error {
	ctx := context.Background()
	addr := env("MINTRUD_ADMIN_ADDR", ":8080")
	dbPath := env("MINTRUD_ADMIN_DB", filepath.Join("data", "mintrud-admin.db"))

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	database, err := storage.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := storage.Migrate(database); err != nil {
		return err
	}

	queries := storagedb.New(database)
	if err := admin.EnsureBootstrapAdmin(ctx, admin.NewStore(queries), queries, os.Getenv("MINTRUD_ADMIN_BOOTSTRAP_PASSWORD")); err != nil {
		return err
	}

	server, err := app.NewServer(addr, database, logger, frontendConfigFromEnv())
	if err != nil {
		return err
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.WithField("addr", addr).Info("Mintrud Admin listening")
		serverErr <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case <-stop:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func newLoggerFromEnv(output io.Writer) (*logrus.Logger, error) {
	return logging.New(logging.Config{
		Env:    os.Getenv("MINTRUD_ADMIN_ENV"),
		Level:  os.Getenv("MINTRUD_ADMIN_LOG_LEVEL"),
		Format: os.Getenv("MINTRUD_ADMIN_LOG_FORMAT"),
		Output: output,
	})
}

func frontendConfigFromEnv() app.FrontendConfig {
	switch env("MINTRUD_ADMIN_FRONTEND", string(app.FrontendEmbedded)) {
	case string(app.FrontendDisabled):
		return app.FrontendConfig{Mode: app.FrontendDisabled}
	default:
		return app.FrontendConfig{
			Mode:   app.FrontendEmbedded,
			Assets: os.DirFS(filepath.Join("frontend", "dist")),
		}
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
