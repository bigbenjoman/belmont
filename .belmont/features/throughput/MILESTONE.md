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

