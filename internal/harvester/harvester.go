package harvester

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Joncik91/inflate/internal/config"
)

// Options configures a Harvester.
type Options struct {
	// ProjectDir is the absolute path of the project being worked on.
	// Used both for the git/file collectors and to locate the Claude Code
	// session dir under ~/.claude/projects/.
	ProjectDir string
	// ClaudeProjectsRoot defaults to ~/.claude/projects when empty.
	ClaudeProjectsRoot string
	// ClaudeSessionsDir defaults to ~/.claude/sessions when empty. Used to
	// pick the active session whose cwd matches ProjectDir, which avoids
	// feeding stale unrelated transcripts when multiple sessions exist for
	// the same project dir.
	ClaudeSessionsDir string
	// Profile is the user identity loaded by config.LoadProfile().
	Profile config.Profile
}

// Harvester orchestrates the five collectors and publishes a
// latest-wins ContextBundle every time anything changes.
type Harvester struct {
	opts       Options
	bundle     atomic.Pointer[ContextBundle]
	out        chan ContextBundle
	heartbeat  atomic.Int64 // unix seconds of last successful collect
	collectMu  sync.Mutex
	sessionDir string
}

// New returns a configured Harvester. Use Run to start collection.
func New(opts Options) (*Harvester, error) {
	if opts.ClaudeProjectsRoot == "" {
		home, _ := homeDir()
		opts.ClaudeProjectsRoot = filepath.Join(home, ".claude", "projects")
	}
	if opts.ClaudeSessionsDir == "" {
		home, _ := homeDir()
		opts.ClaudeSessionsDir = filepath.Join(home, ".claude", "sessions")
	}
	h := &Harvester{
		opts:       opts,
		out:        make(chan ContextBundle, 1),
		sessionDir: filepath.Join(opts.ClaudeProjectsRoot, ProjectDirName(opts.ProjectDir)),
	}
	empty := ContextBundle{}
	h.bundle.Store(&empty)
	return h, nil
}

// Bundles returns a channel that emits the latest ContextBundle on every
// successful collection. Latest-wins: a slow consumer can drop intermediates.
func (h *Harvester) Bundles() <-chan ContextBundle { return h.out }

// Latest returns the most recent ContextBundle, or zero if none yet.
func (h *Harvester) Latest() ContextBundle { return *h.bundle.Load() }

// Heartbeat returns the unix-seconds timestamp of the last successful collect.
func (h *Harvester) Heartbeat() int64 { return h.heartbeat.Load() }

// Run blocks until ctx is cancelled. It performs an initial collection then
// re-collects on fsnotify events with debouncing.
func (h *Harvester) Run(ctx context.Context) {
	h.collectOnce()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return // watcher unavailable; we still served the initial bundle
	}
	defer w.Close()
	_ = w.Add(h.opts.ClaudeProjectsRoot) // tolerate missing
	_ = w.Add(h.sessionDir)              // tolerate missing

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	burstUntil := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.Events:
			delay := 200 * time.Millisecond
			if time.Now().Before(burstUntil) {
				delay = time.Second
			}
			burstUntil = time.Now().Add(500 * time.Millisecond)
			debounce.Reset(delay)
		case <-debounce.C:
			h.collectOnce()
			// re-add session dir if it was just created
			_ = w.Add(h.sessionDir)
		case <-w.Errors:
			// keep going
		}
	}
}

// collectOnce runs all six collectors in parallel and publishes a new bundle.
// Each goroutine writes to its own locals to avoid a data race on the struct.
func (h *Harvester) collectOnce() {
	h.collectMu.Lock()
	defer h.collectMu.Unlock()

	var wg sync.WaitGroup
	var (
		profile, git, shell, file, jsonl, procs            string
		profileOK, gitOK, shellOK, fileOK, jsonlOK, procsOK bool
		shellAge                                            time.Duration
	)

	wg.Add(6)
	go func() {
		defer wg.Done()
		profile = CollectProfile(h.opts.Profile)
		profileOK = profile != ""
	}()
	go func() {
		defer wg.Done()
		git, gitOK = CollectGit(h.opts.ProjectDir)
	}()
	go func() {
		defer wg.Done()
		shell, shellAge, shellOK = CollectShellWithAge()
	}()
	go func() {
		defer wg.Done()
		file, fileOK = CollectFile(h.opts.ProjectDir)
	}()
	go func() {
		defer wg.Done()
		jsonl, jsonlOK = CollectJSONLForSession(h.opts.ProjectDir, h.opts.ClaudeSessionsDir, h.opts.ClaudeProjectsRoot)
	}()
	go func() {
		defer wg.Done()
		procs, procsOK = CollectProcesses()
	}()
	wg.Wait()

	effectiveCwd := h.opts.ProjectDir
	// Auto-promotion: if git failed at the launch cwd but the file
	// walker discovered files clustered under one subdirectory that IS
	// a repo, promote to that subdirectory and re-run git + file. The
	// user gets full project context even when they launched from a
	// parent dir like /home/u/apps.
	if !gitOK && fileOK {
		if promoted, ok := PromoteToRepoRoot(h.opts.ProjectDir, file); ok {
			if newGit, newGitOK := CollectGit(promoted); newGitOK {
				effectiveCwd = promoted
				git = newGit
				gitOK = true
				// Re-run file with the narrower root so paths and
				// recent-files focus on the actual project.
				if newFile, newFileOK := CollectFile(promoted); newFileOK {
					file = newFile
					fileOK = true
				}
			}
		}
	}

	bundle := ContextBundle{
		Cwd:         effectiveCwd,
		Profile:     profile,
		Git:         git,
		Shell:       shell,
		File:        file,
		JSONL:       jsonl,
		Processes:   procs,
		ProfileOK:   profileOK,
		GitOK:       gitOK,
		ShellOK:     shellOK,
		FileOK:      fileOK,
		JSONLOK:     jsonlOK,
		ProcessesOK: procsOK,
		ShellAge:    shellAge,
	}
	// Only scan for neighbor repos when the current cwd itself isn't a
	// repo. After auto-promotion, gitOK may now be true, so this only
	// fires when promotion didn't apply.
	if !gitOK {
		if repos, err := config.NeighborRepos(h.opts.ProjectDir); err == nil {
			bundle.NeighborRepos = repos
		}
	}

	// scrub each section independently so flag accuracy is preserved
	scrubTotal := 0
	bundle.Profile, scrubTotal = scrubAdd(bundle.Profile, scrubTotal)
	bundle.Git, scrubTotal = scrubAdd(bundle.Git, scrubTotal)
	bundle.Shell, scrubTotal = scrubAdd(bundle.Shell, scrubTotal)
	bundle.File, scrubTotal = scrubAdd(bundle.File, scrubTotal)
	bundle.JSONL, scrubTotal = scrubAdd(bundle.JSONL, scrubTotal)
	bundle.Redacted = scrubTotal

	h.bundle.Store(&bundle)
	h.heartbeat.Store(time.Now().Unix())
	select {
	case h.out <- bundle:
	default:
		// drop: latest-wins semantics; consumer reads Latest() instead
	}
}

func scrubAdd(s string, total int) (string, int) {
	if s == "" {
		return s, total
	}
	cleaned, n := Scrub(s)
	return cleaned, total + n
}

// homeDir returns the user's home directory or "" on failure.
func homeDir() (string, error) {
	return userHomeDir()
}
