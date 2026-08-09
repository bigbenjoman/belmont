Domains: cli, skills, state

# Repairing a PROGRESS.md nobody can read any more

## Why this matters

Issues #27 and #31 made a corrupted PROGRESS.md **legible**. Neither fixed one.
`belmont status` names the offending lines, `belmont validate` exits 1, the auto
loop refuses to start — and a project already carrying the damage is left
diagnosed and stuck. The agents working in it are no better off: they see a
`[?]`, they know it is wrong, and they have no sanctioned action.

That gap is not a nice-to-have. Detection without repair is worse than it
sounds, because the loud failure lands on people who did nothing wrong and gives
them no route out. The obvious workarounds are both destructive: hand-edit the
marker to whatever seems plausible (which is how the file got this way), or
delete the line (which does not survive a sibling worktree merge).

The second reason this entry exists is the shape of the *wrong* solution, which
is very attractive. The natural design asks the user what each finding meant —
"did this ship? was it withdrawn? was it blocked?" A damaged file carries dozens
of these at once and the honest answer months later is "I don't know", followed
by a guess. Guessing a task's state is exactly issue #27, re-committed by the
tool that was supposed to fix it.

## Invariant

**Evidence-first, never memory-first.** A repair conclusion comes from the
repository — a commit that names the task ID, or the code the task describes —
never from anyone's recollection. Only findings that survive both tiers reach a
human, and by then the question is grounded in what the code says today.

The rest follows from that:

- **Two tiers, like `belmont reverify`.** Mechanical (`lookupCommitEvidence` per
  task ID, pure Go, zero tokens) then review (an agent reading the survivors
  against current code).
- **The action set is CLOSED; the causes are not.** `set_marker` /
  `move_milestone` / `withdraw` / `leave` / `escalate`. Why travels as a free
  reason string. New causes need no new code; a new action does.
- **`validateRepairPlans` is the single gate**, whoever proposed the action —
  the mechanical tier's own output goes through it too.
- **Capped at `[x]`.** Repair never mints `[v]`, in any spelling. That flip has
  its own evidence contract (`auto-mode/verify-evidence.md`) and `belmont
  reverify` is its only legitimate writer.
- **Withdrawal is `[-]` plus the reason in `## Decisions Log`, never a
  deletion.** `set_marker "-"` is refused so the reason cannot be skipped.
- **Milestone structure is immutable.** Repair never creates, renames or removes
  a `### M<n>:` heading, and a move whose destination does not already exist is
  refused with a pointer to `/belmont:tech-plan`.
- **Repair only touches lines it flagged**, and only while they are
  byte-for-byte what it scanned.
- **Ambiguous structure is refused, not guessed** — a repeated `### M<n>:`
  heading, same policy as both runtime guards.
- **The `[v]`-without-evidence audit is reported, never applied.** It is the
  mirror of `runEvidenceCheck` for the half that guard cannot see: it compares a
  phase's before and after, so a `[v]` already on disk when a run started is
  never a flip and is audited by nothing, ever. No commit is not proof of no
  work — `auto-mode/verify-evidence.md` records commit-less tasks as a known
  rough edge — so this reports and the review tier reads the code. The action
  that follows is `set_marker "x"`, handing the flip back to `belmont reverify`
  and its own contract.

## How it's enforced

`cmd/belmont/repair.go`:

- `collectRepairFindings` composes `parseMilestones` and `orphanedTaskLines`
  rather than walking the file again. A third walk here would be a fourth answer
  to "where does the milestones region end", which is the duplication that
  produced #27 and #31 in the first place.
- `attachCommitEvidence` → `lookupCommitEvidence` (in `guards.go`, beside
  `findMergeBaseRef`). `taskHasCommit` routes through the same function, so the
  auto loop's evidence guard and the healer cannot drift about what counts as a
  commit naming a task.
- `mechanicalRepairs` is deliberately narrow: an unreadable marker already
  inside a milestone, with commit evidence, becomes `[x]`. Nothing else is
  auto-applied.
- `validateRepairPlans` enforces every bound above and returns a
  `repairRejection` (with a reason) for anything it refuses — refusals are always
  printed.
- `applyRepairPlans` / `moveTaskLines` / `appendDecisionLogEntry` are the only
  writers, and they are pure string functions. `moveTaskLines` anchors exactly as
  `mergeProgressState` does: after the destination's last task line, or after its
  header when it holds no tasks yet.
- The skill (`skills/belmont/_src/repair.md`) carries the same rules for the
  interactive path, where nothing mechanical enforces them.

**The audit is deliberately NOT a fourth parse finding.** The three findings are
lines Belmont cannot act on; `belmont validate` exits 1 on them and repair exists
to clear them. A `[v]` with no commit parses perfectly, counts correctly, and
breaks nothing downstream — only the claim is wrong. Keeping it out of
`collectRepairFindings` is also what keeps "nothing to repair" and the unresolved
count meaningful: fold a permanent audit in there and a clean file never reads
clean, which is how a warning becomes wallpaper. `commitNamedTaskIDs` answers it
in one `git log` for the whole file rather than one per task.

**Two scoping decisions that look like bugs and are not:**

1. **Repair's commit search is UNSCOPED; the guard's is `mergeBase..HEAD`.** The
   guard asks "did *this phase* earn the flip it just wrote", so it must exclude
   older work. Repair asks the opposite question about damage that is months old
   and usually sitting on the default branch — where `merge-base(HEAD, main)` is
   HEAD, so the scoped range is empty and every finding reports "no commit names
   it". Scoping there is not conservative, it is inert.
2. **Repair does NOT inherit `taskHasCommit`'s fail-open.** That function returns
   true when git errors, so the guard never blocks real work on a shallow clone.
   Inheriting it would mark every unreadable task `[x]` in a directory with no
   git repository at all. Repair calls `lookupCommitEvidence` and reads
   `Checked`; "could not look" routes to the review tier.

**Moving a task between milestones is legal here and nowhere else.** Not an
exemption granted by prose — `runScopeGuard` is called from `executeLoopAction`
(`auto_loop.go`) and from nowhere else, so it is simply not running during an
interactive repair. Verify that with a grep before relying on it; if repair ever
gains an auto-mode path, this stops being true and the move action has to go.

Tests: `cmd/belmont/repair_test.go`. Every fixture that touches evidence builds a
**real git repository** — a bare `t.TempDir()` gets "evidence present" for free
and proves nothing in either direction.

## Failure mode if you break it

- **Ask the human.** You get "I don't know" and then a guess, recorded as fact by
  a tool whose whole purpose was to stop that. At three findings it feels fine; a
  real damaged file has thirty.
- **Auto-apply beyond commit evidence.** A task with no commit is not "not done"
  — it is unproven. Marking it `[ ]` on that basis re-opens finished work; marking
  it `[x]` claims work that may not exist, which is #30's failure mode.
- **Auto-move an orphan.** The commonest orphan is a sibling's completion quoted
  into `## Session History` — `- [x] P1-M1-1: done, commit abc123`. Its ID
  certainly has a commit. Moving it in duplicates a live task ID, and every
  milestone-keyed reader then refuses to run.
- **Let repair write `[v]`.** It would be the one path that mints a verified flip
  with no verification behind it, and `runEvidenceCheck` only inspects a
  post-phase file inside a worktree, so nothing would ever audit it.
- **Express withdrawal as a deletion.** `mergeProgressState` takes the worktree
  as base and carries the other side's missing lines back in, so the task returns
  as outstanding work at the next sibling sync — in either direction.
- **Skip the "is this still the line I scanned?" check.** An editor or a
  concurrent agent shifts the file, and the action lands on a task nobody judged.

## Don't re-do

- **A question-per-finding wizard.** Rejected above; this is the entry's main
  point. Interrogation is what an implementer reaches for first.
- **Inferring state from the task text alone** ("looks done"). Text is a
  specification, not a record. The evidence is the commit log and the code.
- **Fixing the file by deleting unreadable lines.** Loses the task, and the
  deletion does not survive a merge anyway.
- **Letting repair renumber or merge milestones** to resolve a duplicate
  `### M<n>:` heading. That is structural, therefore `/belmont:tech-plan`'s, and
  repair cannot even locate a task reliably in such a file.
- **Reusing `taskHasCommit` directly for the mechanical tier.** Its fail-open is
  correct for the guard and catastrophic here. Two-value question, one
  implementation.
- **Wiring repair into the auto loop.** It is interactive by construction: the
  review tier's conclusions are presented for confirmation, and the milestone-move
  action is only safe because the scope guard is not running. An auto-mode repair
  would need both properties re-derived.
- **Auto-demoting a `[v]` that no commit names.** The obvious next step once the
  audit exists, and wrong: a documentation-only or configuration-only task is
  routinely verified with nothing in the log naming it, so demoting on absence
  alone re-opens finished work — this command's own failure mode, pointed the
  other way. `leave` is a first-class verdict there.
- **Making the audit a `belmont validate` violation.** It would fail CI on files
  that parse and are correct, and `validate` is on `belmont auto`'s startup path.
  The audit belongs to the command you run when you are already looking.
- **A "safe to just fix it" violation class in `belmont validate`.** Considered
  and dropped when the remedies were designed: every remaining case turns on what
  a marker MEANT. Repair is the answer to that, and it answers it with evidence
  rather than by widening the lint.

## Evidence

- Issues #27 and #31, both found by a user on a real project. #31 hid 85 of 541
  tasks with `belmont validate` exiting 0; #27 offered a product-cancelled feature
  to an agent as the next thing to build. Both projects were left detectable and
  unfixable by the fixes themselves.
- The unscoped-log decision was found by running the first cut against a real
  fixture on `main`: every finding reported "no commit names it" while the commits
  were sitting in the log. The mechanical tier looked like it had run.
- `TestRepairDoesNotInheritFailOpen` pins the fail-open split in both directions —
  repair proposes nothing without git, and `taskHasCommit` still returns true.

## Revisions

- 2026-08-09 — added the `[v]`-without-evidence audit: the mirror of
  `runEvidenceCheck` for markers already on disk, reported and never applied,
  kept out of the parse findings so a clean file still reads clean. Records why
  auto-demotion and a `validate` violation are both rejected.
- 2026-08-09 — initial. Records the two-tier design, the evidence-first
  invariant and why the interrogating alternative is rejected, the closed action
  set, the `[x]` cap, the two scoping decisions that differ deliberately from
  `runEvidenceCheck`, and the grep that licenses the milestone-move action.
