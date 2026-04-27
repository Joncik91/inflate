# Inflate

Type a fragment in Inflate (Terminal B), get a context-loaded prompt for Claude Code (Terminal A).

```
┌────────────────────┐         ┌────────────────────┐
│   Terminal A       │         │   Terminal B       │
│   $ claude         │         │   $ inflate        │
│                    │  reads  │                    │
│   writes JSONL ────┼────────►│  inflated preview  │
│                    │         │  + input box       │
└────────────────────┘         └─────────┬──────────┘
                                         │ on Enter
                                         ▼
                                   [clipboard]
                                         │
                                         ▼ paste
                                  Terminal A input
```

## Install

```bash
go install github.com/Joncik91/inflate@latest
```

## Run it

```bash
inflate
```

That's it. The first launch walks you through a 5-question setup wizard:

1. Who are you? (e.g. *senior backend engineer*)
2. What kind of work? (e.g. *CLI tools*)
3. Style preference? *terse / standard / verbose*
4. Which LLM provider? *anthropic / deepseek / openai / google / custom*
5. Paste your API key (input is hidden).

The wizard creates `~/.config/inflate/{profile,config}.toml` and `.env` (mode `0600`). Every future launch reads them automatically — **no shell sourcing required, no `~/.bashrc` edits**. Real shell env vars still override `.env`, so CI keeps working.

After setup: open `claude` in Terminal A, run `inflate` in Terminal B, type a fragment, press Enter, paste into Claude Code.

## Subcommands

| Command | Does |
|---|---|
| `inflate` | Launch the TUI (default) |
| `inflate doctor` | Run startup checks (config readable, key resolves, provider ping, xclip present, …). Exits 0 / 1 |
| `inflate config` | Edit `config.toml` in `$EDITOR` |
| `inflate config profile` | Edit `profile.toml` |
| `inflate config env` | Edit `.env` (rotate keys, add a second provider) |

## Hotkeys (TUI)

| Key | Action |
|---|---|
| Enter | inflate, copy to clipboard |
| Tab | force inflate now (skip the 600 ms idle wait) |
| Esc | clear input + preview |
| Ctrl-C | quit |

## Top-level flags

| Flag | Default | What |
|---|---|---|
| `--cwd PATH` | nearest `.git` ancestor of `$PWD`, else `$PWD` | Project dir to harvest |
| `--paste-window N` | `0` (focused) | X11 window ID to auto-paste into (Linux only) |
| `--force` | `false` | Override stale-lockfile detection |

## API key resolution order

When inflate starts, it looks for the key in this order:

1. **Real shell env var** (e.g. `DEEPSEEK_API_KEY`) — wins; CI uses this.
2. **`~/.config/inflate/.env`** — what the wizard writes.
3. **Inline `provider.api_key`** in `config.toml` — explicit override.

If none resolves, inflate prints a clear error and points at `inflate doctor`.

## Provider config examples

**Anthropic:**

```toml
auto_paste = false

[provider]
kind        = "anthropic"
model       = "claude-haiku-4-5"
api_key_env = "ANTHROPIC_API_KEY"
```

**DeepSeek / Groq / OpenRouter / Together / vLLM (anything OpenAI-compatible):**

```toml
[provider]
kind        = "openai_compat"
base_url    = "https://api.deepseek.com/v1"
model       = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
```

**Google Gemini:**

```toml
[provider]
kind        = "google"
model       = "gemini-2.0-flash"
api_key_env = "GOOGLE_API_KEY"
```

## Troubleshooting

```bash
inflate doctor
```

Lists every startup check with `[✓]` / `[✗]`. Most "inflate keeps asking for my key" / "no JSONL detected" / "lockfile stuck" cases are diagnosed here in one screen.

## Status

v0.1.1 — interactive setup, dotenv-backed keys, smart `--cwd`, lockfile self-cleanup, `doctor` + `config edit` subcommands.

Streaming-per-chunk TUI delivery and the PTY wrapper still land in v0.2.

See `docs/superpowers/specs/2026-04-27-inflate-design.md` for the full design.
