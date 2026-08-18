# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `ad3559de`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 5 of 7.
- **Created**: 2026-08-18T14:45:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-5: The `origin/fix/wave-merge-state-loss` abandon reason states a mechanism `main` does not have

## Orchestrator Context

### Current Task
`P0-M1-FIX-5` — the only task in this run. A Warning from M1's verification pass: the verdict is defensible, the stated reason is factually wrong.

### Active Task IDs
`P0-M1-FIX-5`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-13 holds the parent task's acceptance criteria
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` — Pass 1 holds the original triage; `## Task P0-M1-FIX-3` records the rewrite of a *different* row in the same file, the evidence bar it set, and a measurement trap it discovered
- **The artefact to correct**: `docs/branch-triage.md`, the `origin/fix/wave-merge-state-loss` row (was around line 101 — `P0-M1-FIX-3` has since edited this file, so locate the row rather than trusting the line number)
- **The code the reason must describe**: `cmd/belmont/worktree.go` — `syncFeatureStateAfterMerge` and `mergeProgressState`

### The defect

`docs/branch-triage.md` abandons `origin/fix/wave-merge-state-loss` on the stated ground that *"`main`'s `worktree.go` already scopes the sync per milestone"*.

`main` does not do that. `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` takes **no milestoneID**, still performs `os.RemoveAll(dstFeature)` followed by `copyDir`, and there is no `resolveMilestoneProgress`.

The defect class **is** addressed on `main`, but by a different mechanism: master's `PROGRESS.md` is read *before* the wipe and then union-merged via `mergeProgressState` (`worktree.go:504`, called from `:410` — confirm these line numbers rather than trusting them, the file has changed this session). Separately, "refuses duplicate headings" is true, at `worktree.go:560/588`.

The row also states that *"only the branch's own test filename is absent"*, which understates a 251-line test file and an entire alternative implementation.

**The verdict itself is defensible — do not flip it.** This task corrects the reasoning, not the outcome.

### Required fix

1. Rewrite the `origin/fix/wave-merge-state-loss` reason to name the mechanism `main` **actually** uses, citing the lines that implement it. Verify those line numbers against the current file; do not copy them from this brief.
2. Correct the "only the branch's own test filename is absent" characterisation to reflect what the branch actually carries.
3. Keep the verdict as **abandon**, and make the evidence support it — or, if the evidence genuinely does not support abandon once you have read both sides, say so explicitly in your log rather than quietly changing the verdict.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-5`.
- **Out of Scope**: `P0-M1-FIX-3` already rewrote the `origin/feat/maintenance-ci` row, the headline bullet, the abandon-section preamble and the measurement-traps section in this same file. **Leave all of that alone** — read it for the evidence bar it sets, do not redo it.
- `P0-M1-FIX-6` (the "three of the five" wording) and `P0-M1-FIX-7` (`runCensus`'s silent skip) each get their own run.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase, cherry-pick or delete any remote branch. This task is a documentation correction — no git history changes.
- Do **not** start M2–M11 work. Do not change `worktree.go`; you are describing it, not fixing it.
- **Zero external Go dependencies, no `go.sum`.**

### Verification before marking `[x]`
```bash
cd /Users/benlavender/repos/belmont
go build ./cmd/belmont
go test ./cmd/belmont
go test -race ./cmd/belmont
go test -tags eval ./cmd/belmont
go vet ./...
staticcheck ./...          # at $(go env GOPATH)/bin — not on PATH
gofmt -l cmd/              # must print nothing
belmont validate --root /Users/benlavender/repos/belmont
```
Plus, specific to this task: every line reference you write must resolve to what you claim it does, checked by reading it at HEAD.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this defect's cause: ***"triage verdicts written from summary evidence rather than from content"*** — a mechanism that sounds plausible is not evidence that it exists.

**Prevention rule, and this task's bar**: an "abandon" reason must name the superseding mechanism on `main` and **cite the lines that implement it**. A destructive recommendation carries a higher evidence bar than a keep.

`P0-M1-FIX-3` applied that bar to the neighbouring row and, in doing so, found a trap worth knowing before you touch this file: **`git cherry` and three-dot diffs cannot attribute a change to a specific unlanded commit.** `git diff main...<branch>` shows every change the branch made since the merge-base, including ones `main` has independently. Use `git show <sha>` to learn what a specific commit contains.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and required fix are specified above; the original triage is in `MILESTONE-M1.done.md` Pass 1, and `## Task P0-M1-FIX-3` shows the standard the corrected rows are held to. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-5 — The `origin/fix/wave-merge-state-loss` abandon reason states a mechanism `main` does not have

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `docs/branch-triage.md` | Line 115 — the `origin/fix/wave-merge-state-loss` row's Reason cell rewritten in full. |
| `.belmont/features/throughput/NOTES.md` | Two entries: the non-destructive `git merge-tree` probe, and the residual non-`PROGRESS.md` last-writer-wins in `syncFeatureStateAfterMerge`. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-5` marked `[x]`. |

**Tests Added**: none — documentation-only correction. `cmd/belmont/worktree.go` was **not** touched, per the scope boundary.

#### What the old reason claimed, and what is actually true

The row read: *"`main`'s `worktree.go` already scopes the sync per milestone and refuses duplicate headings; `syncFeatureStateAfterMerge` is covered by three test files … Only the branch's own test filename is absent."*

Verified against HEAD (`ad3559de`), every line number read rather than copied from the brief:

- **"scopes the sync per milestone" — false.** `syncFeatureStateAfterMerge(mainRoot, wtPath, slug string)` at `cmd/belmont/worktree.go:370` takes no milestone ID; `os.RemoveAll(dstFeature)` is at `:397` and `copyDir` at `:398`. `grep -rn "resolveMilestoneProgress\|milestoneBlockRange" cmd/` returns nothing — neither symbol exists on `main`.
- **The defect class *is* handled, by a different mechanism.** Master's `PROGRESS.md` is read **before** the wipe (`:395`) and union-merged back afterwards by `mergeProgressState` — defined at `:504`, called at `:410`. The brief's `:504`/`:410` both hold at HEAD.
- **"refuses duplicate headings" — true, but of `mergeProgressState`, not of milestone scoping.** Duplicate `### M<n>:` headings are counted at `:569-585` and warned at `:586-589`; duplicate task IDs get the same refusal, counted `:530-558`, warned `:618-621`. The brief's `:560/588` is the same construct: `:560` is the first line of its explanatory comment, `:588` the warning string. I cited the code range rather than the comment.
- **"only the branch's own test filename is absent" — a large understatement.** `git show --stat 1fcbc7d4` (the branch's single unlanded commit, `git cherry -v main` = 1 outstanding): 418 insertions across 7 files, including a **251-line** `cmd/belmont/sync_feature_state_test.go` carrying **11 tests**, and a complete alternative implementation — a `milestoneID` parameter on `syncFeatureStateAfterMerge`, `resolveMilestoneProgress` + `milestoneBlockRange` (a line-exact splice of the worktree's own milestone block onto master's file), and both `auto_parallel.go` call sites updated.

Per the trap `P0-M1-FIX-3` recorded, the commit's content was read with `git show 1fcbc7d4`, never inferred from `git diff main...<branch>`.

#### Verdict: abandon stands, and is now supported

Kept as **abandon**. Reading both sides strengthened it rather than changing it:

- `main`'s per-task-ID union merge is **stricter** than the branch's per-milestone-block splice: most-advanced-state-wins by `markerRank` (`state.go:260`), `[!]` wins in both directions, unrecognised markers never ranked, master-only tasks carried into their milestone.
- `main` covers the branch's own scenario: `TestSyncPreservesSiblingCompletions` in `worktree_sync_test.go` (15 tests) is the two-sibling clobber the branch commit describes; `merge_symmetry_test.go` and `worktree_state_test.go` also exercise the path. All three named test files exist on `main`; only `sync_feature_state_test.go` does not.
- `main`'s `knowledge/auto-mode/worktree-state-isolation.md:44-58` (§*PROGRESS.md is merged on sync, never replaced*) is a superset of the branch's 28-line edit to the same file.
- The branch no longer applies where it sits: `git merge-tree --write-tree --name-only main origin/fix/wave-merge-state-loss` conflicts in four files, `worktree.go` among them. (Probe only — no ref, index or working tree was modified.)

**One honest residual, now recorded in the row.** The branch's milestone mode skipped the `RemoveAll` altogether, so a non-`PROGRESS.md` file an earlier sibling merge had left in the feature dir survived — pinned by `TestWaveMergeDoesNotDeleteSiblingFiles`, for which `main` has no equivalent. `main` still wipes the whole directory at `:397`, so every file in the feature dir except `PROGRESS.md` remains last-writer-wins across a wave. That is a gap to close on `main`, not a reason to resurrect the branch, and the row says so. It is consistent with what NOTES.md already records about `worktree.go:397` under Polish.

**Verification Results**:
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass
- `go test -tags eval ./cmd/belmont`: pass
- `go vet ./...`: pass
- `staticcheck ./...`: pass (no output)
- `gofmt -l cmd/`: clean (no output)
- `belmont validate --root /Users/benlavender/repos/belmont`: ✓ No milestone-structure violations found
- **Line-citation gate**: every line reference written into the row was re-read at HEAD by script and matched its expected content exactly — `worktree.go` 370, 395, 397, 398, 410, 504, 530, 558, 569, 585, 586, 589, 618, 621; `state.go:260`; `worktree-state-isolation.md` 44 (§ heading) through 58 (last bullet before the next `## `).

**Self-Validation**:
- Acceptance Criteria: 3/3 — mechanism named with verified citations; the "only a test filename" characterisation corrected; verdict held at abandon with evidence that supports it.
- Visual Check: N/A (documentation).
- Table integrity: the row is a single line with exactly 4 pipes, no bare `|` in the prose.

**Scope adherence**: `worktree.go` untouched. `P0-M1-FIX-3`'s work (the `origin/feat/maintenance-ci` row, the headline bullet, the abandon-section preamble, the measurement-traps section) read for its evidence bar and left byte-for-byte unchanged. `P0-M1-FIX-6`, `P0-M1-FIX-7` and the three `[!]` tasks untouched. No `belmont install`. No push, merge, rebase, cherry-pick or branch deletion — the only git commands run were read-only (`log`, `show`, `cherry`, `cat-file`, `grep`, `merge-base`, `merge-tree --write-tree`).

**Commit**:
- **Hash**: the single commit on `main` whose subject is the message below (recorded by subject rather than hash — this log lives inside the commit it would name; same convention as `P0-M1-FIX-3`)
- **Message**: `P0-M1-FIX-5: Name the mechanism main actually uses in the wave-merge-state-loss verdict`

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-5 | `docs/branch-triage.md` headline bullet 2 lists `fix/wave-merge-state-loss` among collisions *"already on `main`"*. Its one commit `1fcbc7d4` is genuinely unlanded; what is on `main` is a different fix for the same defect. The bullet's conclusion (M3/M4/M6 uncontested) survives, but the wording overstates. Not corrected here — bullet edits belong to whoever owns that bullet, and this task's scope is the row. | P2 |
| FWLUP-2 | P0-M1-FIX-5 | `syncFeatureStateAfterMerge`'s `os.RemoveAll(dstFeature)` (`worktree.go:397`) still loses non-`PROGRESS.md` feature-dir files written by an earlier sibling merge in the same wave. The abandoned branch fixed this and pinned it with a test; `main` has neither. Closing it means either an overlay copy or an explicit preserve-list, plus a regression test. | P2 |

### Notes for Verification
- The single load-bearing claim to re-check is the line-citation gate: every number in the new row should resolve at HEAD to what the row says it does. It was checked by script, not by eye.
- The verdict was **not** flipped. If a reviewer disagrees, the argument to engage is the residual `RemoveAll` gap (FWLUP-2) — that is the only thing the branch does that `main` does not, and the row states it openly rather than burying it.
- `cmd/belmont/worktree.go` is unchanged in this commit; `git show --stat` should confirm only three files, all documentation or `.belmont/` tracking.
