package storage

import (
	"errors"
	"fmt"
	"strings"
)

var ErrConflict = errors.New("conflict")

// MapSQLiteError maps low-level SQLite constraint messages into app-level
// errors that services can turn into field errors.
func MapSQLiteError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	if strings.Contains(message, "constraint failed") || strings.Contains(message, "constraint violation") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}

	return err
}
