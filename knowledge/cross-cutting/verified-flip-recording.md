# Recording the `[x]` → `[v]` Flip

**Domains:** skills, cli, state, auto-mode

**Why this matters.** Verification is the only state transition in Belmont that no Go code can perform. `runEvidenceCheck` can *demote* a `[v]`; nothing anywhere *promotes* one. The flip is written by the verify orchestrator editing markdown, and if that write is skipped there is no second writer, no reconciliation pass, and no guard that notices — because every stop condition in the product treats `[x]` as finished. The run ends reporting success on work it never verified. See issue #30.

This is the mirror of [`auto-mode/verify-evidence.md`](../auto-mode/verify-evidence.md), which covers the opposite direction (a `[v]` written *without* justification). Both halves are needed: that entry stops an unearned flip, this one stops a missing earned flip.

## Invariant

- **The flip is recorded before the report is written**, on every outcome — including a clean ALL PASSED run with no follow-ups.
- **The report states what was written, re-read from disk.** Not what the status implies.
- **`Complete` is never treated as finished.** Only `Verified` is. `computeOverallStatus` returns `"Complete"` when every task is `[x]` **or** `[v]`, so the two are indistinguishable to any consumer that only reads the status string.
- **`belmont status` says so out loud** when a feature reads Complete with unverified tasks, and names `belmont reverify` — the only recovery. It appeared in no skill or partial when this entry was written; this branch added it to `verify.md`, `status.md`, `loop.md`, `repair.md` and `_partials/milestone-immutability.md`, so the naming in `status` is now reinforced rather than the sole mention.
- **The recovery must not destroy more `[v]` than it can re-earn in one step.** `belmont reverify` has to reset `[v]` → `[x]` before a milestone is dispatched, because the agent looks for `[x]`; that reset is destructive and runs *before* anything justifies it, with no backup, no signal handler, and a failure branch that records the error and moves on. It used to run for the **whole range in one write**, so a run interrupted seconds in — Ctrl-C, a sleeping laptop, a rate limit — left every milestone downgraded and none re-verified, producing a diff that reads exactly like a regression. Since #49 the reset is scoped to the milestone being dispatched and happens immediately before its dispatch, so the loss is bounded to the one milestone in flight. The counterpart rule: **`resetMilestoneBeforeVerify` owns its own read of PROGRESS.md** rather than working from a startup snapshot, because the agent commits its own edits between milestones and writing M2's reset from a stale copy would revert what M1's agent just recorded. This is the third instance of the silent-`[v]`-loss class #24 and #30 fixed, on a path neither touched — and the one most likely to be hit, since `loop.md` instructs the agent to run `belmont reverify` directly.

## How it's enforced

Mechanically (cannot be dropped by an agent):

- `doneNotVerifiedTasks(milestones)` in `state.go` — mirror of `unknownMarkerTasks`.
- `renderStatus` warns in **both** views, gated on the feature reading `Complete`: the detail view naming each task, and the listing view (`f.Status == "Complete" && f.TasksVerified < f.TasksTotal`). The listing half is the one that matters most — `status.md`'s fast path runs `belmont status` with no `--feature`.
- `statusReport.FeatureSlug` exists so the message can print a pasteable `belmont reverify --feature <slug>`; `Feature` is the human-readable PRD name and is wrong for that.
- Tests in `cmd/belmont/verify_flip_test.go`, each with a control.

By prose (reduces the chance of the drop, cannot prevent it):

- `verify.md` — the flip lives in its own `### Mark Verified Tasks` section **above** `### Create Follow-up Tasks`, and ends with an explicit re-read-and-confirm step.
- `references/verify-report-format.md` — required `## Tasks Marked Verified` field, with a *re-read PROGRESS.md to populate this* clause and a STOP instruction if ALL PASSED yields an empty list.
- `loop.md` — stop condition requires **verified**, bounded so a legitimately-`[x]` failed task cannot churn forever.
- `status.md` — instructs reproducing the CLI warning verbatim rather than summarising it.

## Failure mode if you break it

- **Warn on any `[x]`, not just when the feature reads Complete.** Fires mid-run on every project — `[x]` between implement and verify is the normal state. The warning gets ignored, and with it the real one.
- **Change `computeOverallStatus` to stop returning `"Complete"` for all-`[x]`.** `isFeatureTerminal` switches on the literal string and `auto --all` wave planning depends on it. Silent blast radius. The warning must be rendered text only.
- **Rely on the prose fix alone.** It is unfalsifiable without live evals: Tier-1 tests never read a `SKILL.md`. The Go warning is the only part testable in CI, which is why it is primary.
- **Assume this is interactive-only.** `auto_loop.go` sends the bare `/belmont:verify --feature <slug>`, which resolves to the same generated `SKILL.md` the REPL loads. Serial auto completes on all-`[x]` via `decideLoopActionSmart`; parallel auto's `computeWaves` skips `milestoneAllDone` milestones entirely. A fix in `loop.md` alone would miss both.

## Don't re-do

- **Have Belmont auto-flip `[x]` → `[v]` in Go.** Forges precisely the evidence `verify-evidence.md` exists to protect. Rejected outright.
- **Make `nextMilestone` treat an all-`[x]` milestone as pending.** Tempting — it would make `loop.md`'s original stop condition correct for free. But `auto_decide.go` calls `milestoneAllDone` independently at ~11 sites, so auto would keep terminating while `next` re-offered work. That is divergence between the two invocation paths, not a fix.
- **A post-verify flip-completeness guard in `guards.go`.** The infrastructure exists (pre/post snapshots, `injectEvidenceSteering`). Deferred: its discriminator — "verify succeeded, zero `[x]`→`[v]` flips, no new `[ ]` tasks" — has a real false-positive path, and it only covers auto mode, not where the symptom was reported.
- **Copy `implement.md:136`'s "the sub-agent should have marked it; if missed, do it now" backstop.** Structurally unavailable here: `agents/belmont/verification-agent.md` says "You do NOT update state files yourself — only report results." The verify orchestrator is the sole writer, so there is nothing to double-check.

## Evidence

- Measured on the real binary: an all-`[x]` feature reports `Status: Complete`, `Tasks: 0 verified, N done`, `Next Milestone: None`, `Next Individual Task: None`. Every stop condition is satisfied.
- Only the **final** milestone's drop sticks. A mid-run drop self-corrects, because a later milestone is still pending and `verify.md` Step 1 rescans the whole feature for `[x]` on the next iteration.
- **Not observed in a transcript.** That an orchestrator actually skips the section is inference from the instruction surface, not a recorded run. What is established is that nothing would catch it. Frequency is unmeasured; the fingerprint to look for in a real project is a feature containing **both** all-`[v]` and all-`[x]` milestones — verify demonstrably ran and flipped there, yet other implemented milestones never got their flip.

## Revisions

- 2026-08-08 — initial. Records the missing-flip half of the `[v]` contract, the Go-side status warning as the primary defence, the prose complements, and the rejected alternatives (auto-flip, `nextMilestone` change, guards-side check, sibling backstop pattern).
- 2026-08-15 — added the rule that the recovery itself must not destroy more `[v]` than it can re-earn: `belmont reverify` now resets per milestone immediately before dispatching it, rather than the whole range up front (#49), and re-reads PROGRESS.md on each reset so a later milestone's write cannot revert an earlier agent's recorded result. Third instance of the class #24 and #30 fixed, on a path neither touched.
