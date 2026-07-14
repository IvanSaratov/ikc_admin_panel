package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultBackupRetention = 3

type BackupManager struct {
	dbPath    string
	backupDir string
	retention int
	now       func() time.Time
}

func NewBackupManager(dbPath string) *BackupManager {
	cleanPath := filepath.Clean(dbPath)
	return &BackupManager{
		dbPath:    cleanPath,
		backupDir: filepath.Join(filepath.Dir(cleanPath), "backups"),
		retention: defaultBackupRetention,
		now:       time.Now,
	}
}

func (m *BackupManager) Create(ctx context.Context, db *sql.DB, schemaVersion int64) (string, error) {
	return m.create(ctx, db, schemaVersion, "pre-migration", true)
}

func (m *BackupManager) CreateManual(ctx context.Context, db *sql.DB, schemaVersion int64) (string, error) {
	return m.create(ctx, db, schemaVersion, "manual", false)
}

func (m *BackupManager) create(ctx context.Context, db *sql.DB, schemaVersion int64, kind string, prune bool) (string, error) {
	if err := os.MkdirAll(m.backupDir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory %q: %w", m.backupDir, err)
	}

	base := filepath.Base(m.dbPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	timestamp := m.now().UTC().Format("20060102T150405.000000000Z")
	name := fmt.Sprintf("%s.%s.%s.v%06d.db", base, kind, timestamp, schemaVersion)
	backupPath := filepath.Join(m.backupDir, name)

	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("create SQLite backup %q: %w", backupPath, err)
	}
	if err := verifyDatabaseFile(ctx, backupPath); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("verify SQLite backup %q: %w", backupPath, err)
	}
	if prune {
		if err := m.pruneAutomatic(); err != nil {
			return "", err
		}
	}

	return backupPath, nil
}

func verifyDatabaseFile(ctx context.Context, path string) error {
	db, err := OpenReadOnly(ctx, path)
	if err != nil {
		return fmt.Errorf("open database file: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping database file: %w", err)
	}
	if err := QuickCheck(ctx, db); err != nil {
		_ = db.Close()
		return fmt.Errorf("quick-check database file: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close database file: %w", err)
	}
	return nil
}

func (m *BackupManager) pruneAutomatic() error {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return fmt.Errorf("read backup directory %q: %w", m.backupDir, err)
	}

	base := filepath.Base(m.dbPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	prefix := base + ".pre-migration."
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".db") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for len(names) > m.retention {
		backupPath := filepath.Join(m.backupDir, names[0])
		names = names[1:]
		if filepath.Clean(backupPath) == m.dbPath {
			continue
		}
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("remove expired backup %q: %w", backupPath, err)
		}
	}
	return nil
}
