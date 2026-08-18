# Milestone: M1 — Toolchain, atomic writes & baseline (Single Task)

## Status
- **Milestone**: M1: Toolchain, atomic writes & baseline
- **Git Baseline**: d5dfe848 (belmont: verify round 4 — promote P0-M1-FIX-10 [v], hold FIX-9 [x], mint P0-M1-FIX-11)
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: 2026-08-18 17:05
- **Tasks**:
  - [ ] P0-M1-FIX-11: Correct the FIX-9 blockquote's description of its own history (CENSUS.md:180-181)

## Orchestrator Context

### Current Task
P0-M1-FIX-11 — minted by round-4 code review (2026-08-18). The `CORRECTED by P0-M1-FIX-9` blockquote in `.belmont/features/throughput/CENSUS.md` (~lines 179-188) misdescribes the document's own history: it says the withdrawn term-(ii) clause is *"the same over-assertion the reconciliation directly above withdraws, standing one paragraph up"*. Both halves are wrong on the plain reading: the `RECONCILED by P0-M1-FIX-8` block above withdrew a **term (i)** clause asserting "below"; FIX-9 withdrew a **term (ii)** clause asserting "exceeds" — same **class of** over-assertion, opposite directions — and the clause stands three blocks up (its paragraph sits above two intervening blockquotes), not one.

**The fix**, exactly as the review specified:
1. "the same over-assertion" → "the same **class of** over-assertion" (matching the qualifier PROGRESS.md:38 and NOTES.md's root-cause entry already carry).
2. "standing one paragraph up" → "in the paragraph above both corrections" (or delete the locator clause).
3. **Optional, in the same edit**: append "(5 of the 82 registers)" to the adjacent "term (ii) … is frequently zero" hedge in the same blockquote, so the canonical file carries the figure the hedge rests on. That figure is measured and confirmed twice (FIX-9 implementation, round-4 verification): exactly 5 registers have in-body blank lines (`feat-015` 50, `feat-020` 32, `feat-063` 1, `feat-070` 44, `feat-075` 217).

Nothing else changes. No figure, table or conclusion moves.

### Active Task IDs
`P0-M1-FIX-11`. Verification-minted follow-up: full definition on its task line in `.belmont/features/throughput/PROGRESS.md` (M1, directly below `[v] P0-M1-FIX-10`), not in PRD.md.

### File Paths
- **PROGRESS**: .belmont/features/throughput/PROGRESS.md
- **Target document**: .belmont/features/throughput/CENSUS.md — the `CORRECTED by P0-M1-FIX-9` blockquote ONLY
- **PRD / TECH_PLAN**: background only
- **Feature Notes**: .belmont/features/throughput/NOTES.md — see the round-4 addendum in §Root Cause Patterns ("a sentence describing a correction is itself a claim about the document") and the round-4 Polish entries
- **Global Notes**: .belmont/NOTES.md (does not exist)

### Scope Boundaries
- **In Scope**: The two wording corrections (and the optional "(5 of the 82 registers)" append) inside the FIX-9 blockquote, plus PROGRESS.md bookkeeping (`[ ]` → `[x]` on P0-M1-FIX-11). Annotate the change *inside the same blockquote* minimally — e.g. extend its dateline to `(2026-08-18; wording corrected by \`P0-M1-FIX-11\` same day)` — do NOT add a fourth sibling blockquote for a two-word fix.
- **Out of Scope**: Everything else. **No Go code changes — any `git diff -- cmd/` output is itself a Critical finding.** No change to the FIX-8 blockquotes, the corrected clause at ~145, any figure, any other file. After the edit, re-read the full blockquote once against the page and the data (per the round-4 root-cause addendum) and report — do not fix — anything else in it that misstates either.

### Learnings from Previous Sessions
- **Round-4 root-cause addendum (binding here)**: a sentence *describing* a correction is itself a claim about the document and gets the same read-it-against-the-page check as a claim about the data. Your edit is precisely such a sentence — check the corrected locator ("in the paragraph above both corrections") against the actual block layout before writing it.
- **State a measured figure once, canonically**: the "(5 of the 82 registers)" append moves the hedge's figure INTO the canonical file — it is not a duplication. Do not restate the 75/5/2 distribution; the FIX-8 blockquote above already carries it.
- No re-derivation needed: the 5-of-82 figure was independently confirmed twice on 2026-08-18. Cite, don't re-measure.
- **Verification of this task is deliberately deferred** — it will batch with the next verify pass (per round-4 code review, to avoid a fifth standalone round). Mark `[x]` only; do not expect a same-session verify.

## Codebase Analysis
[Not populated — lightweight mode. Two-word documentation edit; no code exploration needed.]

## Design Specifications
[Not populated — no design surface. No Figma URLs.]

## Implementation Log

### Summary
- **Tasks Completed**: 1
- **Tasks Blocked**: 0
- **Total Commits**: 1

---

### Task: P0-M1-FIX-11 — Correct the FIX-9 blockquote's description of its own history

**Status**: SUCCESS

**Files Created**: none

**Files Modified**:
| File | Changes |
|---|---|
| `.belmont/features/throughput/CENSUS.md` | The `CORRECTED by P0-M1-FIX-9` blockquote only (~179-189): "the same over-assertion" → "the same **class of** over-assertion"; "standing one paragraph up" → "in the paragraph above both corrections"; the "frequently zero" hedge now carries its measured figure; dateline extended to name `P0-M1-FIX-11`. |
| `.belmont/features/throughput/PROGRESS.md` | `P0-M1-FIX-11` `[ ]` → `[x]` plus the implementation note on its task line. |

**Tests Added**: none — documentation-only edit, no code surface.

**Verification Results**:
- `git diff -- cmd/`: **empty** (the scope boundary's Critical condition does not fire)
- `go build ./cmd/belmont`: pass
- `go test ./cmd/belmont`: pass (cached, `ok belmont/cmd/belmont`)
- `belmont validate`: clean — no milestone-structure violations
- Diff size: CENSUS.md 7 lines rewrapped, no figure/table/heading touched

**Self-Validation**:
- Acceptance Criteria: 3/3 (both mandated corrections + the optional figure append), plus the two scope conditions (annotation inside the same blockquote, no fourth sibling; nothing else in the document changed)
- Visual Check: N/A — no visual surface
- Locator re-checked against the page before writing, per the round-4 root-cause addendum: the withdrawn term-(ii) clause sits in the italic paragraph at ~137-150, which precedes all three blockquotes — so it is above both FIX-8 corrections, and "one paragraph up" was wrong in the original.

**Deviation from the literal instruction (deliberate, one item)**
The optional append was written as *"is frequently zero — only 5 of the 82 registers have any"* rather than the literal `(5 of the 82 registers)`. Two reasons, both from the addendum's read-it-against-the-page rule:
1. Attached directly to "frequently zero", the bare parenthetical asserts *zero in 5 of 82*. The true count is the complement — 77 of 82 have term (ii) = 0; 5 have in-body blank lines (`feat-015` 50, `feat-020` 32, `feat-063` 1, `feat-070` 44, `feat-075` 217).
2. The sibling blockquote immediately above names a **different** set of 5 (the registers whose moved comes in *below* indented). Same cardinality, different members, one paragraph apart — "have any" states the referent so the two cannot be conflated.
The figure itself is cited, not re-measured, per the milestone brief.

**Blockquote sweep — report only, not fixed (out of scope)**
One inherited imprecision remains in the same blockquote: *"As with the first correction, **both quoted numbers were right and only the claim relating them was wrong**."* The first correction did quote two numbers (141,001 B moved / 140,957 B indented); the clause **this** blockquote withdraws — *"only (ii) is left and moved exceeds indented"* — quotes none, so "both quoted numbers" has no referent inside this correction. Low severity and arguably reads as a back-reference to the first correction's pair. Everything else in the blockquote checks out against the page and the data: the withdrawn text matches what FIX-9 replaced, "the reconciliation directly above" is the `RECONCILED by P0-M1-FIX-8` block, "the two-register exception" matches the +1 B / +44 B pair, and the 75/5/2 distribution is pointed at rather than restated.

**Commit**:
- **Hash**: see `P0-M1-FIX-11` commit on this branch
- **Message**: `P0-M1-FIX-11: Correct the FIX-9 blockquote's description of its own history`

---

### Out-of-Scope Issues Found (across all tasks)
| ID | Found During | Description | Priority |
|---|---|---|---|
| — | P0-M1-FIX-11 | *"both quoted numbers were right"* in the FIX-9 blockquote (detail above) — inherited from the first correction, no referent in this one. Not raised as a task: it is a back-reference a reader resolves correctly, and a sixth round on this section costs more than it fixes. | P3 |

### Notes for Verification
- Verification of this task is **deliberately batched** with the next verify pass (round-4 code review's instruction) — no standalone round expected.
- The two mandated corrections are byte-checkable against the task line in PROGRESS.md. The third change is the deviation documented above; if the reviewer prefers the literal parenthetical, note that it states the complement of the measured figure.
- The dateline annotation is inside the existing blockquote by instruction — there is deliberately no fourth sibling blockquote.
- No figure, table, heading or conclusion moved; `git diff -- cmd/` is empty.
