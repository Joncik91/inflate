// Package cli implements inflate's subcommands (doctor, config edit, ...).
package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Joncik91/inflate/internal/config"
)

// Edit opens the chosen config file in $EDITOR (or vi if unset).
// target: "" or "config" -> config.toml, "profile" -> profile.toml,
// "env" -> .env. Creates the file from a default template if missing.
// Returns a process exit code.
func Edit(target string) int {
	path, err := pathFor(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "inflate config:", err)
		return 2
	}
	if err := ensureFile(path); err != nil {
		fmt.Fprintln(os.Stderr, "inflate config:", err)
		return 1
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	if _, err := exec.LookPath(editor); err != nil {
		fmt.Fprintf(os.Stderr, "inflate config: $EDITOR is unset and %q not found on PATH\n", editor)
		return 1
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "inflate config:", err)
		return 1
	}
	return 0
}

// pathFor resolves a target keyword to an absolute config file path.
func pathFor(target string) (string, error) {
	switch target {
	case "", "config":
		return filepath.Join(config.ConfigDir(), "config.toml"), nil
	case "profile":
		return filepath.Join(config.ConfigDir(), "profile.toml"), nil
	case "env":
		return filepath.Join(config.ConfigDir(), ".env"), nil
	default:
		return "", fmt.Errorf("unknown target %q (try: config, profile, env)", target)
	}
}

// ensureFile creates path with a default template if it doesn't exist.
// Existing files are left untouched.
func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	body := templateFor(filepath.Base(path))
	if filepath.Base(path) == ".env" {
		mode = 0o600
	}
	return os.WriteFile(path, []byte(body), mode)
}

func templateFor(basename string) string {
	switch basename {
	case "config.toml":
		return `auto_paste = false

[provider]
kind        = "anthropic"           # or "openai_compat" or "google"
model       = "claude-haiku-4-5"
api_key_env = "ANTHROPIC_API_KEY"
`
	case "profile.toml":
		return `identity = "developer"
work     = "general software engineering"
style    = "standard"  # terse / standard / verbose
`
	case ".env":
		return `# inflate API keys. One KEY=value per line.
# Real shell env vars override these, so CI can set them externally.
`
	}
	return ""
}
