# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: 5d832d49 (belmont: verify P0-M1-FIX-8 — promoted [v]; mint P0-M1-FIX-9 from round-3 code review)
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: 2026-08-18 16:35
- **Tasks**:
  - [ ] P0-M1-FIX-9: Restate the pre-existing "moved exceeds indented" clause in CENSUS.md §Why the estimate was wrong

## Orchestrator Context

### Current Task
P0-M1-FIX-9 — minted by the round-3 code review of `P0-M1-FIX-8` (2026-08-18). One clause in `.belmont/features/throughput/CENSUS.md:144-146` (the italic paragraph above the FIX-8 blockquotes) over-asserts in exactly the way FIX-8 just withdrew two lines below.

**The finding (from the code-review report, measured)**: the clause *"Where a register has no indentation outside a task body, only (ii) is left and **moved exceeds indented**"* is contradicted by the evidence FIX-8 added. Measured over all 82 live registers: **77 have term (i) = 0**, and of those **75 have moved == indented exactly; only 2 exceed** (`feat-063` +1 B, `feat-070` +44 B). "Exceeds" additionally requires term (ii) > 0, which holds in only 5 of 82 registers.

**The fix (recommended wording, from the review)**: replace the over-assertion with *"only (ii) is left, so moved is **at least** indented, and exceeds it whenever a task body contains a blank line — which is exactly the case for `repo-3/feat-070` below"*. Apply it in the document's established annotation style (see the sibling `CORRECTED by`/`RECONCILED by` blockquotes in the same section — and note NOTES.md's Polish entry asking for consistent vocabulary; prefer `CORRECTED by \`P0-M1-FIX-9\`` since this one *is* a wrong claim). Change no figure, no table, no conclusion.

### Active Task IDs
`P0-M1-FIX-9`. Verification-minted follow-up: its full definition lives on its task line in `.belmont/features/throughput/PROGRESS.md` (M1 section, directly below `[v] P0-M1-FIX-8`), not in PRD.md.

### File Paths
- **PRD**: .belmont/features/throughput/PRD.md — background only; FIX tasks are defined in PROGRESS.md
- **TECH_PLAN**: .belmont/features/throughput/TECH_PLAN.md
- **Master TECH_PLAN**: .belmont/TECH_PLAN.md
- **PROGRESS**: .belmont/features/throughput/PROGRESS.md
- **Feature Notes**: .belmont/features/throughput/NOTES.md
- **Global Notes**: .belmont/NOTES.md (does not exist)
- **Target document**: .belmont/features/throughput/CENSUS.md — the italic paragraph at ~lines 144-146, §Why the estimate was wrong

### Scope Boundaries
- **In Scope**: Only the P0-M1-FIX-9 clause replacement in `CENSUS.md`, plus its PROGRESS.md bookkeeping.
- **Out of Scope**: Everything else. **No Go code changes — any `git diff -- cmd/` output for this task is itself a Critical finding** (established in FIX-8's rounds 2 and 3). No changes to figures, tables, conclusions, the FIX-6/FIX-8 blockquotes, `[!]` tasks, or milestone structure. Do not "fix" the `censusFeature` blank-line behaviour — that write path is M3/P1-1.

### Learnings from Previous Sessions

#### Feature Notes (most binding for this task; read the full 154+-line NOTES.md directly)
- **Root Cause Pattern (2026-08-18, minted with this task)**: fix rounds scoped to the objected clause leave the same defect standing one paragraph up. When a fix withdraws an over-assertion, **re-read the whole enclosing section against the same measurement before committing** — every claim of the same shape (*usually*, *normally*, *exceeds*, *below*) either re-derives from the data or receives the same annotation in the same round. You are the round this rule now applies to: after your edit, sweep §Why the estimate was wrong end-to-end for any remaining claim of that shape and report (do not fix) anything you find beyond your clause.
- **State a measured figure once, canonically** (`CENSUS.md` for census figures); other files link, never restate. The distribution (75/5/2, 67-at-zero, +1 B/+44 B) is already in the FIX-8 blockquote directly below your target clause — reference it, do not duplicate the full table of numbers a second time in the same section.
- **Before writing *usually*/*normally*/*always*/*exceeds*, the distribution must be measured over the full population and the sample size stated.** Yours already is: 77 term-(i)=0 registers, 75 equal, 2 above. Assert nothing beyond it.
- **Byte-vs-character trap** (NOTES §Debugging, 2026-08-18): if you re-derive anything from `census.json`, count bytes (`len(line)+1`), not characters — a character-based port still returns 75/5/2 while disagreeing on every total, so the verdict alone does not prove a port faithful.

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. Documentation-only edit to CENSUS.md; no code exploration should be needed.]

## Design Specifications
[Not populated — no design surface. No Figma URLs.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-9 — Restate the pre-existing "moved exceeds indented" clause in CENSUS.md §Why the estimate was wrong

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/CENSUS.md` | Term (ii)'s clause restated *at least* rather than *exceeds*; new `CORRECTED by P0-M1-FIX-9` blockquote appended to the paragraph's existing annotation group. Paragraph re-wrapped to the file's width. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-9` `[ ]` → `[x]` |

**Tests Added**: none — documentation-only edit, no code surface.

**Verification Results** (`git diff --numstat`: CENSUS.md **17 added / 5 removed**, PROGRESS.md 1/1):
- `git diff -- cmd/`: **empty** (the task's Critical gate)
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass
- `belmont validate --root .`: no milestone-structure violations

**Self-Validation**:
- Acceptance Criteria: 5/5 passed
  1. The clause *"only (ii) is left and moved exceeds indented"* is replaced by *"only (ii) is left, so moved is **at least** indented, and exceeds it whenever a task body contains a blank line"* — the wording the round-3 review recommended.
  2. Annotated in the established style — `**CORRECTED by \`P0-M1-FIX-9\` (2026-08-18).**`, matching the FIX-6/FIX-8 `CORRECTED` siblings. Chose `CORRECTED` over `RECONCILED` per NOTES §Polish (this *is* a wrong claim), which also narrows the vocabulary inconsistency that entry flags rather than widening it.
  3. No figure, table or conclusion changed — confirmed line-by-line from the diff; every number in the section is untouched.
  4. The distribution is referenced, not restated — the new blockquote points at the FIX-8 blockquote above it rather than repeating 75/5/2, per the "state a measured figure once" rule.
  5. Section swept end-to-end (see below).
- Visual Check: N/A (markdown document)

**Re-derivation performed before writing** (the claim is a relation between measurements, so it was re-measured rather than taken from the report of it): replayed the indented-byte rule over all 82 registers `census.json` names, counting **bytes** as `len(line)+1` per NOTES §Debugging's byte-vs-character trap. Reproduced the recorded distribution exactly — **75 equal, 2 above (`repo-3/feat-070` +44 B, `repo-3/feat-063` +1 B), 5 below** (−40,024 / −9,906 / −6,617 / −399 / −231 B), same registers in the same order. The five over-threshold registers' indented-line counts also reproduce exactly (0, 0, 167/84,593 B, 11,832/1,209,680 B, 533/140,957 B), so every figure in the section stands.

**Commit**:
- **Hash**: see `git log -1`
- **Message**: `P0-M1-FIX-9: Restate the "moved exceeds indented" clause as the relation the rule supports`

---

### Section sweep — §Why the estimate was wrong, end to end

Required by the Root Cause Pattern minted with this task ("re-read the whole enclosing section against the same measurement"). Every remaining claim of the *usually / normally / exceeds / below* shape was re-derived:

| Claim | Verdict |
|---|---|
| "Indentation is **approximately** the movable content, not a strict upper bound … differ in both directions" | Holds — 5 below and 2 above, both directions attested. |
| "(i) … the term that **can** pull 'detail moved' below the indented total" | Holds — correctly hedged, and 5 registers exhibit it. |
| "Three of the five gain essentially nothing from extraction — 0%, 0% and 7.6%" | Holds — table figures re-measured on disk. |
| "Two of the five carry zero indented lines — not three" | Holds — `feat-031` and `feat-058` measure 0 indented lines; the other three do not. |
| "only 32 of its 335 tasks have a body at all" / 805,397 B task heads / 84,593 B indented (`feat-015`) | Indented figure re-measured exactly (167 lines / 84,593 B); the task-head and body-count figures are FIX-6's re-derived pair and were not re-run. |

**No further over-assertion found inside the section.** It converges here.

### Out-of-Scope Issues Found
| ID | Found During | Description | Priority |
|---|---|---|---|
| FWLUP-1 | P0-M1-FIX-9 | **The same withdrawn claim is still live in a second file.** `.belmont/features/throughput/NOTES.md:54` (§Pattern) reads *"Where a register has no indentation outside a task body, moved **exceeds** indented"* — verbatim the assertion this task just withdrew from `CENSUS.md`. It is also the "state a measured figure once, link from everywhere else" rule broken in the same line: the bullet restates the relation *and* names CENSUS.md as canonical. Left untouched deliberately — the MILESTONE's Scope Boundaries limit this task to the CENSUS.md clause and instruct me to report, not fix, anything the sweep turns up beyond it. One-line fix: replace *exceeds* with *is at least, and exceeds only where a task body holds a blank line*. | P2 |

### Notes for Verification
- The Critical gate for this task is `git diff -- cmd/` being empty. It is empty; nothing outside the two `.belmont/` files was touched.
- The new blockquote deliberately quotes **no new figure**. Every number it relies on is already in the FIX-8 blockquote immediately above it. If verification wants the distribution independently, the replay is: for each register in `census.json`, sum `len(line)+1` over non-blank lines starting with a space or tab and compare to that register's `detail_bytes` — bytes, not characters.
- FWLUP-1 above is the fourth instance of this defect class in this section's history (FIX-6 → FIX-8 → FIX-9 → NOTES.md:54). The pattern's own prevention rule says the sibling claims get the same annotation *in the same round*; the scope boundary for this task forbade it, so the residue is filed rather than fixed.
