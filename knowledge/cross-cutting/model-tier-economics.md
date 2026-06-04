# Model tier economics — defaults optimize first-pass correctness, not per-token price

Domains: agents, skills, cli, auto-mode

**Why this matters.** Belmont's pipeline amplifies model error rates into token cost: a failed milestone re-enters implement → verify → review (→ debug), and every retry reloads the full feature context (PRD/PROGRESS/NOTES can run to hundreds of KB) across 4–5 agents. The marginal cost of a smarter model on the first pass is small compared to the cost of one extra pipeline re-run. Model-attributed commit history on large Belmont projects (mid-2026) measured ~1.5× the rework-commit ratio when the mid tier handled implementation versus the high tier — with fix chains reaching FIX-10+ on individual tasks — while independent benchmarks (Artificial Analysis, June 2026) put the mid Claude tier at ~90% of the high tier's end-to-end reasoning-mode cost for materially lower intelligence and slower output. The mid tier's per-token discount does not survive contact with the pipeline.

**Invariant.** Agent frontmatter defaults (`agents/belmont/*.md`) are the no-`models.yaml` fallback and stay at the high tier (`model: opus`) for all six agents. Cost reduction happens through (a) deliberate per-feature downgrades in `models.yaml` — primarily to `low` for mechanical, low-blast-radius work — and (b) input-side trimming (MILESTONE architecture, progressive-disclosure references, `/belmont:cleanup`, optional RTK proxy). Never through silently lowering frontmatter defaults.

**How it's enforced.** Frontmatter in `agents/belmont/*.md`; documented fallback chain in `skills/belmont/_src/references/models-yaml-format.md` (Tier economics section) and `_partials/dispatch-strategy.md`; tech-plan heuristics steer per-feature tier choices with the cascade rule ("when torn between two tiers for an agent whose mistakes trigger pipeline re-runs, take the higher one").

**Failure mode if you break it.** Commit `a4fe0ea` downgraded `implementation-agent.md` to `model: sonnet` as a side effect of introducing per-feature tiers, while README and `docs/agent-pipeline.md` continued to document Opus. Every project without a `models.yaml` silently ran implementation on the mid tier; on one large project the monthly rework-commit ratio rose from ~20% to ~32% in the month the mid tier became the dominant implementer, costing an estimated 35–40 extra full-pipeline re-runs in a quarter — more than the per-token discount returned.

**Don't re-do.**
- *"Default to the mid tier, it's 40% cheaper per token"* — rejected. In reasoning mode the end-to-end discount is ~10%, and elevated rework makes it net-negative. Benchmarks and per-model rework ratios both support this (June 2026; re-measure before reversing).
- *"Make defaults `inherit` so agents follow the session model"* — rejected. Auto mode's headless sessions may run at any tier per phase; `inherit` would let a low-tier session silently drag implementation down with it. Explicit frontmatter keeps the fallback deterministic in both invocation paths.
- *"Encode the economics in Go (`modelTiers`)"* — rejected. The registry maps tiers → model IDs; which tier an agent *gets* is policy, and policy lives in frontmatter + `models.yaml` where users can see and override it.

**Evidence.** Artificial Analysis Intelligence Index (June 2026): Opus 4.8 score 61.4 / $4,686 eval cost / 63.7 tok/s vs Sonnet 4.6 (reasoning) 52 / $4,206 / 45.4 tok/s. Model-attributed rework analysis of two production Belmont projects (2,325 and 50+468-archived-milestone commit histories): per-model rework ratios 29.2% (Sonnet) vs 19.7% (Opus); fix-follows-feature clustering 21.6% vs 14.4%; projects that had already hand-set `implementation: high` in every substantive `models.yaml` showed no model-attributable rework elevation.

**Revisions.**
- 2026-06-04 — entry created; frontmatter defaults flipped to `model: opus` for all six agents, restoring the documented pre-`a4fe0ea` implementation default and extending it pipeline-wide.
