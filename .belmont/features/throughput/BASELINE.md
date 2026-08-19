# Pre-change baseline — Belmont Throughput

*Captured 2026-08-18 by `throughput` P0-3, before any of M2–M10 landed.*
*Machine-readable companion: `baseline.json`. M11/P3-3 re-measures against this file.*

## What this was taken against

| | |
|---|---|
| Orchestrator | `/opt/homebrew/bin/belmont (pinned Homebrew v0.11.0)` |
| Belmont version | `belmont 0.11.0 (bb5f6ef, 2026-08-17T10:56:07Z)` |
| Repo commit | `fc33be901b4fd04e0196b50f780810a8fe52d575` |
| Go toolchain | `go version go1.26.0 darwin/arm64` |
| Token conversion | 4 bytes/token — the conversion every measurement in the PRD uses |
| Measured repo revisions | `repo-3 af16dabd3411fc68852e51f3868d69f091326fb4` · `repo-4 2c1f7e857b30722018a3d860bb05a079f8daf1be` |
| Coverage | **repo-3 and repo-4 only — 43 of 82 live registers (52%), 78.0% of register bytes** |

**The measured repos are pinned, not just the binary** *(added 2026-08-19, `P0-M1-FIX-14`)*. The
read-path commands below run against live working trees, so without these revisions re-running them
returns today's estate rather than the baseline. This is not hypothetical: measured one day after
capture, 3 of 43 registers had already moved — repo-3's register total by +21,037 B, its max by
+14,371 B, its master register by +12,593 B. M11/P3-3 scores this feature against this file, and
unpinned, organic register growth is indistinguishable from the change's effect — exactly the
confound the controls exist to remove. Note `repo-4/feat-031`'s 129,354 B was uncommitted
working-tree state at capture, committed verbatim the same day as `79fee00e`.

**What this file does NOT cover** *(added 2026-08-19, `P0-M1-FIX-15`)*. The estate is the five repos
`PRD.md` names and `P0-M1-FIX-2` re-established. This baseline covers two of them. Absent:
repo-1 (10 registers, 574,327 B), repo-2 (18, 386,620 B), repo-5 (11, 219,500 B). Estate-wide
*register sizes* for all five do exist — `CENSUS.md` / `census.json`, captured the same day by
`P0-4` — and that is the record to use for size; `status`-output sizes exist for no repo outside
these two. One consequence is concrete and belongs to P3-3: `repo-1/feat-020` (360,542 B) is one of
census's seven `over_threshold_before` entries and therefore on P3-1's migration list, with **no
read-path baseline to be re-measured against**. P3-3 reports it as unbaselined rather than
estimating it, per the PRD's rule that an unmeasured half is declared, never invented.

The Go toolchain is recorded because P0-12 was sequenced *ahead* of this task for exactly that
reason: a before-and-after comparison whose two halves were built differently proves nothing.
**M11 must re-measure on go1.26.0 or record why not.**

## Read-path cost — the half this feature actually changes

Measured with the pinned v0.11.0 binary, per feature, across both target repos. Every figure is a
byte count of real output, reproducible by re-running the same commands.

**Median and p90 use `sorted[int(p*n)]`, zero-based, no interpolation** — the rule `cmd/belmont/percentile` implements and `belmont extract` publishes. *Corrected 2026-08-19 by `P0-M1-FIX-26(a)`: the original figures used **mean-of-two-middles** for the median and **`sorted[int(p*(n-1))]`** for p90 — off by one in the index base — and neither document said so. repo-4's median moved 17,089 → 20,297 and its p90 60,678 → 129,354; repo-3's p90 100,523 → 114,941. No underlying measurement changed: totals and maxima reproduce byte-for-byte, and the per-feature figures in `baseline.json` are untouched. The finding that raised this diagnosed the old p90 as linear-interpolation-lower; it was not — that yields 103,406.6 and 108,751.2, matching neither repo. The rule was identified by re-deriving every candidate against both published values.*

**Why the documents moved and not the code.** M11/P3-3 re-measures with `belmont extract`, so the baseline has to be re-derivable by the statistic the shipped code produces — otherwise the before-and-after compare two different quantities. Changing `percentile` instead would silently restate every figure in `CENSUS.md`, which two agents have independently re-derived and verified.

| Repo | Live registers | Master register | Register bytes (total) | Median | p90 | Max |
|---|---:|---:|---:|---:|---:|---:|
| repo-3 | 29 | 498,879 | 1,973,359 | 17,950 | 114,941 | 1,022,749 |
| repo-4 | 14 | 247,645 | 2,219,167 | 20,297 | 129,354 | 1,860,979 |

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

Requires instrumented `belmont auto` runs. The P0-2 instrumentation exists only in this working tree; the orchestrating binary is pinned to Homebrew v0.11.0, which has no metrics code, and at capture time (2026-08-18) no .belmont/metrics/ directory existed in either repo. Numbers were NOT invented to fill the gap. *(Re-tensed 2026-08-19 by P0-M1-FIX-26(d), for the same reason fact 2 below was: six records were written into repo-3 later that same day by the withdrawn runs, as §Records that exist states.)*

Three facts make this unavoidable rather than an omission:

1. **The orchestrating binary is pinned to Homebrew v0.11.0** (master TECH_PLAN rule 1), which
   contains no instrumentation. P0-2's code exists only in this working tree.
2. **Neither repo had ever recorded a metric at capture time (2026-08-18)** —
   `~/repo-3/.belmont/metrics/` and `~/repo-4/.belmont/metrics/` did not exist. Verified, not
   assumed. *(Restated 2026-08-19 by `P0-M1-FIX-13`: this read "has ever recorded", present
   perfect, which §Records that exist contradicts two sections below — six records were written
   into `~/repo-3` later that same day by the withdrawn runs. The fact was true when written and
   is now a statement about a moment, so it is tensed like one.)*
3. **Capturing it needs instrumented runs, and where those may happen is now constrained.**
   *(Revised 2026-08-19.)* This originally read "running real milestones to `[v]` in both repos",
   which was acted on and withdrawn — see §How to capture it. Measurement in a product repo is
   **observational only**; anything that drives a run happens in a disposable seeded testbed.
   Neither arm is something an implementation agent should trigger unasked.

### How to capture it

> **METHOD CHANGED 2026-08-19 — product re-planning session, decision by Ben Lavender.**
> The procedure here previously said "run at least one milestone to `[v]` in **each repo**",
> meaning `~/repo-3` and `~/repo-4`. **Withdrawn.** It was acted on once, on 2026-08-18: four
> `belmont auto` runs drove 13 commits into `repo-3`'s `main`, including two database
> migrations (local only, nothing pushed). Review record:
> `~/belmont/repo-3-agent-run-review-2026-08-19.md`.
>
> Two independent reasons, and the second is load-bearing:
> 1. **Blast radius.** `belmont auto` is an implementation agent, not an observer. Measuring
>    Belmont must not be able to damage what it measures, and repo-4 is pre-launch.
> 2. **It could not produce its own comparison.** A milestone is implemented once, so driving a
>    real milestone with the old build leaves no way to re-run *that same milestone* with the
>    new one. The after-arm would necessarily be different work.
>
> The replacement is **two arms with distinct jobs** — see PRD §Success Criteria, §Constraints
> and §Research Notes (2026-08-19), which are canonical. This file holds the procedure only.

> **TIMING CORRECTED 2026-08-19 — tech-planning session.** The block below previously showed both
> arms running back-to-back at P0-3, with arm order alternating across replicates. **That cannot
> happen at M1**: alternation requires both builds to exist at the same time, and when the baseline
> was to be captured the post-change build did not exist. Running arm A now and arm B months later
> would confound prompt-cache warmth, vendor drift and machine state *with the arm* — the three
> things the controls below exist to remove.
>
> **The paired suite therefore runs in full at M11/P3-3**, both arms interleaved, against
> `belmont-pre`: a binary frozen at M1's merge commit and kept. **M13** builds the harness and runs
> an A/A pilot that sizes the suite; **M11** runs it. Nothing about "pre-change" is lost — what
> freezes is the artefact, not the calendar.

**Arm 1 — seeded paired suite (carries the causal claim).** Disposable testbed under a temp
directory, never inside a product repository or this one — the harness refuses to run otherwise.
Its state is **generated** to match the worst measured register's profile in size and structure
(1,860,979 B, 30 milestones, 529 tasks, 458 with detail). It is not a copy: read-path cost is a
function of bytes and structure rather than of words, and the real register is a pre-launch
product's complete feature planning. The milestone under test is deliberately trivial — what is
measured is read-path overhead, which does not scale with how much code a milestone writes.

Built and sized by **M13** (`P0-17`–`P0-20`), executed by **M11/P3-3**. Harness form is build-tagged
test files, not a shipped subcommand — see `TECH_PLAN.md` §The bench harness.

```bash
# Built once when M1 verifies, kept, and never refreshed until the M11 rollout.
git -C ~/repos/belmont rev-parse HEAD > /tmp/belmont-pre.sha
bash scripts/build.sh && cp ./belmont ~/bin/belmont-pre

# Seed once; this commit is the fixed starting state for every arm and replicate.
git -C <testbed> tag baseline-seed

# Pair count is DERIVED from the A/A pilot's measured variance, not assumed:
#   sd 0.3 -> 6 pairs   sd 0.7 -> 12   sd 1.0 -> 20
# ALTERNATE arm order across replicates - A/B, then B/A, then A/B.
# STRICTLY SERIAL: wall-clock is a measured outcome, and concurrent local runs
# contend for the same memory bandwidth (1.43x on two slots, not 14.31x).
git -C <testbed> reset --hard baseline-seed
~/bin/belmont-pre   auto --feature <slug> --from M<n> --to M<n> --tool claude    # arm A
~/bin/belmont-pre   metrics --feature <slug> --root <testbed> --format json

git -C <testbed> reset --hard baseline-seed
~/bin/belmont-post  extract --feature <slug> --root <testbed>    # arm B migrates first;
                                                                 # cost recorded SEPARATELY
~/bin/belmont-post  auto --feature <slug> --from M<n> --to M<n> --tool claude    # arm B
~/bin/belmont-post  metrics --feature <slug> --root <testbed> --format json
```

Arm B migrates because M5's size ceiling would otherwise refuse the deliberately pathological seed
— and because migrated state *is* the post-M11 world, so measuring arm B unmigrated would measure a
configuration that will never exist. Its cost is a separate line, never amortised into the per-run
figure. **Acceptance criterion on the seed**: extraction must land it under `progress_error_bytes`,
because five of 82 real registers still exceed the ceiling after extraction and a seed with that
property makes arm B unable to start at all.

Each arm installs **its own** prose into the testbed from a fixed commit checkout, and the harness
asserts that prose resolved before the timed run begins. Without that check both arms silently
resolve from user-level `~/.agents/skills/belmont/` and M2, M4 and M7 — whose entire deliverable is
prose — measure as zero improvement.

**Arm 2 — passive observation (carries external validity).** No commands and nothing to schedule.
`belmont-pre` goes on PATH once M1 verifies and stays there until the M11 rollout; every normal run
in the observed repositories then records metrics as a side effect, at zero extra token cost and
with no interference.

**This now includes Belmont's own repository.** Its loop was previously pinned to Homebrew v0.11.0,
which contains no metrics code — so twelve milestones of real work would have gone unrecorded by
the feature that exists to measure exactly that. The pin is repointed to `belmont-pre`; isolation is
identical, because both are frozen. Pre-change status is guaranteed by the binary not moving rather
than by anyone remembering: skill prose is likewise frozen at v0.11.0 by master rule 2, and
`P0-M1-FIX-12` guarantees the metrics directory is ignored from the `auto` path, so no
per-repository setup step is needed.

This arm supplies the real **overhead-to-work ratio** that the seeded figure must be translated
through — the small-work design deliberately over-weights overhead, so quoting the seeded number
raw would overstate the real-world reduction.

**Controls — all three are acceptance-relevant, not method trivia. None of the three is
implementable against the code as it stands; that is what M12 exists for.**

- **Pin exact model IDs.** Never the drifting `opus` / `sonnet` aliases in `modelTiers`, or a
  vendor-side model change masquerades as your improvement. *(`P0-14`: generalise the layered
  resolution chain that already serves pi and opencode to every tool. Record `model_requested`
  **and** `model_served` — recording only what was asked for cannot see a vendor serving something
  else, which over a months-long passive window is the whole confound.)*
- **Keep `cache_read` and `cache_creation` separate** in every record, so prompt-cache warmth is
  inspectable after the fact rather than baked into a total. Cache hit rate is both a confound
  here *and* one of the things the feature improves, so it cannot simply be factored out.
  *(Already satisfied by `P0-2`; the fields exist.)*
- **Publish the completion rate beside every median.** A run that terminated cheaply without
  finishing flatters a median — which is exactly what the zero-usage records surfaced by
  `P0-M1-FIX-12` are. *(`P0-15`: nothing currently records whether a run's milestone reached `[v]`.
  A run counts as complete only at `[v]`. **Failed runs are never retried** — they count in the
  denominator and drop out of the ratio, because a retry silently repairs the number this control
  exists to expose.)*

**Analysis** *(`P0-20`, one code path for the pilot and the M11 verdict)*: per-pair log-ratios,
blocked by task and never pooled. Point estimate is the **sample median** of per-pair reductions,
which is what criterion 26 literally names as the pass/fail rule. Interval is the **exact Wilcoxon
signed-rank / Hodges–Lehmann** interval — stable at n = 6–20 where a percentile bootstrap is not,
and matching the signed-rank calculation that sized the suite.

**When sample size and wall-clock conflict, sample size wins.** Cut the seeded work first, then
verify rounds, then the phases measured; the pair count gives last, and if it gives, the report
says so.

Record per-pair figures, not just aggregates, so M11/P3-3 can compute a confidence interval.
Append to `baseline.json` under `per_milestone_cost_baseline` with the seed SHA, the pinned model
IDs and the Belmont commit per arm, then set `status` to `CAPTURED`.

**Until that happens, M11/P3-3 can compare the read-path half of the success criteria and must
report the cost half as unevidenced rather than estimating it.** The PRD is explicit that
instrumentation reports nothing rather than guessing; a baseline is the last place to break that
rule, because every later claim is stated against it.

**Records that exist but are not the baseline.** Six records were captured from the withdrawn
repo-3 runs (`~/repo-3/.belmont/metrics/feat-004.jsonl`). They are real
measurements of real work and are kept for reference, but they are **not** the baseline and must
not be presented as one: no milestone reached `[v]`, and two are zero-usage phases from
session-limit exits. Nothing in `baseline.json` derives from them.

*(Count corrected 2026-08-19: this said seven, as did `PROGRESS.md`. The file holds six —
`wc -l` on it returns 6.)*

**What they do tell us, and it matters for scheduling.** The four non-zero records give the only
measured per-phase figures in existence: implement 27–38 min, verify 5–18 min, so **43–45 minutes
per milestone** on real code work, with cost dominated by cache-read volume (4.1M and 6.9M tokens
on the two implement phases). The seeded design does trivial work, so implement should collapse —
but by how much is unknown until `P0-19`'s pilot measures it, and at 24–30 serial runs the answer
decides whether the M11 block is one sitting or several. **Sizing the M11 block is the pilot's
second job**, and it is why the cut order above is decided in advance rather than improvised.

## Reproducing the read-path half

```bash
# Pin the tree first, or you are measuring today's estate, not the baseline.
# The revisions are in §What this was taken against.
git -C <repo> rev-parse HEAD          # must match the recorded revision
git -C <repo> checkout <recorded-revision>

belmont status --root <repo> --feature <slug> --color never | wc -c   # text
belmont status --root <repo> --feature <slug> --format json   | wc -c   # json
wc -c < <repo>/.belmont/features/<slug>/PROGRESS.md                     # raw

# The pin belongs in this block, not only at the top of the document
# (P0-M1-FIX-26(e)): on any machine but the one that captured this,
# `belmont` is not the build these figures were taken with.
belmont version                       # must be the pinned build named in
                                      # §What this was taken against
```

**Aggregate rows are derived, not measured, and the derivation is here because it was not before**
(`P0-M1-FIX-26(a)`). The three commands above produce per-feature figures only; median, p90 and the
totals in §Read-path cost come from those. `total` is the sum, `max` the largest, and median/p90 are
`sorted[int(p*n)]` — zero-based, no interpolation, the rule `cmd/belmont/percentile` implements:

```python
s = sorted(f["progress_bytes"] for f in baseline["read_path_baseline"][repo]["features"].values())
median = s[min(int(0.50 * len(s)), len(s) - 1)]
p90    = s[min(int(0.90 * len(s)), len(s) - 1)]
```

Run against `baseline.json` this reproduces every aggregate row in this document exactly. Without the
rule written down, P0-3's "reproduce from the documented commands" criterion could not be met, and
M11/P3-3 would have compared two statistics that are not the same statistic.

> **`<repo>` and `<slug>` are aliases here, and this block is not runnable as written**
> *(`P0-M1-FIX-24`, 2026-08-19)*. This repository is a public fork and rule 7 of the master
> TECH_PLAN forbids product identifiers in it, so every repo and feature name above and throughout
> this file is an opaque identifier. Resolve them through
> `~/belmont/private/estate-alias-map.json` in the private planning workspace — that map is the
> only place the mapping exists, and it is deliberately not in this repository. Substituting real
> paths is the last step before running, never something committed back.
