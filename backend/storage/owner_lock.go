package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

var ErrDatabaseInUse = errors.New("database is already owned by another process")

type OwnerLock struct {
	file         *flock.Flock
	databasePath string
}

// AcquireOwnerLock takes the cooperative process-ownership lock for dbPath.
// Hard-linked database aliases cannot be distinguished by this path-based lock
// and are unsupported; callers must not configure the same database through
// hard links.
func AcquireOwnerLock(dbPath string) (*OwnerLock, error) {
	canonicalPath, err := canonicalDatabasePath(dbPath)
	if err != nil {
		return nil, err
	}

	lockPath := canonicalPath + ".lock"
	file := flock.New(lockPath, flock.SetPermissions(0o600))
	locked, err := file.TryLock()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire database owner lock %q: %w", lockPath, err)
	}
	if !locked {
		_ = file.Close()
		return nil, fmt.Errorf("%w: %s", ErrDatabaseInUse, lockPath)
	}

	return &OwnerLock{file: file, databasePath: canonicalPath}, nil
}

// DatabasePath returns the immutable canonical database path protected by the
// lock. Callers must open this path instead of resolving the configured path a
// second time after ownership has been acquired.
func (owner *OwnerLock) DatabasePath() string {
	if owner == nil {
		return ""
	}
	return owner.databasePath
}

func canonicalDatabasePath(dbPath string) (string, error) {
	if dbPath == "" {
		return "", errors.New("database path is empty")
	}

	absolutePath, err := filepath.Abs(dbPath)
	if err != nil {
		return "", fmt.Errorf("make database path absolute %q: %w", dbPath, err)
	}
	_, statErr := os.Stat(absolutePath)
	if statErr == nil {
		canonicalPath, err := filepath.EvalSymlinks(absolutePath)
		if err != nil {
			return "", fmt.Errorf("canonicalize database path %q: %w", absolutePath, err)
		}
		canonicalPath, err = filepath.Abs(canonicalPath)
		if err != nil {
			return "", fmt.Errorf("make canonical database path absolute %q: %w", canonicalPath, err)
		}
		return filepath.Clean(canonicalPath), nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("stat database path %q: %w", absolutePath, statErr)
	}

	info, lstatErr := os.Lstat(absolutePath)
	if lstatErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("dangling database symlink %q is unsupported", absolutePath)
		}
		return "", fmt.Errorf("stat database path %q: %w", absolutePath, statErr)
	}
	if !errors.Is(lstatErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect missing database path %q: %w", absolutePath, lstatErr)
	}

	parent := filepath.Dir(absolutePath)
	canonicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("canonicalize database parent %q: %w", parent, err)
	}
	canonicalParent, err = filepath.Abs(canonicalParent)
	if err != nil {
		return "", fmt.Errorf("make canonical database parent absolute %q: %w", canonicalParent, err)
	}
	return filepath.Clean(filepath.Join(canonicalParent, filepath.Base(absolutePath))), nil
}

func (owner *OwnerLock) Close() error {
	if owner == nil || owner.file == nil {
		return nil
	}

	path := owner.file.Path()
	if err := owner.file.Close(); err != nil {
		return fmt.Errorf("close database owner lock %q: %w", path, err)
	}
	owner.file = nil
	return nil
}
