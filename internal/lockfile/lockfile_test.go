package lockfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.lock")

	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	// second acquire should fail
	if _, err := Acquire(path); err == nil {
		t.Error("expected second Acquire to fail")
	}
}

func TestAcquireAfterStaleLockfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.lock")
	// write a lockfile pointing at a non-existent PID
	if err := os.WriteFile(path, []byte("99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("expected stale-lock takeover to succeed, got %v", err)
	}
	l.Release()
}

func TestAcquireStealsLockHeldByForeignProcess(t *testing.T) {
	// PID 1 (init) is always alive on POSIX systems but is NOT inflate.
	// We expect Acquire to steal the lock rather than block.
	if runtime.GOOS == "windows" {
		t.Skip("PID 1 doesn't have the same semantics on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "run.lock")
	if err := os.WriteFile(path, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("expected steal of foreign-process lock, got %v", err)
	}
	l.Release()
}
