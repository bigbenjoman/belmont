# PROGRESS.md Parsing: One Definition Per Concept

**Domains:** cli, state, auto-mode

**Why this matters.** PROGRESS.md is the single source of truth for all state, and it is read by at least eight independent pieces of code — the parser, the scope-guard snapshot, the evidence guard, the merge-conflict resolver, the post-merge sync, `reverify`, the skip path, the FWLUP scans. Each one historically re-implemented the structural questions "what does this marker mean?" and "where does the milestones region end?" with its own inline regex or string test. **Two shipped issues came from exactly that duplication, and in both cases every copy was wrong in the same way, so no amount of cross-checking between them would have caught it.**

The failure signature is always the same: information disappears and nothing errors. That is worse than a crash, because the file still looks fine and `belmont validate` still exits 0.

## Invariant

- **`canonicalMarker(marker) (taskStatus, bool)`** is the only definition of what a checkbox marker means. Derived helpers — `markerRank`, `markerIsVerified` — route through it. Nothing compares a raw marker byte.
- **`isSectionBreak(line) bool`** is the only definition of where the milestones region ends. It matches a level-2 ATX heading **at column zero only**; indentation is never trimmed first.
- **Nothing is dropped in silence.** An entry Belmont cannot place is surfaced, not discarded: an unreadable marker becomes `taskUnknown` (counted separately, rendered `[?]`, `belmont validate` exit 1), and a task line outside every milestone is reported by `orphanedTaskLines` → `detectOrphanViolations`.
- **Ambiguous structure is refused, not guessed.** A repeated `### M<n>:` heading makes every milestone-keyed lookup resolve arbitrarily. `parseProgressSnapshot` records it in `DupIDs`; `runScopeGuard` and `runEvidenceCheck` both decline to run and say why; `detectViolations` raises `duplicate_milestone_id`. Same policy as a duplicated task ID in `mergeProgressState`.

## How it's enforced

In `cmd/belmont/state.go`:

- `canonicalMarker` / `markerRank` / `markerIsVerified` — marker semantics. Callers: `parseMilestones`, `resolveProgressConflict`, `mergeProgressState`, `resetVerifiedTasks`, `findEvidenceMissingFlips`, `revertEvidenceMissing`.
- `isSectionBreak` — region boundary. Callers: `parseMilestones`, `orphanedTaskLines`, `skipMilestoneInProgress`, `rebuildAfterScopeGuard`, `parseProgressSnapshot`, `revertEvidenceMissing`, `mergeProgressState` (both walks **and** its duplicate-ID counter), `resolveProgressConflict` (both the `theirsStates` scan and the `ours` rewrite), `resetVerifiedTasks`, `pendingTasksInRange`, `fwlupTasksInRange`.
- `orphanedTaskLines(progress)` — task lines outside any milestone. Surfaced by `renderStatus` and by `detectOrphanViolations` (wired into `belmont validate` and auto's startup lint).

**Verify that caller list with `grep`, not by reading a commit message.** The first version of this entry listed `rebuildAfterScopeGuard` as a caller and it was not one — the commit that claimed to convert "all five readers" converted two, and the same false claim was repeated here and in `docs/prd-format.md`. Three later commits then built on top of a seam that nobody had checked. `grep -n 'isSectionBreak' cmd/belmont/*.go` against `grep -n '"## "' cmd/belmont/*.go` takes seconds and is the only trustworthy answer.

Tests: `cmd/belmont/marker_readers_test.go`, `cmd/belmont/section_break_test.go`, `cmd/belmont/region_scope_test.go`. Each has a control that fails when the defect is reintroduced, including `TestSnapshotAgreesWithParserOnIndentedHeading` and `TestRebuildAgreesWithSnapshotOnBlockBoundary`, which pin the parser, the scope-guard snapshot and the guard's *rebuilder* to the same answer. The controls are mutation-tested: reintroducing any of the region-blind walks fails at least one named test.

## Failure mode if you break it

- **Inline the check "just here".** That is how both bugs shipped. Issue #27: five readers each compared marker bytes; `[V]` counted as verified everywhere while being invisible to the evidence guard. Issue #31: five readers each did `strings.HasPrefix(strings.TrimSpace(line), "## ")`, so a heading indented inside a task's own write-up body ended task collection for the rest of the file — 85 of 541 tasks invisible on a real project, including outstanding `[ ]` and `[!]` work, `Status: Complete`, `validate` exit 0.
- **Trim before testing a structural prefix.** Indentation is semantic in Markdown. `  ## Foo` under a list item is that item's continuation, not a heading.
- **Treat "can't parse this" as a default state.** Guessing `todo` for an unknown marker is what made #27 schedule cancelled work for implementation.
- **Let the parser and the scope-guard snapshot diverge.** They must agree on region boundaries or the guard reverts the wrong lines.
- **Convert *some* of the readers and describe it as all of them.** The half-conversion is worse than none, because the two halves then disagree. The snapshot decides where a milestone block *ends*; the rebuilder decides which post lines that block *replaces*. When only the snapshot was converted, an out-of-scope revert emitted the pre block whole and then re-emitted post's leftover tail below it: the task line appeared twice with contradictory markers, and **the illegal flip the guard had just detected survived on disk** while the run logged `✓ reverted 1 violation(s)`. Re-applying it doubled the duplicates again. In the mirror case (`##` + tab, which `isSectionBreak` accepts and a literal `"## "` test does not) the replacement ran past the real region end and deleted `## Session History` outright. A boundary mismatch is not a missed revert — it is active corruption written to disk and amended into the agent's commit.
- **Region-scope the reads but not the writes.** `resolveProgressConflict` builds its state map last-occurrence-wins, so an unscoped walk let a task-shaped line under `## Session History` shadow the real one in both directions: a quoted `[v]` **minted a verified flip with no commit evidence** (and `runEvidenceCheck` only ever inspects a post-*phase* file inside the worktree, never a post-*merge* file on master, so nothing audits it), while a stale quoted `[ ]` hid a genuine `[x]` and the completion vanished under a green "conflicts auto-resolved". Any map keyed by task ID needs the region gate, and so does the loop that writes the result back.
- **Count with a different rule than you merge with.** `mergeProgressState`'s duplicate-ID refusal counted task-shaped lines document-wide while both of its walks honoured the region. A sibling logging `- [x] P1-M1-1: done, commit abc123` into the history therefore made the real in-region task look ambiguous; the refusal dropped master's completion and the warning told the user to de-duplicate an ID that occurs once.

## Don't re-do

- **Make `isSectionBreak` accept indented headings "for robustness".** It is the bug. A column-zero `## ` genuinely ends the region — `## Session History` and `## Decisions Log` follow the milestones in every real file — so the boundary must stay, only the trimming goes.
- **Count orphaned tasks into their preceding milestone.** Considered for #31 and rejected: past a real column-zero break they are genuinely outside the region, and silently adopting them would make `## Session History` entries into tasks. Report them instead.
- **Suppress a merge carry-over because the other side "already has" the ID somewhere.** Tried and reverted within one commit. Keying `mergeProgressState`'s carry-over off every occurrence — in-region *and* orphaned — looks like duplicate-prevention and is state loss: when a worktree holds an ID only as an orphan (a fork-stale line stranded behind a heading its agent wrote, or a sibling's completion quoted into a log), master's in-region copy is the only counted one, and skipping it deletes the task from the only document that carries it. Carry it in and warn. Splicing is safe precisely *because* `countIDs` is region-scoped — an in-region line plus an orphan sharing its ID is not a duplicate, so the next sibling merge will not refuse it. The carry-over gate must be the in-region set alone.
- **Use a Markdown library.** Belmont's CLI is deliberately stdlib-only (`AGENTS.md`), and the file is a fixed, small dialect. A parser dependency buys correctness on constructs Belmont does not use and costs the zero-dependency property.

## Evidence

- Issue #27 — `[X]`/`[V]`/`[-]` all parsed as `todo`; a completed task was offered as *Next Individual Task*. Reproduced before/after with the harness: before offers `P1-M1-2 - capital X` and `validate` exits 0; after offers `None` and exits 1.
- Issue #31 — the reporter's exact fixture yields 2 of 4 tasks before, 4 of 4 after; `Status: Complete` → `In Progress`, with the `[!]` blocker surfaced.
- Both were found by a user on a real project, not by any check in this repo. The class is invisible to tests that only exercise well-formed fixtures — every fixture in the suite before #31 used column-zero headings.
- The controls only count if they can fail. Before the caller list was completed, six mutations passed the whole suite — including severing orphan detection from `belmont validate` and from `belmont status`, i.e. reproducing #31's exact "validate exits 0" symptom. Two tests were vacuous for the regression they named: one used a markdown table where the fixture needed a task-shaped line, and a resolver test's two sides stopped colliding, so git merged them cleanly and the resolver under test was never called. **A conflict fixture must be asserted to have conflicted** — never `t.Skip` when the resolver declines, or the test passes forever without running anything.

## Verification

Every rule above is mutation-tested. Reintroducing any region-blind walk, severing any wire from orphan detection to a user-visible surface, or reverting the carry-over gate fails at least one named test. Re-run that check after touching this area: back up `cmd/belmont/`, apply each mutation, `go test ./cmd/belmont`, restore. A mutation that survives is a missing control, not a passing suite.

## Revisions

- 2026-08-08 — initial. Records the shared root cause of #27 and #31, the two canonical helpers, the never-drop-in-silence rule, and the rejected alternatives.
- 2026-08-08 — corrected the `isSectionBreak` caller list, which claimed `rebuildAfterScopeGuard` had been converted when it had not. Six more readers converted (`rebuildAfterScopeGuard`, `mergeProgressState`'s duplicate counter, `resolveProgressConflict` ×2, `resetVerifiedTasks`, `pendingTasksInRange`/`fwlupTasksInRange`). Added the half-conversion, region-scoped-writes and mismatched-counter failure modes, and the rule that the caller list is verified by grep rather than by commit message.
- 2026-08-08 — added the refuse-ambiguous-structure invariant (duplicate `### M<n>:` headings), the rejected carry-over suppression that cost state, the vacuous-control evidence, and the Verification section recording that the whole entry is mutation-tested.
