// Package lockfile provides a single-instance lock via PID file.
package lockfile

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Lock represents an acquired single-instance lock.
type Lock struct{ path string }

// Acquire creates the lockfile atomically. If a lockfile already exists, it
// reads the PID, checks whether that process is alive, and steals the lock
// if not.
func Acquire(path string) (*Lock, error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return &Lock{path: path}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		// existing lock — check liveness
		data, _ := os.ReadFile(path)
		pidStr := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
		pid, perr := strconv.Atoi(pidStr)
		if perr != nil || !processAlive(pid) {
			_ = os.Remove(path)
			continue
		}
		return nil, fmt.Errorf("already running (PID %d); use --force to override", pid)
	}
	return nil, errors.New("lockfile race lost twice")
}

// Release removes the lockfile.
func (l *Lock) Release() { _ = os.Remove(l.path) }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 = "is the process there?"
	return p.Signal(syscall.Signal(0)) == nil
}
