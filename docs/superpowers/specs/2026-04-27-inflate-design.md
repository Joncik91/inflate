# Inflate — Design Spec

**Date:** 2026-04-27
**Status:** Brainstormed, awaiting plan
**Author:** jounes

## Summary

Inflate is a CLI tool that runs in a second terminal next to Claude Code. The user types a short fragment in Inflate's TUI input box; Inflate harvests local context (Claude Code's session JSONL, git state, open file, shell history, user profile) and uses an LLM to expand the fragment into a rich, context-loaded prompt. On Enter, the inflated prompt is copied to the clipboard (and optionally auto-pasted into Claude Code's window on Linux), ready for the user to submit to Claude Code.

The product embodies the **Promptism** methodology: every prompt should carry full role, context, task, constraints, and output format. Inflate makes that effortless without requiring the user to write 70 words by hand.

## Goals

- Eliminate the friction of writing context-rich prompts to Claude Code (and other terminal AI tools).
- Live, as-you-type expansion that teaches the user what a good prompt looks like by showing it.
- Zero coupling to Claude Code internals; uses only its public on-disk session JSONL.
- Cross-platform single binary (Linux + macOS + Windows day one).
- BYOK across all major providers; works with any OpenAI-compatible endpoint.

## Non-Goals (v0)

- Modifying the prompt inside Claude Code's input box (impossible today; `UserPromptSubmit` hooks cannot replace the prompt).
- ~~Native local-LLM provider (Ollama, llama.cpp).~~ Shipped in v0.1.4 as `kind = "ollama"`. llama.cpp users still go via `openai_compat` against the llama.cpp HTTP server.
- PTY wrapper (deferred to v1).
- Browser extension or web UI.
- Cloud sync of profile or settings.

## Architecture

```
┌────────────────────┐         ┌────────────────────┐
│   Terminal A       │         │   Terminal B       │
│   $ claude         │         │   $ inflate        │
│                    │         │                    │
│   [Claude Code]    │         │  ┌──────────────┐ │
│                    │         │  │  preview     │ │
│   writes JSONL ────┼────┐    │  │  (inflated)  │ │
│   to ~/.claude/    │    │    │  └──────────────┘ │
│        projects/   │    │    │  ┌──────────────┐ │
│                    │    └───►│  │  input box   │ │
│                    │  tail   │  │ (user types) │ │
└────────────────────┘         │  └──────────────┘ │
                               └─────────┬──────────┘
                                         │
                                         ▼ on Enter
                                    [clipboard]
                                         │
                                         ▼ user pastes
                                  Terminal A input
```

Five components inside the `inflate` binary:

1. **`tui`** — bubbletea app with input box, preview pane, status line.
2. **`harvester`** — gathers context from five sources, publishes a cached `ContextBundle`.
3. **`inflater`** — pure function: `(ContextBundle, seed) → streamed inflated prompt`.
4. **`provider`** — interface with three implementations (Anthropic, OpenAI-compatible, Google).
5. **`output`** — clipboard write + optional `xdotool`/`SendInput` paste.

## Components

### `tui` (bubbletea Model)

- **State:** `seed`, `preview`, `ctxFlags`, `inflightReqID`, `mode (normal|context-add|setup)`.
- **View:** 3 panes. Top = preview (Markdown-rendered via `glamour`). Middle = status line with context flags + token counts + provider name. Bottom = input box.
- **Update loop:** `KeyMsg` → mutate seed → `idleTimer(600ms)`. On idle → cancel inflight, dispatch new `inflateCmd`. On `inflateChunkMsg` → append to preview. On Enter → copy preview → optional paste → flash "sent ✓".
- **Hotkeys:** `Enter` send, `Ctrl-C` quit, `?` open context-add, `Ctrl-D` change watched dir, `Ctrl-S` switch session, `Ctrl-Enter` send raw seed bypassing inflater, `Esc` clear, `Tab` force-inflate now (skip idle wait).
- **Staleness:** while inflation in-flight or seed has changed since last fresh preview, dim preview to ~50% opacity and show `…` in status.

### `harvester`

- Goroutine with `fsnotify` watcher on `~/.claude/projects/<hash>/`. Parent watcher on `~/.claude/projects/` itself catches CREATE of new hash dirs.
- Five parallel collectors with 500ms hard timeout each:
  - `profile()` — read `~/.config/inflate/profile.toml`. Always succeeds.
  - `git()` — branch, last 3 commits, diff stat, modified files.
  - `shell()` — last 20 lines of `$HISTFILE` / `~/.bash_history` / `~/.zsh_history`.
  - `file()` — `lsof -c <editor>` to find open files in `pwd`. Skips silently if missing.
  - `jsonl()` — tail last 200 events of newest JSONL in project hash dir. Extract file paths (regex), errors/stack traces (heuristic), last 3 user prompts, last 3 assistant responses. Hard cap 4k tokens.
- **Secret scrubber** runs over assembled bundle: `AKIA[0-9A-Z]{16}`, `sk-[a-zA-Z0-9]{20,}`, `Bearer [a-zA-Z0-9]+`, `password=`, `token=`. Replaces with `[REDACTED]`. Reports redaction count to status line.
- **Caching:** cached `ContextBundle` published on a channel. Re-collected on fsnotify event (debounced 200ms; backed off to 1s during active streaming detected by 5+ events / 500ms). Force-refresh on every inflation older than 30s.
- **Heartbeat:** TUI checks freshness; if silent for >30s, status: `harvester dead — restart`, auto-restart attempted once.

### `inflater`

- Pure function: `Inflate(ctx context.Context, bundle ContextBundle, seed string) <-chan string`.
- Builds request: system prompt (Promptism skeleton + behavioral rules) + context bundle (XML-tagged sections) + seed.
- Calls `provider.Stream(req)`, forwards UTF-8-buffered chunks to channel.
- Cancellable. New inflation cancels old.
- Token budget: ~4k context + 200 seed + 800 output ≈ 5k per call.
- **Two timeouts:** 5s for first byte from provider; 30s absolute ceiling on the entire inflation regardless of streaming state. Whichever fires first cancels the request.

### `provider` (interface)

```go
type Provider interface {
    Stream(ctx context.Context, req Request) (<-chan string, error)
    Validate(ctx context.Context) error
    Name() string
}
```

Implementations:

- **`anthropic`** — official Go SDK, default model Haiku, Sonnet optional.
- **`openai_compat`** — official Go SDK with `base_url` override. Works for OpenAI, DeepSeek, Groq, OpenRouter, Together, local vLLM.
- **`google`** — official Go SDK, default model Gemini Flash.

Selected via config:
```toml
[provider]
kind = "openai_compat"
base_url = "https://api.deepseek.com/v1"
model = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
```

### `output`

- `Clipboard(text)` — `golang-design/clipboard`, cross-platform.
- `Paste(text)` — Linux: `xdotool type --delay 4 --window <id>`. Windows: clipboard only (v1). macOS: `osascript`-based paste (v1).
- Target window ID captured at startup via `Ctrl-W` so paste never targets the wrong window after alt-tab.
- Both run after Enter. Clipboard always; paste only if `auto_paste = true` in config.

### `config`

- `~/.config/inflate/config.toml` — provider config, API keys (or env var refs), `auto_paste` toggle, default `--cwd` behavior.
- `~/.config/inflate/profile.toml` — user identity, work domain, style preference (Acolyte/Adept/Grandmaster).
- `inflate config edit` opens `$EDITOR`.

## First-Run Intake

Before TUI proper, three short questions:

1. **Who are you?** (e.g., *"senior backend engineer, mostly Go and Python"*)
2. **What kind of work?** (e.g., *"day job: API services, side projects: CLI tools and TUIs"*)
3. **Prompt style preference?** terse / standard / verbose.

Saved to `~/.config/inflate/profile.toml`. Skipped if `!isatty(stdin)` (SSH, CI, Docker — built-in default profile + warning logged).

## Data Flow

### Cold start

1. Load `config.toml` + `profile.toml`. Missing → intake wizard.
2. Acquire lockfile via `O_CREAT|O_EXCL`.
3. Resolve API key from env or config. `provider.Validate()` ping.
4. Compute project hash from `pwd`. Mkdir-watch `~/.claude/projects/`.
5. Spawn `harvester` goroutine. Initial collection runs in parallel.
6. TUI starts. Status line shows what loaded.

### Per inflation

```
keystroke → tui.Update → seed mutated → 600ms idle timer
                                              │
                                              ▼ timer fires
                                       cancel inflight req
                                       harvester.LatestBundle() (cached)
                                       inflater.Inflate(bundle, seed)
                                              │ stream chunks
                                              ▼ UTF-8 buffer
                                       tui.preview update
                                              │ on Enter
                                              ▼
                                       output.Clipboard(preview)
                                       output.Paste(preview)? [if enabled]
                                       status: "sent ✓"
```

### Background

```
fsnotify event → debounce 200ms → harvester.collect()
                                          │ parallel collectors
                                          ▼
                                  scrub secrets → cache ContextBundle
                                  publish on channel (latest-wins)
                                  tui status flags update
```

Inflation **does not** wait for harvester refresh — reads cached bundle synchronously. Worst case: bundle is one keystroke debounce stale.

## Cold-Start & Empty-Context Behavior

When all collectors except `profile()` return empty (brand-new dir, no git, no JSONL, no shell history):

- Status: `ctx: profile✓ git✗ shell✗ file✗ jsonl✗`.
- Inflater runs in **pure-structure mode** — applies Promptism skeleton (Role/Task/Constraints/Output) without inventing context.
- Profile alone is enough to set Role and tone:
  ```
  Role: senior backend engineer (Go, Python).
  Task: <user's seed>.
  Constraints: terse output preferred; ask for file/error if not provided.
  Output: root cause + minimal patch.
  ```
- `?` hotkey opens a one-line interactive context-add for ad-hoc enrichment.

## Error Handling

| Category | Trigger | Behavior |
|---|---|---|
| Config | bad TOML, missing API key env, invalid provider | Setup wizard, TUI input disabled until fixed |
| Provider auth | 401/403 | Status: `auth failed (check key)`, raw-seed mode |
| Provider rate limit | 429 | Status: `rate limited (Xs)`, pure-structure mode for that inflation |
| Provider quota | quota-specific error body | Status: `monthly quota exhausted`, pure-structure mode for rest of session |
| Provider timeout | 5s for first byte; 30s total | Cancel, status: `timeout`, fall back to last good preview or seed |
| Provider stream stall | 3s no chunk mid-stream | Cancel, keep partial output, status: `partial (network)` |
| Provider empty/refusal | empty body or content filter | Pure-structure mode, status: `provider returned empty — using local skeleton` |
| Provider streaming protocol differs | 2s no `data:` line | Fall back to non-streaming for that inflation |
| Provider HTML body | captive portal, proxy interception | Status: `network intercepted (captive portal?)` |
| Harvester collector failure | git not installed, lsof missing, parse error | That source flagged `✗`, others continue |
| Harvester goroutine crash | panic | `recover()`, heartbeat detection, auto-restart once |
| fsnotify watcher fails | dir doesn't exist, FD limit | Retry once, then 30s polling, status: `polling` |
| Lockfile collision | another instance running | Refuse start, print PID + `--force` hint to stderr |
| Stale lockfile | crashed previous run | Check PID exists + matches process name; remove if stale |
| Disk full on log write | ENOSPC | Switch to in-memory ring buffer, status: `logging disabled (disk full)` |
| Clipboard write fails | no clipboard daemon | Status: `clipboard unavailable — preview only` |
| xdotool missing or wrong window | Linux paste path | Fall back to clipboard, status: `paste failed (clipboard ok)` |
| TUI render error | terminal too narrow (<60 cols) | Collapsed single-pane layout |
| Non-ASCII rejected by provider | rare 400 with encoding error | Fall back to seed-as-is in clipboard |
| System sleep mid-inflation | wake after hours | 30s absolute ceiling cancels |

**Never:** modal dialogs, crashes that lose preview, silent failures (every degraded mode shows a flag), auto-retry on user-visible operations.

**Logging:** `~/.cache/inflate/inflate.log` (rotated at 10MB). Provider request/response bodies never logged. Crash log separate at `~/.cache/inflate/crash.log`.

## Testing

### Unit (table-driven)

- `harvester` collectors with mocked filesystem (`testing/fstest`), mocked `git` via `testscript`, JSONL fixtures. Cover empty dir, missing tools, malformed JSONL, oversized JSONL.
- `harvester` secret scrubber — table of (input, expected) pairs covering AWS/OpenAI/bearer/generic. Negative cases for false positives.
- `inflater` request builder — golden file assertion. No network.
- `provider` impls — `httptest.Server` with canned SSE/ndjson chunks. Cover success stream, mid-stream stall, 401, 429, empty body, HTML body, malformed JSON.
- `output.Clipboard` — gated by build tag `clipboard`, skipped in CI.
- `config` loader — TOML parse, env var resolution, missing-file fallback.

### Integration (in-process)

- `harvester` end-to-end with temp dir as `~/.claude/projects/`. Write JSONL, fire fsnotify events, assert `ContextBundle`.
- `inflater` end-to-end with fake provider returning fixed string. Assert TUI receives chunks.
- Lockfile race — two goroutines, assert exactly one wins.
- Cancellation — start inflation, cancel ctx mid-stream, assert no leaked goroutines (`goleak`).

### TUI (snapshot)

- `teatest` for headless render.
- Snapshots on key states: empty input, mid-typing (preview dim), inflation done, error state, intake wizard.
- Hotkey tests: `Tab`, `?`, `Ctrl-S`.

### Manual demo path

`docs/manual-test.md` checklist:

1. Fresh install, no config — intake wizard runs.
2. Brand-new dir, no git — pure-structure mode.
3. Run `claude` in Terminal A, type a prompt — Terminal B's `jsonl✗` flips to `jsonl✓`.
4. Type fragment in B → preview inflates → Enter → paste in A → prompt is rich.
5. Disconnect network → status `timeout` → seed still copies.
6. Revoke API key → status `auth failed`, no crash.

### CI

- GitHub Actions: `go test ./...` on linux/darwin/windows.
- `golangci-lint`.
- Build matrix → signed binaries for all three OSes on tag.
- Clipboard/xdotool gated by build tag.

### Coverage target

- 70% on `harvester`, `inflater`, `provider`, `config`.
- ~30% on TUI via snapshots.
- `output.*` manual only.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Anthropic ships `replaceUserMessage` for `UserPromptSubmit` hooks | medium | makes Inflate's two-terminal architecture obsolete | Pivot Inflate to ship as a hook + standalone TUI. The harvester + inflater + provider layers stay; only `tui` and `output` change. |
| Provider streaming protocol drift | low | inflation breaks for one provider | Per-provider integration tests; non-streaming fallback path. |
| Secret scrubber misses a token format | low | user secrets sent to provider | Conservative regex set; document scrubber rules; allow user-extensible patterns in config. |
| Bubbletea TUI flicker on Windows Terminal | medium | poor demo on Windows | Test on Windows Terminal early; fall back to single-pane if rendering glitches. |
| User confused by two-terminal model | medium | bounce after install | README has 30s GIF + clear "open second terminal" intro on first run. |

## Distribution

- `go install github.com/jounes/inflate@latest` (Go users). _(GitHub owner placeholder — set on repo creation.)_
- Pre-built binaries on GitHub Releases for linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64.
- Homebrew tap (`brew install jounes/tap/inflate`).
- `apt` repo (later).
- Single static binary, ~10-15 MB.

## Out of Scope (deferred)

This is the **canonical deferred list**. PR descriptions point here instead of duplicating their own copies; items move out when shipped.

### v1 (architectural)

- PTY wrapper (`inflate-claude`) for true watch-as-you-type in Claude Code's input box.
- Hook-based mode (when/if Anthropic ships prompt-replacement support).
- WSL clipboard interop.
- Remote Claude Code session (SSH'd into a server).
- Per-template selection beyond defaults (`debug`, `refactor`, `explain`, `design`, `review`).

### v2 (brand layer)

- Daily Liturgy hints, Lexicon of the Silent (Promptism brand-layer features).
- Public leaderboard / rank badge.

### Discovered post-spec (untargeted)

Items surfaced during implementation. No version commitment; they ship when there's a real reason.

- OS keychain integration (`libsecret` / macOS keychain / `credman`). v0.1.1 chose `.env` instead — equivalent UX, zero OS-specific code paths.
- `inflate key set/list/rm` subcommand. `inflate config edit env` covers the same surface.
- Per-event filtering inside a JSONL by the `cwd` field on each entry. v0.1.2 picks the right *session*; this would refine intra-session if a single session ranges across multiple subdirs.
- `inflate sessions ls` to list candidate sessions inflate is choosing between. Picker is deterministic, so this is debug-only.
- Migrate `internal/lockfile` to use `internal/process.Alive` (currently has its own per-OS copy). Cosmetic — both work, no urgency.

### Shipped (moved out)

- ✅ v0.1.1 — interactive first-run wizard (provider + hidden API key prompt → `.env`).
- ✅ v0.1.1 — `inflate doctor` and `inflate config edit` subcommands.
- ✅ v0.1.1 — smart `--cwd` (walks to nearest `.git` ancestor).
- ✅ v0.1.1 — auto-clean stale lockfile via per-OS process-name verification.
- ✅ v0.1.2 — session-aware JSONL picker (matches the live Claude Code session by `cwd`+`pid`+`status` instead of "newest file in dir").
- ✅ v0.1.2 — `claude_projects_dir` / `claude_sessions_dir` config knobs.
- ✅ v0.1.3 — plain-English status legend with severity colors (replaces `profile✓ git✗ shell✓ file✗ jsonl✓`).
- ✅ v0.1.3 — streaming preview with spinner (per-chunk delivery via `tea.Program.Send`).
- ✅ v0.1.3 — `?` help overlay listing every keybinding.
- ✅ v0.1.3 — persistent error banner (errors no longer flash by as 1.5 s toasts).
- ✅ v0.1.3 — named-section preview rendering (Role · Context · Task · …).
- ✅ v0.1.3 — prompt-quality fix: skeleton rule that JSONL is exploration not fact, plus dropping recent user prompts from the JSONL summary (closed issue #4).
- ✅ v0.1.4 — native Ollama provider. Wizard auto-detects a running daemon, lists chat-capable models with parameter sizes, skips the API-key prompt; doctor pings `/api/tags` instead of running a chat completion; factory loosens the API-key requirement for keyless local providers. (llama.cpp users continue to use `kind = "openai_compat"` against llama.cpp's server.)
