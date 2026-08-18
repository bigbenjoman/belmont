# Extraction census — every feature directory, all five repos

*Produced by `throughput` P0-4 on 2026-08-18 with `belmont extract --dry-run`.*
*Machine-readable companion: `census.json` (all 65 registers). Nothing was written or extracted —*
*the move itself is M3/P1-1.*

## The denominator, stated

- **138 feature directories** across `repo-3`, `repo-4`, `repo-5`, `repo-1` and `repos/belmont`.
- **65 carry a live `PROGRESS.md`.** Every figure below is over those 65 only.
- **73 have no register**, 68 of them archived to `ARCHIVE.md` by `/belmont:cleanup`.

The PRD's phrase "the other 83" does not match disk. The real split is 138/65/68, and it is stated here because a census whose denominator is
unstated cannot be checked.

The walker reads `<root>/.belmont/features/` directly and never globs for `PROGRESS.md`.
Globbing double-counts worktree copies — `repo-4` alone carries a 1,293,057-byte copy of a
register already counted.

## What extraction would yield

| | Today | After extraction |
|---|---:|---:|
| Total | 5,005,137 B | 3,260,906 B |
| Detail moved | — | 1,744,231 B (34.8%) |

Extraction removes **34.8%** of all register bytes across the estate.

## The open question, answered

> *Does the extraction census find any feature whose index alone exceeds 25,000 tokens?*

**Yes — 5 of them.** Seven registers exceed the 100 KB ceiling today; extraction brings only
two of those under it. The pre-planning estimate was three, and it was wrong in both magnitude
and mechanism.

| Feature | Today | After extraction | Detail moved |
|---|---:|---:|---:|
| `repo-4/feat-075` | 1,860,979 | **691,323** | 62.9% |
| `repo-3/feat-015` | 1,022,749 | **944,773** | 7.6% |
| `repo-3/feat-070` | 247,361 | **106,360** | 57.0% |
| `repo-4/feat-031` | 129,354 | **129,354** | 0.0% |
| `repo-3/feat-058` | 114,941 | **114,941** | 0.0% |

### Why the estimate was wrong — the finding that matters

The pre-estimate assumed size implies narrative, and applied the worst file's 62% detail ratio
to every file. It does not hold. **Three of the five contain no indented lines at all**:

| Feature | Detail available to move |
|---|---:|
| `repo-4/feat-031` | **0 B** |
| `repo-3/feat-058` | **0 B** |
| `repo-3/feat-015` | 77,976 B of 1,022,749 (7.6%) |

`repo-3/feat-015` is the clearest case: of its 1,022,749 bytes, **795,799 are task head
lines** and only 83,500 are indented. Its longest single task line is **11,542 characters**. The
register is not a document with narrative beneath its tasks — it is a document whose tasks *are*
the narrative, written on one line each.

Extraction is still the right change: it removes 34.8% of all register bytes across the estate,
and 62.9% of the worst single file (`repo-4/feat-075`, 1,860,979 → 691,323 B),
which is the number the PRD's composition analysis predicted. But it does not, on its own, bring
every register under the ceiling.

## Consequences for the plan

1. **M4's read path becomes a prerequisite of M3, not its successor.** This is the branch the
   PRD's open question defined: *"If one does, the read-path work in M4 stops being an
   optimisation and becomes a prerequisite for M3, and the wave structure changes."* Five do.
   Restructuring milestones is a tech-planning action, not an implementation one, so this is
   recorded and escalated rather than applied — see `[!] P0-4a` in `PROGRESS.md`.
2. **M5/P1-8 ("reject oversized writes at the source") is load-bearing, not defensive.** The
   400-character task-line limit is the only mechanism in the plan that would have prevented
   `feat-015`, and the census shows three registers whose size is entirely a task-line
   problem that no amount of extraction addresses.
3. **P3-1's migration target list is these seven**, not the four the PRD assumes — and for three
   of them migration alone will not be enough.

## Reproducing this

```bash
belmont extract --dry-run \
  --root ~/repo-3 \
  --roots ~/repo-4,~/repo-5,~/repo-1,~/repos/belmont \
  --format json
```

`--dry-run` is required, not optional: this release ships the census only.
