package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAcquireOwnerLockSameProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "owner.db")

	first, err := AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() first owner error = %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dbPath + ".lock")
		if err != nil {
			t.Fatalf("stat owner lock file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("owner lock file mode = %o, want 600", got)
		}
	}

	second, err := AcquireOwnerLock(dbPath)
	if second != nil {
		_ = second.Close()
		t.Fatal("AcquireOwnerLock() second owner returned a lock, want nil")
	}
	if !errors.Is(err, ErrDatabaseInUse) {
		_ = first.Close()
		t.Fatalf("AcquireOwnerLock() second owner error = %v, want ErrDatabaseInUse", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first owner Close() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first owner second Close() error = %v", err)
	}
	if _, err := os.Stat(dbPath + ".lock"); err != nil {
		t.Fatalf("owner lock file after Close: %v", err)
	}

	third, err := AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() after Close error = %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("third owner Close() error = %v", err)
	}
}

func TestAcquireOwnerLockContendsAcrossFinalSymlink(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.db")
	if err := os.WriteFile(realPath, nil, 0o600); err != nil {
		t.Fatalf("create database file: %v", err)
	}
	aliasPath := filepath.Join(root, "alias.db")
	createSymlinkOrSkip(t, realPath, aliasPath)

	owner, err := AcquireOwnerLock(realPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock(real path) error = %v", err)
	}
	defer owner.Close()

	aliasOwner, err := AcquireOwnerLock(aliasPath)
	if aliasOwner != nil {
		_ = aliasOwner.Close()
		t.Fatal("AcquireOwnerLock(symlink) returned a lock, want nil")
	}
	if !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("AcquireOwnerLock(symlink) error = %v, want ErrDatabaseInUse", err)
	}
}

func TestAcquireOwnerLockRejectsDanglingFinalSymlink(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "missing-target.db")
	aliasPath := filepath.Join(root, "alias.db")
	createSymlinkOrSkip(t, targetPath, aliasPath)

	owner, err := AcquireOwnerLock(aliasPath)
	if owner != nil {
		_ = owner.Close()
		t.Fatal("AcquireOwnerLock(dangling symlink) returned a lock, want nil")
	}
	if err == nil {
		t.Fatal("AcquireOwnerLock(dangling symlink) error = nil, want rejection")
	}
	if errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("AcquireOwnerLock(dangling symlink) error = %v, want non-contention error", err)
	}
	if !strings.Contains(err.Error(), "dangling database symlink") {
		t.Fatalf("AcquireOwnerLock(dangling symlink) error = %v, want clear dangling symlink error", err)
	}
	for _, lockPath := range []string{aliasPath + ".lock", targetPath + ".lock"} {
		if _, statErr := os.Stat(lockPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("lock file %q stat error = %v, want not exist", lockPath, statErr)
		}
	}

	targetOwner, err := AcquireOwnerLock(targetPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock(target path) error = %v", err)
	}
	if err := targetOwner.Close(); err != nil {
		t.Fatalf("target owner Close() error = %v", err)
	}
}

func TestAcquireOwnerLockContendsAcrossSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatalf("create database parent: %v", err)
	}
	aliasParent := filepath.Join(root, "alias")
	createSymlinkOrSkip(t, realParent, aliasParent)

	realPath := filepath.Join(realParent, "fresh.db")
	aliasPath := filepath.Join(aliasParent, "fresh.db")
	if _, err := os.Stat(realPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database file stat error = %v, want not exist", err)
	}

	owner, err := AcquireOwnerLock(realPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock(real parent) error = %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })

	aliasOwner, err := AcquireOwnerLock(aliasPath)
	if aliasOwner != nil {
		_ = aliasOwner.Close()
		t.Fatal("AcquireOwnerLock(symlinked parent) returned a lock, want nil")
	}
	if !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("AcquireOwnerLock(symlinked parent) error = %v, want ErrDatabaseInUse", err)
	}

	if err := owner.Close(); err != nil {
		t.Fatalf("real parent owner Close() error = %v", err)
	}
	aliasOwner, err = AcquireOwnerLock(aliasPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock(symlinked parent) after Close error = %v", err)
	}
	defer aliasOwner.Close()
	canonicalParent, err := filepath.EvalSymlinks(realParent)
	if err != nil {
		t.Fatalf("canonicalize real parent: %v", err)
	}
	if got, want := aliasOwner.file.Path(), filepath.Join(canonicalParent, "fresh.db.lock"); got != want {
		t.Fatalf("symlinked parent lock path = %q, want canonical path %q", got, want)
	}
}

func TestAcquireOwnerLockCrossProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "owner.db")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcquireOwnerLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"GO_WANT_OWNER_LOCK_HELPER=1",
		"OWNER_LOCK_DB_PATH="+dbPath,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("helper stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("helper stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("helper stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	helperFinished := false
	t.Cleanup(func() {
		if helperFinished {
			return
		}
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		<-done
	})

	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read helper readiness: %v", err)
	}
	if ready != "ready\n" {
		t.Fatalf("helper readiness = %q, want %q", ready, "ready\n")
	}

	owner, err := AcquireOwnerLock(dbPath)
	if owner != nil {
		_ = owner.Close()
		t.Fatal("AcquireOwnerLock() while helper owns database returned a lock, want nil")
	}
	if !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("AcquireOwnerLock() while helper owns database error = %v, want ErrDatabaseInUse", err)
	}

	if _, err := fmt.Fprintln(stdin); err != nil {
		t.Fatalf("release helper: %v", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatalf("close helper stdin: %v", err)
	}
	waitErr := <-done
	helperFinished = true
	if waitErr != nil {
		t.Fatalf("helper exit: %v", waitErr)
	}

	owner, err = AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() after helper exit error = %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("owner Close() error = %v", err)
	}
}

func createSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" || errors.Is(err, os.ErrPermission) {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}
}

func TestAcquireOwnerLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_OWNER_LOCK_HELPER") != "1" {
		return
	}

	owner, err := AcquireOwnerLock(os.Getenv("OWNER_LOCK_DB_PATH"))
	if err != nil {
		t.Fatalf("AcquireOwnerLock() helper error = %v", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
		_ = owner.Close()
		t.Fatalf("write readiness: %v", err)
	}
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		_ = owner.Close()
		t.Fatalf("wait for release: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("owner Close() error = %v", err)
	}
}
