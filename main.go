package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

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
	var (
		cwdFlag = flag.String("cwd", "", "project directory to harvest (default: $PWD)")
		forceLk = flag.Bool("force", false, "ignore stale lockfile if process check fails")
		winID   = flag.Int("paste-window", 0, "X11 window ID to auto-paste into (Linux only)")
	)
	flag.Parse()

	cwd := *cwdFlag
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fatal("getwd: %v", err)
		}
	}
	cwd, _ = filepath.Abs(cwd)

	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "inflate")
	logger, _ := logging.Init(cacheDir)
	logger.Info("inflate starting", "cwd", cwd)

	if err := output.Init(); err != nil {
		logger.Warn("clipboard init failed", "err", err)
	}

	profile, _ := config.LoadProfile()
	if profile.Identity == "developer" && term.IsTerminal(int(os.Stdin.Fd())) {
		// first run — wizard
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
			fatal("no config.toml found at %s. Create it (see README) and re-run.", config.ConfigDir())
		}
		fatal("config: %v", err)
	}

	prov, err := provider.NewFromConfig(cfg)
	if err != nil {
		fatal("provider: %v", err)
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
		ProjectDir: cwd,
		Profile:    profile,
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
