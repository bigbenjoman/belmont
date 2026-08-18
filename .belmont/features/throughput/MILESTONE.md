# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `9b521752798fd0d3eb46edca6983cf9a4e44866f`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 1 of 7.
- **Created**: 2026-08-18T13:03:03Z
- **Tasks**:
  - [x] P0-M1-FIX-1: Metrics are written to the worktree root and destroyed with it

## Orchestrator Context

### Current Task
`P0-M1-FIX-1` — the only task being implemented in this run. It is the Critical finding from M1's code review.

### Active Task IDs
`P0-M1-FIX-1`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — see §P0-2 for the parent task's acceptance criteria
- **TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/TECH_PLAN.md` — §Go Implementation Notes (P0-2/P0-3), §File-Format Specifications (Metrics)
- **Master TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/TECH_PLAN.md` — the six self-hosting rules
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` in the same directory — the full pipeline's Codebase Analysis and Implementation Log for M1, including the enumeration of every state writer and the metrics design as built
- **Repo conventions**: `AGENTS.md`, `CONTRIBUTING.md`, `knowledge/KNOWLEDGE.md`. **`knowledge/cross-cutting/dual-invocation-paths.md` is the rule this defect broke — read it.**

### The defect

`recordPhaseMetrics` (`cmd/belmont/auto_loop.go:614-628`) passes `cfg.Root` to `appendMetricsRecord`, which resolves `<root>/.belmont/metrics/`. But `auto_parallel.go:74` and `:474` set `mCfg.Root = wtPath` before calling `runLoop`, so in parallel-wave mode the records land inside the worktree. `.belmont/metrics/` is gitignored, `syncFeatureStateAfterMerge` copies only `PROGRESS.md` back to the main root, and `removeWorktree` (`worktree.go:194-205`) then deletes the tree.

This is not a corner case. `autocmd.go:284` routes to `runAutoParallel` whenever any milestone declares `(depends: …)`, and this feature's own `PROGRESS.md` declares dependencies on M2 through M11. So the instrumentation the entire feature is judged by would record **nothing** for the execution mode the feature actually runs in, and M11/P3-3 would have no data to compare against the M1 baseline.

### Required fix

1. Add `MetricsRoot` to `loopConfig`, defaulting to `Root` so the serial path is unchanged.
2. Set it to the **originating** (main) root at both `auto_parallel.go:74` and `:474`, alongside the existing `mCfg.Root = wtPath`.
3. Propagate `cfg.RunID` from the parent into `mCfg` at the same two sites, so one wave is recorded as one run rather than N separate runs.
4. Have `recordPhaseMetrics` write to `MetricsRoot`.
5. Add a test that pins the worktree case — `MetricsRoot != Root`, and the record lands under the main root.

Concurrency note worth thinking about before you write it: with several worktrees in a wave now appending to **one** file under the main root, the append path is shared across processes. Check whether `appendMetricsRecord` is safe for that, and say so either way in the implementation log — a fix that trades a lost file for a corrupted one is not a fix.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-1`.
- **Out of Scope**: everything in `{base}/PRD.md` §Out of Scope. Do **not** touch the other six follow-ups (`P0-M1-FIX-2` … `P0-M1-FIX-7`) — each gets its own run. Do **not** start M2–M11 work.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase or delete any remote branch.
- **Zero external Go dependencies, no `go.sum`.**

### Verification before marking `[x]`
```bash
cd /Users/benlavender/repos/belmont
go build ./cmd/belmont
go test ./cmd/belmont
go test -race ./cmd/belmont
go test -tags eval ./cmd/belmont
go vet ./...
staticcheck ./...
gofmt -l cmd/          # must print nothing
belmont validate --root /Users/benlavender/repos/belmont
```

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this exact defect's root cause: *"a new state write was only ever exercised on one of two execution paths."* Belmont has two execution paths for the same loop — serial `runLoop`, and `runAutoParallel` per worktree — and `knowledge/cross-cutting/dual-invocation-paths.md` documents the rule as prose with nothing mechanical to gate it. The prevention rule recorded there is this task's acceptance bar: **any new write under `.belmont/` must be exercised on both paths, with a test pinning the worktree case.** If a value is a path root, ask which of the two roots it resolves to under `runAutoParallel`.

Also from the same section, and not to be weakened while editing these files: **never estimate a token count.** Token fields are `*int64` precisely so a reported zero stays distinguishable from "not reported", and hosts that cannot report record `null` with a stated reason.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and the required fix are specified above; the full M1 codebase scan is in `MILESTONE-M1.done.md`. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-1 — Metrics are written to the worktree root and destroyed with it

**Status**: SUCCESS

**Files Modified**:
| File | Changes |
|---|---|
| `cmd/belmont/types.go` | Added `loopConfig.MetricsRoot`, documenting *why* `Root` is not one root under `runAutoParallel`. Empty means "same as `Root`", so no construction site has to remember it and the serial path is untouched. |
| `cmd/belmont/metrics.go` | Added `(loopConfig).metricsRoot()` — `MetricsRoot` when set, `Root` otherwise. Documented `appendMetricsRecord`'s concurrency contract now that the file is shared. |
| `cmd/belmont/auto_loop.go` | `recordPhaseMetrics` writes to `cfg.metricsRoot()` instead of `cfg.Root`. |
| `cmd/belmont/auto_parallel.go` | New `worktreeLoopConfig(cfg, wtPath)` derives the child config in ONE place; both worktree sites (`runFeatureInWorktree`, `runMilestoneInWorktree`) now call it. `runAutoParallel` mints `RunID` before any worktree exists. |
| `cmd/belmont/multifeature.go` | `runAutoMultiFeature` mints `RunID` for the same reason. |
| `knowledge/auto-mode/parallel-wave-orchestration.md` | Amended in place: two new invariants (nothing durable under `Root`; one wave is one run), the corrected `runLoop(mCfg)` line, a failure mode, test pointers, `Revisions` entry. |
| `knowledge/cross-cutting/state-atomicity.md` | Amended in place: `appendMetricsRecord` recorded alongside the other `O_APPEND` rulings, with the single-write requirement and the measured negative control. `Revisions` entry. |
| `.belmont/features/throughput/NOTES.md` | Four `### Pattern` learnings (two roots; orchestrator-minted `RunID`; O_APPEND is per-`write()`; a serial-path test passes either way here). |

**Tests Added** (`cmd/belmont/metrics_test.go`):
| Test | Coverage |
|---|---|
| `TestWorktreeLoopConfigKeepsMetricsOnTheOriginatingRoot` | `Root` moves to the worktree, `MetricsRoot` and `RunID` do not. |
| `TestRecordPhaseMetricsWritesUnderMainRootNotWorktree` | The required worktree-case pin: `MetricsRoot != Root`, the record lands under the main root, **and the worktree gets no `.belmont/metrics/` at all**. |
| `TestWaveOfMilestonesRecordsAsOneRun` | Three worktree configs sharing one parent `RunID` summarise as 1 run / 3 phases, not 3 runs. |
| `TestRecordPhaseMetricsFallsBackToRoot` | The serial path is unchanged with `MetricsRoot` unset. |
| `TestAppendMetricsRecordIsConcurrencySafe` | 16 goroutines x 400 concurrent appends to one file: 6400 intact lines, 0 unparseable — **with an in-test split-write negative control that must fail**. |

**Verification Results** (every gate in the MILESTONE verification block):
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass (21.8s)
- `go test -race ./cmd/belmont`: pass (25.6s)
- `go test -tags eval ./cmd/belmont`: pass (23.2s)
- `go vet ./...`: pass
- `staticcheck ./...`: pass (clean; binary at `$(go env GOPATH)/bin`, not on bare PATH)
- `gofmt -l cmd/`: prints nothing
- `belmont validate --root /Users/benlavender/repos/belmont`: `✓ No milestone-structure violations found.`
- No new dependencies; no `go.sum`.

**Self-Validation**: Acceptance criteria 5/5 (`MetricsRoot` added and defaulting; set at both sites; `RunID` propagated; `recordPhaseMetrics` writes to it; worktree-case test). Visual check: N/A (CLI).

**Functional check beyond the unit tests**: a freshly built binary was pointed at a fixture holding three milestone records sharing one `RunID` — `belmont metrics` reports **"1 run(s), 3 phase(s)"**, which is the shape a wave now produces. Before this change it would have reported 3 runs, from a file that no longer existed.

---

### Rulings the MILESTONE asked for explicitly

**1. Shared-append safety — safe, and the reason is narrow.**

`appendMetricsRecord` is safe for several worktrees appending to one file, on one condition worth stating because it is easy to undo by accident: **the record and its terminating newline go out in a single `f.Write`.** `O_APPEND` makes seek-to-EOF-and-write indivisible, so two appenders cannot land at the same offset — but that guarantee is per *write call*, not per file handle. Emitting the payload and the `\n` separately lets another appender interleave between them and splice two records into one unparseable line; a `bufio.Writer` does the same on buffer boundaries.

This was measured rather than asserted, because this repository has just been reminded that a concurrency test with no failing control proves nothing. Negative control, macOS/APFS, 16 goroutines x 400 records to one file, three runs each:

| Writer | Lines (want 6400) | Unparseable |
|---|---|---|
| single write (`appendMetricsRecord`) | 6400 / 6400 / 6400 | 0 / 0 / 0 |
| split write (control) | 3391 / 3443 / 3437 | 1856 / 1863 / 1853 |

The control is committed as part of `TestAppendMetricsRecordIsConcurrencySafe` (skipped when `GOMAXPROCS < 2`, where it would pass for the wrong reason), so the test can fail.

Two qualifications, stated rather than left implicit. First, **the failure class here is not the one `writeStateFile` exists to prevent**: the hazard is a garbled *line* in an append-only log that `readMetricsRecords` already skips, not a zero-byte *file* that a reader parses as valid-and-empty. Converting this to a read-modify-rewrite would trade a bounded, self-healing failure for an unbounded one — the ruling `state-atomicity.md` already made for `appendSteeringEntry`. Second, `O_APPEND` is **not** atomic over NFS; Belmont's metrics live beside the checkout so that is out of scope, but the call site is where a lock would go, and that is now written into the doc comment.

No lock was added. A mutex would cover only in-process goroutines and would give a false sense of covering the cross-process case, which the kernel already covers better.

**2. `RunID` propagation — one wave is now one run.**

`mCfg := cfg` already copied `RunID`, so the field was never the problem: the *parent* had no `RunID` to copy. `runLoop` mints one from its own start time when it finds none, which is right for a serial run and wrong for a wave — each worktree's `runLoop` minted its own, so one invocation of five milestones would have appeared in `belmont metrics` as five runs, each holding a fragment of the work. That is wrong in exactly the mode P3-3's comparison against the M1 baseline depends on.

Fixed by minting `RunID` in `runAutoParallel` and `runAutoMultiFeature` before any worktree is created, using the same `RFC3339` format as `runLoop`'s fallback so serial and parallel records sort and compare identically. `runLoop`'s fallback is untouched and still covers the serial path.

**How this satisfies `dual-invocation-paths.md` rather than moving the path.** The convention's demand is that a change be *exercised* on both execution paths, not that it name both. Both are now exercised by tests, and the worktree case has its own — necessary because `TestRecordPhaseMetricsFallsBackToRoot` passes with or without the fix, so the serial path can never detect this defect. The derivation itself was pulled into one function, `worktreeLoopConfig`, so the two worktree sites cannot drift; the original bug existed because two sites did the same thing and only one kind of run was ever checked. Verified by reverting the fix: the three worktree tests fail, the serial test passes.

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | P0-M1-FIX-1 | `runLoop` computes `cfg.CriticalPath` from `buildStatus(cfg.Root, …)`, so inside a worktree it reads that worktree's copy of `PROGRESS.md`. Correct today (the copy carries every milestone and its deps), but it is a second value derived from the root that moves. Worth pinning if `copyBelmontStateToWorktree` ever narrows what it copies. Not changed — out of scope. | P3 |
| — | P0-M1-FIX-1 | `RunID` is `RFC3339` at second granularity, so two runs starting within the same second collide into one aggregate. Pre-existing; unchanged here. | P3 |

### Notes for Verification
- The one thing a unit test cannot reach is the real `runMilestoneInWorktree`, which needs git, `belmont install` and a live tool. The derivation it performs is covered by calling the production `worktreeLoopConfig` rather than by restating it in the test — the closest available substitute, and deliberately not a copied assignment.
- `staticcheck` is not on the bare `PATH` on this machine; it lives at `$(go env GOPATH)/bin/staticcheck`. It ran clean.
- `MILESTONE.md` shows as heavily modified against `HEAD` because the orchestrator replaced the full M1 file with this single-task one; the original is the untracked `MILESTONE-M1.done.md`. Not an implementation change.
