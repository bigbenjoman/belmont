---
name: loop
description: Claude Code only. Drive a single feature to completion by self-pacing /belmont:implement → verify → next → status until no pending milestones remain.
alwaysApply: false
---

# Belmont: Loop

**Claude Code only.** This skill drives one Belmont feature to completion by repeatedly running the implement → verify → next → status cycle, pausing between iterations so you can watch progress and steer. It is a thin orchestration wrapper around Claude Code's built-in `/loop` skill (self-paced via `ScheduleWakeup`) — those mechanics do not exist on other AI CLIs, which is why this skill is installed only for Claude Code.

This is the interactive, in-session counterpart to the headless `belmont auto` CLI (also aliased `belmont loop`). Use `/belmont:loop` when you want to stay in the Claude Code REPL and have the agent advance the feature milestone-by-milestone without you re-typing each skill. Use `belmont auto` when you want fully headless, parallel, worktree-based execution from the terminal.

**Loop is the steering tool, not the throughput tool.** Auto will always be faster — it runs milestones in parallel worktrees and gives every phase a fresh context. Loop's value is that you are present and can redirect with a sentence. The recipe below therefore optimises for *not wasting your session* — batching follow-up fixes, triaging polish out of the critical path, and scoping re-verification — rather than for raw parallelism. Do not try to recover auto's parallelism here.

<!-- Canonical milestone-immutability rule. Included by every skill that can modify PROGRESS.md. Do not paraphrase in skill bodies — @include this partial so the rule stays a single source of truth. -->

## Milestone structure is immutable outside `/belmont:tech-plan`

**You MUST NOT add, remove, rename, re-scope, or re-parent any `## M<N>:` milestone heading in `PROGRESS.md`.** Only `/belmont:tech-plan` may restructure milestones. Every other skill — `implement`, `verify`, `next`, `debug-auto`, `debug-manual`, the triage phase — may only edit tasks **inside** existing milestone headings.

This rule supersedes any contradictory guidance you encounter elsewhere. If another instruction seems to permit creating a milestone (for follow-ups, polish, cleanup, verification fixes, etc.), prefer this rule.

### Where follow-ups go

- **Issue discovered while implementing or verifying milestone `M<N>`** → new `[ ]` task inside `M<N>`, under the same `## M<N>:` heading. Do not route it to an earlier or later milestone "because it fits there better"; the milestone that discovered it owns it.
- **Issue blocked by work that will land in a later milestone `M<N+k>`** → new `[!]` task inside `M<N>`, with a one-line reason that names `M<N+k>`. Auto surfaces `[!]` tasks as blockers; the task can be reopened as `[ ]` once the blocker lifts.
- **Cosmetic / nice-to-have item the user may never want** → append to `NOTES.md` under a `## Polish` section, creating the file if needed. These are context, not tasks.
- **Never a new milestone.** Not "M<last+1>: Polish", not "M<N>-FIX", not "MX: Deviations from M<N>", not "MY: Verification Fixes". Even if the existing `PROGRESS.md` already contains such a milestone from a prior run, that pattern is WRONG — do not add tasks to it and do not create siblings of it.

### Why this rule is non-negotiable

A polish/follow-up milestone looks tidy on paper but quietly breaks two invariants of the auto loop:

1. **Dependency graph lies.** A milestone labelled "polish M<N>" typically declares `(depends: M<N>)`. That makes it a sibling of every other `M<N+i>` that depends on `M<N>`. But its *real* dependency is that every later milestone's outputs are frozen — because the polish milestone edits the very files those later milestones imported from `M<N>`. Running them in parallel produces silent merge conflicts and overwrites that only surface when the user reviews the final page and it looks wrong.
2. **Auto loop grows without bound.** Every verify pass can discover follow-ups. If those follow-ups become a new milestone instead of new tasks in the current one, a 5-milestone feature can turn into 9 milestones mid-run, each re-triggering its own verify-fix-reverify cycle, compounding scope drift with every iteration.

Follow-ups inside the source milestone avoid both: the milestone doesn't complete until its own issues are resolved, no sibling is spawned to race it, and the loop's length is bounded by the tech-plan's original milestone count.

### If you find a pre-existing bad milestone

If `PROGRESS.md` already contains a milestone whose name or description matches the forbidden patterns (polish, follow-ups, cleanup, verification fixes, deviations from M<N>, etc.), do the following:

- Do NOT add new tasks to it.
- Do NOT create new milestones that depend on it or reference its tasks.
- Surface the issue in your summary/report to the user, suggesting `belmont validate` and `/belmont:tech-plan` to restructure.

Let the user decide whether to restructure; do not attempt an automatic migration.

## Argument

`$ARGUMENTS` is the feature name or slug to drive (e.g. `/belmont:loop checkout`). 

- If `$ARGUMENTS` is empty: list the feature directories under `.belmont/features/`, read each `PRD.md` for its name and status, and ask the user which feature to drive. If exactly one feature exists, you may select it and confirm. If none exist, tell the user to run `/belmont:product-plan` first, then stop.
- If `$ARGUMENTS` names a feature that does not resolve to a `.belmont/features/<slug>/` directory: report the mismatch, list the available feature slugs, and ask the user to clarify rather than guessing.

Resolve the argument to a single feature slug before starting the loop. The loop only ever progresses this one feature — never start unrelated work.

## Preflight (run once, before looping)

1. Resolve the feature slug from `$ARGUMENTS` as described above. Call it `<feature>`.
2. Confirm the feature exists and see how many milestones are pending. Prefer `belmont status --feature <feature>` if the CLI is installed — the Go CLI parses PROGRESS.md itself, so this costs one command and no file reads. Fall back to `/belmont:status <feature>` only if the CLI is unavailable.
3. If every milestone is already **verified**, report that the feature is complete and **stop** — do not start a loop.
4. If milestones read *done* but not verified (`[x]`, not `[v]`), that is not finished — `belmont status` flags it and names `belmont reverify`. Which route depends on whether there is anything left to build:
   - **No pending milestone, only `[x]` tasks** (the whole feature is built but unverified): run `belmont reverify --feature <feature>` and stop. Do **not** enter the loop. Iteration step 1 is "implement the next pending milestone", and there is no such milestone — the loop has nothing to do on its first step, and `reverify` is the command that exists for exactly this state.
   - **Pending milestones as well**: start the loop. Step 2's earlier-milestone rescan picks the `[x]` tasks up as it goes.
5. Otherwise, hand off to the loop driver below.

## Loop driver — delegate to `/loop`

Start Claude Code's built-in **`/loop`** skill in **self-paced mode** (no fixed interval — let the model decide when to schedule the next iteration). Pass it the iteration recipe below, with `<feature>` substituted for the resolved slug. The exact handoff is:

```
/loop Drive the <feature> Belmont feature to completion. Each iteration:
  0. PICK THE TARGET MILESTONE <M>, and do not assume the status check
     picked it for you. `belmont status` names the first milestone that is
     not fully verified, and /belmont:implement independently selects the
     first milestone holding any task that is not [v] or [-]. A [!] task
     satisfies both forever, so a milestone whose ONLY live work is [!]
     is named again on every iteration and neither tool will ever move on.
     So: confirm the named milestone has at least one [ ], [>] or [x] task.
     If it has none, walk down PROGRESS.md's milestone headings to the
     first one that does, and use that. If NO milestone has one, every
     remaining pending task is [!] — go straight to the blocked-task rule
     below and stop. Name <M> explicitly in every append below; never let
     a sub-skill re-derive it.
  1. Run /belmont:implement <feature> to build milestone <M>.
     Append: "IMPLEMENT MILESTONE <M>. This names the milestone
     explicitly and supersedes your own Step 1 selection — do not
     re-derive it. MILESTONE-SCOPED IMPLEMENTATION: only implement tasks in
     milestone <M>. Do NOT flip checkboxes, add/remove tasks, or edit notes
     for any other milestone — treat their state as read-only context."
  2. ALWAYS run /belmont:verify <feature> — there is no skip. Only verify
     writes [v], so a skipped milestone can never reach a verified state,
     and every milestone must. Append:
     "MILESTONE-SCOPED VERIFICATION: verify milestone <M>. Do NOT change
     the task state of any other milestone — they may be intentionally
     incomplete. ONE EXCEPTION, and it is an instruction to INCLUDE, not
     merely to record: if your Step 1 scan surfaces [x] tasks in an EARLIER
     milestone, add them to this pass — dispatch them to the verification
     and code-review agents alongside <M>'s tasks, and record the resulting
     [x]->[v] flips for those that pass. That rescan is the documented
     recovery for a verification whose flips were never written; this
     scoping rule must not suppress it. Never flip a task this pass did not
     actually verify."
  3. If verify reported follow-up (FWLUP) tasks, TRIAGE before fixing.
     Read the actual follow-up descriptions in PROGRESS.md — do not just
     count them. Classify each into exactly one of THREE classes:
       - Human-gated — no agent can close it, at any effort, in any number
         of rounds. It needs an approval ("the apply needs sign-off"), a
         product or architecture ruling ("decide whether…", "rule on…"), a
         credential or console action nobody automated has ("rotate the
         passwords", "populate the roster", "wire it once the App exists"),
         or a spec change that belongs to /belmont:tech-plan. The test is
         not difficulty — it is whether the missing thing is a PERSON.
       - Blocking — build/test failures, runtime errors, security issues,
         acceptance criteria not met, significant visual mismatch from the
         design, missing PRD-specified behaviour, missing i18n keys for
         primary user-facing text.
       - Deferrable — missing aria-labels, Lighthouse warnings, code style,
         docs, console.log cleanup, 1-2px spacing, import ordering, naming,
         perf micro-optimisations, tests for non-critical paths.
     Check human-gated FIRST — it outranks the other two, and a security or
     acceptance-criteria item that needs a person is human-gated, NOT
     blocking. Then, between the remaining two, err toward blocking when
     genuinely unsure; UI/visual fidelity issues are usually blocking.
     Then act:
       - Human-gated → mark the task `[!]` in place, leave its body where it
         is, and add one line to `## Decisions Log` naming what is being
         asked and of whom. NEVER fix it, NEVER defer it, NEVER withdraw it,
         NEVER let the circuit breaker sweep it. It is a question, and the
         only thing that clears it is an answer. Do not count it toward the
         fix rounds in step 4 — an attempt that cannot succeed is not a
         round. Exclude it from every "pending FWLUP" set below.
       - All remaining are deferrable → mark each `[-]` withdrawn in
         PROGRESS.md, add one line per item to `## Decisions Log` saying it
         was deferred as polish, move the detail to NOTES.md under
         `## Polish`, remove its PRD section, commit as "belmont: triage —
         deferred N polish items to NOTES.md", and go to step 5. Do NOT fix
         them, do NOT re-verify.
       - Any blocking → defer only the deferrable ones as above, leave the
         blocking ones pending, and go to step 4.
     DEFERRAL IS `[-]`, NOT A DELETION. Do not delete the checkbox line.
     `mergeProgressState` takes the worktree as base and carries master's
     missing lines back in, so a deleted task is resurrected by the next
     sync in either direction — and a line that is simply gone records no
     reason and shows up in no count. `[-]` is excluded from both counts,
     never offered as next work, does not stop a milestone reading complete,
     and wins from either side of a merge. That is what it is for.
     CIRCUIT BREAKER: if two fix rounds have already run for this milestone,
     defer EVERYTHING still classified blocking — as `[-]` with a Decisions
     Log line, same as above — and go to step 5. Human-gated tasks are NOT
     swept: they stay `[!]`.
     Deferral NEVER means creating a milestone — see the milestone-structure
     rule above.
  4. Fix the blocking follow-ups in ONE batch, not one at a time.
     Run /belmont:next <feature> and append: "BATCH MODE: implement ALL
     pending FWLUP tasks in <M> sequentially. Pending means `[ ]` — SKIP
     every `[!]`, which is waiting on a person and cannot be closed by
     working harder. For each: find it, create the MILESTONE file, dispatch
     to the implementation agent, process results, archive MILESTONE, then
     continue to the next pending FWLUP. Stop when no `[ ]` FWLUP tasks
     remain in <M>. Only work on FWLUP tasks belonging to <M>;
     if there are none, stop immediately and report 'No FWLUP tasks to fix.'
     ARCHIVE: Step 5 appends to MILESTONE-<M>.done.md rather than
     overwriting it, which is what keeps every task's log in a batch.
     Follow it as written."
     Do NOT invoke /belmont:next once per task — each invocation reloads the
     whole skill, and that cost is why this loop runs out of session.
     Then re-verify FOCUSED: run /belmont:verify <feature> and append
     "FOCUSED RE-VERIFICATION: only verify (1) the FWLUP tasks just fixed,
     (2) build and tests pass, (3) any previously-failing acceptance
     criteria. Do NOT re-run Lighthouse. Do NOT re-check visual specs unless
     a FWLUP addressed UI. Do NOT create new Polish-level issues."
     Return to step 3 to triage whatever that re-verify surfaced.
  5. Decide whether to continue, then record what the verdict will need.
     Run `belmont status --feature <feature>`. Do NOT use --format json — it
     is ~3x larger and grows with task count. Only fall back to
     /belmont:status <feature> if the CLI is unavailable: that skill loads
     ~6KB of its own instructions before shelling out to the same command,
     and this step runs once per milestone.
     CONTINUE to the next milestone unless a STOP condition below fires.
     Neither a task still at [x] nor a fired circuit breaker is a stop.
     Both BOUND A MILESTONE, not the run. Halting the whole feature because
     one milestone hit its bound strands every milestone that had nothing
     to do with it — which is exactly the mistake the blocked-task rule
     below exists to prevent, and the two rules must not contradict each
     other. They are verdict inputs, not stop triggers.
     STOP when any of these holds:
       - every milestone is verified — the "Next Milestone" line reads None
         AND status prints no done-but-unverified warning ("Next Milestone:
         None" alone is not enough, because [x] counts as done);
       - every remaining pending task in the feature is [!] (see the
         blocked-task rule below);
       - a COUNTED STOP CONDITION (a-d) fires.
     RECORD each iteration, for the final report: any task left at [x], and
     whether the step-3 circuit breaker fired for that milestone. Both go in
     {base}/NOTES.md under `## Loop decisions` — you will have compacted by
     the time you need them.
     THE VERDICT, when you stop. Report COMPLETE only if every milestone is
     verified, no task is at [x], the circuit breaker never fired, and no
     [!] remains. Otherwise report INCOMPLETE and say which of these holds:
       - Tasks still at [x]. THERE IS NO SUCCESSFUL RUN THAT LEAVES ONE:
         step 2 has no skip, so a task stays [x] only because verification
         found issues, errored, or never recorded its flips — all failures.
         Name them and say they need investigation, not just a
         `belmont reverify` re-run. (A feature reading "Complete" with
         unverified tasks is NOT finished; status warns and names reverify.)
       - Milestones the circuit breaker bounded. It defers everything still
         classified blocking, so its deferrals may include real defects.
         Name them — they are [-] in PROGRESS.md, listed in
         `## Decisions Log`, with detail in NOTES.md `## Polish` — and name
         /belmont:debug-manual <feature> as the next step, NOT
         `belmont reverify`: these are open defects, not unrecorded
         verifications.
       - [!] tasks outstanding — report them per the blocked-task rule.
  BLOCKED TASKS DO NOT STOP THE LOOP. A `[!]` is a question queued for a
  person; it is not a failure, not a stall, and not a reason to abandon
  work that has nothing to do with it. A milestone holding one can never
  read complete, so DO NOT wait on it — step 0 already routed you past it.
  Never flip a HUMAN-GATED `[!]` to any other marker to unblock yourself,
  and never guess the answer. Two `[!]` tasks are NOT human-gated and may
  be reopened as `[ ]`, because their reason names the condition and you
  can check it: one whose reason names a later milestone `M<N+k>` (the
  milestone-structure rule above mints these), once `M<N+k>` reads
  verified; and one the reconciliation agent raised over a merge, once the
  other side is `[x]`/`[v]`. If the reason names a PERSON, it is not one
  of these — leave it. If the reason names nothing checkable, leave it and
  say so in the report. Stop for
  the user ONLY when every remaining pending task in the feature is `[!]`:
  at that point there is nothing an agent can do, and continuing just
  re-reads the same file. When you stop for that reason, or for any other,
  report the queue with `belmont blockers --feature <feature> --summary`
  (drop --summary when the user needs the full question) and say plainly
  that the feature cannot finish until those are answered. A feature
  holding a `[!]` is INCOMPLETE, never complete — same as a remaining `[x]`
  or a bounded milestone, and for the same reason: the work is not settled.
  Like those two, it is a verdict input, not a reason to halt early.
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
       `belmont status` twice running. Note that a run whose only remaining
       work is `[!]` trips this on its own; the blocked-task rule above is
       what makes the report say WHY, so report the blocker queue rather
       than "no progress".
    d. The user steers you to stop, change features, or do other work.
       Stop immediately — this outranks every other rule here.
  These counters must live ON DISK, not in your head — this loop survives
  compaction and a remembered count does not. Nothing else records them, so
  after every failed phase and every blocking-issue-survived-a-fix-round,
  append one line to {base}/NOTES.md under `## Loop decisions`:
  "<iso date> <phase> failed for <M> — <one-line reason>". Count from that
  file. (c) alone is derivable without it, from `belmont status` task counts.
  Do not start unrelated work; only progress this one feature.
```

When delegating, you are invoking the `/loop` skill — follow its self-pacing guidance (it uses `ScheduleWakeup` to re-enter the task between milestones, surviving context compaction). Each iteration advances exactly one milestone, so the loop converges as milestones flip to verified.

**If the `/loop` skill is unavailable** in this Claude Code build, fall back to driving the cycle inline: run steps 0–5 yourself in sequence, then repeat from step 0 for the next milestone, using `ScheduleWakeup` to self-pace between milestones. Stop on the same conditions — all of them, including the blocked-queue one.

## Stop conditions

Every stop condition lives **inside** the fenced recipe above, deliberately: the recipe is the only text guaranteed to travel with the delegated `/loop` task and survive compaction. Stop-condition prose that sits only out here can be summarised away mid-run, which is precisely when a stall guard is needed. This section restates them for a reader; the fence is the copy that runs. Do not move any of them out here, and if you add one, add it to the fence.

**Stopping and the verdict are different questions, and step 5 keeps them apart.** Three things bound a *milestone* without ending the *run* — a task left at `[x]`, a fired circuit breaker, and a `[!]` — and all three make the final verdict INCOMPLETE. None of them halts the loop. Ending the whole feature because one milestone hit its bound strands every milestone that had nothing to do with it, which is the failure this skill's blocked-task rule exists to prevent; a stop rule that did it for the breaker instead would just reintroduce it under another name.

The loop stops — no further iteration — when any of these holds. Each is stated in the recipe:

- Every milestone is verified. Combined with no `[x]`, no breaker and no `[!]`, this is **the only COMPLETE verdict**.
- Every remaining pending task in the feature is `[!]` — the decision queue is all that is left, and no agent action can change the file. A single `[!]`, or one whole blocked milestone, is **not** a stop: step 0 selects past it.
- Any of the counted conditions (a) three consecutive phase failures, (b) the same milestone failing verification twice, (c) no state change across two iterations, or (d) the user steers you to stop, change features, or do other work.

On stop, report: the feature, which milestones completed this run, the verdict, and each reason it is INCOMPLETE if it is. Name tasks left at `[x]` and say they need investigation rather than a plain `belmont reverify`. Name milestones the breaker bounded, with `/belmont:debug-manual <feature>` as their next step — its deferrals are open defects, not unrecorded verifications. And if any `[!]` exist, report them with `belmont blockers --feature <feature> --summary`: they are the work the user has to do before the feature can finish, and a count buried in a status dump is not a handover.

## Scope rules

- **One feature only.** Never let an iteration pull in a different feature or unrelated refactor. The recipe's final line ("Do not start unrelated work") is load-bearing.
- **Do not edit milestone structure.** This skill orchestrates the existing implement/verify/next/status skills — it never adds, renames, or removes milestones. The canonical rule and the routing for discovered work are stated above; the triage step in particular must never turn a deferral into a milestone.
- **Respect each underlying skill's rules.** `/belmont:implement`, `/belmont:verify`, and `/belmont:next` enforce their own scope guards, evidence checks, and feature-detection prompts. Do not bypass them; just sequence them.
- **A human-gated `[!]` belongs to the user.** The loop may *write* one — that is what triage's human-gated class does — but it may never clear one, answer it on the user's behalf, or convert it to `[-]` to make a milestone read complete. `mergeProgressState` refuses to rank over `[!]` from either direction; this is the same rule at the skill layer. Two `[!]` writers are **not** human-gated and carry their own reopen condition — the milestone-structure rule's later-milestone dependency, and the reconciliation agent's merge blocker — and the recipe names both. Tell them apart by the reason on the task; that is what the reason is for. `belmont blockers` is how you show the rest.
- **Deferral is a marker, not an edit.** Withdrawn work is `[-]` plus a `## Decisions Log` line. Never express it by deleting the checkbox — see the recipe's step 3 for why that does not survive Belmont's own merge model.
