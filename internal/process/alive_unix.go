//go:build linux || darwin

package process

import (
	"errors"
	"os"
	"syscall"
)

// Alive returns true if pid identifies a running process on this host.
//
// Implementation note: kill(pid, 0) is the POSIX standard. It returns:
//   - nil       => process exists and we may signal it
//   - EPERM     => process exists but we lack permission (e.g. it's owned
//                  by another user). We must treat this as alive — denying
//                  it would cause inflate, running as a normal user, to
//                  miss Claude Code sessions started as root on the same
//                  machine.
//   - ESRCH     => process does not exist (truly dead)
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
