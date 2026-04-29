# Architecture

How inflate fits together. Read this if you want to extend it, debug a weird inflation, or just understand what's running.

## The three boxes

```
┌─────────────────────────────────────────────────────────────────┐
│                          inflate process                         │
│                                                                  │
│   ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│   │  harvester   │───►│   inflater   │───►│       TUI        │  │
│   │              │    │              │    │   (bubbletea)    │  │
│   │ git, shell,  │    │ assembles    │    │                  │  │
│   │ file, jsonl, │    │ system+user  │    │ streams chunks,  │  │
│   │ processes,   │    │ prompts,     │    │ renders preview, │  │
│   │ profile      │    │ calls LLM    │    │ handles input    │  │
│   └──────────────┘    └──────────────┘    └──────────────────┘  │
│         │                                          │             │
│         ▼                                          ▼             │
│   ContextBundle                              clipboard /         │
│   (latest snapshot)                          $WAYLAND_PASTE      │
└─────────────────────────────────────────────────────────────────┘
                                                    │
                                                    ▼
                                              Terminal A's
                                              Claude Code input
```

Three packages, three concerns. Nothing crosses concerns: the harvester doesn't know LLMs exist, the inflater doesn't know how the TUI renders, the TUI doesn't know how context is gathered.

## `internal/harvester` — context collection

Runs at startup, in the background, refreshing a `ContextBundle` snapshot the TUI reads on each inflation.

Six collectors, all running in parallel goroutines:

| Collector | What it reads | Output |
|---|---|---|
| `git.go` | `git status`, `git diff --cached`, recent commits, untracked files | branch + modified/staged/untracked file lists |
| `shell.go` | `~/.bash_history`, `~/.zsh_history` | recent commands, dead-path-pruned |
| `file.go` | `lsof` for files open in `$EDITOR`; falls back to "recently modified files" walk when no editor is detected | currently-edited file content (truncated) |
| `jsonl.go` | `~/.claude/projects/<project-hash>/*.jsonl` — Claude Code's session log | recent assistant replies, file references, error messages |
| `processes.go` | `ps -o comm= -u $USER`, filtered to a dev-tool allowlist | "running tools: Claude Code, Go toolchain, Cargo (×2)" |
| (profile) | `~/.config/inflate/profile.toml` (loaded once) | identity, work kind, style |

Each collector returns its findings + an `ok` boolean. The bundle aggregates them so the TUI's status line can show `Using profile, git, shell` / `Missing: open editor file, Claude session`.

### Auto-promotion

If the user launches inflate from a parent directory containing one git repo (e.g. `~/apps/` when only `~/apps/inflate/` is a repo), the harvester promotes itself to that repo's root and re-harvests. This handles the common "I cd'd up one level" case without making the user pass `--cwd`.

### Session-aware JSONL picker

Claude Code can have multiple sessions per project dir. The picker matches by `cwd + pid + status: active` instead of "newest file in dir," so concurrent sessions don't pollute each other.

## `internal/inflater` — prompt assembly + LLM call

Tiny package: one file, one entry point.

```go
func Inflate(ctx, provider, bundle, seed) <-chan string
```

It:

1. Builds the **system prompt** (`SystemPrompt(b)`) — the Promptism skeleton + rules ("never invent files", "JSONL is exploration not fact", "emit all 5 sections", etc.). Different rules apply when the bundle is empty (`pure-structure` mode vs `rich-context` mode).
2. Builds the **user prompt** (`UserPrompt(b, seed)`) — XML-tagged context blocks (`<git>`, `<shell>`, `<file>`, `<jsonl>`, `<processes>`, `<profile>`, `<cwd>`) followed by the user's `<seed>`.
3. Calls `provider.Stream(ctx, req)` and forwards chunks on the returned channel.

The 180-second deadline is enforced here. Cloud usually finishes in 3-10s; local Ollama with a 36B MoE on iGPU can take 50-90s. 180s is the "genuinely hung" threshold.

## `internal/provider` — LLM backends

A thin `Provider` interface:

```go
type Provider interface {
    Name() string
    Validate(ctx) error
    Stream(ctx, Request) (<-chan string, error)
}
```

Four implementations:

| File | Endpoint | Streaming format |
|---|---|---|
| `anthropic.go` | `https://api.anthropic.com/v1/messages` | SSE, Anthropic's event types |
| `openai_compat.go` | `<base_url>/chat/completions` | SSE, `data: {"choices":[{"delta":{"content":...}}]}` + `data: [DONE]` |
| `google.go` | Gemini's streamGenerateContent | SSE-ish JSON events |
| `ollama.go` | `<base_url>/api/chat` | NDJSON, `{"message":{"content":...},"done":...}` |

`factory.go` builds a Provider from `config.Config`. The `providerNeedsKey()` function is what lets Ollama skip the API-key requirement.

### Why native `/api/chat` for Ollama instead of OpenAI-compat?

Ollama exposes `/v1/chat/completions` as a compatibility shim, but the shim doesn't pass through Ollama-specific options like `think` or `num_ctx`. For reasoning models (qwen3.6, gemma4) this is the difference between getting output and getting empty stream. See [providers.md](providers.md) for details.

## `internal/tui` — terminal interface

[bubbletea](https://github.com/charmbracelet/bubbletea) Model-Update-View loop.

### Key files

- `model.go` — the Model struct + Update dispatcher. Owns input handling, idle timer, inflight cancellation.
- `view.go` — rendering. Section parser (Promptism preview), status line, help overlay, error banner.
- `provider_switch.go` — the `?`/`p` cycle that swaps Provider live and persists to disk.
- `messages.go` — bubbletea message types (chunks, idle fired, bundle updated, etc.).

### Streaming flow

```
inflater.Inflate(ctx) ──► chan string
                              │
                              ▼
                    goroutine forwards each
                    chunk via prog.Send(...)
                              │
                              ▼
                    bubbletea routes msg
                              │
                              ▼
                    Update appends to preview,
                    View re-renders pane
```

The `prog *tea.Program` ref is needed for `prog.Send` from inside Cmds. main.go injects it after `tea.NewProgram` returns, via a `ProgramInjectMsg` self-send.

### Idle debounce

Every seed-mutating keystroke bumps `m.idleGen` and schedules an `idleAfter(600ms, gen)` Cmd. When that fires, it carries its scheduled gen — only acts if gen matches `m.idleGen` (the latest). Without this, every keystroke leaves a pending timer that fires later and cancels whatever inflation is running. Cloud was fast enough to outrun the cancellation race; local wasn't. ([commit `2d58b21`](https://github.com/Joncik91/inflate/commit/2d58b21) for the full diagnosis.)

## `internal/cli` — subcommands

`doctor.go` — exhaustive startup check, exits 0/1.

`edit.go` — opens config files in `$EDITOR`. Creates them with defaults if missing.

`provider.go` — re-runs the provider wizard step against existing config, preserving non-provider fields (auto_paste, claude_projects_dir, etc.).

## `internal/config`, `internal/intake`, `internal/output`, `internal/lockfile`, `internal/process`, `internal/logging`

The boring utility packages:

- `config` — TOML load/save, dotenv reading + writing.
- `intake` — first-run wizard. Shared between `RunFullSetup` and `RunProviderOnly`.
- `output` — clipboard write (xclip/xsel/wl-copy/pbcopy depending on OS) and X11 auto-paste.
- `lockfile` — single-instance lockfile with cross-user PID liveness check.
- `process` — `Alive(pid)` that handles cross-user EPERM correctly.
- `logging` — slog-based file logger to `~/.cache/inflate/inflate.log`.

## Data flow on a single inflation

1. **User types** `what's next?` in the TUI.
2. After 600ms idle, `Update` receives `idleFiredMsg` (matching gen) → calls `startInflation()`.
3. `startInflation` cancels any prior inflight, increments `inflightID`, snapshots `bundle = harvester.Latest()`, and spawns a goroutine.
4. The goroutine calls `inflater.Inflate(ctx, provider, bundle, seed)` and reads its channel.
5. Each chunk → `prog.Send(inflateChunkMsg{Text, ReqID})`.
6. `Update` receives `inflateChunkMsg`, appends to `m.preview`, re-renders.
7. When the channel closes, the goroutine sends `inflateDoneMsg`.
8. User presses `Enter` → `output.WriteClipboard(m.preview)` → toast "copied ✓".

## Where to add things

| Want to add… | Where it goes |
|---|---|
| A new LLM backend | New file in `internal/provider/` implementing the interface, register in `factory.go`. |
| A new context source | New collector in `internal/harvester/`, plug into `Run()` and `ContextBundle`. |
| A new TUI key | Handle in `model.go:handleKey`. |
| A new CLI subcommand | Add a case in `main.go`'s dispatcher and a file in `internal/cli/`. |
| A change to the prompt skeleton | Edit `internal/inflater/prompt.go`. |

## Further reading

- [Design spec](superpowers/specs/2026-04-27-inflate-design.md) — full design doc with the canonical deferred list and the why behind each decision.
- [Provider configuration](providers.md) — the LLM-side details.
