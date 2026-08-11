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
the standard once, per feature, interactively, where a human is already reviewing.

## Invariant

- **A Design Contract is derived in `/belmont:tech-plan` Phase 3.5, once per
  feature, and only when the feature has a UI *and* no task in its PRD carries a
  Figma URL.** Check every task regardless of `[ ]`/`[x]`/`[v]` state — this is a
  feature-level gate, and keying it on the incomplete subset would flip it as work
  progresses.
- **`**Mode**` is the single machine-read signal, and silence is not allowed.**
  `derived — UI, no Figma` is a contract. `N/A — no UI` and `N/A — Figma present`
  are not, and neither is an absent section. Downstream agents must be able to
  tell "not applicable" from "not done".
- **Never key on the `## Design Contract` heading.** The feature template carries
  it unconditionally, so a *Figma* feature's plan has the heading too — holding
  the Figma tokens tech-plan extracted in-session. Discriminating on the heading
  demands contract checks on every Figma feature, finds none, and fires the
  fourth enforcement rule: an unconditional failure on every Figma milestone.
- **Only the `tech-plan` skill writes the `## Design Contract` section.** The rule
  is scoped to the **section**, not the file, because `debug-manual` legitimately
  edits other TECH_PLAN prose under per-edit approval and `review-plans` offers
  rewrites elsewhere. Neither may touch this section: it carries an approval stamp
  their flows do not renew.
- **Headless `actionReplan` preserves and never creates.** It may fill in
  *subsections* absent from a contract that already exists, stamping
  `**Approval**: unreviewed (headless replan <date>)`. If there is no contract at
  all, it derives nothing.
- **The gate is not retroactive.** A feature planned before contracts existed
  falls to the no-contract branch and keeps its existing verification.
- **A contract is derived ONCE. Re-running `tech-plan` must not re-derive it.**
  The gate has a third condition — no `derived` contract already present. This
  skill is re-run routinely (add a milestone, reconcile drift, replan), and
  without that condition every visit re-opens the approval interview, churns the
  four judgement sections and restamps `**Approval**` — which can put already
  verified milestones out of compliance with a standard that moved under them.
  Filling in *absent* subsections is fine; re-deriving needs an explicit ask.
- **Headless detection must be observable by the agent.** Belmont emits no
  auto-mode marker and `buildLoopPrompt` produces a prompt byte-identical to what
  a user types, so "under `belmont auto`" is not a condition an agent can check.
  Use the idiom `debug-manual.md` already established: *bare programmatic syntax
  with no human prose, or no structured question tool* ⇒ non-interactive. And say
  explicitly that the general "degrade to plain-text questions" fallback in
  `user-questions.md` does **not** apply — a contract is an approval artifact and
  there is nobody to approve it headlessly.
- **A Figma feature's extracted tokens live under `### Design Tokens (from Figma)`
  inside the `## Design Contract` section.** Generalising the old
  `## Design Tokens (from Figma)` heading removed their only home; the write
  instruction must put them back or Figma features silently lose their exact
  values from the plan. This is also the *stated rationale* for the
  never-key-on-the-heading rule, so dropping them falsifies the change's own
  justification.
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
  written by the team that built the component. Phase 3.5 *asks* whether one
  exists — it is the one rung that cannot be discovered by reading the repo —
  but **only interactively**. A headless replan may fill an absent subsection,
  which walks the same ladder, so the ask needs its own carve-out: headless uses
  only a URL already in the master plan, the PRD or `package.json`, and skips the
  rung otherwise. Skip `type: "docs"` entries; they are generated, not states.
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

Prose across three layers, plus one negative eval fixture. **There is no
mechanical guard — see `Don't re-do`.**

- **Authoring** — `_src/tech-plan.md` Phase 3.5 (gate, derivation, structured
  approval, headless clause) and `_src/references/design-authority-baseline.md`
  (the per-section authority ladder, the derivation order, the tier-2 rules).
- **The write instruction** — `_src/references/tech-plan-feature-format.md` is a
  **whole-file template**, so the preserve rule is stated *there too*, not only in
  Phase 3.5. Without that, R4 is bypassed by construction: Phase 4 writes the
  template and destroys the approved section on its way past.
- **Consumption** — `design-agent.md` gains a contract mode; `implement.md`
  Phase 2 gains a fourth run-trigger; `implementation-agent.md` validates against
  the contract when the spec carries a `**Contract Source**` line.
- **The gate** — `verification-agent.md` Phase 2 is three-way (references /
  contract / neither), branch 1 runs contract checks *in addition to* comparison,
  the `:131` escape clause is narrowed to "no references **and** no `derived`
  contract", and a fourth enforcement rule fails a milestone whose required
  contract checks were not performed.
- **Triage** — `prompts/belmont/post-verify-triage.md` lists contract a11y
  failures as never-deferrable, so the loop cannot quietly defer them.
- **The fixture** — `cmd/belmont/testdata/eval/ui-no-figma/`. Its `badge.css`
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
- **Let a worktree phase write the section.** `syncFeatureStateAfterMerge` copies
  the worktree's feature dir over master's after every merge, as a plain
  filesystem copy outside git. You lose the *approval*, not the edit — the
  worktree's version silently becomes main's. `PROGRESS.md` is the one exception
  (#28 merges it via `mergeProgressState`); `TECH_PLAN.md`, and so the contract,
  is still taken wholesale.
- **Trust `--assume-unchanged` as a write guard.** It stops the *branch* carrying
  the edit, but the merge copy is not git-aware, so the change ships anyway. It
  also covers only files already in the index, so a new `design-preview.html` is
  not covered at all.
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
- **A separate `DESIGN-CONTRACT-<M>.md`.** Rejected — same resume/merge profile as
  any other worktree-local state, with an extra file to keep in sync.
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
  re-running `/belmont:tech-plan --feature <slug>`.
- **A mechanical guard on the merge path, in this PR.** Deferred, not rejected. It
  would mean snapshotting the section before a wave and restoring it in
  `syncFeatureStateAfterMerge` — a Go change on the merge path every feature
  depends on, which does not belong in a prose-only design-quality change. Note
  that `recoverMerge` never calls `syncFeatureStateAfterMerge` at all, so a guard
  there would need to handle that path too.

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
  main.
- **The eval fixture's numbers, computed not asserted:** `#9a9a9a` on `#ffffff` is
  2.81:1; the ok and warn variants are 8.15:1 and 7.38:1, so the failure is
  isolated to one variant and cannot be mistaken for a broken palette.
- **Tier 1 cannot license any of this.** The change is prose-only, and nothing in
  Tier 1 reads a `SKILL.md`. The fixture's Tier-1 assertions were still control-
  tested (flip a task to `[v]` → parse fails; rename the milestone to a polish
  pattern → validate fails), but they pin parse/decision behaviour, not prose.
  Only Tier 2 or a manual run can say whether the gate works.

## Revisions

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
