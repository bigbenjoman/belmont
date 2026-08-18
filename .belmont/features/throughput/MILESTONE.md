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
