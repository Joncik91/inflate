package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsMissingProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	report := runDoctor(false) // false = skip provider validate ping
	if !containsLineWithMark(report, "✗", "profile.toml") {
		t.Errorf("expected ✗ for missing profile, got:\n%s", report)
	}
}

func TestDoctorReportsAllOK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "inflate")
	mustMkdir(t, cfgDir)
	mustWrite(t, filepath.Join(cfgDir, "profile.toml"),
		"identity = \"x\"\nwork = \"y\"\nstyle = \"standard\"\n")
	mustWrite(t, filepath.Join(cfgDir, "config.toml"),
		"auto_paste = false\n[provider]\nkind = \"anthropic\"\nmodel = \"claude-haiku\"\napi_key = \"sk-test\"\n")
	mustWrite(t, filepath.Join(cfgDir, ".env"), "")

	report := runDoctor(false) // skip ping (no real network)
	if !containsLineWithMark(report, "✓", "profile.toml") {
		t.Errorf("expected ✓ for profile.toml, got:\n%s", report)
	}
	if !containsLineWithMark(report, "✓", "config.toml") {
		t.Errorf("expected ✓ for config.toml, got:\n%s", report)
	}
	if !containsLineWithMark(report, "✓", "API key") {
		t.Errorf("expected ✓ for API key, got:\n%s", report)
	}
}

func containsLineWithMark(out, mark, needle string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, mark) && strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
