# Next session

Written 2026-08-08. The job below is self-contained — it assumes no memory of
the session that wrote it. Remaining proposal work is listed at the bottom.

---

## The job — implement 0003

[`0003-design-quality-without-figma.md`](0003-design-quality-without-figma.md), rev 7. Read it in
full; it is long but it is the spec. It has survived six adversarial review
passes and 53 confirmed fixes, so treat its reasoning as load-bearing rather
than as a draft to improve.

**Short version.** Three states exist and only one is broken:

| Feature has | Today | Status |
|---|---|---|
| No user interface | nothing to design | fine — and now genuinely skipped, see below |
| UI **and** Figma URLs | design-agent extracts exact specs | works |
| **UI and no Figma** | four bullets that all defer | **this is the job** |

0003's answer is to derive a **Design Contract** once per feature, interactively,
in `/belmont:tech-plan` — where a human is already reviewing — rather than
per-milestone inside a sub-agent. Downstream agents consume it; verification
gets an objective standard to check against.

---

## What changed under it since rev 7 was written

Rev 7 was written before PR #26. **Six things it says are now stale.** None
invalidate the design; all need repointing before you start.

### 1. The design agent now skips — but only for the row 0003 calls "fine"

PR #26 made `/belmont:implement` Phase 2 conditional. It skips when **none** of
the active tasks has a Figma URL, a reference image, or **visible UI**.

Read that condition carefully, because it is easy to misread as pre-empting
0003. It does not:

- **No UI at all** → skipped. This is 0003's first row, and #26 implemented it.
- **UI, no Figma** → the `visible UI` clause fires, so the agent **still runs**.
  This is 0003's broken row, untouched.

So 0003's §1 claim that "the design phase is a paid no-op" is now true **only**
for the UI-and-no-Figma case. The backend case is fixed. Correct that sentence
rather than repeating it.

**What you must not do:** widen the skip so a UI-no-Figma milestone stops
running the agent. Once the Design Contract exists, the condition should
probably gain a fourth trigger — *run when a Design Contract exists* — so a
contract can never be silently ignored. Decide that explicitly.

### 2. `implement.md` Phase 2 has new prose sitting exactly where 0003 edits

0003 restates Phase 2's purpose as contract consumption. That paragraph now has
a skip rule, a placeholder block and an "if in doubt, run it" line immediately
above it. Merge, do not replace — the skip is measured and load-bearing.

### 3. `verify.md` Step 1b and both dispatch prompts already changed

0003 adds contract collection at Step 1b and carries it into the dispatch
prompt. #26 already edited **four** sites in that file: Setup, Step 1b, and both
sub-agent dispatch prompts. Two of those are 0003's exact targets.

Preserve the `[Not populated —` **prefix match**. #26 generalised it from the
lightweight-mode wording so it covers both reasons a section can be empty. A
rewrite that goes back to matching exact wording silently breaks the design-skip
path.

### 4. The eval harness exists — use it

`cmd/belmont/eval_harness_test.go`, build tag `eval`, six fixtures in
`cmd/belmont/testdata/eval/`. `go test -tags eval ./cmd/belmont` runs Tier 1
free and in CI.

**0003 should add a fixture**: a feature with visible UI and no Figma. Today
nothing in the suite covers that state, which is the exact state 0003 exists to
fix. Follow the existing shape — inert content plus `commits.txt`, materialised
into a real repo in `t.TempDir()`.

Read [`knowledge/meta/evals.md`](../../knowledge/meta/evals.md) first. It records
what the tiers can and cannot license, and the controls each assertion was
checked against. **Tier 1 cannot license a prose change** — a stub agent never
reads a `SKILL.md`. 0003 is prose-only, so Tier 1 will tell you nothing about
whether it works.

### 5. The re-baseline 0003 owes is a different measurement now

§11 says 0003 owes 0004's Tier-2 re-baseline. **0004 did not use the Tier-2
gate.** It was set up, started, then abandoned mid-run — it drives
`executeLoopAction`, the headless path, to license a change whose consumer is
interactive. What #26 actually measured was **four `/belmont:loop` runs, two per
arm, compared from session transcripts**.

So the obligation you inherit is: re-run that comparison after the contract
lands, and report whether the contract's added content erodes #26's reduction.
The method is in #26's Evidence section. Two things it learned the hard way:

- **Pin the source tree.** `runInstall --source` installs whatever prose is in
  the repo *at run time*, so editing skills mid-run compares the new prose
  against itself. Use a `git worktree` at a fixed commit for the baseline arm.
- **Read the transcripts, not `/cost`.** `~/.claude/projects/<encoded-path>/<session>.jsonl`,
  summing `usage` per turn. `/cost` rounds hard. Note that transcripts record the
  **main session only** — sub-agent usage is not in them.

### 6. Dependencies and line numbers

- **0006 is merged** (PR #21). §5's "Requires 0006" for `plugin/` is satisfied.
- **0005 is merged** (PR #23). Its `main.go` split is done, so 0003's banner
  about pre-0005 line numbers applies in full: **every `main.go:NNNN` citation is
  dead.** `main.go` is now 1,522 lines across 22 files in the same package.
  Re-resolve with `grep -rn 'func <name>' cmd/belmont/`.
- **0004 is PR #26**, open. 0003 rebases onto it. Shared files: `_src/next.md`,
  `_src/implement.md`, `_src/verify.md`, `implementation-agent.md`,
  `_src/references/implement-milestone-template.md`, `plugin/`, `AGENTS.md`.
  Resolve `plugin/` conflicts by regeneration, never by hand.

---

## How to test it for real

0003 is **prose only**. Nothing in Go changes, so the unit tests and Tier 1 will
stay green whether or not the change works. That is the trap.

The only evidence that counts is a run against a feature with **visible UI and
no Figma**. Two matched repos, identical feature, differing only in skill prose,
each in a fresh Claude Code session:

1. Does `/belmont:tech-plan` actually produce a Design Contract, with all six
   sections populated rather than headed-and-empty?
2. Does the implementation honour it — tokens used, states covered, motion
   present?
3. Does `/belmont:verify` **fail** an implementation that violates the contract?
   That third one is the whole PR. A gate that cannot fail is not a gate, and
   `verification-agent.md:131` currently lets visual verification pass on
   acceptance criteria alone whenever no references exist.

Build a deliberately non-compliant implementation and confirm verify catches it,
the same way `failing-acceptance` works in the eval fixtures.

## When you're done

Small PR, and read the merged ones for the bar — #21, #23, #26. The pattern
Blake merges is: **problem first, result last.** Quantify the problem, say who
is affected and why nobody noticed, show the mechanism, then the fix, then the
evidence. State plainly what you did not test.

---

## Rules that caused problems last time

- **Measure, do not calculate.** In #26 the estimate went 68% → 12% → −32%, and
  only the last came from a measurement. Byte-counting the files a skill reads
  consistently over-predicts, because a session's cost is dominated by cache
  reads of accumulated context, not by any one file. If you publish a number,
  publish where it came from.
- **Verify every number before it ships.** Two totals were published in #26 that
  had been computed rather than extracted; both were wrong. Re-derive from source
  before pasting.
- **Deliberately break each test and confirm it fails.** #26 ran six controls;
  one did not fail, and the reason was a weak *fixture*, not a weak assertion.
  You would not have found that by reading.
- **Check the document's own claims against the code.** Rev 7 is careful, but the
  previous handoff asserted a reproduction was written up in a knowledge entry
  that did not mention it. Open the file.
- **Belmont runs two ways** — headless `belmont auto` and interactively. Any
  change has to work both ways, and 0003 touches `tech-plan`, which is
  interactive by construction.
- After changing Go code: `go build ./cmd/belmont`, `go test ./cmd/belmont`,
  `go test -tags eval ./cmd/belmont`, `go vet ./...`, `go vet -tags eval ./cmd/belmont`,
  `staticcheck ./...`, `gofmt`. All currently clean — keep them clean.
- Regenerate and verify: `./scripts/generate-skills.sh --check`, then
  `./scripts/generate-plugin.sh <version> && git diff --exit-code plugin/`.
  Use `git diff --exit-code`, **not** `--check` — since #21, `--check` ignores
  the `version` field and will pass over a re-stamped `plugin.json`.
- Don't report context remaining. It has been wrong every time.

## Where things are

- `main` is at **v0.10.16**.
- **PR #26 open** — 0004. Lazy Setup blocks in implement/verify/next, the design
  agent skip, the `/belmont:loop` status call, and the eval harness. Measured
  −18.5% tokens from the Setup change alone, −32.1% with the design skip.
- **PR #25 closed** as deprioritised — a real parallel-wave state-loss fix, kept
  on branch `fix/wave-merge-state-loss`, with issue **#24 left open** pointing at
  it. Auto-mode only; reopen if parallel waves start mattering.
- **PRs #21 and #23 merged.** CI exists and is green on every push and PR.
- The `/belmont:loop` comparison repos from #26 are at `~/belmont-loop-BEFORE`
  and `~/belmont-loop-AFTER` if you want the method; delete them once #26 merges.

## Remaining proposal work

| # | State |
|---|---|
| [0003](0003-design-quality-without-figma.md) | rev 7. **This session's job.** Two adversarial passes done |
| [0004](0004-context-budget-with-evidence.md) | rev 5. Shipped as PR #26 — see §5 above for where it departed from the spec |
| [0005](0005-maintainability.md) | **Done** — PR #23 |
| [0006](0006-plugin-generator-correctness.md) | **Done** — PR #21 |

**0005 is stale in one place** if you touch it: §2.1 quotes a 398-declaration
baseline; the real figure from the built tool is 444, because `declsum` hashes
`var`/`const` groups per-spec. §2.1 also omits that output normalisation is
required, and the third control (statement reorder) that was added.

**0004 departed from its own spec in two ways**, both recorded in #26's
description: the Tier-2 N≥3 gate was dropped in favour of interactive
`/belmont:loop` measurement, and the scope grew to include the design-agent skip,
which turned out to be the larger of the two savings.
