# Eval Harness

**Why this matters.** Belmont's auto loop is a workflow run thousands of times, and until now nothing measured whether a change to it degraded agent output. Unit tests prove the Go machinery works; they say nothing about whether trimming a skill's Setup block makes the agent pick the wrong milestone. Without a measurement, every context optimisation is a guess — and the first one attempted (a cache-prefix reordering) turned out to be based on a premise that measurement disproved. The harness exists so that class of change has to earn its way in.

## Invariant

- **Two tiers, because one cannot do both jobs.** Tier 1 is offline and free; Tier 2 shells out to a real tool and costs tokens. Tier 1 validates Go machinery. **Only Tier 2 can license a change to skill prose** — a stub agent never reads a `SKILL.md`, so an offline harness cannot tell you whether prose still works.
- **Assertions are state transitions, never prose.** Assert that a task moved `[ ]` → `[x]`, never that the agent said a particular thing. An eval that asserts on wording fails the first time a model is swapped, and a suite that cries wolf gets ignored.
- **Fixtures are inert content, never nested git repos.** `git add -A` on a repo inside `testdata/` stages a gitlink (mode 160000); a fresh clone yields empty directories. Fixtures store `PRD.md` / `PROGRESS.md` / `TECH_PLAN.md` plus a `commits.txt` script, and `materialiseFixture` builds a real repo in `t.TempDir()`.
- **Every fixture's `PROGRESS.md` must pass `belmont validate`.** `belmont auto` lints at startup, so a fixture that fails validation could never run in practice.
- **A suite with no negative fixture cannot fail meaningfully.** On a clean-pass milestone a rigorous verify and a lazy one produce identical `PROGRESS.md` bytes. `failing-acceptance` is what makes the suite able to detect a *worse* agent.
- **Tier 2 fixtures must have a skill surface.** A bare `t.TempDir()` repo has no `.agents/skills/`, so a live agent would read no prose and the test would pass while measuring nothing. `materialiseLiveFixture` runs the source-mode installer and then asserts the directory exists.

## How it's enforced

`cmd/belmont/eval_harness_test.go`, build tag `eval`; fixtures in `cmd/belmont/testdata/eval/`.

```bash
go test -tags eval ./cmd/belmont                     # Tier 1 — offline, free
BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 \
  -run TestEvalLive ./cmd/belmont                    # Tier 2 — live, costs tokens
```

- **Location is forced.** `cmd/belmont` is `package main`; a subpackage cannot import it, every needed symbol (`parseMilestones`, `computeWaves`, `checkHardGuardrails`, `executeLoopAction`, …) is unexported, and `go test ./cmd/belmont` does not descend into subdirectories — so a harness at `cmd/belmont/eval/` would leave the DoD green over a broken suite. It must be an in-package test file.
- **`-timeout 0` is required for Tier 2.** Go's default is a 10-minute budget for the *whole* test binary and `executeLoopAction` has no watchdog. On a timeout panic the deferred `killProcessGroup` never runs, so the live `claude -p` child is orphaned and keeps spending budget after the test is gone.
- **Tier 1 runs in CI** (`.github/workflows/ci.yml`, *Eval harness (Tier 1)*), plus a `go vet -tags eval` step — the plain `go vet ./...` never type-checks a build-tagged file, so without it the harness can rot silently.
- **Tier 2 never runs in CI.** It needs credentials and spends money per run.

### The five fixtures

| Fixture | Pins | Live |
|---|---|---|
| `single-milestone-clean` | three `[ ]` tasks → `IMPLEMENT_MILESTONE` | yes |
| `mid-milestone` | mixed `[x]`/`[ ]` → still `IMPLEMENT_MILESTONE` | yes |
| `blocked-task` | `[!]` → `PAUSE` via hard guardrail, before any AI call | no |
| `multi-milestone-deps` | wave ordering: `[M1]` then `[M2, M3]` | no |
| `failing-acceptance` | a milestone whose code violates its own PRD | yes |

### What Tier 1 actually asserts

The deterministic prefix of `runLoop`: `checkHardGuardrails` → `decideLoopActionSmart`. It deliberately stops before `decideLoopActionAI`, which shells out to a model.

That makes a nil result meaningful in its own right: **a fixture that stops resolving deterministically now costs an AI call it did not cost before.** The harness fails on that, which is a cost regression a normal test would never notice.

`decideLoopAction` (as opposed to `…Smart`) is only the AI-failure fallback and is *not* on the asserted path. Breaking its blocked-task rule leaves the suite green; breaking `checkHardGuardrails`' does not.

## Failure mode if you break it

- **Assert on prose.** The suite starts failing on model swaps, everyone learns to re-run it until it passes, and it stops being a signal.
- **Drop the negative fixture.** The suite goes green against an agent that verifies code violating its own acceptance criteria — the exact regression it exists to catch.
- **Skip the installer in Tier 2 setup.** Live tests pass while the agent reads no skill prose. Worse than no test: it reports safety it never measured.
- **Let Tier 1 license a prose change.** The most tempting shortcut, because Tier 1 is free. It cannot: nothing in Tier 1 reads a `SKILL.md`.
- **Run Tier 2 without `-timeout 0`.** Orphaned tool processes keep burning budget after the run dies.

## Don't re-do

- **A harness at `cmd/belmont/eval/`.** Rejected: `package main` cannot be imported, and `go test ./cmd/belmont` would not run it — the DoD would stay green over a broken harness.
- **Fixtures as committed git repos under `testdata/`.** Stages a gitlink; clones empty.
- **A single-tier harness with a stub agent.** It validates Go and nothing else, while appearing to validate the whole loop. The tier split is the point.
- **A fixture whose dependency is already satisfied.** `multi-milestone-deps` originally had M1 pre-verified. With the edge satisfied at t=0, deleting dependency handling from `computeWaves` entirely still produced the expected wave shape — the assertion could not fail. Caught by running that exact control; M1 is now `[ ]` so the dependency genuinely orders the waves.
- **Asserting `failing-acceptance` reaches VERIFY through the decision engine.** It does not. `milestoneAllDone` treats `[x]` as done, so a fresh loop over an all-`[x]` milestone resolves to `COMPLETE` and `computeWaves` yields no waves. That is intended — `belmont reverify` is the documented `[x]` → `[v]` path. Tier 2 therefore constructs `actionVerify` directly.

## Evidence

Every Tier-1 assertion was checked against a deliberate control — the code it guards was broken on purpose and the suite had to fail:

| Control | Result |
|---|---|
| `milestoneAllDone` stops counting `[x]` as done | caught (2 subtests) |
| `computeWaves` ignores dependencies | caught — *after* the fixture was fixed; missed before |
| `checkHardGuardrails` no longer pauses on `[!]` | caught (`IMPLEMENT_MILESTONE`, want `PAUSE`) |
| `parseMilestones` maps `[x]` to todo | caught (4 fixtures) |
| `runScopeGuard` neutered | caught |
| fixture violates the milestone-structure lint | caught (both lint rules) |

The second row is the one worth remembering: the control did not fail at first, and the reason was a weak *fixture*, not a weak assertion. Running the controls is what found it.

## Revisions

- 2026-08-07 — initial. Records the two-tier split, the five fixtures, the `package main` location constraint, the `-timeout 0` requirement, CI wiring for Tier 1, and the six controls each assertion was checked against.
