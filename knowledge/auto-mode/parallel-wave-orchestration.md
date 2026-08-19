# Parallel Wave Orchestration

**Why this matters.** The auto loop's hard work is orchestrating multiple milestones in parallel via git worktrees: creating them, copying state into them, running the loop inside each, merging their branches back in milestone-ID order, surfacing conflicts, and cleaning up. Every corner we cut here (shortcuts, symlinks, silent merges) has come back as a silent-data-loss bug. Uniform behavior per wave is worth the small startup cost.

## Invariant

- Every wave — including single-milestone waves — runs through the worktree path. No master-tree shortcut.
- Each worktree has isolated `.belmont/` state (a copy, not a symlink). The agent commits state changes to `belmont/auto/<feature>/<milestone>` as part of its work.
- Worktree-local files that master never holds (`STEERING.md`, and potentially others added later) are preserved across the resume-time wipe-and-recopy.
- Merges happen in milestone-ID order, sequentially, with pre-merge overlap reporting.
- **`MaxParallel <= 1` interleaves merges with execution.** When the user passes `--max-parallel=1`, both `runWaveParallel` (single-feature) and `runAutoMultiFeature` (multi-feature) take a serial branch: run unit N → merge unit N → run unit N+1. The merge happens inline before the next worktree is created so subsequent units fork from the post-merge tip. Stale-worktree resolution and rebase-on-resume are deferred to just-in-time in this branch. With `MaxParallel > 1` the parallel-then-post-wave-merge sequence above remains the only path. This is **not** the previously rejected master-tree shortcut — every unit still runs in its own worktree, only the merge timing changes.
- **Anything that must outlive the wave does not live under `Root`.** The worktree is deleted by `removeWorktree` on merge, `.belmont/` is `--assume-unchanged` inside it so no commit carries it out, and `syncFeatureStateAfterMerge` copies back only `PROGRESS.md`. `.belmont/metrics/` is the case that proved it: gitignored as well, so records written under the worktree were destroyed unread. `loopConfig.MetricsRoot` pins them to the originating root, and `worktreeLoopConfig` is where both worktree sites get it. **If you add a path root to `loopConfig`, decide there which of the two roots it resolves to** — see [cross-cutting/dual-invocation-paths.md](../cross-cutting/dual-invocation-paths.md).
- **One wave is one run.** `RunID` is minted by `runAutoParallel` / `runAutoMultiFeature` before any worktree is created and carried into each child, not left to `runLoop`'s own start-time fallback. `belmont metrics` aggregates per run, so N worktrees minting N IDs would report one invocation as N runs.
- Live state is observable from outside the run via `belmont status --feature <slug>`, which per-milestone overlays each worktree's view of its own milestone on top of master's baseline.

## How it's enforced

In `cmd/belmont/auto_parallel.go`:

- `runAutoParallel` unconditionally dispatches to `runWaveParallel` for every wave (no single-milestone master-tree shortcut; that was removed 2026-04-22). The shortcut helper `singleMilestoneHasExistingWorktree` was deleted.
- `runWaveParallel` → `runMilestoneInWorktree`:
  - `git worktree add -b belmont/auto/<feature>/<ms>`
  - `copyBelmontStateToWorktree` overlays master's feature state on top of the worktree's HEAD checkout. Preserves `STEERING.md` (and future peers — see `ensureMigrations` if more appear) across the wipe-and-recopy.
  - Setup hooks run with the worktree's `$PORT` / `$BELMONT_PORT` / `$BELMONT_BASE_URL` etc. (see [cross-cutting/port-isolation.md](../cross-cutting/port-isolation.md)).
  - `runLoop(mCfg)` executes inside the worktree, with `mCfg` built by `worktreeLoopConfig(cfg, wtPath)` — the single place that decides which of a config's roots follows the worktree and which stays behind. `Root = wtPath` scopes everything the loop reads and writes to the worktree; `MetricsRoot` and `RunID` stay on the originating run.
- Merge loop in `runWaveParallel`:
  - Sort successes by `parseMilestoneNum`.
  - Before each merge, `reportMergeOverlap(cfg.Root, branch, msID, mergedFiles)` prints a visibility warning listing files the branch touches that earlier-merged siblings also touched. **Does not block** — scope guards + verify evidence + milestone-immutability should catch the cases where overlap implies scope leak; this is diagnostic so a human can still review before pushing.
  - Record this branch's touched files in `mergedFiles` for the next iteration's overlap check.
- Live status in `buildStatus`: when `loadAutoWorktreeStateByMilestone` returns a non-empty map, `overlayLiveMilestones` replaces each active milestone's tasks with the worktree's current view. Each overlaid milestone carries a `LiveFrom` pointer so the renderer tags it `(live from worktree)`.
- **The overlay reports every milestone it could not source, and that is not optional for a caller to ignore.** It has two fallbacks to master — the worktree's `PROGRESS.md` cannot be read, or it parses but holds no such milestone — and both used to be silent. That was defensible while the overlay governed *display*; it stopped being defensible when #42 routed `belmont validate` through it and #44 routed `belmont auto`'s startup gate through `validateFeature`. A half-cleaned worktree then made validate print `✓ No milestone-structure violations found` and let a run start — a false green in the one place a false green starts a run. `overlayLiveMilestones` now returns `[]liveOverlayGap` alongside the milestones; `status` (detail and listing), `blockers` and `validate` each state it, and validate raises `unreadable_live_milestone` at **warning** severity so a stale worktree directory cannot refuse a run that worked yesterday. The whole-feature override on the serial / multi-feature path had the identical hole — `loadAutoWorktrees` admits a worktree on the strength of its feature *directory* existing — and `validateFeature` reports that one too. The gap carries a `Kind`, because the two shapes do not cover the same thing: under `gapUnreadable` nothing in that worktree was seen, while under `gapMilestoneAbsent` the file read fine and its own violations *are* in the report, so one message for both is false for the second. Issue #48.
- **Never point a live-run diagnostic at `belmont recover`.** These warnings can only fire while `auto.json` is active, and `listPreservedWorktrees` scans `worktreeBasePath(root)` — the exact directory `runAutoParallel` creates the running wave's worktrees in. The messages name the unreadable path instead. (The missing active-run filter in `recover` itself **was** a separate hazard — `recover --list` mid-run reported the live worktrees as preserved and offered `--clean-all` on them. Fixed in #52: `activeRunWorktrees` reads the same `auto.json` and `refuseIfRunOwns` declines, with `--force` as the deliberate override. This bullet still stands regardless — a diagnostic that fires *because* a run is live should never route the user to the command whose whole job is disposing of dead worktrees.)
- `auto.json` schema carries `mode` (`"single-feature-parallel"` or `"multi-feature"`) and `feature` slug so readers can tell per-milestone-worktree runs from feature-per-worktree runs.

## Failure mode if you break it

- **Re-introducing the single-milestone shortcut**: M1 runs in master tree; scope-guard amends rewrite the user's working branch history directly; `belmont steer` targets the wrong root; rollback is a `git reset` rather than `worktree remove`. Asymmetry between waves makes every other mechanism harder to reason about.
- **Not preserving STEERING.md across state copy**: the resume-time wipe-and-recopy silently deletes any pending user instructions that landed before auto resumed. User's steer reports success; zero injection fires. (This was the 2026-04-21 STEERING.md loss bug.)
- **Missing merge overlap report**: two branches write the same file, git picks one arbitrarily, the other's work disappears. Only detectable later when the feature "looks wrong." (This was the hero-section.tsx overwrite in the about-2 run.)
- **Writing something durable under the worktree root**: it disappears on merge with no error anywhere. Metrics did exactly this — the instrumentation an entire feature was to be judged by recorded nothing in the only mode that feature runs in, and the symptom was an absent file, not a failure.
- **Missing live status overlay**: user has no way to observe parallel work in progress; has to wait until merge to see whether M2 is stuck or making progress. Blind flying on 30–60 minute wave durations.

## Gotcha — `--max-parallel` does not by itself produce worktrees

`runAutoCmd` only dispatches to `runAutoParallel` when at least one milestone in the selected range declares a dependency (the `hasExplicitDeps` loop in `cmd/belmont/autocmd.go`). With no `(depends: …)` anywhere, `--max-parallel 2` is silently ignored and the run goes through `runLoop` in the master tree.

This matters most when writing a smoke test. A fixture with two independent milestones and `--max-parallel 2` looks like it covers the parallel path, creates no worktrees, and passes — proving nothing about `auto_parallel.go`, `worktree.go` or `copyBelmontStateToWorktree`. Observed exactly this on 2026-08-07 while validating the `main.go` split.

To force a genuine concurrent wave, pre-mark a baseline milestone `[v]` and give two others the same dependency on it:

```
### M1: Baseline
- [v] P1-M1-1 Baseline exists

### M2: Thing A (depends: M1)
- [ ] P1-M2-1 …

### M3: Thing B (depends: M1)
- [ ] P1-M3-1 …
```

M1 is skipped as already verified; M2 and M3 form one wave and get a worktree each. Confirm with `.belmont/auto.json` — it should report `mode: single-feature-parallel` with an entry per milestone.


## Don't re-do

- **Master-tree shortcut for single-milestone waves.** Was in place as an optimization to save ~5–10s of worktree setup. Cost: asymmetric behavior per wave, scope-guard amends on the wrong branch, confused `belmont steer` targeting. Rejected in the same session it was diagnosed; do not bring it back even under a flag. If worktree setup ever becomes a real bottleneck, make setup faster.
- **Distinguish from `MaxParallel <= 1` serial inline-merge.** That path is allowed and intentional: every unit still runs in its own worktree, only the merge happens before the next unit starts so cross-unit implicit deps resolve at the fork point. No master-tree elision, no scope-guard rewrite, no steering-target confusion.
- **Symlinked `.belmont/` state across worktrees.** Was the pre-2025 default. Resulted in state races: worktree A's agent could read PROGRESS.md mid-flip while worktree B was writing. The copy-based isolation solved it; the merge-time reconciliation via `mergeWorktreeBranch` + reconciliation-agent handles the inevitable conflicts semantically.
- **Auto-serialize-on-directory-overlap** (detect that two milestones touch the same files at plan time and force serial execution). Heuristic is unreliable before either milestone runs. Over-serializes conservatively, kills the point of parallel mode. Scope guard + merge overlap report between them give the same coverage at run time without the heuristic.
- **Caching or daemonizing `belmont status`** so it doesn't re-read every worktree's PROGRESS.md per call. Current cost is ~N file reads per invocation; N is usually <10; infrastructure cost of a daemon is far too high for a CLI that runs on demand.

## Evidence

`belmont-test/about-4-fresh` in the canonical test repo: clean parallel wave (M2/M3/M4) with merge-overlap report firing on `about/page.tsx`, reconciliation-agent resolving the import-union at high confidence, live status overlay showing `(live from worktree)` tags during the run. See [meta/validated-runs.md](../meta/validated-runs.md).

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestOverlayLiveMilestones_*`, `TestCopyBelmontStateToWorktreePreservesSteering`. For the root split: `cmd/belmont/metrics_test.go` → `TestWorktreeLoopConfigKeepsMetricsOnTheOriginatingRoot`, `TestRecordPhaseMetricsWritesUnderMainRootNotWorktree`, `TestWaveOfMilestonesRecordsAsOneRun`, plus `TestRecordPhaseMetricsFallsBackToRoot` for the serial path — the worktree case needs its own test because the serial path passes either way.

## Known rough edges

- **`stash-before-merge` can drop state-file edits.** When master has uncommitted changes under `.belmont/features/<slug>/` at merge time, `runWaveParallel` stashes them to clear the tree, merges the worktree branch, but PROGRESS.md edits captured in the stash may not pop cleanly. Result: master's PROGRESS.md shows `[ ]` for tasks that actually landed in code. Workaround: `belmont sync` + `belmont reverify`. Proper fix: make the merge path aware of `.belmont/features/<slug>/` as state-sensitive.
- **`STEERING.md` left on disk at completion.** The file is only deleted by `consumePendingSteering`, which fires during auto phases. When auto ends with only consumed entries, nothing triggers the final delete. Cosmetic; fix is a cleanup sweep at end-of-auto.

## Revisions

- 2026-04-21 — initial (worktree lifecycle, state copy).
- 2026-04-22 — removed single-milestone master-tree shortcut; unified every wave through `runWaveParallel`.
- 2026-04-22 — added live-status overlay via `overlayLiveMilestones`, `worktreeTracker.feature`/`mode` fields, `loadAutoWorktreeStateByMilestone`.
- 2026-04-22 — added `reportMergeOverlap` pre-merge visibility.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
- 2026-05-12 — added `MaxParallel <= 1` inline-merge semantic (every unit still goes through a worktree; only the merge interleaves). Paired with `resume-rebase.md`. See [`auto-mode/multi-feature-scheduling.md`](multi-feature-scheduling.md) for the multi-feature-mode equivalent.
- 2026-08-07 — `cmd/belmont/main.go` split into 22 files in the same package; file paths in this entry repointed to their new homes. Symbol names are unchanged and remain the durable identifier.
- 2026-08-07 — recorded that `--max-parallel` is inert without an explicit `(depends: …)`; a smoke fixture without one exercises the master-tree path and silently proves nothing about worktrees.
- 2026-08-14 — the live overlay stopped falling back to master in silence (#48). `overlayLiveMilestones` returns the gaps it hit; `status`, `blockers` and `validate` all name them, and validate files `unreadable_live_milestone` as a warning. Recorded because the "shouldn't happen" comment on the second fallback was a fair bet when this governed display only, and had not been revisited after the overlay became a gate.
- 2026-08-17 — the live-run-diagnostic bullet called `recover`'s missing active-run filter "a separate, unfixed hazard". #52 fixed it (`activeRunWorktrees` + `refuseIfRunOwns`, `--force` to override), so the entry was asserting an absent guard that two other documents already described as present. The bullet's own rule is unchanged and still stands on its own reasoning.
- 2026-08-18 — `mCfg` derivation moved into `worktreeLoopConfig`, and `loopConfig.MetricsRoot` added, after every metrics record in parallel-wave mode was found to be written into the worktree and deleted with it (`throughput` P0-M1-FIX-1). `RunID` is now minted by the orchestrator so a wave records as one run. Recorded as an invariant about *any* durable write, not as a metrics detail.
