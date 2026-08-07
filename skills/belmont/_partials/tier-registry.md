### Model Tier Registry

Belmont uses three user-facing tiers — `low`, `medium`, `high` — which map to concrete model identifiers per AI CLI. When you need to pass a model override explicitly (see `dispatch-strategy.md` Model Tier Overrides or `tier-preflight.md`), translate via this table.

| Tier   | Claude  | Codex          | Gemini                | Cursor             | Copilot              | Pi                   | opencode                     |
|--------|---------|----------------|-----------------------|--------------------|----------------------|----------------------|------------------------------|
| low    | haiku   | gpt-5.4-mini   | gemini-2.5-flash-lite | sonnet-4           | haiku-4.5            | user-configured¹     | anthropic/claude-haiku-4-5²  |
| medium | sonnet  | gpt-5.4        | gemini-2.5-flash      | sonnet-4-thinking  | claude-sonnet-4.5    | user-configured¹     | anthropic/claude-sonnet-4-6² |
| high   | opus    | gpt-5.5        | gemini-2.5-pro        | gpt-5              | gpt-5.4              | user-configured¹     | anthropic/claude-opus-4-8²   |

¹ Pi runs against user-provided local (or remote) models whose IDs Belmont cannot know in advance. The user maps tiers → providers + models in `~/.belmont/local-llms.json` (or per-project `.belmont/local-llms.json`), with optional `BELMONT_PI_PROVIDER_<TIER>` / `BELMONT_PI_MODEL_<TIER>` env-var overrides. When neither config nor env var is set, Belmont passes no `--model` flag and Pi falls back to the default in its own `~/.pi/agent/models.json`. See `docs/supported-tools.md` and `docs/local-llms.example.json`.

² opencode model IDs are `provider/model` tokens; the defaults assume the Anthropic provider. Users on another provider (opencode zen, OpenAI, local models, …) override per tier via `opencode.tiers.<tier>` in `~/.belmont/local-llms.json` / `.belmont/local-llms.json`, or `BELMONT_OPENCODE_MODEL_<TIER>` / `BELMONT_OPENCODE_MODEL` env vars. Codex users can similarly override `codex.tiers.<tier>.model`, `codex.tiers.<tier>.reasoning_effort`, optional `codex.tiers.<tier>.service_tier`, or the corresponding `BELMONT_CODEX_*` env vars. See `docs/supported-tools.md` and `docs/local-llms.example.json`.

The canonical source for the closed-model tiers (Claude / Codex / Gemini / Cursor / Copilot / opencode) is the `modelTiers` map in the Belmont CLI source (`grep -rn "modelTiers" cmd/belmont/`). If this table drifts from the Go registry, the Go registry wins — file an issue and update this partial. `scripts/generate-skills.sh --check` is the place to add a drift guard.
