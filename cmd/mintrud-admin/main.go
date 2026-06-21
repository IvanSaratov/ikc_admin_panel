package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
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
		if errors.Is(err, admin.ErrBootstrapPasswordMissing) {
			// Surface as a clear stderr message and exit non-zero so an
			// operator running under systemd / docker immediately sees
			// what to do.
			fmt.Fprintln(os.Stderr, "FATAL: "+err.Error())
			os.Exit(1)
		}
		return err
	}

	server, err := app.NewServer(addr, database, nil)
	if err != nil {
		return err
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Mintrud Admin listening on http://localhost%s", addr)
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

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
