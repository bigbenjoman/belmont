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
