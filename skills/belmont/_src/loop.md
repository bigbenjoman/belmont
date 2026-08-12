---
description: Claude Code only. Drive a single feature to completion by self-pacing /belmont:implement → verify → next → status until no pending milestones remain.
alwaysApply: false
---

# Belmont: Loop

**Claude Code only.** This skill drives one Belmont feature to completion by repeatedly running the implement → verify → next → status cycle, pausing between iterations so you can watch progress and steer. It is a thin orchestration wrapper around Claude Code's built-in `/loop` skill (self-paced via `ScheduleWakeup`) — those mechanics do not exist on other AI CLIs, which is why this skill is installed only for Claude Code.

This is the interactive, in-session counterpart to the headless `belmont auto` CLI (also aliased `belmont loop`). Use `/belmont:loop` when you want to stay in the Claude Code REPL and have the agent advance the feature milestone-by-milestone without you re-typing each skill. Use `belmont auto` when you want fully headless, parallel, worktree-based execution from the terminal.

**Loop is the steering tool, not the throughput tool.** Auto will always be faster — it runs milestones in parallel worktrees and gives every phase a fresh context. Loop's value is that you are present and can redirect with a sentence. The recipe below therefore optimises for *not wasting your session* — batching follow-up fixes, triaging polish out of the critical path, and scoping re-verification — rather than for raw parallelism. Do not try to recover auto's parallelism here.

<!-- @include milestone-immutability.md -->

## Argument

`$ARGUMENTS` is the feature name or slug to drive (e.g. `/belmont:loop checkout`). 

- If `$ARGUMENTS` is empty: list the feature directories under `.belmont/features/`, read each `PRD.md` for its name and status, and ask the user which feature to drive. If exactly one feature exists, you may select it and confirm. If none exist, tell the user to run `/belmont:product-plan` first, then stop.
- If `$ARGUMENTS` names a feature that does not resolve to a `.belmont/features/<slug>/` directory: report the mismatch, list the available feature slugs, and ask the user to clarify rather than guessing.

Resolve the argument to a single feature slug before starting the loop. The loop only ever progresses this one feature — never start unrelated work.

## Preflight (run once, before looping)

1. Resolve the feature slug from `$ARGUMENTS` as described above. Call it `<feature>`.
2. Confirm the feature exists and see how many milestones are pending. Prefer `belmont status --feature <feature>` if the CLI is installed — the Go CLI parses PROGRESS.md itself, so this costs one command and no file reads. Fall back to `/belmont:status <feature>` only if the CLI is unavailable.
3. If every milestone is already **verified**, report that the feature is complete and **stop** — do not start a loop.
   - If milestones read *done* but not verified (`[x]`, not `[v]`), that is not finished: `belmont status` flags it and names `belmont reverify`. Either run verification for them or start the loop; do not stop here. See issue #30.
4. Otherwise, hand off to the loop driver below.

## Loop driver — delegate to `/loop`

Start Claude Code's built-in **`/loop`** skill in **self-paced mode** (no fixed interval — let the model decide when to schedule the next iteration). Pass it the iteration recipe below, with `<feature>` substituted for the resolved slug. The exact handoff is:

```
/loop Drive the <feature> Belmont feature to completion. Each iteration:
  1. Run /belmont:implement <feature> to build the next pending milestone.
     Append: "MILESTONE-SCOPED IMPLEMENTATION: only implement tasks in
     milestone <M>. Do NOT flip checkboxes, add/remove tasks, or edit notes
     for any other milestone — treat their state as read-only context."
     (Substitute the milestone the status check named.)
  2. Decide whether verification is warranted, then run it.
     Skip /belmont:verify only when the milestone's diff is trivially
     unverifiable: zero files changed, pure documentation, or non-critical
     config touching <=2 files. Anything touching frontend, backend, schema,
     or critical config is ALWAYS verified — when unsure, verify.
     When you do verify, run /belmont:verify <feature> and append:
     "MILESTONE-SCOPED VERIFICATION: verify milestone <M>. Do NOT change
     the task state of any other milestone — they may be intentionally
     incomplete. ONE EXCEPTION, and it is an instruction to INCLUDE, not
     merely to record: if your Step 1 scan surfaces [x] tasks in an EARLIER
     milestone that are NOT recorded as a deliberate skip in NOTES.md
     `## Loop decisions`, add them to this pass — dispatch them to the
     verification and code-review agents alongside <M>'s tasks, and record
     the resulting [x]->[v] flips for those that pass. That rescan is the
     documented recovery for a verification whose flips were never written;
     this scoping rule must not suppress it. Never flip a task this pass
     did not actually verify."
     If you SKIP verify, you must leave a record on disk — step 5's success
     test depends on it and your memory of this decision will not survive
     compaction. Append to {base}/NOTES.md under `## Loop decisions`:
     "verify skipped for <M> — <reason>", and commit it. An [x] with no such
     record is NOT a deliberate gap: it is a verification that failed,
     crashed, or never recorded its flips, and step 5 must treat it that way.
  3. If verify reported follow-up (FWLUP) tasks, TRIAGE before fixing.
     Read the actual follow-up descriptions in PROGRESS.md — do not just
     count them. Classify each as:
       - Blocking — build/test failures, runtime errors, security issues,
         acceptance criteria not met, significant visual mismatch from the
         design, missing PRD-specified behaviour, missing i18n keys for
         primary user-facing text.
       - Deferrable — missing aria-labels, Lighthouse warnings, code style,
         docs, console.log cleanup, 1-2px spacing, import ordering, naming,
         perf micro-optimisations, tests for non-critical paths.
     Err toward blocking when genuinely unsure; UI/visual fidelity issues are
     usually blocking. Then act:
       - All deferrable → move them to NOTES.md under `## Polish` (removing
         their PRD sections and PROGRESS checkbox lines), commit as
         "belmont: triage — deferred N polish items to NOTES.md", and go to
         step 5. Do NOT fix them, do NOT re-verify.
       - Any blocking → move only the deferrable ones to NOTES.md, leave the
         blocking ones pending, and go to step 4.
     CIRCUIT BREAKER: if two fix rounds have already run for this milestone,
     defer EVERYTHING remaining regardless of classification and go to step 5.
     Deferral NEVER means creating a milestone — see the milestone-structure
     rule above.
  4. Fix the blocking follow-ups in ONE batch, not one at a time.
     Run /belmont:next <feature> and append: "BATCH MODE: implement ALL
     pending FWLUP tasks in <M> sequentially. For each: find it, create the
     MILESTONE file, dispatch to the implementation agent, process results,
     archive MILESTONE, then continue to the next pending FWLUP. Stop when no
     FWLUP tasks remain in <M>. Only work on FWLUP tasks belonging to <M>;
     if there are none, stop immediately and report 'No FWLUP tasks to fix.'
     ARCHIVE CAVEAT: your own Step 5 says to OVERWRITE
     MILESTONE-<M>.done.md when it already exists. That rule assumes one
     task per invocation; in batch mode it would destroy every log but the
     last. APPEND each task's log to that file instead."
     Do NOT invoke /belmont:next once per task — each invocation reloads the
     whole skill, and that cost is why this loop runs out of session.
     Then re-verify FOCUSED: run /belmont:verify <feature> and append
     "FOCUSED RE-VERIFICATION: only verify (1) the FWLUP tasks just fixed,
     (2) build and tests pass, (3) any previously-failing acceptance
     criteria. Do NOT re-run Lighthouse. Do NOT re-check visual specs unless
     a FWLUP addressed UI. Do NOT create new Polish-level issues."
     Return to step 3 to triage whatever that re-verify surfaced.
  5. Check whether work remains: run `belmont status --feature <feature>`.
     Do NOT use --format json here — it is ~3x larger and grows with task
     count. Only fall back to /belmont:status <feature> if the CLI is
     unavailable: that skill must load ~6KB of its own instructions before
     it even shells out to the same command, and this step runs once per
     milestone. STOP only when every milestone is verified — the "Next
     Milestone" line reads None AND status prints no done-but-unverified
     warning ("Next Milestone: None" alone is not enough, because [x]
     counts as done) — or when status still reports tasks
     done-but-unverified after a re-verify pass has already run this
     session (some tasks legitimately stay [x] when verification found
     issues). A feature reading "Complete" with unverified tasks is NOT
     finished — status warns about it and names `belmont reverify`.
     SUCCESS-WITH-KNOWN-GAPS: if no pending milestones remain and EVERY
     remaining [x] task has DISK EVIDENCE that the run left it on purpose,
     that is a SUCCESSFUL run, not a stall. Only two things count as that
     evidence, and you must actually look — never infer it from memory:
       - a `## Loop decisions` entry in NOTES.md recording a step-2 skip
         for that milestone; or
       - a `belmont: triage — deferred N polish items` commit, where the
         item is listed under `## Polish` in NOTES.md.
     Stop, report the feature complete, name those tasks, and offer
     `belmont reverify --feature <feature>`. Do NOT iterate again hoping
     they flip: nothing in the remaining steps can flip them.
     NOT SUCCESS — report INCOMPLETE, not complete, if either holds:
       - any remaining [x] lacks that evidence. It failed verification,
         crashed mid-phase, or never had its flips recorded. Name those
         tasks separately from the deliberate ones and say they need
         investigation, not just `belmont reverify`.
       - the step-3 CIRCUIT BREAKER fired this session. It defers
         everything regardless of classification, so its deferrals may
         include blocking issues. Report INCOMPLETE, name the deferred
         items from NOTES.md `## Polish` — their PROGRESS lines are gone,
         so that file is the only remaining record — and name
         /belmont:debug-manual <feature> as the next step.
     Otherwise continue to the next milestone.
  COUNTED STOP CONDITIONS — track these across iterations; when one fires,
  stop and report rather than scheduling another iteration:
    a. Three consecutive phase failures (any mix of implement/verify/next).
    b. The same milestone FAILS VERIFICATION twice. "Fails verification"
       means the verify phase errored, OR reported any issue you classified
       as BLOCKING in step 3 that survived a fix round — use step 3's
       blocking list, not verify's Critical/Warning tiers, which do not line
       up with it. It does NOT mean merely producing follow-ups, which is
       the normal path into step 3. This rule outranks the step-3 circuit
       breaker: if both would fire, stop rather than defer-and-proceed, and
       name /belmont:debug-manual <feature> as the next step.
    c. No state change across two iterations — identical task counts from
       `belmont status` twice running.
  These counters must live ON DISK, not in your head — this loop survives
  compaction and a remembered count does not. Nothing else records them, so
  after every failed phase and every blocking-issue-survived-a-fix-round,
  append one line to {base}/NOTES.md under `## Loop decisions`:
  "<iso date> <phase> failed for <M> — <one-line reason>". Count from that
  file. (c) alone is derivable without it, from `belmont status` task counts.
  Do not start unrelated work; only progress this one feature.
```

When delegating, you are invoking the `/loop` skill — follow its self-pacing guidance (it uses `ScheduleWakeup` to re-enter the task between milestones, surviving context compaction). Each iteration advances exactly one milestone, so the loop converges as milestones flip to verified.

**If the `/loop` skill is unavailable** in this Claude Code build, fall back to driving the cycle inline: run steps 1–5 yourself in sequence, then repeat from step 1 for the next milestone, using `ScheduleWakeup` to self-pace between milestones. Stop on the same condition (no pending milestones left).

## Stop conditions

The counted conditions (a/b/c) live **inside** the fenced recipe above, deliberately: the recipe is the only text guaranteed to travel with the delegated `/loop` task and survive compaction. Stop-condition prose that sits only out here can be summarised away mid-run, which is precisely when a stall guard is needed. Do not move them back out.

Stop the loop — do not schedule another iteration — when any of these holds:

- The status check in step 5 shows every milestone verified (the success case: feature complete).
- The status check in step 5 reaches **success-with-known-gaps**: no pending milestones, and every remaining `[x]` has disk evidence it was left on purpose — a `## Loop decisions` skip entry or a triage-deferral commit. Report success, name the unverified tasks, offer `belmont reverify`. Memory is not evidence; if the record is not on disk this branch does not apply.
- The status check in step 5 shows remaining `[x]` tasks *without* that evidence, or the step-3 circuit breaker fired this session — report **INCOMPLETE**, name those tasks separately from the deliberate ones, and say they need investigation rather than a plain `belmont reverify`.
- A milestone is blocked (`[!]` tasks) and cannot proceed after a batch fix attempt; report the blocker and stop for user input.
- Any of the counted conditions (a) three consecutive phase failures, (b) the same milestone failing verification twice, or (c) no state change across two iterations.
- The user steers you to stop, change features, or do other work.

On stop, report: the feature, which milestones completed this run, the final status, any blockers or stuck tasks that need user attention, and — if `belmont status` warned about done-but-unverified tasks — say so explicitly and name `belmont reverify --feature <feature>` as the recovery.

## Scope rules

- **One feature only.** Never let an iteration pull in a different feature or unrelated refactor. The recipe's final line ("Do not start unrelated work") is load-bearing.
- **Do not edit milestone structure.** This skill orchestrates the existing implement/verify/next/status skills — it never adds, renames, or removes milestones. The canonical rule and the routing for discovered work are stated above; the triage step in particular must never turn a deferral into a milestone.
- **Respect each underlying skill's rules.** `/belmont:implement`, `/belmont:verify`, and `/belmont:next` enforce their own scope guards, evidence checks, and feature-detection prompts. Do not bypass them; just sequence them.
