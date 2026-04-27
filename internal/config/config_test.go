package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfileMissingReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p, err := LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if p.Style != "standard" {
		t.Errorf("default style = %q, want standard", p.Style)
	}
}

func TestLoadProfileFromTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "inflate"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `
identity = "senior backend engineer (Go, Python)"
work     = "API services, CLI tools"
style    = "terse"
`
	if err := os.WriteFile(filepath.Join(dir, "inflate", "profile.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if p.Identity != "senior backend engineer (Go, Python)" {
		t.Errorf("identity = %q", p.Identity)
	}
	if p.Style != "terse" {
		t.Errorf("style = %q", p.Style)
	}
}
