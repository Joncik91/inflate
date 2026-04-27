//go:build windows

package lockfile

import "golang.org/x/sys/windows"

// processAlive returns true if pid identifies a running process.
// On Windows, Signal(0) is not supported; instead open the process with
// PROCESS_QUERY_LIMITED_INFORMATION which succeeds for any live process
// accessible to the caller without requiring special privileges.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const access = windows.PROCESS_QUERY_LIMITED_INFORMATION
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	return true
}
