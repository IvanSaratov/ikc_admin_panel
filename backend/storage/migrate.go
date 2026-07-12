package storage

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/IvanSaratov/ikc_admin_panel/backend/migrations"
	"github.com/pressly/goose/v3"
)

// migrateOnce guards calls to goose.SetBaseFS / goose.SetDialect, which
// write to package-level globals in pressly/goose. Concurrent Migrate
// calls (notably from parallel tests under -race) would otherwise race
// on those globals and fail the detector. The actual migration work
// (goose.Up) is still per-DB and unaffected by the lock.
var migrateOnce sync.Mutex

// Migrate applies embedded database migrations.
//
// The SetBaseFS / SetDialect calls are no-ops once the globals are
// initialised, but goose still writes to them on every call. Serialise
// that write under migrateOnce so parallel test runs don't race on the
// goose globals; goose.Up itself remains per-DB and is safe to run
// concurrently on different *sql.DB handles.
func Migrate(db *sql.DB) error {
	migrateOnce.Lock()
	defer migrateOnce.Unlock()

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
