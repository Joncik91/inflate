package harvester

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CollectShell is the harvester hot path. Use DiagnoseShell when you need the
// underlying error (e.g. `inflate doctor`).
func CollectShell() (string, bool) {
	out, ok, _ := DiagnoseShell()
	return out, ok
}

// DiagnoseShell returns the last ~20 lines of the user's shell history file,
// ok flag, and the underlying error when no readable history is found.
func DiagnoseShell() (string, bool, error) {
	candidates := []string{}
	if h := os.Getenv("HISTFILE"); h != "" {
		candidates = append(candidates, h)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".bash_history"),
			filepath.Join(home, ".zsh_history"),
		)
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		lines := tailLines(f, 20)
		f.Close()
		lines = pruneStaleDirRefs(lines)
		if len(lines) == 0 {
			continue
		}
		return strings.Join(lines, "\n"), true, nil
	}
	return "", false, fmt.Errorf("no readable shell history in %v", candidates)
}

// pathRE captures both absolute paths (/foo/bar) and relative-style path
// fragments (foo/bar/baz) in shell history. The latter catches cases like
// `cd inflate-impl/internal/tui` where the regex for absolute paths would
// miss the leak. We require at least one separator to avoid matching
// every plain word, and intentionally have NO upper segment cap so deep
// paths (macOS /var/folders/tb/.../T/.../leaf) get checked at the leaf
// rather than at a still-existing prefix.
var pathRE = regexp.MustCompile(`(?:^|[\s'"])(/?[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+)`)

// pruneStaleDirRefs drops shell-history lines that reference any path which
// no longer exists on disk. Two reasons:
//   - Renamed/deleted project dirs (e.g. `cd /home/u/apps/old-name` or
//     `cd inflate-impl/internal/tui`) keep leaking into inflations even
//     after the rename, polluting context.
//   - The LLM treats them as ground truth despite the skeleton-rule that
//     <shell> is "low-signal background noise."
//
// Resolution for relative-style fragments: try as-is (relative to inflate's
// cwd), then under $HOME. If neither resolves, the line is dropped.
// Conservative: only drops when a stat() proves absence. Lines without
// any matched path fragment pass through unchanged.
func pruneStaleDirRefs(lines []string) []string {
	home, _ := os.UserHomeDir()
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		matches := pathRE.FindAllStringSubmatch(line, -1)
		stale := false
		for _, m := range matches {
			frag := strings.TrimRight(m[1], "/")
			if frag == "" {
				continue
			}
			if pathExists(frag) {
				continue
			}
			// Relative fragment: try resolving under $HOME before
			// declaring it stale, so common patterns like
			// `cd Documents/foo` work when run from $HOME.
			if !filepath.IsAbs(frag) && home != "" {
				if pathExists(filepath.Join(home, frag)) {
					continue
				}
			}
			stale = true
			break
		}
		if !stale {
			out = append(out, line)
		}
	}
	return out
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func tailLines(f *os.File, n int) []string {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	ring := make([]string, 0, n)
	for scanner.Scan() {
		line := scanner.Text()
		if len(ring) < n {
			ring = append(ring, line)
		} else {
			copy(ring, ring[1:])
			ring[n-1] = line
		}
	}
	return ring
}
