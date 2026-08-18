# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: 97b282a9 (belmont: verify round 5 — promote P0-M1-FIX-9 and P0-M1-FIX-11 [v])
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: 2026-08-18 20:22
- **Tasks**:
  - [ ] P0-M1-FIX-12: `.belmont/metrics/` is not gitignored on the `auto` path

## Orchestrator Context

### Current Task
P0-M1-FIX-12 — found live on 2026-08-18 while running `BASELINE.md` §How to capture it verbatim against `~/repo-3`. **This is a real Go fix, not a documentation edit.**

**The defect**: `ensureGitignoreEntry(projectRoot, ".belmont/metrics/")` is called only from `ensureStateFiles` (`cmd/belmont/fsutil.go:177`), which runs on the **install** path. Nothing on the `auto` path guarantees the rule. So in any project installed by a Belmont older than the metrics feature — `~/repo-3` is exactly this case, installed under v0.11.0 — the first instrumented run writes `.belmont/metrics/<feature>.jsonl`, `git status` then reports `?? .belmont/metrics/`, and the **next** `belmont auto` is refused by `requireCleanWorkingTree`. Observed exactly that; worked around by hand with `.git/info/exclude`, which is not a fix.

The consequence is specific and bad: the instrumentation this feature exists to add dirties the tree it is measuring, and the documented capture procedure (which has no install step) cannot be run twice in a row.

**The fix**: guarantee the ignore rule before the first metrics write can happen. Preferred shape — call `ensureGitignoreEntry(cfg.Root, ".belmont/metrics/")` at `auto` startup, alongside wherever the run's other preflight work happens, so it is idempotent and runs once per invocation rather than per record. Placing it immediately before the first `appendMetricsRecord` is acceptable if that is cleaner, but do not put it inside `appendMetricsRecord` itself (called per phase; would re-read/rewrite `.gitignore` on a hot path). Consider whether the worktree case needs anything: metrics are written to `MetricsRoot` (the originating root, per `P0-M1-FIX-1`), so the master `.gitignore` is the one that matters — confirm this and say so.

**Add a test** pinning the behaviour: a project root whose `.gitignore` lacks the entry, after the `auto` preflight (or whatever seam you choose), has it — and `requireCleanWorkingTree` is not tripped by a metrics write. Follow the existing test conventions in `cmd/belmont/*_test.go`.

**Second, smaller item on the same task line** (do this too): a phase that fails before the tool emits usage still appends a record with `input:0,output:0` — the three "session limit" exits on 2026-08-18 produced exactly that, at 3–4 s each. Determine what `summariseMetrics` currently does with such records, and either exclude or flag them so failed phases cannot drag the per-phase baseline down. **Record which behaviour is correct in `metrics.go`'s header comment** either way — the point is that the semantics are stated, not that a particular choice is forced. If the existing behaviour is already right, say so in the log and change only the comment.

### Active Task IDs
`P0-M1-FIX-12`. Verification-minted follow-up: full definition on its task line in `.belmont/features/throughput/PROGRESS.md` (M1, first task under the `### M1:` heading), not in PRD.md.

### File Paths
- **Code**: `cmd/belmont/fsutil.go` (`ensureStateFiles`, the existing call site), `cmd/belmont/main.go:1438` (`ensureGitignoreEntry`), `cmd/belmont/autocmd.go` (`requireCleanWorkingTree`, auto startup), `cmd/belmont/metrics.go` (`appendMetricsRecord`, `summariseMetrics`, header comment)
- **Tests**: `cmd/belmont/*_test.go` — `metrics_test.go` has the closest neighbours (`TestMetricsPathIsUnderGitignoredDirectory` at ~:355)
- **PROGRESS**: `.belmont/features/throughput/PROGRESS.md`
- **Feature Notes**: `.belmont/features/throughput/NOTES.md`
- **PRD / TECH_PLAN**: `.belmont/features/throughput/PRD.md`, `TECH_PLAN.md` — P0-2 defines the instrumentation this protects

### Scope Boundaries
- **In Scope**: the gitignore guarantee on the `auto` path + its test; the zero-usage-record semantics decision + `metrics.go` header comment (+ a test if you change behaviour); PROGRESS.md bookkeeping (`[ ]` → `[x]`).
- **Out of Scope**: everything else. Do not change the metrics record schema, the storage location, or `MetricsRoot` resolution (settled by `P0-M1-FIX-1`). Do not touch `.belmont/features/throughput/CENSUS.md` — that document is settled and five fix rounds deep. Do not touch the `[!] P0-3` / `[!] P0-3a` task lines: a live `belmont auto` run against `~/repo-3` is capturing their baseline right now.
- **A live run is in flight** using `/tmp/belmont-instrumented`, an already-built binary — your source changes cannot affect it. Do not rebuild to that path, and do not run `belmont auto` yourself.

### Learnings from Previous Sessions
- **`loopConfig` has two roots** (NOTES §Pattern): `Root` follows the worktree under `runAutoParallel`; `MetricsRoot` is the originating root. Ask which one any new path expression resolves to — the answer is usually not the one you want.
- **Dual execution paths** (`knowledge/cross-cutting/dual-invocation-paths.md`, and NOTES §Root Cause Patterns "a new state write was only ever exercised on one of two execution paths"): serial `runLoop` and `runAutoParallel` per worktree. This defect is that pattern again — a rule enforced on one path only. Make sure your fix and its test cover the path that actually runs.
- **A serial-path test can pass either way** (NOTES §Pattern): confirm your test fails with the fix reverted. Say so in the log.
- Run `go build ./cmd/belmont` and `go test ./cmd/belmont` (CLAUDE.md requires both after any Go change). `go vet ./...` and `staticcheck ./...` are clean in CI — keep them clean.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. Explore `cmd/belmont/` as needed; the call sites are named above.]

## Design Specifications
[Not populated — no design surface. No Figma URLs.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-12 — `.belmont/metrics/` is not gitignored on the `auto` path

**Status**: SUCCESS

**Files Modified**:
| File | Changes |
|---|---|
| `cmd/belmont/autocmd.go` | New `runAutoPreflight` (clean-tree check + artefact guarantee, in that order) replacing the inline preflight block; new `ensureMetricsIgnored` |
| `cmd/belmont/main.go` | `ensureGitignoreEntry` now returns whether it wrote the entry (install call sites unchanged — a Go call statement discards it) |
| `cmd/belmont/metrics.go` | `metricsIgnoreEntry` const; header paragraph stating the zero-usage decision; `zeroUsageRecord`; `ZeroUsage` on summary/run/tool aggregates + counting + render line |
| `cmd/belmont/fsutil.go` | Install call site uses the shared const; comment says why the install path is not sufficient |
| `docs/cli-commands.md` | §Clean-working-tree preflight now describes the second half |
| `AGENTS.md` | Preflight bullet + CLI-commands paragraph |
| `knowledge/auto-mode/clean-tree-preflight.md` | Amended in place (invariant, enforcement, 3 new `Don't re-do`, evidence, `Revisions`) |
| `.belmont/features/throughput/{PROGRESS,NOTES}.md` | `[ ]` → `[x]`; two Patterns and one Discovery |

**Tests Added** (`cmd/belmont/metrics_test.go`):
| Test | Coverage |
|---|---|
| `TestMetricsWriteDirtiesAnUnprotectedTree` | Negative control — same fixture, no preflight, tree goes dirty |
| `TestAutoPreflightLetsMetricsBeWrittenWithoutDirtyingTheTree` | The defect itself: after the preflight, an `appendMetricsRecord` write leaves `requireCleanWorkingTree` passing |
| `TestAutoPreflightCommitsOnlyTheGitignoreItWrote` | A new commit exists, touches only `.gitignore`, and staged user work survives |
| `TestAutoPreflightIsIdempotent` | Second invocation: no edit, no commit |
| `TestAutoPreflightDryRunTouchesNothing` | `--dry-run` mutates nothing |
| `TestAutoPreflightStillRefusesADirtyTree` | The clean-tree check still fires, and fires *first* |
| `TestEnsureMetricsIgnoredOutsideGit` | Best-effort outside git: rule written, no commit, no panic |
| `TestZeroUsageRecordSeparatesAbsentFromZero` | All-zero vs nothing-reported vs cache-read-only |
| `TestSummariseFlagsZeroUsagePhasesWithoutExcludingThem` | Counted in Phases/WallMs/totals, flagged in summary + per-run + per-tool, present in text and JSON |

**Verification Results**
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass (31s)
- `go test -tags eval ./cmd/belmont` (Tier 1): pass. Tier 2 not run — no skill prose changed.
- `go vet ./...`: clean · `staticcheck ./...`: clean (rebuilt with `GOTOOLCHAIN=go1.26.0` per NOTES) · `gofmt -l`: clean

**Revert check (mandated by NOTES §Pattern)** — deleting the single line `ensureMetricsIgnored(root)` fails `TestAutoPreflightLetsMetricsBeWrittenWithoutDirtyingTheTree` and `TestAutoPreflightCommitsOnlyTheGitignoreItWrote`; deleting the `zeroUsageRecord` counting block fails `TestSummariseFlagsZeroUsagePhasesWithoutExcludingThem`. The other four preflight tests are guard tests that assert the fix does **not** act (idempotence, dry-run, dirty-tree refusal, non-git) and pass either way by construction — stated rather than presented as proofs. The commit test initially passed with the fix reverted, because the fixture's own setup commit also touches `.gitignore`; it now captures `HEAD` first and asserts it moved.

**Functional check on real data** — the live `~/repo-3` record file (read-only copy into a temp root) rendered through a fresh build: `2 phase(s) reported zero tokens`, `"phases_with_zero_usage": 2` of 5, per-run breakdown attributing both to the `2026-08-18T15:11:09Z` run. That is 40% of the phase count carrying no model work — the exact distortion the flag exists to expose.

**Both invocation paths** — this is auto-path-only, and deliberately so. `recordPhaseMetrics` is called from `executeLoopAction` alone, so no metrics file is ever created by interactive `/belmont:*` use; there is nothing for the interactive path to ignore and no skill prose changed. Within the auto path the single call site in `runAutoCmd` covers all three dispatches (serial, parallel-milestone, multi-feature).

**Worktree question, answered** — only the originating root needs the rule. Records go to `loopConfig.metricsRoot()`, which stays on the root that started the run even under `runAutoParallel` (settled by `P0-M1-FIX-1` and pinned by `TestWorktreeLoopConfigKeepsMetricsOnTheOriginatingRoot`), so no metrics file is ever created inside a worktree; worktrees forked later in the run inherit the committed rule anyway.

**Zero-usage decision: flag, do not exclude** — recorded in `metrics.go`'s header with the rejected alternative. The behaviour before this change was to count such records silently, which is *half* right: counting them is correct, saying nothing about them is not. Reasons: a record's wall-clock is a real measurement whatever the tokens say (the three session-limit exits really did spend 3–4 s each), so dropping the record would delete a reported figure — the same class of act as inventing one; zero is a real measurement here, which is why the token fields are pointers; and nothing in a record proves *why* it is zero, so "a failed phase" is an interpretation a summariser should not make. What a reader computing tokens-per-phase needs is the count, so the denominator can be corrected knowingly — an exclusion would have hidden it. `ZeroUsage` and `Unreported` are disjoint: "told us nothing" vs "told us nothing happened".

**Self-Validation**
- Acceptance criteria: 5/5 — (1) ignore rule guaranteed on the auto path, (2) test pinning the rule *and* the untripped preflight, (3) worktree case confirmed and stated, (4) zero-usage semantics determined, flagged and recorded in the header, (5) PROGRESS bookkeeping.
- Visual check: N/A (CLI).

**Commit**: see `P0-M1-FIX-12` in `git log`.

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-12 | `belmont metrics` is absent from `docs/cli-commands.md` and from `AGENTS.md`'s CLI-commands paragraph, which enumerate every other command. P0-2 shipped the command without its documentation; `BASELINE.md` is currently the only place its invocation is written down. | P2 |
| FWLUP-2 | P0-M1-FIX-12 | The two zero-usage records in `~/repo-3`'s live file still carry `"input_semantics":"excludes_cache_read"`, which is a claim about a count no model produced. Harmless today (they aggregate into claude's existing key) but a phase that reported nothing arguably should declare no definition, exactly as a `nil`-usage record does. Not changed here: it is a record-schema question and the schema is out of scope. | P3 |

### Notes for Verification
- The load-bearing assertion is `requireCleanWorkingTree` passing *after* a metrics write, not the presence of the `.gitignore` line. `TestMetricsWriteDirtiesAnUnprotectedTree` is the control proving that assertion can fail.
- `ensureMetricsIgnored` commits on the user's branch at auto startup. That is a deliberate third auto-commit site (alongside `commitBelmontUpdate`) and the rationale, including why it does not violate the existing "never auto-commit unrelated user changes" rule, is in `knowledge/auto-mode/clean-tree-preflight.md` §Don't re-do.
- No `belmont auto` run was started and nothing was rebuilt to `/tmp/belmont-instrumented`; the live `~/repo-3` run was only read from.
