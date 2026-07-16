package imports

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const uploadSuffix = ".upload"

// LocalFileStore keeps uploads in a private directory under opaque names.
type LocalFileStore struct {
	root string
}

// NewLocalFileStore creates or hardens a private upload directory.
func NewLocalFileStore(root string) (*LocalFileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, &ServiceError{Code: CodeInvalidInput, Detail: "temporary storage root is required"}
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, storageFailure("temporary storage root is invalid", err)
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(absolute, 0o700); err != nil {
			return nil, storageFailure("temporary storage cannot be created", err)
		}
		info, err = os.Lstat(absolute)
	}
	if err != nil {
		return nil, storageFailure("temporary storage cannot be inspected", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, storageFailure("temporary storage root must be a real directory", nil)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, storageFailure("temporary storage permissions cannot be applied", err)
	}
	return &LocalFileStore{root: absolute}, nil
}

// Save streams an upload to a private file while calculating its SHA-256.
func (s *LocalFileStore) Save(ctx context.Context, source io.Reader, maxBytes int64) (StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	if source == nil || maxBytes <= 0 {
		return StoredFile{}, &ServiceError{Code: CodeInvalidInput, Detail: "upload stream and positive limit are required"}
	}
	token, err := randomHexToken(32)
	if err != nil {
		return StoredFile{}, storageFailure("temporary file token cannot be generated", err)
	}
	path, err := s.pathForToken(token)
	if err != nil {
		return StoredFile{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return StoredFile{}, storageFailure("temporary file cannot be created", err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(path)
		}
	}()

	hash := sha256.New()
	written, err := copyWithContextAndLimit(ctx, io.MultiWriter(file, hash), source, maxBytes)
	if err != nil {
		return StoredFile{}, err
	}
	if err := file.Sync(); err != nil {
		return StoredFile{}, storageFailure("temporary file cannot be synchronized", err)
	}
	if err := file.Close(); err != nil {
		return StoredFile{}, storageFailure("temporary file cannot be closed", err)
	}
	committed = true
	return StoredFile{
		Token:  token,
		Path:   path,
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		Size:   written,
	}, nil
}

// Path resolves an existing regular upload file by opaque token.
func (s *LocalFileStore) Path(token string) (string, error) {
	path, err := s.pathForToken(token)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", storageFailure("temporary file is unavailable", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", storageFailure("temporary file must be a regular file", nil)
	}
	return path, nil
}

// Delete removes an upload. Repeating deletion of an absent token is safe.
func (s *LocalFileStore) Delete(token string) error {
	path, err := s.pathForToken(token)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return storageFailure("temporary file cannot be deleted", err)
	}
	return nil
}

func (s *LocalFileStore) pathForToken(token string) (string, error) {
	if !validFileToken(token) {
		return "", &ServiceError{Code: CodeInvalidInput, Detail: "temporary file token is invalid"}
	}
	return filepath.Join(s.root, token+uploadSuffix), nil
}

func validFileToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, char := range token {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func randomHexToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func copyWithContextAndLimit(ctx context.Context, destination io.Writer, source io.Reader, maxBytes int64) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		remainingWithSentinel := maxBytes - written + 1
		readSize := int64(len(buffer))
		if remainingWithSentinel < readSize {
			readSize = remainingWithSentinel
		}
		read, readErr := source.Read(buffer[:readSize])
		if read > 0 {
			emptyReads = 0
			if written+int64(read) > maxBytes {
				return written, &ServiceError{Code: CodeFileTooLarge, Detail: "upload exceeds maximum size"}
			}
			if err := ctx.Err(); err != nil {
				return written, err
			}
			count, writeErr := destination.Write(buffer[:read])
			written += int64(count)
			if writeErr != nil {
				return written, storageFailure("temporary file cannot be written", writeErr)
			}
			if count != read {
				return written, storageFailure("temporary file write was incomplete", io.ErrShortWrite)
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return written, &ServiceError{Code: CodeInvalidInput, Detail: "upload stream made no progress", Err: io.ErrNoProgress}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, &ServiceError{Code: CodeInvalidInput, Detail: "upload stream cannot be read", Err: readErr}
		}
	}
}

func storageFailure(detail string, err error) *ServiceError {
	return &ServiceError{Code: CodeStorageUnavailable, Detail: detail, Err: err}
}

var _ FileStore = (*LocalFileStore)(nil)
