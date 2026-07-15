package runner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
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

func Serve(ctx context.Context, config ServeConfig, stdout io.Writer) error {
	resolved, err := ResolveServeConfig(config)
	if err != nil {
		return err
	}
	return withLogger(config.Runtime, stdout, func(logger *zap.Logger) error {
		return runServe(ctx, resolved, logger)
	})
}

func runServe(parent context.Context, resolved ResolvedServeConfig, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"Mintrud Admin starting",
		zap.String("addr_config", "IKC_SERVER_ADDR"),
		zap.String("db_config", "IKC_SERVER_DB"),
	)

	return withOwnedDatabase(ctx, resolved.DatabasePath, databaseMayCreate, func(database *sql.DB, ownedPath string) error {
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
			resolved.BootstrapPassword,
		); err != nil {
			return err
		}
		logger.Info("bootstrap admin ensured")

		csrfMiddleware, err := admin.NewCSRFMiddleware(resolved.CSRF, logger)
		if err != nil {
			return err
		}
		frontend := resolved.Frontend
		if frontend.Mode == app.FrontendEmbedded {
			frontend.Assets = os.DirFS(frontendAssetsDir())
		}
		server, err := app.NewServer(app.ServerConfig{
			Addr:     resolved.Address,
			Sessions: admin.NewSessionManager(resolved.Session),
			CSRF:     csrfMiddleware,
			Frontend: frontend,
		}, database, logger)
		if err != nil {
			return err
		}
		serverErr := make(chan error, 1)
		go func() {
			logger.Info("Mintrud Admin listening", zap.String("addr_config", "IKC_SERVER_ADDR"))
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

func frontendAssetsDir() string {
	fromRepositoryRoot := filepath.Join("frontend", "dist")
	if _, err := os.Stat(filepath.Join(fromRepositoryRoot, "index.html")); err == nil {
		return fromRepositoryRoot
	}
	return filepath.Join("..", "frontend", "dist")
}
