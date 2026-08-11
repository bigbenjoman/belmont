Domains: skills, agents, cli

# The ux-design phase: product-plan → ux-design → tech-plan

## Why this matters

Belmont planned in two phases — `product-plan` (what and why) and `tech-plan`
(how) — and design fell in the gap between them. PR #32 closed the gap by putting
the Design Contract in `tech-plan` Phase 3.5. That worked, and it cost three
things:

- **The authority ran uphill.** `tech-plan` asked *"design system details (colors,
  spacing, typography — especially if no Figma)"* as an interview input, then
  derived a contract from the answers. The design standard was a by-product of the
  implementation plan rather than a constraint on it.
- **A technical skill owned a human-approved artifact.** Every rule protecting the
  contract had to be section-scoped, because `tech-plan`, `debug-manual` and
  `review-plans` all legitimately write elsewhere in `TECH_PLAN.md`.
- **One skill carried two jobs and one approval gate.** `tech-plan` was 490 lines
  / 35.5 KB, the largest in the tree, on a codebase where PR #26 exists purely to
  cut skill tokens.

Splitting design out into `/belmont:ux-design` fixes all three and drops
`tech-plan` to roughly 337 lines / ~28 KB. The contract's own rules live in
[`design-authority.md`](design-authority.md); this entry is about the phase
boundary and the ordering hazards the split created.

## Invariant

- **Three planning phases, one artifact and one owner each.** `product-plan` →
  `{base}/PRD.md` (what and why). `ux-design` → `{base}/UX_DESIGN.md` plus
  `{base}/design-preview.html` and `{base}/ux-flows.html` (what it looks like and
  how it behaves). `tech-plan` → `{base}/TECH_PLAN.md` (how it is built). No skill
  writes another's artifact.
- **`ux-design` *renders* what the PRD settled; it never re-elicits it.** Flow
  **steps**, copy **content**, design intent, edge cases in user behaviour and any
  target WCAG level are all PRD answers. `ux-design` renders flows into
  `## Flows`, keyed to the PRD's BDD scenarios; writes Microcopy **Rules**, never
  the strings the PRD fixed; turns edge cases into State Inventory *entries*; and
  adopts the PRD's WCAG level (the floor may be raised, never lowered). Read them
  out of the PRD. Ask only to resolve a contradiction. Design intent is the
  highest-risk duplicate of the six and is the one to name explicitly.
- **`ux-design` owns five questions and only five:** the deployed-Storybook URL
  (the one rung nothing in the repo reveals), token values no ladder rung supplies
  (ratio, radius, accent hue), which surfaces are **reused unchanged** vs newly
  built, whether motion is in scope at all, and the approval itself. *Which*
  animation library is not one of them — that stays in `tech-plan`.
- **The authority inverts at `tech-plan`.** Token values, the Accessibility Floor,
  UX Strategy, the State Inventory's contents, and motion bands and easing move
  out of `tech-plan`'s ASK list into DO NOT RE-ASK. It asks only how to *express*
  them — theme layer, CSS variable naming, Tailwind theme keys, Framer Motion vs
  CSS vs GSAP within the fixed bands. Re-asking re-opens a human-approved
  decision.
- **The State Inventory's unit is an interactive *surface*, because the component
  list no longer exists upstream.** A surface is a control or region the user acts
  on, named from the PRD's flows and `UX_DESIGN.md`'s `## Screens` table, never
  from a file path or a React component name. The eight state names are unchanged
  — they are interaction-level. The scoping guard is unchanged in meaning and
  restated in the new unit: *a surface the feature reuses unchanged gets no
  entry*, which is what stops the inventory enumerating the whole app shell, and
  is determinable from the PRD plus the Storybook/config inventory with no file
  plan. Omissions still carry a one-line reason.
- **The two statements of that rule move in lockstep.**
  `references/design-authority-baseline.md` and
  `references/ux-design-format.md` state one rule twice; if they diverge, the
  ladder and the template disagree about what an entry is.
- **`tech-plan` closes the loop, report-only.** Its `## Component Specifications`
  maps each surface to one or more components, and it may not add, remove or
  rename a surface. An unmapped surface is a Phase 4.5 reconciliation finding:
  name it, tell the user to re-run `/belmont:ux-design --feature <slug>`, and
  never edit `UX_DESIGN.md`.
- **Ordering is advisory in both directions.** `tech-plan` notices a missing
  `UX_DESIGN.md` on a UI feature and offers to stop; it never blocks and never
  derives one. `ux-design` needs nothing from `tech-plan` — on a greenfield first
  feature ladder rungs 0–2 are all empty (`tailwind.config.*` and `globals.css`
  do not exist until code is written, whichever skill runs first), which is the
  expected case, not a regression: fall to rung 3 and set
  `**Source**: none — established here`. The contract records token **values**,
  not their expression.
- **Install surface eight, auto surface zero.** `ux-design` installs to all eight
  CLIs and is invoked by the auto loop on none of them. Do not conflate the two —
  `claudeOnlySkills` *removes* a skill from the shared `.agents/skills/` surface
  and is the wrong mechanism for keeping something off the auto surface.

## How it's enforced

- **Install is directory-driven**, so a new `_src/ux-design.md` reaches all eight
  tools for free: `syncSkillsFolderDir`, `syncSkillCommands`,
  `writeCodexSkillInterfaces` and `linkOpencodeCommands` all enumerate the skills
  dir. The only Go edits are two hardcoded *printed* skill lists in `install.go`,
  where `ux-design` sits between `product-plan` and `tech-plan`.
- **The hand-offs carry the ordering**: `product-plan`'s closing hand-off points
  at `/belmont:ux-design` (with the Codex `$belmont:ux-design` and opencode
  `/belmont/ux-design` forms), and `ux-design`'s points at `/belmont:tech-plan`.
- **`_partials/plan-separation.md` is three-way** — a `What belongs in UX_DESIGN`
  list beside the PRD and TECH_PLAN lists, so the boundary is stated once and
  included everywhere.
- **`tech-plan` carries three clauses**: the advisory Prerequisites bullet, the
  read-only ALLOWED ACTIONS rule covering both `UX_DESIGN.md` paths in every mode,
  and the Phase 4.5 seventh reconciliation item.
- **`_partials/user-questions.md`** names `ux-design` as a third Codex-fallback
  restart target. Without it, a Codex session that degrades to the plain-text
  fallback tells the user to restart the wrong skill.
- **`toolexec.go`'s auto-prompt state inventory** names `UX_DESIGN.md`. It reaches
  all seven auto-capable CLIs and is the only state hint Pi and opencode get after
  `adaptPromptForTool` rewrites the skill reference.
- **Tier 2 is mandatory.** The change is almost entirely prose, and nothing in
  Tier 1 reads a `SKILL.md`. See [`../meta/evals.md`](../meta/evals.md).

## Failure mode if you break it

- **Let `ux-design` re-ask the PRD.** The user answers flows, copy and intent
  twice, gives different answers the second time, and two approved documents now
  disagree with nothing to reconcile them.
- **Let `tech-plan` keep asking for design values.** The interview silently
  re-opens an approved decision, and whichever answer lands last wins — usually
  the technical one, which is the exact inversion the split undid.
- **Name State Inventory surfaces from component files.** They do not exist yet.
  The agent either invents an architecture to name them from, or writes an
  inventory it cannot fill — and `tech-plan`'s surface→component mapping has
  nothing to map.
- **Add `ux-design` to `claudeOnlySkills`.** That hides it from seven CLIs, which
  is a different goal entirely and breaks the phase for most of the install base.
- **Add an `actionUxDesign` so headless runs can produce a contract.** There is
  nobody to approve it. See `design-authority.md`.
- **Read the greenfield empty ladder as a bug and add a rung that needs
  `tech-plan`.** That re-creates the circular dependency the split removed.

## Don't re-do

- **Fold design into `product-plan` instead of a third skill.** Rejected — the two
  have different outputs, different question sets and different approval gates,
  and `product-plan` is already the interview a user runs first and longest.
  Design questions there compete with scope questions for the user's attention at
  the point they know least about the feature.
- **Move the file but leave derivation in `tech-plan`.** Rejected — it keeps the
  authority inversion (design derived from technical answers) and buys only a
  file-scoped guard. The ordering *is* the fix; the file is what makes the fix
  enforceable.
- **One combined HTML artifact, or one per screen.** Rejected both ways. The two
  pages have different content, different reviewers and different regeneration
  triggers — token/a11y/state changes regenerate `design-preview.html`,
  screen/flow changes regenerate `ux-flows.html` — and one combined page on a
  twelve-screen feature is unreviewable. A variable count is worse still: every
  enumeration of the artifact set is a literal list (`cleanup`, `reset`, the Codex
  packet, `docs/directory-structure.md`), and a variable count makes all four
  unstateable. Two fixed names is the largest count that keeps them literal.
- **Rename `design-preview.html`.** Rejected — three files quote it as a literal
  (`codex-plan-apply.md`'s carve-out sentence, `codex-plan-handoff.md`, the
  README) for zero benefit.
- **Map surfaces 1:1 to components.** Rejected — that is the architecture-dependent
  framing the ordering broke. One surface may become several components, and
  `tech-plan` owns that mapping.

## Evidence

- **The inventory really is derivable before `tech-plan`.** A deployed Storybook's
  `index.json` gives component titles (the `## Screens` surfaces) and story names
  (`Empty`, `Loading`, `Pending`, `Declined`) which are a real team's enumeration
  of the states a component actually has. It is the richest rung on the ladder and
  needs zero `tech-plan` input — a single URL fetch. `storybook.studia.io` returns
  104 components and 767 stories.
- **The token reduction is the largest available in the skill tree.** Phase 3.5
  plus the "Preserving the Design Contract (CRITICAL)" block plus the template's
  contract section is ~167 lines out of `tech-plan.md` for ~14 back.
- **The old State Inventory wording was architecture-dependent, in writing.**
  `design-authority-baseline.md` read *"per interactive component the feature
  **builds**"* and *"components the feature only **consumes** get no entry"* —
  both answered by `tech-plan`'s own "what existing components should be reused?"
  question, which now runs afterwards.

## Revisions

- 2026-08-11 — created when design authority was extracted into
  `/belmont:ux-design`. Records the three-phase boundary and what each phase may
  ask, the authority inversion at `tech-plan`, the State Inventory's unit change
  from component to surface (and why the ordering forced it), the greenfield
  empty-ladder case, the advisory-both-directions ordering, and the install-8 /
  auto-0 surface split.
