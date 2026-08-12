# Milestone Immutability

**Domains**: skills, state, auto-mode

**Why this matters.** Agents left to their own devices will invent a "Polish / follow-ups from M<N>" milestone to hold deferred items discovered during implement or verify. That milestone declares `(depends: M<N>)`, which makes it a sibling of every later `M<N+i>` that also depends on M<N>. But its actual work mutates files those siblings depend on. Running them in parallel produces silent merge conflicts that only surface after the user reviews the final feature and sees something is wrong. The root cause of the about-2-dynamic-mode cascade was exactly this — an M5 polish milestone editing `hero-section.tsx` in parallel with M2.

## Invariant

Only `/belmont:tech-plan` may add, remove, rename, or re-parent a `### M<N>:` heading in PROGRESS.md. Every other skill (implement, verify, next, debug-auto, triage) may only edit tasks **inside** existing milestone headings.

**Milestone headings are level 3.** The partial said `## M<N>:` in two places (the issue reported four) while the parser, `validate`'s expected-structure output and every real file use `### M<n>:`. That is not cosmetic: a level-2 heading at column zero is exactly what *ends* the milestones region (`isSectionBreak`), so an agent following the partial literally would both fail to create a milestone and silently orphan every task after it — manufacturing more of the finding this entry's routing rules exist to answer. Fixed 2026-08-12 (issue #34).

**Exception**: interactive `/belmont:debug-manual` may edit spec prose (PRD/TECH_PLAN/NOTES/PROGRESS task text and follow-up `[x]` flips) in place under human-gated per-edit approval. The structural rules above (no new/renamed/removed milestones, no polish-pattern naming) still apply to `debug-manual`. See [cross-cutting/debug-spec-reconciliation.md](debug-spec-reconciliation.md) for the rationale and bounds.

Routing for discovered work:

- **Follow-up from M<N>'s own implement/verify cycle** → new `[ ]` task inside M<N>.
- **Follow-up blocked by work that will land in a later M<N+k>** → new `[!]` task inside M<N>, one-line reason naming M<N+k>. Reopens as `[ ]` when the blocker lifts.
- **Follow-up from a cross-cutting sweep, belonging to no single milestone** → new `[ ]` task inside the **highest-numbered existing milestone whose work it touches**; the last milestone in the plan if it is genuinely global. **Highest, not earliest.** The earliest milestone it touches is the intuitive answer and it is wrong: the fix edits files the later milestones already imported, so filing it there makes it a sibling racing its own downstream — the identical failure this entry bans a "polish from M<N>" milestone for. The honest dependency edge is the last milestone whose outputs the fix depends on.
- **Cosmetic / nice-to-have item the user may never want** → append to `NOTES.md` under `## Polish`. Not a milestone task.
- **Never a new milestone.** Not "M<last+1>: Polish", not "M<N>-FIX", not "MX: Deviations from M<N>", not "MY: Verification Fixes". Even if existing PROGRESS.md already contains such a milestone from a prior run, do not add to it and do not create siblings.

## How it's enforced

Three layers, each sufficient on its own but deployed together for defense in depth:

1. **Skill prose** — canonical text lives in `skills/belmont/_partials/milestone-immutability.md` and is `@include`d into `implement.md`, `verify.md`, `next.md`, `debug-auto.md`, `tech-plan.md`, and referenced by `prompts/belmont/post-verify-triage.md`. The partial is the single source of truth for those skills; skill bodies point to it rather than paraphrasing. `debug-manual.md` is the sole exception — it `@include`s `debug-scope-rules.md` instead, which permits in-place spec edits while keeping the structural prohibitions (no new/renamed milestones, no polish-pattern naming) intact.
2. **Runtime scope guard** — `runScopeGuard` in `cmd/belmont/types.go` reverts new milestone headings added during any non-`actionReplan` phase. See [auto-mode/scope-guard-runtime.md](../auto-mode/scope-guard-runtime.md).
3. **CLI lint** — `belmont validate` detects residual violations in PROGRESS.md (polish-pattern milestone names; cross-milestone task IDs like `P3-FWLUP-M2-1` sitting under a non-M2 milestone). Runs at `belmont auto` startup; interactive runs get `[y/N]` prompt, non-interactive abort.

## Failure mode if you break it

Without enforcement: a polish milestone declared `(depends: M1)` becomes a sibling of M2, M3, M4 which also depend on M1. All four run in parallel. The polish milestone mutates `hero-section.tsx` (which it considers M1's) while M2 is actively writing that file (because M2 owns the hero task). Merge picks one side arbitrarily. No error, no warning — only the final feature looking wrong.

With broken enforcement (one layer regressed but others intact): the regressed layer allows the violation; the next layer catches it. Stream shows `[SCOPE-GUARD] reverted 1 violation(s) — new milestone M5 "Polish / follow-ups…"`. Agent sees STEERING correction, adapts or escalates. Loud failure, cheap to diagnose. This is why the three layers compose.

## Don't re-do

- **Leave the cross-cutting case unruled because "it depends".** It was unruled until 2026-08-12 and the absence was not neutral — it was a loop. `belmont validate` / `belmont repair` say a task outside every milestone must move under a `### M<n>:` heading and escalate structural work to `/belmont:tech-plan`; `tech-plan` says follow-ups never get a new milestone; this partial says that rule supersedes everything else and then lists no destination for a follow-up belonging to no single milestone. Three correct instructions, followed in order, return you to the finding you started with, and the task stays counted by nothing. On the reporting project that was 40 tasks from one P1/P2 audit sweep — nine open, one flagged P1 and live in production, all invisible to scheduling. **Any executable ruling beats none**; the one chosen is above. See issue #34.
- **`implement.md:135-137` with the "create a new milestone with the next sequential number" permission.** This was the exact loophole that produced the M5 milestone in about-2. Removed; replaced with "always extend the current milestone." If you're tempted to add an "escape hatch" for cross-milestone work, the correct escape hatch is `[!]` with a reason, not a new milestone.
- **Allowing `triage`'s `defer_and_proceed` to create polish milestones.** The post-verify-triage prompt explicitly forbids this now. Deferral means NOTES.md or same-milestone `[!]`, never a new milestone.
- **Per-milestone PROGRESS fragment files** (`M1.md`, `M2.md`, …). Architecturally cleaner (scope violation becomes structurally impossible for checkbox flips). Rejected as a bigger refactor than skill-prose + runtime guard combined. Revisit only if the runtime guard proves fiddly; the two approaches are redundant-but-harmless if both adopted.
- **Pre-plan-time regex that blocks milestones with "Polish" / "Follow-ups" in the name.** Would false-positive on legitimate cross-cutting milestones like "M6: Accessibility audit across public routes." `belmont validate` uses targeted patterns (`polish`, `follow-ups`, `cleanup`, `verification fixes`, `deviations from M<N>`, `from M<N> implementation`, `fwlup(s)`) that match the anti-pattern without false-flagging real work.
- **Extending the spec-edit prohibition to interactive `/belmont:debug-manual`.** Considered. Rejected because debug-manual is interactive-only (auto invokes `/belmont:debug-auto`, never `debug-manual`), every spec edit is gated on explicit user approval, edits commit atomically with the code fix, and the auto-mode parallel-orchestration risk this entry guards against simply does not exist for synchronous human-in-the-loop sessions. The structural prohibitions (no new/renamed milestones, no polish-pattern naming) still apply to debug-manual. See [debug-spec-reconciliation.md](debug-spec-reconciliation.md).

## Evidence

`belmont-test/about-2-dynamic-mode` (the M5 spawn + cascade) vs `belmont-test/about-4-fresh` (clean M1–M4 run, zero milestones created mid-flight) in the canonical test repo. See [meta/validated-runs.md](../meta/validated-runs.md).

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestDetectViolations_PolishMilestoneNames`, `TestDetectViolations_CrossMilestoneTaskID`, `TestDiffScopeViolations_DetectsNewMilestone`.

## Known rough edges

- **Pre-existing bad milestones in a legacy PROGRESS.md.** If a feature was planned before the rule landed and contains an M5-polish milestone, skills will see it and may add to it. Agents are told to flag such milestones in their summary rather than migrate automatically. `belmont validate` detects these; the user runs `/belmont:tech-plan` to restructure.
- **`validate` regex may catch legitimate cross-cutting milestones** whose description happens to contain "cleanup" or "polish" in some innocuous way. Acceptable false-positive rate — the violation is easily waved past with the interactive `[y/N]` prompt. If false-positives become common, tighten the regex.

## Revisions

- 2026-04-21 — initial: canonical partial, `implement.md` loophole closed, tech-plan / verify / triage tightened, `belmont validate` lint added.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
- 2026-05-11 — `debug-manual` switched from `milestone-immutability.md` to `debug-scope-rules.md`; spec-text edits permitted in interactive sessions under per-edit user approval; structural prohibitions unchanged. See [debug-spec-reconciliation.md](debug-spec-reconciliation.md).
- 2026-08-07 — `cmd/belmont/main.go` split into 22 files in the same package; file paths in this entry repointed to their new homes. Symbol names are unchanged and remain the durable identifier.
- 2026-08-12 — ruled on the cross-cutting follow-up (issue #34): highest-numbered milestone it touches, last in the plan if global, and why "earliest" is the wrong intuition. Corrected the partial's milestone headings from level 2 to level 3, which was manufacturing orphans rather than merely reading oddly. Recorded leaving the case unruled as a rejected option.
