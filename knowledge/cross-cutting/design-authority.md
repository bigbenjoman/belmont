Domains: agents, skills

# Design authority: where it comes from when there is no Figma

## Why this matters

Belmont has three design states, and for a long time only two of them worked.

| Feature has | Design phase | Verification |
|---|---|---|
| No user interface | nothing to do — now genuinely skipped | acceptance criteria, correctly |
| UI **and** Figma URLs | design-agent extracts exact specs | pixel comparison against the design |
| **UI and no Figma** | four bullets that all deferred | **fell back to acceptance criteria** |

The third row was broken in a way that was easy to miss, because nothing errored.
`design-agent.md`'s old `## Handling No Design` told the agent to "recommend
following existing component patterns" — advice, not a specification. Verification
then hit `verification-agent.md`'s escape clause: *if no design references existed
at all, Visual Verification CAN pass based on acceptance criteria alone.* No
references is the **defining condition** of this state, so any design-quality gate
would have been opt-out by construction. The result was a UI shipped with no
design authority anywhere in the pipeline, and a green verify report saying so.

The fix is not a better prompt for the design agent. It is **placement**: derive
the standard once, per feature, interactively, where a human is already reviewing
— in a phase of its own, after `product-plan` and before `tech-plan`. See
[`ux-design-phase.md`](ux-design-phase.md) for why that phase exists and what it
may and may not ask.

## Invariant

- **A Design Contract is derived in `/belmont:ux-design`, once per feature, into
  `{base}/UX_DESIGN.md`, and only when the feature has a UI *and* no task in its
  PRD carries a Figma URL.** Check every task regardless of `[ ]`/`[x]`/`[v]`
  state — this is a feature-level gate, and keying it on the incomplete subset
  would flip it as work progresses.
- **`**Mode**` is the single machine-read signal, and silence is not allowed.**
  `derived — UI, no Figma` is a contract. `N/A — no UI` and `N/A — Figma present`
  are not. `UX_DESIGN.md` is written for **every** feature `ux-design` runs
  against, in all three modes, so an absent file means *`ux-design` never ran* —
  not "no UI". Downstream agents must be able to tell "not applicable" from
  "not done".
- **Never key on the `## Design Contract` heading — and never on the file
  existing either.** Because the file is written in all three modes, both the
  file and the heading are present on Figma features and on features with no UI
  at all. Discriminating on either demands contract checks on every Figma
  feature, finds none, and fires the fourth enforcement rule: an unconditional
  failure on every Figma milestone. A separate file makes existence-keying
  newly tempting; it is still wrong. Read `**Mode**`.
- **Only `/belmont:ux-design` writes `{base}/UX_DESIGN.md` or
  `.belmont/UX_DESIGN.md`.** The rule is now scoped to the **file**, not to a
  section — strictly simpler than the version it replaces, which had to be
  section-scoped only because other skills legitimately write elsewhere in
  `TECH_PLAN.md`. `tech-plan`, `implement`, `verify`, `next`, `review-plans` and
  `debug-manual` read it and never write it: it carries an approval stamp their
  flows do not renew. `debug-auto` neither reads nor writes it — see
  `Failure mode` for the gap that leaves. Six prose statements of the no-write
  rule move together or contradict each other — `tech-plan.md`,
  `review-plans.md`, `_partials/debug-scope-rules.md`,
  `references/debug-manual-spec-reconcile.md`, `docs/workflow.md` and
  `AGENTS.md`.
- **Headless `actionReplan` never touches the design authority.** `actionReplan`
  routes to `/belmont:tech-plan`, for which both `UX_DESIGN.md` paths are
  read-only **in every mode** — never create, never edit, never reformat, never
  restamp `**Approval**`. The old "fill in absent subsections headlessly,
  stamping `**Approval**: unreviewed (headless replan <date>)`" affordance lost
  its invoker when derivation moved out: nothing headless reaches `ux-design`.
  That stamp survives only as a **legacy value** on features planned before the
  split — preserve it byte-for-byte where you find it, and never write a new one.
- **`/belmont:ux-design` is interactive-only and is never invoked by the auto
  loop.** There is no `actionUxDesign`. Detecting headless, it reads nothing,
  writes nothing, asks nothing, prints one line and terminates normally. It still
  carries that clause despite having no auto invoker, because a user can type the
  bare programmatic form into a `-p` session and because for Pi and opencode
  `adaptPromptForTool` makes the skill body literally what the agent reads.
- **The gate is not retroactive, and the ordering is advisory.** A feature planned
  before contracts existed falls to the no-contract branch and keeps its existing
  verification. `tech-plan` *notices* a missing `UX_DESIGN.md` on a UI feature,
  says so, and offers to stop — it never derives or creates one. Adoption is
  opt-in by running `/belmont:ux-design --feature <slug>`.
- **A contract is derived ONCE. Re-running `ux-design` must not re-derive it.**
  The gate's third condition is no `**Mode**: derived — UI, no Figma` already in
  `{base}/UX_DESIGN.md` — **and** no `## Design Contract` left over in
  `{base}/TECH_PLAN.md` from before the split. Without the first half, every
  visit re-opens the approval interview, churns the judgement sections and
  restamps `**Approval**`, which can put already verified milestones out of
  compliance with a standard that moved under them. Without the second, every
  migrated feature gets a second, conflicting contract. Filling in *absent*
  subsections is fine; re-deriving needs an explicit ask.
- **Migration is offered, never performed silently, and never mints a second
  contract.** On finding a `## Design Contract` in `{base}/TECH_PLAN.md`,
  `ux-design` offers exactly two options through the structured question tool:
  **migrate** — move the section verbatim into `{base}/UX_DESIGN.md`,
  `**Approval**` byte-for-byte intact, and delete it from `TECH_PLAN.md`; or
  **leave it** — write nothing at all and report that consumers read
  `UX_DESIGN.md` now, so this feature falls to the no-contract branch. Never both
  files.
- **Headless detection must be observable by the agent.** Belmont emits no
  auto-mode marker and `buildLoopPrompt` produces a prompt byte-identical to what
  a user types, so "under `belmont auto`" is not a condition an agent can check.
  Use the idiom `debug-manual.md` already established: *bare programmatic syntax
  with no human prose, or no structured question tool* ⇒ non-interactive. And say
  explicitly that the general "degrade to plain-text questions" fallback in
  `user-questions.md` does **not** apply — a contract is an approval artifact and
  there is nobody to approve it headlessly. The paragraph lives in
  `_partials/headless-detection.md` and is `@include`d by both `ux-design` and
  `tech-plan`; two divergent copies of invariant-bearing wording is the failure
  the partial exists to prevent.
- **Figma extraction stays in `/belmont:tech-plan`, so `**Mode**` and the Figma
  tokens live in different files.** A Figma feature's `UX_DESIGN.md` carries
  `**Mode**: N/A — Figma present` plus the four header fields and nothing else.
  The extracted values live in `{base}/TECH_PLAN.md` under a top-level
  `## Design Tokens (from Figma)`, written only when Figma URLs exist, by the
  same Phase 1 extraction tech-plan already does. The asymmetry is deliberate —
  see `Don't re-do`. What must not happen is the tokens losing their home a
  second time: without that section a Figma feature silently loses its exact
  values from the plan, which is the regression the original entry was created to
  record.
- **The State Inventory's unit is an interactive *surface*, not a component.**
  `ux-design` runs before the component breakdown exists, so a surface is named
  from the PRD's flows and `UX_DESIGN.md`'s own `## Screens` table, never from a
  file path or a React component name. `tech-plan` maps each surface to one or
  more entries in `## Component Specifications` and may not add, remove or rename
  one. Full reasoning in [`ux-design-phase.md`](ux-design-phase.md).
- **No browser MCP at all is an environment gap, not an implementation defect.**
  Every row `UNVERIFIABLE`, Visual Verification INCOMPLETE, **one** Warning
  naming the missing tool — never thirteen Criticals blaming the code — and that
  Warning is the one deferrable contract finding. A browser MCP is recommended,
  not required, so failing every contract milestone on a browser-less install
  would make Belmont unusable for a supported configuration.
- **A deployed Storybook outranks local story files, and `index.json` is the way
  in.** Rung 1 detects both forms. A hosted Storybook's `<url>/index.json` is a
  static build artifact — fetching it runs nothing, needs no port, and starts no
  server, so the planning-session prohibition on build commands does not apply.
  Its `entries` give component titles (the reuse inventory) and story names
  (`Empty`, `Loading`, `Pending`, `Declined`) which **are** the State Inventory,
  written by the team that built the component. `ux-design` *asks* whether one
  exists — it is the one rung that cannot be discovered by reading the repo.
  The ladder is now walked only interactively, because `ux-design` refuses to run
  headlessly at all; keep the explicit interactive-only wording anyway, since it
  is what tells the agent the ask is legitimate. Skip `type: "docs"` entries;
  they are generated, not states.
- **A story index supplies components and states, never tokens — so `**Source**`
  names more than one rung.** The ladder stops at the first rung supplying a
  *token family*, and `index.json` supplies none, so the walk continues to rung 2
  while rung 1 still owns the inventory. `**Source**` therefore records one rung
  per family (`storybook (<url>) — components & states; tailwind.config.ts —
  tokens`). Naming a rung the tokens did not come from is not cosmetic:
  `verification-agent.md`'s anti-circularity rule is keyed on `**Source**`, so a
  token file that goes unnamed becomes legal to read back as an `Actual` value,
  and the check agrees with itself. For the same reason that rule is scoped to
  *anything* `**Source**` names — file, directory or URL — not to "the file".
- **Identify a URL as a Storybook before fetching it, and validate what comes
  back.** `homepage` and `repository` in `package.json` are the product site and
  the source repo; appending `/index.json` to either fetches something that is
  not a story index. And a fetch has succeeded only when the body parses as JSON
  carrying an `entries` (7+) or `stories` (6.x) object — a 200 alone proves
  nothing, exactly as with the client-rendered download links higher in the same
  file. Anything else falls through to local story files or project config.
- **A contract is never a fallback for a *failed* Figma load.** Mode is keyed on
  URL *presence*, never on load outcome. The NO FALLBACK rule stands untouched.

## How it's enforced

Prose across three layers, **one mechanical guard**, and one negative eval
fixture.

- **Authoring** — `_src/ux-design.md` (gate, migration prompt, derivation,
  structured approval, headless refusal) and
  `_src/references/ux-design-format.md` (the `UX_DESIGN.md` template) plus
  `_src/references/design-authority-baseline.md` (the per-section authority
  ladder, the derivation order, the tier-2 rules). The baseline moves into
  `ux-design/references/` for free: `generate-skills.sh` stage 3 copies a
  reference into the skill whose body names it, so it follows the mention.
- **The write instruction** — nothing to preserve any more. The contract used to
  live inside `tech-plan`'s **whole-file** feature template, so the preserve rule
  had to be restated in `tech-plan-feature-format.md` or Phase 4 destroyed the
  approved section on its way past. A dedicated file that only `ux-design` writes
  removes the hazard and both copies of the warning with it.
- **The ladder's rungs 0 and 0b** are `.belmont/UX_DESIGN.md` (master: Token
  Contract and Accessibility Floor only) and `.belmont/features/*/UX_DESIGN.md`.
  `worktree.go`'s `contextFiles` carries `UX_DESIGN.md` so every parallel
  worktree receives the master authority.
- **Consumption** — `design-agent.md` has a contract mode; `implement.md`
  Phase 2 has a fourth run-trigger; `implementation-agent.md` validates against
  the contract when the spec carries a `**Contract Source**` line, and falls back
  to `UX_DESIGN.md` when that line names a `TECH_PLAN.md` with no contract in it
  (archived `MILESTONE-*.done.md` files written before the split).
- **The write guard** — `runDesignAuthorityGuard` in `cmd/belmont/guards.go`,
  wired into `executeLoopAction` beside `runScopeGuard` and `runEvidenceCheck`:
  hash `{base}/UX_DESIGN.md` and `.belmont/UX_DESIGN.md` before the agent
  shell-out, compare after, restore on mismatch, best-effort amend the agent's
  commit, append a `(pending)` STEERING.md entry. It is **unconditional** — no
  auto action legitimately writes those files, because `ux-design` is not
  auto-invocable — so it needs no per-action exception table. It runs *inside the
  worktree*, which stops the edit before `syncFeatureStateAfterMerge` ever sees
  it and so sidesteps the `recoverMerge` gap a merge-path guard would have had to
  handle.
- **The gate** — `verification-agent.md` Phase 2 is three-way (references /
  contract / neither), branch 1 runs contract checks *in addition to* comparison,
  the `:131` escape clause is narrowed to "no references **and** no `derived`
  contract", and a fourth enforcement rule fails a milestone whose required
  contract checks were not performed.
- **Triage** — `prompts/belmont/post-verify-triage.md` lists contract a11y
  failures as never-deferrable, so the loop cannot quietly defer them. Its path
  must track `UX_DESIGN.md`: a stale one finds no contract, and every violation
  becomes deferrable.
- **Ordering** — advisory prose only, in `tech-plan`'s Prerequisites, in the same
  register as its existing "PRD is template-only" bullet. Not a lint: the gate's
  first condition is "a page, component, layout, style, or user-facing copy
  surface", which no Go parser can judge, and a `belmont validate` violation
  aborts `belmont auto` at startup — one mis-flagged backend feature would stop
  the whole run.
- **The fixture** — `cmd/belmont/testdata/eval/ui-no-figma/`, whose contract now
  lives in its `UX_DESIGN.md`. Its `badge.css`
  satisfies **every** acceptance criterion in its PRD while violating the contract
  (2.81:1 error text against a 4.5:1 floor; a semantic declaring a background but
  no border or text colour). That separation is deliberate: acceptance criteria
  alone cannot catch it, so only a verify that runs the contract checks can fail
  the milestone.

### The two clauses that keep the fourth rule satisfiable

Both were omitted in earlier revisions and both produced unconditional failures:

1. **"checks were required"** — a *focused re-verification* whose follow-ups did
   not touch UI is exempt, and records `Contract checks performed: NO — focused
   re-verification, no UI follow-ups`. This is the **only** exemption. Without the
   clause the rule fired on every legitimate focused re-verify.
2. **Branch 1 performs contract checks.** A feature can have both a reference and
   a contract (Step 2.0 counts screenshots and mockups as references). If branch 1
   skipped contract checks while the fourth rule demanded them, every such
   milestone failed.

### Severity, and why spacing is Polish

**Warning blocks the milestone and generates follow-ups; only Polish does not.**
Contrast, missing focus indicator and missing label are **Critical**. Missing
state and out-of-band duration are **Warning**. Off-scale spacing, radius,
elevation and easing are **Polish** — deliberately, because on any project with
pre-existing drift, grading them Warning would block the first milestone the gate
ever ran on.

## Failure mode if you break it

- **Widen the design-agent skip so a UI-no-Figma milestone stops running it.** The
  skip added in PR #26 fires only when *none* of {Figma URL, reference image,
  visible UI} is present, so it never touched this state. A contract is now a
  fourth trigger — an explicit, human-approved statement that this feature has a
  UI worth holding to a standard, which beats the three judgement calls. Widening
  the skip silently ignores the standard for exactly the milestones it was
  written for.
- **Rewrite the `[Not populated —` check to match exact wording.** It is a
  **prefix** match on purpose: it covers both reasons a section is empty
  (lightweight `/belmont:next` mode, and no design input). Matching the full
  lightweight-mode sentence silently breaks the skip path.
- **Assume a separate file is safe by itself, or neuter
  `runDesignAuthorityGuard`.** Extraction did not remove the hazard, only its
  shape. `copyBelmontStateToWorktree` copies the whole `{base}` dir, so
  `UX_DESIGN.md` travels into every worktree exactly like `TECH_PLAN.md` did, and
  `syncFeatureStateAfterMerge` copies the worktree's feature dir back over
  master's after every merge, outside git — every file but `PROGRESS.md`, which
  #28 merges via `mergeProgressState`. You lose the *approval*, not the edit: the
  worktree's version silently becomes main's. The guard is the only thing
  stopping that, and it stops it at the shell-out, not at the merge.
- **Trust `--assume-unchanged` as a write guard.** It stops the *branch* carrying
  the edit, but the merge copy is not git-aware, so the change ships anyway. It
  also covers only files already in the index, so a new `design-preview.html` or
  `ux-flows.html` is not covered at all.
- **Assume `debug-auto` honours the contract. It does not — known gap, not fixed
  by the extraction.** `_src/debug-auto.md` carries zero contract references, and
  `_partials/debug-phase1-design.md` skips design analysis whenever there are no
  Figma URLs — which is precisely the UI-no-Figma state. So an auto-loop debug
  round on a contract feature reasons about the code with the standard invisible
  to it, and can "fix" a symptom by moving a value off the token scale. `AGENTS.md`
  asserted the opposite until 2026-08-11; the claim was never true. Wiring it up
  is a behaviour change and needs its own Tier-2 eval, not a path repoint.
- **Let `tech-plan` become a writer again** — by "just filling in" a missing
  contract, by regenerating an artifact, or by restamping `**Approval**` after a
  reconciliation finding. `actionReplan` sends `tech-plan` into the headless loop,
  so any write permission it gains is a write permission the auto loop gains. A
  Phase 4.5 contract conflict is **report-only**: name the conflicting decision
  and tell the user to re-run `/belmont:ux-design --feature <slug>`.
- **Let `next.md` clobber the archive.** It overwrites
  `MILESTONE-<ID>.done.md` wholesale. Without the carry-forward rule, one fix
  round replaces populated per-task design specs with `[Not populated — …]` and
  the next verify has nothing to check against.

## Don't re-do

- **Infer design values on a *failed* Figma load.** Rejected — NO FALLBACK stands.
  A design exists and could not be read; guessing at it is worse than blocking.
  Mode is keyed on URL presence, never on load outcome.
- **Skip the design dispatch on a no-Figma UI feature.** Rejected. That is the
  state with the *most* design work to do, not the least.
- **Store the contract in the MILESTONE file.** Rejected — `next.md` overwrites
  the archive, so the contract would not survive a fix round.
- **A separate `DESIGN-CONTRACT-<M>.md`.** Rejected — *per-milestone*, so it had
  the same resume/merge profile as any other worktree-local state, with an extra
  file per milestone to keep in sync. Not what `{base}/UX_DESIGN.md` is: that is
  one durable per-feature file written by one interactive skill, and the reason it
  is now safe to have is the guard, which a milestone-scoped file could never have
  had (nothing knows which milestone's contract a phase may legitimately write).
- **Relax `design-agent.md`'s `## FORBIDDEN ACTIONS`.** Rejected twice. A
  *consuming* agent needs no new write permission; MILESTONE stays its only
  writable output.
- **Derive the contract per-milestone, inside a sub-agent.** Rejected — that was
  v1–v4 of the proposal. It puts derivation inside the implementation loop, into
  worktree-local state nothing durable owns and no human approves. Planning also
  does not reliably know which milestones are UI-bearing before the work is broken
  down, so milestone-level scoping cannot be decided when it would need to be.
- **A single available/unavailable switch for the design skills.** Rejected — a
  user holding three of the four would get the baseline for all four sections,
  and which sections were actually enriched would be invisible. The ladder is
  per-section and `**Authorities**` names the skill per section.
- **Vary the contract schema by which tier ran.** Rejected. Tier 2 is the tested
  path (all four tier-1 skills are user-scope *Claude Code* skills, absent for
  most users and on all seven other CLIs), so a schema that varied by tier would
  make the common case the unusual one. No downstream consumer branches on tier.
- **Vendor the Yummy Labs skill files.** Rejected — all four are distributed as
  downloads with no LICENSE, no repository and no redistribution grant. Belmont
  states the same published standards in its own words and credits the source.
- **Apply the gate retroactively to existing features.** Rejected — it would fail
  milestones on a standard their author never agreed to. Adoption is opt-in by
  running `/belmont:ux-design --feature <slug>`.
- **Move Figma extraction into `ux-design` along with everything else.**
  Rejected, deliberately, when the phase was split out. The tidy-looking version
  put `### Design Tokens (from Figma)` in `UX_DESIGN.md` so one skill owned all
  design values; the accepted version keeps extraction in `tech-plan` Phase 1
  exactly as it was and puts the values in `{base}/TECH_PLAN.md` under a
  top-level `## Design Tokens (from Figma)`. So `**Mode**` lives in
  `UX_DESIGN.md` while a Figma feature's tokens live in `TECH_PLAN.md` — an
  asymmetry worth stating plainly, because it looks like an oversight and is not.
  The reasoning: extraction is a working MCP integration on the one path that was
  never broken, and moving it buys nothing for the UI-no-Figma state this whole
  entry exists for while putting a live Figma path through a rewrite. It also
  keeps `ux-design` free of any MCP dependency, so the skill that must run on all
  eight CLIs has no tool-permission precondition. The cost is one asymmetry;
  the alternative was risking the row that already worked.
- **A `belmont migrate` CLI command for the PR #32 → `UX_DESIGN.md` move.**
  Rejected. Migration is an interactive prompt inside `ux-design`, offered when it
  finds a `## Design Contract` in `{base}/TECH_PLAN.md`. A contract is an approval
  artifact, so moving one is a decision a human should see; and a CLI command
  would need its own Markdown section parser, its own tests, and a headless path
  into files nothing headless is allowed to write. Declining leaves both files
  untouched and `ux-design` exits without writing.
- **Hard-block `tech-plan` when `UX_DESIGN.md` is missing on a UI feature.**
  Rejected. `tech-plan` is re-run routinely (add a milestone, reconcile drift,
  replan), so a blocking prerequisite stops every routine re-run on every
  unmigrated project; it is retroactive, which is already rejected above; and
  `actionReplan` sends `tech-plan` into the headless loop, turning a prose
  ordering preference into a production loop failure. Advisory, non-blocking, and
  never derive one.
- **A mechanical guard on the merge path.** ~~Deferred~~ — **superseded: the
  guard is implemented.** The deferral reasoned about a *section*-scoped guard,
  which meant a Markdown parser running in `syncFeatureStateAfterMerge`, on the
  path every feature depends on. File-scoping collapsed that to
  `sha256.Sum256` of one file, and the cheapest placement turned out not to be
  the merge path at all: `runDesignAuthorityGuard` hashes both `UX_DESIGN.md`
  paths around each agent shell-out in `executeLoopAction`, reusing the
  restore-amend-steer mechanism `runScopeGuard` and `runEvidenceCheck` already
  have. That also answers the old note that `recoverMerge` never calls
  `syncFeatureStateAfterMerge` — the guard runs inside the worktree, before
  either path is reached.

## Evidence

- **The escape clause was real.** `verification-agent.md:131` (pre-change) read:
  *"If no design references existed at all, Visual Verification CAN pass based on
  acceptance criteria alone."* No references is the defining condition of the
  UI-no-Figma state, so the gate had to be narrowed rather than added alongside.
- **PR #26's skip does not cover this state.** Its condition fires only when none
  of {Figma URL, reference image, visible UI} is present. The `visible UI` clause
  fires here, so the agent still runs — the backend row was fixed, this row was
  untouched. Worth restating because "the design agent problem is solved" is an
  easy misreading of that change.
- **State movement is bidirectional, and the risk is the opposite of the obvious
  one.** `copyBelmontStateToWorktree` wipes and re-copies master → worktree;
  `syncFeatureStateAfterMerge` replaces master's feature dir with the worktree's
  after every merge (every file but `PROGRESS.md`, which #28 merges instead),
  outside git, and `commitBelmontState` then commits it. So a
  worktree edit is **not** lost — it silently replaces the approved contract on
  main. `UX_DESIGN.md` rides both copies exactly as `TECH_PLAN.md` does, which is
  why splitting the file out was never on its own a durability fix.
- **The eval fixture's numbers, computed not asserted:** `#9a9a9a` on `#ffffff` is
  2.81:1; the ok and warn variants are 8.15:1 and 7.38:1, so the failure is
  isolated to one variant and cannot be mistaken for a broken palette.
- **Tier 1 cannot license any of this.** The prose is the change, and nothing in
  Tier 1 reads a `SKILL.md`. The fixture's Tier-1 assertions were still control-
  tested (flip a task to `[v]` → parse fails; rename the milestone to a polish
  pattern → validate fails), but they pin parse/decision behaviour, not prose.
  Only Tier 2 or a manual run can say whether the gate works.
  `runDesignAuthorityGuard` is the one part that Tier 1 *can* cover, because it is
  Go — and covering it there is not evidence about anything else in the change.

## Revisions

- 2026-08-11 — design authority extracted out of `/belmont:tech-plan` into a new
  `/belmont:ux-design` skill running between `product-plan` and `tech-plan`, and
  out of `{base}/TECH_PLAN.md` into `{base}/UX_DESIGN.md` (plus two artifacts:
  `design-preview.html`, unrenamed, and the new `ux-flows.html`). The
  single-writer rule became file-scoped, `tech-plan` became read-only in every
  mode, and the headless "fill in absent subsections" affordance was deleted
  because nothing headless reaches `ux-design` any more. Three decisions worth
  keeping: Figma extraction deliberately **stayed** in `tech-plan`, so `**Mode**`
  and a Figma feature's tokens live in different files; migration of a PR #32
  contract is an interactive prompt, not a CLI command; and the deferred
  mechanical guard is now implemented as `runDesignAuthorityGuard` — file-scoping
  made it a hash rather than a parser, and `executeLoopAction` a better home than
  the merge path. Also recorded the State Inventory's unit changing from
  "component" to "surface", which the new ordering forced (see
  [`ux-design-phase.md`](ux-design-phase.md)), and corrected a false claim
  inherited from `AGENTS.md`: `debug-auto` never read the contract, and still
  does not.

- 2026-08-09 — hardened the deployed-Storybook rung after a second red-team (140
  agents, 8 lenses, 44 findings, 41 refuted). One survivor and two convergences
  fixed: `**Source**` was told to name rung 1a even though a story index supplies
  no token family, which wrote false provenance *and* unkeyed the verifier's
  anti-circularity rule — `**Source**` now names one rung per family and the rule
  is scoped to anything it names, not to "the file". The Storybook ask gained an
  explicit interactive-only carve-out in both Phase 3.5 and the ladder (three
  lenses independently read the headless path as reachable; the refuters were
  right that an existing rule covers it, but only by inference). Detection no
  longer treats `homepage`/`repository` as Storybook URLs, and a fetch now has a
  positive shape test rather than a two-item failure list.
- 2026-08-08 — rung 1 extended to deployed Storybooks after a real one
  (storybook.studia.io) exposed the gap: detection was local-files-only, so a
  project that ships Storybook as a deployment had its richest design source
  invisible to the ladder. `index.json` there returns 104 components and 767
  stories. The original "do NOT run Storybook" rule was right about a local
  build and wrongly excluded the hosted case, which requires no running at all.
- 2026-08-08 — hardened after a 108-agent adversarial red-team (49 findings, 39
  refuted, 10 survived, zero blockers) plus an independent Fable senior review.
  Four majors fixed, all self-inflicted by the first draft: the gate re-derived an
  existing contract on every `tech-plan` re-run; the headless clause's trigger was
  not observable by the agent it instructs; the Phase 4 write instruction forbade
  writing the Figma tokens Phase 3.5 requires *and* the template had lost their
  slot, silently regressing the Figma path; and `next.md` carried a populated
  design spec forward then told the sub-agent it was empty. Also resolved the
  browser-less-install question four lenses raised independently.
- 2026-08-11 — rebased onto v0.10.17. #28 made `syncFeatureStateAfterMerge` merge
  `PROGRESS.md` instead of replacing it, so the "whole feature dir is replaced"
  claim is now qualified in both places it appears. The contract lives in
  `TECH_PLAN.md`, which is still taken wholesale, so the unguarded-durability
  position is unchanged.
- 2026-08-08 — created when landing the Design Contract. Records the three design
  states and which was broken; the placement argument (derive once, interactively,
  in tech-plan) over the per-milestone sub-agent alternatives; `**Mode**` as the
  single signal and the never-key-on-the-heading rule; the two clauses that keep
  the fourth enforcement rule satisfiable; the prose-only, unguarded durability
  position with the mechanical guard deferred; and the eval fixture that separates
  acceptance criteria from contract compliance so the gate can actually fail.
