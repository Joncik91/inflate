package harvester

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeSession(t *testing.T, dir, name string, s sessionFile) {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONL(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindActiveSessionPrefersBusyOverIdle(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	projectsDir := filepath.Join(root, "projects")
	projectDir := "/home/u/work/repo"
	hash := ProjectDirName(projectDir)
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	live := os.Getpid()
	writeSession(t, sessionsDir, "1.json", sessionFile{
		PID: live, SessionID: "idle-one", Cwd: projectDir, Status: "idle", UpdatedAt: 200,
	})
	writeSession(t, sessionsDir, "2.json", sessionFile{
		PID: live, SessionID: "busy-one", Cwd: projectDir, Status: "busy", UpdatedAt: 100,
	})
	writeJSONL(t, filepath.Join(projectsDir, hash), "idle-one.jsonl")
	writeJSONL(t, filepath.Join(projectsDir, hash), "busy-one.jsonl")

	got, ok, err := FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir)
	if !ok {
		t.Fatalf("expected ok, got err=%v", err)
	}
	want := filepath.Join(projectsDir, hash, "busy-one.jsonl")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindActiveSessionFiltersOtherCwd(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	projectsDir := filepath.Join(root, "projects")
	projectDir := "/home/u/work/repo"
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeSession(t, sessionsDir, "1.json", sessionFile{
		PID: os.Getpid(), SessionID: "wrong-cwd", Cwd: "/somewhere/else", Status: "busy", UpdatedAt: 999,
	})

	if _, ok, _ := FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir); ok {
		t.Errorf("should not match a session whose cwd is a different directory")
	}
}

func TestFindActiveSessionSkipsExited(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	projectsDir := filepath.Join(root, "projects")
	projectDir := "/home/u/work/repo"
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeSession(t, sessionsDir, "1.json", sessionFile{
		PID: os.Getpid(), SessionID: "ghost", Cwd: projectDir, Status: "exited", UpdatedAt: 999,
	})

	if _, ok, _ := FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir); ok {
		t.Errorf("exited session should not be selected")
	}
}

func TestFindActiveSessionSkipsDeadPid(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	projectsDir := filepath.Join(root, "projects")
	projectDir := "/home/u/work/repo"
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// PID 1 is init / always alive on linux; pick a high one likely dead.
	writeSession(t, sessionsDir, "1.json", sessionFile{
		PID: 999999, SessionID: "ghost", Cwd: projectDir, Status: "busy", UpdatedAt: 999,
	})

	if _, ok, _ := FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir); ok {
		t.Errorf("dead PID should be skipped")
	}
}

func TestFindActiveSessionMissingJSONL(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	projectsDir := filepath.Join(root, "projects")
	projectDir := "/home/u/work/repo"
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeSession(t, sessionsDir, "1.json", sessionFile{
		PID: os.Getpid(), SessionID: "no-file", Cwd: projectDir, Status: "busy", UpdatedAt: 999,
	})

	if _, ok, _ := FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir); ok {
		t.Errorf("session whose jsonl file is missing should not be selected")
	}
}
