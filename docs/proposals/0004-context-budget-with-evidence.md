# PR 2 — Context budget, with evidence

**Revision 5.** v2 dropped Optimisation B (its rationale was empirically false); v3 resolved the second review; v4 made the Tier-2 gate executable and the plugin check able to fail; v5 reverses the 0003 ordering and replaces the single Mode-B-inclusive baseline with a measure-then-re-baseline pair. Changes in §11.

**Type:** test harness (Go, test-only) + prose (three skill sources).
**Size:** harness dominates. Skill edits ~60 lines. **No non-test Go changes.**
**Sequencing:** depends on **0006** (its `--check` DoD is unsatisfiable until the generator is fixed) and lands **after 0005, before 0003** — build order `0005 → 0004 → 0003`, set by the author on 2026-08-06. Through v4 this PR required 0003 first so the baseline would include Mode B; that is reversed. The baseline is now measured **pre-Mode-B**, and a **re-baseline is owed once 0003 lands** (§2, §8, §10). **Blocks nothing** — with Optimisation B dropped, this PR no longer touches `main.go`, so the PR 2 / PR 3 conflict disappears.

---

## 1. What changed and why (read first)

v1 claimed steering at prompt position 0 destroyed a ~550-line stable cache prefix. **Both halves were wrong**, verified against the code and by measurement:

- `buildLoopPrompt` (`main.go:7165-7172`) emits `/belmont:implement --feature <slug>` plus a scope paragraph — **549 bytes, ~140 tokens**. The 575-line SKILL.md is resolved *downstream by the tool*; Belmont never assembles it.
- Prefixed vs suffixed steering against `claude -p` yields **identical** cache figures (13233 creation / 21689 read both ways). Cache breakpoints sit at the end of the user message, so intra-message reordering recovers nothing.

The only measurable effect is slash-command expansion — `claude -p "/x"` resolves in 1 turn at position 0 versus 3 when prefixed. That may be worth a future PR *with measurement attached*; it is not worth one on the strength of a theory. **Optimisation B is removed.**

The wider lesson is the one this PR is about: an optimisation without a measurement is a guess. Hence the harness leads.

## 2. Problem

**Cost.** Each phase is a cold shell-out — `executeLoopAction` runs `claude -p` per action (`main.go:6862`). A milestone costs implement → verify → optionally triage → fix-all → re-verify, so 4–5 cold starts. Each orchestrator reads its full Setup block before knowing which milestone it is on:

| Orchestrator | Eager reads | Location |
|---|---|---|
| `implement.md` | 7 — PRD, PROGRESS, TECH_PLAN, master TECH_PLAN, NOTES, master NOTES, models.yaml | `:18-25` |
| `verify.md` | **5** — PRD, PROGRESS, TECH_PLAN, master TECH_PLAN, models.yaml (v1 said four) | `:18-23` |
| `next.md` | 6 — identical pattern, identical "Still read the files above" sentence | `:46-55` |

Then implement fans out to 3 sub-agents and verify to 2, each re-reading PRD and TECH_PLAN independently — roughly seven full spec reads per milestone.

**Honest reach.** This PR's remedy touches **4 of those 7**. `verify.md` issues the read instruction inside both its sub-agent dispatch prompts, so fixing the orchestrator fixes those two as well. The 3 untouched are `implement.md`'s sub-agents — that is the deferred task-scoped spec extract (§8), and it is named here rather than left implied.

**Known addition from 0003 — measured later, not here.** Mode B adds design content on the sub-agent fan-out path this PR defers. Through v4 this PR landed after 0003 so the baseline would absorb that cost; the build order is now `0005 → 0004 → 0003`, so it does not. Consequences, stated plainly:

- The reduction reported by this PR is measured against a **pre-0003 milestone** and is therefore an **upper bound** on what survives once 0003 lands.
- A **re-baseline after 0003** is owed, and is in this PR's DoD (§8) as a follow-up obligation rather than a merge gate — it cannot run before the code it measures exists.
- 0003 is itself slated for a **rev 5 restructure** that moves design derivation out of `implement` Phase 2 and into `tech-plan` — one design pass per feature instead of one per milestone. If that lands as described, Mode B stops adding per-task cost to the fan-out path this PR defers, and the re-baseline may show no Mode B delta here at all. That is a reason the re-baseline is cheap to defer, not a reason to skip it: the restructure is not yet written, so its token profile is unknown.

**Only 3 of 16 skill sources have this pattern.** Fixing two would leave `next.md` as the sole outlier, which is why it is in scope.

**No way to prove a change is safe.** ~2,989 lines of unit tests across 8 files, plus a manual smoke test. Nothing measures whether a context reduction degrades agent output.

## 3. Rationale from the graph-engineering method

**Evals for what you run thousands of times; vibes for the rest (§6).** Boris Cherny's split, stated directly: for a workflow repeated "thousands, tens of thousands, hundreds of thousands, millions of times, you definitely want to have evals," because they let you swap in a new model and confirm the harness improved; for one-off product experience "vibes actually go pretty far." Belmont's auto loop is squarely the first case and has none.

**Stop pre-loading context (§5).** Anthropic moved away from spoon-feeding: customers are "moving more to skills… more to tools and more to MCPs because this way the model has a little bit more control over how to load this in." Belmont's eager Setup blocks are the pattern they moved past.

**Tokens must be spent productively (§2).** Boris frames multi-agent work as test-time compute with the caveat that "if you do it naively, you spend more tokens, but you don't get a better result."

**Optimise what proves it works (§9).** Measure first, cut second — which is also why v1's Optimisation B is gone rather than patched.

## 4. Design

### 4.1 Commit 1 — Eval harness (no behaviour change)

**Location (v1 blocker).** v1 said `cmd/belmont/eval/`. `main.go:5` is `package main`; Go rejects the import outright, every needed symbol is unexported (`parseMilestones:2523`, `computeWaves:8478`, `runScopeGuard:12061`, `snapshotProgress:11988`, `executeLoopAction:6862`), and `go test ./cmd/belmont` does not descend into a subpackage — so the DoD would have stayed green over a broken harness.

- File: `cmd/belmont/eval_harness_test.go`, `//go:build eval`
- Fixtures: `cmd/belmont/testdata/eval/`
- Entry point: `go test -tags eval ./cmd/belmont`

**Two tiers, because one cannot do both jobs.**

| | Tier 1 — offline | Tier 2 — live |
|---|---|---|
| Gate | build tag `eval` | tag + `BELMONT_EVAL_LIVE=1` |
| Drives | pure Go: wave computation, guard behaviour, prompt assembly, state parsing | a real tool via `executeLoopAction` |
| Asserts | deterministic outputs | final PROGRESS state across **N ≥ 3** runs |
| Licenses | nothing about prose | **Optimisation A** |

This split is the honest answer to a v1 flaw: an offline harness with a stub agent validates Go machinery only, and Optimisation A is *prose* a stub never reads. Say plainly in the PR description that **Tier 2 is what licenses Optimisation A**, and that four of the five Tier-1 assertion rows are already covered by `cmd/belmont/scope_guard_test.go` (719 lines) — Tier 1's value is regression-locking during PR 3's split, not novel coverage.

**Fixtures cannot be nested git repos (v1 blocker).** `git add -A` on a repo inside `testdata/` stages a gitlink (mode 160000) and a clone yields empty dirs; Belmont has no submodules. Store **inert content** — `PRD.md`, `PROGRESS.md`, `TECH_PLAN.md`, plus `commits.txt` describing a commit sequence — and materialise a real repo in `t.TempDir()` via `git init` and scripted commits. Document the mechanism in `knowledge/meta/evals.md`.

**Fixtures (five):**

1. `single-milestone-clean` — one milestone, three `[ ]` tasks
2. `mid-milestone` — mixed `[ ]` / `[x]`
3. `blocked-task` — one `[!]`, asserts the pause path
4. `multi-milestone-deps` — `(depends: M1)`, asserts wave computation
5. **`failing-acceptance`** — committed implementation violates one acceptance criterion. Expect: task stays `[x]`, a follow-up `[ ]` is added. **Without a negative fixture the suite cannot fail meaningfully** — on a clean-pass M1 a rigorous and a lazy verify produce identical PROGRESS bytes (`verify.md:175-176`).

Plus, from PR 1: a fixture asserting Mode B contract **structure** (deterministic even though quality is not).

**Determinism.** Assert state transitions, never prose. Every fixture PROGRESS.md must pass `belmont validate`, since `auto` lints at startup.

**No CI exists.** `.github/workflows/` contains only tag-triggered `release.yml`. Drop v1's "CI stays green offline" rationale — **PR 3 owns `ci.yml`**; this PR notes that its Tier-1 evals wire in there.

### 4.2 Commit 2 — Optimisation A: lazy Setup

Applies to `implement.md`, `verify.md` **and `next.md`**:

1. Read `PROGRESS.md` first. Select the milestone.
2. Read PRD sections for the selected milestone's task IDs.
3. Read `TECH_PLAN.md` only if the milestone touches architecture described there.
4. Read master `TECH_PLAN.md` only when the milestone is cross-cutting.
5. `models.yaml` unchanged — small, needed up front for tier resolution.

`verify.md` additionally prefers the archived `MILESTONE-*.done.md`, **but only when its sections are populated** — after any `/belmont:next` run the archive carries `[Not populated — lightweight mode skips the design agent]` (`next.md:114-118`), so the preference must be conditional. Fall back to the spec otherwise.

**Escape hatch, not prohibition.** Phrase as "read X first; fall back to Y if X is missing or insufficient." An agent that needs a canonical file must still be able to read it.

**The NOTES conflict must be resolved, not stepped around.** `references/implement-milestone-template.md:45-47` requires copying `{base}/NOTES.md` and `.belmont/NOTES.md` into MILESTONE's `### Learnings from Previous Sessions` whenever they exist, and `implement.md:52` makes writing that template mandatory. If the reads are merely deferred there is no saving; if they are dropped, Learnings goes empty and the `/belmont:note` → implement loop silently breaks — and the DoD's PROGRESS-state diff cannot see it. The template file is therefore **in scope**, and the decision must be explicit: keep NOTES eager (they are small and feed a mandatory section) and take the saving from TECH_PLAN / master TECH_PLAN instead. `implementation-agent.md:56-61` reads both NOTES files itself in Step 0b, so the loss is narrower than a full break — but narrow is not zero.

## 5. Scope

### In scope

| Component | Files |
|---|---|
| Harness | `cmd/belmont/eval_harness_test.go`, `cmd/belmont/testdata/eval/` |
| Optimisation A | `_src/implement.md`, `_src/verify.md`, `_src/next.md`, `_src/references/implement-milestone-template.md` |
| Plugin | Regenerate — `plugin/skills/{implement,verify,next}/SKILL.md` are git-tracked |
| Knowledge | **New** `meta/evals.md`, **new** `cross-cutting/context-budget.md` |

**Preflight correction.** v1 required checking overlap against open PRs #4–#7. Verified via `gh pr list`: **#4–#7 were closed unmerged on 2026-04-21, superseded by merged #8** ("skills: token-saver — MILESTONE coordinator + references/ convention"). The only open PRs are #10–#13 and none touches a `.go` file. Sequence against **#8's merged state**. **PR state as measured 2026-08-06:** #4–#7 closed unmerged (superseded by merged #8); #18 merged; #10–#13 **all closed unmerged**; the only open PR is **#21** (plugin generator fix), which touches `scripts/generate-plugin.sh` and `plugin/agents/` and is therefore disjoint from this PR's scope. **No open PR overlaps this work.** Re-measure with `gh pr list --state open` at branch time rather than trusting this paragraph — it has now been wrong twice.

**Shared file with 0003.** This PR's conditional-archive edits land at `_src/verify.md:110`/`:147`; 0003 inserts its Mode B block at `:114`. Four lines apart, inside default diff context, so they will conflict. Under the current order **this PR lands first**, so 0003 rebases onto it and folds its Mode B block around this wording — the reverse of what v4 said. Verify the line numbers at branch time; 0003's rev 5 restructure may move its insertion point out of `verify.md` entirely.

`steering.md` no longer needs amending — Optimisation B is gone, so both injection sites (`main.go:6887` and `executeTriageAction` at `:7026-7027`) are untouched, and the existing `steeringHeader()` marker (`:11317-11326`, `## URGENT — User steering (higher priority than NOTES.md)`) stands unchanged.

### Out of scope — deferred

1. **Task-scoped spec extract for sub-agents.** The remaining 3 of 7 reads. Needs Tier-2 measurement first.
2. **Default `models.yaml` tiering.** Deferred permanently, not pending evidence — the evidence already exists and points the other way. #18 (merged 2026-06-04) removed frontmatter pins in favour of session-model inheritance precisely because downgrading is a false economy: **Sonnet 4.x is 57% first-pass-correct vs Opus 4.x at 77%**, ~1.9× the rework rate, and Sonnet 5.x is no better. A rework here costs a triage → fix-all → re-verify cycle, so the per-token saving is erased several times over. **The token lever is input-side — progressive disclosure and lazy context loading, which is what this PR does.**

The tier axis is a barbell, not a gradient: **`high` or `low`, never `medium`.** Haiku is genuinely good for basic mechanical work — scanning, extraction, grep-and-read — and choosing it there is not a compromise. Sonnet is the false-economy tier on Claude, and Artificial Analysis (Aug 2026, Intelligence Index task at `max`) shows why it is dominated rather than merely poor value: **$1.72/task vs Opus 5's $2.34 — only 27% cheaper — while taking 9.5 min vs 7.8, i.e. 22% *slower*, at ~5.5 points lower intelligence.** It sits off the Pareto frontier; Opus 5 sits on it. Haiku by contrast is $0.22 and 3.4 min — cheap *and* fast, which is what makes `low` a real tier. A 27% discount cannot absorb a 1.9x rework rate. A future PR proposing `low` for a demonstrably mechanical agent is arguable on its own evidence; one proposing `medium` is not.
3. **Merging implement and verify into one process.** Confirmed real: `runScopeGuard` / `runEvidenceCheck` fire per `executeLoopAction` (`main.go:6881-6882`, `:6975-6976`, `:6985-6986`) and would need to fire per logical phase inside a merged run.

## 6. Both invocation paths

| Path | Impact |
|---|---|
| **Auto** | Both commits |
| **Interactive** | Optimisation A — same Setup block on a live `/belmont:implement`. Must be confirmed unchanged in *effect*: fewer files read, same milestone selected, same work done |
| **Plugin** | `plugin/skills/{implement,verify,next}/SKILL.md` tracked and shipped by `release.yml:66-70` |

## 7. Author smoke test

**Preconditions.** `requireCleanWorkingTree` (`main.go:9818`) aborts on any dirty/untracked path; `install` never commits. Commit after every install. Fixture PROGRESS.md must pass `belmont validate`.

**Step 0 — baseline.** v1's version was broken four ways (a `grep '^\['` that matched nothing because the stream prefix is `"\033[36m[%s][%s]\033[0m: "`; a branch never created; a missing `cd` back from `~/belmont`; and no reinstall, so it measured the *old* skills). Corrected:

```bash
cd ~/belmont && git checkout main && ./scripts/build.sh 0.0.0-smoke
cd ~/path/to/real-project && git checkout -b smoke/pr2-base
belmont install --source ~/belmont --no-prompt && git add -A && git commit -m "baseline install"
BASE=$(git rev-parse HEAD)
belmont auto --feature <slug> --from M1 --to M1
cp .belmont/features/<slug>/PROGRESS.md /tmp/pr2-progress-before.md
cd ~/belmont && git checkout -- plugin/   # build.sh re-stamps plugin.json; do not carry it into the branch
```
Record files read per orchestrator from the run log.

**Step 1 — harness green on unmodified main.**
```bash
cd ~/belmont && go build ./cmd/belmont && go test ./cmd/belmont && go test -tags eval ./cmd/belmont
```
A fixture failing here encodes a wrong expectation — fix the fixture, not the assertion.

**Step 2 — post-change, same starting point.**
```bash
cd ~/belmont && git checkout smoke/pr2 && ./scripts/build.sh 0.0.0-smoke
cd ~/path/to/real-project && git checkout -b smoke/pr2-after $BASE
belmont install --source ~/belmont --no-prompt && git add -A && git commit -m "post-change install"
belmont auto --feature <slug> --from M1 --to M1
diff /tmp/pr2-progress-before.md .belmont/features/<slug>/PROGRESS.md
cd ~/belmont && git checkout -- plugin/   # same cleanup after the second build
```
Expect: no diff in final task states; measurably fewer file reads. Branching from `$BASE` and reinstalling are both load-bearing.

**Step 3 — steering unchanged.** Optimisation B is dropped, so this is a *non-regression* check.
```bash
belmont auto --feature <slug> &
belmont steer --feature <slug> --message "Use the existing Button component, do not create a new one"
```
`--message` is required — positional text is silently discarded (`readSteeringInput`, `main.go:11590-11596`, recognises only literal `-`); with `$EDITOR` set it opens an empty template, unset it exits 1.
Expect `✓ injected → <path>` from the steer command, then `[STEERING] injected …` on the auto run's stderr as it consumes. Two different signals — do not conflate.

**Step 4 — interactive.** `git checkout -b smoke/pr2-interactive $BASE`, then `claude` → `/belmont:implement` → `/belmont:verify`. Same milestone, same tasks as Step 2.

**Step 5 — `next.md` path.** Force a FWLUP round so `actionFixAll` emits `/belmont:next`. Expect the lazy Setup to apply there too and the milestone to complete normally.

**Step 6 — other tools.** 6a: `--tool codex` auto. 6b: interactive `codex` → `$belmont` → `$implement`. Steps 2 and 4 are the Claude-both-modes check.

**Diagnostics.** If wall-clock regresses, re-run and take the second — the first may include MCP cold start.

## 8. Definition of Done

**Harness**
- [ ] `cmd/belmont/eval_harness_test.go`, `//go:build eval`, fixtures in `cmd/belmont/testdata/eval/`
- [ ] Entry point `go test -tags eval ./cmd/belmont` documented in `AGENTS.md` **Verify**
- [ ] Tier 1 offline; Tier 2 gated on `BELMONT_EVAL_LIVE=1`, N ≥ 3 runs
- [ ] PR description states Tier 2 licenses Optimisation A, and that Tier 1 overlaps existing `scope_guard_test.go`
- [ ] Fixtures stored as inert content + `commits.txt`, materialised via `git init` in `t.TempDir()`
- [ ] Five fixtures including `failing-acceptance`
- [ ] Every fixture PROGRESS.md passes `belmont validate`
- [ ] Asserts state transitions only — never prose
- [ ] Green on unmodified `main` before Commit 2

**Optimisation A**
- [ ] `implement.md`, `verify.md`, `next.md` all staged — no outlier left
- [ ] Archive preference conditional on populated sections
- [ ] Escape hatch present in all three
- [ ] NOTES / `implement-milestone-template.md` conflict explicitly resolved, not deferred
- [ ] Exact per-orchestrator/per-sub-agent baseline table in the PR description (implement 7, verify 5, next 6)
- [ ] Measured reduction recorded; states plainly it reaches 4 of 7 reads
- [ ] Reduction reported as a **pre-0003 upper bound**, in those terms, in the PR description — the baseline does not include Mode B under the `0005 → 0004 → 0003` order
- [ ] Re-baseline obligation recorded in the PR description **and** carried into 0003's DoD, naming the exact command to re-run and the number it supersedes. Not a merge gate for this PR: the code it measures does not exist yet
- [ ] Identical final PROGRESS state vs baseline (smoke Step 2)

**Mechanics**
- [ ] `./scripts/generate-skills.sh --check` passes
- [ ] `./scripts/generate-plugin.sh 0.10.14 && git diff --exit-code plugin/` — **not** `--check`. After 0006, `--check` ignores the `version` field, so it would pass over a `plugin.json` accidentally re-stamped to `0.0.0-smoke` or `dev` by a `build.sh` run during the smoke test. `git diff --exit-code` catches that; `--check` cannot. Exposure is the marketplace git surface (`marketplace.json` points at `./plugin`), not the release tarball, which `release.yml` regenerates from the tag
- [ ] Tier 2 actually executed, **before Commit 2 (baseline) and after (post-change)**, both results pasted in the PR description. Commit 2 is gated on execution, not on the description mentioning it:

  ```bash
  BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont
  ```

  `-timeout 0` is **required**: Go's default is a 10-minute budget for the whole test binary, and `executeLoopAction` has no watchdog of its own. On a timeout panic `killProcessGroup` never fires, so the live `claude -p` child is orphaned and keeps consuming budget. Name the fixtures that run live (`single-milestone-clean`, `mid-milestone`, `failing-acceptance` — the three whose transitions depend on agent output), and state expected wall-clock and token cost per run so an implementer can judge before starting
- [ ] Tier-2 fixture setup runs the source-mode installer so the fixture has a skill surface — a bare `t.TempDir()` repo has no `.agents/skills/`, so a live agent could never read the Optimisation-A prose
- [ ] Reuses `runGit` from `commit_update_test.go:13` — no duplicate helper
- [ ] `go build ./cmd/belmont`, `go test ./cmd/belmont` green
- [ ] Commits ordered harness → Optimisation A
- [ ] Auto, interactive and plugin surfaces exercised
- [ ] `meta/evals.md` + `cross-cutting/context-budget.md` created; routing rows added
- [ ] `AGENTS.md` updated (Verify section; `CLAUDE.md` is a symlink)
- [ ] PR description records a PR-status check performed **at branch time** (`gh pr list --state open`) and names the three deferrals
- [ ] PR description states this PR touches **no non-test Go** and therefore does not block PR 3

## 9. Risks

| Risk | Mitigation |
|---|---|
| Lazy Setup starves the agent | Escape hatch + smoke Step 2 PROGRESS diff + Tier-2 N≥3 |
| NOTES silently lost | Template in scope; decision explicit; `implementation-agent.md:56-61` is a partial backstop, not a fix |
| Harness green but blind | `failing-acceptance` fixture; Tier-2 gating stated openly |
| Fixtures encode wrong expectations | Must be green on unmodified `main` (Step 1) |
| Net token cost rises via PR 1 | PR 1 now lands **after** this PR, so the baseline cannot absorb Mode B. Reduction is reported as an upper bound; re-baseline owed post-0003 (§8); PR 1 caps its own output |
| Reduction claim quietly overstated once PR 1 lands | Upper-bound framing is required in the PR description, not optional — a bare "N% reduction" with no pre-0003 qualifier is a DoD failure |
| Harness becomes flaky and ignored | State-only assertions; live cases opt-in |

## 10. Interaction with PR 1 and PR 3

- **PR 1** adds to the sub-agent fan-out path this PR defers. **This PR lands first** (order set 2026-08-06), so PR 1's Mode B output is *not* in this PR's measured baseline — see §2. PR 1 inherits the re-baseline obligation: its own DoD should record the post-Mode-B remeasurement, since it is the change that invalidates this PR's number. The `_src/verify.md` conflict (§5) is unaffected by the order — whichever lands second rebases onto the first.
- **PR 3** — v1 created a conflict by editing `executeLoopAction`, which PR 3 relocates. With Optimisation B dropped this PR touches no non-test Go, so the conflict is gone. PR 3 consumes this harness in its CI workflow.

## 11. Changes from v1

| v1 | v2 |
|---|---|
| Optimisation B (cache prefix) | **Dropped** — 549-byte prompt, prefix vs suffix measured identical |
| Harness at `cmd/belmont/eval/` | `eval_harness_test.go` in `package main` — a subpackage cannot import `main` |
| Fixtures as nested git repos | Inert content + `commits.txt`, materialised in `t.TempDir()` |
| Single-tier harness | Tier 1 offline / Tier 2 live; Tier 2 licenses Optimisation A |
| 4 fixtures, all passing | 5, including a negative |
| "verify.md lists four" | Five |
| `next.md` unscoped | In scope — was the sole remaining outlier |
| Archive preferred unconditionally | Conditional on populated sections |
| NOTES silently dropped | Template in scope; conflict resolved |
| "CI stays green offline" | No CI exists; PR 3 owns `ci.yml` |
| Preflight vs open #4–#7 | Closed unmerged; sequence against merged #8 |
| #18 described as open | Merged — deferral (2) reframed as reversing a merged decision |
| `steering.md` amendment | Unnecessary once B is dropped |
| Smoke Step 2 broken four ways | Rewritten with rebuild, reinstall, `$BASE`, PROGRESS-file diff |
| `belmont steer <positional>` | `--message` |
| `plugin/` unmentioned | In scope |

### v4 → v5

Ordering change only — no design, scope or harness change. Made after the author set the build order to `0005 → 0004 → 0003` on 2026-08-06.

| v4 | v5 |
|---|---|
| Lands **after 0003**; baseline includes Mode B | Lands **before 0003**; baseline is pre-Mode-B |
| Reduction reported against a Mode B milestone | Reduction reported as a **pre-0003 upper bound**, with a re-baseline owed post-0003 (DoD) |
| Risk row "PR 1 lands first so the baseline includes Mode B" | Inverted; second row added for the overstated-claim risk the new order creates |
| §10 "Land PR 1 first" | This PR first; PR 1 inherits the re-baseline obligation |
| §5 "Rebase onto 0003" | 0003 rebases onto this; its insertion point may move under 0003's rev 5 |

### v3 → v4

| v3 | v4 |
|---|---|
| Tier 2 gate would panic at Go's 10-minute default and orphan the `claude -p` child | `-timeout 0`, named live fixtures, stated cost |
| `--check` DoD blind to a re-stamped `plugin.json` | `generate-plugin.sh 0.10.14 && git diff --exit-code plugin/` |
| Smoke `build.sh` runs left `plugin.json` dirty | `git checkout -- plugin/` after Steps 0 and 2 |
| #10/#11 cited as open | All closed; only #21 open and disjoint. Re-measure at branch time |

### v2 → v3

| v2 | v3 |
|---|---|
| `--check` DoD unsatisfiable on main | Depends on **0006**; pin `--check 0.10.14` until then |
| "Tier 2 licenses Optimisation A", never required to run | Executed `BELMONT_EVAL_LIVE=1` N≥3 gate before and after Commit 2 |
| Tier-2 fixtures had no skill surface | Fixture setup runs the source-mode installer |
| Preflight deleted wholesale | Re-measured: #10–#13 all closed, only #21 open and disjoint. Re-measure at branch time |
| Mode B addition not disclosed in Problem | Disclosed; reduction reported against a Mode B milestone |
| — | Reuses `runGit` from `commit_update_test.go:13` |
