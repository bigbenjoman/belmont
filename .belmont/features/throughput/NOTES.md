# Notes

## 2026-08-18

### Environment
- Go stable at the time of P0-12 is **1.26.6** (1.27 was at rc3); 1.21 and 1.24 are both out of Go's two-release support window. `go.mod` now declares `go 1.26.0`, so `GOTOOLCHAIN=auto` auto-downloads go1.26.0 on any machine whose installed toolchain is older (this machine has go1.24.4 installed).
- **`staticcheck` must be built with a toolchain >= the module's `go` directive.** After the bump, the locally installed staticcheck (built with go1.25, because staticcheck's own `go.mod` requires 1.25 and the machine toolchain was 1.24.4) failed with `file requires newer Go version go1.26 (application built with go1.25)` on both `scripts/declsum/main.go` and a vendored stdlib file. Fix is to rebuild it, not to pin or revert: `GOTOOLCHAIN=go1.26.0 go install honnef.co/go/tools/cmd/staticcheck@latest`. CI is unaffected — `setup-go` installs 1.26 *before* the `go install`, so CI's staticcheck is already built with 1.26.

### Discovery
- `go build -tags embed ./cmd/belmont` fails from a clean checkout with `pattern all:agents: no matching files found`. This is **pre-existing and unrelated to the toolchain** (verified by stashing the bump and reproducing on `go 1.21`): `//go:embed` paths in `cmd/belmont/embed.go` resolve relative to the package directory, and `scripts/build.sh` stages `agents/`, `skills/` and `prompts/` into `cmd/belmont/` before building. Use `scripts/build.sh`, never a bare `-tags embed` build.
- `go test -race ./cmd/belmont` was **already clean** before any P0-1 change, on both go1.24.4 and go1.26.0. The MILESTONE's warning 6 (expect pre-existing races from `worktreeTracker`) did not materialise.

### Debugging
- **`git branch -r --no-merged main` over-reports in a rebase-heavy repo.** It compares ancestry, so a rebased-and-landed branch still lists as unmerged. 21 of Belmont's 35 "unmerged" branches were fully or effectively landed. Use `git cherry -v main <branch>` (patch-equivalence: `-` = already upstream, `+` = outstanding), then hand-check the residue — patch-ids are defeated by context drift, which produced false positives on `feat/maintenance-ci` and `fix/worktree-git-excludes`.
- **`git rev-parse "main:$path"` echoes the argument back when it cannot resolve it**, so a missing file reads as present. Use `git rev-parse --verify --quiet` or `git cat-file -e`. This silently inverted a whole triage pass.
- **Two-dot `git diff main..<branch>` is meaningless when main is far ahead** — it reports main's own additions as the branch's deletions (every branch looked like a 30,000-line deletion against a 157-commit-ahead main). Use three-dot `main...<branch>` for a branch's own changes.

### Discovery
- The MILESTONE's two "most dangerous" branches are phantoms: `origin/fix/unrecognised-task-markers` (64 files, "widest collision in the backlog") is 24/24 landed, and `origin/feat/maintenance-ci` (said to collide with P0-12, P0-1 and P0-2 at once) is 29/31 landed with both leftovers verified present. Four of the TECH_PLAN's five named M3/M4/M6 collisions are already on `main`. **M3, M4 and M6 are not standing on contested ground.**
- The real hazard is `origin/fix/post-51-triage` (2026-08-17, newest, 15 unlanded commits, 6 new regression tests) — it merges with **zero** conflicts and touches `reconcile.go` + `worktree.go`, which P0-1 rewrites.

### Pattern
- **The torn-write window is huge, not theoretical.** A negative control (same concurrent harness, old `os.WriteFile`) observed a **0-byte read on every run within 0.02s** against 300 rewrites. `os.WriteFile` truncates then writes, and Belmont's readers parse an empty `PROGRESS.md` as zero milestones without erroring. Always pair an atomicity fix with a negative control — a race test that cannot fail proves nothing.
- `os.CreateTemp` creates **0600**. Any temp-file-then-rename helper must `Chmod` to the caller's perm before the rename or it silently tightens every file it touches from 0644.
- Rename **replaces** a symlink where `os.WriteFile` **follows** it. Two callers depend on the old behaviour (`writeReconciliationResolution`, `copyFile`) and both resolve symlink-ness above the call — convert the final write only, never the whole function.

### Discovery
- `os.RemoveAll(dstFeature)` before `copyDir` in `copyBelmontStateToWorktree` (worktree.go) is a **directory**-level tear that no per-file write helper closes. Left open deliberately in P0-1 and recorded in `knowledge/cross-cutting/state-atomicity.md` so nobody reads "atomic writes: done" and assumes otherwise.

### Discovery
- **Tool usage schemas, verified empirically 2026-08-18** (not from docs — both were probed with a live trivial prompt):
  - **claude 2.1.234**: the `result` event carries `usage.{input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens}`, plus `modelUsage`, `total_cost_usd`, `duration_ms`. Belmont records tokens only — cost is derived and changes under us.
  - **codex**: the `turn.completed` event carries `usage.{input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens}`. **The field names differ from Claude's** — `cached_input_tokens` is the read, `cache_write_input_tokens` the creation. Swapping them yields a plausible wrong number.
  - **gemini could NOT be verified**: `IneligibleTierError — This client is no longer supported for Gemini Code Assist for individuals` (gemini-cli 0.46.0). cursor/copilot/opencode are not installed on this machine.
- **Deviation from the feature TECH_PLAN's usage table**, recorded here because the plan file is spec, not deliverable: the table lists gemini and cursor as "Yes". They now record `null` with the note "usage schema not yet verified against a live run — not estimated". Shipping a parser against a guessed schema fails *silently* (always-zero or always-nil) and would contaminate the M1 baseline. Follow-up: verify both against a live run and turn them on.
- Probe commands need trust flags outside a trusted dir: `codex exec --skip-git-repo-check`, `GEMINI_CLI_TRUST_WORKSPACE=true gemini`. macOS has no GNU `timeout`.

### Pattern
- **`tailWriter` keeps only the last 1500 bytes**, so `executionResult.Output` is the tail of a stream, not the stream. Any future "parse something out of tool output" work must not read `Output` — it will work in testing and fail on long runs. `usageCapture` (render.go) retains a line by *content* rather than position; reuse it rather than raising the tail size, which changes error-reporting semantics and is still only a bigger window.
- **Wave index is not the critical path.** `computeWaves` is Kahn levelling; a milestone sits in wave 2 if any dependency is in wave 1, whether or not it lies on a longest chain. `criticalPathMilestones` (metrics.go) is the longest-chain pass. Pinned by `TestCriticalPathIsNotTheWaveIndex`, which also asserts the fixture still demonstrates the confusion.

### Discovery
- **The extraction census answers the PRD's open question YES, and the pre-estimate was wrong in mechanism as well as magnitude.** Five registers' indexes still exceed 25,000 tokens after extraction, not three. The estimate assumed size implies indented narrative and applied the worst file's 62% ratio to every file; **three of the five contain no indented lines at all**. `repo-3/feat-015` is 1,022,749 B of which **795,799 B are task HEAD lines** (longest single task line: 11,542 chars) and only 83,500 B indented — extraction moves 7.6% of it.
- Consequence: M5/P1-8's 400-char task-line limit is load-bearing, not defensive — it is the only mechanism in the plan that would have prevented `feat-015`. Extraction is still right (34.8% of all register bytes estate-wide, 62.9% of the worst file) but does not on its own bring every register under the ceiling.
- Measured denominator: **138 feature directories, 65 live registers, 68 archived** — matches the milestone scan, and does not match the PRD's "the other 83". Always state the denominator.
- `repo-4/feat-075` extracts 1,860,979 -> 691,323 B (62.9%), reproducing the PRD's composition analysis exactly. The plan is measuring the system it describes.

### Pattern
- Reuse `parseMilestones` + `taskBodyEnd` for anything that needs a task's body extent — `blockers.go:taskDetail` already composes them. Do not write a third line-scanner.
- When summing what a task body would move, record **which line indices** move rather than summing per-task lengths: a nested task bullet lies inside its parent's body *and* is a task itself, so the naive sum double-counts.

### Pattern
- **`loopConfig` has two roots, not one.** `Root` follows the worktree under `runAutoParallel`; anything that must outlive the wave must not. `worktreeLoopConfig` (auto_parallel.go) is now the single place both worktree sites derive the child config, so the question "which root does this resolve to?" is asked once. Ask it of any new path root added to `loopConfig`.
- **`RunID` must be minted by the orchestrator, not by `runLoop`.** `runLoop`'s fallback keys off its own start time, which is right serially and wrong for a wave — N worktrees produce N run IDs for one invocation, and `belmont metrics` aggregates per run. Seeded in `runAutoParallel` and `runAutoMultiFeature` before any worktree exists.
- **O_APPEND concurrency safety is per `write()` call, not per file handle.** Measured 2026-08-18, macOS/APFS, 16 goroutines x 400 records to one file: `appendMetricsRecord`'s single `f.Write(append(data, '\n'))` produced 6400 intact lines, 0 unparseable. A negative control splitting the payload and the newline into two writes lost ~47% of its lines and left ~1850 unparseable **on every run**. So the shared-append path is safe, but only while the record and its newline stay in one write — no `bufio.Writer`, no second `Write`. The failure class is a garbled line, not a torn file, which is categorically different from what `writeStateFile` exists to prevent.
- **A serial-path test passes either way here.** `recordPhaseMetrics` with `MetricsRoot` unset behaves exactly as before the fix, so only a test that sets `MetricsRoot != Root` can detect the defect. Confirmed by reverting the fix and watching the three worktree tests fail and the serial test pass.

## Polish

### From verification [2026-08-18]
- `streamLine.Subtype` added with a comment but never read anywhere in the tree — `cmd/belmont/render.go:249`
- `extract --all` is registered then discarded (`_ = all`) and absent from `printUsage`, though TECH_PLAN §Commands names `--all --dry-run` as *the* census invocation — `cmd/belmont/extract.go:280,290`
- `TestWriteStateFileOverwriteKeepsExistingPermissions` cannot fail as written (seeds 0644, writes 0644). The real behaviour is the opposite of the name: `os.WriteFile` ignores `perm` on an existing file, `writeStateFile` forces it — so a file a user tightened to 0600 is reset to 0644. Rename, seed at 0600, and note the divergence in the helper's doc comment — `cmd/belmont/fsutil_test.go:119-146`
- `TestWriteStateFileTempIsSiblingNotTempDir` relies on Unix directory-permission enforcement; it compiles under `GOOS=windows` but would fail if tests ran there. Add a `runtime.GOOS == "windows"` skip — `cmd/belmont/fsutil_test.go:169-190`
- Both `state-atomicity.md` and `fsutil_test.go:20-22` claim the race proof is meaningful *only* under `-race`. It isn't — it detects torn reads by comparing file contents, which the race detector does not instrument, and fails identically without it. Overstating this invites someone to drop the test when `-race` is inconvenient
- `state-atomicity.md` §"What this does NOT fix" names `copyBelmontStateToWorktree` (`worktree.go:241`) but not the identical `os.RemoveAll` + `copyDir` in `syncFeatureStateAfterMerge` (`worktree.go:397`). The section is otherwise exemplary; naming both would close it
- `toolReportsUsage` is referenced only from `metrics_test.go:365` — fine as an invariant checker, but say so in a comment or use it in `attachUsageCapture` instead of the duplicated switch — `cmd/belmont/metrics.go:86-91`
- `censusFeature` marks every line in a parent task's body as moved, including nested task *head* lines M3's extraction is specified to leave in place, so `IndexBytes` is a lower bound. Cannot change P0-4's answer (it can only push the over-threshold count higher, and the answer is already YES) but M3 should know the census under-reports the residual index — `cmd/belmont/extract.go:113-127`
- `copyFile` for master context files still discards its error (pre-existing, "best-effort"). No orphan risk — the helper's `defer` always cleans the temp — only a silent loss. Inconsistent with the five sites upgraded to a stderr warning — `cmd/belmont/worktree.go:258`
- CI does not run `go test -race ./cmd/belmont`, so P0-1's acceptance rests on a command CI never executes
- This file has four separate `### Discovery` headings; merge or number them

## Root Cause Patterns

### [2026-08-18] Pattern: a new state write was only ever exercised on one of two execution paths
**Issue**: Every metrics record is written to `cfg.Root`, which `auto_parallel.go` sets to the worktree path — so the instrumentation the whole feature is judged by produces nothing in parallel-wave mode, the mode this feature itself will run in.
**Root Cause**: Belmont has two execution paths for the same loop (serial `runLoop`, and `runAutoParallel` per worktree). The repo documents this in `knowledge/cross-cutting/dual-invocation-paths.md`, and the codebase scan explicitly named that rule for P0-2 — but the rule exists only as prose, so applying it depended on the implementer recalling it at the moment of writing the path expression. Nothing mechanical gates it.
**Prevention**: Any new write under `.belmont/` must be exercised on **both** paths before the task is marked done, with a test that pins the worktree case specifically. If a value is a path root, ask which of the two roots it resolves to under `runAutoParallel` — the answer is usually not the one you want.
**Source**: M1 / P0-2

### [2026-08-18] Pattern: a census derived its own scope instead of taking the spec's enumeration
**Issue**: The extraction census walked five roots, but not the five the PRD names — `repo-2` (31 feature dirs, 18 live registers) was never measured, so the denominator is 138/65 instead of 168/82.
**Root Cause**: The scope was assembled from what was conveniently to hand rather than from the PRD's explicit list, and `runCensus` swallows an unreadable root (`os.IsNotExist → continue`). An incomplete walk is therefore indistinguishable from a complete one at every level — no error, no warning, just a smaller number that looks like a finding.
**Prevention**: When a spec enumerates its subjects, use that enumeration verbatim as the input list and **fail loudly** on any member that cannot be read. A missing input must never degrade silently into a smaller denominator.
**Source**: M1 / P0-4

### [2026-08-18] Pattern: a measurement was used to correct the spec before it was known to be complete
**Issue**: CENSUS.md declares *"the PRD's phrase 'the other 83' does not match disk"* — but the PRD's figure reproduces exactly once the omitted repo is included. The PRD was right; the correction was the error. Separately, "three of the five contain no indented lines at all" is wrong (two do), and was copied verbatim into four files, one of them directly beneath a human-gated escalation the owner will read when deciding whether to restructure the waves.
**Root Cause**: Same shape as the numeric-drift pattern already recorded in `framework-evaluation`'s NOTES: a derived figure was restated across documents instead of living in one canonical place, and it was trusted enough to contradict the spec before its own coverage was checked.
**Prevention**: Before contradicting a spec's number, prove your measurement covers the spec's full scope. State a measured figure once, in one file, and have every other document link to it rather than repeat it — a wrong number repeated four times is four times as expensive to retract.
**Source**: M1 / P0-4

### [2026-08-18] Pattern: triage verdicts written from summary evidence rather than from content
**Issue**: `origin/feat/maintenance-ci` was verdicted *abandon* on the claim that both leftover commits are already on `main`. One is not — `1129cf84`'s 23-line `--max-parallel` gotcha is absent, and the triage mis-describes it as a different change entirely. A second abandon verdict gives a reason (`main` "already scopes the sync per milestone") that is simply untrue of `main`'s code.
**Root Cause**: Verdicts were formed from commit counts and plausible-sounding mechanisms rather than by reading the specific content in question. `git cherry` tells you a patch-id is unmatched; it does not tell you whether the *content* landed by another route, and a mechanism that sounds right is not evidence that it exists.
**Prevention**: An "already landed" verdict must name the commit **and** show its content present on `main` (`grep` the heading, diff the file). An "abandon" reason must name the superseding mechanism on `main` and cite the lines that implement it. A destructive recommendation carries a higher evidence bar than a keep.
**Source**: M1 / P0-13

### [2026-08-18] Pattern: identically-named fields from different vendors normalised without recording their semantics
**Issue**: `summariseMetrics` sums `Input` across tools whose `input_tokens` differ in meaning — claude's excludes cache reads, codex's includes them.
**Root Cause**: Two schemas share a field name but not a definition. The per-tool parsers are individually correct (and the field names are *not* transposed — that trap was avoided and pinned by tests), but the divergence was recorded only in a comment at the parse site, not carried to the aggregation boundary where it actually bites.
**Prevention**: When ingesting the same-named field from different vendors, record the semantic **at ingest** as data, not as a comment, and refuse to aggregate across sources whose semantics differ. A summary that silently mixes definitions produces a plausible wrong number — the exact failure mode the "never estimate" rule exists to prevent, arriving by a different door.
**Source**: M1 / P0-2
