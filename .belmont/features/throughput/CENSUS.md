# Extraction census — every feature directory, all five repos

*Produced by `throughput` P0-4 on 2026-08-18 with `belmont extract --dry-run`.*
*Re-run and corrected on 2026-08-18 by `P0-M1-FIX-2` — the first run omitted one of the five*
*repos. See §Correction below; the withdrawn figures are named there rather than deleted.*
*Machine-readable companion: `census.json` (all 82 registers). Nothing was written or extracted —*
*the move itself is M3/P1-1.*

## Correction — this census's own denominator was wrong (2026-08-18)

**Withdrawn.** The first run of this document said:

> *"The PRD's phrase 'the other 83' does not match disk. The real split is 138/65/68."*

That claim is wrong and is retracted. The PRD was right; the correction was the error.

**What caused it.** The first run walked `repo-3`, `repo-4`, `repo-5`, `repo-1` and
`repos/belmont`. That is five roots, but not *the* five: it substituted the Belmont fork for
`repo-2`, one of the five repos the PRD names, which was never measured — 31 feature
directories and 18 live registers. `runCensus` returns no error for a root it cannot read
(`os.IsNotExist → continue`), so an incomplete walk is indistinguishable from a complete one:
no error, no warning, just a smaller number that looks like a finding. Every one of
138 / 65 / 68 was an artefact of the omission, and the document then used them to contradict
the spec before its own coverage had been checked.

**The reconciliation.** The PRD's Success Criteria figure — *"all 139 existing feature registers
keep working untouched (measured 2026-08-18: 82 active + 57 archived across the five repos)"* —
reproduces **exactly**, on both halves, once all five repos are walked:

| PRD | Measured here | |
|---|---:|---|
| 82 active | **82** | live `PROGRESS.md` files across the five repos |
| 57 archived | **57** | registers held under `repo-1/.belmont/features/_archived/` |
| 139 registers | **139** | 82 + 57 |

The 57 are a *different population* from the 81 directories this census counts as archived. The
81 were summarised into an `ARCHIVE.md` by `/belmont:cleanup` and their registers deleted; the
PRD's 57 are intact registers moved wholesale under `_archived/`, all of them in `repo-1` and
nowhere else. Both counts are correct about different things, and the first run's "68 archived"
was neither.

One residual, stated so it is not later mistaken for a second defect: §P0-4's phrase *"the other
83"* refers to those active registers, of which two were already measured in the composition
analysis — so the strictly exact phrasing is "the other 80". That is a wording imprecision of
three against a measured 82. It is not the denominator error this document asserted, and it is
not a reason to change the PRD's Success Criteria figure, which is exact.

## The denominator, stated

**Scope: the five repos the PRD §Rollout names, and only those.**

| Repo | Root | Feature dirs | Live registers |
|---|---|---:|---:|
| repo-1 | `/Users/benlavender/repo-1` | 11 | 10 |
| repo-5 | `/Users/benlavender/repo-5` | 70 | 11 |
| repo-4 | `/Users/benlavender/repo-4` | 17 | 14 |
| repo-2 | `/Users/benlavender/repo-2` | 31 | 18 |
| repo-3 | `/Users/benlavender/repo-3` | 39 | 29 |
| **Total** | | **168** | **82** |

**The Belmont fork (`/Users/benlavender/repos/belmont`) is deliberately excluded**, and its
exclusion is why this run's totals differ from the first one's in both directions. It is the tool
repository, not one of the five deployments the rollout covers, and its only feature directory is
this feature's own register — including it would have the census measure the document that
records it. Including it would make the denominator 169 directories / 83 live registers; no byte
total is quoted for that variant, because a register that grows every time this task is written up
has no stable size to quote.

**Counting rules, so the numbers can be checked:**

- A **feature directory** is an immediate subdirectory of `<root>/.belmont/features/` whose name
  does not begin with `.`. There are **168**.
- A directory is **live** if it holds a `PROGRESS.md` at its own top level. There are **82**.
  **Every byte figure in this document is over those 82 registers only.**
- **86 hold no register.** 81 of those carry an `ARCHIVE.md`; 5 carry neither (`repo-4` has three
  design-only directories, and `repo-1` and `repo-3` each have an `_archived/` container).
- **Archived directories are counted in the 168 and contribute no bytes**, because they hold no
  register to measure.
- **The walk is one level deep and does not descend.** `repo-1/.belmont/features/_archived/` is
  therefore one directory with no register — not the 93 directories and 57 registers inside it.
  Those 57 are the PRD's "57 archived": intact registers held out of the live estate. No figure
  here includes their bytes.
- The walker reads `<root>/.belmont/features/` directly and **never globs for `PROGRESS.md`**.
  Globbing double-counts worktree copies — `repo-4` alone carries a 1,293,057-byte copy of a
  register already counted.

## What extraction would yield

| | Today | After extraction |
|---|---:|---:|
| Total | 5,372,973 B | 3,623,280 B |
| Detail moved | — | 1,749,693 B (32.6%) |
| Median register | 16,196 B | 16,075 B |
| p90 | 71,379 B | 71,379 B |
| Largest | 1,860,979 B | 944,773 B |

Extraction removes **32.6%** of all register bytes across the estate.

The first run reported 34.8% over its smaller walk. The rate fell because `repo-2`'s 18
registers add 386,620 B of which only 7,519 B (1.9%) is indented detail — its registers are
already nearly all index. The absolute quantity of detail available to move barely moved
(1,744,231 B → 1,749,693 B); the denominator grew.

## The open question, answered

> *Does the extraction census find any feature whose index alone exceeds 25,000 tokens?*

**Yes — 5 of them.** Seven registers exceed the 100 KB ceiling today; extraction brings only
two of those under it.

| Feature | Today | After extraction | Detail moved |
|---|---:|---:|---:|
| `repo-4/feat-075` | 1,860,979 | **691,323** | 62.9% |
| `repo-3/feat-015` | 1,022,749 | **944,773** | 7.6% |
| `repo-3/feat-070` | 247,361 | **106,360** | 57.0% |
| `repo-4/feat-031` | 129,354 | **129,354** | 0.0% |
| `repo-3/feat-058` | 114,941 | **114,941** | 0.0% |

**This answer does not depend on the repo that was missing, and that was checked rather than
assumed.** `repo-2`'s largest register is 86,380 B, below the 100 KB ceiling before
extraction and therefore below it after. It contributes no entry to either list: the same seven
are over the ceiling today and the same five remain over it after extraction, with and without
`repo-2`. The escalation in `[!] P0-4a` rests on a number the omission did not touch.

### Why the estimate was wrong — the finding that matters

The pre-estimate assumed size implies narrative, and applied the worst file's 62% detail ratio
to every file. It does not hold. **Three of the five gain essentially nothing from extraction —
0%, 0% and 7.6%**:

| Feature | Indented lines | Detail available to move |
|---|---:|---:|
| `repo-4/feat-031` | **0** / 0 B | **0 B** of 129,354 (0.0%) |
| `repo-3/feat-058` | **0** / 0 B | **0 B** of 114,941 (0.0%) |
| `repo-3/feat-015` | 167 / 84,593 B | 77,976 B of 1,022,749 (7.6%) |

*An indented line is a non-blank line beginning with a space or tab, and every byte figure here
counts each line plus its newline, as `censusFeature` does. Indentation is the upper bound on what
extraction could move; what it does move is smaller, because only a task's own body is detail —
`censusFeature` claims lines via `taskBodyEnd`, so an indented line that sits under a heading
rather than under a task stays in the index.*

**Two of the five carry zero indented lines — not three.** The remaining two over-threshold
registers have plenty: `repo-4/feat-075` has **11,832** indented lines
(1,209,680 B; 62.9% moved) and `repo-3/feat-070` **533** (140,957 B; 57.0% moved).
What the three above share is not an absence of indentation but an absence of *yield*.

> **CORRECTED by `P0-M1-FIX-6` (2026-08-18).** This paragraph previously read *"Three of the five
> contain no indented lines at all"* and then contradicted itself a sentence later by quoting an
> indented-byte figure for one of the three. Only two are at zero. The load-bearing claim — three
> of the five gain essentially nothing from extraction — is what the evidence supports and is
> restated above; no total in this document changes. All five indented-line counts were
> **re-derived from the registers on disk**, not restated. Two byte figures withdrawn with the
> sentence: `repo-3/feat-015`'s 335 task head lines measure **805,397 B**, not 795,799,
> and its indented lines **84,593 B**, not 83,500. Neither of the withdrawn pair reproduces from
> any definition of the terms; the pair above reproduces from the counting rule stated here.

`repo-3/feat-015` is still the clearest case: of its 1,022,749 bytes, **805,397 are task
head lines** and only 84,593 are indented — and extraction moves less again (77,976 B), because
only 32 of its 335 tasks have a body at all. Its longest single task line is **11,542
characters**. The register is not a document with narrative beneath its tasks — it is a document
whose tasks *are* the narrative, written on one line each.

Extraction is still the right change: it removes 32.6% of all register bytes across the estate,
and 62.9% of the worst single file (`repo-4/feat-075`, 1,860,979 → 691,323 B),
which is the number the PRD's composition analysis predicted. But it does not, on its own, bring
every register under the ceiling.

## Consequences for the plan

1. **M4's read path becomes a prerequisite of M3, not its successor.** This is the branch the
   PRD's open question defined: *"If one does, the read-path work in M4 stops being an
   optimisation and becomes a prerequisite for M3, and the wave structure changes."* Five do,
   with or without `repo-2`. Restructuring milestones is a tech-planning action, not an
   implementation one, so this is recorded and escalated rather than applied — see `[!] P0-4a`
   in `PROGRESS.md`.
2. **M5/P1-8 ("reject oversized writes at the source") is load-bearing, not defensive.** The
   400-character task-line limit is the only mechanism in the plan that would have prevented
   `feat-015`, and the census shows three registers whose size is entirely a task-line
   problem that no amount of extraction addresses.
3. **P3-1's migration target list is these seven**, not the four the PRD assumes — and for three
   of them migration alone will not be enough. The complete walk adds none: `repo-2` has
   no register over the ceiling.

## Reproducing this

```bash
belmont extract --dry-run \
  --root /Users/benlavender/repo-1 \
  --roots /Users/benlavender/repo-5,/Users/benlavender/repo-4,/Users/benlavender/repo-2,/Users/benlavender/repo-3 \
  --format json
```

`--dry-run` is required, not optional: this release ships the census only.

Paths are absolute and unabbreviated on purpose — the roots after the first comma are not a
separate shell word, so `~` in them is never expanded. A root that cannot be read is still skipped
silently by `runCensus`, which is what allowed the original omission; making it fail loudly is
`P0-M1-FIX-7` and is not fixed here.
