# PROGRESS.md Parsing: One Definition Per Concept

**Domains:** cli, state, auto-mode

**Why this matters.** PROGRESS.md is the single source of truth for all state, and it is read by at least eight independent pieces of code — the parser, the scope-guard snapshot, the evidence guard, the merge-conflict resolver, the post-merge sync, `reverify`, the skip path, the FWLUP scans. Each one historically re-implemented the structural questions "what does this marker mean?" and "where does the milestones region end?" with its own inline regex or string test. **Two shipped issues came from exactly that duplication, and in both cases every copy was wrong in the same way, so no amount of cross-checking between them would have caught it.**

The failure signature is always the same: information disappears and nothing errors. That is worse than a crash, because the file still looks fine and `belmont validate` still exits 0.

## Invariant

- **`canonicalMarker(marker) (taskStatus, bool)`** is the only definition of what a checkbox marker means. Derived helpers — `markerRank`, `markerIsVerified` — route through it. Nothing compares a raw marker byte.
- **`isSectionBreak(line) bool`** is the only definition of where the milestones region ends. It matches a level-2 ATX heading **at column zero only**; indentation is never trimmed first.
- **Nothing is dropped in silence.** An entry Belmont cannot place is surfaced, not discarded: an unreadable marker becomes `taskUnknown` (counted separately, rendered `[?]`, `belmont validate` exit 1), and a task line outside every milestone is reported by `orphanedTaskLines` → `detectOrphanViolations`.

## How it's enforced

In `cmd/belmont/state.go`:

- `canonicalMarker` / `markerRank` / `markerIsVerified` — marker semantics. Callers: `parseMilestones`, `resolveProgressConflict`, `mergeProgressState`, `resetVerifiedTasks`, `findEvidenceMissingFlips`, `revertEvidenceMissing`.
- `isSectionBreak` — region boundary. Callers: `parseMilestones`, `skipMilestoneInProgress`, `rebuildAfterScopeGuard`, `parseProgressSnapshot`, `revertEvidenceMissing`.
- `orphanedTaskLines(progress)` — task lines outside any milestone. Surfaced by `renderStatus` and by `detectOrphanViolations` (wired into `belmont validate` and auto's startup lint).

Tests: `cmd/belmont/marker_readers_test.go`, `cmd/belmont/section_break_test.go`. Each has a control that fails when the defect is reintroduced, including `TestSnapshotAgreesWithParserOnIndentedHeading`, which pins the parser and the scope-guard snapshot to the *same* answer.

## Failure mode if you break it

- **Inline the check "just here".** That is how both bugs shipped. Issue #27: five readers each compared marker bytes; `[V]` counted as verified everywhere while being invisible to the evidence guard. Issue #31: five readers each did `strings.HasPrefix(strings.TrimSpace(line), "## ")`, so a heading indented inside a task's own write-up body ended task collection for the rest of the file — 85 of 541 tasks invisible on a real project, including outstanding `[ ]` and `[!]` work, `Status: Complete`, `validate` exit 0.
- **Trim before testing a structural prefix.** Indentation is semantic in Markdown. `  ## Foo` under a list item is that item's continuation, not a heading.
- **Treat "can't parse this" as a default state.** Guessing `todo` for an unknown marker is what made #27 schedule cancelled work for implementation.
- **Let the parser and the scope-guard snapshot diverge.** They must agree on region boundaries or the guard reverts the wrong lines.

## Don't re-do

- **Make `isSectionBreak` accept indented headings "for robustness".** It is the bug. A column-zero `## ` genuinely ends the region — `## Session History` and `## Decisions Log` follow the milestones in every real file — so the boundary must stay, only the trimming goes.
- **Count orphaned tasks into their preceding milestone.** Considered for #31 and rejected: past a real column-zero break they are genuinely outside the region, and silently adopting them would make `## Session History` entries into tasks. Report them instead.
- **Use a Markdown library.** Belmont's CLI is deliberately stdlib-only (`AGENTS.md`), and the file is a fixed, small dialect. A parser dependency buys correctness on constructs Belmont does not use and costs the zero-dependency property.

## Evidence

- Issue #27 — `[X]`/`[V]`/`[-]` all parsed as `todo`; a completed task was offered as *Next Individual Task*. Reproduced before/after with the harness: before offers `P1-M1-2 - capital X` and `validate` exits 0; after offers `None` and exits 1.
- Issue #31 — the reporter's exact fixture yields 2 of 4 tasks before, 4 of 4 after; `Status: Complete` → `In Progress`, with the `[!]` blocker surfaced.
- Both were found by a user on a real project, not by any check in this repo. The class is invisible to tests that only exercise well-formed fixtures — every fixture in the suite before #31 used column-zero headings.

## Revisions

- 2026-08-08 — initial. Records the shared root cause of #27 and #31, the two canonical helpers, the never-drop-in-silence rule, and the rejected alternatives.
