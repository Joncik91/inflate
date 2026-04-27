package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathForKnownTargets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cases := map[string]string{
		"":        "config.toml",
		"config":  "config.toml",
		"profile": "profile.toml",
		"env":     ".env",
	}
	for target, basename := range cases {
		got, err := pathFor(target)
		if err != nil {
			t.Errorf("pathFor(%q) err = %v", target, err)
			continue
		}
		want := filepath.Join(dir, "inflate", basename)
		if got != want {
			t.Errorf("pathFor(%q) = %q, want %q", target, got, want)
		}
	}
}

func TestPathForUnknownTarget(t *testing.T) {
	if _, err := pathFor("garbage"); err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestEnsureFileCreatesTemplate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "inflate", "config.toml")

	if err := ensureFile(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[provider]") {
		t.Errorf("template missing [provider] header: %q", string(data))
	}
}

func TestEnsureFilePreservesExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "inflate", ".env")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "MY_KEY=already_here\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureFile(path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != custom {
		t.Errorf("ensureFile overwrote existing content: got %q, want %q", string(got), custom)
	}
}
