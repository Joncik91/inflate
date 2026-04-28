package harvester

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// osGetenv is a thin wrapper that returns def when the env var is unset.
// Inlined as a tiny helper because callers always want this fallback.
func osGetenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// devTools is the allow-list of process names worth surfacing as "what the
// user is currently doing." Anything else (sshd, browsers, system services)
// is noise. Names are matched against `comm` — the kernel-truncated process
// name — so they're short and exact.
var devTools = map[string]string{
	"claude":    "Claude Code",
	"code":      "VS Code",
	"cursor":    "Cursor",
	"nvim":      "Neovim",
	"vim":       "Vim",
	"hx":        "Helix",
	"zed":       "Zed",
	"go":        "Go toolchain",
	"cargo":     "Cargo",
	"rustc":     "rustc",
	"node":      "Node",
	"python":    "Python",
	"python3":   "Python",
	"pytest":    "pytest",
	"jest":      "Jest",
	"npm":       "npm",
	"pnpm":      "pnpm",
	"yarn":      "yarn",
	"deno":      "Deno",
	"bun":       "Bun",
	"docker":    "Docker",
	"kubectl":   "kubectl",
	"terraform": "Terraform",
	"git":       "git",
	"make":      "make",
	"ruff":      "ruff",
	"mypy":      "mypy",
	"tsc":       "TypeScript",
	"vite":      "Vite",
	"webpack":   "Webpack",
	"ollama":    "Ollama",
}

// CollectProcesses returns a compact human-readable list of dev tools
// currently running in the user's session. Empty + ok=false when none
// of the allow-listed tools match. Used as low-signal hint for "what
// is the user actively working on right now."
func CollectProcesses() (string, bool) {
	out, ok, _ := DiagnoseProcesses()
	return out, ok
}

// DiagnoseProcesses runs `ps` and filters to the devTools allow-list.
// Times out aggressively (300 ms) — this is the harvest hot path.
func DiagnoseProcesses() (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "comm=", "-u", currentUsername()).Output()
	if err != nil {
		return "", false, fmt.Errorf("ps: %w", err)
	}
	seen := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		// Strip trailing version digits some toolchains append (e.g. python3.13)
		name = trimDigitTail(name)
		if label, ok := devTools[name]; ok {
			seen[label]++
		}
	}
	if len(seen) == 0 {
		return "", false, fmt.Errorf("no dev tools running")
	}
	type entry struct {
		label string
		count int
	}
	entries := make([]entry, 0, len(seen))
	for k, v := range seen {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].label < entries[j].label
	})
	parts := make([]string, len(entries))
	for i, e := range entries {
		if e.count > 1 {
			parts[i] = fmt.Sprintf("%s (×%d)", e.label, e.count)
		} else {
			parts[i] = e.label
		}
	}
	return "running tools: " + strings.Join(parts, ", "), true, nil
}

// trimDigitTail strips trailing version digits + dots (python3.13 -> python3).
// Bounded; only consumes the *last* run of digits/dots so "go" stays "go".
func trimDigitTail(s string) string {
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if (c >= '0' && c <= '9') || c == '.' {
			end--
			continue
		}
		break
	}
	if end == 0 {
		return s
	}
	return s[:end]
}

// currentUsername returns $USER or the result of `id -un`. Falls back to
// "root" because we always want SOMETHING for ps; if neither resolves
// we'd rather show all-user processes than crash.
func currentUsername() string {
	if u := strings.TrimSpace(osGetenv("USER", "")); u != "" {
		return u
	}
	if u := strings.TrimSpace(osGetenv("LOGNAME", "")); u != "" {
		return u
	}
	out, err := exec.Command("id", "-un").Output()
	if err == nil {
		if u := strings.TrimSpace(string(out)); u != "" {
			return u
		}
	}
	return "root"
}
