package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBackupRetention = 3
	automaticBackupKind    = "pre-migration"
	manualBackupKind       = "manual"
	backupTimestampLayout  = "20060102T150405.000000000Z"
)

type BackupManager struct {
	dbPath    string
	backupDir string
	retention int
	now       func() time.Time
	remove    func(string) error
	mu        sync.Mutex
}

type ownedBackup struct {
	path          string
	name          string
	kind          string
	timestamp     time.Time
	sequence      uint64
	schemaVersion int64
}

type backupReservation struct {
	path   string
	remove func(string) error
}

func NewBackupManager(dbPath string) *BackupManager {
	cleanPath := filepath.Clean(dbPath)
	if absolutePath, err := filepath.Abs(cleanPath); err == nil {
		cleanPath = filepath.Clean(absolutePath)
	}
	pathHash := sha256.Sum256([]byte(cleanPath))
	directoryName := fmt.Sprintf("%s-%x", filepath.Base(cleanPath), pathHash[:6])
	return &BackupManager{
		dbPath:    cleanPath,
		backupDir: filepath.Join(filepath.Dir(cleanPath), "backups", directoryName),
		retention: defaultBackupRetention,
		now:       time.Now,
		remove:    os.Remove,
	}
}

// Create makes a verified pre-migration snapshot. The caller must hold the
// database OwnerLock across Create and the migration operation it protects.
func (m *BackupManager) Create(ctx context.Context, db *sql.DB, schemaVersion int64) (string, error) {
	return m.create(ctx, db, schemaVersion, automaticBackupKind, true)
}

// CreateManual makes a verified administrator-requested snapshot. The caller
// must hold the database OwnerLock while CreateManual runs.
func (m *BackupManager) CreateManual(ctx context.Context, db *sql.DB, schemaVersion int64) (string, error) {
	return m.create(ctx, db, schemaVersion, manualBackupKind, false)
}

func (m *BackupManager) create(ctx context.Context, db *sql.DB, schemaVersion int64, kind string, prune bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.validateSource(ctx, db, schemaVersion); err != nil {
		return "", err
	}
	if err := m.prepareBackupDirectory(); err != nil {
		return "", err
	}

	reservation, err := m.reserveBackup(kind, m.now().UTC(), schemaVersion)
	if err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, reservation.path); err != nil {
		operationErr := fmt.Errorf("create SQLite backup %q: %w", reservation.path, err)
		return "", reservation.cleanup(operationErr)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(reservation.path, 0o600); err != nil {
			operationErr := fmt.Errorf("set SQLite backup mode %q: %w", reservation.path, err)
			return "", reservation.cleanup(operationErr)
		}
	}
	if err := verifyDatabaseFile(ctx, reservation.path, schemaVersion); err != nil {
		operationErr := fmt.Errorf("verify SQLite backup %q: %w", reservation.path, err)
		return "", reservation.cleanup(operationErr)
	}
	if prune {
		if err := m.pruneAutomatic(reservation.path); err != nil {
			return reservation.path, fmt.Errorf("prune automatic SQLite backups after creating %q: %w", reservation.path, err)
		}
	}

	return reservation.path, nil
}

func (m *BackupManager) validateSource(ctx context.Context, db *sql.DB, schemaVersion int64) error {
	if schemaVersion < 0 {
		return fmt.Errorf("backup schema version must be non-negative: %d", schemaVersion)
	}
	if err := m.validateDatabasePath(ctx, db); err != nil {
		return err
	}

	if _, err := InspectDatabaseIdentity(ctx, db); err != nil {
		return fmt.Errorf("inspect backup source identity: %w", err)
	}
	currentVersion, err := readGooseSchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read backup source schema version: %w", err)
	}
	if currentVersion != schemaVersion {
		return fmt.Errorf("backup schema version %d differs from current Goose version %d", schemaVersion, currentVersion)
	}
	return nil
}

func (m *BackupManager) validateDatabasePath(ctx context.Context, db *sql.DB) error {
	if m == nil {
		return errors.New("SQLite backup manager is required")
	}
	if db == nil {
		return errors.New("SQLite backup source database is required")
	}

	configuredPath, err := canonicalDatabasePath(m.dbPath)
	if err != nil {
		return fmt.Errorf("canonicalize configured backup source %q: %w", m.dbPath, err)
	}
	mainPath, err := mainDatabasePath(ctx, db)
	if err != nil {
		return err
	}
	canonicalMainPath, err := canonicalDatabasePath(mainPath)
	if err != nil {
		return fmt.Errorf("canonicalize open SQLite main database %q: %w", mainPath, err)
	}
	if !sameDatabasePath(configuredPath, canonicalMainPath) {
		return fmt.Errorf("open SQLite main database %q differs from configured backup source %q", canonicalMainPath, configuredPath)
	}
	return nil
}

func mainDatabasePath(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("query SQLite database list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			sequence int64
			name     string
			path     string
		)
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			return "", fmt.Errorf("scan SQLite database list: %w", err)
		}
		if name == "main" {
			if path == "" {
				return "", errors.New("open SQLite main database has no filesystem path")
			}
			return filepath.Clean(path), nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate SQLite database list: %w", err)
	}
	return "", errors.New("SQLite database list has no main database")
}

func sameDatabasePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func readGooseSchemaVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var version int64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied = 1
	`).Scan(&version); err != nil {
		return 0, fmt.Errorf("query current Goose schema version: %w", err)
	}
	return version, nil
}

func (m *BackupManager) prepareBackupDirectory() error {
	if err := os.MkdirAll(m.backupDir, 0o700); err != nil {
		return fmt.Errorf("create backup directory %q: %w", m.backupDir, err)
	}
	// Windows ACL hardening remains the responsibility of the later MSI and
	// installer plan; os.FileMode permissions are only enforced on Unix hosts.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(m.backupDir, 0o700); err != nil {
			return fmt.Errorf("set backup directory mode %q: %w", m.backupDir, err)
		}
	}
	return nil
}

func (m *BackupManager) reserveBackup(kind string, timestamp time.Time, schemaVersion int64) (*backupReservation, error) {
	backups, err := m.ownedBackups()
	if err != nil {
		return nil, err
	}
	var maximumSequence uint64
	for _, backup := range backups {
		if backup.sequence > maximumSequence {
			maximumSequence = backup.sequence
		}
	}
	if maximumSequence == ^uint64(0) {
		return nil, errors.New("SQLite backup sequence is exhausted")
	}

	for sequence := maximumSequence + 1; ; sequence++ {
		name := m.backupFilename(kind, timestamp, sequence, schemaVersion)
		path := filepath.Join(m.backupDir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			if sequence == ^uint64(0) {
				return nil, errors.New("SQLite backup sequence is exhausted")
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reserve SQLite backup destination %q: %w", path, err)
		}

		reservation := &backupReservation{path: path, remove: m.remove}
		if err := file.Close(); err != nil {
			operationErr := fmt.Errorf("close reserved SQLite backup destination %q: %w", path, err)
			return nil, reservation.cleanup(operationErr)
		}
		return reservation, nil
	}
}

func (m *BackupManager) backupFilename(kind string, timestamp time.Time, sequence uint64, schemaVersion int64) string {
	return fmt.Sprintf(
		"%s.%s.%s.s%06d.v%06d.db",
		filepath.Base(m.dbPath),
		kind,
		timestamp.UTC().Format(backupTimestampLayout),
		sequence,
		schemaVersion,
	)
}

func (reservation *backupReservation) cleanup(operationErr error) error {
	if reservation == nil || reservation.remove == nil {
		return operationErr
	}
	if err := reservation.remove(reservation.path); err != nil {
		cleanupErr := fmt.Errorf("remove reserved SQLite backup %q: %w", reservation.path, err)
		return errors.Join(operationErr, cleanupErr)
	}
	return operationErr
}

func verifyDatabaseFile(ctx context.Context, path string, expectedVersion int64) (err error) {
	db, err := OpenReadOnly(ctx, path)
	if err != nil {
		return fmt.Errorf("open database file: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database file: %w", closeErr))
		}
	}()

	if err := QuickCheck(ctx, db); err != nil {
		return fmt.Errorf("quick-check database file: %w", err)
	}
	if _, err := InspectDatabaseIdentity(ctx, db); err != nil {
		return fmt.Errorf("inspect database file identity: %w", err)
	}
	version, err := readGooseSchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database file schema version: %w", err)
	}
	if version != expectedVersion {
		return fmt.Errorf("database file Goose version %d differs from expected version %d", version, expectedVersion)
	}
	return nil
}

func (m *BackupManager) ownedBackups() ([]ownedBackup, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup directory %q: %w", m.backupDir, err)
	}

	backups := make([]ownedBackup, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		backup, ok := m.parseOwnedBackup(entry.Name())
		if !ok {
			continue
		}
		backup.path = filepath.Join(m.backupDir, entry.Name())
		backups = append(backups, backup)
	}
	return backups, nil
}

func (m *BackupManager) parseOwnedBackup(name string) (ownedBackup, bool) {
	pattern := `\A` + regexp.QuoteMeta(filepath.Base(m.dbPath)) +
		`\.(pre-migration|manual)\.([0-9]{8}T[0-9]{6}\.[0-9]{9}Z)\.s([0-9]{6,})\.v([0-9]{6,})\.db\z`
	matches := regexp.MustCompile(pattern).FindStringSubmatch(name)
	if matches == nil {
		return ownedBackup{}, false
	}
	timestamp, err := time.Parse(backupTimestampLayout, matches[2])
	if err != nil || timestamp.UTC().Format(backupTimestampLayout) != matches[2] {
		return ownedBackup{}, false
	}
	sequence, err := strconv.ParseUint(matches[3], 10, 64)
	if err != nil || sequence == 0 {
		return ownedBackup{}, false
	}
	schemaVersion, err := strconv.ParseInt(matches[4], 10, 64)
	if err != nil || schemaVersion < 0 {
		return ownedBackup{}, false
	}
	return ownedBackup{
		name:          name,
		kind:          matches[1],
		timestamp:     timestamp,
		sequence:      sequence,
		schemaVersion: schemaVersion,
	}, true
}

func (m *BackupManager) pruneAutomatic(currentPath string) error {
	backups, err := m.ownedBackups()
	if err != nil {
		return err
	}
	automatic := make([]ownedBackup, 0, len(backups))
	for _, backup := range backups {
		if backup.kind == automaticBackupKind {
			automatic = append(automatic, backup)
		}
	}
	sort.Slice(automatic, func(left, right int) bool {
		if automatic[left].sequence == automatic[right].sequence {
			return automatic[left].name < automatic[right].name
		}
		return automatic[left].sequence < automatic[right].sequence
	})

	removeCount := len(automatic) - m.retention
	for _, backup := range automatic {
		if removeCount <= 0 {
			break
		}
		if filepath.Clean(backup.path) == filepath.Clean(currentPath) {
			continue
		}
		if err := m.remove(backup.path); err != nil {
			return fmt.Errorf("remove expired SQLite backup %q: %w", backup.path, err)
		}
		removeCount--
	}
	return nil
}
