package harvester

import (
	"os"
	"path/filepath"
	"testing"
)

func makeRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteSinglePathInsideOneChildRepo(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "inflate")
	if err := os.MkdirAll(filepath.Join(repo, "internal", "tui"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeRepo(t, repo)

	fileBlock := "recently modified:\n" +
		filepath.Join(repo, "internal", "tui", "view.go") + "\n" +
		filepath.Join(repo, "internal", "tui", "model.go")

	got, ok := PromoteToRepoRoot(root, fileBlock)
	if !ok {
		t.Fatalf("expected promotion to succeed")
	}
	if got != repo {
		t.Errorf("promoted to %q, want %q", got, repo)
	}
}

func TestPromoteRefusesWhenAmbiguous(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "inflate")
	repoB := filepath.Join(root, "other")
	makeRepo(t, repoA)
	makeRepo(t, repoB)
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatal(err)
	}

	// Paths span both children — cluster is ambiguous, refuse to promote.
	fileBlock := "open in editor:\n" +
		filepath.Join(repoA, "main.go") + "\n" +
		filepath.Join(repoB, "main.go")

	if _, ok := PromoteToRepoRoot(root, fileBlock); ok {
		t.Errorf("ambiguous cluster (paths in two children) should NOT promote")
	}
}

func TestPromoteRefusesWhenChildHasNoGit(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "no-git-here")
	if err := os.MkdirAll(filepath.Join(child, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	fileBlock := filepath.Join(child, "src", "foo.go")
	if _, ok := PromoteToRepoRoot(root, fileBlock); ok {
		t.Errorf("child without .git should NOT be promoted")
	}
}

func TestPromoteRefusesWhenCwdItselfIsRepo(t *testing.T) {
	repo := t.TempDir()
	makeRepo(t, repo)

	fileBlock := filepath.Join(repo, "main.go")
	if _, ok := PromoteToRepoRoot(repo, fileBlock); ok {
		t.Errorf("when cwd is already a repo, no promotion should occur")
	}
}

func TestPromoteRefusesWhenNoPaths(t *testing.T) {
	root := t.TempDir()
	if _, ok := PromoteToRepoRoot(root, ""); ok {
		t.Errorf("empty file block should not promote")
	}
	if _, ok := PromoteToRepoRoot(root, "open in editor:\nlabel only"); ok {
		t.Errorf("file block with no absolute paths should not promote")
	}
}

func TestPromoteWalksUpToRepoRoot(t *testing.T) {
	// Cluster sits two levels deep inside the repo. Promotion should
	// walk up past the deeper common prefix to the actual repo root,
	// which is the immediate child of cwd.
	root := t.TempDir()
	repo := filepath.Join(root, "inflate")
	deep := filepath.Join(repo, "internal", "tui")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	makeRepo(t, repo)

	fileBlock := filepath.Join(deep, "view.go") + "\n" + filepath.Join(deep, "model.go")
	got, ok := PromoteToRepoRoot(root, fileBlock)
	if !ok {
		t.Fatal("expected promotion to succeed")
	}
	if got != repo {
		t.Errorf("walked to %q, want %q", got, repo)
	}
}

func TestExtractAbsolutePathsSkipsLabels(t *testing.T) {
	in := "open in editor:\n/path/one\n/path/two\nrecently modified (no editor detected):\n/path/three"
	got := extractAbsolutePaths(in)
	want := []string{"/path/one", "/path/two", "/path/three"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("paths[%d] = %q, want %q", i, got[i], w)
		}
	}
}
