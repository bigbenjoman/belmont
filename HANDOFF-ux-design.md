# Handoff — /belmont:ux-design split

**Delete this file before merging.** Working notes, not documentation.

Date: 2026-08-11 · Branch: `feat/ux-design-skill` · Base: `feat/design-contract` (PR #32)

---

## Where things stand

**Done and committed** — `4b77e7d "Split design authority into /belmont:ux-design"`, 76 files,
+2365/−900. Green: `go build`, `go vet ./...`, `go test ./cmd/belmont`, `go test -tags eval`,
`generate-skills.sh --check`, `generate-plugin.sh --check`, gofmt, `GOOS=windows go vet`,
staticcheck.

**Red-team COMPLETE** — run `wf_69eb3be7-100`, 33 agents. 5 hunter lenses, each finding handed to a
separate agent told to *refute* it. **27 verdicts: 18 refuted, 9 confirmed.** The 9 are written out
in full at the bottom of this file. Nothing has been fixed yet — that is task 1. Raw results:

- Journal: `~/.claude/projects/-Users-benlavender-belmont/674a435c-0cca-48fd-8265-a6235ae5d7bb/subagents/workflows/wf_69eb3be7-100/journal.jsonl`
- Task output: `/private/tmp/claude-501/-Users-benlavender-belmont/674a435c-0cca-48fd-8265-a6235ae5d7bb/tasks/wuw0xu5l7.output`

The workflow returns `{survivors, all}`. **Only `survivors` matter** — findings whose refuter could
not kill them. Read them, verify each independently (agents overstate), fix, re-run the full check
suite above.

---

## What changed and why

Design authority was being decided inside a *technical* planning session, and tech-plan's interview
had drifted into product scope it was explicitly forbidden to ask about. `/belmont:ux-design` is now
its own phase between `product-plan` and `tech-plan`, owning `{base}/UX_DESIGN.md`: screens, flows,
per-surface State Inventory, the `## Design Contract`, and two self-contained HTML artifacts
(`design-preview.html`, `ux-flows.html`). tech-plan lost Phase 3.5.

### Decisions taken — do not re-litigate without reading the reasoning

| Decision | Why |
|---|---|
| Contract lives in `UX_DESIGN.md`, not `TECH_PLAN.md` | ux-design runs before TECH_PLAN.md exists |
| **Figma extraction stayed in tech-plan** | User's call. So a Figma feature's `**Mode**` is in UX_DESIGN.md while its tokens are in TECH_PLAN.md § `## Design Tokens (from Figma)`. Asymmetry is deliberate and recorded in the knowledge tree |
| Migration is an interactive prompt, no CLI command | User's call |
| `runDesignAuthorityGuard` is in scope | User's call. Wired at all 3 `executeLoopAction` call sites, 7 tests |
| State Inventory is per **surface**, not per component | It could not move earlier otherwise — components are chosen in tech-plan. Surfaces are named from flows; tech-plan maps surfaces→components and may not add/remove/rename one |
| UX_DESIGN.md written in all three modes | "Silence is not allowed" — an absent file must mean *never ran*, not *no UI* |
| Ordering advisory, not blocking | A hard block breaks routine tech-plan re-runs and fails headless replan on unmigrated features |
| ux-design never invoked by auto | Approval artifact; no approver headlessly. No `actionUxDesign` |

### The invariant most likely to be broken by a future edit

**`**Mode**` is the only machine-read signal.** The file exists and carries a `## Design Contract`
heading on *every* feature, including Figma ones and no-UI ones. Keying on file existence or heading
presence fires contract checks on every Figma feature, finds no contract values, and fails every
Figma milestone. The literal is `derived — UI, no Figma` with a **U+2014 em-dash** — a hyphen in a
grep pattern silently never matches.

---

## Known gap, deliberately not fixed

`debug-auto` does **not** read the contract — zero references, and `_partials/debug-phase1-design.md`
skips design analysis whenever there are no Figma URLs, which is exactly the UI-no-Figma case.
AGENTS.md previously *claimed* it did; that was false and is now corrected there. Wiring it up is a
behaviour change needing its own Tier-2 eval. Separate PR.

---

## Still to do

1. ~~**Fix red-team survivors**~~ — DONE, commit `b90d8db`. All nine verified independently first;
   all nine were real. Full check suite green afterwards (build, vet, test, eval, both generators
   `--check`, gofmt, `GOOS=windows go vet`, staticcheck). No version-stamp dirt.
   - S7's fix gates Migration on `**Mode**` per-mode, with a table: only `derived — UI, no Figma` may
     be moved *or deleted*; `N/A — Figma present` is left in place because it is the parent of
     `### Design Tokens (from Figma)`; an unrecognised/absent mode asks rather than guesses.
   - S6 (SIGINT bypass) was **not fixed** — recorded as a known gap in
     `knowledge/cross-cutting/design-authority.md` beside the guard. It hits all three guards
     equally (scope, evidence, design), the snapshot is a local in `executeLoopAction`, and fixing
     it from the signal handler means plumbing per-worktree snapshots onto `worktreeTracker` while a
     process-group kill is in flight. Wants its own change with its own tests.
2. ~~**Rebuild the test kit**~~ — DONE. `~/belmont-pr32-test` now has **six** fixtures and a battery.
   - Fixtures rebuilt against the branch (skills reinstalled, so `ux-design` exists in each).
     `a-storybook` lost the pre-authored contract that was sitting in its baseline commit, so it
     tests derivation again.
   - **e-legacy** — the migration fixture the handoff asked for: an approved `derived — UI, no Figma`
     contract already in `TECH_PLAN.md`. Graded on one-contract-only, `**Approval**` byte-for-byte,
     and every subsection body identical to the baseline. Its pre-split per-component State
     Inventory is deliberately exempt from the per-surface checks — that is what verbatim means.
   - **f-legacy-figma** — added beyond the ask, because it is the S7 defect made testable: a legacy
     `N/A — Figma present` section nesting `### Design Tokens (from Figma)`, three PRD tasks with
     Figma URLs. Graded on the TECH_PLAN section surviving byte-for-byte with all seven extracted
     colours.
   - `bin/plant-golden.py` writes synthetic correct outputs; `bin/mutation-battery.py` runs **51
     deliberate defects — 51 caught, 0 escapes**, and flags any defect caught by an assertion other
     than the one it targets.
   - **The battery caught the battery.** `reset-fixture.sh` used `${1:?usage: … {a|b|all}}`, whose
     expansion ends at the first `}`, so every single-fixture reset failed with `no such fixture:
     a-storybook}` and exit 2 that nothing checked. Defects had been accumulating across the run —
     the first "51/51 caught" was worthless. Fixed in both scripts; the battery now hard-fails on a
     bad reset or plant and verifies the golden landed before mutating it. Re-run clean afterwards.
3. **Live Tier-2 run** — Blake's to execute. `BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont`.
   Tier 1 **cannot** license this change: it never opens a `SKILL.md`, and this change is almost
   entirely skill prose.
4. **Author smoke test section** for the PR description — AGENTS.md requires one for any non-trivial
   change, covering both invocation paths on the affected tools plus a sanity check that Claude Code
   still works unchanged.
5. **PR #32's own description** still describes the two-step pipeline. Either fold this branch into
   #32 or open a follow-up — decide with Blake.

## Useful context

- Live Storybook used as a real fixture: `https://storybook.studia.io/index.json` — 767 entries,
  104 components, 670 stories, 97 `docs` entries a correct run must ignore. Distinctive names like
  `BookingPicker` / `ExamBoardScreen` are unguessable, which is what makes them proof the index was
  actually fetched rather than invented.
- The design spec the implementation worked from:
  `/private/tmp/claude-501/-Users-benlavender-belmont/674a435c-0cca-48fd-8265-a6235ae5d7bb/scratchpad/uxdesign-spec.md`
  (in `/private/tmp` — copy it somewhere durable if still needed).


---

## Red-team survivors — 27 verdicts, 18 refuted, 9 CONFIRMED

Run `wf_69eb3be7-100`. Each was independently attacked by an agent told to refute it; these
are the ones that could not be killed. **Verify each yourself before fixing** — agents
overstate. Ordered by my read of severity.


### S1

```
The three-way boundary that this commit's own knowledge entry records as landed was never written into the shared partial. skills/belmont/_partials/plan-separation.md (unchanged by 4b77e7d; last touched in 4aead36) is 73 lines, headed "## PRD ↔ TECH_PLAN Boundary" (line 1), with only "What belongs in the PRD" (line 5) and "What belongs in the TECH_PLAN" (line 16), and zero occurrences of UX_DESIGN / ux-design / Design Contract. Two references treat it as three-way: (a) knowledge/cross-cutting/ux-design-phase.md:95-97 states under "How it's enforced" that the partial "is three-way — a `What belongs in UX_DESIGN` list beside the PRD and TECH_PLAN lists, so the boundary is stated once and included everywhere" — a false enforcement record that will stop the next session from adding the missing list; (b) docs/prd-format.md:62 sends the reader there "for the boundary rules" after stating the three-way split. Because the partial is @include'd at skills/belmont/_src/product-plan.md:48 and skills/belmont/_src/tech-plan.md:52 (and product-plan.md:209 calls it "the full PRD ↔ TECH_PLAN boundary rules"), every planning session on all eight install targets, in both invocation paths, receives a two-way split; the design-values→UX_DESIGN rule exists only in skills/belmont/_src/tech-plan.md:289 (Phase 4.5 step 3), which product-plan never reads. FIX: add a "What belongs in the UX_DESIGN (design surface)" list to skills/belmont/_partials/plan-separation.md (token scales, contrast floors, per-surface state inventories, motion bands; read-only for tech-plan; Figma-extracted tokens stay in TECH_PLAN per existing line 25), then regenerate skills/ and plugin/ — or, if the partial edit is deliberately deferred, move that bullet out of "How it's enforced" in knowledge/cross-cutting/ux-design-p
```

### S2

```
CONFIRMED (with one scope correction: three sources assert the read, not two).

Defect: commit 4b77e7d added drift category 10 to `/Users/benlavender/belmont/skills/belmont/_src/references/debug-manual-spec-reconcile.md:22` ("UX design drift — shipped UI contradicts the design authority", targeting `{base}/UX_DESIGN.md`) and published three claims that `/belmont:debug-manual` loads that file — but it did not touch `/Users/benlavender/belmont/skills/belmont/_src/debug-manual.md`, whose Step 0 read list (lines 42-48: PRD / TECH_PLAN / PROGRESS / NOTES / MILESTONE) and Context-load summary block (lines 59-62) still never mention UX_DESIGN.md.

Verified not handled elsewhere:
- Only partial mentioning UX_DESIGN is `/Users/benlavender/belmont/skills/belmont/_partials/debug-scope-rules.md:27`, and it only forbids editing.
- Generated output matches source: `/Users/benlavender/belmont/skills/belmont/debug-manual/SKILL.md:173-177,192` and `/Users/benlavender/belmont/plugin/skills/debug-manual/SKILL.md:173-177,192` both omit it.
- The indirect route is closed: `/Users/benlavender/belmont/skills/belmont/_partials/debug-phase1-design.md:1-3` skips design analysis entirely when no Figma URLs are present — precisely the `derived — UI, no Figma` case — so the contract never reaches DEBUG.md's `## Design Specifications` either.
- The recorded "known gap" at `/Users/benlavender/belmont/knowledge/cross-cutting/design-authority.md:270-277` covers `debug-auto` only.

Failure: user runs `/belmont:debug-manual` on a UI bug in a `derived — UI, no Figma` feature; the fix ships a 3.1:1 contrast value violating the contract's accessibility floor. `_src/references/debug-manual-spec-reconcile.md:9` and `_src/debug-manual.md:287` both scope the catalogue to "every **loaded** spec file", so categor
```

### S3

```
docs/agent-pipeline.md:35 refers to the MILESTONE file's `## File Paths` section, but that section is `### File Paths` — an h3 nested under `## Orchestrator Context` (skills/belmont/_src/references/implement-milestone-template.md:21 and :29; identically skills/belmont/_src/next.md:110). The clause was introduced by commit 4b77e7d (the pre-commit line contained no File Paths reference). It is the only one of 41 repo-wide "File Paths" references at h2; the other 40 all use `### File Paths` (agents/belmont/design-agent.md:36-38, agents/belmont/implementation-agent.md:25-26,28,43,60,69,80,218,224,318,421,426, agents/belmont/codebase-agent.md:29,30,195, skills/belmont/_src/implement.md:67, skills/belmont/_src/next.md:60, plus their plugin/ generated counterparts). The error is notable because docs/agent-pipeline.md otherwise quotes MILESTONE headings at their true level (`## Codebase Analysis`, `## Design Specifications`, `## Implementation Log` at lines 3, 19, 43, 53, 54, 60 are all genuinely h2 per template lines 64, 67, 70), so the level is meant to be load-bearing. Fix: change `## File Paths` to `### File Paths` on docs/agent-pipeline.md:35. Note the reviewer mis-cited `agents/belmont/design-agent.md:41` — the File Paths lines in that file are 36-38; :41 is in plugin/agents/design-agent.md.
```

### S4

```
Commit 4b77e7d removed the Design Contract carve-out from all three of debug-manual's scope statements and re-scoped the replacement to UX_DESIGN.md only, leaving a pre-split ("legacy") `## Design Contract` section inside `{base}/TECH_PLAN.md` editable by `/belmont:debug-manual`'s spec-reconciliation pass.

Exact lines:
- skills/belmont/_src/references/debug-manual-spec-reconcile.md:17 — category 5 row now reads "TECH_PLAN decision wrong or stale | `{base}/TECH_PLAN.md` or `.belmont/TECH_PLAN.md` | Edit narrative in place; preserve heading structure", with the previous "**Excludes `## Design Contract`** — see Hard rules" deleted.
- skills/belmont/_src/references/debug-manual-spec-reconcile.md:132 — the hard rule was "**Never edit the `## Design Contract` section of any TECH_PLAN.**"; it now forbids only `{base}/UX_DESIGN.md` / `.belmont/UX_DESIGN.md` and their html artifacts.
- skills/belmont/_partials/debug-scope-rules.md:16 — the `{base}/TECH_PLAN.md` "What you MAY edit" row lost "**Except `## Design Contract`**, which is written only by `/belmont:tech-plan` and carries an approval stamp this flow cannot renew"; the new prohibition at line 27 names UX_DESIGN.md only.

Reachability (verified, not hypothetical): skills/belmont/_src/ux-design.md:106-111 states that features planned before the split carry their contract in `{base}/TECH_PLAN.md` and offers a "Leave it" branch that writes nothing; knowledge/cross-cutting/design-authority.md states the gate is not retroactive and adoption is opt-in, and `ux-design` is interactive-only with no auto action — so every pre-split feature retains the TECH_PLAN-resident contract until a human runs the new skill.

Failure path: debug-manual's Step 0 (skills/belmont/_src/debug-manual.md, per-feature reads 5-9) loads `{base}/TECH_PLAN
```

### S5

```
cmd/belmont/install.go:181-190 — the post-install `Workflow:` banner still lists `1. Plan` → `2. Tech Plan` and omits the ux-design phase, even though the same commit added `/belmont:ux-design` to the Claude (`install.go:155`) and opencode (`install.go:174`) `Use:` lines printed four lines above it, and updated README.md, docs/workflow.md and docs/supported-tools.md. Reproduced by building the branch and running `belmont install --source <repo> --project <tmp> --tools all --no-prompt`. This is the only ordered pipeline listing printed at install time, it prints for all eight tools, and it is the sole surface naming the phases in order for the six tools whose `Use:` line is just "prompt belmont:<skill>". The banner is not merely a curated subset: ux-design is an unconditional phase (product-plan/SKILL.md:452 — "Run it even when the feature has no user interface"), so `Plan → Tech Plan` is now wrong for every feature. Correction to the reviewer's failure scenario: the outcome is user misdirection, not a silent design-authority gap — tech-plan/SKILL.md:415 detects a missing {base}/UX_DESIGN.md when PRD tasks produce visible UI and offers (recommended) to stop and run `/belmont:ux-design --feature <slug>`, and product-plan/SKILL.md:452 already hands the user to ux-design at the end of the plan phase. Fix: insert `fmt.Println("  2. UX Design  - Derive the design authority (screens, flows, contract)")` after install.go:183 and renumber the remaining rows 3-9 (Tech Plan, Implement, Next, Verify, Status, Reset, Cleanup). No test or other caller depends on the banner text (single occurrence repo-wide).
```

### S6

```
CONFIRMED (with one scope correction): runDesignAuthorityGuard only ever runs after the agent subprocess returns — its three call sites are cmd/belmont/auto_loop.go:252, :347 and :358, all inside executeLoopAction. A phase that is killed rather than returned therefore gets no guard pass, and the agent's write to {base}/UX_DESIGN.md survives to become main's approved contract.

Exact chain, all verified:
1. cmd/belmont/auto_parallel.go:195-204 — the SIGINT goroutine calls activeWorktrees.gracefulShutdown(cfg.Root) then os.Exit(1). gracefulShutdown (cmd/belmont/worktree.go:1081-1100) signals the process group, runs teardown hooks and prints "Worktree preserved"; it restores nothing under .belmont/. executeLoopAction never returns, so none of runScopeGuard / runEvidenceCheck / runDesignAuthorityGuard runs.
2. On the next `belmont auto`, the INTERACTIVE [r] branch of handleStaleWorktree (cmd/belmont/feature.go:254-292) returns resumed=true and states "Leave .belmont/ as-is in the worktree", re-copying only when .belmont/ is absent or a legacy symlink. createWorktreeIfNeeded (cmd/belmont/worktree.go:335-348) then returns after untrackBelmontInWorktree without calling copyBelmontStateToWorktree. rebaseWorktreeOnMain (cmd/belmont/worktree.go:113-123) also cannot restore it: its dirty gate is `git status --porcelain`, and .belmont/ is assume-unchanged, so the modified file is neither seen nor rewritten.
3. snapshotDesignAuthority (cmd/belmont/auto_loop.go:236 -> cmd/belmont/guards.go:301-335) re-reads from disk at the start of every phase, so the corrupted bytes become the baseline and runDesignAuthorityGuard reports "untouched" (guards.go:365) for the rest of the run.
4. syncFeatureStateAfterMerge (cmd/belmont/worktree.go:368-396) does os.RemoveAll(dstFeature) + copyDir(srcFea
```

### S7

```
CONFIRMED (major), with a narrowed failure statement. skills/belmont/_src/ux-design.md:108 triggers Migration on the bare presence of a `## Design Contract` heading in {base}/TECH_PLAN.md, with no `**Mode**` test — the same heading-keying that verify.md:91 and design-agent.md:281 explicitly forbid. Line 110's recommended option then says to move the section verbatim into UX_DESIGN.md and "delete it from {base}/TECH_PLAN.md". For any feature planned by HEAD~1-era Belmont whose PRD carries a Figma URL, that section is exactly `## Design Contract` / `**Mode**: N/A — Figma present` / `### Design Tokens (from Figma)` <extracted values> (HEAD~1 references/tech-plan-feature-format.md:51-105; HEAD~1 _src/tech-plan.md Phase 4 "Do not drop them"). The delete therefore removes the only recorded copy of the extracted Figma values. The values do not reliably survive in UX_DESIGN.md either: references/ux-design-format.md:12-14 says "Figma tokens do not live here" and its template says "[Under either N/A mode the four fields above are the whole file. Stop here.]", and _src/ux-design.md:189 says "Under either `N/A` mode, do step 1 and stop" — so the likelier outcome is outright loss, not relocation. Downstream, references/implement-milestone-template.md:31 still sends the design-agent to TECH_PLAN.md's `## Design Tokens (from Figma)`, which is now absent. ux-design cannot repair it: CRITICAL RULE 5 (_src/ux-design.md:18, restated :74) permits exactly one TECH_PLAN.md edit — the deletion — and no clause anywhere re-homes a nested legacy subsection. The generated skill (skills/belmont/ux-design/SKILL.md:201-203) and the plugin copy (plugin/skills/ux-design/SKILL.md:203) carry the identical text, so this is not a generation-drift artifact. Fix: gate Migration on `**Mode**: derived — UI, n
```

### S8

```
CONFIRMED — major. Commit 4b77e7d makes the Storybook rung and the State Inventory naming rule mutually unsatisfiable, in the same reference file.

skills/belmont/_src/references/design-authority-baseline.md:167 — "**Component titles give you the `## Screens` table's surfaces; story names give you the State Inventory directly**"
skills/belmont/_src/references/design-authority-baseline.md:195-197 — "Either way: record the component inventory into `{base}/UX_DESIGN.md`'s `## Screens` table, under its *Surfaces it contains* column; treat those components as **LAW**"

versus

skills/belmont/_src/references/design-authority-baseline.md:265-267 — "named from the PRD's flows and the `## Screens` table, never from a file path or a component name"
skills/belmont/_src/references/ux-design-format.md:58-59 — same rule, same wording.

Input: a feature whose project has a deployed Storybook — rung 1, the highest rung below a contract, and the one Phase 3 (skills/belmont/_src/ux-design.md:140) tells the agent to ask about FIRST. The agent fetches `<url>/index.json`; each entry carries a `title` that is a component path (`Bookings/BookingPicker`, stated at design-authority-baseline.md:165). Line 167 and 195-197 send those titles into the `## Screens` table's *Surfaces it contains* column. ux-design-format.md:58-59 then names State Inventory surfaces from that same column, and skills/belmont/_src/ux-design.md:182 confirms they are one set ("one panel per `## Screens` row listing that screen's surfaces and each surface's states"). Result: an approved Design Contract whose State Inventory is enumerated per component — exactly the framing the commit message says was moved away from ("it enumerated states per component file and components are chosen in tech-plan").

The damage is then froze
```

### S9

```
CONFIRMED (major). Commit 4b77e7d removed debug-manual's only prohibition on editing a `## Design Contract` that still lives in `{base}/TECH_PLAN.md`, and replaced it with prohibitions scoped exclusively to UX_DESIGN.md.

Exact lines:
- `skills/belmont/_partials/debug-scope-rules.md:16` — MAY-edit row for `{base}/TECH_PLAN.md` lost "**Except `## Design Contract`**, which is written only by `/belmont:tech-plan` and carries an approval stamp this flow cannot renew"; now reads as blanket permission.
- `skills/belmont/_partials/debug-scope-rules.md:27` — the new MUST-NOT bullet names only `{base}/UX_DESIGN.md` and `.belmont/UX_DESIGN.md`.
- `skills/belmont/_src/references/debug-manual-spec-reconcile.md:17` — drift category 5 lost "**Excludes `## Design Contract`**".
- `skills/belmont/_src/references/debug-manual-spec-reconcile.md:132` — the Hard rule was retargeted from "any TECH_PLAN" to UX_DESIGN.md only.
- Generated output confirms it ships: `skills/belmont/debug-manual/SKILL.md:132`, with zero `Design Contract` mentions in the generated debug-manual SKILL.md or reference.

Why the input exists: `skills/belmont/_src/ux-design.md:111` offers a **Leave it** migration option that writes nothing and leaves the section in TECH_PLAN.md, and `_src/tech-plan.md:110-117` / `:264-266` only forbid tech-plan from writing a contract — neither deletes an existing one. Every feature planned before this release keeps it there until the user re-runs ux-design and picks Migrate.

Failure: on such a feature, interactive `/belmont:debug-manual` finds shipped UI at `radius: 6px` against the contract's approved `8px`. Category 10 is scoped by its File column to `{base}/UX_DESIGN.md` so it does not match; drift routes to category 5, whose exclusion is gone, and the skill proposes "update plan 
```