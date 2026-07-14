package storage

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gofrs/flock"
)

var ErrDatabaseInUse = errors.New("database is already owned by another process")

type OwnerLock struct {
	file *flock.Flock
}

func AcquireOwnerLock(dbPath string) (*OwnerLock, error) {
	if dbPath == "" {
		return nil, errors.New("database path is empty")
	}

	lockPath := filepath.Clean(dbPath) + ".lock"
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

	return &OwnerLock{file: file}, nil
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
