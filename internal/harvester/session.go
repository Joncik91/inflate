package harvester

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Joncik91/inflate/internal/process"
)

// sessionFile mirrors the schema written by Claude Code under
// ~/.claude/sessions/<pid>.json. Each interactive Claude Code process
// keeps its row updated in place; on exit, status flips to "exited".
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Status    string `json:"status"` // "busy" | "idle" | "exited"
	UpdatedAt int64  `json:"updatedAt"`
}

// FindActiveSessionJSONL picks the JSONL file for the most relevant active
// Claude Code session whose cwd matches the inflate project dir.
//
// Selection: only sessions with cwd == projectDir, status != "exited",
// and the PID still alive on this host. Prefer status=="busy" over "idle";
// then most recent updatedAt.
//
// Returns ok=false (with an explanatory error) when no live session for
// this project exists. Callers should fall back to newest-jsonl-in-dir
// behavior so the harvester still surfaces the last conversation when
// Claude Code is not currently running.
func FindActiveSessionJSONL(projectDir, sessionsDir, projectsDir string) (string, bool, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, fmt.Errorf("sessions dir %s does not exist", sessionsDir)
		}
		return "", false, fmt.Errorf("read sessions dir %s: %w", sessionsDir, err)
	}

	want, err := filepath.Abs(projectDir)
	if err != nil {
		want = projectDir
	}

	var matches []sessionFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		var s sessionFile
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if s.Status == "exited" {
			continue
		}
		if s.SessionID == "" {
			continue
		}
		abs, err := filepath.Abs(s.Cwd)
		if err != nil {
			abs = s.Cwd
		}
		if abs != want {
			continue
		}
		if !process.Alive(s.PID) {
			continue
		}
		matches = append(matches, s)
	}

	if len(matches) == 0 {
		return "", false, fmt.Errorf("no active Claude Code session for cwd %s", want)
	}

	sort.Slice(matches, func(i, j int) bool {
		bi := matches[i].Status == "busy"
		bj := matches[j].Status == "busy"
		if bi != bj {
			return bi // busy first
		}
		return matches[i].UpdatedAt > matches[j].UpdatedAt
	})

	jsonl := filepath.Join(projectsDir, ProjectDirName(want), matches[0].SessionID+".jsonl")
	if _, err := os.Stat(jsonl); err != nil {
		return "", false, fmt.Errorf("session %s file missing at %s", matches[0].SessionID, jsonl)
	}
	return jsonl, true, nil
}
