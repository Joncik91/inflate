# Inflate

*Type a fragment. Get a context-loaded prompt for Claude Code.*

Inflate sits next to Claude Code and turns whatever you'd type — a fragment, a question, a half-formed thought — into a structured prompt loaded with your project's actual context (git, terminal history, recent Claude session). Works mid-session or from a cold start. Output lands on your clipboard, ready to paste.

https://github.com/user-attachments/assets/ada67a3f-c1b4-48d6-a74d-354ca3d35a40

## Install

```bash
go install github.com/Joncik91/inflate@latest
```

## Quickstart

1. Run `inflate`. The first launch walks you through a 5-question setup wizard (identity, work kind, style, provider, API key). It writes `~/.config/inflate/{profile,config}.toml` and `.env` (mode `0600`) — no shell sourcing required.
2. Open `claude` in Terminal A. Run `inflate` in Terminal B.
3. Type a fragment, press Enter, paste into Claude Code.

## What it does

Inflate reads your project state in parallel — git diff, shell history, files open in your editor, the latest Claude Code JSONL session, your profile, running dev tools — then asks an LLM to expand your fragment into a Promptism-shaped prompt (Role / Context / Task / Constraints / Output) using only that context.

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

## Commands & hotkeys

| Command | Does |
|---|---|
| `inflate` | Launch the TUI (default) |
| `inflate doctor` | Run startup checks (config readable, key resolves, provider ping, xclip present, …). Exits 0 / 1 |
| `inflate config` | Edit `config.toml` in `$EDITOR` |
| `inflate config profile` | Edit `profile.toml` |
| `inflate config env` | Edit `.env` (rotate keys, add a second provider) |

| Key | Action |
|---|---|
| Enter | inflate, copy to clipboard |
| Tab | force inflate now (skip the 600 ms idle wait) |
| Esc | dismiss errors, then clear input + preview |
| `?` | toggle help overlay |
| Ctrl-C | quit |

The TUI streams the inflated prompt in as it arrives. While waiting for the first token a `⠹ Inflating…` spinner shows below the preview. Errors sit in a red banner until you type or press Esc — they don't flash by.

| Flag | Default | What |
|---|---|---|
| `--cwd PATH` | nearest `.git` ancestor of `$PWD`, else `$PWD` | Project dir to harvest |
| `--paste-window N` | `0` (focused) | X11 window ID to auto-paste into (Linux only) |
| `--force` | `false` | Override stale-lockfile detection |

## Provider config

Inflate resolves the API key in this order: real shell env var → `~/.config/inflate/.env` → inline `provider.api_key` in `config.toml`. CI just sets the env var.

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

v0.1.3 — streaming preview, `?` help overlay, persistent error banner, named-section rendering, deeper context (untracked files, dev-tools detection, recent-files fallback, neighbor-repo hints, auto-promotion to repo root), downstream-assistant prompt framing.

Built on v0.1.2 (session-aware JSONL picker), v0.1.1 (interactive setup, dotenv-backed keys, smart `--cwd`, lockfile self-cleanup, `doctor` + `config edit` subcommands), and v0 (TUI, BYOK, harvester, Promptism prompt skeleton).

PTY wrapper for watch-as-you-type still deferred.

## Learn more

See `docs/superpowers/specs/2026-04-27-inflate-design.md` for the full design and the canonical deferred list.
