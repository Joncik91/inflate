package harvester

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectShellNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", "")
	got, ok := CollectShell()
	if ok {
		t.Errorf("expected ok=false, got %q", got)
	}
}

func TestCollectShellFromFile(t *testing.T) {
	dir := t.TempDir()
	hist := filepath.Join(dir, ".bash_history")
	lines := ""
	for i := 1; i <= 25; i++ {
		lines += "cmd-" + string(rune('a'+i%26)) + "\n"
	}
	if err := os.WriteFile(hist, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", "")
	got, ok := CollectShell()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if strings.Count(got, "\n") > 20 {
		t.Errorf("expected at most 20 lines, got %d:\n%s", strings.Count(got, "\n"), got)
	}
}
