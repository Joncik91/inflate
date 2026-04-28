package harvester

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnoseShellNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", "")
	_, ok, err := DiagnoseShell()
	if ok {
		t.Errorf("expected ok=false when no history")
	}
	if err == nil {
		t.Errorf("expected non-nil err when no history readable")
	}
}

func TestCollectShellNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", "")
	got, ok := CollectShell()
	if ok {
		t.Errorf("expected ok=false, got %q", got)
	}
}

func TestPruneStaleDirRefs(t *testing.T) {
	live := t.TempDir() // exists
	dead := filepath.Join(live, "definitely-not-here")
	lines := []string{
		"echo hello",                  // no path
		"cd " + live,                  // live path — keep
		"cd " + dead,                  // stale path — drop
		"ls /tmp /var",                // multiple paths, all live
		"cat " + dead + "/file.txt",   // stale — drop
		"git status",                  // no path
	}
	got := pruneStaleDirRefs(lines)
	wantKept := []string{
		"echo hello",
		"cd " + live,
		"ls /tmp /var",
		"git status",
	}
	if len(got) != len(wantKept) {
		t.Fatalf("kept %d lines, want %d:\nkept=%v", len(got), len(wantKept), got)
	}
	for i, w := range wantKept {
		if got[i] != w {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
}

func TestCollectShellFromFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME env var doesn't redirect os.UserHomeDir on Windows; collector behavior is correct (no bash/zsh history) but the test setup can't be exercised")
	}
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
