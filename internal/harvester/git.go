package harvester

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CollectGit returns a short context block describing the repo at dir.
// ok=false means dir is not a git repo, git is missing, or commands timed out.
func CollectGit(dir string) (string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	branch, ok := gitOut(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if !ok {
		return "", false
	}
	log, _ := gitOut(ctx, dir, "log", "--oneline", "-3")
	stat, _ := gitOut(ctx, dir, "diff", "--stat")
	mods, _ := gitOut(ctx, dir, "diff", "--name-only")

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
	return sb.String(), true
}

func gitOut(ctx context.Context, dir string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
