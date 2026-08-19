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

> **METHOD CHANGED 2026-08-19 — decision by Ben Lavender.** The procedure below previously said
> "run at least one milestone to `[v]` in **each repo**", meaning `~/repo-3` and `~/repo-4`.
> That is **withdrawn**, for two independent reasons, and the second is the load-bearing one.
>
> 1. **Blast radius.** `belmont auto` is an autonomous implementation agent, not an observer.
>    Run against `~/repo-3` it made 13 commits to `main` including two database migrations
>    (local only; nothing pushed). Both repos are revenue-critical and repo-4 is pre-launch.
>    Measurement must not be able to break the thing it measures.
> 2. **It cannot produce the comparison it exists for.** A milestone can only be implemented
>    once. Driving `feat-004` M1 with the *old* Belmont leaves no way to
>    re-run *that same milestone* with the new one, so M11/P3-3 would be comparing two
>    different pieces of work and attributing the difference to Belmont. A before-number whose
>    after-number must come from different work is not a baseline, it is an anecdote.
>
> The replacement is a purpose-built, resettable test repository. Same milestone, same starting
> state, run twice — which is the only design that isolates Belmont's contribution.

The testbed lives outside every product repo and is seeded once, then reset between arms:

```bash
# 1. Seed (once). The seed commit is the fixed starting state for every arm.
#    The register MUST be seeded to the sizes measured in CENSUS.md — this feature targets
#    large-register read paths, and a toy PROGRESS.md exercises none of them.
git -C <testbed> tag baseline-seed

# 2. Arm A — current Belmont (no throughput work), from the seed:
git -C <testbed> reset --hard baseline-seed
<belmont-current> auto --feature <slug> --from M1 --to M1 --tool claude
<belmont-current> metrics --feature <slug> --root <testbed> --format json   # → arm A

# 3. Arm B — this tree's Belmont, from the *identical* seed:
git -C <testbed> reset --hard baseline-seed
cd ~/repos/belmont && go build -o /tmp/belmont-instrumented ./cmd/belmont
/tmp/belmont-instrumented auto --feature <slug> --from M1 --to M1 --tool claude
/tmp/belmont-instrumented metrics --feature <slug> --root <testbed> --format json  # → arm B
```

Append both arms to `baseline.json` under `per_milestone_cost_baseline`, record the seed commit
SHA and the register sizes alongside them, and set `status` to `CAPTURED`. Run each arm more
than once if the budget allows — agent runs are noisy, and a single pair cannot separate a real
improvement from run-to-run variance.

**Until that happens, M11/P3-3 can compare the read-path half of the success criteria and must
report the cost half as unevidenced rather than estimating it.** The PRD is explicit that
instrumentation reports nothing rather than guessing; a baseline is the last place to break that
rule, because every later claim is stated against it.

**Records that exist but are not the baseline.** Seven metrics records were captured from the
withdrawn repo-3 runs (`~/repo-3/.belmont/metrics/feat-004.jsonl`). They
are real measurements of real work and are kept for reference, but they are **not** the baseline
and must not be presented as one: no milestone reached `[v]`, and two of the records are
zero-usage phases from session-limit exits (see `P0-M1-FIX-12`, which is why the summary now
flags them). Nothing in `baseline.json` is derived from them.

## Reproducing the read-path half

```bash
belmont status --root <repo> --feature <slug> --color never | wc -c   # text
belmont status --root <repo> --feature <slug> --format json   | wc -c   # json
wc -c < <repo>/.belmont/features/<slug>/PROGRESS.md                     # raw
```
