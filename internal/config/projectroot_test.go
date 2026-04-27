package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCwdExplicit(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveCwd(dir)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Errorf("ResolveCwd(%q) = %q, want %q", dir, got, abs)
	}
}

func TestResolveCwdWalksToGitAncestor(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(repoRoot, "internal", "foo", "bar")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(deep); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCwd("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(repoRoot)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Errorf("ResolveCwd(\"\") = %q, want %q", gotResolved, want)
	}
}

func TestResolveCwdGitAsFile(t *testing.T) {
	// Worktrees / submodules use .git as a file pointing at the real gitdir.
	root := t.TempDir()
	repoRoot := filepath.Join(root, "wt")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(repoRoot, "x")
	os.MkdirAll(deep, 0o755)
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(deep)
	got, err := ResolveCwd("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(repoRoot)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Errorf("ResolveCwd(\"\") = %q, want %q", gotResolved, want)
	}
}

func TestResolveCwdNoGitFallsBackToPwd(t *testing.T) {
	root := t.TempDir() // no .git anywhere
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(deep)
	got, err := ResolveCwd("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(deep)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Errorf("ResolveCwd(\"\") = %q, want %q", gotResolved, want)
	}
}
