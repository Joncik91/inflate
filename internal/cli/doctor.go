package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Joncik91/inflate/internal/config"
	"github.com/Joncik91/inflate/internal/provider"
)

// Doctor runs all startup checks and prints ✓/✗ per check to stdout.
// Returns process exit code: 0 if everything passes, 1 otherwise.
func Doctor() int {
	report := runDoctor(true) // include provider validate ping
	fmt.Print(report)
	if strings.Contains(report, "[✗]") {
		return 1
	}
	return 0
}

// runDoctor produces the report string. ping=true makes a 1-token round-trip
// to the provider; tests pass false to stay offline.
func runDoctor(ping bool) string {
	var b strings.Builder
	add := func(ok bool, label, hint string) {
		if ok {
			fmt.Fprintf(&b, "[✓] %s\n", label)
		} else {
			fmt.Fprintf(&b, "[✗] %s — %s\n", label, hint)
		}
	}

	cfgDir := config.ConfigDir()
	add(dirExists(cfgDir), "config dir "+cfgDir, "run `inflate` once to create it")

	profilePath := filepath.Join(cfgDir, "profile.toml")
	if fileExists(profilePath) {
		add(true, "profile.toml readable", "")
	} else {
		add(false, "profile.toml readable", "first-run wizard will create it")
	}

	cfg, cfgErr := config.LoadConfig()
	if cfgErr == nil {
		add(true, "config.toml readable + parses", "")
	} else if os.IsNotExist(cfgErr) {
		add(false, "config.toml readable + parses", "missing — run wizard or `inflate config edit`")
	} else {
		add(false, "config.toml readable + parses", "parse error: "+cfgErr.Error())
	}

	envPath := filepath.Join(cfgDir, ".env")
	if fileExists(envPath) {
		add(true, ".env present", "")
	} else {
		add(false, ".env present (warn-only)", "fine if your shell already exports the API key env var")
	}

	if cfgErr == nil {
		_ = config.LoadDotenv()
		p, provErr := provider.NewFromConfig(cfg)
		if provErr == nil {
			add(true, "API key resolves", "")
		} else {
			add(false, "API key resolves", provErr.Error())
		}

		if ping && provErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			pingErr := p.Validate(ctx)
			cancel()
			if pingErr == nil {
				add(true, "provider ping ("+p.Name()+")", "")
			} else {
				add(false, "provider ping", pingErr.Error())
			}
		}
	}

	if runtime.GOOS == "linux" {
		hasClip := commandOnPath("xclip") || commandOnPath("xsel")
		add(hasClip, "xclip or xsel installed (Linux clipboard)", "install xclip: `sudo apt install xclip`")
	}

	claudeRoot := filepath.Join(homeDir(), ".claude", "projects")
	if dirExists(claudeRoot) {
		add(true, "~/.claude/projects exists", "")
	} else {
		add(false, "~/.claude/projects exists (warn-only)", "Claude Code hasn't been launched yet on this machine")
	}

	lockPath := filepath.Join(cfgDir, "run.lock")
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
		add(false, "lockfile present (PID "+pidStr+")",
			"if no inflate is running, this is stale — next launch will auto-clear")
	} else {
		add(true, "no stale lockfile", "")
	}

	return b.String()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func commandOnPath(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
