# Per-Feature Model Tiers: `models.yaml` Format

Belmont assigns each sub-agent (codebase, design, implementation, verification, code-review, reconciliation) to a tier — `low`, `medium`, or `high` — which is mapped to a concrete model ID for whichever AI CLI runs the work (Claude Code, Codex, Gemini, Cursor, Copilot, opencode).

Tiers are stored per feature in `.belmont/features/<slug>/models.yaml`. The tech-plan skill writes this file after assessing the feature's effort profile; the Belmont Go CLI reads it when spawning each phase, and the orchestrator skills (implement, verify, code-review) read it when dispatching sub-agents on Claude Code.

## Schema

```yaml
# Generated during /belmont:tech-plan. Safe to hand-edit.
profile: frontend-heavy
planning: high
tiers:
  codebase: medium
  design: high
  implementation: high
  verification: high
  code-review: medium
  reconciliation: high
```

### Fields

- **`profile`** *(string, free-form)* — short label describing the feature's character. Used for human context; not parsed by Belmont beyond display. Common values: `frontend-heavy`, `backend-heavy`, `fullstack`, `infra`, `docs`, `research`, `refactor`. You can invent a better label if none fits.

- **`planning`** *(string, usually `high`)* — records the tier used for product-plan and tech-plan. This is always `high` in practice because planning produces the spec every downstream agent executes against. Editing this value has no runtime effect (the Go CLI hardcodes `planningTier = "high"`); it exists as a permanent record of the decision.

- **`tiers`** *(map of agent → tier)* — the core config. Each agent gets one of `low`, `medium`, or `high`. Unknown agents are ignored; missing agents inherit the session model (Belmont agent files pin no model). Recognized agents:
  - `codebase` — exploration / pattern scanning
  - `design` — Figma extraction and token mapping where a design exists; **deriving per-task specs against the feature's Design Contract** where the feature has a UI but no Figma. The second mode is real design work, not a null pass — do not read "no Figma" as "no design agent"
  - `implementation` — code generation, acceptance validation
  - `verification` — test runs, visual diff, acceptance checking
  - `code-review` — diff review, lint / pattern validation
  - `reconciliation` — merge-conflict semantic resolution (applied at merge-time, not per-milestone)

## Tier → Model mapping

The Belmont Go CLI maps each tier to a CLI-specific model ID. See the `modelTiers` map in the Belmont CLI source for the canonical table (`grep -rn "modelTiers" cmd/belmont/`). Current mapping (check the Go source for latest):

| Tool    | low                        | medium                | high                |
|---------|----------------------------|-----------------------|---------------------|
| claude  | haiku                      | sonnet                | opus                |
| codex   | gpt-5.4-mini               | gpt-5.4               | gpt-5.5             |
| gemini  | gemini-2.5-flash-lite      | gemini-2.5-flash      | gemini-2.5-pro      |
| cursor  | sonnet-4                   | sonnet-4-thinking     | gpt-5               |
| copilot | haiku-4.5                  | claude-sonnet-4.5     | gpt-5.4             |
| opencode | anthropic/claude-haiku-4-5 | anthropic/claude-sonnet-4-6 | anthropic/claude-opus-4-8 |

Tiers are stable; model IDs get bumped in the Go registry when tools ship new versions. Codex users can override per tier via `codex.tiers.<tier>.model`, `codex.tiers.<tier>.reasoning_effort`, and optional `codex.tiers.<tier>.service_tier` in `~/.belmont/local-llms.json` (or the corresponding `BELMONT_CODEX_*_<TIER>` env vars). opencode defaults assume the Anthropic provider — users on another provider override per tier via `opencode.tiers.<tier>` in `~/.belmont/local-llms.json` (or `BELMONT_OPENCODE_MODEL_<TIER>` env vars).

## Tier economics: errors cost more than tokens

When assigning tiers, optimize **end-to-end cost, not per-token price**. A failed milestone re-enters the implement → verify → review (→ debug) pipeline, and every retry reloads the full feature context across multiple agents — one avoided rework cycle pays for a lot of premium-tier tokens. Real-project telemetry (model-attributed commit history across large Belmont projects, mid-2026) measured ~1.5× the rework-commit ratio when the mid tier handled implementation versus the high tier, with multi-round fix chains where the mid tier's per-token discount was clearly underwater.

**Claude-specific note (as of June 2026)**: in reasoning mode the mid tier's discount largely evaporates — independent benchmarks (Artificial Analysis) measured Sonnet 4.6 at ~90% of Opus 4.8's end-to-end evaluation cost for materially lower intelligence (52 vs 61) and slower output (45 vs 64 tok/s). On Claude, the useful axis is therefore `high` vs `low`: choose `high` wherever an error cascades (implementation, codebase analysis, verification, reconciliation) and `low` (Haiku — genuinely cheap and fast) for mechanical, low-blast-radius work. `medium` remains a healthy middle tier on other CLIs (e.g. gpt-5.4, gemini-2.5-flash). Re-check current benchmarks before treating this note as permanent.

## Starting-point examples (non-definitive)

These are **illustrative heuristics only** — the planning model is expected to reason about the specific feature at hand, not pattern-match to a profile label.

- **frontend-heavy** (rich interactive UI, lots of visual/Figma work): design=high, implementation=high, verification=high, codebase=high, code-review=medium, reconciliation=high.
- **backend-heavy** (APIs, data-layer, migrations): design=low **only when the feature genuinely has no user interface** — check the TECH_PLAN's `## Design Contract` `**Mode**`, and if it reads `derived — UI, no Figma` this is not a backend-heavy feature for design purposes. implementation=high, verification=high (a false pass costs a full debug loop later), codebase=high, code-review=medium, reconciliation=high.
- **infra** (config, CI, deployment, pipelines): implementation=high (config errors ship silently and surface as outages, not test failures), verification=high, codebase=medium, design=low, code-review=medium, reconciliation=high.

> **Never set `design=low` because a feature has no Figma.** "No Figma" and "no UI" are different states. A feature whose contract `**Mode**` is `derived — UI, no Figma` gives the design agent a full derivation job — reading the contract, picking values off declared scales, enumerating every component state — and gives verification an objective standard to measure. `design=low` there produces exactly the sloppy per-task specs the contract exists to prevent. `design=low` is right only for `**Mode**: N/A — no UI`.
- **docs** (content-only changes, README refreshes, ADRs): everything low, reconciliation=medium.
- **refactor** (no behavior change, lots of code movement): implementation=high (reasoning about preservation), verification=high, reconciliation=high (merge conflicts likely).
- **research** (exploration, prototyping): codebase=high (pattern inference), implementation=low (throwaway code), verification=low.

Again, these are loose anchors. A "frontend-heavy" feature that's just restyling one button probably warrants all-`low` except reconciliation. A "docs" feature that rewrites the entire ADR catalog probably warrants `high` implementation. Reason about the specific work — and when torn between two tiers for an agent whose mistakes trigger pipeline re-runs, take the higher one.

## Fallback behavior

- If `models.yaml` is absent, Belmont agent files pin no model, so each sub-agent inherits the **session model** — the model the orchestrator (interactive Claude Code session, or the headless `claude -p` process in auto mode) is running on. Run Belmont on a strong model and the whole pipeline follows; use `models.yaml` to pin specific tiers per agent. **Exception**: in auto mode the Go CLI forces the high tier for planning and reconciliation regardless (`planningTier` / `reconciliationDefaultTier`), since both are high-blast-radius and not per-milestone.
- If `models.yaml` exists but omits an agent, that agent inherits the session model.
- If a tier value is invalid (`extreme`, typos, etc.), the runtime omits `--model` and the tool uses its own default model.
- The user can accept Belmont defaults explicitly during tech-plan — in that case the skill does NOT create `models.yaml`, and every agent falls through to the session model.

## Editing by hand

The file is plain YAML with a deliberately flat schema. Editing by hand is supported and intentional — if the tech-plan's recommendation turns out to be wrong mid-implementation, just edit the file and re-run `belmont auto` (or restart your manual session). The Go parser ignores unknown keys and empty values, so comments and extra fields won't break it.
