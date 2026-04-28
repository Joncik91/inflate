package harvester

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var editors = []string{"nvim", "vim", "code", "subl", "emacs", "hx", "zed", "micro", "nano"}

// CollectFile is the harvester hot path. Use DiagnoseFile when you need the
// underlying error (e.g. `inflate doctor`).
func CollectFile(dir string) (string, bool) {
	out, ok, _ := DiagnoseFile(dir)
	return out, ok
}

// DiagnoseFile tries to find files open by a common editor inside dir,
// falling back to recently-modified files if no editor is detected.
// Either signal is useful as <file> context; both fail only when nothing
// inside dir has been touched recently AND no editor has it open.
//
// Labels the output so the LLM can distinguish "open right now" (lsof
// match) from "edited in the last 30 min" (mtime fallback). The two
// signals are different: an open file is what the user is *currently*
// looking at; a recently-modified file is what they touched recently.
func DiagnoseFile(dir string) (string, bool, error) {
	// Path 1: lsof against known editor process names.
	if _, err := exec.LookPath("lsof"); err == nil {
		matches, err := lsofMatches(dir)
		if err == nil && len(matches) > 0 {
			return "open in editor:\n" + strings.Join(matches, "\n"), true, nil
		}
	}
	// Path 2: fallback — recently-modified files. Distinct label.
	recent, err := recentFilesIn(dir, 30*time.Minute, 5)
	if err != nil {
		return "", false, err
	}
	if len(recent) == 0 {
		return "", false, fmt.Errorf("no supported editor (%s) currently open AND no recently-modified files inside %s", strings.Join(editors, ", "), dir)
	}
	return "recently modified (no editor detected):\n" + strings.Join(recent, "\n"), true, nil
}

// lsofMatches returns paths reported by lsof for any editor process whose
// path starts with dir. Returns nil + error when lsof itself fails or
// finds nothing matching.
func lsofMatches(dir string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	args := []string{"-Fn"}
	for _, ed := range editors {
		args = append(args, "-c", ed)
	}
	out, err := exec.CommandContext(ctx, "lsof", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, fmt.Errorf("no supported editor open")
		}
		return nil, err
	}
	var matches []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		path := line[1:]
		if strings.HasPrefix(path, dir) {
			matches = append(matches, path)
		}
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches, nil
}

// recentFilesIn walks dir (one level deep, then two levels into well-known
// source subdirs) and returns up to n files modified within window. Skips
// .git, node_modules, and similar noise. The walk is intentionally bounded
// — this is harvest-time hot path.
func recentFilesIn(dir string, window time.Duration, n int) ([]string, error) {
	cutoff := time.Now().Add(-window)
	type ent struct {
		path  string
		mtime time.Time
	}
	var found []ent
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		".idea": true, ".vscode": true, "target": true, "dist": true, "build": true,
	}
	maxDepth := 4
	rootDepth := strings.Count(filepath.Clean(dir), string(os.PathSeparator))
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth
		if depth > maxDepth {
			return filepath.SkipDir
		}
		base := filepath.Base(path)
		if info.IsDir() {
			if skipDirs[base] || strings.HasPrefix(base, ".") && depth > 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(base, ".") {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}
		found = append(found, ent{path, info.ModTime()})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mtime.After(found[j].mtime) })
	if len(found) > n {
		found = found[:n]
	}
	out := make([]string, len(found))
	for i, e := range found {
		out[i] = e.path
	}
	return out, nil
}
