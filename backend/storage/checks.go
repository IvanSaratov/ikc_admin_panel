package storage

import (
	"context"
	"database/sql"
	"fmt"
)

func checkIntegrityPragma(ctx context.Context, db *sql.DB, pragma string) error {
	rows, err := db.QueryContext(ctx, pragma)
	if err != nil {
		return fmt.Errorf("query SQLite integrity check %q: %w", pragma, err)
	}
	defer rows.Close()

	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("scan SQLite integrity check %q: %w", pragma, err)
		}
		if result != "ok" {
			return fmt.Errorf("SQLite integrity check %q failed: %s", pragma, result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate SQLite integrity check %q: %w", pragma, err)
	}
	return nil
}

func QuickCheck(ctx context.Context, db *sql.DB) error {
	return checkIntegrityPragma(ctx, db, "PRAGMA quick_check")
}

func IntegrityCheck(ctx context.Context, db *sql.DB) error {
	return checkIntegrityPragma(ctx, db, "PRAGMA integrity_check")
}

func ForeignKeyCheck(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("query SQLite foreign key check: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var (
			table        string
			rowID        sql.NullInt64
			parent       string
			foreignKeyID int64
		)
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return fmt.Errorf("scan SQLite foreign key violation: %w", err)
		}
		return fmt.Errorf(
			"SQLite foreign key violation: table %q, row ID %v, parent %q, foreign key ID %d",
			table,
			rowID,
			parent,
			foreignKeyID,
		)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate SQLite foreign key check: %w", err)
	}
	return nil
}
