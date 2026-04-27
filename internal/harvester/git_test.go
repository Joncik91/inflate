package harvester

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectGitNotARepo(t *testing.T) {
	dir := t.TempDir()
	got, ok := CollectGit(dir)
	if ok {
		t.Errorf("expected ok=false outside a repo, got %q", got)
	}
}

func TestCollectGitInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "initial")

	got, ok := CollectGit(dir)
	if !ok {
		t.Fatalf("expected ok=true in repo, got empty")
	}
	if !strings.Contains(got, "main") {
		t.Errorf("missing branch in output: %q", got)
	}
	if !strings.Contains(got, "initial") {
		t.Errorf("missing commit message: %q", got)
	}
}
