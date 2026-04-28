//go:build windows

package process

import "golang.org/x/sys/windows"

// Alive returns true if pid identifies a running process on this host.
func Alive(pid int) bool {
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
