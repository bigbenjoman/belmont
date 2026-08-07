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
- `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` — filesystem copy worktree → master. Two call sites; `recoverMerge` is **not** one of them.
- Merge conflicts involving `.belmont/` files are not special-cased. They hit the generic detection (`strings.Contains(output, "CONFLICT")`, `mergeFailureKind`) and route to the reconciliation agent like any other conflict.

Test coverage in `cmd/belmont/worktree_state_test.go`:
- `TestWorktreeTrackedBelmontEditsAreNotCommittable` — pins the first half.
- `TestWorktreeNewBelmontFilesAreCommittable` — pins the second, and fails with a message pointing at the recover path if an effective per-worktree exclude is reintroduced.

## Failure mode if you break it

- **Make new `.belmont/` files uncommittable.** `MILESTONE.md` and any other worktree-authored state stops reaching main on the `belmont recover` path, silently. The normal merge path still works, which is what makes this so easy to ship — it only shows up after a failed merge.
- **Drop `--assume-unchanged` on tracked files.** The worktree commits its fork-time snapshot of every feature's `.belmont/`, and merging reverts sibling features' progress to whatever it was when the worktree was created.
- **Write the exclude to `$GIT_COMMON_DIR/info/exclude`.** That file is shared with the main repo and every other worktree, where `.belmont/` must stay tracked. Far worse than the bug it would fix.

## Don't re-do

- **`writeWorktreeGitExcludes` — writing `.belmont/` to the worktree's own `$GIT_DIR/info/exclude`.** Removed 2026-08-07. It never worked: git resolves `info/exclude` from `$GIT_COMMON_DIR`, not from a linked worktree's gitdir, so the file was written and never read. Verified at git 2.50.1 — with the exclude in place exactly as the code wrote it, `git check-ignore` exits non-zero and `git status` still reports the file as untracked. It was a no-op for its entire life.
- **"Fix" it by writing to the common dir instead.** Works mechanically, breaks the main repo. See Failure mode above.
- **Make the exclusion effective per-worktree via `extensions.worktreeConfig` + a per-worktree `core.excludesFile`.** This does work — tested. Do not do it. Turning it on makes new `.belmont/` files uncommittable, which strands worktree state on the recover path. The no-op has been load-bearing.
- **Assume `--assume-unchanged` covers new files.** It only applies to paths already in the index. This has been misread at least once in a proposal review.

## Known open gap

`recoverMerge` does not call `syncFeatureStateAfterMerge`. Worktree edits to **tracked** feature state are therefore lost on the recover path — they are held back by `--assume-unchanged`, and nothing copies them home afterwards. Only *new* files survive, via git.

**The obvious one-line fix is unsafe.** `syncFeatureStateAfterMerge` is not additive — it does `os.RemoveAll(dstFeature)` and then `copyDir(src, dst)`, destructively replacing master's feature state with the worktree's. Its two existing call sites fire moments after a merge in the same run, where the worktree is by construction the freshest copy.

A recovered worktree is not. It was preserved from a *failed* merge and may predate other features' merges by hours or days. Dropping `syncFeatureStateAfterMerge` into `recoverMerge` would `RemoveAll` master's feature dir and replace it with that stale copy — trading a defect that loses worktree edits for one that loses main's.

So the fix needs a decision first, not a call: replace unconditionally, merge the two states, copy only files newer than their master counterpart, or prompt. Whichever is chosen, the copy must happen **before** `removeWorktree`, which currently runs immediately after the merge in `recoverMerge`.

## Evidence

- `writeWorktreeGitExcludes` removal, 2026-08-07. Found while reviewing worktree lifecycle for an unrelated proposal, not by any automated check. Both replacement approaches were tested against a real git repo before being rejected.
- Unit coverage: `cmd/belmont/worktree_state_test.go`. Both tests were checked against controls — they pass on the real code, and reintroducing an effective per-worktree exclude makes the second fail.

## Revisions

- 2026-08-07 — initial. Records the asymmetric isolation (tracked held back, new files committable), the removal of `writeWorktreeGitExcludes`, the two rejected repairs, and the `recoverMerge` gap.
- 2026-08-07 — `cmd/belmont/main.go` split into 22 files in the same package; file paths in this entry repointed to their new homes. Symbol names are unchanged and remain the durable identifier.
