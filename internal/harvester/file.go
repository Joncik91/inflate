package harvester

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var editors = []string{"nvim", "vim", "code", "subl", "emacs", "hx", "zed", "micro", "nano"}

// CollectFile is the harvester hot path. Use DiagnoseFile when you need the
// underlying error (e.g. `inflate doctor`).
func CollectFile(dir string) (string, bool) {
	out, ok, _ := DiagnoseFile(dir)
	return out, ok
}

// DiagnoseFile tries to find files open by a common editor inside dir.
// Best-effort; ok=false and a non-nil error if lsof is missing, no editor
// is open, or no matching files are found inside dir.
func DiagnoseFile(dir string) (string, bool, error) {
	if _, err := exec.LookPath("lsof"); err != nil {
		return "", false, fmt.Errorf("lsof not installed (sudo apt install lsof)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	args := []string{"-Fn"}
	for _, ed := range editors {
		args = append(args, "-c", ed)
	}
	out, err := exec.CommandContext(ctx, "lsof", args...).Output()
	if err != nil {
		// lsof exits 1 when it finds no processes — not really an error.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", false, fmt.Errorf("no supported editor (%s) currently open", strings.Join(editors, ", "))
		}
		return "", false, fmt.Errorf("lsof: %w", err)
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
		return "", false, fmt.Errorf("no matching open files inside %s", dir)
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return strings.Join(matches, "\n"), true, nil
}
