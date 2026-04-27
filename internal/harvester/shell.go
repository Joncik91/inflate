package harvester

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// CollectShell returns the last ~20 lines of the user's shell history file.
// ok=false means no history file is readable.
func CollectShell() (string, bool) {
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
		if len(lines) == 0 {
			continue
		}
		return strings.Join(lines, "\n"), true
	}
	return "", false
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
