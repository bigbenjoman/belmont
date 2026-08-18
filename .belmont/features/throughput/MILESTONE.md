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
