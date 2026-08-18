## Pass 1 — /belmont:implement (M1 full pipeline, 2026-08-18)

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
   > **WITHDRAWN by `P0-M1-FIX-2` (2026-08-18).** This scan omitted `repo-2`, one of the five repos the PRD names. Over all five the count is **168 directories / 82 live registers**, and the PRD's "82 active + 57 archived = 139" reproduces exactly. The PRD was right. Only the closing instruction — state the denominator and its scope — survives. Canonical figures: `CENSUS.md` §The denominator, stated.
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

*Written by implementation-agent, 2026-08-18. Baseline `ffd4b4bc` → `a42c45ea`.*

### Summary
- **Tasks Completed**: 6 of 6 (P0-12, P0-13, P0-1, P0-2, P0-3, P0-4)
- **Tasks Blocked**: 3 new `[!]` sub-tasks recording deliberately-deferred clauses (P0-13a, P0-3a, P0-4a)
- **Total Commits**: 6, one per task
- **Gate suite green on every commit**: `go build`, `go test`, `go test -race`, `go test -tags eval`, `go vet` (default / `-tags eval` / `GOOS=windows`), `staticcheck`, `gofmt -l cmd/ scripts/`, `generate-skills.sh --check`, `belmont validate`

### Headline findings

1. **The branch backlog is a phantom.** Both branches the codebase scan flagged as most dangerous are already fully landed on `main`. M3, M4 and M6 are not standing on contested ground.
2. **P0-4 answered the PRD's open question YES.** Five registers' indexes still exceed 25,000 tokens after extraction, not three — and for three of them extraction moves *nothing*, because their bulk is task head lines rather than indented narrative. Per the PRD's own terms the wave structure changes; escalated as `[!] P0-4a`.
3. **P0-2 found a blocker the plan did not name and settled it before fixing the record shape**, as instructed.
4. **`go test -race` was already clean** before P0-1 — warning 6's predicted pre-existing races did not materialise.

---

### Task: P0-12 — Bring the toolchain onto a supported version

**Status**: SUCCESS · **Commit**: `dc6ba7e9`

Measured against the module proxy rather than assumed: latest stable Go is **1.26.6**, with 1.27 at rc3. Both 1.21 and the locally installed 1.24.4 are outside Go's two-release support window. Bumped to `go 1.26.0`.

**Files Modified**: `go.mod` (`go 1.21` → `go 1.26.0`); `.github/workflows/ci.yml` ×3 (test, generated, cross-platform); `.github/workflows/release.yml` ×1; `README.md:469`, `CONTRIBUTING.md:14` (`Go 1.21+` → `Go 1.26+`).

All seven sites bumped in lockstep, per the warning that a partial bump fails silently — `GOTOOLCHAIN` defaults to `auto`, so CI left on the old pin would download a different toolchain rather than error.

**Ruling on warning 7** (single-commit-vs-docs): README and CONTRIBUTING are included. They state the same version; AGENTS.md requires docs to move with the change. This is the same change, not other work.

**Hazard 2 materialised and was fixed rather than worked around.** `staticcheck` failed after the bump: `file requires newer Go version go1.26 (application built with go1.25)`. Cause is staticcheck's *build* toolchain, not its version. Fix is `GOTOOLCHAIN=go1.26.0 go install honnef.co/go/tools/cmd/staticcheck@latest`. **CI is unaffected** — `setup-go` installs 1.26 before the `go install`, so CI's staticcheck is already built with 1.26. No pin needed.

**Verification**: all gates green on go1.26.0, plus all five cross-compile targets. A pre-existing, unrelated failure was confirmed by stashing the bump: `go build -tags embed` fails with `pattern all:agents: no matching files found` on `go 1.21` too — `//go:embed` resolves relative to `cmd/belmont/`, and `scripts/build.sh` stages those directories first. Not a regression.

---

### Task: P0-13 — Triage the unmerged branch backlog

**Status**: SUCCESS (classification and documentation, per instruction) · **Commit**: `d417b0c2`

**File Created**: `docs/branch-triage.md` — all 35 branches, each with a verdict and a one-line reason. Verified programmatically to match `git branch -r --no-merged main` exactly: 35 rows, no duplicates, none invented.

**Verdicts**: 2 merge, 3 rebase, 30 abandon.

**The two branches flagged as most dangerous are phantoms.**
- `origin/fix/unrecognised-task-markers` — "the widest collision in the backlog, 66 files" — is **24/24 patches already on `main`**.
- `origin/feat/maintenance-ci` — "the only branch that collides with P0-12 and P0-1 directly" — is 29/31 landed, and both apparent leftovers were hand-verified present (the `worktree-state-isolation` row in `knowledge/KNOWLEDGE.md`; the `main.go`→`autocmd.go` repointing in `clean-tree-preflight.md`).
  > **WITHDRAWN by `P0-M1-FIX-3` (2026-08-18; annotation added by `P0-M1-FIX-8`).** *"Both apparent leftovers were hand-verified present"* is wrong, and so are the two changes it names. The leftovers are `1129cf84` and `7a9b7b56`. `1129cf84` — 23 lines adding §*Gotcha — `--max-parallel` does not by itself produce worktrees* to `knowledge/auto-mode/parallel-wave-orchestration.md` — was **genuinely absent from `main`** and has since been cherry-picked as `23e71008`. The `worktree-state-isolation` row in `knowledge/KNOWLEDGE.md` and the `main.go`→`autocmd.go` repointing in `clean-tree-preflight.md` are both on `main`, but neither is what either leftover commit contains: they were picked out of a three-dot diff, which shows every change since the merge-base and so cannot say which change belongs to which unlanded SHA. **The abandon verdict itself stands**, now on evidence rather than on a commit count, and the branch still does not collide with P0-12/P0-1/P0-2. Canonical verdict and evidence: `docs/branch-triage.md` §*Abandon — content verified on `main`* and its §Revisions entry for `P0-M1-FIX-3`.
- **Four of the TECH_PLAN's five named collisions are already on `main`.** Only `feat/verify-dedup` survives, and it is unmergeable for an unrelated reason — it edits the pre-`_src/` layout, now gitignored generated output, so it is a re-author (fold into M2/P0-5), not a merge.

**The real hazard the plan did not name**: `origin/fix/post-51-triage` — newest branch (2026-08-17), 15 genuinely unlanded commits, six new regression tests, and it merges with **zero conflicts**. It touches `reconcile.go` and `worktree.go`, which P0-1 rewrites.

**`origin/docs/pr-proposals` → merge.** 15 files under `docs/proposals/` exist nowhere on `main`, including `0004-context-budget-with-evidence.md` rev 5, M2's specification. `git merge-tree` yields exactly one conflict, in `.gitignore`. The triage also records how M2 reads 0004 *before* that merge (`git show origin/docs/pr-proposals:…`), so **M2 is not gated on it**.

**Proposals-path convention settled**: `docs/proposals/` is canonical (larger, far more reviewed set; the path named by master TECH_PLAN rule 5; where 0007–0009 will live). 0001/0002 relocate on rebase.

**Deliberately not done**: the "default branch matches the working state" clause. `origin/main` is 157 commits behind, so satisfying it means publishing 157 commits to a public fork. Recorded as `[!] P0-13a` and escalated to the repository owner. **No push, remote deletion, merge, rebase or PR was performed**; every `abandon` verdict is a recommendation to delete.

Written to `docs/branch-triage.md` rather than the TECH_PLAN's nominated `docs/proposals/NEXT-SESSION.md`, because that file exists only on `origin/docs/pr-proposals` — writing there would manufacture a conflict in the branch this triage most wants to land cleanly.

**Method note**: `git branch --no-merged` over-reports badly in a rebase-heavy repo (21 of 35 branches were fully or effectively landed). `git cherry` is the right primitive, with hand-checks on the residue. Two measurement traps are recorded in `NOTES.md`, both of which produced a wrong answer first: `git rev-parse "main:$path"` echoes unresolvable arguments back, so every missing file reads as present unless `--verify --quiet` is used; and two-dot `git diff main..<branch>` is meaningless when `main` is 157 ahead.

---

### Task: P0-1 — Atomic state writes

**Status**: SUCCESS · **Commit**: `8e5981c7`

**Files Created**: `cmd/belmont/fsutil_test.go` (race proof + 5 supporting cases), `knowledge/cross-cutting/state-atomicity.md`.
**Files Modified**: `fsutil.go` (+`writeStateFile`, 2 template writes), `feature.go`, `guards.go` ×2, `merge_conflict.go`, `reverify.go`, `state.go`, `repair.go` ×2, `worktree.go` ×5, `reconcile.go` ×2, `steer.go`, `main.go` (`copyFile`), `knowledge/KNOWLEDGE.md` (routing row).

**19 call sites** = the plan's 16, plus `copyFile` and the two `ensureStateFiles` templates. Every remaining `os.WriteFile` was confirmed non-state and is documented as such.

**Rulings on each flagged site, as required:**

| Site | Ruling |
|---|---|
| `main.go:846` `copyFile` | **Converted.** The plan's sixteen missed it; it is the transport for `PROGRESS.md` into a worktree and the highest-exposure site in the tree. Symlink-unlink preserved. |
| `reconcile.go:308` `writeReconciliationResolution` | **Final write only.** Rename *replaces* a symlink where `os.WriteFile` *follows* it, so a mechanical whole-function swap silently converts symlinks to regular files. Symlink branch untouched. |
| `worktree.go:241` `os.RemoveAll` before `copyDir` | **Not fixed — cannot be.** A *directory*-level tear no per-file helper closes. Recorded prominently in the knowledge entry under "What this does NOT fix", so nobody reads "atomic writes: done" and assumes otherwise. |
| `steer.go:454` `appendSteeringEntry` | **Left on `O_APPEND`.** Converting an append to read-modify-rewrite is strictly worse — it widens the window to the whole file and adds a lost-update race. |
| `main.go:1443` `ensureGitignoreEntry` | **Left.** `.gitignore` is a git file, not Belmont state. |
| `fsutil.go:129/139` | **Converted** for consistency (cheapest possible). |
| install/install_sync/monorepo/copyEnvFiles/release/probe | **Left.** Not Belmont state. |

All four non-negotiable constraints implemented and pinned by tests: sibling temp via `os.CreateTemp(filepath.Dir(path), …)`, `Chmod` to the caller's perm before rename, `Sync` before rename, and no directory fsync (with the reason stated — this promises reader visibility, not crash durability).

The five sites that discarded the write error now warn on stderr: a swallowed *rename* failure is worse than a swallowed truncate failure, because it leaves an orphan `.belmont-tmp-*` inside a feature directory that the walkers read as state.

**The race proof is real, and was validated with a negative control.** `TestWriteStateFileIsNeverObservedPartiallyWritten` passes under `-race`, stable across 5 runs. A throwaway control using the *old* `os.WriteFile` in the identical harness observed a **0-byte read on every one of 3 runs within 0.02s**. The control was removed after validating; the result is recorded in the knowledge entry so nobody weakens the test's payload sizes without knowing what it is calibrated against.

**`go test -race` was clean on the whole suite both before and after** — warning 6's predicted `worktreeTracker` races did not appear.

---

### Task: P0-2 — Token and wall-clock instrumentation

**Status**: SUCCESS · **Commit**: `25249853`

**Files Created**: `cmd/belmont/metrics.go`, `cmd/belmont/metrics_test.go` (15 cases).
**Files Modified**: `render.go` (`streamLine` +`Subtype`/`Usage`, `claudeStreamWriter.usage`, new `usageCapture`), `auto_loop.go` (both exec sites + `attachUsageCapture` + `recordPhaseMetrics` + run identity), `types.go` (`loopConfig.RunID`, `.CriticalPath`), `fsutil.go` + `.gitignore` (`.belmont/metrics/`), `main.go` (`belmont metrics`).

**The `tailWriter` blocker was settled first, as instructed.** Non-Claude tools write into a `tailWriter` keeping only the last 1500 bytes, so `executionResult.Output` is the *tail* of a stream. Whether a usage event lands inside it is luck: codex emits `turn.completed` last, but the `item.completed` events before it grow with agent output, so the usage line drifts out of a fixed window on exactly the long runs whose cost matters most. **Resolution**: a `usageCapture` tee that retains the usage-bearing line by *content* rather than position. Rejected raising the tail size — a bigger window is still a window, and it would also change error-reporting semantics.

**Schemas were verified empirically against live runs, not guessed:**
- **claude 2.1.234** — `result` event → `usage.{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}`. `render.go:270` discarded every non-`assistant` line, which is exactly where it was being thrown away.
- **codex** — `turn.completed` → `usage`, with **different field names**: `cached_input_tokens` is the read, `cache_write_input_tokens` the creation. Swapping them yields a plausible wrong number.

**Deviation from the TECH_PLAN's usage table, made deliberately.** The table lists gemini and cursor as "Yes". **gemini could not be verified** (`IneligibleTierError` on this account, gemini-cli 0.46.0) and cursor is not installed. Both record `null` with the note *"usage schema not yet verified against a live run — not estimated"*. A parser written against a guessed schema fails *silently* — always-zero or always-nil — and would contaminate the M1 baseline P3-3 is judged against. The plan file is spec-not-deliverable per the scope boundary, so this is recorded here and in `NOTES.md` rather than edited into it. **Follow-up: verify both against a live run and switch them on.**

copilot / pi / opencode record `null` with a stated reason, per the plan. Token fields are pointers so a *reported* zero stays distinguishable from "not reported". **Nothing estimates a token count.**

**Critical-path attribution is a new longest-chain pass**, not the wave index — `computeWaves` is Kahn levelling, so a milestone sits in wave 2 whenever any dependency sits in wave 1, whether or not it lies on a longest chain. `TestCriticalPathIsNotTheWaveIndex` pins this and also asserts the fixture still demonstrates the confusion.

Wall-clock needed no new measurement, only persistence, exactly as the scan said.

**Acceptance verified functionally**: two consecutive runs on one feature produce comparable per-run records (checked via `belmont metrics`), and `.belmont/metrics/` is gitignored with no working-tree change (checked via `git check-ignore` and `git status`).

---

### Task: P0-3 — Capture the pre-change baseline

**Status**: PARTIAL — read-path half captured in full; cost half deliberately not fabricated · **Commit**: `c952426c`

**Files Created**: `.belmont/features/throughput/BASELINE.md`, `baseline.json`. Both committed — metrics are gitignored, but a baseline M11 must read is not a metric.

**Captured**, measured with the pinned v0.11.0 binary: per-feature register bytes for all 43 live registers in repo-3 and repo-4; `status --feature` text cost and `--format json` cost per feature; distribution (total/median/p90/max); the registers over the 100 KB ceiling; and the Belmont version, repo commit and Go toolchain.

**The PRD's cited figures reproduce exactly** — `repo-4/feat-075` measures 1,860,979 B raw, 57,145 B text, 682,148 B JSON, the same numbers its composition analysis and 12× JSON warning rest on.

**Not captured, and no number invented**: tokens and wall-clock per verified milestone. Three verified facts make this unavoidable — the orchestrating binary is pinned to v0.11.0 and carries no instrumentation (P0-2's code exists only in this tree); neither `~/repo-3/.belmont/metrics/` nor `~/repo-4/.belmont/metrics/` exists; and capturing it means running real milestones to `[v]` in two production repositories with an instrumented build, which is live agent spend an implementation agent should not trigger unasked. Tracked as `[!] P0-3a`, with the exact commands in `BASELINE.md`.

The PRD is explicit that instrumentation reports nothing rather than guessing. A baseline is the last place to break that, because every later claim is stated against it.

---

### Task: P0-4 — Extraction census across all feature directories

**Status**: SUCCESS · **Commit**: `a42c45ea`

**Files Created**: `cmd/belmont/extract.go` (census only), `extract_test.go` (9 cases), `.belmont/features/throughput/CENSUS.md`, `census.json`.

**Census only.** `extract` refuses without `--dry-run`, naming M3/P1-1 as where the write path and its round-trip proof land. `TestCensusWritesNothing` asserts no `details/` directory is created.

**Measured denominator, stated**: **138 feature directories, 65 live registers, 68 archived.** Matches the scan; does not match the PRD's "the other 83". Walker reads `<root>/.belmont/features/` directly and never globs.

> **WITHDRAWN by `P0-M1-FIX-2` (2026-08-18).** This walk substituted the Belmont fork for `repo-2` and so covered four of the PRD's five repos. Corrected: **168 directories / 82 live registers / 81 archived**, detail removed estate-wide **1,749,693 B (32.6%)**, against 138/65/68 and 34.8% below. The PRD's denominator reproduces exactly and the contradiction of it is retracted — see `CENSUS.md` §Correction. **The answer to the open question is unchanged**: the same seven registers exceed the ceiling today and the same five remain over it after extraction, with and without `repo-2`.

**The open question is answered, and the answer is YES — five, not three.**

| | |
|---|---|
| Over 100 KB today | 7 |
| **Still over after extraction** | **5** |
| Detail removed estate-wide | 1,744,231 B (**34.8%**) |

`repo-3/feat-015`, `repo-3/feat-058`, `repo-3/feat-070`, `repo-4/feat-031`, `repo-4/feat-075`.

**The pre-estimate was wrong in mechanism, not just magnitude.** It assumed size implies indented narrative and applied the worst file's 62% ratio to every file. **Three of the five contain no indented lines at all.** `repo-3/feat-015`: of 1,022,749 bytes, **795,799 are task *head* lines** and only 83,500 are indented, with a single task line of **11,542 characters**. Extraction moves 7.6% of it.

> **CORRECTED by `P0-M1-FIX-6` (2026-08-18).** *"Three of the five contain no indented lines at all"* is withdrawn: only **two** are (`repo-4/feat-031` 0, `repo-3/feat-058` 0). `repo-3/feat-015` has **167 indented lines / 84,593 B** — which is why the sentence contradicted itself in the next clause. The claim the measurement supports, and which the escalation in `[!] P0-4a` actually rests on, is that **three of the five gain essentially nothing from extraction (0%, 0%, 7.6%)**; that stands, and no total in this log changes. The two byte figures in the sentence above are withdrawn with it: re-derived from the register on disk, `feat-015`'s 335 task head lines are **805,397 B** and its indented lines **84,593 B**. All five counts were re-measured rather than restated — canonical copy in `CENSUS.md` §Why the estimate was wrong.

Extraction remains right — 34.8% estate-wide, and 62.9% of the worst file (1,860,979 → 691,323 B, reproducing the PRD's composition analysis) — but does not alone bring every register under the ceiling.

**Per the PRD's own terms, M4 becomes a prerequisite of M3 and the wave structure changes.** Only tech-planning may restructure milestones, so this is escalated as `[!] P0-4a` rather than applied.

Reuses `parseMilestones` + `taskBodyEnd` rather than a third line-scanner, and records which *line indices* move rather than summing per-task lengths — a nested task bullet lies inside its parent's body *and* is a task itself, so the naive sum double-counts.

---

### Out-of-Scope Issues Found (across all tasks)

| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-2 | gemini and cursor usage schemas unverified; both record `null`. Verify against a live run and switch on. The TECH_PLAN's usage table still lists them as "Yes". | P1 |
| FWLUP-2 | P0-13 | `origin/fix/post-51-triage` merges with zero conflicts and touches `reconcile.go`/`worktree.go`, which P0-1 rewrote. Merge it before M3 or pay for it later. | P1 |
| FWLUP-3 | P0-4 | M5/P1-8's 400-char task-line limit is load-bearing, not defensive — it is the only mechanism that would have prevented `feat-015`. Consider raising its priority. | P1 |
| FWLUP-4 | P0-4 | P3-1's migration list is **seven** features, not the four the PRD assumes — and for three, migration alone is insufficient. | P2 |
| FWLUP-5 | P0-1 | `os.RemoveAll(dstFeature)` before `copyDir` (`worktree.go:241`) still tears at directory level. Closing it means staging into a sibling directory and renaming. | P2 |
| FWLUP-6 | P0-12 | `go test -race` is still not in `ci.yml`, though it is now clean and P0-1's proof depends on it. | P2 |
| FWLUP-7 | P0-12 | `go build -tags embed` fails from a clean checkout (pre-existing). `scripts/build.sh` stages the embed inputs; a bare `-tags embed` build is a trap. | P3 |

### Notes for Verification

- **Three `[!]` tasks are deliberate escalations, not failures**: P0-13a (publishing 157 commits — owner's decision), P0-3a (live agent runs in two production repos — owner's decision), P0-4a (wave restructure — tech-planning's decision). Each names what is being asked and of whom.
- **P0-4a is the one with downstream consequences.** It changes M3/M4 ordering if accepted, and it should be resolved before M3 begins.
- **`staticcheck` must be rebuilt** with a toolchain ≥ the module's `go` directive or it errors locally; CI is unaffected. Recorded in `NOTES.md`.
- **The race proof is calibrated.** Do not reduce `TestWriteStateFileIsNeverObservedPartiallyWritten`'s payload sizes or iteration count without re-running a negative control — a test that cannot fail proves nothing.
- **Metrics fixtures are real captured tool output**, not invented JSON. If claude or codex change their event shape, those constants are what fail — that is intended.
- Nothing was pushed; no remote branch was deleted, merged or rebased; `belmont install` was not run; no external Go dependency was added and there is still no `go.sum`.


---

## Task P0-M1-FIX-1 — /belmont:next batch round 1 (2026-08-18)

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

---

## Task P0-M1-FIX-2 — /belmont:next batch round 2 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `962f0e84e290cde5a5282f7e4cbe72a5942d606d`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 2 of 7.
- **Created**: 2026-08-18T13:30:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-2: The census never walked `repo-2`, and used the incomplete walk to "correct" the PRD

## Orchestrator Context

### Current Task
`P0-M1-FIX-2` — the only task being implemented in this run. It is the Critical finding from M1's verification pass.

### Active Task IDs
`P0-M1-FIX-2`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-4 holds the parent task's acceptance criteria, and §Success Criteria carries the "139 existing feature registers (82 active + 57 archived)" figure this task must reconcile against
- **TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/TECH_PLAN.md` — §Command Specifications (`belmont extract`)
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` — Pass 1 holds the full M1 codebase scan and the census as originally built
- **The artefacts to correct**: `CENSUS.md` and `census.json` in this directory
- **The code**: `cmd/belmont/extract.go` (`runCensus`)

### The defect

P0-4's acceptance criterion is *"a dry-run report over every feature directory **in all five repos**"*. The PRD names them: repo-1, repo-5, repo-4, **repo-2**, repo-3.

The census walked `repo-3, repo-4, repo-5, repo-1, repos/belmont` — it substituted the Belmont fork for `repo-2`, which was **never measured**. It exists at `/Users/benlavender/repo-2/.belmont/` with roughly 31 feature directories and 18 further live registers.

The second half is worse than the omission. `CENSUS.md` uses the incomplete walk to declare the PRD wrong:

> *"The PRD's phrase 'the other 83' does not match disk. The real split is 138/65/68."*

But the PRD's own figure — 82 active — **reproduces exactly** once `repo-2` is included. The PRD was right; the correction is the error, and it currently sits in a document whose stated purpose is to be the authoritative measurement.

### Required fix

1. **Re-run the census over the PRD's exact five repos**, including `/Users/benlavender/repo-2`. Confirm the repo path first — do not assume it. Decide, and state, whether the Belmont fork itself is a sixth root or is excluded; either is defensible, but the PRD's five must all be present.
2. **Update `CENSUS.md` and `census.json`** with the corrected totals — expected around 168 dirs / 82 live / 32.6% estate reduction against 138 / 65 / 34.8%. **Measure them; do not copy these numbers from this file.**
3. **Retract the claim that the PRD's denominator does not match disk.** Replace it with the reconciliation: the PRD's 82 active reproduces once all five repos are walked. Say plainly that the earlier claim was wrong and why, rather than quietly deleting it — a census's value is that its corrections are auditable.
4. **State the denominator and its scope explicitly** in `CENSUS.md`, including whether archived directories are counted.
5. Re-check whether the **five-over-threshold conclusion changes**. Verification determined it does not — the same five registers exceed the ceiling with or without `repo-2` — but confirm it rather than inherit it, because `[!] P0-4a`'s wave-restructuring escalation rests on that number.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-2`.
- **Out of Scope**: everything in `{base}/PRD.md` §Out of Scope. Do **not** touch the other follow-ups — `P0-M1-FIX-3` … `P0-M1-FIX-7` each get their own run. In particular **`P0-M1-FIX-7` owns** the broken `§Reproducing this` command and `runCensus`'s silent skip of an unreadable root, and **`P0-M1-FIX-6` owns** the "three of the five contain no indented lines" correction. Leave both alone even though they live in the same file — if your re-run makes one of them trivially fixable, say so in the log and leave the task standing.
- Do **not** start M2–M11 work. `extract` stays census-only: `--dry-run` remains mandatory and nothing may write a detail tier.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner. `P0-4a`'s body quotes census figures; if the corrected numbers change what it says, report that in your log and leave the task itself untouched.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase or delete any remote branch. **Do not modify any of the five audited repositories** — this is a read-only measurement over them.
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
Plus: the corrected census must be reproducible by a command you have actually run, and the five audited repos must be unmodified afterwards (`git status` clean in each, where each is a git repo).

### Learnings from Previous Sessions

#### Feature Notes
Two entries in `NOTES.md` §Root Cause Patterns bear directly on this task:

- ***"a census derived its own scope instead of taking the spec's enumeration"*** — the scope was assembled from what was to hand rather than from the PRD's explicit list, and `runCensus` swallows an unreadable root, so an incomplete walk is indistinguishable from a complete one. **Prevention rule, and this task's bar: when a spec enumerates its subjects, use that enumeration verbatim and fail loudly on any member that cannot be read.**
- ***"a measurement was used to correct the spec before it was known to be complete"*** — this is the same numeric-drift pattern already recorded in the sibling `framework-evaluation` feature. **State a measured figure once, in one file, and have every other document link to it.** Before contradicting a spec's number, prove your measurement covers the spec's full scope.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and the required fix are specified above; the census as originally built is in `MILESTONE-M1.done.md` Pass 1. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-2 — The census never walked `repo-2`, and used the incomplete walk to "correct" the PRD

**Status**: SUCCESS

**Files Modified**:
| File | Changes |
|------|---------|
| `.belmont/features/throughput/CENSUS.md` | Re-measured over the PRD's five repos; new §Correction retracting the withdrawn claim; denominator and counting rules stated explicitly; five-over-threshold invariance stated as measured |
| `.belmont/features/throughput/census.json` | Regenerated from the five-repo run (82 registers, was 65) |
| `.belmont/features/throughput/NOTES.md` | §Discovery denominator bullet corrected and pointed at CENSUS.md as the single canonical copy; 34.8% → 32.6% |
| `.belmont/features/throughput/MILESTONE-M1.done.md` | Two `WITHDRAWN by P0-M1-FIX-2` annotations against the passages that assert 138/65/68 as fact (history annotated, not rewritten) |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-2` → `[x]` |

**No code changed.** The defect is in the measurement's scope and in the document, not in
`censusFeature`. `runCensus`'s silent skip of an unreadable root is the mechanism that hid the
omission, but it is `P0-M1-FIX-7`'s to fix and was left alone.

**Tests Added**: none — no code changed. The measurement is verified by two independent walks
(the Go census and a `find`-based shell count) agreeing exactly on 168 / 82 / 81 / 5.

#### What was measured

Roots, confirmed on disk before the run, all five present:

| Repo | Root | Feature dirs | Live registers |
|---|---|---:|---:|
| repo-1 | `/Users/benlavender/repo-1` | 11 | 10 |
| repo-5 | `/Users/benlavender/repo-5` | 70 | 11 |
| repo-4 | `/Users/benlavender/repo-4` | 17 | 14 |
| repo-2 | `/Users/benlavender/repo-2` | 31 | 18 |
| repo-3 | `/Users/benlavender/repo-3` | 39 | 29 |
| **Total** | | **168** | **82** |

**The Belmont fork is excluded, deliberately and stated in `CENSUS.md`.** It is the tool repo, not
one of the five deployments, and its one feature directory is this feature's own register — the
census would be measuring the document that records it. Including it gives 169 dirs / 83 live.

Measured totals: **5,372,973 B today → 3,623,280 B after extraction; 1,749,693 B moved = 32.6%**.
Distribution: median 16,196 → 16,075 B, p90 71,379 B unchanged, max 1,860,979 → 944,773 B.

These figures were derived here, not copied from this MILESTONE file. They agree with the
reviewer's spot-check (168 / 82 / 32.6%) to the digit; the byte totals the spot-check did not
quote are new.

Why the reduction rate fell from the withdrawn 34.8% to 32.6%: `repo-2`'s 18 registers add
386,620 B of which only 7,519 B (1.9%) is indented detail. Estate-wide detail barely moved
(1,744,231 → 1,749,693 B); the denominator grew.

#### The retraction, and the reconciliation behind it

`CENSUS.md`'s claim — *"The PRD's phrase 'the other 83' does not match disk. The real split is
138/65/68"* — is quoted verbatim under a **§Correction** heading, marked withdrawn, with the cause
named (a walk that substituted the Belmont fork for `repo-2`, and a `runCensus` that
returns no error for a root it cannot read). Nothing was silently deleted.

The reconciliation is stronger than the task anticipated: **both halves of the PRD's figure
reproduce exactly**, not just the active count.

| PRD Success Criteria | Measured | Where |
|---|---:|---|
| 82 active | **82** | live `PROGRESS.md` across the five repos |
| 57 archived | **57** | registers under `repo-1/.belmont/features/_archived/` |
| 139 registers | **139** | 82 + 57 |

The 57 were found by inspection, not assumed: `repo-1/.belmont/features/_archived/` holds 93
subdirectories, exactly 57 of which carry a `PROGRESS.md`, and no other repo has any register
nested below feature level. That population is **different from** the 81 directories this census
counts as archived — those had their registers summarised into an `ARCHIVE.md` and deleted by
`/belmont:cleanup`. Both counts are right about different things; the withdrawn "68 archived" was
right about neither.

One residual, stated in `CENSUS.md` so it is not later mistaken for a fresh defect: with 82 live
registers of which two were already measured, §P0-4's *"the other 83"* is strictly "the other 80".
That is a wording imprecision of three, not the denominator error the census asserted, and it is
not a reason to touch the PRD — whose Success Criteria figure is exact. **No PRD edit was made.**

#### The five-over-threshold conclusion — confirmed, not inherited

Run three ways rather than reasoned about:

| Walk | Over 100 KB today | Still over after extraction |
|---|---:|---:|
| Five repos | 7 | **5** |
| Four repos (`repo-2` excluded) | 7 | **5** |
| `repo-2` alone | 0 | **0** |

Both lists are element-for-element identical between the five- and four-repo walks.
`repo-2`'s largest register is 86,380 B — below the ceiling before extraction, therefore
below it after. **`[!] P0-4a`'s escalation rests on a number the omission did not touch**, and its
body quotes nothing that this correction changes ("Five do", the "three of the five" wording which
belongs to `P0-M1-FIX-6`, and "seven, not four" — the complete walk adds no eighth). `P0-4a` was
not touched, nor were `[!] P0-3a` or `[!] P0-13a`.

The per-feature numbers for the five are byte-identical to the first run, since all five live in
repos the first walk did cover.

#### Read-only over the five repos — verified

`git status --porcelain` and `git rev-parse HEAD` were captured for each of the five before the
census and diffed after. All five byte-identical, including the repos that were already dirty
(`repo-2` had 85 pre-existing modified files, `repo-5` 3, `repo-1` and `repo-3` 1
each, `repo-4` clean — unchanged in every case). Nothing was written to any of them.

#### Adjacent tasks left standing

- **`P0-M1-FIX-7`** — §Reproducing this now uses absolute unabbreviated paths, because the command
  had to be re-written for the five-repo root list and had to be one actually run. That
  incidentally removes the tilde-expansion half of FIX-7's symptom in this one code block. **The
  task still stands**: its substantive half — `runCensus` swallowing an unreadable root with
  `os.IsNotExist → continue`, which is *how* the omission stayed invisible — is untouched, and
  `CENSUS.md` now says so explicitly in that section rather than implying the doc is fixed.
- **`P0-M1-FIX-6`** — the "three of the five contain no indented lines at all" sentence was left
  verbatim in `CENSUS.md` and `NOTES.md` although both were edited around it. Re-measurement did
  not make it correct, so nothing about it became trivially fixable.
  **Closed by `P0-M1-FIX-6` (2026-08-18).** Corrected in `CENSUS.md` (canonical), and in
  `NOTES.md` and `PROGRESS.md` by reference to it. This log was annotated above, not rewritten.

#### Root Cause Patterns acknowledged (Step 0b)

- *"a census derived its own scope instead of taking the spec's enumeration"* — the five roots were
  taken verbatim from PRD §Rollout, each confirmed to exist and to hold `.belmont/features/`
  **before** the run, with the presence check printed rather than assumed. The exclusion of the
  Belmont fork is a stated decision in `CENSUS.md`, not a silent one.
- *"a measurement was used to correct the spec before it was known to be complete"* — no spec
  figure is contradicted here. The measurement now matches the PRD, and the figure is stated once:
  `CENSUS.md` §The denominator, stated is canonical, `NOTES.md` points at it instead of restating
  it, and the two stale copies in the M1 done-log are annotated as withdrawn with a pointer.

**Verification Results**:
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass
- `go test -tags eval ./cmd/belmont`: pass
- `go vet ./...`: pass (no output)
- `staticcheck ./...`: pass (no output; run from `$(go env GOPATH)/bin/staticcheck`, 2026.1 — not on `PATH`)
- `gofmt -l cmd/`: pass (no output)
- `belmont validate --root /Users/benlavender/repos/belmont`: pass — no milestone-structure violations
- Census reproducible by the exact command in `CENSUS.md` §Reproducing this: yes — it is the command that produced `census.json`
- Five audited repos unmodified: yes — porcelain + HEAD identical before and after

**Self-Validation**:
- Acceptance Criteria: 5/5 of the MILESTONE's Required fix items (five-repo re-run; CENSUS.md and
  census.json updated; claim retracted rather than deleted; denominator and its scope stated;
  five-over-threshold re-confirmed by direct measurement)
- P0-4's own criterion — *"a dry-run report over every feature directory in all five repos"* — now
  met: all five walked, 168 directories, nothing written
- Visual Check: N/A (no visual output)

**Commit**:
- **Hash**: the single commit on `main` whose subject is the message below (recorded by subject rather than hash — the log lives inside the commit it would name)
- **Message**: `P0-M1-FIX-2: Re-run the census over all five repos and retract its correction to the PRD`

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-2 | The census counts `_archived/` container directories as feature directories. `repo-1` and `repo-3` each have one, so 2 of the 168 are not features, and the 57 intact registers inside `repo-1/_archived/` are invisible to every byte figure. Harmless to P0-4's answer (they are held out of the live estate deliberately) but the walker should either skip `_`-prefixed directories or the census should say what it does with them. Documented as a counting rule in `CENSUS.md` for now. | P3 |
| FWLUP-2 | P0-M1-FIX-2 | `repo-4` has three feature directories with neither `PROGRESS.md` nor `ARCHIVE.md` (`frontend-performance-overhaul`, `provider-services-parity`, `test-agent-booking-controls`) — design/README material only. They inflate the directory denominator without being registers. Worth a `belmont validate` check rather than a census one. | P3 |

### Notes for Verification
- The corrected figures are `168 / 82 / 81 archived / 5 with neither` and `32.6%`. They were
  produced twice by independent means (Go census, `find`-based shell count) and agree exactly.
  Re-derive rather than trust: the command is in `CENSUS.md` §Reproducing this.
- The exclusion of the Belmont fork is a judgement call, stated openly in `CENSUS.md` with the
  alternative's figures (169 / 83). If a reviewer prefers it included, only the directory and
  register counts change — no conclusion does.
- **The 57 archived registers under `repo-1/_archived/` are the load-bearing discovery.** They
  are what makes the PRD's "139 registers" exact rather than approximately right, and they are not
  reachable by the census walker, which is one level deep by design.
- `MILESTONE-M1.done.md` was annotated, not rewritten — two `> **WITHDRAWN by P0-M1-FIX-2**`
  block quotes beneath the passages that state 138/65/68 as fact. If the convention is that done
  logs are immutable, those two annotations are the only thing to revert; the corrections in
  `CENSUS.md` and `NOTES.md` stand on their own.
- `P0-M1-FIX-6` and `P0-M1-FIX-7` were deliberately left open. See §Adjacent tasks left standing
  for exactly what was and was not touched in the files they own.


---

## Task P0-M1-FIX-3 — /belmont:next batch round 3 (2026-08-18)

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

---

## Task P0-M1-FIX-4 — /belmont:next batch round 4 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `2e7d6f43`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 4 of 7.
- **Created**: 2026-08-18T14:20:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-4: `summariseMetrics` sums `Input` across tools whose `input_tokens` mean different things

## Orchestrator Context

### Current Task
`P0-M1-FIX-4` — the only task in this run. A Warning from M1's code review, in the file that produces the baseline every later milestone is scored against.

### Active Task IDs
`P0-M1-FIX-4`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-2 holds the parent task's acceptance criteria; §Clarifications carries the "instrumentation reports nothing rather than guessing" rule
- **TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/TECH_PLAN.md` — §File-Format Specifications (Metrics), §Go Implementation Notes (P0-2/P0-3)
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` — Pass 1 documents the metrics design as built; `## Task P0-M1-FIX-1` documents the `MetricsRoot` change to the same file
- **The code**: `cmd/belmont/metrics.go` (`summariseMetrics`, around lines 296–341) and `cmd/belmont/metrics_test.go`

### The defect

`summariseMetrics` adds `Input` across records from different tools. The field means different things depending on which tool produced it:

- **claude**: `input_tokens` **excludes** `cache_read_input_tokens` and `cache_creation_input_tokens`.
- **codex** (OpenAI lineage): `input_tokens` **includes** `cached_input_tokens` — and the parser in this same file already notes that codex's `cached_input_tokens` is "the read".

So a summary spanning both tools double-counts cached input for codex records relative to claude ones, and a P3-3 comparison whose two halves used different tools produces a plausible wrong number. That is precisely the failure the file's own "never estimate" discipline exists to prevent, arriving through a different door — nothing is estimated, but the arithmetic is still wrong.

Note what is **not** broken, and must not be "fixed": the per-tool field mappings are correct and are pinned by tests using real captured event JSON. Both reviewers checked specifically for transposed field names and found none. This task is about the **aggregation boundary**, not the parsers.

### Required fix

Either:
- **normalise on ingest** — store a defined, tool-independent quantity so records are comparable by construction; or
- **refuse to aggregate across tools** — break the summary out per tool and decline to produce a combined `Input` figure.

Pick one and say why in the code. Whichever you choose, record the per-tool semantics **as data rather than as a comment**, so the distinction survives to the aggregation boundary where it actually bites — a comment at the parse site is what failed here.

The code review asks that codex's inclusion semantics be confirmed against a live run rather than assumed. **If you cannot obtain a live codex run, do not guess.** Record the uncertainty explicitly and choose the option that is safe under either interpretation — refusing to aggregate is safe without knowing; normalising is not.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-4`.
- **Out of Scope**: `P0-M1-FIX-5`, `-6`, `-7` each get their own run — leave `docs/branch-triage.md`, the "three of the five" wording, and `runCensus`'s silent skip alone.
- Do **not** re-open `P0-M1-FIX-1`'s work in the same file (`MetricsRoot`, `worktreeLoopConfig`, the `appendMetricsRecord` concurrency contract). It is done and verified; leave it intact.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase or delete any remote branch.
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
Plus, specific to this task: a test that would **fail** under the old behaviour — mixed-tool records whose combined figure is wrong today. A test that passes both before and after proves nothing, which is exactly how the `MetricsRoot` defect survived its own serial-path test.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this defect's cause: ***"identically-named fields from different vendors normalised without recording their semantics."*** The per-tool parsers are individually correct; the divergence was recorded only in a comment at the parse site, not carried to the aggregation boundary.

**Prevention rule, and this task's bar**: when ingesting the same-named field from different vendors, record the semantic **at ingest as data**, and refuse to aggregate across sources whose semantics differ.

Two other entries bear on it:
- ***"never estimate"*** — token fields are `*int64` so a reported zero stays distinguishable from "not reported"; hosts that cannot report record `null` with a stated reason. Do not weaken that while editing this file.
- ***"a new state write was only ever exercised on one of two execution paths"*** — the general lesson being that a test which passes with and without the fix is not a test of the fix.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and required fix are specified above; the metrics design as built is in `MILESTONE-M1.done.md` Pass 1. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-4 — `summariseMetrics` sums `Input` across tools whose `input_tokens` mean different things

**Status**: SUCCESS

**Decision — refuse to aggregate, not normalise.** Both options were open. Normalising to a
tool-independent quantity (e.g. whole-prompt tokens) is correct only while every tool's semantics are
known *and stay known*: a tool added later, or a vendor redefining the field under us, silently
re-arms exactly the wrong-but-plausible baseline being fixed. Refusing is correct under every
interpretation, including unverified ones, and costs no information because the per-tool figures are
reported beside the null. The reasoning is in `metrics.go`'s `inputSemantics` doc comment, and the
file header now states the rule.

**The uncertainty the code review flagged was resolved, not assumed.** codex's inclusion semantics
were confirmed against a live run (codex-cli 0.147.0, 2026-08-18): a three-turn session reported
input/cached of 17308/4480 → 34647/21248 → 52003/38016. Those are session-cumulative, so per-turn
they are 17308/4480, 17339/16768, 17356/16768 — each turn re-sends the same ~17.3k context with the
cache read a subset of it. Under the excluding reading the context would have had to grow from 21,788
to 34,107 tokens across a 26-token exchange. claude's opposite semantics are confirmed from Anthropic's
own documentation (`input_tokens` is the uncached remainder; whole prompt = input + cache_creation +
cache_read). One thing the run did **not** settle is recorded as unresolved in the code: whether cache
*writes* also sit inside codex's `input_tokens` (`cache_write_input_tokens` was 0 on every turn). It
does not need settling — the rule keys off the definitions differing at all, not which part differs.

**Semantics are recorded as data, not as a comment.** `metricsRecord.InputSemantics`
(`"input_semantics"` in the JSONL) is written at ingest by `buildMetricsRecord` from the
`toolInputSemantics` table, so the distinction reaches `summariseMetrics` and any later reader of a
record already on disk — which is precisely what a comment at the parse site could not do.

**Files Modified**:
| File | Changes |
|---|---|
| `cmd/belmont/metrics.go` | New `inputSemantics` type, `toolInputSemantics` table and `inputSemanticsFor` (ingest lookup); `metricsRecord.InputSemantics` written by `buildMetricsRecord`; `summariseMetrics` rewritten around `inputAcc`/`inputAggKey` so `Input`/`CriticalInput` (now `*int64`) are reported only when one definition contributed, with `InputNote` and a new per-tool `ByTool`/`tools_detail` breakdown; `runAgg.Input` likewise `*int64`; `renderMetricsSummary` prints `n/a` (never `0`) plus the per-tool table; header comment states the rule |
| `cmd/belmont/metrics_test.go` | Three existing assertions updated for the pointer `Input`; four new tests |
| `.belmont/features/throughput/TECH_PLAN.md` | §Metrics file-format spec: `input_semantics` added to the record example, plus the aggregation rule |
| `.belmont/features/throughput/NOTES.md` | codex cumulative/inclusive discovery, the `codex exec resume` flag gotcha, and the refuse-don't-normalise pattern |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-4` → `[x]` |

**Tests Added**:
| Test | Coverage |
|---|---|
| `TestSummariseMetricsRefusesToMixInputSemantics` | Two phases of identical real cost (1,000-token prompt, 900 from cache) reported as claude `Input:100`/`CacheRead:900` and codex `Input:1000`/`CacheRead:900`. The old code summed these to **1100** — neither the uncached total (200) nor the prompt total (2000). Asserts null `input`/`critical_path_input`, a note naming both definitions, the per-tool rows, that Output and cache figures still aggregate, and that neither the JSON nor the text output contains `1100` |
| `TestUnknownInputSemanticsDoNotMergeAcrossTools` | Fail-closed half: one undeclared tool still aggregates with itself; two undeclared tools do not; unknown never folds into a known definition |
| `TestInputSemanticsRecordedAtIngestAndOnDisk` | The mechanism, not the outcome — the definition is written at ingest per tool and survives the JSONL round trip; a phase with no usage claims no definition |
| `TestInputSemanticsCoversEveryToolThatReportsUsage` | Every tool `toolReportsUsage` says yes to has a recorded definition, and vice versa (mirrors the existing note-table invariant) |

**Negative control (the test would fail under the old behaviour)**: a full revert makes the new tests
fail to compile, which proves little, so the fix was neutered in place instead — `inputAggKey` made to
return a constant, i.e. "aggregate regardless of semantics", which is exactly the old arithmetic on
the new API. `TestSummariseMetricsRefusesToMixInputSemantics` then failed with
`combined input: got 1100, want null`, the JSON assertion showed `"input":1100`, and
`TestUnknownInputSemanticsDoNotMergeAcrossTools` failed on both cross-tool cases. Restored, all green.

**Verification Results** (every gate in this file's verification block):
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass (25.2s)
- `go test -tags eval ./cmd/belmont`: pass (23.2s)
- `go vet ./...`: clean
- `staticcheck ./...`: clean
- `gofmt -l cmd/`: no output
- `belmont validate --root /Users/benlavender/repos/belmont`: ✓ no violations
- Zero external Go dependencies; no `go.sum`; no `.belmont/metrics/` written into the tree (tests use `t.TempDir()`)

**Self-Validation**:
- Acceptance Criteria: 4/4 — combined `Input` no longer crosses definitions; the semantics are data at
  ingest rather than a comment; the choice is safe under either reading of codex (and the reading was
  confirmed anyway); a test that fails under the old behaviour exists and was demonstrated failing.
- Visual Check: N/A (CLI output only; the text renderer change is covered by assertion)

**Scope**: `P0-M1-FIX-1`'s work in the same file (`MetricsRoot`, `worktreeLoopConfig`, the
`appendMetricsRecord` concurrency contract) is untouched — its three tests still pass unchanged.
`P0-M1-FIX-5/-6/-7`, the `[!]` human-gated tasks, `docs/branch-triage.md`, the "three of the five"
wording and `runCensus` were not touched. No `belmont install`, no push/merge/rebase/branch deletion.

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-4 | `Output` has the same shape of question and was deliberately left alone: codex reports `reasoning_output_tokens` as a separate field, and the live probe suggests `output_tokens` already includes it (turn 1: output 20, reasoning 13, visible reply ~7), but that was not the task and is not pinned by a test. If M11 ever compares output tokens across tools, verify and record it the same way `input_semantics` now is. | P3 |
| FWLUP-2 | P0-M1-FIX-4 | codex's `turn.completed` usage is session-cumulative (verified above). This happens to be what Belmont wants — `usageCapture` keeps the last usage-bearing line — but it is undocumented vendor behaviour that nothing pins. A per-turn regression upstream would silently under-report a multi-turn phase. | P3 |

### Notes for Verification
- The load-bearing behaviour is the **refusal**, not the breakdown: `s.Input == nil` plus `InputNote`
  whenever two definitions contribute. `tools_detail` exists so the refusal loses no information.
- `metricsSummary.Input`, `metricsSummary.CriticalInput` and `runAgg.Input` are now `*int64`. This is
  a JSON-shape change for `belmont metrics --format json` (`"input": null` is newly possible, and
  `input_note` / `tools_detail` are new keys). Nothing in the tree consumed those fields —
  `.belmont/metrics/` does not exist in this repo or in repo-3/repo-4, and `baseline.json` records
  byte counts, not metrics-summary JSON — so no captured baseline is invalidated.
- A record written before this change carries no `input_semantics` and is therefore treated as
  `unknown`, which refuses to merge with a declared definition. That is deliberate (fail-closed): no
  such records exist anywhere yet, and inferring the definition from the tool name at read time would
  re-introduce the assumption this task removes.

---

## Task P0-M1-FIX-5 — /belmont:next batch round 5 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `ad3559de`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 5 of 7.
- **Created**: 2026-08-18T14:45:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-5: The `origin/fix/wave-merge-state-loss` abandon reason states a mechanism `main` does not have

## Orchestrator Context

### Current Task
`P0-M1-FIX-5` — the only task in this run. A Warning from M1's verification pass: the verdict is defensible, the stated reason is factually wrong.

### Active Task IDs
`P0-M1-FIX-5`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-13 holds the parent task's acceptance criteria
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Prior implementation log**: `MILESTONE-M1.done.md` — Pass 1 holds the original triage; `## Task P0-M1-FIX-3` records the rewrite of a *different* row in the same file, the evidence bar it set, and a measurement trap it discovered
- **The artefact to correct**: `docs/branch-triage.md`, the `origin/fix/wave-merge-state-loss` row (was around line 101 — `P0-M1-FIX-3` has since edited this file, so locate the row rather than trusting the line number)
- **The code the reason must describe**: `cmd/belmont/worktree.go` — `syncFeatureStateAfterMerge` and `mergeProgressState`

### The defect

`docs/branch-triage.md` abandons `origin/fix/wave-merge-state-loss` on the stated ground that *"`main`'s `worktree.go` already scopes the sync per milestone"*.

`main` does not do that. `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` takes **no milestoneID**, still performs `os.RemoveAll(dstFeature)` followed by `copyDir`, and there is no `resolveMilestoneProgress`.

The defect class **is** addressed on `main`, but by a different mechanism: master's `PROGRESS.md` is read *before* the wipe and then union-merged via `mergeProgressState` (`worktree.go:504`, called from `:410` — confirm these line numbers rather than trusting them, the file has changed this session). Separately, "refuses duplicate headings" is true, at `worktree.go:560/588`.

The row also states that *"only the branch's own test filename is absent"*, which understates a 251-line test file and an entire alternative implementation.

**The verdict itself is defensible — do not flip it.** This task corrects the reasoning, not the outcome.

### Required fix

1. Rewrite the `origin/fix/wave-merge-state-loss` reason to name the mechanism `main` **actually** uses, citing the lines that implement it. Verify those line numbers against the current file; do not copy them from this brief.
2. Correct the "only the branch's own test filename is absent" characterisation to reflect what the branch actually carries.
3. Keep the verdict as **abandon**, and make the evidence support it — or, if the evidence genuinely does not support abandon once you have read both sides, say so explicitly in your log rather than quietly changing the verdict.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-5`.
- **Out of Scope**: `P0-M1-FIX-3` already rewrote the `origin/feat/maintenance-ci` row, the headline bullet, the abandon-section preamble and the measurement-traps section in this same file. **Leave all of that alone** — read it for the evidence bar it sets, do not redo it.
- `P0-M1-FIX-6` (the "three of the five" wording) and `P0-M1-FIX-7` (`runCensus`'s silent skip) each get their own run.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase, cherry-pick or delete any remote branch. This task is a documentation correction — no git history changes.
- Do **not** start M2–M11 work. Do not change `worktree.go`; you are describing it, not fixing it.
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
Plus, specific to this task: every line reference you write must resolve to what you claim it does, checked by reading it at HEAD.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this defect's cause: ***"triage verdicts written from summary evidence rather than from content"*** — a mechanism that sounds plausible is not evidence that it exists.

**Prevention rule, and this task's bar**: an "abandon" reason must name the superseding mechanism on `main` and **cite the lines that implement it**. A destructive recommendation carries a higher evidence bar than a keep.

`P0-M1-FIX-3` applied that bar to the neighbouring row and, in doing so, found a trap worth knowing before you touch this file: **`git cherry` and three-dot diffs cannot attribute a change to a specific unlanded commit.** `git diff main...<branch>` shows every change the branch made since the merge-base, including ones `main` has independently. Use `git show <sha>` to learn what a specific commit contains.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and required fix are specified above; the original triage is in `MILESTONE-M1.done.md` Pass 1, and `## Task P0-M1-FIX-3` shows the standard the corrected rows are held to. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-5 — The `origin/fix/wave-merge-state-loss` abandon reason states a mechanism `main` does not have

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `docs/branch-triage.md` | Line 115 — the `origin/fix/wave-merge-state-loss` row's Reason cell rewritten in full. |
| `.belmont/features/throughput/NOTES.md` | Two entries: the non-destructive `git merge-tree` probe, and the residual non-`PROGRESS.md` last-writer-wins in `syncFeatureStateAfterMerge`. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-5` marked `[x]`. |

**Tests Added**: none — documentation-only correction. `cmd/belmont/worktree.go` was **not** touched, per the scope boundary.

#### What the old reason claimed, and what is actually true

The row read: *"`main`'s `worktree.go` already scopes the sync per milestone and refuses duplicate headings; `syncFeatureStateAfterMerge` is covered by three test files … Only the branch's own test filename is absent."*

Verified against HEAD (`ad3559de`), every line number read rather than copied from the brief:

- **"scopes the sync per milestone" — false.** `syncFeatureStateAfterMerge(mainRoot, wtPath, slug string)` at `cmd/belmont/worktree.go:370` takes no milestone ID; `os.RemoveAll(dstFeature)` is at `:397` and `copyDir` at `:398`. `grep -rn "resolveMilestoneProgress\|milestoneBlockRange" cmd/` returns nothing — neither symbol exists on `main`.
- **The defect class *is* handled, by a different mechanism.** Master's `PROGRESS.md` is read **before** the wipe (`:395`) and union-merged back afterwards by `mergeProgressState` — defined at `:504`, called at `:410`. The brief's `:504`/`:410` both hold at HEAD.
- **"refuses duplicate headings" — true, but of `mergeProgressState`, not of milestone scoping.** Duplicate `### M<n>:` headings are counted at `:569-585` and warned at `:586-589`; duplicate task IDs get the same refusal, counted `:530-558`, warned `:618-621`. The brief's `:560/588` is the same construct: `:560` is the first line of its explanatory comment, `:588` the warning string. I cited the code range rather than the comment.
- **"only the branch's own test filename is absent" — a large understatement.** `git show --stat 1fcbc7d4` (the branch's single unlanded commit, `git cherry -v main` = 1 outstanding): 418 insertions across 7 files, including a **251-line** `cmd/belmont/sync_feature_state_test.go` carrying **11 tests**, and a complete alternative implementation — a `milestoneID` parameter on `syncFeatureStateAfterMerge`, `resolveMilestoneProgress` + `milestoneBlockRange` (a line-exact splice of the worktree's own milestone block onto master's file), and both `auto_parallel.go` call sites updated.

Per the trap `P0-M1-FIX-3` recorded, the commit's content was read with `git show 1fcbc7d4`, never inferred from `git diff main...<branch>`.

#### Verdict: abandon stands, and is now supported

Kept as **abandon**. Reading both sides strengthened it rather than changing it:

- `main`'s per-task-ID union merge is **stricter** than the branch's per-milestone-block splice: most-advanced-state-wins by `markerRank` (`state.go:260`), `[!]` wins in both directions, unrecognised markers never ranked, master-only tasks carried into their milestone.
- `main` covers the branch's own scenario: `TestSyncPreservesSiblingCompletions` in `worktree_sync_test.go` (15 tests) is the two-sibling clobber the branch commit describes; `merge_symmetry_test.go` and `worktree_state_test.go` also exercise the path. All three named test files exist on `main`; only `sync_feature_state_test.go` does not.
- `main`'s `knowledge/auto-mode/worktree-state-isolation.md:44-58` (§*PROGRESS.md is merged on sync, never replaced*) is a superset of the branch's 28-line edit to the same file.
- The branch no longer applies where it sits: `git merge-tree --write-tree --name-only main origin/fix/wave-merge-state-loss` conflicts in four files, `worktree.go` among them. (Probe only — no ref, index or working tree was modified.)

**One honest residual, now recorded in the row.** The branch's milestone mode skipped the `RemoveAll` altogether, so a non-`PROGRESS.md` file an earlier sibling merge had left in the feature dir survived — pinned by `TestWaveMergeDoesNotDeleteSiblingFiles`, for which `main` has no equivalent. `main` still wipes the whole directory at `:397`, so every file in the feature dir except `PROGRESS.md` remains last-writer-wins across a wave. That is a gap to close on `main`, not a reason to resurrect the branch, and the row says so. It is consistent with what NOTES.md already records about `worktree.go:397` under Polish.

**Verification Results**:
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass
- `go test -tags eval ./cmd/belmont`: pass
- `go vet ./...`: pass
- `staticcheck ./...`: pass (no output)
- `gofmt -l cmd/`: clean (no output)
- `belmont validate --root /Users/benlavender/repos/belmont`: ✓ No milestone-structure violations found
- **Line-citation gate**: every line reference written into the row was re-read at HEAD by script and matched its expected content exactly — `worktree.go` 370, 395, 397, 398, 410, 504, 530, 558, 569, 585, 586, 589, 618, 621; `state.go:260`; `worktree-state-isolation.md` 44 (§ heading) through 58 (last bullet before the next `## `).

**Self-Validation**:
- Acceptance Criteria: 3/3 — mechanism named with verified citations; the "only a test filename" characterisation corrected; verdict held at abandon with evidence that supports it.
- Visual Check: N/A (documentation).
- Table integrity: the row is a single line with exactly 4 pipes, no bare `|` in the prose.

**Scope adherence**: `worktree.go` untouched. `P0-M1-FIX-3`'s work (the `origin/feat/maintenance-ci` row, the headline bullet, the abandon-section preamble, the measurement-traps section) read for its evidence bar and left byte-for-byte unchanged. `P0-M1-FIX-6`, `P0-M1-FIX-7` and the three `[!]` tasks untouched. No `belmont install`. No push, merge, rebase, cherry-pick or branch deletion — the only git commands run were read-only (`log`, `show`, `cherry`, `cat-file`, `grep`, `merge-base`, `merge-tree --write-tree`).

**Commit**:
- **Hash**: the single commit on `main` whose subject is the message below (recorded by subject rather than hash — this log lives inside the commit it would name; same convention as `P0-M1-FIX-3`)
- **Message**: `P0-M1-FIX-5: Name the mechanism main actually uses in the wave-merge-state-loss verdict`

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-5 | `docs/branch-triage.md` headline bullet 2 lists `fix/wave-merge-state-loss` among collisions *"already on `main`"*. Its one commit `1fcbc7d4` is genuinely unlanded; what is on `main` is a different fix for the same defect. The bullet's conclusion (M3/M4/M6 uncontested) survives, but the wording overstates. Not corrected here — bullet edits belong to whoever owns that bullet, and this task's scope is the row. | P2 |
| FWLUP-2 | P0-M1-FIX-5 | `syncFeatureStateAfterMerge`'s `os.RemoveAll(dstFeature)` (`worktree.go:397`) still loses non-`PROGRESS.md` feature-dir files written by an earlier sibling merge in the same wave. The abandoned branch fixed this and pinned it with a test; `main` has neither. Closing it means either an overlay copy or an explicit preserve-list, plus a regression test. | P2 |

### Notes for Verification
- The single load-bearing claim to re-check is the line-citation gate: every number in the new row should resolve at HEAD to what the row says it does. It was checked by script, not by eye.
- The verdict was **not** flipped. If a reviewer disagrees, the argument to engage is the residual `RemoveAll` gap (FWLUP-2) — that is the only thing the branch does that `main` does not, and the row states it openly rather than burying it.
- `cmd/belmont/worktree.go` is unchanged in this commit; `git show --stat` should confirm only three files, all documentation or `.belmont/` tracking.

---

## Task P0-M1-FIX-6 — /belmont:next batch round 6 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `c76c20c0`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 6 of 7.
- **Created**: 2026-08-18T15:10:00Z
- **Tasks**:
  - [x] P0-M1-FIX-6: *"Three of the five contain no indented lines at all"* is wrong — only two are

## Orchestrator Context

### Current Task
`P0-M1-FIX-6` — the only task in this run. A Warning from M1's verification pass: a factually wrong claim sitting adjacent to a human-gated escalation.

### Active Task IDs
`P0-M1-FIX-6`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-4 and §Open Questions
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Canonical measurement**: `CENSUS.md` in the same directory — `P0-M1-FIX-2` established it as the single canonical copy of census figures, and pointed the other documents at it
- **The census data**: `census.json`
- **The code that measures it**: `cmd/belmont/extract.go` (`censusFeature`)

### The defect

The claim, repeated across several documents, is:

> *"Three of the five contain no indented lines at all."*

**Only two do.** Measured directly: `feat-058` has 0 indented lines, `feat-031` has 0 — but `repo-3/feat-015` has **167 indented lines / 84,593 B**. Every occurrence contradicts itself within the next clause or two, which typically continues "…only 83,500 are indented".

The claim the evidence actually supports is that **three of the five gain essentially nothing from extraction** — 0%, 0% and 7.6% respectively. That is the load-bearing point; "no indented lines" was a wrong shorthand for it.

**Why this one matters more than its size suggests.** The copy in `PROGRESS.md` (around line 28) sits directly beneath the human-gated `[!] P0-4a`, which is the escalation asking the repository owner to decide whether the wave structure is re-cut so M4 precedes M3. The owner will read that sentence as established fact while making the call.

### Required fix

1. **Measure first.** Re-derive the indented-line counts for all five over-threshold registers before editing anything. Do not inherit the numbers in this brief — the whole defect is a figure that was restated rather than re-derived.
2. **Correct every remaining copy.** As of this brief, grep finds occurrences in `CENSUS.md` (~line 128), `PROGRESS.md` (~line 28), `NOTES.md` (~line 45) and `MILESTONE-M1.done.md` (~lines 571, 987). `P0-M1-FIX-2` already corrected some copies, so **check what actually remains** rather than assuming this list is complete or that every hit needs changing.
3. **Restate it as the claim the evidence supports** — three of the five gain essentially nothing from extraction (0%, 0%, 7.6%) — rather than deleting the sentence. The point it was making is true and load-bearing for `[!] P0-4a`.
4. **Respect the canonical-copy rule** `P0-M1-FIX-2` established: the figure lives in `CENSUS.md`; other documents should reference it rather than restate it. Where a document must state it inline, keep it short and point at `CENSUS.md`.
5. **`MILESTONE-M1.done.md` is an archive.** `P0-M1-FIX-2` set the precedent of *annotating* it (`> **WITHDRAWN by …**`) rather than rewriting history. Follow that precedent — annotate, do not silently edit what a past agent wrote.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-6`.
- **Out of Scope**: `P0-M1-FIX-7` owns `runCensus`'s silent skip of an unreadable root — leave `cmd/belmont/extract.go` alone unless measuring requires reading it. `P0-M1-FIX-2`'s census totals (168/82/32.6%) and its §Correction are settled; do not revisit them. `P0-M1-FIX-3` and `-5`'s work in `docs/branch-triage.md` is settled; do not touch that file.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner. **This includes `P0-4a`'s body**, even though the wrong sentence sits inside it: correcting a factual error *within* a blocked task's description is permitted and is the point of this task, but do **not** change its `[!]` marker, its verdict, or the decision it asks for.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase, cherry-pick or delete any remote branch.
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
Plus, specific to this task: `grep -rn "no indented lines" .belmont/features/throughput/` must return nothing but the task line itself and any archive annotation that quotes it as withdrawn.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records this defect's cause: ***"a measurement was used to correct the spec before it was known to be complete"*** — and specifically that a derived figure was **restated across documents instead of living in one canonical place**. This is the same numeric-drift pattern already recorded in the sibling `framework-evaluation` feature, where four locations reported four different code-bleed counts.

**Prevention rule, and this task's bar**: state a measured figure once, in one file, and have every other document link to it. A wrong number repeated four times is four times as expensive to retract.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect and required fix are specified above; the census as re-measured is in `CENSUS.md` and `MILESTONE-M1.done.md` `## Task P0-M1-FIX-2`. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-6 — *"Three of the five contain no indented lines at all"* is wrong

**Status**: SUCCESS

**Measurement first (nothing in the brief was inherited).** All five over-threshold registers were
re-counted directly from the registers on disk, using the census's own rule — an indented line is a
non-blank line beginning with a space or tab; byte figures count the line plus its newline:

| Register | Total B | Indented lines | Indented B | Detail moved (census) |
|---|---:|---:|---:|---:|
| `repo-4/feat-075` | 1,860,979 | 11,832 | 1,209,680 | 1,169,656 (62.9%) |
| `repo-3/feat-015` | 1,022,749 | **167** | **84,593** | 77,976 (7.6%) |
| `repo-3/feat-070` | 247,361 | 533 | 140,957 | 141,001 (57.0%) |
| `repo-4/feat-031` | 129,354 | **0** | **0** | 0 (0.0%) |
| `repo-3/feat-058` | 114,941 | **0** | **0** | 0 (0.0%) |

Confirms the defect independently: **two** of the five are at zero, not three. Each file's total
bytes match `census.json` exactly, so the registers are unchanged since the census ran and the
disparity is in the sentence, not in the data.

Two further figures in the same clause were checked and **do not reproduce from any definition of
their terms**: `feat-015`'s task head lines measure **805,397 B** (335 unindented task lines,
matching `census.json`'s `tasks: 335`), not 795,799; its indented bytes are 84,593, not 83,500.
They are withdrawn alongside the sentence they qualified, and both replacements are stated with
the counting rule that produces them.

A third distinction surfaced while measuring and is now stated in `CENSUS.md`: **indented bytes
(84,593) are not movable bytes (77,976)**. `taskBodyEnd` claims a line only if it lies in a
*task's* body, so indentation beneath a heading stays in the index. "Has no indentation" and
"gains nothing from extraction" are different claims; conflating them is what produced the defect.

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/CENSUS.md` | §Why the estimate was wrong restated as *three of the five gain essentially nothing from extraction — 0%, 0%, 7.6%*; table gains a re-derived indented-line column; counting rule stated; the two registers with narrative (11,832 and 533 indented lines) named so the contrast is explicit; a `> **CORRECTED by P0-M1-FIX-6**` block withdraws the old sentence and the 795,799/83,500 pair. **This is the canonical copy.** |
| `.belmont/features/throughput/PROGRESS.md` | The copy inside `[!] P0-4a`'s body (line 28) restated to the supported claim and pointed at `CENSUS.md` rather than restating its figures. `[!]` marker, verdict, "Asked of", and the decision the owner is being asked to make are **unchanged**. Task marked `[x]`. |
| `.belmont/features/throughput/NOTES.md` | §Discovery bullet restated and pointed at `CENSUS.md`; a §Pattern line added recording that indented bytes ≠ movable bytes. |
| `.belmont/features/throughput/MILESTONE-M1.done.md` | **Annotated, not rewritten** (the `P0-M1-FIX-2` precedent): a `> **CORRECTED by P0-M1-FIX-6**` block beneath the passage that asserts the claim, and a *Closed by* line on the adjacent-tasks note that recorded the sentence as left standing. |

**Canonical-copy rule honoured**: the figures live in `CENSUS.md` §Why the estimate was wrong.
`PROGRESS.md` and `NOTES.md` state only the short claim and link there; neither restates the
per-register table. `MILESTONE-M1.done.md` links there too.

**Tests Added**: none. Documentation-only change; no Go source was modified. `cmd/belmont/extract.go`
was read (`censusFeature`, `taskBodyEnd`) to reproduce the census's definitions and was left
untouched — `runCensus`'s silent skip remains `P0-M1-FIX-7`'s.

**Verification Results** (full block from `### Verification before marking [x]`):
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go test -race ./cmd/belmont`: pass
- `go test -tags eval ./cmd/belmont`: pass
- `go vet ./...`: pass
- `staticcheck ./...`: pass (no findings)
- `gofmt -l cmd/`: no output
- `belmont validate --root /Users/benlavender/repos/belmont`: no milestone-structure violations
- **Grep gate** `grep -rn "no indented lines" .belmont/features/throughput/`: 14 hits remain, none of
  them an assertion of the claim — 11 in the documents, plus 3 in this log itself (its task heading,
  the gate command quoted on this line, and the commit message below). Classified in full:
  - `PROGRESS.md:37` and `MILESTONE.md:9` — the task line itself (allowed by the gate).
  - `MILESTONE.md:35`, `:39`, `:71` — this run's own brief quoting the defect it specifies, and the
    gate command's own text. Editing the brief would delete the specification, so these stand.
  - `CENSUS.md:149`, `MILESTONE-M1.done.md:573` — the two `CORRECTED by` annotations, which quote
    the sentence *as withdrawn* (allowed by the gate).
  - `MILESTONE-M1.done.md:571` — the archived assertion, now carrying its retraction on the line
    directly beneath it, per the annotate-don't-rewrite precedent.
  - `MILESTONE-M1.done.md:835`, `:989` — archived notes that name the sentence as this task's
    subject; `:989` now carries a *Closed by* annotation.
  - `NOTES.md:105` — the §Root Cause Patterns entry that already states the claim **is wrong**.
  No live document asserts the claim. Wording elsewhere was deliberately changed to "zero indented
  lines" so that the phrase itself survives only in retractions, task text, and this log. Counted
  after this log was written, so the figure is the one a reviewer re-running the gate will see.

**Self-Validation**:
- Acceptance Criteria: 5/5 — (1) measured before editing, all five re-derived; (2) every remaining
  copy corrected, checked by grep rather than by trusting the brief's list; (3) restated rather
  than deleted, keeping the load-bearing point for `[!] P0-4a`; (4) canonical-copy rule honoured;
  (5) the archive annotated, not edited.
- Visual Check: N/A — documentation only.

**Commit**:
- **Hash**: the single commit on top of the M1 baseline `c76c20c0` — `git log -1 --grep 'P0-M1-FIX-6'`. Stated this way rather than as a literal SHA because the log lives *inside* the commit it would name, so writing the SHA in and amending changes it again.
- **Message**: `P0-M1-FIX-6: Restate the "no indented lines" claim as the one the measurement supports`

---

### Out-of-Scope Issues Found (across all tasks)
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | P0-M1-FIX-6 | `P0-M1-FIX-6`'s own task line in `PROGRESS.md` cites stale line numbers (`CENSUS.md:48`, `NOTES.md:43`, `MILESTONE.md:566`) that never matched the files as they stand; the real locations were found by grep. Not corrected — it is the task line, and line-number citations in task text go stale by design. Worth noting as an argument for citing section headings rather than line numbers. | P3 |

### Notes for Verification
- The `795,799 B` and `83,500 B` figures were **not** in the task's literal wording, but sat inside
  the same clause as the sentence being corrected and were shown by measurement to be wrong. They
  were replaced rather than left standing; if a reviewer judges that outside scope, the change is
  confined to `CENSUS.md`'s §Why the estimate was wrong, the annotation beside it, and the
  `MILESTONE-M1.done.md` annotation, and is separable from the rest.
- `census.json`, the census totals (168 / 82 / 32.6%), `§Correction`, `docs/branch-triage.md`,
  `cmd/belmont/extract.go`, `[!] P0-3a`, `[!] P0-4a`'s marker/verdict/decision, and `[!] P0-13a`
  are all untouched. No `belmont install`; no push, merge, rebase or remote-branch change.
- Re-derivation is reproducible with a single pass over each `PROGRESS.md`: count non-blank lines
  whose first character is a space or tab, summing `len(line) + 1`.

---

## Task P0-M1-FIX-7 — /belmont:next batch round 7 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `4f0d7d6d`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Batch round 7 of 7 — the last follow-up.
- **Created**: 2026-08-18T15:35:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-7: `runCensus` silently skips a root it cannot read, so an incomplete walk looks complete

## Orchestrator Context

### Current Task
`P0-M1-FIX-7` — the only task in this run, and the last of M1's seven follow-ups.

### Active Task IDs
`P0-M1-FIX-7`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PRD**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PRD.md` — §P0-4 holds the parent task's acceptance criteria
- **TECH_PLAN**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/TECH_PLAN.md` — §Command Specifications (`belmont extract`)
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md`
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Canonical measurement**: `CENSUS.md` — settled by `P0-M1-FIX-2` and `-6`; do not revisit its figures
- **The code**: `cmd/belmont/extract.go` — `runCensus`, the `os.IsNotExist` branch at **line 148**
- **The tests**: `cmd/belmont/extract_test.go`

### The defect

`runCensus` walks each root's `.belmont/features/` directory. When a root cannot be read it hits `os.IsNotExist(err)` at `extract.go:148` and **`continue`s silently** — no error, no warning, no note in the output.

The consequence is not a crash, it is something worse: **an incomplete walk is indistinguishable from a complete one.** The denominator simply comes out smaller and reads as a finding.

This is the mechanism behind the Critical that `P0-M1-FIX-2` just spent a round correcting. The census substituted the Belmont fork for `repo-2`, walked four of the PRD's five repos, reported 138 dirs / 65 live, and then used that number to declare the PRD's own denominator wrong. Nothing in the tooling objected. Both reviewers independently hit the same class of failure from the other direction — running the documented reproduction command with unexpanded tildes and getting 43 live registers instead of 65, again with no error.

### Required fix

1. **Make an unreadable or missing root a reported condition, not a silent skip.** Decide whether it is a hard error or a prominent warning that still completes the run, and say why in the code. Consider that a census's entire value is that its coverage is knowable — a warning nobody reads reproduces the defect.
2. **The report must state its own coverage.** Whatever the output surface is (text, `census.json`, `CENSUS.md`), a reader must be able to tell which roots were actually walked and whether any were missed, without re-running anything.
3. **Add a test that fails under the old silent-skip behaviour.** Point the census at a root that does not exist and assert the run reports it. Confirm by reverting your change and watching the test fail — a test that passes both before and after proves nothing, which is exactly how the `MetricsRoot` Critical survived its own serial-path test earlier in this batch.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-7`.
- **Out of Scope**: `P0-M1-FIX-2` already rewrote `CENSUS.md` §Reproducing this with absolute paths, so the tilde symptom is gone — **do not redo that**, and do not revisit the settled census totals (168 dirs / 82 live / 32.6%) or its §Correction. `P0-M1-FIX-6` settled the indented-lines wording. `P0-M1-FIX-3` and `-5` settled `docs/branch-triage.md` — leave that file alone entirely.
- `extract` stays **census-only**: `--dry-run` remains mandatory, nothing may write a detail tier, and the M3/P1-1 refusal path must keep working.
- **Do NOT touch** `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated, awaiting the repository owner.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase, cherry-pick or delete any remote branch.
- Do **not** start M2–M11 work.
- **Zero external Go dependencies, no `go.sum`.**
- **Do not modify any of the five audited repositories** — the census is a read-only measurement over them.

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
Plus, specific to this task: the new test must fail with the change reverted, demonstrated rather than asserted. And re-running the census over the five real repos must still reproduce `CENSUS.md`'s settled figures — this fix must not change the measurement, only its honesty about coverage.

### Learnings from Previous Sessions

#### Feature Notes
`NOTES.md` §Root Cause Patterns records the pattern this task closes: ***"a census derived its own scope instead of taking the spec's enumeration"*** — the scope was assembled from what was to hand rather than the PRD's explicit list, **and `runCensus` swallows an unreadable root, so an incomplete walk is indistinguishable from a complete one.**

**Prevention rule, and this task's bar**: when a spec enumerates its subjects, use that enumeration verbatim and **fail loudly on any member that cannot be read**. A missing input must never degrade silently into a smaller denominator.

You are implementing the second half of that rule in code. The first half — taking the PRD's enumeration — was fixed by hand in `P0-M1-FIX-2`; this makes the tooling refuse to hide the failure next time.

Also relevant: ***"a new state write was only ever exercised on one of two execution paths"*** — the general lesson being that a test which passes with and without the fix is not a test of the fix.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The defect is at `extract.go:148`; the census as built is in `MILESTONE-M1.done.md` Pass 1 and `## Task P0-M1-FIX-2`. Explore further as needed.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-7 — `runCensus` silently skips a root it cannot read

**Status**: SUCCESS

**Files Modified**:
| File | Changes |
|---|---|
| `cmd/belmont/extract.go` | `os.IsNotExist -> continue` replaced by collect-and-report; `runCensus` gains an `allowUnreadableRoots` argument and returns an error naming every unread root; `censusReport` gains `RootsWalked`, `UnreadableRoots`, `CoverageComplete`; `renderCoverage` / `renderCoverageFooter` state coverage above and below the figures; `extract` gains `--allow-unreadable-roots` |
| `cmd/belmont/main.go` | `printUsage` names the new flag |
| `cmd/belmont/extract_test.go` | `TestCensusMissingRootIsNotAnError` (which pinned the defect) replaced by four tests; existing `runCensus` call sites take the new argument |
| `.belmont/features/throughput/CENSUS.md` | §Reproducing this: the sentence deferring the fix to this task replaced by what the tooling now does, plus a coverage statement for the run that produced the document. No figure touched. |
| `.belmont/features/throughput/census.json` | Regenerated from the same five-repo run — **purely additive**: `roots_walked`, `unreadable_roots`, `coverage_complete`. Every settled figure and the whole `features` array are byte-identical (verified by diff). |
| `.belmont/features/throughput/NOTES.md` | Root Cause Pattern "a census derived its own scope" gains a **Now enforced in code** line; two new Pattern entries |

**Tests Added**:
| Test | Coverage |
|---|---|
| `TestCensusMissingRootIsAnError` | a root that cannot be read fails the census; the error names the root and names `--allow-unreadable-roots`; the report still carries the coverage facts on the failing path |
| `TestCensusPartialWalkStatesItsOwnCoverage` | under the opt-in the run completes, the readable root is still measured, and the rendered report names both the walked root and the missed one, above **and** below the figures |
| `TestCensusCompleteWalkSaysSo` | a complete walk says `Coverage: COMPLETE`, names its root, and carries no incomplete footer |
| `TestCensusUnlistableRootIsReportedNotAborted` | a directory that exists but cannot be listed lands in the same coverage report rather than aborting mid-walk (skipped on Windows and as root) |

**The decision, and why** (requirement 1). **Hard error by default**, with the reasoning written into `runCensus`'s doc comment. A warning was rejected on this feature's own history: the numbers travel into other documents and the caveat does not — 138/65 was quoted into four files while its scope was never checked. So the number must not exist unless its scope is whole. The cost of that strictness — a five-repo census being useless while one repo is unmounted — is paid by an explicit `--allow-unreadable-roots` the operator has to type, which is *not* a quieter version of the same failure: the missed roots stay in the report rather than disappearing from the command line.

**Coverage in the report** (requirement 2). Text output opens with `Coverage: COMPLETE — all N requested roots were read` or `Coverage: INCOMPLETE — M of N …`, then lists each root as `walked` or `MISSED  <path> (<reason>)`; an incomplete run repeats the verdict below the table, because a caveat above a long table is the one a reader scrolls past. JSON carries `roots_walked`, `unreadable_roots` and `coverage_complete` (empty arrays, never `null`). `CENSUS.md` states the coverage of the run that produced it.

**The revert demonstration** (requirement 3). Reverting the change wholesale would break compilation — the tests would "fail" as a build error, which proves nothing — so only the behavioural line was reverted: `if os.IsNotExist(err) { continue }` re-inserted, signature kept. Result:

```
--- FAIL: TestCensusMissingRootIsAnError (0.00s)
    extract_test.go:248: a root that cannot be read must fail the census, not shrink its denominator
--- FAIL: TestCensusPartialWalkStatesItsOwnCoverage (0.00s)
    extract_test.go:284: coverage must be reported incomplete
    extract_test.go:293: unreadable roots: got [], want [.../gone]
    extract_test.go:298: the rendered census must state that its coverage is incomplete
    extract_test.go:304: the rendered census must name the root it missed
    extract_test.go:307: the verdict must be repeated below the figures, not only above them
```

Real assertion failures, not a build break. The file was then restored from a byte-identical backup and the suite re-run green. Stated plainly: the other two new tests pass with the change reverted — they cover the complete-walk and unlistable-directory branches, not the defect. Four tests, two proofs.

**The invariant: the measurement is unchanged.** Re-ran `CENSUS.md` §Reproducing this verbatim over the five real repos:

```
Coverage: COMPLETE — all 5 requested roots were read.
Denominator: 168 feature directories, of which 82 carry a live PROGRESS.md.
Detail moved   1749693 B  (32.6% of all register bytes)
```

`feature_dirs`, `live_registers`, `archived_dirs`, `dirs_without_register`, `total_bytes`, `total_index_bytes`, `total_detail_bytes`, `over_threshold_after` and the full `features` array all compare equal to the committed `census.json`. The five over-threshold registers and `[!] P0-4a` are untouched.

**The historical symptom, now loud.** The reviewers' unexpanded-tilde invocation:

```
$ belmont extract --dry-run --root /Users/benlavender/repo-1 --roots '~/repo-5,~/repo-4'
extract: 2 of 3 roots could not be read, so this census would cover only part of what was asked for:
  /Users/benlavender/repos/belmont/~/repo-5: no .belmont/features directory under this root
  /Users/benlavender/repos/belmont/~/repo-4: no .belmont/features directory under this root
…
exit=1
```

Previously this printed a plausible smaller number and exited 0.

**Verification Results** (every gate in the MILESTONE's block, all from `/Users/benlavender/repos/belmont`):
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass (21.8s)
- `go test -race ./cmd/belmont`: pass (25.2s)
- `go test -tags eval ./cmd/belmont`: pass (22.6s)
- `go vet ./...`: clean
- `staticcheck ./...`: clean
- `gofmt -l cmd/`: prints nothing
- `belmont validate --root /Users/benlavender/repos/belmont`: ✓ no milestone-structure violations

**Self-Validation**:
- Acceptance Criteria: 3/3 (reported condition with the decision justified in code; report states its own coverage on text, JSON and `CENSUS.md`; test demonstrated failing with the behaviour reverted)
- Visual Check: N/A — CLI output only

**Scope**: `docs/branch-triage.md`, the §Correction, the settled totals, the indented-lines wording and the three `[!]` tasks were not touched. `extract` remains census-only: `--dry-run` still mandatory (`TestExtractRefusesToWrite` green), nothing writes a detail tier. No `belmont install`, no push/merge/rebase/branch deletion, no write to any audited repository.

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-7 | `extract --all` is still registered then discarded (`_ = all`) and absent from `printUsage`, though TECH_PLAN §Commands names `--all --dry-run` as *the* census invocation. Already recorded in NOTES §Polish; not touched here. | P3 |

### Notes for Verification
- The behavioural change is deliberate beyond the reported defect: `belmont extract --dry-run` in a root with **no** `.belmont/features` now errors instead of printing an empty census. That is the rule working as intended (a named subject that yielded nothing is reported), but it is a user-visible change worth a second opinion.
- `runCensus`'s signature changed; all call sites are in `extract.go` and `extract_test.go`.
- `census.json` was regenerated. The diff is three added keys and nothing else — re-run the diff if in any doubt.

---

## Task P0-M1-FIX-8 — /belmont:next fix round 2 (2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: `3538b639`
- **Mode**: Lightweight (next skill — single task, no analysis agents). Fix round 2, the last before settle.
- **Created**: 2026-08-18T16:05:00Z
- **Tasks**:
  - [ ] P0-M1-FIX-8: Reconcile the three documentation inconsistencies the fix batch left behind

## Orchestrator Context

### Current Task
`P0-M1-FIX-8` — the only task in this run. Three Warnings from M1's focused re-verification, filed as one task because they share a single root cause.

### Active Task IDs
`P0-M1-FIX-8`

### File Paths

**Absolute paths. Use them verbatim.** The session working directory is `/Users/benlavender/belmont` — a *different repository*, the planning workspace, holding a different feature with open tasks. Never resolve `.belmont/` from the working directory and never write anything under `/Users/benlavender/belmont`. The harness resets the working directory after every Bash call, so prefix each shell command with `cd /Users/benlavender/repos/belmont &&`.

- **Repository root**: `/Users/benlavender/repos/belmont`
- **PROGRESS**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/PROGRESS.md` — the task's full description is on its own task line
- **Feature Notes**: `/Users/benlavender/repos/belmont/.belmont/features/throughput/NOTES.md`
- **Canonical measurement**: `CENSUS.md`
- **Canonical branch verdicts**: `docs/branch-triage.md`
- **Archive**: `MILESTONE-M1.done.md` — ~100 KB, append-only, annotated by FIX-2 and FIX-6

### The defect — three symptoms, one cause

The seven-task fix batch reproduced the very pattern it documented: **a claim restated across several documents, corrected in one place, left standing in the others.**

**(a) A retracted claim is still live where agents will read it.** `P0-M1-FIX-3` established that `origin/feat/maintenance-ci`'s leftovers were mis-described and that commit `1129cf84` was genuinely unlanded. It corrected `docs/branch-triage.md` in three places and added a §Revisions entry — but left the original claim standing at **`NOTES.md:21`** and **`MILESTONE-M1.done.md:460`** ("29/31 landed with both leftovers verified present"). `NOTES.md` is loaded into every session, so the *retracted* version is the one an agent actually reads. FIX-2 and FIX-6 both annotated their archive copies in place; FIX-3 did not, and that asymmetry is the defect.

**(b) A caveat is false at HEAD.** `docs/branch-triage.md:98-107` ends "The rows below it were not re-audited to that bar." `P0-M1-FIX-5` then re-audited the **immediately following row** (`fix/wave-merge-state-loss`) to exactly that bar, and `P0-M1-FIX-3` had already re-audited `worktree-git-excludes`. FIX-5 also added no §Revisions entry, though the document's own convention requires one.

**(c) A canonical document contradicts its own numbers.** `CENSUS.md` §Why the estimate was wrong asserts *"indentation is the upper bound on what extraction could move; what it does move is smaller"* — then gives `feat-070` **140,957 B indented** against **141,001 B moved**. Moved exceeds the stated upper bound by 44 B. The cause is real and verifiable: `censusFeature` (`cmd/belmont/extract.go:135-141`) claims every line index from `idx+1` to `taskBodyEnd`, blank lines inside a task body included, and a blank line is not an indented line. The conclusions are unaffected — but this is the canonical document feeding the human-gated `[!] P0-4a` decision.

### Required fix

1. **(a)** Annotate `MILESTONE-M1.done.md:460` with a `> **WITHDRAWN by P0-M1-FIX-3**` block in the same style FIX-2 and FIX-6 used — annotate, never rewrite the archive. Correct the `NOTES.md` bullet and point it at `docs/branch-triage.md` as canonical rather than restating the verdict.
2. **(b)** Amend the caveat to name the two rows that *have* been re-audited, and add FIX-5's missing §Revisions entry.
3. **(c)** Restate the upper-bound claim so it is true — either soften it to "approximately the movable content" or state the blank-line exception explicitly. Verify the 44 B discrepancy yourself against `census.json` and the register on disk before writing the correction; do not take this brief's numbers on trust.

### Scope Boundaries
- **In Scope**: only `P0-M1-FIX-8`.
- **Out of Scope**: every settled figure — the census totals (168 dirs / 82 live / 32.6%), §Correction, the indented-lines restatement, the branch verdicts themselves. You are fixing *consistency between documents*, not re-opening any conclusion. If a correction would change a conclusion, stop and report rather than proceeding.
- Do **not** re-audit branch verdicts. The 27 unchecked rows are a known, deliberately deferred gap.
- **Do NOT touch** `[!] P0-3`, `[!] P0-3a`, `[!] P0-4a`, `[!] P0-13a` — human-gated or awaiting the repository owner.
- **Do NOT** change any Go source. This is a documentation task; `censusFeature`'s blank-line behaviour is being *described*, not fixed.
- **Do NOT** run `belmont install`. **Do NOT** push, merge, rebase, cherry-pick or delete any remote branch.
- Do **not** start M2–M11 work.

### Verification before marking `[x]`
```bash
cd /Users/benlavender/repos/belmont
go build ./cmd/belmont
go test ./cmd/belmont
go vet ./...
gofmt -l cmd/              # must print nothing
belmont validate --root /Users/benlavender/repos/belmont
```
Plus, specific to this task: `git diff --stat` must show **no change under `cmd/`**, and a grep for the retracted "29/31" claim must return only annotated retractions.

### Learnings from Previous Sessions

#### Feature Notes
This task closes the loop on a pattern `NOTES.md` §Root Cause Patterns already records twice — once as *"a measurement was used to correct the spec before it was known to be complete"*, and once, in the sibling `framework-evaluation` feature, as four locations reporting four different code-bleed counts.

**Prevention rule, and this task's bar**: state a figure or a verdict once, in one canonical file, and have every other document link to it. A wrong claim repeated N times costs N times as much to retract — and a retraction that reaches only some copies is worse than none, because it makes the remaining copies look confirmed.

Worth noticing, and worth recording: this batch **documented that pattern and then reproduced it**, three times, in the same round. That is the finding, not the three edits.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The three symptoms are located precisely above.]

## Design Specifications
[Not populated — no design input: this task has no Figma URL, reference image, or visible UI.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-8 — Reconcile the three documentation inconsistencies the fix batch left behind

**Status**: SUCCESS

**Files Created**: none.

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/NOTES.md` | **(a)** The `29/31 landed with both leftovers verified present` bullet at `:21` no longer restates a branch verdict. It now records that `feat/maintenance-ci`'s abandon verdict stands but not on the reason first written here, quotes the withdrawn claim *as withdrawn*, and points at `docs/branch-triage.md` as canonical. The settled conclusion — *M3, M4 and M6 are not standing on contested ground* — is unchanged. **(Step 4b)** One new §Pattern line recording the both-directions indented-vs-movable finding, pointing at `CENSUS.md` rather than restating it. |
| `.belmont/features/throughput/MILESTONE-M1.done.md` | **(a)** A `> **WITHDRAWN by P0-M1-FIX-3 (2026-08-18; annotation added by P0-M1-FIX-8)**` block inserted beneath the `:460` bullet, indented into the list item exactly as `P0-M1-FIX-2`'s `:332` annotation is. **Annotated, not rewritten** — the archive line itself is untouched. |
| `docs/branch-triage.md` | **(b)** §*Abandon — content verified on `main`* preamble: the false closing sentence *"The rows below it were not re-audited to that bar"* is replaced by a naming of the two rows that have been re-audited (`fix/worktree-git-excludes` by FIX-3, `fix/wave-merge-state-loss` by FIX-5) and the two that have not (both pre-rebase backup branches), plus an italic note recording the correction. §Revisions gains the entry `P0-M1-FIX-5` never wrote, marked as added retrospectively, and an entry for this preamble correction. **No verdict re-opened; no row's verdict changed.** |
| `.belmont/features/throughput/CENSUS.md` | **(c)** §Why the estimate was wrong: *"Indentation is the upper bound on what extraction could move"* is withdrawn and restated as *approximately the movable content, not a strict upper bound*, with both directions of the gap stated explicitly and the 44 B named. A `> **CORRECTED by P0-M1-FIX-8 (2026-08-18)**` block follows, in the same style as the FIX-6 block below it. **No figure, table or conclusion changed.** |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-8` marked `[x]`. |

**Tests Added**: none — documentation-only task, and the brief forbids Go changes. A throwaway `cmd/belmont/*_test.go` was used purely as a measurement harness for the (c) verification and **deleted before commit**; `git diff --stat -- cmd/` and `git status --porcelain -- cmd/` are both empty.

**The 44 B, re-derived rather than taken on trust** (required by the brief):
- `census.json` → `repo-3/feat-070`: `total_bytes` 247,361, `index_bytes` 106,360, `detail_bytes` **141,001**.
- Register on disk (`/Users/benlavender/repo-3/.belmont/features/feat-070/PROGRESS.md`), counting rule as stated in `CENSUS.md`: **533 indented lines / 140,957 B**. Both reproduce the document exactly.
- Re-running `censusFeature`'s own claim logic (`parseMilestones` + `taskBodyEnd`, lines `idx+1..end`) over that register: moved = 141,001 B, of which **44 lines are blank, contributing 44 B** (one newline each); **0 B** of moved bytes are non-blank and unindented; and **0 B** of its indented lines sit outside a task body. So `141,001 = 140,957 + 44` exactly, and the discrepancy is entirely in-body blank lines.
- Cross-checked against the other four over-threshold registers: for `feat-075` (40,241 B indented-but-not-moved vs 217 B in-body blanks) and `feat-015` (6,667 B vs 50 B) the other term dominates and moved comes in *below* indented — which is why the restated sentence says the two quantities differ in both directions rather than simply inverting the old claim. All five registers' published indented-line counts reproduced.

**Verification Results**:
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `go vet ./...`: pass
- `gofmt -l cmd/`: prints nothing
- `belmont validate --root /Users/benlavender/repos/belmont`: `✓ No milestone-structure violations found.`
- `git diff --stat` shows **no change under `cmd/`**: confirmed (empty diff and empty status for that path)
- `grep -rn "29/31"`: 6 hits, all accounted for — `docs/branch-triage.md:125` (the row that carries its own replacement notice), `MILESTONE-M1.done.md:460` (now directly followed by the WITHDRAWN block), `NOTES.md:21` (quotes it as withdrawn), `PROGRESS.md:39` and `MILESTONE.md:34,64` (the task brief itself). **No unannotated copy remains.**

**Self-Validation**:
- Acceptance Criteria: 3/3 — (a) archive annotated in FIX-2/FIX-6 style and NOTES bullet re-pointed at the canonical file; (b) caveat amended to name the re-audited rows and FIX-5's §Revisions entry added; (c) upper-bound claim restated as true, with the 44 B independently re-derived first.
- Visual Check: N/A — documentation only.

**Commit**:
- **Hash**: see below
- **Message**: `P0-M1-FIX-8: Reconcile the three documentation inconsistencies the fix batch left behind`

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | P0-M1-FIX-8 | None. Every settled figure was left as it stands; nothing encountered warranted a follow-up. | — |

### Notes for Verification
- **Nothing was re-measured except (c)'s 44 B, and nothing was re-decided.** The census totals, §Correction, the indented-lines restatement and all branch verdicts are byte-identical to their pre-task state. The `CENSUS.md` diff is confined to one italic paragraph plus one new blockquote; `docs/branch-triage.md`'s to one preamble paragraph plus two §Revisions entries.
- **The archive was annotated, never rewritten** — `MILESTONE-M1.done.md:460` still reads exactly as `P0-13` wrote it, with the retraction beneath it. This follows the precedent the archive itself records at `:1654`.
- **The two remaining un-re-audited rows in §Abandon — content verified on `main` are now named in the document** (`origin/backup/loop-efficiency-pre-rebase`, `origin/backup/state-readers-pre-rebase`). This is a statement of the known deferred gap, not an audit of it — the other sections' rows are untouched and remain deferred.
- One judgement call worth a second pair of eyes: the restated (c) sentence says term (i) — indentation outside a task body — is *"usually the larger term"*. That is asserted from the five over-threshold registers, where it holds in every case that has any of either, not from an estate-wide measurement. It is hedged for that reason.

---

## Task P0-M1-FIX-8 (round 3, 2026-08-18)

# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: e49c04fd8a311f102a024fab7a409728fb8b113e
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: 2026-08-18 16:18
- **Tasks**:
  - [ ] P0-M1-FIX-8: Reconcile the three documentation inconsistencies the fix batch left behind — final promotion edit

## Orchestrator Context

### Current Task
P0-M1-FIX-8 — the single remaining edit that verification's PARTIAL verdict requires before promotion. The task was DELIVERED in a prior round and verification passed all five acceptance criteria with one substantiated Warning. Only that Warning is in scope now.

**The objection (verbatim from PROGRESS.md)**: the replacement sentence in `CENSUS.md` §Why the estimate was wrong introduced a *new* claim asserted beyond its evidence — "usually the larger term, and why detail moved normally comes in below the indented total". Measured over all 82 live registers: **75 (91.5%) have moved == indented exactly**; 5 come in below (`feat-020`, `feat-016`, `feat-071`, `feat-075`, `feat-015`); 2 come in above (`feat-063` +1 B, `feat-070` +44 B). The normal case is equality, not "below".

**The fix is one sentence** — replace that clause with the measured distribution above, as an annotation in the FIX-6/FIX-8 blockquote style already used elsewhere in `CENSUS.md`, changing nothing else. No re-derivation is needed; the measurement is recorded above and is authoritative. Change no figure, table or conclusion. `[!] P0-4a` is unaffected and must not be touched.

### Active Task IDs
`P0-M1-FIX-8`. This is a follow-up task minted by verification; its full definition lives on its task line in `.belmont/features/throughput/PROGRESS.md` (M1 section), not in PRD.md.

### File Paths
- **PRD**: .belmont/features/throughput/PRD.md — authoritative constraints (note: FIX tasks are defined in PROGRESS.md, not here)
- **TECH_PLAN**: .belmont/features/throughput/TECH_PLAN.md — technical specs
- **Master TECH_PLAN**: .belmont/TECH_PLAN.md — cross-cutting architecture
- **PROGRESS**: .belmont/features/throughput/PROGRESS.md
- **Feature Notes**: .belmont/features/throughput/NOTES.md
- **Global Notes**: .belmont/NOTES.md (does not exist)
- **Target document**: .belmont/features/throughput/CENSUS.md — §Why the estimate was wrong

### Scope Boundaries
- **In Scope**: Only P0-M1-FIX-8's one-sentence reconciliation in `CENSUS.md`. If the same over-asserted clause is quoted verbatim in another file, check `NOTES.md` line ~54 first — the feature's convention is that `CENSUS.md` is canonical and other files link rather than restate; do not propagate the new sentence into non-canonical copies.
- **Out of Scope**: Everything else. No Go code changes — a previous verification pass established that any `git diff -- cmd/` output for this task would itself be a Critical finding. No changes to figures, tables, conclusions, `[!]` tasks, or milestone structure.

### Learnings from Previous Sessions

#### Feature Notes (selected, most binding for this task)
- **State a measured figure once, in one canonical file** (`CENSUS.md` for census figures); every other document links to it rather than restating. A wrong number repeated four times is four times as expensive to retract (Root Cause Patterns, P0-4 and FIX-6).
- **The batch documented a pattern and then reproduced it** (Root Cause Patterns, P0-M1-FIX-8): round 2 fixed three inconsistencies and introduced a fourth over-asserted claim in the very sentence it corrected. Before writing *usually*, *normally*, *always* or *the upper bound*, the distribution must be measured over the full population — here it already has been (75/82 equal, 5 below, 2 above); use exactly that, assert nothing beyond it.
- **An indented line is not the same thing as a movable byte, and the gap runs in both directions** (NOTES §Pattern): `taskBodyEnd` claims blank lines inside a task body (not indented) and leaves indentation outside task bodies (not moved). `feat-070` +44 B and `feat-063` +1 B are the above-cases; the five named registers are the below-cases.
- The full NOTES.md at `.belmont/features/throughput/NOTES.md` is 154 lines and must be read directly — the sections above are the ones that bind this edit.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. This is a documentation-only edit to CENSUS.md; no code exploration should be needed.]

## Design Specifications
[Not populated — no design surface. No Figma URLs.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-8 — Reconcile the three documentation inconsistencies the fix batch left behind (final promotion edit)

**Status**: SUCCESS

**Files Created**: none.

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/CENSUS.md` | §Why the estimate was wrong: the over-asserted clause on term (i) (*"usually the larger term, and why 'detail moved' normally comes in below the indented total"*) replaced with the mechanical statement *"which is the term that can pull 'detail moved' below the indented total"*; a `RECONCILED by P0-M1-FIX-8` blockquote added below the existing FIX-8 blockquote, in the same FIX-6/FIX-8 style, carrying the measured distribution. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-8` `[ ]` → `[x]`; one sentence appended to the task line recording the round-3 outcome (the previous line said "Not attempted", which would have contradicted the marker). |
| `.belmont/features/throughput/NOTES.md` | One `### Debugging` line: `census.json` is re-derivable, and the byte-vs-character trap in doing so. |

**Tests Added**: none — documentation-only edit; `git diff -- cmd/` is empty by design (a Go change here would itself be a Critical finding).

**Verification Results**:
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass (`ok belmont/cmd/belmont 22.879s`)
- `git diff -- cmd/`: empty ✔
- Figures/tables/conclusions changed: none. The only prose deleted is the withdrawn clause.

**Self-Validation**:
- Acceptance criteria: 5/5.
  1. *The over-asserted clause is gone* — replaced. `grep "usually the larger term" CENSUS.md` returns exactly one line, and it is the annotation quoting the clause as withdrawn, not an assertion of it.
  2. *Replaced with the measured distribution* — 75/82 (91.5%) equal, 5 below (named, with magnitudes), 2 above (`feat-063` +1 B, `feat-070` +44 B).
  3. *Annotation in the FIX-6/FIX-8 blockquote style* — same `> **… by \`P0-M1-FIX-8\` (2026-08-18…)**` opening, withdrawal + restatement + "no figure, table or conclusion changes" close.
  4. *Nothing else changed* — 20 lines added, 3 removed in `CENSUS.md`; no code.
  5. *`P0-4a` untouched* — confirmed in the `PROGRESS.md` diff. (Noted for accuracy: it reads `[v]` on disk, resolved 2026-08-18, not `[!]` as the brief described it. Either way, not touched.)
- Visual check: N/A (documentation).

**Beyond the brief (deliberate, and cheap)**: the MILESTONE said no re-derivation was needed. It was done anyway, because the failure this task exists to correct is precisely *restating a figure instead of measuring it*. `censusFeature`'s rule (`parseMilestones` task bullets + `taskBodyEnd` indent-bounded bodies, `len(line)+1` **bytes**) was replayed over all 82 registers `census.json` names in a scratchpad script. It reproduced **every** `total_bytes` and `detail_bytes` field in `census.json` exactly (0/82 disagreements), and returned 75 equal / 5 below / 2 above with the same seven registers and the same +1 B / +44 B magnitudes recorded in the objection. The recorded measurement is confirmed, not merely repeated.

**One fact the objection did not carry, now in the annotation**: 67 of the 75 equal registers are equal at **zero** — they hold no indented lines at all — so only 8 registers balance two non-zero terms. Without that, "the normal case is equality" reads as "the two terms usually cancel", which is a stronger claim than the data supports and would have been the same over-assertion in the opposite direction.

**Commit**:
- **Hash**: see `git log -1` for `P0-M1-FIX-8`
- **Message**: `P0-M1-FIX-8: Replace the over-asserted term-(i) clause with the measured distribution`

---

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | P0-M1-FIX-8 | None. `MILESTONE-M1.done.md:2139` also records the hedge behind the withdrawn clause, but it is an archived run log — a historical record of what was believed at the time, correctly left as written. | — |

### Notes for Verification
- The distribution can be re-checked without trusting this run: replay `censusFeature`'s rule over the registers `census.json` names and confirm it reproduces `total_bytes`/`detail_bytes` for all 82 before reading the verdict. Count **bytes**; a character-based port disagrees with all 82 on the totals while still returning 75/5/2, so the verdict alone does not prove the port faithful.
- `CENSUS.md` §Why the estimate was wrong now carries three blockquotes (FIX-8, FIX-8 second pass, FIX-6). That is chronologically odd but positionally correct — each annotates the paragraph directly above it.
- The `PROGRESS.md` task line was edited beyond the marker. This is the one prose edit outside `CENSUS.md`, and it is there because the line's "**Not attempted**" clause would otherwise stand beside an `[x]`.
