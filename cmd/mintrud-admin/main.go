package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
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

	server := app.NewServer(addr, database)
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
