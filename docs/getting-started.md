# Getting started

This walks you from `go install` to your first inflation in about 5 minutes.

## Install

Inflate is a single Go binary. Two ways to get it:

**Via `go install`** (requires Go 1.22+):

```bash
go install github.com/Joncik91/inflate@latest
```

The binary lands at `$GOPATH/bin/inflate` (or `$HOME/go/bin/inflate` if `GOPATH` is unset). Make sure that directory is on your `PATH`.

**From a release tarball:** download from the [releases page](https://github.com/Joncik91/inflate/releases) and drop the binary somewhere on your `PATH`.

### Linux clipboard prerequisite

Inflate copies the inflated prompt to your clipboard. On Linux, that needs `xclip` (X11) or `xsel`:

```bash
sudo apt install xclip      # Debian/Ubuntu
sudo dnf install xclip      # Fedora
```

`inflate doctor` will tell you if either is missing.

## First launch — the wizard

```bash
inflate
```

The first launch is a 5-question setup wizard:

1. **Who are you?** *e.g. "senior backend engineer, mostly Go and Python"*
2. **What kind of work?** *e.g. "API services, CLI tools"*
3. **Style preference?** `terse` / `standard` / `verbose`
4. **Which LLM provider?** Pick one. The wizard auto-detects a local Ollama and offers it first if it's running.
5. **API key** (skipped for Ollama, since it's keyless).

The wizard writes:

- `~/.config/inflate/profile.toml` — your identity and style
- `~/.config/inflate/config.toml` — provider settings (no secrets)
- `~/.config/inflate/.env` — your API key, mode `0600`

You don't need to source anything in your shell. Inflate reads `.env` itself on every launch. Real shell env vars still override `.env`, so CI works.

To rotate the key later:

```bash
inflate config edit env       # edit the .env in $EDITOR
inflate config provider       # re-run the provider step of the wizard
```

## The two-terminal setup

Inflate lives alongside Claude Code, not inside it.

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

Open `claude` in one terminal. Open `inflate` in another. Type a fragment in inflate, press Enter, paste into Claude Code.

## Your first inflation

In the inflate TUI:

1. Type a fragment — anything from `what's next?` to `fix the bug from the last test run` to `add a /healthz endpoint`.
2. Wait 600ms (or press `Tab` to skip the wait). Inflate reads your harvested context and asks the LLM to expand the fragment into a structured prompt.
3. The expanded prompt streams into the preview pane. You'll see Role / Context / Task / Constraints / Output sections.
4. Press `Enter` to copy to clipboard.
5. Paste into Claude Code (Terminal A).

Press `?` any time to see the full keybinding cheatsheet.

## Troubleshooting

If anything doesn't work:

```bash
inflate doctor
```

That runs every startup check (config readable, key resolves, provider ping, `xclip` present, harvester collectors, JSONL detection) and tells you which step failed.

Most issues fall into:

- **"no JSONL detected"** — Claude Code hasn't been launched in this directory yet, or `claude_projects_dir` is set wrong. Open `claude` once, then re-run `inflate`.
- **"API key resolves" `[✗]`** — your env var isn't set or `.env` is empty. Run `inflate config edit env`.
- **"provider ping" `[✗]`** — your network or API key is bad. The error tells you which.
- **"xclip not installed"** — install it (Linux only).

## What's next

- [Configure additional providers](providers.md) — DeepSeek, OpenAI-compatible, Google, local Ollama.
- [Learn every keybinding](keybindings.md).
- [Understand how it works](architecture.md).
