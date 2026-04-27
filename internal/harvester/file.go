package harvester

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

var editors = []string{"nvim", "vim", "code", "subl", "emacs", "hx", "zed", "micro", "nano"}

// CollectFile tries to find files open by a common editor inside dir.
// Best-effort; ok=false if lsof is missing or no matches were found.
func CollectFile(dir string) (string, bool) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	args := []string{"-Fn"}
	for _, ed := range editors {
		args = append(args, "-c", ed)
	}
	out, err := exec.CommandContext(ctx, "lsof", args...).Output()
	if err != nil {
		return "", false
	}
	var matches []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		path := line[1:]
		if strings.HasPrefix(path, dir) {
			matches = append(matches, path)
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return strings.Join(matches, "\n"), true
}
