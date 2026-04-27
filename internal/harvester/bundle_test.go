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
