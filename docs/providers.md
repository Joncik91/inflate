# Provider configuration

Inflate uses one LLM per launch (the one in `~/.config/inflate/config.toml`). You can switch interactively:

- **From inside the TUI:** press `?` then `p` to cycle between your saved provider and any installed local Ollama model.
- **From the shell:** `inflate config provider` re-runs the provider step of the wizard.
- **By editing TOML:** `inflate config edit` opens `config.toml` in `$EDITOR`.

## API key resolution order

1. **Real shell env var** (e.g. `DEEPSEEK_API_KEY` exported in your shell). CI uses this.
2. **`~/.config/inflate/.env`** — what the wizard writes, mode `0600`.
3. **Inline `provider.api_key`** in `config.toml` — explicit override.

If none resolves, inflate prints a clear error and points at `inflate doctor`.

---

## Anthropic

```toml
[provider]
kind        = "anthropic"
model       = "claude-haiku-4-5"
api_key_env = "ANTHROPIC_API_KEY"
```

Recommended models: `claude-haiku-4-5` (fastest, cheapest), `claude-sonnet-5` (balanced), `claude-opus-5` (most capable).

## DeepSeek / Groq / OpenRouter / Together / vLLM (OpenAI-compatible)

Anything that speaks OpenAI's `/v1/chat/completions` works under `kind = "openai_compat"`:

```toml
[provider]
kind        = "openai_compat"
base_url    = "https://api.deepseek.com/v1"
model       = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"
```

For OpenAI itself, swap `base_url` to `https://api.openai.com/v1`. For Groq, `https://api.groq.com/openai/v1`. For OpenRouter, `https://openrouter.ai/api/v1`. The model identifier follows whichever vendor you're hitting.

## Google Gemini

```toml
[provider]
kind        = "google"
model       = "gemini-2.0-flash"
api_key_env = "GOOGLE_API_KEY"
```

## Local Ollama

```toml
[provider]
kind     = "ollama"
model    = "gemma4:26b"
base_url = "http://localhost:11434"  # default — omit for local
```

**No API key required.** Inflate verifies the model is pulled at startup and points you at `ollama pull <model>` if it's missing.

The first-run wizard auto-detects a running Ollama and offers it as the first provider option, listing installed chat-capable models with their parameter sizes.

### Ollama-specific behavior

Inflate uses Ollama's native `/api/chat` endpoint (not the OpenAI-compat shim) because it needs three things the shim doesn't expose:

- **`think: false`** — disables reasoning mode. Without this, reasoning models (qwen3.6, gemma4, DeepSeek-R1 distills) burn the entire output budget on hidden reasoning tokens and produce empty responses.
- **`num_ctx: 16384`** — Ollama's runtime default is **4096** regardless of what the model supports. Inflate's full prompt (system rules + harvested context blocks) routinely exceeds 4096; without `num_ctx` the model silently drops the trailing skeleton rules and produces malformed output.
- **`num_predict` scaled up** — Promptism's 5-section output needs ~1500 tokens of headroom. Cloud models rarely hit caps; local always does. Inflate floors `num_predict` at 2000 for Ollama.

If you want to use llama.cpp's HTTP server or LocalAI directly, use `kind = "openai_compat"` against their endpoints.

### Switching between local models live

Once configured, press `?` then `p` from the TUI to cycle through providers:

```
DeepSeek (your saved cloud) → ollama:gemma4:26b → ollama:qwen3.6:35b → wraps back
```

The cycle starts with whatever was in `config.toml` at boot, then walks each chat-capable Ollama model the daemon advertises (smallest parameter count first, since smaller models load faster on iGPUs). Each cycle persists to `config.toml` — quit and relaunch to pick a new "anchor."

## Troubleshooting per provider

**Cloud — "API key resolves" `[✗]` in `inflate doctor`:** the env var isn't set or `.env` is empty. Run `inflate config edit env` and paste the key.

**Cloud — "provider ping" `[✗]` with a 401:** key is invalid. Check the dashboard.

**Cloud — "provider ping" `[✗]` with a 429:** rate limited. Wait or upgrade.

**Ollama — "Ollama not reachable on http://localhost:11434":** start the daemon with `ollama serve`.

**Ollama — "model 'X' not pulled":** literally what it says — `ollama pull X`.

**Ollama — output cuts off mid-sentence:** the prompt + output exceeded `num_ctx`. Inflate ships with `num_ctx=16384` which is plenty for any realistic inflation, but if you've got a *very* long Claude session feeding the harvester, you can bump it by editing `internal/provider/ollama.go` and rebuilding. (PR welcome to make this a config field.)

**Ollama — `[…cut off — increase num_predict in config or rephrase the seed]`** appears at the end of the preview: the model hit `num_predict` before finishing. Either rephrase the seed to something tighter, or pull a smaller model (less to say, less likely to hit the cap).
