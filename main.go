package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/Joncik91/inflate/internal/cli"
	"github.com/Joncik91/inflate/internal/config"
	"github.com/Joncik91/inflate/internal/harvester"
	"github.com/Joncik91/inflate/internal/intake"
	"github.com/Joncik91/inflate/internal/lockfile"
	"github.com/Joncik91/inflate/internal/logging"
	"github.com/Joncik91/inflate/internal/output"
	"github.com/Joncik91/inflate/internal/provider"
	"github.com/Joncik91/inflate/internal/tui"
)

func main() {
	// Subcommand dispatcher: only fires when the first arg is a bare word
	// (not a flag). Top-level flags fall through to the TUI launch.
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "doctor":
			os.Exit(cli.Doctor())
		case "config":
			sub := ""
			if len(os.Args) >= 3 {
				sub = os.Args[2]
			}
			os.Exit(cli.Edit(sub))
		}
	}

	var (
		cwdFlag = flag.String("cwd", "", "project directory to harvest (default: walk up to .git from $PWD, else $PWD)")
		forceLk = flag.Bool("force", false, "ignore stale lockfile if process check fails")
		winID   = flag.Int("paste-window", 0, "X11 window ID to auto-paste into (Linux only)")
	)
	flag.Parse()

	cwd, err := config.ResolveCwd(*cwdFlag)
	if err != nil {
		fatal("resolve cwd: %v", err)
	}

	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "inflate")
	logger, _ := logging.Init(cacheDir)
	logger.Info("inflate starting", "cwd", cwd)

	if err := output.Init(); err != nil {
		logger.Warn("clipboard init failed", "err", err)
	}

	// Load .env into the process environment so the provider factory's
	// os.Getenv lookup works without the user sourcing anything. Real env
	// vars already set in the shell take precedence (CI-friendly).
	if err := config.LoadDotenv(); err != nil {
		logger.Warn("load .env", "err", err)
	}

	profile, _ := config.LoadProfile()
	cfgPath := filepath.Join(config.ConfigDir(), "config.toml")
	_, cfgStatErr := os.Stat(cfgPath)
	cfgMissing := errors.Is(cfgStatErr, os.ErrNotExist)
	firstRun := profile.Identity == "developer" && cfgMissing

	if firstRun && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("Welcome to inflate. A few quick questions to get you set up:")
		setup, err := intake.RunFullSetup(os.Stdin, os.Stdout, intake.TerminalKeyReader{})
		if err != nil {
			fatal("intake: %v", err)
		}
		if err := config.SaveProfile(setup.Profile); err != nil {
			fatal("save profile: %v", err)
		}
		if err := config.SaveConfig(config.Config{Provider: setup.Provider, AutoPaste: false}); err != nil {
			fatal("save config: %v", err)
		}
		if setup.APIKeyValue != "" {
			if err := config.WriteEnvVar(setup.APIKeyName, setup.APIKeyValue); err != nil {
				fatal("save .env: %v", err)
			}
			// Make the key visible to this process immediately so we don't
			// have to spawn a fresh shell.
			_ = os.Setenv(setup.APIKeyName, setup.APIKeyValue)
		}
		profile = setup.Profile
		fmt.Printf(`
✓ profile saved to %s
✓ config  saved to %s
✓ key     saved to %s (mode 0600)

Inflate will read these on every launch — no shell setup needed.
To rotate the key later: inflate config edit env

`, filepath.Join(config.ConfigDir(), "profile.toml"), cfgPath, filepath.Join(config.ConfigDir(), ".env"))
	} else if profile.Identity == "developer" && term.IsTerminal(int(os.Stdin.Fd())) {
		// Profile missing but config exists — old v0 user; just collect a profile.
		fmt.Println("Welcome to inflate. Three quick questions:")
		p, err := intake.RunFromReader(os.Stdin, os.Stdout)
		if err == nil {
			profile = p
			if err := config.SaveProfile(p); err != nil {
				logger.Warn("save profile", "err", err)
			}
		}
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fatal("no config.toml found at %s. Run `inflate config edit` or re-run for the wizard.", config.ConfigDir())
		}
		fatal("config: %v", err)
	}

	prov, err := provider.NewFromConfig(cfg)
	if err != nil {
		fatal("provider: %v\n\nTry `inflate doctor` to see which step failed.", err)
	}

	// Lockfile
	lockPath := filepath.Join(config.ConfigDir(), "run.lock")
	if *forceLk {
		_ = os.Remove(lockPath)
	}
	lock, err := lockfile.Acquire(lockPath)
	if err != nil {
		fatal("%v", err)
	}
	defer lock.Release()

	// Harvester
	h, err := harvester.New(harvester.Options{
		ProjectDir:         cwd,
		ClaudeProjectsRoot: cfg.ClaudeProjectsDir,
		ClaudeSessionsDir:  cfg.ClaudeSessionsDir,
		Profile:            profile,
	})
	if err != nil {
		fatal("harvester: %v", err)
	}
	go h.Run(rootContext())

	// TUI
	m := tui.New(prov, h, cfg.AutoPaste, *winID)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal("tui: %v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "inflate: "+format+"\n", args...)
	os.Exit(1)
}

// rootContext returns a Background context for the harvester. Cancelled when
// main exits (program teardown is implicit via os.Exit / TUI return).
func rootContext() context.Context { return context.Background() }
