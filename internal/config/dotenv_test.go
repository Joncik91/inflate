package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotenvMissingFileNoError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := LoadDotenv(); err != nil {
		t.Errorf("missing .env should not error, got %v", err)
	}
}

func TestLoadDotenvExportsValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "inflate"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "DEEPSEEK_API_KEY=sk-test123\n"
	if err := os.WriteFile(filepath.Join(dir, "inflate", ".env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "") // ensure clean
	os.Unsetenv("DEEPSEEK_API_KEY")
	if err := LoadDotenv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DEEPSEEK_API_KEY"); got != "sk-test123" {
		t.Errorf("DEEPSEEK_API_KEY = %q, want sk-test123", got)
	}
}

func TestLoadDotenvDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "inflate"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "FOO=fromfile\n"
	if err := os.WriteFile(filepath.Join(dir, "inflate", ".env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "preexisting")
	if err := LoadDotenv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("FOO"); got != "preexisting" {
		t.Errorf("FOO = %q, want preexisting (real env should win)", got)
	}
}

func TestWriteEnvVarCreatesFileWithMode0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := WriteEnvVar("ANTHROPIC_API_KEY", "sk-ant-xyz"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inflate", ".env")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %o, want 600", mode)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "ANTHROPIC_API_KEY=sk-ant-xyz") {
		t.Errorf("file content = %q, want it to contain the key=value", string(data))
	}
}

func TestWriteEnvVarReplacesExistingLineAndPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "inflate"), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := "OTHER=preserved\nDEEPSEEK_API_KEY=old-value\nTRAILING=also_preserved\n"
	path := filepath.Join(dir, "inflate", ".env")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteEnvVar("DEEPSEEK_API_KEY", "new-value"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "OTHER=preserved") {
		t.Errorf("OTHER line was lost: %q", s)
	}
	if !strings.Contains(s, "TRAILING=also_preserved") {
		t.Errorf("TRAILING line was lost: %q", s)
	}
	if !strings.Contains(s, "DEEPSEEK_API_KEY=new-value") {
		t.Errorf("new value not present: %q", s)
	}
	if strings.Contains(s, "old-value") {
		t.Errorf("old value still present: %q", s)
	}
}
