# Milestone: M1 — Toolchain, atomic writes & baseline

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `ffd4b4bcfcbd03077eb61a8947e12a6dd57a65ff`
- **Created**: 2026-08-18T11:38:26Z
- **Tasks**:
  - [ ] P0-12: Bring the toolchain onto a supported version
  - [ ] P0-13: Triage the unmerged branch backlog
  - [ ] P0-1: Atomic state writes
  - [ ] P0-2: Token and wall-clock instrumentation
  - [ ] P0-3: Capture the pre-change baseline
  - [ ] P0-4: Extraction census across all feature directories

## Orchestrator Context

### Current Milestone
**M1: Toolchain, atomic writes & baseline.** Bump the toolchain and clear the branch backlog, fix the non-atomic state write, then stand up the measurement the whole feature is judged against. Everything else in the feature depends on this milestone. All six tasks are incomplete.

**Required order.** P0-12 must land *before* P0-2 and P0-3, so the recorded baseline and the eventual M11 re-measurement are taken on the same toolchain — a comparison whose two halves were built differently proves nothing. P0-13 should complete before any task touches shared files. Otherwise: P0-12 → P0-13 → P0-1 → P0-2 → P0-3 → P0-4.

### Active Task IDs
`P0-12, P0-13, P0-1, P0-2, P0-3, P0-4`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont`, which is a *different repository* — the planning workspace. It contains a different feature (`framework-evaluation`) with open tasks. Never resolve `.belmont/` relative to the working directory, and never edit anything under `/Users/benlavender/belmont`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — authoritative task definitions and acceptance criteria
- **TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/TECH_PLAN.md` — read it; every M1 task touches architecture it specifies. §Prerequisites, §Go Implementation Notes (P0-12, P0-13, P0-1, P0-2/P0-3), §File-Format Specifications and §Commands are the relevant parts.
- **Master TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/TECH_PLAN.md` — read it. This milestone is cross-cutting and self-hosting; it carries the six rules that make that safe.
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md`
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md` — does not exist yet
- **Global Notes**: `/Users/benlavender/repos/belmont/.belmont/NOTES.md` — does not exist
- **Repo conventions**: `/Users/benlavender/repos/belmont/AGENTS.md`, `CONTRIBUTING.md`, `knowledge/KNOWLEDGE.md` — authoritative for build, test and review. Consult `knowledge/KNOWLEDGE.md` first, per `AGENTS.md:5`.

### Scope Boundaries
- **In Scope**: only `P0-12, P0-13, P0-1, P0-2, P0-3, P0-4`
- **Out of Scope**: see §Out of Scope in the PRD. Specifically scored out on evidence and NOT to be built: code-graph index, speculative execution, blast-radius verification, rolling wave scheduler, task-level dependency metadata.
- **Milestone Boundary**: do NOT implement tasks from M2–M11. In particular, do not start the register/detail split (M3), the `belmont slice` command (M4), size ceilings (M5) or loop bounding (M6). P0-4 is a **census only** — it measures what extraction would yield; it does not extract.

### Self-hosting rules (load-bearing — from the master TECH_PLAN)
1. **The orchestrating binary is pinned** to Homebrew v0.11.0 at `/opt/homebrew/bin/belmont`. Go changes take effect only on an explicit rebuild.
2. **Do NOT run `belmont install`** — not in this repo, not at user level. This is what stops a milestone changing the loop that is running it.
3. **Model tiers are a barbell** — high or low, never medium.
4. **Zero external Go dependencies and no `go.sum`.** Any change that would add one is out of scope by construction.
5. **Contract-changing work needs a proposal first.** No M1 task changes a file contract, so none is owed here — but note that `docs/proposals/` is not on `main`; proposals 0003–0006 live on `origin/docs/pr-proposals`. This is directly relevant to P0-13.
6. **Tier 1 evals cannot license a prose change.** No M1 task is prose, so Tier 1 suffices for this milestone.

### Verification commands (from TECH_PLAN §Commands)
```bash
cd /Users/benlavender/repos/belmont
go build ./cmd/belmont
go test ./cmd/belmont
go test -race ./cmd/belmont          # required for P0-1
go test -tags eval ./cmd/belmont     # Tier 1, offline, free
go vet ./...
staticcheck ./...
gofmt -l cmd/                        # must print nothing
bash scripts/generate-skills.sh --check
belmont validate --root /Users/benlavender/repos/belmont
```

### Learnings from Previous Sessions
No previous learnings found. Neither `.belmont/NOTES.md` nor the feature's `NOTES.md` exists yet — this is the first implementation session for this feature, and the first time Belmont has run on itself in this repository.

Two facts established during the 2026-08-18 plan review that bear directly on M1, recorded here because there is no NOTES file to carry them:

- **There are exactly 35 unmerged branches** (verified by `git branch -r --no-merged main`, excluding `upstream/*` and `HEAD`). Three carry known dependencies: `origin/docs/pr-proposals` holds proposal 0004 rev 5, which is **M2's specification**, so its verdict gates M2; `origin/proposal/0001-quick-mode` and `origin/proposal/0002-prd-hygiene` carry the `framework-evaluation` feature's P6-1 and P6-2 specs from 2026-05-23.
- **`go.mod` declares `go 1.21`**; the locally installed toolchain is `go1.24.4`.

### Additional User Instructions
> Sequence P0-12 before P0-2/P0-3 so the baseline is captured on the bumped toolchain. Skip the design agent on every task. Model tiers are a barbell (high or low, never medium) per `models.yaml`. Zero external Go dependencies and no `go.sum`. The orchestrating binary stays pinned to Homebrew v0.11.0 — do NOT run `belmont install`.

## Codebase Analysis

*Written by codebase-agent, 2026-08-18, against `ffd4b4bc` in `/Users/benlavender/repos/belmont`. Every path below is repo-relative to that root. Line numbers are as-of that commit.*

### Project Stack

| Category | Technology | Version / detail |
|---|---|---|
| Language | Go | `go.mod` declares `go 1.21`; no `toolchain` directive |
| Module | `module belmont` | **zero `require` lines, no `go.sum`, stdlib only** |
| Build | `go build ./cmd/belmont` (dev, `!embed`) / `scripts/build.sh <ver>` (release, `-tags embed`) |
| Testing | stdlib `testing`, 27 `*_test.go` files in `cmd/belmont/` | plus Tier-1 eval harness behind `-tags eval` |
| Lint | `go vet ./...`, `staticcheck ./...`, `gofmt -l cmd/ scripts/` | all currently clean, enforced in CI |
| Package manager | **n/a — Go with no external deps.** No `package.json`, no lockfile anywhere. |
| Monorepo | No. `BELMONT_MONOREPO` unset; single Go package `cmd/belmont`. |

Stdlib imports in use across `cmd/belmont/`: `bufio bytes embed encoding/json errors flag fmt io io/fs net net/http os os/exec os/signal path/filepath regexp runtime sort strconv strings sync syscall text/template time`. **No `slices`, `maps`, `cmp`, `log/slog`, or any other post-1.21 stdlib package** — nothing in the tree depends on a language or library feature newer than 1.21, which is what makes P0-12 a directive-only change.

### Project Structure (relevant subset)

```
cmd/belmont/            # single Go package, all logic; 30 non-test .go files
├── main.go             # subcommand switch (main.go:295-325), copyFile/copyDir, ensureGitignoreEntry
├── fsutil.go           # 148 lines: resolveSourceRoot, ensureSymlink, ensureStateFiles  ← P0-1 seam
├── state.go            # parseMilestones, flattenTasks, markers, section breaks        ← the parser
├── types.go            # task/milestone/loopAction/executionResult/historyEntry
├── feature.go          # listFeaturesWithOverrides, syncMasterFeatureStatuses, computeWaves
├── status.go           # buildStatus, renderStatus, renderFeatureListing
├── auto_loop.go        # runLoop, executeLoopAction (the exec.Command + wall-clock)     ← P0-2 seam
├── auto_decide.go / auto_parallel.go / autocmd.go / multifeature.go
├── guards.go           # snapshotProgress, runScopeGuard, runEvidenceCheck
├── toolexec.go         # toolHeadlessArgs, buildToolCommand, model tiers                ← P0-2 seam
├── render.go           # tailWriter, claudeStreamWriter (NDJSON parser)                 ← P0-2 seam
├── worktree.go         # copyBelmontStateToWorktree, syncFeatureStateAfterMerge, taskBodyEnd
├── merge_conflict.go / reconcile.go / repair.go / reverify.go / steer.go / validate.go
└── testdata/eval/…     # Tier-1 eval fixtures
skills/belmont/_src/, _partials/, agents/belmont/, prompts/belmont/, knowledge/, docs/
.github/workflows/ci.yml, release.yml
```

**Files named in TECH_PLAN §File Structure that do not exist yet:** `cmd/belmont/metrics.go`, `metrics_test.go`, `extract.go`, `extract_test.go`, `slice.go`, `slice_test.go`, `config.go`, `config_test.go`, `journal.go`, `journal_test.go`. All are NEW. `fsutil.go` exists and is small.

### AGENTS.md / KNOWLEDGE.md rules that bind this milestone

- **`knowledge/KNOWLEDGE.md` first** (`AGENTS.md:5`). For M1 the matching entries are `knowledge/cross-cutting/progress-md-parsing.md` (any new PROGRESS.md structural reader — do not write inline marker/`## ` regexes; use `canonicalMarker` / `markerRank` / `isSectionBreak` / `orphanedTaskLines`), `knowledge/auto-mode/worktree-state-isolation.md` (P0-1 touches `syncFeatureStateAfterMerge` and `copyBelmontStateToWorktree`), `knowledge/auto-mode/scope-guard-runtime.md` and `verify-evidence.md` (P0-1 touches both guards' writes), and `knowledge/meta/evals.md` (Tier 1 vs Tier 2).
- **Amend knowledge entries in place**, one line in the `Revisions` footer; new concept → new file + a row in the routing table. TECH_PLAN asks for a new `knowledge/cross-cutting/state-atomicity.md` — that is a new entry, so it needs a KNOWLEDGE.md row too.
- **Both invocation paths or it's not done** — applies to anything touching tool integration or the on-disk surface. P0-2 touches `toolexec.go`, which is auto-mode only, but the metrics file lands under `.belmont/`, which is the shared surface; say so explicitly in the plan.
- **Docs are part of the change**: "always ensure the README and docs/ are up to date". P0-12 must therefore also touch `README.md:469` and `CONTRIBUTING.md:14`, both of which say "Go 1.21+".
- **After every change**: `go build ./cmd/belmont` and `go test ./cmd/belmont`. Tier 1 (`go test -tags eval ./cmd/belmont`) after touching state parsing, the decision engine, wave computation, or the guards — P0-1 touches the guards, so Tier 1 is required. Tier 1 **cannot** license a prose change; no M1 task is prose.
- `go vet ./...`, `go vet -tags eval ./cmd/belmont`, `GOOS=windows go vet ./...`, `staticcheck ./...`, `gofmt -l cmd/ scripts/` are all CI gates and all currently clean.

---

### P0-12 — Toolchain: exactly what is pinned

| Site | Current | Note |
|---|---|---|
| `go.mod:3` | `go 1.21` | no `toolchain` directive; module has **zero requires and no `go.sum`** |
| `.github/workflows/ci.yml:20` | `go-version: '1.21'` | job `test` (build, test, eval Tier 1, vet ×3, staticcheck, gofmt) |
| `.github/workflows/ci.yml:75` | `go-version: '1.21'` | job `generated` (generate-skills/plugin `--check`) |
| `.github/workflows/ci.yml:125` | `go-version: '1.21'` | job `cross-platform` (5-target build matrix) |
| `.github/workflows/release.yml:33` | `go-version: '1.21'` | tag-push cross-compile |
| `README.md:469` | "Go 1.21+ is needed to build from source" | prose |
| `CONTRIBUTING.md:14` | "Make sure you have **Go 1.21+** installed" | prose |

Seven sites. Local toolchain is `go1.24.4 darwin/arm64`.

**What could break, concretely:**
1. **Lockstep is mandatory.** Bumping `go.mod` without bumping the four `setup-go` pins leaves CI on 1.21; Go ≥1.21 would then auto-download a toolchain (GOTOOLCHAIN default `auto`) rather than fail loudly, so CI could silently build on a different toolchain than the one declared. That is precisely the contamination P0-12 exists to prevent. Bump all seven together.
2. **`staticcheck@latest`** is installed by `ci.yml:63` with `go install honnef.co/go/tools/cmd/staticcheck@latest`. Recent staticcheck releases require a newer Go to *build*; this is the one CI step that can fail on a version mismatch in either direction. If it breaks, pin staticcheck to a version rather than reverting the bump.
3. **Nothing in the source blocks the bump.** No post-1.21 stdlib package is imported (list above), no build constraint mentions a Go version, no generics beyond what 1.21 already supports.
4. **`-race` needs cgo on some platforms** but works out of the box on darwin/arm64 and linux/amd64. `go test -race ./cmd/belmont` is *not* in CI today — P0-1's proof will be the first race-detector run in this repo. Consider whether it should be added to `ci.yml` as part of P0-1 rather than P0-12 (P0-12's acceptance says "a single commit touching nothing else").

---

### P0-1 — Atomic state writes

#### There is no shared write helper today

Every state write is a bare `os.WriteFile` at the call site. The nearest thing to a helper is `copyFile` (`cmd/belmont/main.go:833`), which is `ReadFile` → symlink-unlink → `MkdirAll` → `os.WriteFile`, and `copyDir` (`main.go:860`) which recurses through it.

`fsutil.go` is the right home: 148 lines, imports `errors fmt os path/filepath strings`, already the file that owns `ensureStateFiles` and calls `ensureGitignoreEntry`. Adding `writeStateFile(path string, data []byte, perm os.FileMode) error` there adds no import.

#### The nine `PROGRESS.md` writers — verified, with file:line and enclosing function

| # | Site | Function | Writes | Error handling today |
|---|---|---|---|---|
| 1 | `cmd/belmont/feature.go:274` | `syncMasterFeatureStatuses` (feature.go:181) | master `.belmont/PROGRESS.md` feature table | **error discarded** |
| 2 | `cmd/belmont/guards.go:75` | `runScopeGuard` (guards.go:33) | feature `PROGRESS.md` via `pre.Path` | warns on stderr |
| 3 | `cmd/belmont/guards.go:258` | `runEvidenceCheck` (guards.go:227) | feature `PROGRESS.md` via `pre.Path` | warns on stderr |
| 4 | `cmd/belmont/merge_conflict.go:722` | `resolveProgressConflict` (merge_conflict.go:108) | the conflicted file (`filePath`) | returns `false` |
| 5 | `cmd/belmont/reverify.go:114` | `runReverifyCmd` (reverify.go:57) | feature `PROGRESS.md` | wrapped error |
| 6 | `cmd/belmont/state.go:1024` | `skipMilestoneInProgress` (state.go:977) | feature `PROGRESS.md` | returned |
| 7 | `cmd/belmont/repair.go:1293` | `runRepairCmd` (repair.go:1219), mechanical tier | feature `PROGRESS.md` | wrapped error |
| 8 | `cmd/belmont/repair.go:1502` | `runRepairCmd` (repair.go:1219), agent tier | feature `PROGRESS.md` | wrapped error |
| 9 | `cmd/belmont/worktree.go:413` | `syncFeatureStateAfterMerge` (worktree.go:368) | worktree feature `PROGRESS.md` (merged) | warns on stderr |

The PRD's figure of nine is **correct** and the TECH_PLAN's table maps exactly onto these lines.

#### The seven other Belmont-state writers — verified

| # | Site | Function | Writes | Error handling |
|---|---|---|---|---|
| 10 | `cmd/belmont/reconcile.go:333` | `writeReconciliationResolution` (reconcile.go:308) | any resolved conflict file | returned |
| 11 | `cmd/belmont/reconcile.go:372` | `reviewConflict` (reconcile.go:337), `e` (edit) branch | resolved content for `$EDITOR` | wrapped error |
| 12 | `cmd/belmont/steer.go:134` | `consumePendingSteering` (steer.go:102) | `STEERING.md` rewrite after consumption | **error discarded (`_ =`)** |
| 13 | `cmd/belmont/worktree.go:246` | `copyBelmontStateToWorktree` (worktree.go:211) | worktree `STEERING.md` (restore) | wrapped error |
| 14 | `cmd/belmont/worktree.go:269` | `copyBelmontStateToWorktree` (worktree.go:211) | `.belmont/.worktree` marker | **error discarded** |
| 15 | `cmd/belmont/worktree.go:1075` | `(*worktreeTracker).persistAutoJSON` (worktree.go:1056) | `.belmont/auto.json` | **"best-effort", discarded** |
| 16 | `cmd/belmont/worktree.go:1094` | `writeLoopAutoJSON` (worktree.go:1080) | `.belmont/auto.json` | **"best-effort", discarded** |

Sixteen total — the TECH_PLAN's refinement is accurate.

#### Sites the plan's sixteen do NOT cover, and that a "every state writer, not a subset" reading should

These are real and each is a genuine torn-read window. Flagging rather than deciding — the implementation agent should rule on each explicitly rather than discover them at review.

- **`cmd/belmont/main.go:846` — `copyFile`.** Called by `copyBelmontStateToWorktree` at `worktree.go:257` for the master context files (`PRD.md`, **`PROGRESS.md`**, `PR_FAQ.md`, `TECH_PLAN.md`, `worktree.json`) and, via `copyDir` (`main.go:860`, called at `worktree.go:241`), for **the entire feature directory including its `PROGRESS.md`** into a worktree. This is the highest-exposure write in the codebase for the exact hazard P0-1 names: the worktree copy is the *only* transport for that state (`.belmont/` is `--assume-unchanged` in worktrees) and it is written while `belmont status` in another process may be reading the same paths. It is not in the plan's sixteen.
- **`cmd/belmont/worktree.go:241` — `os.RemoveAll(dstFeature)` immediately before `copyDir`.** An atomic write helper does **not** fix this: for the duration of the recopy a concurrent reader sees *no file at all*, then a growing tree. Whatever P0-1 does, this window remains unless the recopy is staged into a sibling directory and renamed. Worth an explicit note in `knowledge/cross-cutting/state-atomicity.md` so the next session does not assume the helper closed it.
- **`cmd/belmont/steer.go:454` — `appendSteeringEntry`**, `os.OpenFile(..., O_APPEND|O_CREATE|O_WRONLY)`. Append, not truncate, so the torn-file mode differs — but a reader can still observe a half-written entry. Different fix (single `Write` of a complete block is already close to atomic for small writes on local fs); decide and record.
- **`cmd/belmont/main.go:1443` — `ensureGitignoreEntry`**, same `O_APPEND` shape, two separate `WriteString` calls (newline, then entry). Not Belmont state proper; low priority.
- **`cmd/belmont/fsutil.go:129` and `:139` — `ensureStateFiles`** template writes for `.belmont/PR_FAQ.md` and `.belmont/PRD.md`. Both guarded by `!fileExists`, so no reader is watching an existing file — but they are Belmont state and sit in the very file the helper is being added to. Cheapest possible conversion; do it for consistency.
- **Not Belmont state, leave alone**: `install.go:368/390/605/727`, `install_sync.go:448/504/577`, `main.go:757`, `monorepo.go:33/51` (`.env` seeding), `worktree.go:92` (`copyEnvFiles`), `main.go:959` (release download), `main.go:971` (`checkWriteAccess` probe).

#### Constraints the helper must satisfy

1. **Sibling temp file, not `os.TempDir()`.** `os.Rename` is atomic only within a filesystem. `.belmont/` and `/tmp` are frequently different volumes on macOS. Use `os.CreateTemp(filepath.Dir(path), ".belmont-tmp-*")`.
2. **`f.Sync()` before `Close()`**, then `os.Rename`. Optionally fsync the parent directory; if you skip it, say why (crash-durability is not what P0-1 promises — visibility to a concurrent reader is).
3. **Permissions.** `os.CreateTemp` creates `0600`. Every existing site writes `0644`/`0o644`, so the helper must `Chmod(perm)` (or `Fchmod`) before the rename, or the first atomic write silently tightens every `PROGRESS.md` in the tree.
4. **`writeReconciliationResolution` (reconcile.go:308) is symlink-aware** — it detects an existing symlink, removes it, and may recreate the target *as a symlink* when the resolved content is a single-line path. A blind rename-based replacement destroys that behaviour. Either leave the symlink branch above the helper call, or make the helper symlink-aware. This is the one site where a mechanical swap is wrong.
5. **`copyFile` (main.go:833) already unlinks an existing symlink** before writing. If `copyFile` is converted, keep that.
6. **Windows.** `ci.yml:52` runs `GOOS=windows go vet ./...` and `ci.yml:125` cross-builds `windows/amd64`. Go's `os.Rename` maps to `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`, so overwrite-by-rename works — but it fails if another process holds the destination open. Note it; do not add a `_windows.go` variant unless a test forces one (`process_unix.go` / `process_windows.go` is the existing precedent for build-constrained files, and CI already vets both).
7. **Six of the sixteen discard the write error today** (feature.go:274, steer.go:134, worktree.go:269, worktree.go:1075, worktree.go:1094, plus `copyFile`'s callers at worktree.go:241/257). Converting them to a helper that returns `error` means those call sites need an explicit `_ =` or a real handler. `staticcheck` does not flag unchecked errors by default (that is `errcheck`, which is not in CI), so this will not break the build — but silently swallowing a *rename* failure is worse than swallowing a truncate failure, because the temp file is left behind. Prefer at least a stderr warning at the three `auto.json`/marker sites.

#### The parse sites — where a torn file surfaces

The TECH_PLAN's "seventeen parse sites across three dialects" refers to **grammar regexes**, not file reads. Verified enumeration of the register-grammar parsers (a torn write is observed at these, so each is a place the bug is *visible*, and each is a place M3 must not break):

*Milestone-heading regexes (12):* `state.go:479` (`msHeaderRe`, package-level), `state.go:483` (inside `parseMilestones`), `state.go:985` (`skipMilestoneInProgress`), `state.go:1082` (`pendingTasksInRange`), `state.go:1131` (`fwlupTasksInRange`), `guards.go:161`, `guards.go:457`, `guards.go:538`, `merge_conflict.go:147`, `reverify.go:26`, `repair.go:822`, `worktree.go:503`.

*Task-line regexes (14):* `state.go:403` (`mergeTaskLineRe`), `state.go:406` (`taskIDRe`), `state.go:441` (`orphanedTaskLines`), `state.go:486` (`parseMilestones`), `state.go:986`, `state.go:989`, `state.go:1084`, `state.go:1133`, `guards.go:475`, `guards.go:542`, `merge_conflict.go:335`, `reverify.go:27`, `repair.go:410`, `repair.go:683`, `repair.go:823`.

*Dependency-annotation regex, duplicated verbatim (2):* `state.go:484` and `guards.go:458` — identical `\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`.

**Three dialects confirmed:** (a) strict `^###\s+M(\d+):` — `state.go:479`, `state.go:483`; (b) emoji-tolerant `^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):` — guards, merge_conflict, reverify, repair, worktree; (c) case-insensitive `(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):` — `state.go:985/1082/1131`. Grouped by enclosing function this is 17–18 sites, matching the plan. **No M1 task changes this grammar**; the enumeration is here because the codebase agent was asked to establish it and because P0-4's census must read through the same parsers.

Separately, the **file-read** sites that would observe a torn `PROGRESS.md` number **27**, not 17: `auto_parallel.go:499`, `autocmd.go:168`, `autocmd.go:377`, `blockers.go:159`, `feature.go:73`, `feature.go:96`, `feature.go:102`, `feature.go:183`, `guards.go:23`, `guards.go:41`, `guards.go:234`, `repair.go:385`, `repair.go:1242`, `repair.go:1475`, `repair.go:1540`, `reverify.go:88`, `state.go:102`, `state.go:617`, `state.go:979`, `state.go:1073`, `state.go:1122`, `status.go:125`, `validate.go:237`, `validate.go:258`, `worktree.go:393`, `worktree.go:404`, plus `main.go:834` (`copyFile`'s read, reached from `worktree.go:241/257`). None of them need changing for P0-1 — they are the blast radius, not the fix.

#### Test placement for the race proof

No concurrent read-during-write test exists. There is no `t.Parallel()` anywhere and the only two `go func()` in tests (`repair_test.go:1061`, `withdrawn_test.go:611`) feed stdin. `go test -race` is **not** in `ci.yml` today. Existing helpers to reuse: `t.TempDir()`, `mustWrite`, `runGit` (see `worktree_state_test.go:13-35` for the canonical setup shape). New file should be `cmd/belmont/fsutil_test.go` — note `fsutil_test.go` does not exist yet.

---

### P0-2 — Instrumentation seams

#### Where the invocation happens

- **`cmd/belmont/toolexec.go:274 toolHeadlessArgs(tool, prompt, root, modelFlags, streaming)`** builds the argv per tool. This is the single place that knows each tool's output format.
- **`cmd/belmont/toolexec.go:336 buildToolCommand`** — non-streaming path, used by AI-decision / triage-template / reconciliation calls.
- **`cmd/belmont/auto_loop.go:268`** — `exec.Command(toolBinary(cfg.Tool), args...)` in `executeLoopAction` (auto_loop.go:234). This is the loop's main shell-out.
- **`cmd/belmont/auto_loop.go:~440`** — the same shape inside `executeTriageAction` (auto_loop.go:368).
- Two more `claudeStreamWriter` consumers outside the loop: `reverify.go:194` and `repair.go:1181`.

#### Wall-clock is already measured

`auto_loop.go:316 start := time.Now()` … `auto_loop.go:344 durationMs := time.Since(start).Milliseconds()`, and the mirror at `:451`/`:477` for triage. It already lands in `executionResult.DurationMs` (`types.go:191-196`) and is already printed at `auto_loop.go:224`. P0-2 needs no new timing — only a place to persist it.

Everything the JSONL record needs is in scope at that point: `cfg.Feature`, `cfg.Tool`, `action.MilestoneID`, `string(action.Type)` (the phase — the eleven `loopActionType` constants at `types.go:161-174`), and `durationMs`. `historyEntry` (`types.go:212-226`) already carries `Result *executionResult` plus `GitSHA`/`PostGitSHA`, if a richer record is wanted.

#### Where token usage is (and is not) reachable

**Claude — reachable, and the parser already exists.** `render.go:230-295 claudeStreamWriter` consumes the `stream-json` NDJSON line by line. Its `streamLine` struct (`render.go:238-241`) declares only `Type` and `Message`, and **`render.go:270-272` discards every line whose `type` is not `"assistant"` — which is exactly where the `result` event carrying the `usage` block is thrown away.** The seam is: add `Subtype`/`Usage` fields to `streamLine`, capture the `result` event's usage into a field on `claudeStreamWriter`, and expose a getter. Four construction sites to thread through: `auto_loop.go:289`, `auto_loop.go:424`, `reverify.go:194`, `repair.go:1181`.

**Every other tool — a real obstacle the plan does not mention.** Non-Claude tools write stdout straight into `tailWriter` (`auto_loop.go:292-294`, `:427-429`), and `tailWriter` (`render.go:187-228`) **keeps only the last 1500 bytes** (`newTailWriter(os.Stderr, 1500, prefix)` at `auto_loop.go:287` and `:421`). `executionResult.Output` is therefore the tail of the stream, not the stream. Consequences:

- `codex exec --json` emits an NDJSON event stream; a final usage event *may* fall inside the last 1500 bytes, but that is luck, not a contract.
- `gemini --output-format json` and `cursor --output-format json` emit a single JSON document. If usage sits anywhere but the final ~1500 bytes it is unrecoverable from `Output`.
- Fix options, both cheap: raise the tail size for tools whose usage must be parsed, or tee a second small "last complete JSON object" capture writer alongside `tailWriter`. Decide this before writing `metrics.go` — retrofitting it after the record format is fixed is the expensive order.

**Hosts that genuinely cannot report usage — confirmed in code, matching the TECH_PLAN table:**

| Tool | argv (toolexec.go) | Usage |
|---|---|---|
| `copilot` | `toolexec.go:295-299` — `-p <prompt> --yolo`, no output-format flag | **No** — wall-clock only |
| `pi` | `toolexec.go:304-313` — `-p` plus optional `--provider/--model`; comment states "Pi's `-p` does not emit structured JSON" | **No** — wall-clock only |
| `opencode` | `toolexec.go:315-329` — `run --dangerously-skip-permissions`; comment records that `--format json` is **deliberately not passed** because its escaped event stream defeats `extractDecisionJSON` | **No** — wall-clock only |

The opencode note is load-bearing per the plan (it is the Oct-2026 local-LLM target) and the code comment already explains *why* the JSON format was rejected — do not "fix" it to get usage without re-solving `extractDecisionJSON` (`toolexec.go:357`).

#### Critical-path attribution

`computeWaves` (`cmd/belmont/feature.go:494-567`) is a **Kahn topological levelling**, not a longest-path computation. Its output is `[]wave` with `wave.Index`; a milestone's wave index is *not* its position on the critical path (a milestone can sit in wave 2 and be off every longest chain). P0-2's `critical_path` flag therefore needs new logic — a longest-chain-through-the-DAG pass over the same `milestone.Deps` data. `computeWaves` already builds `byID` and validates for cycles (`feature.go:534-543`), so the new pass can reuse the same inputs.

Note while you are in there (this is **M10 / P2-4**, not M1 — do not fix it here): `feature.go:513` reads `if dm, ok := byID[dep]; ok && !milestoneAllDone(dm)`, so a dependency naming a milestone that does not exist contributes **zero** to in-degree and the milestone becomes first-wave eligible. The feature-level equivalent in `multifeature.go` errors correctly.

#### Gitignoring the metrics directory

Copy the existing pattern verbatim: `fsutil.go:112-113` inside `ensureStateFiles` already does

```go
ensureGitignoreEntry(projectRoot, ".belmont/auto.json")
ensureGitignoreEntry(projectRoot, ".belmont/worktrees/")
```

`ensureGitignoreEntry` is defined at `cmd/belmont/main.go:1434-1454` (idempotent `strings.Contains` check, then `O_APPEND`). Add `.belmont/metrics/` next to those two. This repo's own `.gitignore` already lists `.belmont/auto.json` and `.belmont/worktrees/` at lines 26-27 — add the metrics entry there too, since Belmont is self-hosting here and `ensureStateFiles` only runs on `install`, which rule 2 forbids running between M1 and M11.

---

### P0-3 — Baseline

No code seam of its own; it is a run of the P0-2 instrumentation against `~/repo-3` and `~/repo-4`. Two things to record with the baseline so M11's comparison is defensible: the Belmont commit (`git rev-parse HEAD` in this repo) **and** the Go toolchain version, since P0-12 is the reason for the ordering. The pinned orchestrator is Homebrew v0.11.0 at `/opt/homebrew/bin/belmont`; the baseline is taken with that binary, not a worktree build.

---

### P0-4 — Extraction census

#### Nothing resembling `extract.go` exists

`cmd/belmont/extract.go` is absent. There is no `--dry-run` machinery, no size-reporting command, and no code anywhere that separates a task's indented body from its head line *for output*. The subcommand switch is a plain `switch os.Args[1]` at `main.go:295-325` with `printUsage` at `main.go:329-347` — adding `extract` means one `case` and one usage line, no registry to touch.

#### The pieces that already exist and should be reused

| Piece | Location | Why it matters to the census |
|---|---|---|
| `listFeaturesWithOverrides` | `feature.go:28` | the only existing walker of `.belmont/features/*`; skips non-dirs and dot-dirs. `--all` should iterate the same way |
| `validateFeature` | `validate.go:196` | per-feature scanner, already resolves live worktree overlays; the shape a per-feature census report should mirror |
| `parseMilestones` | `state.go:481` | the canonical parser; returns `[]milestone` with `task.Line` **1-based** (`state.go:539`) |
| **`taskBodyEnd`** | **`worktree.go:449-469`** | **the exact indentation-based body separator the census needs.** Bounds a task's body by the task's own indent; a sibling checkbox at the same indent ends it; trailing blanks belong to the document, not the task |
| `lineIndentWidth` | `worktree.go:474-476` | tabs count as one, like spaces — matches the "deeper than" semantics the census needs |
| **`taskDetail`** | **`blockers.go:356-372`** | closest prior art: given `lines` and a 1-based `line`, returns the task's indented body with indent stripped, using `taskBodyEnd` |
| `orphanedTaskLines` | `state.go:440` | task-shaped lines outside every milestone — the census must decide whether these count toward register size (they are inside the file but under no `M<n>`) |
| `isSectionBreak` | `state.go:314` | bounds the milestones region; a column-zero `##` ends it (issue #31) |
| `canonicalMarker` / `markerRank` | `state.go:220` / `:260` | never write an inline marker regex — `knowledge/cross-cutting/progress-md-parsing.md` forbids it |

**The join that makes extraction mechanical:** `task` (`types.go:58-73`) has **no `Body` field** — `parseMilestones` records `Line` and `Marker` but never captures the indented body. So `parseMilestones` gives you every task's line number, and `taskBodyEnd(lines, line-1)` gives you that task's body extent. Those two together are the whole separator. `blockers.go:356` already composes them; `extract.go` should do the same rather than write a third line-scanner.

#### Census scope, measured

The "five repos" resolve locally to `~/repo-3`, `~/repo-4`, `~/repo-5`, `~/repo-1`, `~/repos/belmont`:

| Repo | Feature dirs | With `PROGRESS.md` | Archived (`ARCHIVE.md`) |
|---|---|---|---|
| repo-3 | 39 | 29 | 9 |
| repo-4 | 17 | 14 | 0 |
| repo-5 | 70 | 11 | 59 |
| repo-1 | 11 | 10 | 0 |
| repos/belmont | 1 | 1 | 0 |
| **Total** | **138** | **65** | **68** |

Distribution of the 65 live registers: total 5,002,869 B, median 16,516 B, p90 71,379 B, max 1,860,979 B. The max is `~/repo-4/.belmont/features/feat-075/PROGRESS.md` at exactly the 1,860,979 bytes the PRD's composition analysis cites — **the PRD's worst-case figure reproduces**. Master files are extra: `~/repo-3/.belmont/PROGRESS.md` is 498,879 B, `~/repo-4/.belmont/PROGRESS.md` 247,645 B.

Three notes for the implementer:
1. **The PRD's "the other 83" does not match what is on disk.** 138 feature directories exist; only 65 carry a live `PROGRESS.md` (68 have been archived to `ARCHIVE.md` by `/belmont:cleanup`). Whichever number the report uses, state the denominator and whether archived directories are counted — otherwise the census's own coverage claim is unverifiable.
2. **Crude pre-answer to the open question P0-4 exists to settle** (byte-proportional, applying the worst file's measured 38% post-extraction residue to every file — *not* a substitute for the real per-file measurement): only three registers currently exceed ~263 KB, i.e. would plausibly still exceed 25,000 tokens after extraction — `repo-4/feat-075` (1,860,979 B), `repo-3/feat-015` (1,022,749 B), `repo-1/feat-020` (360,542 B). Seven exceed 100 KB today. If the real census confirms three or fewer, M4 does **not** become a prerequisite of M3.
3. **Worktree copies will double-count.** `~/repo-4/.claude/worktrees/agent-*/` contains a 1,293,057 B copy of the same `feat-075/PROGRESS.md`. The census walker must scope to `<root>/.belmont/features/` and not glob for `PROGRESS.md` across the tree.

---

### P0-13 — Branch triage (git state, verified)

- **Unmerged remote branches: 35 confirmed**, via `git branch -r --no-merged main` excluding `upstream/*` and `HEAD`. Matches the plan.
- **`origin/main` is at `e0cd0d07` (2026-04-10, "Release v0.9.1"); local `main` is at `ffd4b4bc` (2026-08-18) and is 156 commits ahead, 0 behind.** The default branch does misrepresent the repo, exactly as the task says. `git describe --tags main` → `v0.11.0`. `upstream/main` (blake-simpson) is at `17897aa2` (2026-08-16).
- **`docs/proposals/` does not exist on `main`** — confirmed. It exists only on `origin/docs/pr-proposals`, which carries 0003–0006 plus `NEXT-SESSION.md`, `README.md`, and the rev5/rev6 review artefacts (16 files, all under `docs/proposals/`, plus `.gitignore`). Last commit 2026-08-08.
- **Path conflict worth deciding during triage:** `origin/proposal/0001-quick-mode` and `origin/proposal/0002-prd-hygiene` (both 2026-05-23) put their files at **root `proposals/`**, not `docs/proposals/`. Two conventions are in flight; the triage verdict should pick one, because the TECH_PLAN commits this feature to writing 0007–0009 and rule 5 of the master TECH_PLAN names `docs/proposals/NNNN-*.md`.
- **Working tree is clean** apart from the untracked `MILESTONE.md` this milestone created.

#### Branches touching files M3 / M4 / M6 will rewrite

M3 rewrites `state.go`, `validate.go`, `merge_conflict.go`, `worktree.go`. M4 adds `slice.go` and touches `main.go` + the read-path skills. M6 rewrites `guards.go`, `auto_decide.go`, `auto_loop.go`, `autocmd.go`. Measured with `git diff --name-only main...<branch>`:

| Branch | Last commit | Collides with | Files that collide |
|---|---|---|---|
| `origin/fix/unrecognised-task-markers` | 2026-08-11 | **M3, M6, M10** | `state.go`, `merge_conflict.go`, `worktree.go`, `validate.go`, `feature.go`, `repair.go`, `reverify.go`, `guards.go`, `auto_decide.go`, `auto_parallel.go`, `autocmd.go`, `render.go`, `types.go`, `main.go` — **the widest collision in the backlog, 66 files** |
| `origin/feat/maintenance-ci` | 2026-08-07 | **M1, M3, M6, everything** | touches **every** `cmd/belmont/*.go` including **`fsutil.go`** and `toolexec.go`, plus `.github/workflows/ci.yml`. This is the only branch that collides with **P0-12 and P0-1 directly** and the plan's table does not list it |
| `origin/fix/state-readers-and-live-views` | 2026-08-14 | **M3, M4, M6** | `state.go`, `merge_conflict.go`, `worktree.go`, `validate.go`, `feature.go`, `main.go`, `auto_loop.go`, `autocmd.go`, `status.go`, `docs/feature-auto.md` (which **M10/P2-5** also rewrites) |
| `origin/fix/post-51-triage` | 2026-08-17 | **M3, M4** | `worktree.go`, `validate.go`, `feature.go`, `reverify.go`, `repair.go`, `reconcile.go`, `autocmd.go`, `status.go`, `main.go`, `install.go`, `steer.go` — newest branch in the backlog |
| `origin/fix/dropped-state-and-dispatch` | 2026-08-16 | **M3, M8** | `state.go`, `merge_conflict.go`, `validate.go`, `feature.go`, `guards.go`, `types.go`, `_partials/dispatch-strategy.md` (**M8/P1-11**) |
| `origin/fix/repair-move-and-guidance` | 2026-08-13 | **M3** | `state.go`, `worktree.go`, `guards.go`, `repair.go` |
| `origin/fix/wave-merge-state-loss` | 2026-08-07 | **M3 (P1-14, P1-15)** | `worktree.go`, `auto_parallel.go`, `knowledge/auto-mode/worktree-state-isolation.md` |
| `origin/feat/loop-efficiency` | 2026-08-13 | **M6, M2** | `auto_loop.go`, `_partials/loop-recipe.md` (**P0-6** corrects its scale factor), `_src/implement.md`, `_src/loop.md`, `_src/next.md` |
| `origin/feat/verify-dedup` | 2026-04-12 | **M2 (P0-5)** | `skills/belmont/verify.md` — note this is the **pre-`_src/` layout**, so it cannot merge cleanly; it is a rewrite-or-abandon, not a merge |
| `origin/feat/design-contract` | 2026-08-13 | **M1, M6** | `auto_loop.go`, `guards.go`, **`toolexec.go`**, `worktree.go`, `steer.go`, `eval_harness_test.go` — 80+ files |
| `origin/feat/eval-harness` | 2026-08-08 | **M1 verification** | `.github/workflows/ci.yml`, `eval_harness_test.go`, all eval fixtures. `eval_harness_test.go` **is already on `main`**, so this is likely superseded |
| `origin/backup/loop-efficiency-pre-rebase`, `origin/backup/state-readers-pre-main-rebase`, `origin/backup/state-readers-pre-rebase` | 2026-08-13/14 | — | explicit pre-rebase backups of three of the above; almost certainly `abandon`, but confirm the rebased version actually landed before deleting |

The plan's table names five collisions. The measured answer is **eleven** branches touching files M1–M6 rewrite, and the two most dangerous — `origin/feat/maintenance-ci` (touches `fsutil.go`, `toolexec.go` and `ci.yml`, i.e. all three of P0-12/P0-1/P0-2) and `origin/fix/unrecognised-task-markers` (66 files across the whole state layer) — are **not** in the plan's list. Triage those two first.

Remaining branches, no collision with M1–M6 Go work (skills/docs/plugin only, or narrow): `origin/codex-model-tier-overrides`, `origin/codex-plan-apply-handoff`, `origin/codex-skill-display-names`, `origin/feat/claude-loop-skill`, `origin/feat/declsum` (single new file `scripts/declsum/main.go`), `origin/feature/agent-dedup-fix`, `origin/fix-gemini-ask-user`, `origin/fix/plugin-generator-empty-agents`, `origin/fix/worktree-git-excludes`, `origin/framework-awareness`, `origin/milestone-dereference-prd`, `origin/opencode-slash-commands`, `origin/opencode-support`, `origin/opus-default-model-selection`, `origin/skill-progressive-disclosure`, `origin/skill-progressive-disclosure-rest`, `origin/status-colors`, `origin/trim-dispatch-strategy`, `origin/docs/pr-proposals`, `origin/proposal/0001-quick-mode`, `origin/proposal/0002-prd-hygiene`. Several of the pre-June ones (`status-colors`, `skill-progressive-disclosure*`, `milestone-dereference-prd`, `trim-dispatch-strategy`) predate the `_src/` + generated-skills layout and edit files that are now **gitignored generated output** (`skills/belmont/implement.md`, `skills/belmont/verify.md`, …) — they cannot merge as-is.

---

### Code patterns to follow

**Error handling.** Wrapped with `%w` and a command prefix when returned to `main`: `fmt.Errorf("repair: write %s: %w", progressPath, err)` (repair.go:1294). Warn-and-continue on stderr with ANSI colour when the operation is advisory: `fmt.Fprintf(os.Stderr, "\033[33m⚠ scope guard write failed: %s\033[0m\n", err)` (guards.go:76). Genuinely best-effort writes are commented as such (`worktree.go:1075`).

**Comment style.** This codebase carries unusually dense *rationale* comments — most non-obvious lines explain the defect they prevent, often with an issue number (`#24`, `#27`, `#29`, `#31`, `#34`). Match that. A new atomic-write helper is expected to say in its doc comment why the temp file must be a sibling, not merely that it is one; the TECH_PLAN already drafts that comment.

**Tests.** `package main`, table-free explicit sub-cases, `t.TempDir()`, helpers `runGit(t, root, ...)` and `mustWrite(t, path, content)`. Test names read as assertions: `TestWorktreeTrackedBelmontEditsAreNotCommittable`. Each test file carries a header comment saying which defect it pins.

**Subcommand shape.** `runXCmd(args []string) error` with its own `flag.FlagSet`, dispatched from `main.go`'s switch, plus a `printUsage` line. `--root PATH` and `--format text|json` on every command.

**No inline state regexes.** Route through `canonicalMarker`, `markerRank`, `isSectionBreak`, `orphanedTaskLines`, `parseMilestones`, `taskBodyEnd`, `lineIndentWidth`.

### Dependencies to use

Stdlib only — `os`, `path/filepath`, `encoding/json`, `time`, `sync`. **No new module requires and no `go.sum`**, per master TECH_PLAN rule 4. JSONL is one `json.Marshal` per line plus `O_APPEND`; nothing else is needed for `metrics.go`.

### Files to NOT modify in this milestone

- `skills/belmont/_src/*` and `_partials/*` — no M1 task is prose; changing them requires Tier 2 evals (`AGENTS.md:59`) and would violate the pinning rule.
- `agents/belmont/*.md` — M8/P1-11 territory.
- `skills/belmont/<skill>/` — gitignored generated output; edit `_src/` and regenerate, never the output.
- `.agents/` — gitignored; `belmont install` must not be run (master TECH_PLAN rule 2).
- `.belmont/features/throughput/PRD.md`, `TECH_PLAN.md` — spec, not deliverable.
- `cmd/belmont/state.go`'s task-line and milestone-heading grammar — frozen by decision, changed by nothing in M1.

### Warnings / considerations

1. **`os.RemoveAll` before `copyDir` at `worktree.go:241` is a hole no write helper closes.** Record it in `knowledge/cross-cutting/state-atomicity.md` so the next session does not read "atomic state writes: done" and assume otherwise.
2. **`writeReconciliationResolution` must keep its symlink branch** (reconcile.go:308-334). A mechanical swap to a rename-based helper silently converts symlinks to regular files.
3. **`os.CreateTemp` yields `0600`.** Without an explicit chmod, the first atomic write tightens permissions on every state file it touches.
4. **`tailWriter`'s 1500-byte cap is the blocker for non-Claude usage capture** (`render.go:199-205`, `auto_loop.go:287`/`:421`). Settle this before fixing the JSONL record shape.
5. **Wave index ≠ critical path.** `computeWaves` (feature.go:494) is Kahn levelling; `critical_path` needs a new longest-chain pass.
6. **`go test -race` has never run in this repo** and is not in CI. Expect the first run to surface pre-existing races unrelated to P0-1 — `worktreeTracker` (worktree.go:1056) mutates shared state from the parallel wave runner and is the likeliest source. Budget for triage; do not silently widen P0-1's scope to fix them.
7. **P0-12's "single commit touching nothing else" collides with AGENTS.md's docs rule.** README.md:469 and CONTRIBUTING.md:14 both state the version. Include them — they are the same change, not other work.
8. **`origin/feat/maintenance-ci` touches `fsutil.go`, `toolexec.go` and `ci.yml`.** If it is ever going to merge, it must merge *before* P0-12 and P0-1, or be abandoned. It is the one branch in the backlog that collides with M1 itself, and the plan's collision table omits it.
9. **The eval harness is build-tagged.** `go test ./cmd/belmont` does not compile `eval_harness_test.go`. P0-1 touches the guards, so run `go test -tags eval ./cmd/belmont` and `go vet -tags eval ./cmd/belmont` too.
10. **`gofmt -l cmd/ scripts/` must print nothing** and `GOOS=windows go vet ./...` must pass — both are CI gates and both are easy to miss locally.

## Design Specifications
[Not populated — no design input: no task in this milestone has a Figma URL,
reference image, or visible UI.]

## Implementation Log
[Written by implementation-agent — per-task status, files changed, commits, issues]
