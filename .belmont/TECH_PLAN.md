# Master Tech Plan: Belmont, self-hosted

*Kind B conventions for `~/repos/belmont` — Belmont running Belmont on itself.*

Created 2026-08-18 during `/belmont:review-plans`, when the `throughput` feature moved here.

## Authority

This is a **thin** document. It carries only the rules an agent executing a milestone *in this repository* must know and cannot get from anywhere else here.

- **Cross-cutting product and framework decisions live at `~/belmont/.belmont/`** (PRD, PROGRESS, TECH_PLAN). That workspace is the authority; this file is a pointer plus the six rules below. If the two ever disagree, the workspace wins and this file is wrong.
- **Engineering conventions live in this repo's own `AGENTS.md` and `CONTRIBUTING.md`**, and the self-learning loop in `knowledge/KNOWLEDGE.md` — consult it first, per `AGENTS.md:5`.
- Deletable in one commit once `throughput` merges. Do not grow it into a second copy of the workspace master.

## The rules that make self-hosting safe

**1. The orchestrating binary is pinned.** The loop runs the Homebrew build (`/opt/homebrew/bin/belmont`, v0.11.0). Go changes made inside a milestone take effect only on an explicit rebuild. This is the isolation that stops a milestone changing the loop that is running it.

**2. Do not run `belmont install` between M1 and M11** — not here, and not at user level. Skill prose resolves from `~/.agents/skills/belmont/` (refreshed to v0.11.0 on 2026-08-18, as prerequisite 0; it had been sitting at v0.10.x since 2026-05-04 and would have served 3½-month-old prose into every worktree). `.claude/` is gitignored here, so a fresh worktree has no project-local commands and that user-level install is the *only* resolver of `/belmont:*` inside one.

**3. Model tiers are a barbell — `high` or `low`, never `medium`.** Opus is the default; Haiku for genuinely mechanical work; the Sonnet/medium tier is skipped for real coding work. Basis: measured rework ~37% (Sonnet 4.6) against ~19% (Opus 4.7) — a milestone rebuilt, re-verified and re-fixed costs far more than the tier difference. Per-feature overrides in `<feature>/models.yaml`; reconciliation and planning are always `high`. Cost is reduced through input structure, never by downgrading the model that determines rework.

**4. Zero external Go dependencies, and no `go.sum`.** This is a property of the product, not an accident. Score every candidate change against it — it is what disqualified the code-graph index during `throughput`'s planning, and why the new project config is stdlib JSON rather than YAML.

**5. Contract-changing work gets a proposal first.** `docs/proposals/NNNN-*.md`, specification before implementation, adversarial review rounds. **Note the current state, verified 2026-08-18: there is no `docs/proposals/` directory on `main`.** 0003–0006 live only on `origin/docs/pr-proposals`; 0001 and 0002 on `origin/proposal/0001-quick-mode` and `origin/proposal/0002-prd-hygiene`. `throughput`'s M2 takes `0004-context-budget-with-evidence.md` rev 5 as its specification and the feature owes 0007–0009, so **P0-13's branch triage must return a verdict on `origin/docs/pr-proposals` before M2 begins.**

**6. Tier 1 evals cannot license a prose change.** `AGENTS.md:59`. Nothing in Tier 1 reads a `SKILL.md`. Any milestone whose deliverable is skill prose needs Tier 2 — `BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont`, where `-timeout 0` is required or the live tool child is orphaned. See the feature TECH_PLAN's §Commands for which milestones that covers.

## Ignore rules, and why self-hosting needs them

| Path | Reason |
|---|---|
| `.agents/` | `belmont install --source .` copies `agents/` and the generated `skills/` onto the `.agents/` surface. Verified byte-identical to in-repo source — committing it keeps two drifting copies of our own prose in the repo where that prose is the product. |
| `_research/` | Raw measurement notes behind a feature's plan (436 KB for `throughput`). Read by agents, never a deliverable. |
| `.claude/`, `.belmont/bin/`, `.belmont/cache/` | Pre-existing. Note that `.claude/` being ignored is what makes rule 2 load-bearing. |

`.belmont/` itself **is** committed — the feature register is the state of record.

## Concurrency

Run `belmont auto` at `--max-parallel 2` or `3`, not the default 5. Wave 2 of `throughput` holds six milestones, but the measured local speedup is ~1.4× — LLM decode is memory-bandwidth bound and concurrent agents share one budget. Overcommitting is measurably worse than running serially.

## Open question inherited from the workspace

**Where framework-feature state lives long-term.** `throughput` executes here while the workspace keeps the master catalogue, so that catalogue now spans two repositories. Workable for one feature; revisit before a second framework feature is planned. Recorded in `~/belmont/.belmont/TECH_PLAN.md` §Open architectural questions.
