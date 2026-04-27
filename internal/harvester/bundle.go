package harvester

import "fmt"

// ContextBundle is the snapshot of harvested context published to the inflater.
type ContextBundle struct {
	Cwd     string `json:"cwd,omitempty"`
	Profile string `json:"profile,omitempty"`
	Git     string `json:"git,omitempty"`
	Shell   string `json:"shell,omitempty"`
	File    string `json:"file,omitempty"`
	JSONL   string `json:"jsonl,omitempty"`

	Redacted int `json:"redacted,omitempty"`

	ProfileOK bool `json:"-"`
	GitOK     bool `json:"-"`
	ShellOK   bool `json:"-"`
	FileOK    bool `json:"-"`
	JSONLOK   bool `json:"-"`
}

// FlagsString renders the per-source flags for the TUI status line.
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

// IsEmpty returns true if no source produced any content (profile included).
func (b ContextBundle) IsEmpty() bool {
	return !b.ProfileOK && !b.GitOK && !b.ShellOK && !b.FileOK && !b.JSONLOK
}
