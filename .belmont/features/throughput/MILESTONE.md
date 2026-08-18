# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: abcc3cb1 (belmont: archive P0-M1-FIX-9 milestone log; mint P0-M1-FIX-10 from its sweep)
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: 2026-08-18 16:45
- **Tasks**:
  - [ ] P0-M1-FIX-10: Restate the withdrawn "moved exceeds indented" relation still live in NOTES.md:54

## Orchestrator Context

### Current Task
P0-M1-FIX-10 — found by `P0-M1-FIX-9`'s mandated section sweep (2026-08-18). `.belmont/features/throughput/NOTES.md` line ~54, the §Pattern bullet beginning **"The gap runs in both directions, so indentation is not an upper bound."**, still asserts verbatim the relation FIX-9 just withdrew from CENSUS.md: *"Where a register has no indentation outside a task body, moved **exceeds** indented"*. Measured (FIX-8/FIX-9, all 82 registers): 77 registers have no indentation outside task bodies, 75 of them have moved **equal** to indented, only 2 exceed.

**The fix is one clause**: restate the NOTES.md sentence to match the corrected canonical statement — moved is *at least* indented, and exceeds it *whenever a task body contains a blank line*. The bullet's `repo-3/feat-070` +44 B example is measured, correct, and stays. The bullet's closing pointer ("Canonical statement of the rule: `CENSUS.md` §Why the estimate was wrong — not restated here beyond this warning") stays. Nothing else in NOTES.md moves. NOTES.md is a notes file, not a canonical annotated document — a plain in-place rewording of the clause is right; do NOT add a CENSUS-style `CORRECTED by` blockquote, but a short parenthetical `*(clause corrected by \`P0-M1-FIX-10\`, 2026-08-18 — first written as "exceeds", the measured relation is "at least")*` at the end of the sentence keeps the file's own correction convention (see the FIX-3/FIX-8 parenthetical at line ~21 for the style).

### Active Task IDs
`P0-M1-FIX-10`. Verification-minted follow-up: its full definition lives on its task line in `.belmont/features/throughput/PROGRESS.md` (M1 section, directly below `[x] P0-M1-FIX-9`), not in PRD.md.

### File Paths
- **PROGRESS**: .belmont/features/throughput/PROGRESS.md
- **Target document**: .belmont/features/throughput/NOTES.md — §Pattern bullet at ~line 54 ONLY
- **Canonical reference**: .belmont/features/throughput/CENSUS.md §Why the estimate was wrong (read-only for this task — FIX-9's corrected clause and blockquote are the wording to match)
- **PRD / TECH_PLAN**: .belmont/features/throughput/PRD.md, TECH_PLAN.md — background only
- **Global Notes**: .belmont/NOTES.md (does not exist)

### Scope Boundaries
- **In Scope**: Only the one clause in the NOTES.md:54 bullet, plus PROGRESS.md bookkeeping (`[ ]` → `[x]` on P0-M1-FIX-10).
- **Out of Scope**: Everything else. **No Go code changes — any `git diff -- cmd/` output is itself a Critical finding.** No CENSUS.md changes (FIX-9 already landed there). No figures change anywhere. Do not touch other NOTES.md bullets, the `[!]` tasks, or milestone structure. After your edit, grep the feature dir for any remaining live assertion of the withdrawn relation ("moved exceeds indented" / "exceeds the indented total") outside archived logs and the annotations that quote it as withdrawn — report (do not fix) anything you find.

### Learnings from Previous Sessions
- **This is the fourth instance of one defect class** (FIX-6 → FIX-8 → FIX-9 → FIX-10): an over-asserted relation restated across files. The prevention rule (NOTES §Root Cause Patterns, 2026-08-18): when a fix withdraws an over-assertion, every claim of the same shape in reach either re-derives from the data or gets corrected in the same round — hence the closing sweep required above.
- **State a measured figure once, canonically.** CENSUS.md holds the distribution; NOTES.md:54 must keep only the warning + the pointer, exactly as its own closing sentence promises.
- No re-derivation is needed for this task: the relation was re-derived twice on 2026-08-18 (FIX-8 round 3 verification, FIX-9 implementation), both reproducing 75 equal / 2 above / 5 below exactly. Cite, don't re-measure.

## Codebase Analysis
[Not populated — lightweight mode. One-clause NOTES.md edit; no code exploration needed.]

## Design Specifications
[Not populated — no design surface. No Figma URLs.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-10 — Restate the withdrawn "moved exceeds indented" relation still live in NOTES.md:54

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/NOTES.md` | §Pattern bullet at line 54, one clause only: *"moved **exceeds** indented"* → *"moved is **at least** indented, and exceeds it whenever a task body contains a blank line"*, matching `CENSUS.md:145` verbatim in relation. A short in-line parenthetical `*(clause corrected by \`P0-M1-FIX-10\`, 2026-08-18 — first written as "exceeds", the measured relation is "at least")*` closes the sentence, in the file's own correction convention (cf. the FIX-3/FIX-8 parenthetical at line 21). The `repo-3/feat-070` 141,001 B / 140,957 B / +44 B example and the closing canonical pointer are untouched. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-10` marker `[ ]` → `[x]`. |

**Tests Added**: none — documentation-only clause edit, no code surface.

**Verification Results**:
- Go build (`go build ./cmd/belmont`): pass
- Unit tests (`go test ./cmd/belmont`): pass
- `git diff -- cmd/`: **empty** (the scope boundary's Critical condition does not fire)
- Total diff: 2 files, 2 insertions, 2 deletions — one line each

**Self-Validation**:
- Acceptance Criteria: 7/7 passed
  1. The withdrawn relation is gone from NOTES.md:54; the clause now reads *at least … exceeds it whenever a task body contains a blank line* — the same relation as the corrected canonical statement in `CENSUS.md` §Why the estimate was wrong. Pass.
  2. The `feat-070` +44 B example is byte-identical. Pass.
  3. The closing pointer ("Canonical statement of the rule: `CENSUS.md` … not restated here beyond this warning") is byte-identical. Pass.
  4. No figure changes anywhere — the diff carries no digit change; 141,001 / 140,957 / 44 / 2026-08-18 all survive verbatim. Pass.
  5. No CENSUS.md change, no CENSUS-style `CORRECTED by` blockquote added to NOTES.md, no other NOTES.md bullet touched, no `[!]` task touched, no milestone structure touched. Pass.
  6. No Go code changes. Pass.
  7. The mandated closing sweep ran (below). Pass.
- Visual Check: N/A (no visual surface)

**Closing sweep** (mandated by the Scope Boundaries and by the FIX-9 Root Cause Pattern). `grep -rn -i "exceeds" | "upper bound" | "indented total"` across `.belmont/features/throughput/`. **No remaining live assertion of the withdrawn relation.** Every surviving hit is one of:
- **Archived log** — `MILESTONE-M1.done.md` (rounds 2/3 of FIX-8, the FIX-9 brief and log, and FWLUP-1 which is this very task). Explicitly outside the sweep's scope.
- **An annotation that quotes it as withdrawn** — `CENSUS.md:180,183` (the `CORRECTED by P0-M1-FIX-9` blockquote), `CENSUS.md:153` (the `CORRECTED by P0-M1-FIX-8` blockquote quoting the retired "upper bound" sentence), `NOTES.md:163,165` (the Root Cause Pattern that records the class).
- **The task line that defines the fix** — `PROGRESS.md:38,39` and this MILESTONE file.
- **Unrelated senses of the word** — `TECH_PLAN.md:223,264`, `PRD.md:28,97,230,354`, `CENSUS.md:106` (the 25,000-token open question), and the `_research/` corpus. None concerns the moved-vs-indented relation.
- **Correct, in-force statements** — `CENSUS.md:139` (*"approximately the movable content, not a strict upper bound … differ in both directions"*), `CENSUS.md:142` (*"the term that **can** pull 'detail moved' below the indented total"*, correctly hedged), `CENSUS.md:145` (the corrected clause this task matched to), `NOTES.md:153` (the prevention rule). All re-read against the 82-register distribution (77 term-(i)=0; of those 75 equal, 2 above; 5 below estate-wide) and all hold.

Nothing found to report beyond the clause fixed.

**Commit**:
- **Hash**: the single commit carrying this log — `git log -1 --oneline -- .belmont/features/throughput/MILESTONE.md`. Deliberately not written out: this log is *inside* the commit it would name, so any literal hash is stale the moment it is amended in (it was — `1274f76c` became `621a03f9` on the amend that added it). A restated figure that cannot be right is the defect this whole fix chain is about.
- **Message**: `P0-M1-FIX-10: Restate NOTES.md's "moved exceeds indented" clause as the measured relation`

---

### Out-of-Scope Issues Found (across all tasks)
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | — | None. The mandated sweep found no further live instance of this defect class in the feature directory. | — |

### Notes for Verification
- **Re-derivation was explicitly not required** and was not performed: the distribution was measured twice on 2026-08-18 (FIX-8 round 3, FIX-9 implementation), both reproducing 75 equal / 2 above / 5 below. This task cites it; per the MILESTONE's own instruction, "cite, don't re-measure". The check that matters here is *textual agreement with `CENSUS.md:144-146`*, which was made by direct comparison.
- **The bullet's bold lead — "The gap runs in both directions, so indentation is not an upper bound" — was deliberately left as-is.** It is not the withdrawn relation: it states the two-directional gap, which `CENSUS.md:139` asserts in force and which 5-below/2-above attests. Changing it was out of scope and would have been an unmandated edit to a correct sentence.
- **No `NOTES.md` §Root Cause Patterns entry was added.** The pattern this task is the fourth instance of is already recorded (`NOTES.md:162-166`, minted by FIX-9), and its prevention rule is exactly the sweep performed above. Adding a fifth restatement of it would reproduce the "state it once, canonically" defect the section exists to prevent.
- This is a documentation-only change: `git diff -- cmd/` is empty and the whole diff is two single-line changes.
