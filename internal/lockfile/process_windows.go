//go:build windows

package lockfile

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// processAlive returns true if pid identifies a running process.
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

// processIsInflate checks whether the process image is inflate.exe.
// Fail-closed: any error returns false.
//
// Special case: if pid is the current process we always return true.
// This preserves the double-launch guard when a live inflate calls Acquire
// a second time.
func processIsInflate(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	const access = windows.PROCESS_QUERY_LIMITED_INFORMATION
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var pathBuf [windows.MAX_PATH]uint16
	size := uint32(len(pathBuf))
	if err := windows.QueryFullProcessImageName(h, 0, &pathBuf[0], &size); err != nil {
		return false
	}
	imagePath := windows.UTF16ToString(pathBuf[:size])
	base := filepath.Base(imagePath)
	return strings.EqualFold(base, "inflate.exe")
}
