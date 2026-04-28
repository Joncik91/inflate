// Package config loads ~/.config/inflate/{config,profile}.toml.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile is the user identity captured at first-run intake.
type Profile struct {
	Identity string `toml:"identity"`
	Work     string `toml:"work"`
	Style    string `toml:"style"` // "terse" | "standard" | "verbose"
}

// Config is provider + behavior settings.
type Config struct {
	Provider  ProviderConfig `toml:"provider"`
	AutoPaste bool           `toml:"auto_paste"`
	// ClaudeProjectsDir overrides the default ~/.claude/projects path used
	// to locate Claude Code session JSONL. Empty means default.
	ClaudeProjectsDir string `toml:"claude_projects_dir"`
}

// ProviderConfig selects and configures the LLM backend.
type ProviderConfig struct {
	Kind      string `toml:"kind"`        // "anthropic" | "openai_compat" | "google"
	BaseURL   string `toml:"base_url"`    // openai_compat only
	Model     string `toml:"model"`
	APIKey    string `toml:"api_key"`     // inline; or use APIKeyEnv
	APIKeyEnv string `toml:"api_key_env"` // env var holding the key
}

// ConfigDir returns the directory holding inflate's config files,
// honoring XDG_CONFIG_HOME when set.
func ConfigDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "inflate")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "inflate")
}

// LoadProfile reads profile.toml. Returns defaults if missing.
func LoadProfile() (Profile, error) {
	path := filepath.Join(ConfigDir(), "profile.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultProfile(), nil
	}
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if _, err := toml.Decode(string(data), &p); err != nil {
		return defaultProfile(), nil // half-written file: keep going with default
	}
	if p.Style == "" {
		p.Style = "standard"
	}
	return p, nil
}

// SaveProfile writes profile.toml, creating the dir if needed.
func SaveProfile(p Profile) error {
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(ConfigDir(), "profile.toml"))
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(p)
}

// SaveConfig writes config.toml, creating the dir if needed.
// This is what the first-run wizard calls after the user picks a provider.
// We deliberately do NOT write inline api_key here — keys live in .env.
func SaveConfig(c Config) error {
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(ConfigDir(), "config.toml"))
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// LoadConfig reads config.toml. Returns zero Config + os.ErrNotExist if missing.
func LoadConfig() (Config, error) {
	path := filepath.Join(ConfigDir(), "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if _, err := toml.Decode(string(data), &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func defaultProfile() Profile {
	return Profile{
		Identity: "developer",
		Work:     "general software engineering",
		Style:    "standard",
	}
}
