package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Joncik91/inflate/internal/config"
	"github.com/Joncik91/inflate/internal/intake"
)

// SwitchProvider re-runs the wizard's provider step against the current
// config and writes the result back. Lets users hop between providers
// (e.g. DeepSeek → local Ollama) without hand-editing config.toml.
//
// Preserves all non-provider settings (auto_paste, claude_projects_dir,
// claude_sessions_dir). Existing API keys in .env are preserved too —
// switching to Anthropic doesn't wipe your DeepSeek key.
func SwitchProvider() int {
	current, err := config.LoadConfig()
	if err != nil {
		// Missing config? Tell the user to run inflate normally first so the
		// first-run wizard kicks in. Don't pretend this is a recovery path.
		fmt.Fprintln(os.Stderr, "inflate config provider: no config.toml yet — run `inflate` once to set up a profile")
		return 1
	}

	if current.Provider.Kind != "" {
		label := current.Provider.Kind
		if current.Provider.Model != "" {
			label += ":" + current.Provider.Model
		}
		fmt.Printf("Current provider: %s\n", label)
	}

	prov, keyName, keyVal, err := intake.RunProviderOnly(os.Stdin, os.Stdout, intake.TerminalKeyReader{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "inflate config provider:", err)
		return 1
	}

	// Preserve everything except [provider]. AutoPaste and the Claude path
	// overrides aren't part of the provider step.
	current.Provider = prov
	if err := config.SaveConfig(current); err != nil {
		fmt.Fprintln(os.Stderr, "save config:", err)
		return 1
	}

	if keyVal != "" {
		if err := config.WriteEnvVar(keyName, keyVal); err != nil {
			fmt.Fprintln(os.Stderr, "save .env:", err)
			return 1
		}
		_ = os.Setenv(keyName, keyVal)
		fmt.Printf("\n✓ provider switched to %s\n", prov.Kind)
		fmt.Printf("✓ key saved to %s (mode 0600)\n", filepath.Join(config.ConfigDir(), ".env"))
	} else {
		fmt.Printf("\n✓ provider switched to %s (no API key needed)\n", prov.Kind)
	}
	fmt.Println("Run `inflate doctor` to verify, then `inflate` to use the new provider.")
	return 0
}
