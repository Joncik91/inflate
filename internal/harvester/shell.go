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

// absPathRE captures absolute Unix paths in shell history. We only inspect
// the *first* component beyond the root so cheap stat() calls are enough
// to decide if the line references a directory that no longer exists.
var absPathRE = regexp.MustCompile(`(?:^|[\s'"])(/(?:[A-Za-z0-9._-]+/?){1,4})`)

// pruneStaleDirRefs drops shell-history lines that reference an absolute
// path which no longer exists on disk. Two reasons:
//   - Renamed/deleted project dirs (e.g. `cd /home/u/apps/old-name`) keep
//     leaking into inflations even after the rename, polluting context.
//   - The LLM treats them as ground truth despite the skeleton-rule that
//     <shell> is "low-signal background noise."
//
// Conservative: only drops when a stat() proves absence. Lines without
// absolute paths pass through unchanged.
func pruneStaleDirRefs(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		matches := absPathRE.FindAllStringSubmatch(line, -1)
		stale := false
		for _, m := range matches {
			path := m[1]
			path = strings.TrimRight(path, "/")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				stale = true
				break
			}
		}
		if !stale {
			out = append(out, line)
		}
	}
	return out
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
