# Inflate

*Type a fragment. Get a context-loaded prompt for Claude Code.*

Inflate sits next to Claude Code and turns whatever you'd type — a fragment, a question, a half-formed thought — into a structured prompt loaded with your project's actual context. Works mid-session or from a cold start. Output lands on your clipboard, ready to paste.

https://github.com/user-attachments/assets/6702f681-01e7-460a-9524-2c5bc97ca9dd

## Install

```bash
go install github.com/Joncik91/inflate@latest
```

## Quickstart

```bash
inflate              # first launch walks through a 5-question wizard
```

Then in two terminals:

```bash
# Terminal A                    # Terminal B
claude                          inflate
# write code                    # type "what's next?", Enter
                                # paste into Terminal A
```

That's it. [Full getting-started guide →](docs/getting-started.md)

## Highlights

- **Real context, not pasted boilerplate.** Reads your `git status`, shell history, open editor file, recent Claude Code session, and running dev tools in parallel.
- **Bring your own model.** Anthropic, DeepSeek, OpenAI-compatible, Google Gemini, or local Ollama. [Provider setup →](docs/providers.md)
- **Switch providers live.** `?` then `p` cycles between cloud and any installed local Ollama model. No restart.
- **Streams the inflated prompt** as it generates. Errors sit in a banner until you ack them — no flashed-by 1.5s toasts.
- **Local-LLM friendly.** Native `/api/chat`, `think:false` for reasoning models, `num_ctx` opt-in to avoid silent prompt truncation.

## Documentation

- [Getting started](docs/getting-started.md) — install, wizard, first inflation.
- [Provider configuration](docs/providers.md) — Anthropic, DeepSeek, OpenAI-compatible, Google, Ollama.
- [Keybindings & subcommands](docs/keybindings.md) — every key the TUI listens for, every CLI subcommand.
- [Architecture](docs/architecture.md) — how the harvester, inflater, and TUI fit together.
- [Design spec](docs/superpowers/specs/2026-04-27-inflate-design.md) — full design + canonical deferred list.

## Status

v0.1.4 — native Ollama provider, in-TUI provider switcher, downstream-assistant prompt framing, plus correctness fixes for slow local-model streaming. Built on v0.1.3 / v0.1.2 / v0.1.1 / v0. PTY wrapper for watch-as-you-type still deferred.
