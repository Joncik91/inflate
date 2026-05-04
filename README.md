<div align="center">

<img src="docs/logo.png" alt="Inflate" width="160" height="160">

# Inflate

[![Go 1.25+](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-E8954A.svg)](LICENSE)
[![BYO model](https://img.shields.io/badge/bring%20your%20own-model-E8954A)]()
[![Anthropic · DeepSeek · OpenAI · Gemini · Ollama](https://img.shields.io/badge/Anthropic%20·%20DeepSeek%20·%20OpenAI%20·%20Gemini%20·%20Ollama-supported-E8954A)]()
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-E8954A.svg)](#contributing)

***Type a fragment. Get a context-loaded prompt for Claude Code.***

Inflate sits next to Claude Code and turns whatever you'd type — a fragment, a question, a half-formed thought — into a structured prompt loaded with your project's actual context. Works mid-session or from a cold start. Output lands on your clipboard, ready to paste.

https://github.com/user-attachments/assets/6702f681-01e7-460a-9524-2c5bc97ca9dd

</div>

## Table of Contents

- [Install](#install)
- [Quickstart](#quickstart)
- [Highlights](#highlights)
- [Documentation](#documentation)
- [Security](#security)
- [Status](#status)
- [Contributing](#contributing)
- [License](#license)

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

## Security

Inflate harvests **everything that makes a useful prompt** — `git status`,
recent shell history, the file open in your editor, the current Claude Code
session transcript, and any running dev tools — then sends that bundle to the
LLM provider you've configured.

Two implications:

1. **Treat the harvested bundle as you would a code-review send-off.**
   It includes file paths, partial source, and any secrets that landed in
   shell history or untracked working-tree changes. Inflate masks none of
   this for you. If your codebase or shell history is sensitive, point
   inflate at a self-hosted endpoint (Ollama, an internal OpenAI-compatible
   gateway) rather than a third-party SaaS.
2. **Your provider's data-retention policy is the relevant one.** Inflate
   keeps no copies — it streams the inflated prompt to your clipboard and
   forgets — but the LLM endpoint you chose may log, train on, or retain
   the request. Configure accordingly.

The included Ollama provider runs the model entirely on your machine and is
the recommended default for proprietary work.

## Status

v0.1.4 — native Ollama provider, in-TUI provider switcher, downstream-assistant prompt framing, plus correctness fixes for slow local-model streaming. Built on v0.1.3 / v0.1.2 / v0.1.1 / v0. PTY wrapper for watch-as-you-type still deferred.

## Contributing

PRs welcome. The codebase is intentionally compact — Go stdlib + Bubbletea
TUI + a small set of provider adapters under `internal/`. Useful directions:

- **More providers.** The provider interface is small; adding Mistral,
  Cohere, or a new local runtime mostly means writing the streaming adapter.
  See [`docs/providers.md`](docs/providers.md) for the existing shape.
- **More harvesters.** Today inflate reads git, shell history, the open
  editor file, the recent Claude Code session, and running dev tools.
  Useful candidates: open browser tabs, the system journal, container
  state, recent test failures.
- **The deferred PTY wrapper** for watch-as-you-type. The design is in
  [`docs/superpowers/specs/2026-04-27-inflate-design.md`](docs/superpowers/specs/2026-04-27-inflate-design.md).

## License

MIT © Joncik91. See [LICENSE](LICENSE).
