package harvester

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CollectGit is the harvester hot path. Use DiagnoseGit when you need the
// underlying error (e.g. `inflate doctor`).
func CollectGit(dir string) (string, bool) {
	out, ok, _ := DiagnoseGit(dir)
	return out, ok
}

// DiagnoseGit returns context bundle text, ok flag, and the underlying error.
// On success, error is nil. On failure, ok is false and error explains why.
func DiagnoseGit(dir string) (string, bool, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false, fmt.Errorf("git not on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	branch, branchErr := gitOutErr(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branchErr != nil {
		return "", false, branchErr
	}
	log, _ := gitOutErr(ctx, dir, "log", "--oneline", "-3")
	stat, _ := gitOutErr(ctx, dir, "diff", "--stat")
	mods, _ := gitOutErr(ctx, dir, "diff", "--name-only")

	var sb strings.Builder
	fmt.Fprintf(&sb, "branch: %s\n", branch)
	if log != "" {
		fmt.Fprintf(&sb, "recent commits:\n%s\n", log)
	}
	if stat != "" {
		fmt.Fprintf(&sb, "diff stat:\n%s\n", stat)
	}
	if mods != "" {
		fmt.Fprintf(&sb, "modified:\n%s", mods)
	}
	return sb.String(), true, nil
}

// gitOutErr runs a git subcommand and returns stdout or an error that includes
// git's stderr (so messages like "dubious ownership" are visible to callers).
func gitOutErr(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
