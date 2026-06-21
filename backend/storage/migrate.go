package storage

import (
	"database/sql"
	"fmt"

	"github.com/IvanSaratov/ikc_admin_panel/migrations"
	"github.com/pressly/goose/v3"
)

// Migrate applies embedded database migrations.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
