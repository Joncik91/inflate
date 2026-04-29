# Keybindings & subcommands

## TUI keys

| Key | Action |
|---|---|
| Type any character | Append to the seed. After 600ms idle, inflation kicks off. |
| `Tab` | Force inflate now — skip the 600ms idle wait. |
| `Enter` | Copy the inflated preview to your clipboard. |
| `Esc` | If an error banner is showing: dismiss it. Otherwise: clear seed + preview. |
| `?` | Toggle the help overlay (only when seed is empty, so you can still type `?` mid-question). |
| `p` *(in help overlay only)* | Cycle through providers — your saved one + each Ollama model the daemon advertises. |
| `Ctrl-C` | Quit. Cancels any in-flight inflation cleanly. |

The help overlay (`?`) shows the same table inside the TUI in case you forget. Esc closes it.

## TUI behavior notes

- **The 600ms idle timer** is what triggers automatic inflation while you type. Press `Tab` if you don't want to wait.
- **Streaming preview**: the inflated prompt fills the preview pane chunk-by-chunk as the LLM generates it. While waiting for the first token, a `⠹ inflating…` spinner runs below.
- **Error banner**: errors stay in a red banner until you type a character or press Esc. They don't flash by as toasts (those are reserved for confirmations like "copied ✓").
- **Status line**: shows what context is being used (`Using profile, git, shell, open editor file`) and which provider is active (`ollama:gemma4:26b`). Color reflects health: green when git is present, yellow when partial, red when only profile is usable.

## CLI subcommands

| Command | Does |
|---|---|
| `inflate` | Launch the TUI. The default. |
| `inflate doctor` | Run every startup check (config readable, key resolves, provider ping, `xclip` present, harvester collectors). Exits 0 if all pass, 1 otherwise. Use this first when something's wrong. |
| `inflate config` | Open `config.toml` in `$EDITOR` (defaults to `vi`). |
| `inflate config profile` | Open `profile.toml` in `$EDITOR`. |
| `inflate config env` | Open `.env` in `$EDITOR` — for rotating keys or adding a second provider's env var. |
| `inflate config provider` | Re-run the provider step of the first-run wizard. Used to switch backends after first-run (e.g. DeepSeek → local Ollama) without hand-editing TOML. |

## Top-level flags

| Flag | Default | What |
|---|---|---|
| `--cwd PATH` | nearest `.git` ancestor of `$PWD`, else `$PWD` | Project dir to harvest from. Use this if you launch inflate from outside the repo. |
| `--paste-window N` | `0` (focused) | X11 window ID to auto-paste into (Linux only). With `auto_paste = true` in config, the inflated prompt is also typed into this window. |
| `--force` | `false` | Override stale-lockfile detection. Use only if `inflate doctor` says a stale lockfile is blocking startup. |

## Environment variables

- `INFLATE_TRACE=1` — enable verbose tracing to `/tmp/inflate-trace.log`. Used to diagnose mid-stream cutoffs against slow local models. Off by default. (Currently a no-op in shipped binaries; the trace facility is added when needed during development.)
- `EDITOR` — used by `inflate config edit` and friends. Falls back to `vi`.
- `XDG_CONFIG_HOME` — overrides `~/.config` for inflate's config dir.
- `HOME` — used to locate `~/.cache/inflate` for logs and `~/.claude/projects` for JSONL session files.

## Config files at a glance

All under `~/.config/inflate/`:

| File | What's in it | Mode |
|---|---|---|
| `profile.toml` | identity, work kind, style preference | 644 |
| `config.toml` | provider settings, auto_paste, optional `claude_projects_dir` / `claude_sessions_dir` overrides | 644 |
| `.env` | API keys, one `KEY=value` per line | 600 |
| `run.lock` | active-process lockfile (auto-cleaned) | 644 |
