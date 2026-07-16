package imports_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/imports"
)

func TestLocalFileStoreCreatesPrivateDirectoryAndFile(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "private", "uploads")
	store, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	directoryInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := directoryInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode = %04o, want 0700", got)
	}

	stored, err := store.Save(context.Background(), strings.NewReader("synthetic upload"), 1024)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(stored.Token) {
		t.Fatalf("token has unsafe format")
	}
	if filepath.Base(stored.Path) != stored.Token+".upload" {
		t.Fatalf("stored basename does not use opaque token")
	}
	fileInfo, err := os.Lstat(stored.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want regular 0600", fileInfo.Mode())
	}
}

func TestLocalFileStoreEnforcesPrivateModeOnExistingRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "uploads")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.NewLocalFileStore(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("existing root mode = %04o", info.Mode().Perm())
	}
}

func TestLocalFileStoreComputesSHAAndSizeWhileStreaming(t *testing.T) {
	t.Parallel()

	store, err := imports.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("streamed synthetic content")
	stored, err := store.Save(context.Background(), bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	if stored.Size != int64(len(payload)) || stored.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("stored metadata = %+v", stored)
	}
	readBack, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readBack, payload) {
		t.Fatal("stored payload differs")
	}
}

func TestLocalFileStoreRejectsBodyOverLimitAndRemovesPartialFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Save(context.Background(), strings.NewReader("123456"), 5)
	assertImportErrorCode(t, err, imports.CodeFileTooLarge)
	assertDirectoryEmpty(t, root)
}

func TestLocalFileStoreRemovesPartialFileWhenReaderFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	readerErr := errors.New("synthetic reader failure")
	_, err = store.Save(context.Background(), &errorAfterReader{data: []byte("partial"), err: readerErr}, 1024)
	if !errors.Is(err, readerErr) {
		t.Fatalf("save error = %v, want reader cause", err)
	}
	assertDirectoryEmpty(t, root)
}

func TestLocalFileStoreHonorsCancelledContextAndRemovesPartialFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Save(ctx, strings.NewReader("not written"), 1024)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("save error = %v, want context.Canceled", err)
	}
	assertDirectoryEmpty(t, root)
}

func TestLocalFileStorePathRejectsTraversalAndMalformedToken(t *testing.T) {
	t.Parallel()

	store, err := imports.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"../escape", strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("g", 64)} {
		if _, err := store.Path(token); err == nil {
			t.Errorf("Path accepted malformed token")
		} else {
			assertImportErrorCode(t, err, imports.CodeInvalidInput)
		}
	}
}

func TestLocalFileStorePathRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := imports.NewLocalFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, token+".upload")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Path(token); err == nil {
		t.Fatal("Path accepted symlink")
	} else {
		assertImportErrorCode(t, err, imports.CodeStorageUnavailable)
	}
}

func TestLocalFileStoreDeleteIsIdempotent(t *testing.T) {
	t.Parallel()

	store, err := imports.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Save(context.Background(), strings.NewReader("delete me"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(stored.Token); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(stored.Token); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestNewLocalFileStoreRejectsSymlinkRoot(t *testing.T) {
	t.Parallel()

	realRoot := filepath.Join(t.TempDir(), "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkRoot := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.NewLocalFileStore(symlinkRoot); err == nil {
		t.Fatal("NewLocalFileStore accepted a symlink root")
	}
}

type errorAfterReader struct {
	data []byte
	err  error
	done bool
}

func (r *errorAfterReader) Read(buffer []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(buffer, r.data), nil
	}
	return 0, r.err
}

func assertImportErrorCode(t *testing.T, err error, want imports.ErrorCode) *imports.ServiceError {
	t.Helper()
	var serviceErr *imports.ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("error = %v, want *imports.ServiceError", err)
	}
	if serviceErr.Code != want {
		t.Fatalf("error code = %q, want %q", serviceErr.Code, want)
	}
	return serviceErr
}

func assertDirectoryEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("temporary directory retained files: %v", names)
	}
}

var _ io.Reader = (*errorAfterReader)(nil)
