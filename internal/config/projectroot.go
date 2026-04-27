package config

import (
	"os"
	"path/filepath"
)

// maxAncestorWalk caps the upward walk so a malformed path can't loop forever.
const maxAncestorWalk = 32

// ResolveCwd returns the project root for inflate's harvester.
// If explicit is non-empty, returns its absolute form.
// Otherwise: walks from $PWD upward looking for a directory containing
// a .git entry (file or directory; .git can be a file in worktrees and
// submodules). If found, returns that ancestor. Otherwise returns $PWD.
func ResolveCwd(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(pwd)
	if err != nil {
		return "", err
	}
	for i := 0; i < maxAncestorWalk; i++ {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break // hit filesystem root
		}
		abs = parent
	}
	// no .git found — fall back to original $PWD
	return filepath.Abs(pwd)
}
