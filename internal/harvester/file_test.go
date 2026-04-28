package harvester

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnoseFileNoLSOFNoRecent(t *testing.T) {
	// Empty temp dir + lsof unavailable → both paths exhausted, ok=false.
	t.Setenv("PATH", "")
	dir := t.TempDir()
	_, ok, err := DiagnoseFile(dir)
	if ok {
		t.Errorf("expected ok=false when lsof missing AND no recent files")
	}
	if err == nil {
		t.Errorf("expected non-nil err when no signal at all")
	}
}

func TestDiagnoseFileFallsBackToRecent(t *testing.T) {
	// lsof unavailable, but a recently-modified file exists in dir.
	// Fallback should kick in and return that file.
	t.Setenv("PATH", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "recent.go")
	if err := os.WriteFile(target, []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := DiagnoseFile(dir)
	if !ok {
		t.Fatalf("expected ok=true when recent file exists, err=%v", err)
	}
	if got == "" {
		t.Errorf("expected the recent file path, got empty")
	}
}

func TestRecentFilesInWindowFiltering(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.go")
	new := filepath.Join(dir, "new.go")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(new, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backdate `old` to outside the window.
	pastTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}

	got, err := recentFilesIn(dir, 30*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 recent file (new), got %d: %v", len(got), got)
	}
	if filepath.Base(got[0]) != "new.go" {
		t.Errorf("expected new.go, got %q", got[0])
	}
}

func TestEditorList(t *testing.T) {
	if len(editors) == 0 {
		t.Error("editors list is empty")
	}
}
