package config

import (
	"os"
	"path/filepath"
	"sort"
)

// maxAncestorWalk caps the upward walk so a malformed path can't loop forever.
const maxAncestorWalk = 32

// maxNeighborScan caps how many immediate children to inspect when looking
// for sibling git repos.
const maxNeighborScan = 50

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

// NeighborRepos returns the names of immediate-child directories under
// dir that contain a .git entry. Used by the TUI to surface a hint
// when inflate is launched from a parent dir that has multiple repos
// underneath it (e.g. /home/u/apps containing inflate/, foo/, bar/).
//
// We DON'T auto-descend because a parent dir with N child repos is
// ambiguous — picking one would be wrong. Surfacing the list lets the
// user decide.
//
// Returns nil + nil error when there are no neighbor repos or when
// the dir can't be read. The caller should treat nil as "no hint."
func NeighborRepos(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxNeighborScan {
		entries = entries[:maxNeighborScan]
	}
	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Hidden dirs (.cache, .config) are unlikely to be project repos.
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			continue
		}
		gitPath := filepath.Join(dir, e.Name(), ".git")
		if _, err := os.Stat(gitPath); err == nil {
			repos = append(repos, e.Name())
		}
	}
	sort.Strings(repos)
	return repos, nil
}
