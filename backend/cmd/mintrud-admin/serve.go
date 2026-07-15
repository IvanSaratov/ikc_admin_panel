package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
	"go.uber.org/zap"
)

const gracefulShutdownTimeout = 10 * time.Second

type shutdownCapableServer interface {
	Shutdown(context.Context) error
	Close() error
}

func runServe(parent context.Context, config runtimeConfig, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"Mintrud Admin starting",
		zap.String("addr_config", "MINTRUD_ADMIN_ADDR"),
		zap.String("db_config", "MINTRUD_ADMIN_DB"),
	)

	return withOwnedDatabase(ctx, config.DBPath, databaseMayCreate, func(database *sql.DB, ownedPath string) error {
		initializer, err := storage.NewEmbeddedInitializer(database, ownedPath)
		if err != nil {
			return err
		}
		preparationStarted := time.Now()
		prepared, err := initializer.Prepare(ctx)
		preparationDuration := time.Since(preparationStarted)
		logPendingMigrations(logger, prepared.Before.Pending)
		logAppliedMigrations(logger, prepared.Migration.Applied)
		logVerifiedBackup(logger, ownedPath, prepared.BackupPath)
		if err != nil {
			logMigrationFailure(logger, err)
			logDatabasePreparationFailure(
				logger,
				ownedPath,
				prepared,
				preparationDuration,
				err,
			)
			return err
		}
		logger.Info(
			"database ready",
			zap.String("database_path", ownedPath),
			zap.Int64("schema_from", prepared.Before.Current),
			zap.Int64("schema_target", prepared.Before.Target),
			zap.Int64("schema_to", prepared.After.Current),
			zap.Int("pending_migrations", len(prepared.Before.Pending)),
			zap.Int("migrations_applied", len(prepared.Migration.Applied)),
			zap.String("backup_path", prepared.BackupPath),
			zap.String("post_migration_checks", "passed"),
			zap.Duration("preparation_duration", preparationDuration),
		)

		queries := storagedb.New(database)
		if err := admin.EnsureBootstrapAdmin(
			ctx,
			admin.NewStore(queries),
			queries,
			os.Getenv("MINTRUD_ADMIN_BOOTSTRAP_PASSWORD"),
		); err != nil {
			return err
		}
		logger.Info("bootstrap admin ensured")

		sessionConfig, err := admin.LoadSessionConfig()
		if err != nil {
			return err
		}
		csrfMiddleware, err := admin.LoadCSRFWithLogger(logger)
		if err != nil {
			return err
		}
		server, err := app.NewServer(app.ServerConfig{
			Addr:     config.Addr,
			Sessions: admin.NewSessionManager(sessionConfig),
			CSRF:     csrfMiddleware,
			Frontend: frontendConfigFromEnv(),
		}, database, logger)
		if err != nil {
			return err
		}
		serverErr := make(chan error, 1)
		go func() {
			logger.Info("Mintrud Admin listening", zap.String("addr_config", "MINTRUD_ADMIN_ADDR"))
			serverErr <- server.ListenAndServe()
		}()

		select {
		case serveErr := <-serverErr:
			closeErr := server.Close()
			if errors.Is(serveErr, http.ErrServerClosed) {
				serveErr = nil
			}
			return errors.Join(serveErr, closeErr)
		case <-ctx.Done():
			logger.Info("shutdown requested")
			if err := shutdownServer(server, serverErr, gracefulShutdownTimeout); err != nil {
				return err
			}
			logger.Info("server shutdown complete")
			return nil
		}
	})
}

func shutdownServer(server shutdownCapableServer, serverErr <-chan error, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr == nil {
		serveErr := <-serverErr
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}

	closeErr := server.Close()
	serveErr := <-serverErr
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(
		fmt.Errorf("shutdown server: %w", shutdownErr),
		closeErr,
		serveErr,
	)
}

func frontendConfigFromEnv() app.FrontendConfig {
	switch env("MINTRUD_ADMIN_FRONTEND", string(app.FrontendEmbedded)) {
	case string(app.FrontendDisabled):
		return app.FrontendConfig{Mode: app.FrontendDisabled}
	default:
		return app.FrontendConfig{
			Mode:   app.FrontendEmbedded,
			Assets: os.DirFS(frontendAssetsDir()),
		}
	}
}

func frontendAssetsDir() string {
	fromRepositoryRoot := filepath.Join("frontend", "dist")
	if _, err := os.Stat(filepath.Join(fromRepositoryRoot, "index.html")); err == nil {
		return fromRepositoryRoot
	}
	return filepath.Join("..", "frontend", "dist")
}
