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

## Configure

```bash
mkdir -p ~/.config/inflate
cat > ~/.config/inflate/config.toml <<'EOF'
auto_paste = false

[provider]
kind        = "anthropic"           # or "openai_compat" or "google"
model       = "claude-haiku-4-5"
api_key_env = "ANTHROPIC_API_KEY"
EOF
export ANTHROPIC_API_KEY=sk-ant-...
```

For DeepSeek / Groq / OpenRouter / vLLM, use:

```toml
[provider]
kind        = "openai_compat"
base_url    = "https://api.deepseek.com/v1"
model       = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
```

## Run

In Terminal A: `claude`. In Terminal B: `inflate`. Type a fragment in B, press Enter, paste in A.

First run prompts you for three short questions to build your profile.

## Hotkeys

| Key | Action |
|---|---|
| Enter | inflate, copy to clipboard |
| Tab | force inflate now |
| Esc | clear input + preview |
| Ctrl-C | quit |

## Status

Pre-alpha. v0 single-shot inflation. Streaming-per-chunk TUI delivery and PTY wrapper land in v1.

See `docs/superpowers/specs/2026-04-27-inflate-design.md` for the full design.
