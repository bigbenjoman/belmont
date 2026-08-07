LOGS:
  lens completeness: 11 findings -> refuting all
  lens facts: 8 findings -> refuting all
  lens gating-logic: 11 findings -> refuting all
  lens consistency: 14 findings -> refuting all
  lens design-soundness: 10 findings -> refuting all
  lens executability: 15 findings -> refuting all
  69 raised across 6 lenses; 69 judged; 53 survived, 16 refuted
  survived: blocker 1, major 21, minor 31

raised 69 | judged 69
survived: blocker 1, major 21, minor 31 = 53
refuted: 16  (23%)
lenses: ['facts:8', 'consistency:14', 'gating-logic:11', 'executability:15', 'design-soundness:10', 'completeness:11']

======================================================================
# BLOCKERS

## B1 @ docs/proposals/0003-design-quality-without-figma.md:94 (R1) vs :225-227 (§4.6 branches), :233 (fourth rule), :281 (§4.9), :379 (Step 7), :426 (DoD)
Every quote checks out verbatim: :94 ("Record the outcome in the contract's `**Mode**` field … Backend features record `N/A — no UI`; Figma features record `N/A — Figma present`. **Silence is not allowed**"), :154-155 (Mode is the first line under `## Design Contract`; no other container exists), :225 ("plus contract checks if a contract is present"), :226 ("No references, contract present"), :227, :233 ("a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE"), :379, :426. I searched the whole 491-line document for a definition of "a contract is present" — grep for `contract is present|contract present|no contract|contract exists` returns only :95, :225, :226, :227, :233, :418, :426. None defines the discriminator. So the failure mode (b) refutation ("addressed elsewhere") does not apply.

The defect is real and gets WORSE than the finding states. `skills/bel

**FIX:** 1. In §4.6, insert immediately after the intro paragraph at :223 and before branch 1: "**\"A contract is present\" means one thing only:** `{base}/TECH_PLAN.md` contains `## Design Contract` whose `**Mode**` is `derived — UI, no Figma`. Both `N/A` modes and an absent section are \"no contract\" for all three branches and for the fourth enforcement rule. **Never key on the presence of the `## Design Contract` heading** — the feature template carries it unconditionally (`tech-plan-feature-format.md:44`, generalised per §4.4), so a Figma feature's plan has the heading too, holding its Figma-extracted tokens."
2. Restate the fourth rule at :233 as: "*`**Mode**` is `derived — UI, no Figma` but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*", and narrow the `:131` rewrite to "no design references **and** no `derived` design contract".
3. In §4.4, after the schema block, state what each `N/A` mode's section contains: Mode line only for `N/A — no UI`; Mode line plus today's Figma-extracted tokens for `N/A — Figma present`; no baseline subsections in either case.
4. Reword Step


======================================================================
# MAJORS

## M1 @ §6 "Both invocation paths" (line 322) and §9 Definition of Done → "Corrections carried by this PR" (line 430)
QUOTE CHECK — the spec says exactly what the finding claims. Line 322: "For **six of eight CLIs** that is false: `buildLoopPrompt` emits a bare slash reference (`main.go:7168`, `:7189`)" and "For **Pi and opencode**, `adaptPromptForTool` (`main.go:7143-7157`) deliberately rewrites the reference". Line 430: "`AGENTS.md:26` and `dual-invocation-paths.md:9` corrected **per tool** — bare slash reference for six CLIs, del

**FIX:** In §6 line 322, replace "For **six of eight CLIs** that is false" with "For **five of the seven CLIs `belmont auto` can shell out to** that is false", and append after the Pi/opencode sentence: "Windsurf is an install-only target — `belmont auto --tool windsurf` is rejected (`main.go:5338-5341`) and `toolHeadlessArgs` (`:7922`) has no windsurf case — so the eight-CLI install surface (`AGENTS.md:197`) and the seven-CLI auto surface must not be con

## M2 @ §4.7 Downstream consumers, `skills/belmont/_src/verify.md` bullet (line 267); scope row at line 296
QUOTE CHECK — line 267 reads verbatim: "the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behaviour by enumerating only Figma/screenshot references and injecting \"No design references found\"". The parenthetical citation attaches to "the sub-agent dispatch prompt", and the clause immediately following describes the specific defect. No ellipsis, no misquote.

FILE CHECK — skills/be

**FIX:** In §4.7 line 267, change the middle clause from "the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behaviour" to "Agent 1's dispatch prompt (`:114-121`) steers the agent away from the new behaviour". Keep the rest of the sentence unchanged. Drop `:146` entirely; if a contract read is also wanted in the code-review dispatch, add it as a separate, explicitly-labelled fourth edit citing `:146` for that narrow addi

## M3 @ docs/proposals/0003-design-quality-without-figma.md:233 (§4.6 fourth enforcement rule) vs :256 + :373 (focused re-verification) vs :418 + :421 (DoD)
QUOTES CHECKED, ALL ACCURATE. :233 reads verbatim "add a fourth enforcement rule: *a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*" — the antecedent keys on EXISTENCE, with no qualifier. :256 reads verbatim "Contract checks are **exempt from that skip when the milestone's follow-ups touched UI**; otherwise the attestation records `Contract checks performed: NO — focused re-verifica

**FIX:** Three edits. (1) §4.6 line 233 — replace "and add a fourth enforcement rule: *a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*." with: "and add a fourth enforcement rule: *a contract exists (`**Mode**: derived — UI, no Figma`), contract checks were required, and they were not performed ⇒ MUST be FAIL/INCOMPLETE*. Checks are **not** required only in the focused-re-verification case defined below (focused scope, n

## M4 @ docs/proposals/0003-design-quality-without-figma.md:82 (the write rule) vs §5 in-scope table :289-308
QUOTE CHECKED: :82 reads verbatim "> **Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`. `implement`, `verify`, `next` and `debug` read it and never write it.**", introduced at :80 as "**Therefore, the rule of this PR — and what enforces it:**" — so it is normative, and its first sentence is unqualified. CODE CHECKED, ALL THREE CITATIONS EXACT: knowledge/cross-cutting/debug-spec-reconciliation.md:9 — "Inter

**FIX:** Four edits, and note the finding names the wrong file for two of them — the permission lives in the partial and the reference, not in `_src/debug-manual.md` (which merely `@include`s the partial at :29 and restates it at :299-304). (1) §4.1 line 82 — replace with: "> **Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`'s `## Design Contract`. `implement`, `verify`, `next` and `debug-auto` read it and never write it.** When `tech-plan` its

## M5 @ docs/proposals/0003-design-quality-without-figma.md:267 (§4.7 verify.md bullet) and the §5 row at :296
Spec quote confirmed verbatim at :267. Code evidence confirmed by opening skills/belmont/_src/verify.md: :109 and :146 are the identical line `> Read \`{base}/TECH_PLAN.md\` for technical specifications (if it exists).` — :109 inside `### Agent 1: Verification (verification-agent)` (:92) and :146 inside `### Agent 2: Code Review (code-review-agent)` (:129, block runs to :152). The content the spec describes is at :11

**FIX:** In docs/proposals/0003-design-quality-without-figma.md line 267, replace the middle clause only.

OLD (middle clause): `the **sub-agent dispatch prompt** (\`:109\`, \`:146\`) steers the agent away from the new behaviour by enumerating only Figma/screenshot references and injecting "No design references found";`

NEW: `the **Agent 1 (verification) dispatch prompt** (\`:114-121\`) steers the agent away from the new behaviour — \`:114-118\` enumerat

## M6 @ docs/proposals/0003-design-quality-without-figma.md:111 + :119 (§4.3) vs :51-55 (§3 table) vs :158-159 (§4.4 Authorities schema)
All three sub-defects are real and I traced them to the exact commit that caused them. `git show 1d060a7` ("docs(0003): drop interactive-prototype from the design ladder") deletes three things and replaces only one of them:

(a) §3 table — the diff removes `| [interactive-prototype](…) | State Inventory — gesture and transition states a static spec omits |` and does NOT add State Inventory to the ux-designer/ux-motio

**FIX:** Three edits, all in docs/proposals/0003-design-quality-without-figma.md.

1. §3 table, replace lines 53 and 55 with:
`| [ux-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | UX Strategy; the component-state half of State Inventory |`
`| [ux-motion](https://drive.google.com/drive/folders/1gYrK4aT4A-LYr4GbM88-PaKu0wYyY4C6) | Motion Contract; the transition half of State Inventory |`
(mirrors t

## M7 @ docs/proposals/0003-design-quality-without-figma.md:94 (R1 gate) vs :264 (§4.7 design-agent) and :300 (§5 design-agent row)
Quotes verified: :94, :264 ("mode keyed on Figma-URL presence in the PRD, never on load outcome. With Figma: unchanged. Without: derive per-task component specs *against the approved contract*"), :300 ("Consume the contract; replace `## Handling No Design`; read master TECH_PLAN"). Code evidence verified: `agents/belmont/design-agent.md:253` ("If Figma URLs are provided for a task, they MUST load. Block ONLY that tas

**FIX:** 1. Add to R1 (after :96's scope paragraph): "**Mixed Figma coverage.** A feature counts as a Figma feature if **any** task in `{base}/PRD.md` carries a Figma URL, even when other UI tasks carry none. Those uncovered tasks keep today's per-task no-Figma behaviour — deliberately, since a partially-covered feature has no single design authority to reconcile against. The replacement for `## Handling No Design` MUST therefore keep a no-contract arm."


## M8 @ docs/proposals/0003-design-quality-without-figma.md:99 (R4) vs :281-283 (§4.9) and :404 (DoD)
QUOTE CHECK: exact. :281 = "**This gate is not retroactive.** A feature planned before this PR has a `{base}/TECH_PLAN.md` with no `## Design Contract`, so §4.6 branch 3 applies … That is deliberate: silently applying a new quality gate to already-approved plans would fail milestones on a standard their author never agreed to." :404 = "Pre-contract features fall to branch 3 and are documented as non-retroactive (§4.9

**FIX:** Append a fourth clause to R4 (line 99), after "…and never silently regenerate an approved contract.": "**A headless replan never creates a contract that does not exist.** If `{base}/TECH_PLAN.md` contains no `## Design Contract` heading at all, Phase 3.5 writes nothing and the feature stays on §4.6 branch 3 — 'deriving only sections that are absent' means subsections of a contract that is already present, never the contract itself. Contract autho

## M9 @ docs/proposals/0003-design-quality-without-figma.md:151 (§4.4) and :292 (§5 tech-plan-feature-format row) vs :95 and :379/:426
Every quote checks out and the code evidence is exact.

Spec :151: "The feature template's `## Design Tokens (from Figma)` (`tech-plan-feature-format.md:44`) is **generalised**, not supplemented". Spec :292: "Generalise `## Design Tokens (from Figma)` → `## Design Contract` incl. all six subsections". Spec :95: "Figma features keep today's behaviour untouched." Spec :379: "Expect `**Mode**: N/A — Figma present`, **no

**FIX:** Resolve the ambiguity in favour of keeping the Figma destination, in three places:

1. §4.4 (:151) — replace "is **generalised**, not supplemented — it is Figma-scoped today, and its verification checklist hard-codes \"Matches Figma design pixel-perfect\"" with: "is **generalised**, not supplemented — it is Figma-scoped today, and its verification checklist hard-codes \"Matches Figma design pixel-perfect\". Under `**Mode**: N/A — Figma present` t

## M10 @ docs/proposals/0003-design-quality-without-figma.md:370 (Step 4)
Quote checked verbatim against spec line 370 — accurate. Every cited fact verified independently: `skills/belmont/_src/references/verify-report-format.md:23` is exactly `- Visual Verification: [PASS/FAIL/N/A]`; `skills/belmont/_src/verify.md:125` is exactly `**Collect**: The verification report document.`; `agents/belmont/verification-agent.md:210` is `## Visual Verification (if applicable)` and `:231` is `### Visual

**FIX:** Replace line 370 in full with:

"**Step 4 — the gate fires.** *Observed in the Step 1b interactive run, not the auto run.* The per-row table and the attestation exist only in the verification sub-agent's returned report (`verification-agent.md` `## Output Format`, `:210` / `:231`); `verify.md:125` only *collects* it into context and never writes it to disk, and the orchestrator's combined summary (`references/verify-report-format.md:23`) collapse

## M11 @ docs/proposals/0003-design-quality-without-figma.md:388 (Step 10) vs :434 (DoD Mechanics)
Both quotes checked verbatim — line 388 is `**Step 10 — plugin surface.** `./scripts/generate-plugin.sh <ver>`; confirm …` and line 434 is `- [ ] `plugin/` regenerated and committed; verified with `./scripts/generate-plugin.sh --check` (version-agnostic after 0006), **not with a version argument that mutates `plugin.json`**`. The 'not with a version argument' clause modifies 'verified with', and Step 10 is a verifica

**FIX:** Replace line 388 in full with:

"**Step 10 — plugin surface.** Run from the **belmont source repo**, not the smoke project:
```bash
cd ~/belmont
./scripts/generate-plugin.sh --check && echo "plugin/ up to date"
test -s plugin/skills/tech-plan/references/design-authority-baseline.md
! find plugin/agents -name '*.md' -size 0 | grep -q .   # 0006's fix
```
Regeneration and commit of `plugin/` happen inside the PR at the release version, **not as a s

## M12 @ docs/proposals/0003-design-quality-without-figma.md:360-363 (Step 2)
Quote checked verbatim against lines 360-363 — accurate, including the trailing clause 'to hash-match master's after resume'. Every supporting citation the finding offers is correct and I re-derived each: `mergeWorktreeBranch` calls `syncFeatureStateAfterMerge(cfg.Root, wtPath, cfg.Feature)` at main.go:9157 and `removeWorktree(cfg.Root, wtPath, milestoneID)` at :9164, then `git branch -d` at :9166-9170; `hasExplicitD

**FIX:** Replace line 363 in full with:

"Expect the banner `Belmont Auto (parallel)` — a serial fallback means the precondition was not met. Then, **in a second shell while wave 1 is still running**, confirm the resume-time state copy preserved the contract. The window is narrow and must be named: the worktree exists only between `Resuming with existing worktree at …` (`main.go:6113`) and the wave-1 merge, which runs `syncFeatureStateAfterMerge` (`:9157`

## M13 @ docs/proposals/0003-design-quality-without-figma.md:366-368 (Step 3)
Quote check passes verbatim. Spec :367-368 reads "`git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md` — expect **empty**." / "Do **not** use a bare `git diff`: `.belmont/` is `--assume-unchanged` in a worktree, so `git diff`, `git diff -- .belmont/` and `git status --porcelain` all report nothing even after the file has been completely rewritten (measured)."

Code check passes. cmd/belmont/main.go:10169-1

**FIX:** Edit §8 Step 3 (lines 366-368). (1) Add a lead-in sentence: "Run these three from the **master repo root**, after the auto run has fully exited — not mid-run." (2) Replace the single command with:

```bash
# positive control — fails loudly if the contract is untracked or .belmont/ is gitignored
git ls-files --error-unmatch .belmont/features/<slug>/TECH_PLAN.md
# committed clobber
git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md   # e

## M14 @ docs/proposals/0003-design-quality-without-figma.md:402 (DoD) vs §8 Steps 1-10
Quote check passes. Line 402 reads verbatim: "- [ ] Contract created only when the feature has UI **and** no Figma URLs; `**Mode**` is the single signal, with `N/A` values for both exclusions".

I searched the whole 492-line document for the second exclusion rather than trusting the finding's step inventory. `grep -n 'no UI\|backend\|Backend'` returns only :23 (problem framing), :94 (R1, "Backend features record `N/A

**FIX:** Add a new step to §8, immediately after Step 7 (line 379), so the two exclusions sit together:

"**Step 7b — backend regression (the other exclusion).** A feature with **no user interface** (API, pipeline, infra). `claude` → `/belmont:tech-plan --feature <backend-slug>`.
Expect: the interview asks **no design questions**; `{base}/TECH_PLAN.md` carries `## Design Contract` with `**Mode**: N/A — no UI` and **no** `### Token Contract`, `### UX Strat

## M15 @ docs/proposals/0003-design-quality-without-figma.md:279-283 (§4.9) and :404 (DoD)
QUOTE CHECK: both exact. :283 reads "To adopt the contract on an existing feature, re-run `/belmont:tech-plan --feature <slug>`; Phase 3.5 derives the missing section and the user approves it." :404 reads "- [ ] Pre-contract features fall to branch 3 and are documented as non-retroactive (§4.9)".

COVERAGE CHECK (failure mode (b) — is it handled elsewhere?): I enumerated every step header in §8 — Preconditions, 1, 1b

**FIX:** Two edits.

(1) In §8, insert after Step 1b (line 358) a new step:

**Step 1c — a pre-contract feature adopts the contract (§4.9 migration).** Take a **second** UI-bearing, no-Figma feature `<slug3>` whose `{base}/TECH_PLAN.md` was written before this branch and has no `## Design Contract` (or strip the section from a copy of Step 1's feature). Record `PRE=$(git rev-parse HEAD)`, then `claude` → `/belmont:tech-plan --feature <slug3>` and approve.

## M16 @ §8 Step 3, lines 366-368; DoD line 395; §10 line 444
QUOTE CHECK — verbatim. Line 366: "**Step 3 — design-agent consumes, never writes.** After the wave **merges**:"; line 367: "`git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md` — expect **empty**."; line 368 ends "...which is why this compares committed revisions." DoD line 395 and §10 line 444 are quoted accurately.

CODE CHECK — every call site is exactly as cited. main.go:8638-8640 `if err := runWavePa

**FIX:** Rewrite §8 Step 3's timing and check (lines 366-368). Replace "After the wave **merges**:" and the `$PLAN..HEAD` command with:

"**Step 3 — design-agent consumes, never writes.** Run this **after `belmont auto` exits with `✓ All waves complete`**, not after an individual wave merges:
`git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md` — expect **empty**.
**If the run aborted mid-wave, that check is invalid and silently passes.** `sync

## M17 @ §4.6 line 260
QUOTE CHECK: line 260 reads verbatim "**Severity** uses the existing Critical / Warning / Polish ladder: contrast failure, missing focus indicator, missing label → Critical; missing state, off-scale spacing, out-of-band duration → Warning; radius, elevation or easing inconsistency → Polish. Off-scale spacing stays Warning so pre-existing drift cannot block a milestone." The finding's ellipsis removes nothing that cha

**FIX:** In `docs/proposals/0003-design-quality-without-figma.md`, replace all of line 260 with:

"**Severity** uses the existing Critical / Warning / Polish ladder (`verification-agent.md:295-326`). Note that **Warning blocks the milestone and generates FWLUP tasks** (`:307`, `:292`) — only Polish does not (`:314`). Contrast failure, missing focus indicator, missing label → Critical. Missing state, out-of-band duration → Warning. Off-scale spacing, radiu

## M18 @ §4.1 line 82 (the PR's central rule); §5 in-scope table lines 289-308; DoD line 395; §7 line 335
QUOTE CHECK: line 82 reads verbatim "> **Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`. `implement`, `verify`, `next` and `debug` read it and never write it.** When `tech-plan` itself runs headlessly as `actionReplan`, R4 constrains what it may change." DoD line 395 and §7 line 335 are as quoted; `grep -n -i debug` over the whole 492-line document returns only lines 82 and 375, so nothing elsewhere addre

**FIX:** Three edits to `docs/proposals/0003-design-quality-without-figma.md`.

1. Replace line 82 with:
"> **Only the `tech-plan` skill may write the `## Design Contract` section of `{base}/TECH_PLAN.md`. `implement`, `verify` and `next` read it and never write it.** When `tech-plan` itself runs headlessly as `actionReplan`, R4 constrains what it may change."
and add immediately below it:
"**One documented exception must be carved back.** Interactive `/b

## M19 @ §4.9 lines 279-283; R4 line 99; R1 line 94
PRIMARY CLAIM SURVIVES. Quotes check out verbatim: line 281 'This gate is not retroactive. A feature planned before this PR has a `{base}/TECH_PLAN.md` with no `## Design Contract`, so §4.6 branch 3 applies…'; line 99 R4 'preserve an existing `## Design Contract` verbatim, deriving only sections that are absent'. Code evidence verified line-by-line: main.go:7188-7189 `case actionReplan:` / `return fmt.Sprintf("/belmo

**FIX:** Two edits, both in the proposal.

1. §4.2, R4 (line 99) — after 'and never silently regenerate an approved contract' insert: 'And never *create* one: if `{base}/TECH_PLAN.md` carries no `## Design Contract` heading at all, the headless step derives nothing and leaves the file's design surface untouched — a pre-contract feature is adopted only by an interactive run (§4.9). R4's derive-missing-sections behaviour applies only to a contract that alre

## M20 @ §4.1 write rule (line 82); §5 In-scope table
Quotes check out. Spec line 82 reads exactly: "**Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`. `implement`, `verify`, `next` and `debug` read it and never write it.**" — the first clause is universal over skills, not scoped to loop phases, so any other skill that writes that path contradicts a rule the PR states. `skills/belmont/_src/review-plans.md` is that skill: `:36` `- **DO** edit PRDs, Tech Plans,

**FIX:** 1) Add a row to §5's In-scope table (after the `_src/next.md` row):

| `skills/belmont/_src/review-plans.md` | Treat `## Design Contract` as read-only: exclude it from Drift/Conflict rewrite options; emit a finding instead |

2) In §4.1, immediately after the blockquoted write rule at line 82, add:

"One existing skill also writes this path and is therefore in scope: `/belmont:review-plans` loads every feature `TECH_PLAN.md` (`review-plans.md:50`

## M21 @ §4.1 write rule (line 82, "debug"); §4.6 fourth enforcement rule (line 233); §5 In-scope table
All citations verified: debug-phase1-design.md:1 and :3 exact; debug-auto.md:30, :60-61, :93 exact; debug-auto.md Phase 3 at :125 with `**DEBUG MODE OVERRIDE**` at :132 and `**Outcome**: [FIXED | PARTIAL | NO_CHANGE | REGRESSION]` at :147; debug-manual.md:114 and the same include at :150, override at :160, Outcome at :226. Spec lines 82 and 233 say what is claimed, and no debug file is in §5.

Two of the finding's th

**FIX:** In §4.6, extend the "Close the escape clause" paragraph (line 233). After "...and add a fourth enforcement rule: *a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*", append:

"**Milestone mode only.** Both the narrowing of `:131` and the fourth rule apply to milestone-mode verification. Under **DEBUG MODE OVERRIDE** (`debug-auto.md:132`, `debug-manual.md:160`) verification is scoped to the reported bug and never 


======================================================================
# MINORS

- **m1** @ §8 Author smoke test, Step 2 precondition (line 361) — QUOTE CHECK — line 361 reads verbatim: "otherwise `hasExplicitDeps` is false (`main.go:5441-5453`) and `runAuto` routes to `runLoop` — master tree, no worktree, no `[r]` prompt." No misquote.

CODE CH
  *Fix:* In §8 Step 2 precondition (line 361), replace `runAuto` with `runAutoCmd` so the clause reads "…and `runAutoCmd` routes to `runLoop` — master tree, no worktree, no `[r]` prompt." No other change.
- **m2** @ §4.4 Contract format, "Per-task specs stay in MILESTONE" (line 211) — Quote check first: line 211 reads verbatim "design-agent still produces one `### Design: [Task ID]` section per active task (`design-agent.md:232`), because `implementation-agent.md` Step 1.1 looks it
  *Fix:* In `docs/proposals/0003-design-quality-without-figma.md`, replace line 211 in full:

OLD:
**Per-task specs stay in MILESTONE, unchanged in shape.** design-agent still produces one `### Design: [Task ID]` section per acti
- **m3** @ §4.2 R8 (line 103) — Quote check first: line 103 reads verbatim "R4 constrains Phase 3.5, but the instruction that actually writes the file is Phase 4 (`tech-plan.md:310`) via `tech-plan-feature-format.md`, which is an un
  *Fix:* In `docs/proposals/0003-design-quality-without-figma.md`, edit line 103.

OLD fragment:
R4 constrains Phase 3.5, but the instruction that actually writes the file is Phase 4 (`tech-plan.md:310`) via `tech-plan-feature-fo
- **m4** @ §12 Changes from v5, author-decisions table, `interactive-prototype` row (line 475) — Quote check first: line 475 reads verbatim "`interactive-prototype` is a *builder*: five of its eight steps produce React code with a named animation library, and its own description excludes static s
  *Fix:* In `docs/proposals/0003-design-quality-without-figma.md` line 475, replace the ratio clause. Drop the numerator/denominator rather than recount, since only Step 5 emits library code unambiguously.

OLD fragment:
`interac
- **m5** @ §4.3 "Use exact registered names" (line 123) — Quote check first: line 123 reads verbatim "`tech-plan.md:160` and `product-plan.md:165` currently name `vercel-react-best-practices` and `security`; neither resolves (`vercel:react-best-practices` is
  *Fix:* In §4.3 line 123, replace the sentence:

  **Use exact registered names.** `tech-plan.md:160` and `product-plan.md:165` currently name `vercel-react-best-practices` and `security`; neither resolves (`vercel:react-best-pr
- **m6** @ §6 (line 322) — Quote check first: line 322 reads verbatim "`AGENTS.md:26` and `dual-invocation-paths.md:9` say auto mode bypasses the tool's skill discovery and that Belmont injects the skill body, agent files and p
  *Fix:* In §6 line 322, replace:

  **Correct the record on auto mode, per tool.** `AGENTS.md:26` and `dual-invocation-paths.md:9` say auto mode bypasses the tool's skill discovery and that Belmont injects the skill body, agent 
- **m7** @ docs/proposals/0003-design-quality-without-figma.md:413 (DoD) vs :139 (§4.3 warning box) — Quote checked and accurate. :413 reads verbatim "- [ ] Every link in that table checked resolving at branch time — they are third-party URLs and this PR does not control them" (no bold in the source; 
  *Fix:* In docs/proposals/0003-design-quality-without-figma.md, replace line 413 in full:

OLD: `- [ ] Every link in that table checked resolving at branch time — they are third-party URLs and this PR does not control them`

NEW
- **m8** @ docs/proposals/0003-design-quality-without-figma.md:388 (smoke Step 10) vs :434 (DoD Mechanics) — Both quotes check out verbatim, and the mechanical claim is confirmed by measurement, not reasoning: in a scratch copy of scripts/ skills/ agents/ plugin/, `bash scripts/generate-plugin.sh 9.9.9` rewr
  *Fix:* In docs/proposals/0003-design-quality-without-figma.md, replace line 388 in full:

OLD: `**Step 10 — plugin surface.** \`./scripts/generate-plugin.sh <ver>\`; confirm \`plugin/skills/tech-plan/references/design-authority
- **m9** @ docs/proposals/0003-design-quality-without-figma.md:143-145 (§4.3) — Confirmed verbatim: :143 reads "**Two packaging notes**, both verified against the downloaded packages:" and exactly one bullet follows at :145. :147 ("These are **user-scope Claude Code skills**…") i
  *Fix:* docs/proposals/0003-design-quality-without-figma.md:143 — replace
`**Two packaging notes**, both verified against the downloaded packages:`
with
`**One packaging note**, verified against the downloaded packages:`
Do not 
- **m10** @ docs/proposals/0003-design-quality-without-figma.md:304 (§5 plugin row) and :320 (§6 Plugin row) — Quotes verified verbatim. :298 puts `skills/belmont/_src/codex-plan-apply.md` in scope; :304 reads "| `plugin/` | Regenerate — `plugin/skills/{tech-plan,product-plan,verify,next,implement}/**` and `pl
  *Fix:* Two edits, both single-line.

1. Line 304, replace the cell text
`| `plugin/` | Regenerate — `plugin/skills/{tech-plan,product-plan,verify,next,implement}/**` and `plugin/agents/*`. **Requires 0006** |`
with
`| `plugin/`
- **m11** @ docs/proposals/0003-design-quality-without-figma.md:94 (R1) vs :264 (§4.7) vs :348 (smoke precondition) — Quote verified: :94 reads "Run only when the feature has a user interface **and** no active task carries a Figma URL." The terminology defect is real and confirmed against the code — "active task" has
  *Fix:* Single edit at line 94. Replace
`- **R1 — Gate: UI **and** no Figma.** Run only when the feature has a user interface **and** no active task carries a Figma URL.`
with
`- **R1 — Gate: UI **and** no Figma.** Run only when
- **m12** @ docs/proposals/0003-design-quality-without-figma.md:281 (§4.9) vs :227 (§4.6 branch 3) — Both quotes verified verbatim. :281 asserts "so §4.6 branch 3 applies and it keeps today's acceptance-criteria verification"; :227 defines branch 3 as "**No references, no contract**" and :225 defines
  *Fix:* Two edits.

1. Line 281, replace the first sentence pair
`**This gate is not retroactive.** A feature planned before this PR has a `{base}/TECH_PLAN.md` with no `## Design Contract`, so §4.6 branch 3 applies and it keeps
- **m13** @ docs/proposals/0003-design-quality-without-figma.md:260 (§4.6 severity ladder) vs the check table at :237-250 — QUOTES VERIFIED. :260 reads verbatim "radius, elevation or easing inconsistency → Polish"; :193-194 reads "Easing — one documented curve per class (enter / exit / move). Bounce and overshoot are opt-i
  *Fix:* Add one row to the §4.6 check table, inserted between the `Transition durations within declared bands` row (:248) and the `Only transform/opacity animated` row (:249):

| Easing curve matches the declared curve for its c
- **m14** @ docs/proposals/0003-design-quality-without-figma.md:94 (R1, "no active task carries a Figma URL") — The quote is exact, and the terminology collision is real. `skills/belmont/_src/references/implement-milestone-template.md:26` defines `### Active Task IDs` as "[Comma-separated list of the **incomple
  *Fix:* In R1 at :94, replace "no active task carries a Figma URL" with "no task in `{base}/PRD.md` carries a Figma URL — every task, regardless of `[ ]`/`[x]`/`[v]` state". Repeat the same wording in the Phase 3.5 body when it 
- **m15** @ docs/proposals/0003-design-quality-without-figma.md:94 (R1, "the feature has a user interface") — QUOTE CHECK: accurate. :94 reads exactly "Run only when the feature has a user interface **and** no active task carries a Figma URL. Record the outcome in the contract's `**Mode**` field … Backend fea
  *Fix:* In §4.2, extend R1 (line 94) with an evidence rule, immediately after "Run only when the feature has a user interface **and** no active task carries a Figma URL.": add — "**Both halves are decided from the feature's acti
- **m16** @ docs/proposals/0003-design-quality-without-figma.md:301 (§5 verification-agent row) vs :235/:252/:260 (§4.6) and :419 (DoD) — Factually correct on every point I could check, including the history claim.

Spec :301 reads exactly as quoted and stops at "**rework the `## Output Format` Visual Verification block (`:210-221`)** —
  *Fix:* Two edits in §5 plus one DoD line.

1. §5 In-scope table — insert a new row immediately after the `skills/belmont/_src/references/models-yaml-format.md` row (:299):
"| `skills/belmont/_src/references/verify-report-format
- **m17** @ docs/proposals/0003-design-quality-without-figma.md:267 (§4.7 verify.md, "the **sub-agent dispatch prompt** (`:109`, `:146`)") — Confirmed. The spec at :267 reads: "the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behaviour by enumerating only Figma/screenshot references and injecting \"No d
  *Fix:* In docs/proposals/0003-design-quality-without-figma.md:267, replace the middle clause of the `_src/verify.md` bullet.

FROM:
  the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behavio
- **m18** @ docs/proposals/0003-design-quality-without-figma.md:247 (verification table, State Inventory row) — QUOTE CHECK: exact. Line 247 reads `| State Inventory entries present | \`browser_click\` / \`browser_hover\` / \`browser_press_key\` to enter each state + \`browser_take_screenshot\` |`. The load-bea
  *Fix:* Two coupled edits, both required.

(1) Replace line 247 in full with:
`| State Inventory entries present | Enter each state with the interaction it requires: \`browser_hover\` (hover) · \`browser_press_key Tab\` (focus) 
- **m19** @ docs/proposals/0003-design-quality-without-figma.md:397 (DoD, Placement and durability) — QUOTE CHECK: exact. :397 reads "- [ ] §4.1 states plainly that the rule is prose-enforced and unguarded, and does **not** claim `--assume-unchanged` as a backstop". The two facts it asserts are alread
  *Fix:* Replace line 397 with a PR-scoped, checkable item:
`- [ ] The PR description states plainly that the contract write rule is prose-enforced and unguarded, and claims no mechanical backstop — specifically does **not** cite
- **m20** @ docs/proposals/0003-design-quality-without-figma.md:364 (Step 2) — Quote is exact (:364 reads verbatim as claimed). The code claim holds and I reproduced it end-to-end rather than reasoning about it. `ensureStateFiles` runs unconditionally from the install path (cmd/
  *Fix:* In §8 Step 2, delete line 364 in full: "The interrupted run may leave `.belmont/auto.json` untracked, which `requireCleanWorkingTree` rejects on the re-run; delete it or pass `--allow-dirty`."  If a dirty-tree note is st
- **m21** @ docs/proposals/0003-design-quality-without-figma.md:348 (Step 1 preconditions) — Quote is exact: :348 reads `grep -c "Figma" .belmont/features/<slug>/PRD.md              # expect 0`. R1 is quoted correctly too (:94 'no active task carries a Figma URL'). The mismatch is real and th
  *Fix:* Replace line 348 with:

```
grep -ci 'figma\.com\|\*\*Figma\*\*:' .belmont/features/<slug>/PRD.md || true   # expect 0
```

and add one sentence after the fenced block: "R1 gates on a Figma **URL** or a `**Figma**:` task
- **m22** @ docs/proposals/0003-design-quality-without-figma.md:370 (Step 4) vs :252 (§4.6) — Both quotes are exact and neither is truncated in a way that changes meaning — :370 reads "shows per-row pass/fail/UNVERIFIABLE with a Mechanism column", :252 reads "the three motion rows are recorded
  *Fix:* In §8 Step 4 (line 370), replace "shows per-row pass/fail/UNVERIFIABLE with a Mechanism column" with: "shows per-row PASS / FAIL / N/A / UNVERIFIABLE with a Mechanism column — and if the contract's `**Applies**` is `N/A 
- **m23** @ docs/proposals/0003-design-quality-without-figma.md:413 (DoD) vs :139 (§4.3) — Both quotes are verbatim and complete. Line 413 reads "Every link in that table checked resolving at branch time — they are third-party URLs and this PR does not control them"; line 139 reads "**Never
  *Fix:* In §9 Definition of Done, under **Authority and portability**, replace the bullet at :413:

`- [ ] Every link in that table checked resolving at branch time — they are third-party URLs and this PR does not control them`

- **m24** @ §4.1, lines 67-78 (the two-row "State movement is bidirectional" table) — QUOTE CHECK — all three quotes are verbatim. Line 70: "as a **plain filesystem copy outside git**". Line 78: "so the *branch* cannot carry the edit — verified, even an explicit `git add` stages nothin
  *Fix:* Add a third row to the §4.1 table (after the `worktree → master` row), and one clarifying sentence. Row:

| **worktree → main via git** | `commitWorktreeChanges` (`main.go:10179`; `git add -A` at `:10192`) then `attemptM
- **m25** @ §4.5 line 217; §10 line 452; §8 Step 3 lines 366-368 — QUOTE CHECK — verbatim on both. Line 217: "...**unless `.belmont/` is gitignored**, a supported configuration in which that partial short-circuits entirely and neither contract nor preview is ever com
  *Fix:* Two edits, no new mechanism.

1. §4.5 line 217 — replace the final clause "Durability is unaffected (the copy paths are filesystem-level), but reviewability is; say so rather than promising a diff that may not exist." wi
- **m26** @ §4.6 check table lines 237-250 and severity line 260; §4.3 line 113 — QUOTE/COUNT CHECK: the table is header 237-238 plus twelve rows at 239-250. Line 260 assigns nine severities that land on eight of those rows (239 spacing, 241 contrast, 243 focus, 244 labels, 245 rad
  *Fix:* In `docs/proposals/0003-design-quality-without-figma.md` §4.6, extend the severity mapping so all twelve check rows are covered, and add a fallback clause. Apply ONLY this — do not adopt the ownership rule or the touch-t
- **m27** @ §4.7 line 267 — Quote check first: the finding's quotation of §4.7 line 267 is exact and unellipsed — "the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behaviour by enumerating on
  *Fix:* In /Users/benlavender/belmont/docs/proposals/0003-design-quality-without-figma.md, replace line 267 in full with:

- **`skills/belmont/_src/verify.md`** — three narrow edits, not one: Step 1b's collect list (`:62-72`) is
- **m28** @ §4.4 derivation ladder rung 0 (line 205); §4.4 `**Source**` enum (line 156); §10 risk row (line 450) — The factual core checks out and I could not break it. Verified:
- Line 205 is exactly "0. A `## Design Contract` in the **master** `.belmont/TECH_PLAN.md`." and line 156 lists `master TECH_PLAN` in th
  *Fix:* Apply option (b) only — four small edits, no scope change:
1. §4.4, delete line 205 ("0. A `## Design Contract` in the **master** `.belmont/TECH_PLAN.md`.") and renumber the existing `0b.` (line 206) to `0.`, keeping its
- **m29** @ §4.1 write rule (line 82); §5 In-scope table; §7 knowledge list (lines 334-335) — The quote at line 82 is accurate and the cited evidence is real (`references/debug-manual-spec-reconcile.md:17` is exactly the category-5 row permitting "Edit narrative in place" on `{base}/TECH_PLAN.
  *Fix:* Two edits to §4.1 only — no scope change:
1. Line 82, change the rule to: "> **Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`. The auto-loop phases — `implement`, `verify`, `next` and `debug-auto` — read it a
- **m30** @ §4.5 (line 215); §5 In-scope table; §7 docs list — Spec quote confirmed (line 215: "`tech-plan` also writes **`{base}/design-preview.html`**"), and reset.md/cleanup.md are indeed absent from §5. But the finding's central consequence is FALSE and its s
  *Fix:* Add ONE row to §5's In-scope table:

| `skills/belmont/_src/cleanup.md` | Add `design-preview.html` to the feature-dir file enumerations at `:49` (scan/count) and `:139` (archive deletion) |

And append one sentence to §
- **m31** @ §7 (lines 328-333); §12 minors (line 491, "`docs/` list extended to five pages") — Quote check passes. §7 (lines 328-333) lists exactly five surfaces — agent-pipeline.md, skills-reference.md, workflow.md, prd-format.md, README.md — plus AGENTS.md at :333; line 6 counts "5 docs pages
  *Fix:* In §7, insert after the `docs/prd-format.md` bullet (line 331):

`- \`docs/directory-structure.md\` — add \`design-preview.html\` to the \`.belmont/features/<slug>/\` tree (after the \`models.yaml\` line at \`:91\`), ann
