package harvester

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBundleFlagsString(t *testing.T) {
	b := ContextBundle{
		ProfileOK: true,
		GitOK:     true,
		ShellOK:   false,
		FileOK:    true,
		JSONLOK:   false,
	}
	got := b.FlagsString()
	want := "profile✓ git✓ shell✗ file✓ jsonl✗"
	if got != want {
		t.Errorf("FlagsString() = %q, want %q", got, want)
	}
}

func TestBundlePresentMissingSources(t *testing.T) {
	b := ContextBundle{ProfileOK: true, GitOK: true, ShellOK: false, FileOK: true, JSONLOK: false, ProcessesOK: true}
	if got, want := b.PresentSources(), "profile, git, open editor file, running tools"; got != want {
		t.Errorf("PresentSources() = %q, want %q", got, want)
	}
	if got, want := b.MissingSources(), "shell, Claude session"; got != want {
		t.Errorf("MissingSources() = %q, want %q", got, want)
	}
}

func TestBundlePresentSourcesEmpty(t *testing.T) {
	b := ContextBundle{}
	if got := b.PresentSources(); got != "" {
		t.Errorf("PresentSources() = %q, want empty", got)
	}
	allMissing := "profile, git, shell, open editor file, Claude session, running tools"
	if got := b.MissingSources(); got != allMissing {
		t.Errorf("MissingSources() = %q, want %q", got, allMissing)
	}
}

func TestBundleSeverity(t *testing.T) {
	cases := []struct {
		name string
		b    ContextBundle
		want string
	}{
		{"git present is always ok", ContextBundle{GitOK: true}, "ok"},
		{"no git but 2+ sources is warn", ContextBundle{ProfileOK: true, ShellOK: true, JSONLOK: true}, "warn"},
		{"only profile is err", ContextBundle{ProfileOK: true}, "err"},
		{"nothing is err", ContextBundle{}, "err"},
		{"profile + shell only is warn", ContextBundle{ProfileOK: true, ShellOK: true}, "warn"},
	}
	for _, c := range cases {
		if got := c.b.Severity(); got != c.want {
			t.Errorf("%s: Severity() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBundleSerializesStably(t *testing.T) {
	b := ContextBundle{
		Profile:   "Role: senior backend engineer",
		Git:       "branch: main",
		Shell:     "git status",
		File:      "src/foo.go",
		JSONL:     "user: fix bug",
		Redacted:  2,
		ProfileOK: true,
		GitOK:     true,
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"profile":"Role: senior backend engineer"`) {
		t.Errorf("missing profile in JSON: %s", string(data))
	}
}
