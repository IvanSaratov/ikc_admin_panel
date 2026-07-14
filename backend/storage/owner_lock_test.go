package storage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAcquireOwnerLockSameProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "owner.db")

	first, err := AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() first owner error = %v", err)
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

	third, err := AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() after Close error = %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("third owner Close() error = %v", err)
	}
}

func TestAcquireOwnerLockCrossProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "owner.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAcquireOwnerLockHelperProcess$")
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
	if err := <-done; err != nil {
		t.Fatalf("helper exit: %v", err)
	}
	helperFinished = true

	owner, err = AcquireOwnerLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireOwnerLock() after helper exit error = %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("owner Close() error = %v", err)
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
