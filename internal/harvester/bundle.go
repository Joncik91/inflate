package harvester

import (
	"fmt"
	"strings"
)

// ContextBundle is the snapshot of harvested context published to the inflater.
type ContextBundle struct {
	Cwd       string `json:"cwd,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Git       string `json:"git,omitempty"`
	Shell     string `json:"shell,omitempty"`
	File      string `json:"file,omitempty"`
	JSONL     string `json:"jsonl,omitempty"`
	Processes string `json:"processes,omitempty"`

	// NeighborRepos lists immediate-child directory names of Cwd that
	// contain a .git entry. Populated only when Cwd itself isn't a repo,
	// so the TUI can surface "did you mean to run from one of: inflate,
	// Codexbar-fork, …?" Empty otherwise.
	NeighborRepos []string `json:"neighbor_repos,omitempty"`

	Redacted int `json:"redacted,omitempty"`

	ProfileOK   bool `json:"-"`
	GitOK       bool `json:"-"`
	ShellOK     bool `json:"-"`
	FileOK      bool `json:"-"`
	JSONLOK     bool `json:"-"`
	ProcessesOK bool `json:"-"`
}

// FlagsString renders the per-source flags for the TUI status line.
// Kept for tests / external callers that may already depend on the
// terse glyph format. The TUI itself now uses PresentSources /
// MissingSources / Severity for human-readable output.
func (b ContextBundle) FlagsString() string {
	mark := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}
	return fmt.Sprintf("profile%s git%s shell%s file%s jsonl%s",
		mark(b.ProfileOK), mark(b.GitOK), mark(b.ShellOK),
		mark(b.FileOK), mark(b.JSONLOK))
}

// labels is the technical→user-facing name map for context sources.
// Keep order identical to humanOrder so the rendered list reads
// predictably regardless of which sources happen to be present.
var humanLabels = []struct {
	name string
	ok   func(b ContextBundle) bool
}{
	{"profile", func(b ContextBundle) bool { return b.ProfileOK }},
	{"git", func(b ContextBundle) bool { return b.GitOK }},
	{"shell", func(b ContextBundle) bool { return b.ShellOK }},
	{"open editor file", func(b ContextBundle) bool { return b.FileOK }},
	{"Claude session", func(b ContextBundle) bool { return b.JSONLOK }},
	{"running tools", func(b ContextBundle) bool { return b.ProcessesOK }},
}

// PresentSources returns a comma-separated, human-readable list of
// context blocks successfully harvested. Empty when nothing was found.
func (b ContextBundle) PresentSources() string {
	var present []string
	for _, l := range humanLabels {
		if l.ok(b) {
			present = append(present, l.name)
		}
	}
	return strings.Join(present, ", ")
}

// MissingSources returns the comma-separated, human-readable list of
// blocks that failed or are unavailable. Empty when all are OK.
func (b ContextBundle) MissingSources() string {
	var missing []string
	for _, l := range humanLabels {
		if !l.ok(b) {
			missing = append(missing, l.name)
		}
	}
	return strings.Join(missing, ", ")
}

// Severity reflects bundle health for the TUI status line color:
//   "ok"   — git is present (we have project ground truth)
//   "warn" — git missing but at least 2 other sources present
//   "err"  — only profile, or nothing usable
func (b ContextBundle) Severity() string {
	if b.GitOK {
		return "ok"
	}
	count := 0
	for _, l := range humanLabels {
		if l.ok(b) {
			count++
		}
	}
	if count >= 2 {
		return "warn"
	}
	return "err"
}

// IsEmpty returns true if no source produced any content (profile included).
func (b ContextBundle) IsEmpty() bool {
	return !b.ProfileOK && !b.GitOK && !b.ShellOK && !b.FileOK && !b.JSONLOK
}
