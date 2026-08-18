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
