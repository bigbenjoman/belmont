# Worktree `.belmont/` State Isolation

**Why this matters.** A worktree holds a full copy of `.belmont/` — every feature's state, not just its own. If the worktree committed that copy, merging it back would overwrite sibling features' state with a stale snapshot taken at fork time. So worktree `.belmont/` must be *partly* invisible to git. But only partly: `MILESTONE.md` is created **inside** the worktree and has to come home, and on the `belmont recover` route git is the **only** way it can. The isolation is therefore deliberately asymmetric, and both halves are load-bearing. Two plausible "fixes" break it in opposite directions.

## Invariant

- **Tracked `.belmont/` files are held back.** `untrackBelmontInWorktree` marks every path returned by `git ls-files .belmont/` as `--assume-unchanged`, so edits and deletions in the worktree never reach the index. This is what stops a merge from clobbering sibling state.
- **New `.belmont/` files are deliberately committable.** They are not in the index at copy time, so `--assume-unchanged` does not apply to them. `commitWorktreeChanges` stages them with `git add -A` and they travel home in the merge commit.
- **That second half is the only return path on `belmont recover`.** The normal merge path calls `syncFeatureStateAfterMerge`, which copies worktree → master on disk. `recoverMerge` does not. So for a recovered worktree, anything not carried by git is lost.
- **There is no per-worktree git exclude, and there must not be one.** `.belmont/` is tracked in the main repo and must stay tracked.
- **The post-merge sync is milestone-scoped whenever the worktree covered one milestone.** `syncFeatureStateAfterMerge` takes a `milestoneID`. Empty means the worktree covered the whole feature and its copy replaces master's wholesale. Non-empty means one milestone of a parallel wave: nothing is deleted, and only that milestone's block of `PROGRESS.md` is taken from the worktree. Master is authoritative for everything else, because siblings that merged earlier in the same wave wrote marks this worktree forked before and has never seen. See "Failure mode" below and issue #24.

## How it's enforced

In `cmd/belmont/worktree.go`:

- `untrackBelmontInWorktree(wtPath, slug)` — iterates `git ls-files .belmont/` and runs `git update-index --assume-unchanged` per path, best-effort. Called at the end of `copyBelmontStateToWorktree`.
- `commitWorktreeChanges(wtPath, label)` — `git add -A` plus commit, so untracked `.belmont/` files are staged.
- `syncFeatureStateAfterMerge(mainRoot, wtPath, slug, milestoneID)` — filesystem copy worktree → master. Two call sites, both in `auto_parallel.go`; `recoverMerge` is **not** one of them.
  - `mergeFeatureBranch` passes `""` (whole-feature worktree, multi-feature mode) → `os.RemoveAll` + `copyDir`, unchanged.
  - `mergeWorktreeBranch` passes the milestone ID (single-feature parallel wave) → no `RemoveAll`, `copyDir` overlays, then `PROGRESS.md` is rewritten from `resolveMilestoneProgress`.
- `resolveMilestoneProgress(masterRaw, worktreeRaw, milestoneID)` — line-exact splice. Master's copy is the base; the worktree supplies the **body** of its own milestone block (task marks, follow-ups its verify phase added, tasks triage removed). The **header** line stays master's, because milestone names and `(depends: …)` annotations are immutable outside `/belmont:tech-plan` — see [cross-cutting/milestone-immutability.md](../cross-cutting/milestone-immutability.md). Block boundaries match `parseProgressSnapshot`, so every byte outside the block survives verbatim. Degenerate cases fail safe: no master block → take the worktree's file; no worktree block → keep master's.
- Merge conflicts involving `.belmont/` files are not special-cased. They hit the generic detection (`strings.Contains(output, "CONFLICT")`, `mergeFailureKind`) and route to the reconciliation agent like any other conflict.

Test coverage in `cmd/belmont/worktree_state_test.go`:
- `TestWorktreeTrackedBelmontEditsAreNotCommittable` — pins the first half.
- `TestWorktreeNewBelmontFilesAreCommittable` — pins the second, and fails with a message pointing at the recover path if an effective per-worktree exclude is reintroduced.

Test coverage in `cmd/belmont/sync_feature_state_test.go` (issue #24):
- `TestWaveMergeKeepsEarlierSiblingMarks` / `…ReversedOrder` — the regression itself, both merge orders.
- `TestWaveMergeCarriesFollowUpTasks`, `TestWaveMergeHonoursTaskRemovalInOwnMilestone` — the worktree still owns its own block's contents, in both directions.
- `TestWaveMergeDoesNotDeleteSiblingFiles` — dropping the `RemoveAll`.
- `TestWholeFeatureSyncStillReplacesWholesale` — pins the `milestoneID == ""` path, including that it still deletes stale files.
- `TestResolveMilestoneProgress*` — header preservation and the two degenerate cases.

## Failure mode if you break it

- **Make new `.belmont/` files uncommittable.** `MILESTONE.md` and any other worktree-authored state stops reaching main on the `belmont recover` path, silently. The normal merge path still works, which is what makes this so easy to ship — it only shows up after a failed merge.
- **Drop `--assume-unchanged` on tracked files.** The worktree commits its fork-time snapshot of every feature's `.belmont/`, and merging reverts sibling features' progress to whatever it was when the worktree was created.
- **Write the exclude to `$GIT_COMMON_DIR/info/exclude`.** That file is shared with the main repo and every other worktree, where `.belmont/` must stay tracked. Far worse than the bug it would fix.
- **Make the wave's post-merge sync a whole-directory replace again.** This was the shipped behaviour until 2026-08-07 and it silently lost verified marks in every parallel wave — issue #24. Siblings merge sequentially into the same main repo, so `RemoveAll` + `copyDir` is last-writer-wins on the whole feature dir: the final milestone's fork-time `PROGRESS.md` overwrote every mark its siblings had earned. The task's code was on `main`, its task-ID commit was in history, and `PROGRESS.md` said `[ ]`. Nothing caught it: `runEvidenceCheck` reverts `[v]` marks that *lack* commit evidence, and this is the mirror image — a legitimate `[v]`, with evidence, reverted to `[ ]`. The wider the wave, the more marks lost.

## Don't re-do

- **`writeWorktreeGitExcludes` — writing `.belmont/` to the worktree's own `$GIT_DIR/info/exclude`.** Removed 2026-08-07. It never worked: git resolves `info/exclude` from `$GIT_COMMON_DIR`, not from a linked worktree's gitdir, so the file was written and never read. Verified at git 2.50.1 — with the exclude in place exactly as the code wrote it, `git check-ignore` exits non-zero and `git status` still reports the file as untracked. It was a no-op for its entire life.
- **"Fix" it by writing to the common dir instead.** Works mechanically, breaks the main repo. See Failure mode above.
- **Make the exclusion effective per-worktree via `extensions.worktreeConfig` + a per-worktree `core.excludesFile`.** This does work — tested. Do not do it. Turning it on makes new `.belmont/` files uncommittable, which strands worktree state on the recover path. The no-op has been load-bearing.
- **Assume `--assume-unchanged` covers new files.** It only applies to paths already in the index. This has been misread at least once in a proposal review.

## Known open gaps

### `recoverMerge` never syncs

`recoverMerge` does not call `syncFeatureStateAfterMerge`. Worktree edits to **tracked** feature state are therefore lost on the recover path — they are held back by `--assume-unchanged`, and nothing copies them home afterwards. Only *new* files survive, via git.

**The obvious one-line fix is unsafe.** Neither mode of `syncFeatureStateAfterMerge` is a merge in the git sense. The whole-feature mode does `os.RemoveAll(dstFeature)` then `copyDir(src, dst)`, destructively replacing master's feature state with the worktree's. The milestone-scoped mode is additive across milestones, but *within* the named milestone's block the worktree still wins outright. Both existing call sites fire moments after a merge in the same run, where the worktree is by construction the freshest copy.

A recovered worktree is not. It was preserved from a *failed* merge and may predate other features' merges by hours or days. Dropping `syncFeatureStateAfterMerge` into `recoverMerge` would `RemoveAll` master's feature dir and replace it with that stale copy — trading a defect that loses worktree edits for one that loses main's.

So the fix needs a decision first, not a call: replace unconditionally, merge the two states, copy only files newer than their master counterpart, or prompt. Whichever is chosen, the copy must happen **before** `removeWorktree`, which currently runs immediately after the merge in `recoverMerge`.

### Non-`PROGRESS.md` files are still last-writer-wins within a wave

The #24 fix narrows `PROGRESS.md` only. Every other file in the feature dir is still overlaid by `copyDir`, so when two siblings in a wave both edit the same file — `NOTES.md` is the realistic one, since post-verify triage moves deferrable follow-ups into it — the later merge's copy wins and the earlier one's additions are gone. Dropping the `RemoveAll` fixed the half where a sibling's *new* files were deleted; same-name content is untouched.

This was left out deliberately. `PROGRESS.md` has a parseable block structure that makes a scoped splice exact; prose does not, and a union of two prose files is a design call rather than a patch. A real fix probably reads the fork-time copy — which is recoverable, since `--assume-unchanged` means the branch's committed version *is* the fork-time version — and does a three-way merge.

## Evidence

- `writeWorktreeGitExcludes` removal, 2026-08-07. Found while reviewing worktree lifecycle for an unrelated proposal, not by any automated check. Both replacement approaches were tested against a real git repo before being rejected.
- Unit coverage: `cmd/belmont/worktree_state_test.go`. Both tests were checked against controls — they pass on the real code, and reintroducing an effective per-worktree exclude makes the second fail.
- Issue #24, 2026-08-07. Found while smoke-testing PR #23 on a two-milestone fixture (`M2`/`M3`, both `(depends: M1)`). Not a regression from the `main.go` split: a binary built from the pre-split commit lost `P1-M2-1` identically. Fixed the same day by narrowing the wave's sync to the merged milestone. The new tests in `cmd/belmont/sync_feature_state_test.go` were checked against four deliberate controls — reverting to the whole-directory replace, taking the worktree's header line, ignoring the worktree's body, and returning the worktree file when it has lost its own block — and each control fails the tests it should.

## Revisions

- 2026-08-07 — initial. Records the asymmetric isolation (tracked held back, new files committable), the removal of `writeWorktreeGitExcludes`, the two rejected repairs, and the `recoverMerge` gap.
- 2026-08-07 — `cmd/belmont/main.go` split into 22 files in the same package; file paths in this entry repointed to their new homes. Symbol names are unchanged and remain the durable identifier.
- 2026-08-07 — issue #24: `syncFeatureStateAfterMerge` gained a `milestoneID` parameter and a milestone-scoped mode (`resolveMilestoneProgress`). Recorded as an invariant, a `Don't re-do`, and a second known gap for the non-`PROGRESS.md` files the fix deliberately leaves alone.
