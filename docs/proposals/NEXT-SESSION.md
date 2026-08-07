# Next session

Written 2026-08-07. The job below is self-contained — it assumes no memory of
the session that wrote it. Remaining proposal work is listed at the bottom.

---

## The job — fix issue #24

Read the issue first:

```bash
gh issue view 24 --repo blake-simpson/belmont
```

Short version: in a parallel wave, the second milestone's merge overwrites the
first one's state. A task ends up marked `[ ]` while its code is merged and its
commit is in history. Belmont has been silently losing verified marks in
parallel mode for a long time. It is **not** a regression from any recent
change — reproduced identically against an older binary.

### The cause

`syncFeatureStateAfterMerge` in `cmd/belmont/worktree.go` does:

```go
os.RemoveAll(dstFeature)
copyDir(srcFeature, dstFeature)
```

It runs after every merge, so in a wave it is last-writer-wins on the whole
feature directory. M3's merge replaces master's state with M3's worktree copy,
which still has M2 as `[ ]` from fork time.

Two call sites, and only one is wrong:

- `auto_parallel.go:135` — whole-feature merge. Wholesale replace is **correct**
  here. Leave it alone.
- `auto_parallel.go:565` — milestone merge in a wave. This is the bug. The
  caller has `milestoneID` in scope.

### The fix, sketched but not written

Take a milestone ID. Empty means whole-feature replace (current behaviour, for
the `:135` caller). Non-empty means:

1. Do **not** `RemoveAll`. Master may hold files from siblings that already
   merged, and deleting them is part of the bug.
2. Copy everything across **except** `PROGRESS.md`.
3. For `PROGRESS.md`: start from **master's** copy, which is the accumulated
   truth, and apply only that milestone's task marks from the worktree.

For step 3, `parseMilestones()` is in `state.go` and the `task` struct
(`ID`/`Name`/`Status`/`MilestoneID`) is in `types.go`. Line-based editing is
safer than re-rendering — find the line carrying each task ID and swap its
`[.]` marker, rather than regenerating `PROGRESS.md` and risking formatting
loss.

**Watch out for one thing.** A worktree can *add* tasks to its own milestone —
the verify phase writes follow-up tasks as plain `[ ]` entries. If a task ID in
the worktree's milestone is not in master, it needs inserting, not dropping.
Losing follow-ups would be a new bug in place of the old one.

Treat the sketch as a starting point, not a specification. If you find a better
shape, take it — but say what changed and why.

### Do not go further than the fix

`recoverMerge` does not call `syncFeatureStateAfterMerge` at all, so tracked
state never comes home on that path. It is a real defect and it shares a root
cause. It is **not** part of this job. Adding the call naively makes things
worse — a recovered worktree comes from a failed merge and may be days stale,
so it would overwrite main with old state. Leave it. It is already documented
as an open gap in [`knowledge/auto-mode/worktree-state-isolation.md`](../../knowledge/auto-mode/worktree-state-isolation.md).

### How to test it for real

There is a reproduction fixture in issue #24 — two files, no dependencies,
two minutes to set up. Use it.

**One trap:** `--max-parallel` does nothing on its own. `runAutoCmd` only takes
the parallel path when a milestone declares `(depends: M1)`. Without that you
get no worktrees, the bug does not appear, and your test proves nothing. The
fixture in the issue has the dependency for this reason. Written up in
[`knowledge/auto-mode/parallel-wave-orchestration.md`](../../knowledge/auto-mode/parallel-wave-orchestration.md).

A live run costs API budget and takes a few minutes per milestone. Prefer a
unit test for the merge logic, and use one live run at the end to confirm the
whole thing works.

### When you're done

Small PR. Blake reviews these and has been merging them quickly. Explain the
mechanism, show before/after on the fixture, and say plainly what you tested
and what you did not.

---

## Rules that caused problems last time

- **Check facts against the actual code before relying on them.** If a document
  says "file X line 40 does Y", open it and look. Confident claims have turned
  out to be invented — tool version numbers, line numbers, a grep that was
  described but never run, and a "not caused by" inherited from an earlier
  commit message without re-checking.
- **When you write a test, deliberately break the thing it tests and confirm
  the test fails.** A test written last session passed even with the code it
  guarded disabled, and was thrown away. A test that cannot fail is worse than
  no test.
- **Run a differential when a smoke test surfaces something ugly.** Build a
  binary from the pre-change commit and run the identical fixture. That is what
  proved #24 predated the `main.go` split, and it took ten minutes.
- After changing Go code: `go build ./cmd/belmont`, `go test ./cmd/belmont`,
  `go vet ./...`, `staticcheck ./...` (all currently clean — keep them clean),
  `gofmt`.
- Belmont runs two ways: automatically, and by a user typing a command. Any
  change has to work both ways.
- Don't tell the user how much context you have left. It has been wrong every
  time.

---

## Where things are

- `main` is at **v0.10.16**, clean.
- **PR #23 merged 2026-08-07**: added CI, deleted nine staticcheck findings,
  removed the no-op `writeWorktreeGitExcludes`, and split `main.go` from 12,613
  lines into 22 files in the same package. CI now runs on every push and PR and
  is green.
- **PR #21 merged**: fixed the plugin generator that had been publishing four
  of six agents as 0-byte files since June.
- `scripts/declsum` is on main — proves a refactor moved code without changing
  it. Probably not needed for this job.

---

## Remaining proposal work

| # | State |
|---|---|
| [0003](0003-design-quality-without-figma.md) | rev 7. Two adversarial passes done. Not started |
| [0004](0004-context-budget-with-evidence.md) | rev 5. Not started. Its CI dependency now exists, so the eval entry point can be wired in as a real line |
| [0005](0005-maintainability.md) | **Done** — shipped in PR #23 |
| [0006](0006-plugin-generator-correctness.md) | **Done** — shipped as PR #21 |

Build order was `0005 → 0004 → 0003`. 0005 is done, so **0004 is next** once
#24 is fixed.

**0005 is stale in one place** and it is worth correcting if you touch it: §2.1
quotes a 398-declaration baseline. The real figure from the built tool is 444 —
`declsum` hashes `var`/`const` groups per-spec, so the counts are not
comparable. §2.1 also does not mention that output normalisation is required
(without it the printer's position-derived line breaks make every legitimate
move read as an edit), nor the third control that was added (statement
reorder). The accurate detail is in `scripts/declsum/main.go`'s commit message.
