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
