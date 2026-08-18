# Pre-change baseline — Belmont Throughput

*Captured 2026-08-18 by `throughput` P0-3, before any of M2–M10 landed.*
*Machine-readable companion: `baseline.json`. M11/P3-3 re-measures against this file.*

## What this was taken against

| | |
|---|---|
| Orchestrator | `/opt/homebrew/bin/belmont (pinned Homebrew v0.11.0)` |
| Belmont version | `belmont 0.11.0 (bb5f6ef, 2026-08-17T10:56:07Z)` |
| Repo commit | `25249853a727dd251861557801f67e4b940867e5` |
| Go toolchain | `go version go1.26.0 darwin/arm64` |
| Token conversion | 4 bytes/token — the conversion every measurement in the PRD uses |

The Go toolchain is recorded because P0-12 was sequenced *ahead* of this task for exactly that
reason: a before-and-after comparison whose two halves were built differently proves nothing.
**M11 must re-measure on go1.26.0 or record why not.**

## Read-path cost — the half this feature actually changes

Measured with the pinned v0.11.0 binary, per feature, across both target repos. Every figure is a
byte count of real output, reproducible by re-running the same commands.

| Repo | Live registers | Master register | Register bytes (total) | Median | p90 | Max |
|---|---:|---:|---:|---:|---:|---:|
| repo-3 | 29 | 498,879 | 1,973,359 | 17,950 | 100,523 | 1,022,749 |
| repo-4 | 14 | 247,645 | 2,219,167 | 17,089 | 60,678 | 1,860,979 |

| Repo | Raw registers | `status --feature` (text) | `status --format json` | json:text | raw:text |
|---|---:|---:|---:|---:|---:|
| repo-3 | 1,973,359 | 237,735 | 2,008,535 | 8.4x | 8.3x |
| repo-4 | 2,219,167 | 136,638 | 1,090,735 | 8.0x | 16.2x |

**The PRD's cited figures reproduce.** `repo-4/feat-075` measures 1,860,979 B raw,
57,145 B as default text and 682,148 B as JSON — the same numbers the PRD's composition analysis
and its 12x JSON warning are built on. The plan is measuring the system it describes.

### Registers over 100 KB (the M5 error ceiling, = 25,000 tokens)

**repo-3** — 4 of 29:

- `feat-015` — 1,022,749 B (~255,687 tokens)
- `feat-070` — 247,361 B (~61,840 tokens)
- `feat-058` — 114,941 B (~28,735 tokens)
- `feat-010` — 100,523 B (~25,130 tokens)

**repo-4** — 2 of 14:

- `feat-075` — 1,860,979 B (~465,244 tokens)
- `feat-031` — 129,354 B (~32,338 tokens)

Six registers across the two repos exceed the error ceiling today. These are P3-1's migration
targets; the median feature (17 KB) gains nothing from migration, exactly as the plan assumes.

## Per-milestone tokens and wall-clock — NOT CAPTURED, and why

**This half of the baseline does not exist yet, and no number was invented to fill it.**

Requires instrumented `belmont auto` runs. The P0-2 instrumentation exists only in this working tree; the orchestrating binary is pinned to Homebrew v0.11.0, which has no metrics code, and no .belmont/metrics/ directory exists in either repo. Numbers were NOT invented to fill the gap.

Three facts make this unavoidable rather than an omission:

1. **The orchestrating binary is pinned to Homebrew v0.11.0** (master TECH_PLAN rule 1), which
   contains no instrumentation. P0-2's code exists only in this working tree.
2. **Neither repo has ever recorded a metric** — `~/repo-3/.belmont/metrics/` and
   `~/repo-4/.belmont/metrics/` do not exist. Verified, not assumed.
3. **Capturing it means running real milestones to `[v]` in both repos** with an instrumented
   build. That is live agent work costing real tokens against two production repositories; it is
   not something an implementation agent should trigger unasked.

### How to capture it

```bash
cd ~/repos/belmont && go build -o /tmp/belmont-instrumented ./cmd/belmont
# run at least one milestone to [v] in each repo with THAT binary, then:
/tmp/belmont-instrumented metrics --feature SLUG --root ~/repo-3 --format json
/tmp/belmont-instrumented metrics --feature SLUG --root ~/repo-4  --format json
```

Append the results to `baseline.json` under `per_milestone_cost_baseline` and set its `status`
to `CAPTURED`. **Until that happens, M11/P3-3 can compare the read-path half of the success
criteria and must report the cost half as unevidenced rather than estimating it.** The PRD is
explicit that instrumentation reports nothing rather than guessing; a baseline is the last place
to break that rule, because every later claim is stated against it.

## Reproducing the read-path half

```bash
belmont status --root <repo> --feature <slug> --color never | wc -c   # text
belmont status --root <repo> --feature <slug> --format json   | wc -c   # json
wc -c < <repo>/.belmont/features/<slug>/PROGRESS.md                     # raw
```
