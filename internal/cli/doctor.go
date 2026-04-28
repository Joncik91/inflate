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
	"github.com/Joncik91/inflate/internal/harvester"
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
	if cfgErr == nil && cfg.ClaudeProjectsDir != "" {
		claudeRoot = cfg.ClaudeProjectsDir
	}
	if dirExists(claudeRoot) {
		add(true, claudeRoot+" exists", "")
	} else {
		add(false, claudeRoot+" exists (warn-only)", "Claude Code hasn't been launched yet on this machine, or claude_projects_dir is wrong")
	}

	// Harvester collectors — surface underlying errors so the user can fix
	// what's broken (e.g. "git config safe.directory ..."), not just see a
	// silent ✗ in the TUI status line.
	cwd, _ := os.Getwd()
	if cfgErr == nil {
		if root, rerr := config.ResolveCwd(""); rerr == nil {
			cwd = root
		}
	}

	if _, ok, gErr := harvester.DiagnoseGit(cwd); ok {
		add(true, "harvester: git in "+cwd, "")
	} else {
		hint := "not a git repo, or run: git config --global --add safe.directory " + cwd
		if gErr != nil && strings.Contains(gErr.Error(), "dubious ownership") {
			hint = "git refuses to read this repo as the current user. Run: git config --global --add safe.directory " + cwd
		}
		add(false, "harvester: git — "+flatErr(gErr), hint)
	}

	if _, ok, sErr := harvester.DiagnoseShell(); ok {
		add(true, "harvester: shell history", "")
	} else {
		add(false, "harvester: shell history", flatErr(sErr))
	}

	if _, ok, fErr := harvester.DiagnoseFile(cwd); ok {
		add(true, "harvester: open editor file", "")
	} else {
		add(false, "harvester: open editor file (warn-only)", flatErr(fErr))
	}

	jsonlDir := filepath.Join(claudeRoot, harvester.ProjectDirName(cwd))
	if _, ok, jErr := harvester.DiagnoseJSONL(jsonlDir); ok {
		add(true, "harvester: jsonl session", "")
	} else {
		add(false, "harvester: jsonl session (warn-only)", flatErr(jErr))
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

// flatErr is errMsg + newline collapse. Some collectors propagate underlying
// errors that include embedded newlines (notably git stderr like "dubious
// ownership ... To add an exception ..."). Doctor renders one check per line,
// so we collapse interior whitespace to keep the line readable. The full
// command the user needs is preserved in the hint we pass to add().
func flatErr(err error) string {
	if err == nil {
		return "(no detail)"
	}
	s := err.Error()
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// errMsg returns the error string or a fallback if err is nil.
func errMsg(err error) string {
	if err == nil {
		return "(no detail)"
	}
	return err.Error()
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
