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
