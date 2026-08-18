# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `d2ae8061` (run `git rev-parse HEAD` to confirm)
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 3 of 7.
- **Created**: 2026-08-18T13:55:00Z
- **Tasks**:
  - [x] P0-M1-FIX-3: `origin/feat/maintenance-ci` wrongly verdicted abandon — commit `1129cf84` is not on `main`

## Orchestrator Context

### Current Task
`P0-M1-FIX-3` — the only task in this run. A wrong "already landed" verdict on an **abandon** recommendation, which would destroy real content if acted on.

### Active Task IDs
`P0-M1-FIX-3`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-13 holds the parent task's acceptance criteria
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` — Pass 1 holds the original triage; `## Task P0-M1-FIX-1` records the knowledge-file amendment that will collide with this cherry-pick
- **The artefact to correct**: `docs/branch-triage.md` (the `origin/feat/maintenance-ci` row, around line 99)
- **The file the cherry-pick touches**: `knowledge/auto-mode/parallel-wave-orchestration.md`

### The defect

`docs/branch-triage.md` verdicts `origin/feat/maintenance-ci` as **abandon**, on the stated ground that its two apparent leftover commits are both already present on `main`.

Only one is. `7a9b7b56` (worktree-state-isolation) is on `main`. **`1129cf84` is not** — verified: `git show --stat 1129cf84` is a 23-line addition to `knowledge/auto-mode/parallel-wave-orchestration.md` titled *"knowledge: record that --max-parallel is inert without explicit deps"*, and `grep` for its heading on `main` returns nothing.

The triage additionally **mis-describes** that leftover as "the `main.go` → `autocmd.go` repointing in `clean-tree-preflight.md`", which is a different change entirely.

Why this matters beyond bookkeeping: the content records that `runAutoCmd` only reaches `runAutoParallel` when a milestone in range declares `(depends: …)`, so `--max-parallel` is silently inert without one and a smoke fixture without explicit deps proves nothing about the parallel path. That is the exact mechanism `P0-M1-FIX-1`'s Critical defect depended on, and it is directly relevant to M6 and M10. Acting on the abandon verdict would delete it.

### Required fix

1. **Cherry-pick `1129cf84` onto local `main`.** Expect a conflict: `P0-M1-FIX-1` (commit `962f0e84`) amended the same file, adding invariants about `MetricsRoot` and one-wave-one-run. **Resolve by keeping both** — the two additions are complementary, not competing. Do not drop either.
2. **Re-verdict `origin/feat/maintenance-ci`** in `docs/branch-triage.md`: correct the count of outstanding commits, correct the mis-description of what `1129cf84` contains, and state the verdict that now applies given the cherry-pick.
3. **Re-check the other "already landed" verdicts in the same row's reasoning** if they rest on the same evidence method. Verification spot-checked eight branches and found this one wrong; you are not required to re-audit all 35, but say in your log which you checked.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-3`.
- **Out of Scope**: `P0-M1-FIX-5` owns the separate `origin/fix/wave-merge-state-loss` reason correction in the same file — leave that row alone. `P0-M1-FIX-4`, `-6`, `-7` each get their own run.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **LOCAL ONLY.** Cherry-pick onto local `main` and commit. Do **NOT** `git push`. Do **NOT** merge, rebase, delete or otherwise mutate any remote branch. `origin/main` is 165+ commits behind and publishing is the repository owner's decision, tracked as `[!] P0-13a`.
- Do **not** start M2–M11 work.
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
Plus, specific to this task: after the cherry-pick, `grep` for the `--max-parallel` gotcha heading on `main` must succeed, **and** the `MetricsRoot` invariants added by `962f0e84` must still be present in the same file. Both, not either.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this defect's cause: ***"triage verdicts written from summary evidence rather than from content."*** `git cherry` tells you a patch-id is unmatched; it does not tell you whether the *content* landed by another route, and a mechanism that sounds plausible is not evidence that it exists.

**The prevention rule is this task's bar**: an "already landed" verdict must name the commit **and** show its content present on `main` — grep the heading, diff the file. An "abandon" reason must name the superseding mechanism and cite the lines implementing it. **A destructive recommendation carries a higher evidence bar than a keep.** Apply that standard to the verdict you write, not just to the one you are correcting.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and required fix are specified above; the original triage is in `MILESTONE-M1.done.md` Pass 1. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 2 (one cherry-pick, one task commit)

---

### Task: P0-M1-FIX-3 — `origin/feat/maintenance-ci` wrongly verdicted abandon

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `knowledge/auto-mode/parallel-wave-orchestration.md` | `1129cf84` cherry-picked in: §*Gotcha — `--max-parallel` does not by itself produce worktrees* at lines 43–62, plus its `2026-08-07` Revisions entry. Conflict was in the Revisions list only; resolved by keeping **both** sides in date order. |
| `docs/branch-triage.md` | `origin/feat/maintenance-ci` row rewritten; headline bullet corrected; abandon-section preamble caveated; a third measurement trap added; Revisions entry added. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-3` → `[x]`. |
| `.belmont/features/throughput/NOTES.md` | Two Debugging entries (patch-id after a conflicted cherry-pick; identifying a leftover commit from `git show`, not from a three-dot diff). |

**Tests Added**: none — the change is a knowledge file and a triage document. No Go code was touched.

**Verification Results** (every gate in the MILESTONE's block, run at HEAD):
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass
- `go test -tags eval ./cmd/belmont`: pass
- `go vet ./...`: pass
- `staticcheck ./...` (from `$(go env GOPATH)/bin`): pass, no findings
- `gofmt -l cmd/`: empty
- `belmont validate --root /Users/benlavender/repos/belmont`: `✓ No milestone-structure violations found.`
- Zero external Go dependencies; `go.mod` still has no `require` block and there is no `go.sum`.

**Task-specific gate — both knowledge additions survived the conflict**:
```
git grep -q "does not by itself produce worktrees" main -- knowledge/auto-mode/parallel-wave-orchestration.md   → PASS (1129cf84)
git grep -q "loopConfig.MetricsRoot"                main -- knowledge/auto-mode/parallel-wave-orchestration.md   → PASS (962f0e84)
git grep -q "One wave is one run"                   main -- knowledge/auto-mode/parallel-wave-orchestration.md   → PASS (962f0e84)
```
Both, not either. The two additions are disjoint: `1129cf84` adds a `## Gotcha` section between *Failure mode* and *Don't re-do*; `962f0e84` amends the `## Invariant` bullets and the *How it's enforced* / *Evidence* prose. Only their Revisions entries collided.

**What the leftovers actually were**

| Commit | Content | Status |
|---|---|---|
| `1129cf84` | 23 lines: §*Gotcha — `--max-parallel` does not by itself produce worktrees* in `knowledge/auto-mode/parallel-wave-orchestration.md` | **Was absent from `main`.** Cherry-picked as `23e71008` (`-x`, so the commit records its source). |
| `7a9b7b56` | 26 lines: §*Confirmed defect — the second merge in a wave clobbers the first's state* in `knowledge/auto-mode/worktree-state-isolation.md` | **Superseded on `main`** — same defect *and its fix* recorded at `worktree-state-isolation.md:44-58` (§*PROGRESS.md is merged on sync, never replaced*, issue #24, revision `2026-08-08`), implemented by `mergeProgressState` (`cmd/belmont/worktree.go:504`, called from `syncFeatureStateAfterMerge` at `worktree.go:410`). The branch's text records the defect as **open**, so picking it would regress the file. Do not cherry-pick it. |

Neither is the change the original triage named. The two changes it *did* name are both on `main` (`knowledge/KNOWLEDGE.md:25`; `knowledge/auto-mode/clean-tree-preflight.md:13`) — they simply belong to other, landed commits on the same branch. The mis-attribution came from reading `git diff main...<branch>`, which shows every change the branch made since the merge-base including ones `main` has independently, and therefore cannot say which change belongs to which unlanded SHA.

**Verdict now written into `docs/branch-triage.md`**: **abandon stands** — but on evidence, not on a count. `1129cf84` is recovered and cited by line; `7a9b7b56` is superseded and the superseding mechanism is named with its implementing lines. Nothing on the branch is unrecovered.

**Which other verdicts I re-checked** (item 3 of the required fix)
- `origin/fix/worktree-git-excludes` — **re-checked to the new bar and holds.** Its 2 outstanding commits are `39c5f219` (removes `writeWorktreeGitExcludes`) and `db254236` (adds the knowledge entry). `writeWorktreeGitExcludes` is absent from `cmd/`, with three removal comments at `worktree.go:286`, `worktree_state_test.go:60`, `steer_test.go:148`; `39c5f219`'s 81-line `worktree_state_test.go` is on `main`; and `main`'s `worktree-state-isolation.md` is a superset of `db254236`'s version (9 headings vs 8). The row's stated reason is accurate as written.
- `origin/backup/loop-efficiency-pre-rebase` and `origin/backup/state-readers-pre-rebase` — **spot-checked, consistent.** Each lists 14 outstanding commits under `git cherry`, but their rebased counterparts `origin/feat/loop-efficiency` and `origin/fix/state-readers-and-live-views` both report **0** outstanding, which is what the rows claim. I did not content-verify all 14 commits.
- `origin/fix/wave-merge-state-loss` — **deliberately not touched**; it is `P0-M1-FIX-5`'s task.
- The remaining 30 branches were not re-audited.

**Caveat now recorded in the triage doc**: `git cherry -v main origin/feat/maintenance-ci` *still* lists `1129cf84` as outstanding after the cherry-pick, because resolving the conflict changed its context lines and so its patch-id. Re-running §How this was measured will contradict the corrected row; the file content is the authority. Added as trap 3 in that section so the next reader does not "re-discover" the defect.

**Scope compliance**
- Local only. No `git push`, no merge, no rebase, no branch deletion, no remote mutation. `origin/feat/maintenance-ci` is untouched — the verdict remains a *recommendation* to delete, and publishing stays with `[!] P0-13a`.
- `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` untouched. `belmont install` not run.
- No Go source changed; no dependency added.

**Commits**
| Hash | Message |
|---|---|
| `23e71008` | `knowledge: record that --max-parallel is inert without explicit deps` (cherry-picked from `1129cf84`, original authorship preserved, `-x` provenance line) |
| _(this commit — find it with `git log --grep='^P0-M1-FIX-3'`; a commit cannot record its own hash in a file it carries)_ | `P0-M1-FIX-3: Re-verdict origin/feat/maintenance-ci and recover 1129cf84` |

Two commits rather than one: the cherry-pick is kept as its own commit so the original author, message and `cherry picked from commit 1129cf84…` line survive as provenance for the recovery. The task's own changes are in the second.

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-3 | The `## Gotcha` section arrives with a stray double blank line before `## Don't re-do` (present in `1129cf84` as authored). Left as-is to keep the cherry-pick faithful; sweep it with any future edit to the file. | P3 |
| FWLUP-2 | P0-M1-FIX-3 | 30 of the 35 triage rows have never been checked to the evidence bar this task established. The five `abandon — content verified` rows matter most, since each is a recommendation to delete. Two of the five have now been checked; three have not. | P2 |

### Notes for Verification
- The load-bearing check is the **conjunction**: both `1129cf84`'s `--max-parallel` gotcha and `962f0e84`'s `MetricsRoot` / one-wave-one-run invariants must be present in `knowledge/auto-mode/parallel-wave-orchestration.md`. Dropping either to settle the conflict would have reproduced this task's own defect.
- Do not use `git cherry` to confirm the recovery — see the caveat above. Grep the file.
- Every line number cited in the new triage row was read at HEAD, not recalled: `parallel-wave-orchestration.md:43-62`, `autocmd.go:278-284` / `:286`, `worktree-state-isolation.md:44-58`, `worktree.go:504` / `:410`, `KNOWLEDGE.md:25`, `clean-tree-preflight.md:13`, `worktree.go:286`, `worktree_state_test.go:60`, `steer_test.go:148`.
