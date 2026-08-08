# Worktree `.belmont/` State Isolation

**Why this matters.** A worktree holds a full copy of `.belmont/` — every feature's state, not just its own. If the worktree committed that copy, merging it back would overwrite sibling features' state with a stale snapshot taken at fork time. So worktree `.belmont/` must be *partly* invisible to git. But only partly: `MILESTONE.md` is created **inside** the worktree and has to come home, and on the `belmont recover` route git is the **only** way it can. The isolation is therefore deliberately asymmetric, and both halves are load-bearing. Two plausible "fixes" break it in opposite directions.

## Invariant

- **Tracked `.belmont/` files are held back.** `untrackBelmontInWorktree` marks every path returned by `git ls-files .belmont/` as `--assume-unchanged`, so edits and deletions in the worktree never reach the index. This is what stops a merge from clobbering sibling state.
- **New `.belmont/` files are deliberately committable.** They are not in the index at copy time, so `--assume-unchanged` does not apply to them. `commitWorktreeChanges` stages them with `git add -A` and they travel home in the merge commit.
- **That second half is the only return path on `belmont recover`.** The normal merge path calls `syncFeatureStateAfterMerge`, which copies worktree → master on disk. `recoverMerge` does not. So for a recovered worktree, anything not carried by git is lost.
- **There is no per-worktree git exclude, and there must not be one.** `.belmont/` is tracked in the main repo and must stay tracked.

## How it's enforced

In `cmd/belmont/worktree.go`:

- `untrackBelmontInWorktree(wtPath, slug)` — iterates `git ls-files .belmont/` and runs `git update-index --assume-unchanged` per path, best-effort. Called at the end of `copyBelmontStateToWorktree`.
- `commitWorktreeChanges(wtPath, label)` — `git add -A` plus commit, so untracked `.belmont/` files are staged.
- `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` — filesystem copy worktree → master, **except `PROGRESS.md`, which is merged via `mergeProgressState`** (see below). Two call sites; `recoverMerge` is **not** one of them.
- `createWorktreeIfNeeded(root, wtPath, branch, slug, resumed)` — the only place a worktree is created and seeded. Returns early when `resumed`. Both `runFeatureInWorktree` and `runMilestoneInWorktree` call it.
- Merge conflicts involving `.belmont/` files are not special-cased. They hit the generic detection (`strings.Contains(output, "CONFLICT")`, `mergeFailureKind`) and route to the reconciliation agent like any other conflict.

Test coverage in `cmd/belmont/worktree_state_test.go`:
- `TestWorktreeTrackedBelmontEditsAreNotCommittable` — pins the first half.
- `TestWorktreeNewBelmontFilesAreCommittable` — pins the second, and fails with a message pointing at the recover path if an effective per-worktree exclude is reintroduced.

In `cmd/belmont/worktree_sync_test.go`:
- `TestSyncPreservesSiblingCompletions` — the #24 regression: two sibling worktrees merging in sequence, both milestones' work must survive.
- `TestSyncNeverRegressesAState`, `TestSyncPreservesBlockedMarkers`, `TestSyncLeavesUnrecognisedMarkersAlone`, `TestSyncCarriesOverMasterOnlyTasks` — the merge rules.
- `TestCreateWorktreeIfNeededResumeIsNoOp` — the #29 guard, with `TestCreateWorktreeIfNeededSeedsFreshWorktree` as its control proving the hazard is real.

## Failure mode if you break it

- **Make new `.belmont/` files uncommittable.** `MILESTONE.md` and any other worktree-authored state stops reaching main on the `belmont recover` path, silently. The normal merge path still works, which is what makes this so easy to ship — it only shows up after a failed merge.
- **Drop `--assume-unchanged` on tracked files.** The worktree commits its fork-time snapshot of every feature's `.belmont/`, and merging reverts sibling features' progress to whatever it was when the worktree was created.
- **Write the exclude to `$GIT_COMMON_DIR/info/exclude`.** That file is shared with the main repo and every other worktree, where `.belmont/` must stay tracked. Far worse than the bug it would fix.

## Don't re-do

- **`writeWorktreeGitExcludes` — writing `.belmont/` to the worktree's own `$GIT_DIR/info/exclude`.** Removed 2026-08-07. It never worked: git resolves `info/exclude` from `$GIT_COMMON_DIR`, not from a linked worktree's gitdir, so the file was written and never read. Verified at git 2.50.1 — with the exclude in place exactly as the code wrote it, `git check-ignore` exits non-zero and `git status` still reports the file as untracked. It was a no-op for its entire life.
- **"Fix" it by writing to the common dir instead.** Works mechanically, breaks the main repo. See Failure mode above.
- **Make the exclusion effective per-worktree via `extensions.worktreeConfig` + a per-worktree `core.excludesFile`.** This does work — tested. Do not do it. Turning it on makes new `.belmont/` files uncommittable, which strands worktree state on the recover path. The no-op has been load-bearing.
- **Assume `--assume-unchanged` covers new files.** It only applies to paths already in the index. This has been misread at least once in a proposal review.

## PROGRESS.md is merged on sync, never replaced

`syncFeatureStateAfterMerge` reads master's `PROGRESS.md` **before** the wipe and merges it back afterwards via `mergeProgressState`. Every other file in the feature dir still takes the worktree's version wholesale.

This is not a refinement — it is load-bearing. The function runs **once per sibling merge** inside `runWaveParallel`'s merge loop, and sibling worktrees each hold a full copy of the same feature dir from fork time. A destructive replace was therefore last-writer-wins across the wave: M2 merged and master recorded its tasks done, then M3 merged and reverted them to their fork-time state. Because `.belmont/` is `--assume-unchanged`, this copy is the *only* transport, so the flips existed in no commit and were unrecoverable. `resolveProgressConflict` — the union merge built for exactly this shape — never fired, because assume-unchanged means git never registers a conflict on the file. Issue #24.

`mergeProgressState` rules, in order:

- The **worktree's document is the base**, so structure and ordering match what a plain copy produced. Master contributes states and lines, never structure.
- **Most-advanced state wins**, ranked by `markerRank` (canonical `taskStatus`, not raw marker).
- **`[!]` on either side wins, both directions.** Rank alone would clear it — `taskBlocked` sorts below todo, so anything outranks it. The failure modes are asymmetric: a stale `[!]` costs someone a look and auto pauses loudly on blocked tasks; a dropped live one means work is silently treated as fine.
- **Unrecognised markers are never ranked** — left exactly as written and reported as a warning. This check runs **before** the `[!]` rule, not after: a blocked-wins rule that fired first would happily overwrite a `[?]` the user wrote deliberately. See [`cross-cutting/progress-md-parsing.md`](../cross-cutting/progress-md-parsing.md) and issue #27.
- **A duplicated task ID is not merged at all.** Matching is by ID, so a repeated ID makes the match ambiguous and a plain map would silently collapse the entries and merge the wrong line. Both sides are counted first; any non-unique ID is skipped and warned about.
- **The milestones region is bounded by `isSectionBreak`, same as every other reader.** The merge tracks the current milestone from `### M<n>:` headers *and* resets it at a column-zero `## `. Without that it was the one reader disagreeing after #31: a task under `## Session History` was attributed to the last milestone header seen, and a master-only orphan was spliced into that milestone. Orphans on either side are now left exactly as written and never adopted.
- **Task lines only master has are carried over** into their milestone (a follow-up a sibling added after this worktree forked). If the milestone is absent from the worktree's copy the task is skipped with a warning, never silently dropped.

## Worktree seeding is skipped on resume

`createWorktreeIfNeeded(root, wtPath, branch, slug, resumed)` is the single entry point for creating a worktree and seeding its state, and it is a **no-op when `resumed` is true**. Seeding calls `copyBelmontStateToWorktree`, a destructive replace of the feature dir — run against a preserved worktree it overwrites the live `PROGRESS.md` with master's fork-time snapshot, and those flips are in no commit. `resume-rebase.md` rejects exactly this under *Don't re-do*.

This lived inline in two callers that drifted: `runFeatureInWorktree` guarded it, `runMilestoneInWorktree` did not (issue #29). Both now route through the helper so they cannot diverge again.

**The resume path still re-arms `untrackBelmontInWorktree`.** Skipping the copy must not skip the untracking: `--assume-unchanged` is index state, and `handleStaleWorktree` can reattach a worktree whose directory was removed by running `git worktree add` again, which builds a fresh index with the bits cleared. Seeding used to re-arm them as a side effect, so guarding the copy removed the only re-arming — and without it the worktree's `.belmont/` edits become committable and merge back over sibling state, the exact failure this entry exists to prevent. It is idempotent and cheap, so `createWorktreeIfNeeded` always calls it. Pinned by `TestResumeReArmsUntracking`. `STEERING.md` preservation inside `copyBelmontStateToWorktree` remains — it protects the fresh-seed path, not this one.

## Known open gap

`recoverMerge` does not call `syncFeatureStateAfterMerge`. Worktree edits to **tracked** feature state are therefore lost on the recover path — they are held back by `--assume-unchanged`, and nothing copies them home afterwards. Only *new* files survive, via git.

**The objection to the one-line fix is now weaker, but not gone.** It used to be that dropping `syncFeatureStateAfterMerge` into `recoverMerge` would `RemoveAll` master's feature dir and replace it with a copy preserved from a *failed* merge, possibly hours or days stale — trading a defect that loses worktree edits for one that loses main's. With `PROGRESS.md` now merged rather than replaced, a stale worktree can no longer drag task state backwards: the most-advanced state wins regardless of which side is older.

What still needs a decision is **every other file** in the feature dir — `MILESTONE.md`, `NOTES.md`, archived `MILESTONE-*.done.md` — which is still a wholesale replace and would still let a stale recovered worktree clobber newer master copies. Scope any fix to that question. Whichever is chosen, the copy must happen **before** `removeWorktree`, which currently runs immediately after the merge in `recoverMerge`.

## Evidence

- `writeWorktreeGitExcludes` removal, 2026-08-07. Found while reviewing worktree lifecycle for an unrelated proposal, not by any automated check. Both replacement approaches were tested against a real git repo before being rejected.
- Unit coverage: `cmd/belmont/worktree_state_test.go`. Both tests were checked against controls — they pass on the real code, and reintroducing an effective per-worktree exclude makes the second fail.

## Revisions

- 2026-08-07 — initial. Records the asymmetric isolation (tracked held back, new files committable), the removal of `writeWorktreeGitExcludes`, the two rejected repairs, and the `recoverMerge` gap.
- 2026-08-07 — `cmd/belmont/main.go` split into 22 files in the same package; file paths in this entry repointed to their new homes. Symbol names are unchanged and remain the durable identifier.
- 2026-08-08 — `syncFeatureStateAfterMerge` now merges `PROGRESS.md` instead of replacing it (`mergeProgressState`), fixing the last-writer-wins wave bug (#24); worktree seeding extracted to `createWorktreeIfNeeded` with the resume guard in one place (#29). `recoverMerge` gap re-scoped: task state is now safe, non-PROGRESS files are still a wholesale replace.
- 2026-08-08 — red-team follow-ups: recognition check moved ahead of the `[!]` rule, duplicate task IDs declined rather than collapsed, and the resume path re-arms `untrackBelmontInWorktree` (skipping the copy had removed the only re-arming).
- 2026-08-08 — `mergeProgressState` now honours `isSectionBreak`, closing the last reader that disagreed about where the milestones region ends (#31 ↔ #24 seam).
