# Master Tech Plan: Belmont, self-hosted

*Kind B conventions for `~/repos/belmont` — Belmont running Belmont on itself.*

Created 2026-08-18 during `/belmont:review-plans`, when the `throughput` feature moved here.

## Authority

This is a **thin** document. It carries only the rules an agent executing a milestone *in this repository* must know and cannot get from anywhere else here.

- **Cross-cutting product and framework decisions live at `~/belmont/.belmont/`** (PRD, PROGRESS, TECH_PLAN). That workspace is the authority; this file is a pointer plus the six rules below. If the two ever disagree, the workspace wins and this file is wrong.
- **Engineering conventions live in this repo's own `AGENTS.md` and `CONTRIBUTING.md`**, and the self-learning loop in `knowledge/KNOWLEDGE.md` — consult it first, per `AGENTS.md:5`.
- Deletable in one commit once `throughput` merges. Do not grow it into a second copy of the workspace master.

## The rules that make self-hosting safe

**1. The orchestrating binary is pinned — and as of 2026-08-19 it is pinned to `belmont-pre`, not Homebrew.** The loop runs a **frozen** build; Go changes made inside a milestone take effect only on an explicit rebuild, and that is the isolation stopping a milestone from changing the loop running it.

*Amended 2026-08-19 during `throughput`'s tech-planning.* The frozen build is `~/bin/belmont-pre`, built from `throughput`'s M1 merge commit, replacing Homebrew v0.11.0 (`/opt/homebrew/bin/belmont`). Isolation is identical — both are frozen — but v0.11.0 contains no instrumentation, so pinning to it meant the feature whose entire purpose is measuring Belmont recorded **nothing** across its own twelve remaining milestones. The same artefact is also the daily driver in the five product repositories, which is what opens the passive observation arm: there is one `belmont` on PATH and both roles want the same frozen build. Do not refresh it until the M11 rollout. **Reverting is one command** — put Homebrew back on PATH; the cost is losing the passive arm, not the run.

**2. Do not run `belmont install` between M1 and M11** — not here, and not at user level. Skill prose resolves from `~/.agents/skills/belmont/` (refreshed to v0.11.0 on 2026-08-18, as prerequisite 0; it had been sitting at v0.10.x since 2026-05-04 and would have served 3½-month-old prose into every worktree). `.claude/` is gitignored here, so a fresh worktree has no project-local commands and that user-level install is the *only* resolver of `/belmont:*` inside one.

**3. Model tiers are a barbell — `high` or `low`, never `medium`.** Opus is the default; Haiku for genuinely mechanical work; the Sonnet/medium tier is skipped for real coding work. Basis: measured rework ~37% (Sonnet 4.6) against ~19% (Opus 4.7) — a milestone rebuilt, re-verified and re-fixed costs far more than the tier difference. Per-feature overrides in `<feature>/models.yaml`; reconciliation and planning are always `high`. Cost is reduced through input structure, never by downgrading the model that determines rework.

**4. Zero external Go dependencies, and no `go.sum`.** This is a property of the product, not an accident. Score every candidate change against it — it is what disqualified the code-graph index during `throughput`'s planning, and why the new project config is stdlib JSON rather than YAML.

**5. Contract-changing work gets a proposal first.** `docs/proposals/NNNN-*.md`, specification before implementation, adversarial review rounds. **Note the current state, verified 2026-08-18: there is no `docs/proposals/` directory on `main`.** 0003–0006 live only on `origin/docs/pr-proposals`; 0001 and 0002 on `origin/proposal/0001-quick-mode` and `origin/proposal/0002-prd-hygiene`. `throughput`'s M2 takes `0004-context-budget-with-evidence.md` rev 5 as its specification and the feature owes 0007–0009, so **P0-13's branch triage must return a verdict on `origin/docs/pr-proposals` before M2 begins.**

**6. Tier 1 evals cannot license a prose change.** `AGENTS.md:59`. Nothing in Tier 1 reads a `SKILL.md`. Any milestone whose deliverable is skill prose needs Tier 2 — `BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont`, where `-timeout 0` is required or the live tool child is orphaned. See the feature TECH_PLAN's §Commands for which milestones that covers.

**7. No product's confidential information enters this repository.** Added 2026-08-19. `github.com/bigbenjoman/belmont` is **public** and a fork of `blake-simpson/belmont`, and `.belmont/` is committed here as the state of record — so anything written into a feature directory is published. Product feature names, roadmaps and per-feature measurements appear here only as **opaque identifiers**; the identifier↔product mapping lives at `~/belmont/`, never here. This binds every artefact: census output, baselines, per-run records, test fixtures, code comments and knowledge entries alike.

*Measured, not hypothetical.* On 2026-08-19 `origin/main` was found to carry a complete inventory of 82 feature slugs across all five products with per-feature sizes and task counts, plus product names in two *source* files that would travel in any upstream contribution. Remediation is a gate on `throughput`'s wave 2; the residual exposure — objects already pushed to a public fork stay reachable through the parent's network — was knowingly accepted by the repository owner rather than resolved by deleting the fork. See `features/throughput/TECH_PLAN.md` §Prerequisites — before wave 2.

## Ignore rules, and why self-hosting needs them

| Path | Reason |
|---|---|
| `.agents/` | `belmont install --source .` copies `agents/` and the generated `skills/` onto the `.agents/` surface. Verified byte-identical to in-repo source — committing it keeps two drifting copies of our own prose in the repo where that prose is the product. |
| `_research/` | Raw measurement notes behind a feature's plan (436 KB for `throughput`). Read by agents, never a deliverable. |
| `.claude/`, `.belmont/bin/`, `.belmont/cache/` | Pre-existing. Note that `.claude/` being ignored is what makes rule 2 load-bearing. |

`.belmont/` itself **is** committed — the feature register is the state of record.

## Concurrency

Run `belmont auto` at `--max-parallel 2` or `3`, not the default 5. Wave 2 of `throughput` holds seven milestones (M2, M3, M6, M7, M9, M10, M12 — M12 was added 2026-08-19), but the measured local speedup is ~1.4× — LLM decode is memory-bandwidth bound and concurrent agents share one budget. Overcommitting is measurably worse than running serially.

## Open question inherited from the workspace

**Where framework-feature state lives long-term.** `throughput` executes here while the workspace keeps the master catalogue, so that catalogue now spans two repositories. Workable for one feature; revisit before a second framework feature is planned. Recorded in `~/belmont/.belmont/TECH_PLAN.md` §Open architectural questions.

*Decided 2026-08-19 during `/belmont:review-plans`, so the next session finds a decision rather than an absence:* **this repository deliberately has no master `.belmont/PROGRESS.md`**, and therefore no features table. `belmont status` still enumerates `features/throughput/` from disk, so nothing in the single-feature path needs one; what a missing table costs is `belmont sync` (nothing to write) and `parseMasterDeps` (no Priority/Dependencies to read, so `belmont auto --features` / `--all` would schedule without them). With one feature both are free, and a second catalogue here is the two-drifting-copies failure this file exists to avoid. **Create it at either of two triggers, not before**: a second framework feature is planned here, or `belmont auto --all` is wanted in this repository.
