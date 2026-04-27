//go:build !windows

package lockfile

import (
	"os"
	"syscall"
)

// processAlive returns true if pid identifies a running process.
// On POSIX, signal 0 is the standard "is the process there?" probe.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
