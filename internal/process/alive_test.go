//go:build linux || darwin

package process

import (
	"os"
	"testing"
)

func TestAliveCurrentProcess(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Errorf("current process should always be Alive")
	}
}

func TestAliveDeadHighPid(t *testing.T) {
	// PIDs far above the system max are unlikely to exist.
	if Alive(99999999) {
		t.Errorf("PID 99999999 should not be alive")
	}
}

func TestAliveZeroAndNegative(t *testing.T) {
	for _, pid := range []int{0, -1, -100} {
		if Alive(pid) {
			t.Errorf("Alive(%d) = true, want false", pid)
		}
	}
}

// TestAliveCrossUserEPERM verifies kill(0) returning EPERM is treated as
// alive. PID 1 (init) is always alive on linux/darwin and always owned by
// root, so when this test runs as a non-root user it exercises the EPERM
// path; running as root, it exercises the nil-error path. Either way the
// answer must be true.
func TestAliveCrossUserEPERM(t *testing.T) {
	if !Alive(1) {
		t.Errorf("PID 1 (init) must be reported alive — non-root callers hit EPERM, which must mean alive")
	}
}
