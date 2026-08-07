# PR 1 — Design quality for UI features without Figma

**Revision 7.** Rev 6 was reviewed by six adversarial lenses: 69 findings raised, all 69 individually refuted, **53 confirmed and 16 refuted**. v7 fixes the 53. Its one blocker was a re-run of the failure rev 6 believed it had fixed — see §12. Evidence: [`rev6-CONFIRMED-DEFECTS.md`](rev6-CONFIRMED-DEFECTS.md), with rev 5's record in [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md).

**Type:** prose only — 3 agents, 7 skill sources, 2 existing skill references, 1 new skill reference, 1 Go-embedded prompt = **14 tracked source files**. **No Go changes.**
**Size:** ~600 lines net across those 14 (§5) + regenerated `plugin/` + 5 docs pages + 1 new knowledge entry + 1 `KNOWLEDGE.md` routing row + 3 corrected statements in `AGENTS.md`/knowledge. *(Counted from the §5 table, not asserted — rev 5 claimed 9 against a table of 10, and this figure was itself wrong at 11 on first draft.)*
**Sequencing:** depends on **0006** (PR #21, open). Lands **after 0005 and 0004**. This PR owes 0004's post-contract re-baseline (§11).

> **All `main.go:NNNN` citations in this document are pre-0005.** They were resolved against `d1236f1`, where `main.go` is 12,613 lines. 0005 lands first and splits that file, so **every line number here will be wrong by the time this is implemented**. Function names are the durable identifier and are given first throughout; treat line numbers as a hint for locating them pre-split. Re-resolve at branch time with `grep -n 'func <name>'`.

---

## 1. Problem

Three states exist, and only one is broken:

| Feature has | Today | Status |
|---|---|---|
| No user interface | Nothing to design | Fine |
| UI **and** Figma URLs | design-agent extracts exact specs | Works |
| **UI and no Figma** | `## Handling No Design` (`design-agent.md:270-276`) — four bullets that all defer | **Broken** |

This PR addresses the third row only. Earlier revisions said "no Figma", which wrongly swept in backend features that need nothing.

On a UI feature with no Figma:

1. **The design phase is a paid no-op.** A full sub-agent reads its agent file, MILESTONE and the PRD to return "no design references were provided."
2. **No design quality gate exists.** `verification-agent.md` Phase 2 is comparison-only — its check target is a loaded Figma screenshot or image (`Step 2.1`, `Step 2.4`). With nothing to compare against it falls back to acceptance criteria, a correctness check rather than a design-quality one.
3. **The fallback is explicitly permissive.** `verification-agent.md:131` lets visual verification pass on acceptance criteria alone whenever no references exist — which is the defining condition of this case, so any new gate would be opt-out by construction.

## 2. Root cause — state this first in the PR description

Two failures, conflated:

- **Figma load *failed*** — a real design exists and we could not read it. Guessing is dangerous; the NO FALLBACK rule (`design-agent.md:61-68`, `:255`) is **correct and untouched by this PR**.
- **Figma *absent*, but a UI is being built** — nothing exists to be faithful to, so the same prohibition leaves the pipeline with no design authority at all.

The second root cause is **placement**: v1–v4 derived design per-milestone, in a sub-agent, inside the implementation loop, into worktree-local state that nothing durable owns and no human approves.

## 3. Rationale and attribution

**Derive once, where a human is already reviewing.** `tech-plan` is interactive by construction, already asks about "Design system details (colors, spacing, typography — **especially if no Figma**)" (`tech-plan.md:197`), already lists "Styling & design system … design tokens; theme layer" as a domain (`:70`), already asks "What existing components/patterns should be reused?" (`:196`), and its feature template already has `## Existing Components to Reuse`. The capability exists and is under-specified. This PR hardens it.

**Checking is its own job.** Belmont separates implementation from verification, but with no design reference the checker has nothing to check against — structurally present, substantively empty. A contract supplies the objective standard.

**Design content source and attribution.** The rules in `references/design-authority-baseline.md` are Belmont-original prose stating published standards: WCAG 2.1 **SC 1.4.3** (contrast 4.5:1 body / 3:1 large, **Level AA**), **SC 2.4.7** (focus visible, **AA**), **SC 2.5.5** (44×44 targets, **Level AAA**), **SC 2.3.3** (animation from interactions, **Level AAA**), the 8pt spacing grid, ratio-derived type scales, and the 60-30-10 colour convention. **State the conformance level wherever these are cited** — two of the four are AAA, and presenting AAA criteria as a mandatory floor without saying so misrepresents the standard. The baseline adopts them as *Belmont's* floor, which is a project choice, not a WCAG requirement.

The *curation* — which rules matter and how they compose into a checkable contract — is drawn from the **[Yummy Labs](https://www.yummy-labs.com/) Claude design skills**. Credit them by name and link in the reference file header, in `design-authority.md`, and in the PR description. §4.3 carries the same links with install paths:

| Skill | Contract section it feeds |
|---|---|
| [ui-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | Token Contract; the "existing design system is LAW" rule; the banned-defaults list |
| [ux-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | UX Strategy; the component-state half of State Inventory |
| [ux-copywriter](https://drive.google.com/drive/folders/1lSwUatVOzOX5TGWgBDjKA820RiUsVLNr) | Microcopy Rules |
| [ux-motion](https://drive.google.com/drive/folders/1gYrK4aT4A-LYr4GbM88-PaKu0wYyY4C6) | Motion Contract; the transition half of State Inventory |

**Do not vendor their files.** All four are distributed as downloads with **no LICENSE, no repository and no redistribution grant** — verified against the actual packages, not the website. Belmont states the same published standards in its own words and credits the source. If written permission is obtained (`yummylabs@yummydesign.xyz`), vendoring with attribution can replace the baseline in a follow-up.

## 4. Design

### 4.1 Where the contract lives, and what actually protects it

The contract is written into **`{base}/TECH_PLAN.md`** (`.belmont/features/<slug>/TECH_PLAN.md`) by `/belmont:tech-plan`, in the master tree, before `belmont auto` starts.

**State movement is bidirectional.** Rev 5 claimed worktree-authored content is destroyed. That is true in one direction only, and the omission inverted the risk:

| Direction | Function | Effect |
|---|---|---|
| master → worktree | `copyBelmontStateToWorktree` (`main.go:9980`; `os.RemoveAll` at `:10010`, `copyDir` at `:10011`) | Wipes the worktree's feature dir and re-copies from master, preserving only `STEERING.md`. Unconditional on the single-feature milestone path (`:9042`); `if !resumed` on the multi-feature path (`:6178-6195`) |
| **worktree → master** | **`syncFeatureStateAfterMerge`** (`main.go:10160`; `os.RemoveAll` on **master** at `:10169`, `copyDir` from the **worktree** at `:10170`) | After every successful merge, replaces master's feature dir with the worktree's, as a **plain filesystem copy outside git**. Called from `mergeWorktreeBranch` (`:9157`) and `mergeFeatureBranch` (`:6297`). `commitBelmontState` (`:10211`) then commits it to main |

So the real failure mode is the opposite of the one rev 5 documented:

> **A worktree phase that edits `{base}/TECH_PLAN.md` does not lose its edit. It silently replaces the human-approved contract on main.** You lose the approval, not the edit.

Measured, not argued — replaying the call sequence over a real worktree produced `WORKTREE-EDITED CONTRACT` on main, committed as `belmont: update state files`.

**`--assume-unchanged` is not a backstop.** `untrackBelmontInWorktree` (`main.go:10104-10122`) marks tracked `.belmont/` files assume-unchanged, so the *branch* cannot carry the edit — verified, even an explicit `git add` stages nothing. But the copy above is not git-aware, so the change ships anyway. Two further limits: it covers only files **already in the index**, so a *new* file such as `design-preview.html` is not covered at all; and its companion `writeWorktreeGitExcludes` (`main.go:10057`) writes `.belmont/` into the per-worktree `$GIT_DIR/info/exclude` while git reads `$GIT_COMMON_DIR/info/exclude`, so it does nothing (an out-of-scope `main` bug, §5).

**Therefore, the rule of this PR — and what enforces it:**

> **Only the `tech-plan` skill may write the `## Design Contract` section of `{base}/TECH_PLAN.md`. `implement`, `verify`, `next` and `debug-auto` read it and never write it.** When `tech-plan` itself runs headlessly as `actionReplan`, R4 constrains what it may change.

The rule is scoped to the **section**, not the file, because two existing skills legitimately write elsewhere in `TECH_PLAN.md` and this PR must not silently forbid them:

- **`/belmont:debug-manual`** is documented as the one skill permitted to edit spec prose in place, under per-edit user approval (`knowledge/cross-cutting/debug-spec-reconciliation.md`, and the callout in `AGENTS.md`). It runs interactively in the master tree. It may continue to edit TECH_PLAN prose; it may **not** edit `## Design Contract`, because that section carries an approval stamp its per-edit flow does not renew.
- **`/belmont:review-plans`** loads every feature `TECH_PLAN.md` and offers rewrite options for drift and conflicts. It must treat `## Design Contract` as **read-only** and emit a finding rather than a rewrite. In scope (§5).

**This is prose-enforced and unguarded, and the PR says so plainly.** Nothing mechanical prevents a worktree phase from writing that path: `runScopeGuard` parses `PROGRESS.md` only and returns early for `actionReplan` (`main.go:12065-12068`), and the merge copy is a filesystem operation. The detection is smoke Step 3's content hash (§8), not git.

*Author decision, 2026-08-07: prose rule now, mechanical guard later.* A guard — snapshot `## Design Contract` before the wave, restore it in `syncFeatureStateAfterMerge` — would be a **Go change** on the merge path every feature depends on. It is deferred to its own proposal rather than smuggled into a design-quality PR. Also unhandled there: `recoverMerge` (`main.go:11156-11196`) never calls `syncFeatureStateAfterMerge` at all.

**`design-agent.md`'s `## FORBIDDEN ACTIONS` (`:5-15`) is untouched.** Its only writable output stays MILESTONE `## Design Specifications`. A consuming design-agent needs no relaxation.

### 4.2 The tech-plan design step

Insert **`### Phase 3.5 — Design Contract`** into `skills/belmont/_src/tech-plan.md` between `:217` and `:219` — after Phase 3's interview (it consumes the `:197` answers) and before Phase 4 writes the plan.

- **R1 — Gate: UI **and** no Figma.** Run only when the feature has a user interface **and** no task in `{base}/PRD.md` carries a Figma URL — *every* task, regardless of `[ ]`/`[x]`/`[v]` state. Do **not** say "active task": `### Active Task IDs` is defined in `implement-milestone-template.md:26` as the incomplete tasks of one milestone, and keying the feature-level gate on a per-milestone subset would flip the gate as work progresses. Record the outcome in the contract's `**Mode**` field, which is the single machine-read signal (§4.4) — there is no separate `**Design contract**` field. Backend features record `N/A — no UI`; Figma features record `N/A — Figma present`. **Silence is not allowed**, so downstream agents can distinguish "not applicable" from "not done".
  *Author decision, 2026-08-07: the contract exists only for UI-without-Figma.* Figma features keep today's behaviour untouched.
  **Scope is the whole feature, one conversation** (author decision): the contract covers every milestone, including ones that turn out to be backend-only. Milestone-level scoping was rejected — planning does not reliably know which milestones are UI-bearing before the work is broken down.
  **Mixed Figma coverage.** A feature counts as a Figma feature if **any** task carries a Figma URL, even where other UI tasks carry none. Those uncovered tasks keep today's per-task no-Figma behaviour: a partially-covered feature has no single design authority to reconcile against, and inventing one would contradict the Figma tokens already extracted for its covered tasks. The replacement for `## Handling No Design` (§4.7) **must therefore keep a no-contract arm** — it cannot assume a contract exists whenever Figma is absent from a task.
- **R2 — Figma detection only, no loading.** The step needs to know *whether* Figma URLs exist, not what is in them. Do not load Figma here. (If Figma is loaded elsewhere in tech-plan it must stay **in-session** — `tech-plan.md:158`: "sub-agents cannot get MCP tool permissions approved.")
- **R3 — Structured questions or stop.** The approval step inherits `user-questions.md:8` ("NEVER ask questions as plain inline text when a structured question tool exists") and the strict Codex fallback at `:11`. A Markdown pick-list is not an approval gate.
- **R4 — Headless behaviour is defined, not implied.** Under `belmont auto`, `actionReplan` invokes `/belmont:tech-plan` (`buildLoopPrompt`, `main.go:7188-7189`) with no user and no `AskUserQuestion`. The step must: **preserve an existing `## Design Contract` verbatim**, deriving only sections that are absent; mark it `**Approval**: unreviewed (headless replan <ISO date>)`; regenerate `design-preview.html` in the same step if and only if a section changed, stamping it `<!-- unreviewed: headless replan <ISO date> -->`; and never silently regenerate an approved contract. **And never *create* one:** if `{base}/TECH_PLAN.md` carries no `## Design Contract` at all, the headless step derives nothing and leaves the design surface untouched. "Deriving only sections that are absent" means subsections of a contract that already exists, never the contract itself — otherwise a headless replan would silently adopt a pre-contract feature into a gate its author never approved, contradicting §4.9. Contract authoring is interactive only. Nothing mechanical catches a violation — see §4.1.
- **R5 — Codex packet.** The contract and preview must be enumerable as `BELMONT_PLAN_PACKET` operations (`tech-plan.md:37-45`). `codex-plan-apply` applies **two** gates: a path constraint (satisfied — both are under `.belmont/`) and a **source-code content prohibition** which a self-contained `.html` would trip. `_src/codex-plan-apply.md` is therefore in scope (§5) to carve out planning artifacts explicitly.
- **R6 — Update CRITICAL RULE 6.** `tech-plan.md:19` names Phase 4.5 as the last mandatory phase and is *already* stale with respect to Phase 4.6. Adding Phase 3.5 without fixing it leaves three contradictory statements of when the skill terminates.
- **R7 — Extend ALLOWED ACTIONS.** `tech-plan.md:96-105` enumerates the write surface exhaustively; add `{base}/design-preview.html`. **Do not touch the shared `forbidden-actions.md` partial** — other skills include it. The preview is a planning artifact under `.belmont/`, never project source; CRITICAL RULE 2 stands unchanged for `.tsx/.ts/.css`.
- **R8 — The rule must be stated where the file is written.** R4 constrains Phase 3.5, but the instruction that actually writes the file is Phase 4 (`tech-plan.md:310`) via `tech-plan-feature-format.md`, which is an unconditional whole-file template write. Both must carry the preserve rule or R4 is bypassed by construction.

### 4.3 Design authority ladder

Placed in the new `skills/belmont/_src/references/design-authority-baseline.md`, cited from the tech-plan body as a literal `references/design-authority-baseline.md` path — `generate-skills.sh:140` greps the generated SKILL.md for exactly that pattern and ships only what it finds.

> Check by **name**, in your available skills. Do not probe the filesystem. Load **every** tier-1 skill present — they cover different contract sections and are not alternatives.
>
> - **Tier 1 — enrichment, per section.** `ui-designer` → Token Contract · `ux-designer` → UX Strategy **and State Inventory** · `ux-copywriter` → Microcopy Rules · `ux-motion` → Motion Contract **and the transition half of State Inventory**. A section whose skill is absent falls back to the baseline **for that section alone**.
> - **Tier 1b — conditional.** `frontend-design:frontend-design` for aesthetic direction. `dataviz` **only** if the feature renders charts. `vercel:shadcn` **only** if the project already depends on shadcn — evidence being `components.json` at repo root or a shadcn entry in the package manifest, not a guess.
> - **Tier 2 — normative floor, all tools, always.** The baseline rules in this file. Tier 2 runs *underneath* tier 1, not instead of it: a tier-1 skill may add detail or argue a deviation and must record it, but may never lower the a11y floor.
>
> **The contract schema is identical whichever tiers ran, and no downstream consumer may branch on it.**
>
> Record on an `**Authorities**` line, naming the skill per section — e.g. `baseline; ui-designer (tokens); ux-copywriter (microcopy)`, or `baseline only (no design skills available in this session)`. Never omit it, mirroring `verification-agent.md:93`.

**Why per-section.** The four skills are complementary. A single available/unavailable switch would give a user holding three of the four the baseline for all four sections, and would hide which sections were actually enriched.

**Portability — state it exactly.** **All four tier-1 skills are user-scope Claude Code skills** under `~/.claude/skills/`; none ships with Belmont and none is present for most users. What is absent elsewhere is the **content**, not the mechanism — opencode exposes a native `skill` tool, Gemini has `activate_skill`, and Codex/Cursor/Windsurf/Copilot/Pi auto-activate on `description:`. On the seven non-Claude CLIs every section runs tier 2, which is the shipped behaviour for essentially every user. Tier 2 is therefore the tested path, not the fallback.

**Use exact registered names.** `tech-plan.md:160` and `product-plan.md:165` currently name `vercel-react-best-practices` and `security`; neither resolves (`vercel:react-best-practices` is the real one; `security` is a slash command). `frontend-design` is likewise a plugin skill registered as `frontend-design:frontend-design`. Fix all three; do not copy that line's style.

#### Obtaining the tier-1 skills

The ladder says "if `ui-designer` is available" — a reader's next question is how to make it available. **`design-authority-baseline.md` must carry this table**, so the answer ships with the skill rather than living only in this proposal. None of these is bundled with Belmont; all are optional, and tier 2 is the tested path without them.

| Skill | Source | Archive | Installs to |
|---|---|---|---|
| `ui-designer`, `ux-designer` | [Claude UX & UI Design Skills](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) (Notion) | `ux-ui designer skill Jul 26.zip` | `~/.claude/skills/{ui,ux}-designer/` |
| `ux-copywriter` | [Drive folder](https://drive.google.com/drive/folders/1lSwUatVOzOX5TGWgBDjKA820RiUsVLNr) | `ux-copywriter.zip` | `~/.claude/skills/ux-copywriter/` |
| `ux-motion` | [Drive folder](https://drive.google.com/drive/folders/1gYrK4aT4A-LYr4GbM88-PaKu0wYyY4C6) | `ux-motion.zip` | `~/.claude/skills/ux-motion/` |

Vendor homepage, always valid: **<https://www.yummy-labs.com/>**.

**Why the sources differ.** The designer pair has a confirmed Notion page. The other three are linked to their Google Drive locations because their Notion URLs could not be established: each page carries a distinct `site_id`, so they are published as separate Notion Sites with no shared, derivable domain. Every Drive link above was verified by **downloading the archive and inspecting its contents**, which is the only check that means anything here — see the warning below.

> **Never verify a link with an HTTP status code.** An earlier revision of this document carried three Notion URLs constructed by deriving a slug from each page title, "verified" with `curl` status checks that returned 200 for all of them. Notion — and Google Drive — are client-rendered and return 200 with an empty shell for almost any path, so the check cannot distinguish a real page from a fabricated one. Three of the three were wrong. A link is verified when its content has been fetched and identified, not when it responds.

**These are unauthenticated share links and the vendor controls them.** They can be revoked, re-pointed at a new version, or renamed without notice, and the Drive links are more fragile than a Notion page for exactly that reason. Re-check every row at branch time and prefer a Notion page wherever one becomes available.

**One packaging note**, verified against the downloaded packages:

- The designer pair ships as `.skill` bundles (themselves archives) inside one zip; the other two ship as plain directories. Either way the on-disk result is a directory per skill under `~/.claude/skills/`.

These are **user-scope Claude Code skills**. They do not exist on the other seven CLIs, and they are not installable into a project — which is precisely why tier 2 is normative and the contract schema may not vary by tier.

### 4.4 Contract format

Written into `{base}/TECH_PLAN.md`. The feature template's `## Design Tokens (from Figma)` (`tech-plan-feature-format.md:44`) is **generalised**, not supplemented — it is Figma-scoped today, and its verification checklist hard-codes "Matches Figma design pixel-perfect".

```markdown
## Design Contract
**Mode**: [derived — UI, no Figma | N/A — no UI | N/A — Figma present]
**Source**: [storybook | tailwind.config.ts | globals.css | components.json | master TECH_PLAN |
             sibling feature <slug> | none — established here]
**Authorities**: baseline[; ui-designer (tokens); ux-designer (strategy, states); ux-copywriter (microcopy);
             ux-motion (motion); frontend-design:frontend-design (aesthetic)]
**Approval**: [approved <ISO date> | unreviewed (headless replan <ISO date>)]

### Token Contract
Spacing — 8pt grid: 4, 8, 12, 16, 24, 32, 48, 64, 96. Internal ≤ external on every component.
Typography — ratio [1.25], sizes [list], line-height 1.4–1.6 body / 1.1–1.3 heading. Max 4 sizes.
Colour — 60/30/10. Max 3 hues + neutrals. Never pure #000/#FFF. Body ≥ 4.5:1.
  Each semantic (success/error/warning/info) declares bg, border, text, icon.
Radius — committed value [8px]. Nested children strictly smaller than parent.
Elevation — [levels]. Interactive elements rise one level on hover.

### Accessibility Floor
Targets ≥ 44×44px (SC 2.5.5, AAA — adopted as Belmont's floor) · contrast ≥ 4.5:1 text / 3:1 large
(SC 1.4.3, AA) · visible focus on every interactive element (SC 2.4.7, AA) · every input has a visible
label, never placeholder-only · `prefers-reduced-motion` respected (SC 2.3.3, AAA) · no meaning by
colour alone.

### UX Strategy
User · emotional state on arrival · hero element · primary action · biggest UX risk. Five lines.

### State Inventory
Per interactive component: default · hover · focus · active · disabled · loading · empty · error,
plus any gesture or transition state the interaction introduces (drag, swipe, long-press, optimistic
update). Components the feature only consumes get no entry. Omissions carry a reason.

### Microcopy Rules
Buttons: verb matching the outcome, never "Submit". Errors: what happened / why / what next, never
blame or jokes. Empty states: what appears here, why it's empty, the action to fill it.
Destructive confirmations: name what is destroyed. Possessive framing for user-owned objects.

### Motion Contract
**Applies**: [yes | N/A — no motion in this feature]
Duration bands — instant feedback ≤ 100ms · state change 150–200ms · context shift 250–300ms ·
page transition 300–400ms. Above 400ms must justify itself.
Easing — one documented curve per class (enter / exit / move). Bounce and overshoot are opt-in per
product voice, never a default.
Performance — animate `transform` and `opacity` only. Animating layout properties is a defect.
Reduced motion — `prefers-reduced-motion: reduce` removes movement, never functionality.
```

**The Motion Contract is conditional.** Set `**Applies**: N/A — no motion in this feature` when the feature introduces no transition, animation or micro-interaction, and leave the four rules unfilled. This is the same pattern as R1's `**Mode**` gate: an explicit N/A, never an empty section, so a downstream agent can tell "does not apply" from "not done". **Silence is not allowed.**

Two consequences downstream. Verification skips the three motion rows when `**Applies**` is `N/A` and records them as such rather than `UNVERIFIABLE` — a skipped check and an unmeasurable one are different findings and must not be conflated. And `prefers-reduced-motion` stays in the Accessibility Floor regardless: it is a WCAG requirement about honouring a user preference, and a feature with no motion satisfies it trivially rather than being exempt from it.

**What the `N/A` modes contain.** `N/A — no UI`: the `**Mode**` line only, no subsections. `N/A — Figma present`: the `**Mode**` line plus the Figma-derived tokens tech-plan already extracts in-session — the section is the destination for those, which is why the template generalises rather than being conditional. Neither carries the baseline subsections, and neither is "a contract" for §4.6.

**Derivation order — reuse before invention.** Walk in order; stop at the first rung that supplies a token family, and name it in `**Source**`. Never invent a competing scale where one exists.

0. A `## Design Contract` in the **master** `.belmont/TECH_PLAN.md`.
0b. An approved `## Design Contract` in a **sibling** `.belmont/features/*/TECH_PLAN.md` (most recently approved wins; name which). *Without these two rungs, feature 1 and feature 4 of a greenfield project mint different scales with nothing to reconcile them.*
1. **Storybook, if present** — the strongest project-local source, because stories enumerate components *and their states*, which is what State Inventory needs. Detect via `.storybook/`, a `storybook` entry in the package manifest, or `*.stories.@(js|jsx|ts|tsx|mdx)` files. **Read the story files and `.storybook/preview.*` statically — do not run Storybook.** `forbidden-actions.md` prohibits planning sessions from running build or package-manager commands, and booting it would also collide with worktree port isolation. Record the existing component inventory into the feature template's `## Existing Components to Reuse`, and treat those components as **LAW** — the contract records them rather than redesigning them.
2. Project config — `tailwind.config.*`, CSS custom properties, `components.json`, theme files.
3. Nothing exists → establish the baseline defaults and set `**Source**: none`.

**Per-task specs stay in MILESTONE, unchanged in shape.** design-agent still produces one `### Design: [Task ID]` section per active task (`design-agent.md:232`), because `implementation-agent.md` Step 1.1 looks its per-task spec up there.

### 4.5 The reviewable artifact

`tech-plan` also writes **`{base}/design-preview.html`** — a self-contained page rendering the contract: colour swatches with computed contrast ratios, the type ramp, the spacing scale, radius and elevation samples, and a state row per entry in the State Inventory. No external assets or network references.

It is authored in the master tree and staged by `commit-belmont-changes.md`'s `git add .belmont/`, so it lands in the PR diff — **unless `.belmont/` is gitignored**, a supported configuration in which that partial short-circuits entirely and neither contract nor preview is ever committed. Durability is unaffected (the copy paths are filesystem-level), but reviewability is; say so rather than promising a diff that may not exist.

It is **not** written during an auto wave — and that is a **prose rule, not a mechanism**, for the reasons in §4.1. R4 is the one exception: a headless replan that derives a missing section regenerates the preview and stamps it unreviewed.

### 4.6 Verification

**`agents/belmont/verification-agent.md` Phase 2 becomes three-way.** Because a contract now exists only when there are no Figma URLs (R1), the branches are near-disjoint — but *not* fully, since `Step 2.0` counts linked screenshots and mockups as design references. A feature can therefore have a contract *and* a reference. The branches must handle that rather than assume it away.

> **"A contract is present" means one thing only:** `{base}/TECH_PLAN.md` contains `## Design Contract` whose `**Mode**` is `derived — UI, no Figma`. Both `N/A` modes, and an absent section, are **"no contract"** for all three branches and for the fourth enforcement rule.
>
> **Never key on the presence of the `## Design Contract` heading.** The feature template carries that heading unconditionally once §4.4 generalises it (`tech-plan-feature-format.md:44`), so a Figma feature's plan has the heading too — holding the Figma-extracted tokens tech-plan already loads in-session. Discriminating on section presence would demand contract checks on every Figma feature, find none, and fire the fourth rule: **the exact unconditional-failure blocker rev 6 believed it had fixed.**

1. **Design references exist** → existing comparison flow, unchanged — **plus contract checks if a contract is present.** A contract is orthogonal to a reference and its a11y floor still applies.
2. **No references, contract present** → contract checks only.
3. **No references, no contract** → existing acceptance-criteria fallback, unchanged. This is the failed-load path and must keep working.

Rev 5's fourth enforcement rule combined with an unconditional branch 1 would have failed **every** milestone carrying both. Branch 1 now performs the checks, so the rule is satisfiable.

verification-agent **already reads** TECH_PLAN (`:20`, `:60`), but only as a place to *search for* references. Making it the *check target* is new behaviour this PR specifies.

**Close the escape clause.** Narrow `:131` to "no design references **and** no `derived` design contract", and add a fourth enforcement rule: *`**Mode**` is `derived — UI, no Figma`, contract checks were **required**, and they were not performed ⇒ MUST be FAIL/INCOMPLETE*. Checks are not required in exactly one case — the focused re-verification exemption defined below. Stating the requirement clause is what keeps the rule satisfiable; rev 6 omitted it and the rule fired on every legitimate focused re-verify.

**Named tooling per check.** A check whose mechanism is unavailable is recorded **`UNVERIFIABLE`**, never `PASS`. `Actual` values must come from the running UI — sourcing them from `tailwind.config` or `globals.css` is forbidden, since that is what the contract was derived from and the check would be circular.

| Check | Mechanism |
|---|---|
| Spacing on declared scale; internal ≤ external | `browser_evaluate` → `getComputedStyle` |
| Type sizes from declared scale | `browser_evaluate` |
| Contrast ≥ 4.5:1 / 3:1 | `browser_evaluate` computing fg/bg luminance |
| Touch targets ≥ 44×44 | `browser_evaluate` → `getBoundingClientRect` |
| Focus visible on all interactive | `browser_press_key Tab` per stop, then `browser_evaluate` → `getComputedStyle(document.activeElement)` asserting non-`none` `outline-style` with non-zero width, **or** a `box-shadow` differing from the unfocused computed style. `browser_snapshot` alone cannot do this — it returns the accessibility tree, so it reports tab order but never whether an indicator renders |
| Labels present, not placeholder-only | `browser_evaluate` DOM inspection |
| Radius consistent, nested smaller | `browser_evaluate` |
| Elevation levels as declared; interactive rise one level on hover | `browser_hover` + `browser_evaluate` → `getComputedStyle` `box-shadow` |
| State Inventory entries present | `browser_click` / `browser_hover` / `browser_press_key` to enter each state + `browser_take_screenshot` |
| Transition durations within declared bands | `browser_evaluate` → `transitionDuration` / `animationDuration` |
| Easing matches the declared curve for its class | `browser_evaluate` → `transitionTimingFunction` / `animationTimingFunction` |
| Only `transform`/`opacity` animated | `browser_evaluate` → `transitionProperty` / keyframe inspection |
| Reduced motion removes movement, not function | `browser_run_code_unsafe` → `page.emulateMedia({ reducedMotion: 'reduce' })`, then re-run the state pass and assert the element still reaches its end state. **This is the only row needing a write-capable tool**; if it is unavailable, record `UNVERIFIABLE` |

**Motion rows are conditional.** When the contract records `**Applies**: N/A — no motion in this feature`, the three motion rows are recorded `N/A` and skipped. Do **not** record them `UNVERIFIABLE` — that value means "the mechanism was unavailable", and conflating it with "the check does not apply" would make a genuinely missing tool look like a design decision.

**Make MCP tool references install-independent.** `verification-agent.md:93-94` and `implementation-agent.md:192-193` hardcode `mcp__playwright__browser_*`; the same files hardcode `mcp__plugin_figma_figma__*` elsewhere. Neither prefix is canonical — Claude Code synthesises `mcp__plugin_<plugin>_<server>__` for plugin-registered servers and `mcp__<server>__` for directly-registered ones, and Belmont's own README recommends the direct install. **Instruct agents to resolve the browser tools by matching `*browser_navigate` / `*browser_evaluate` in their available tools rather than pinning a prefix**, and record `UNVERIFIABLE` when none matches.

**Focused re-verification.** After a fix-all round the default verify scope is *focused*, and both `verify.md:50` and the injected prompt (`main.go:7184`) instruct the agent to skip visual/design comparison. Contract checks are **required when the milestone's follow-ups touched UI** and **not required otherwise**; in the not-required case the attestation records `Contract checks performed: NO — focused re-verification, no UI follow-ups`. This is the single exemption the fourth enforcement rule's "were required" clause refers to; there are no others.

**Attestation gains a field:** `Contract checks performed: [YES against <path> | NO — <reason> | N/A]`.

**Severity** uses the existing Critical / Warning / Polish ladder (`verification-agent.md:295-326`). **Warning blocks the milestone and generates follow-up tasks; only Polish does not.** Grade accordingly: contrast failure, missing focus indicator, missing label → **Critical**. Missing state, out-of-band duration → **Warning**. Off-scale spacing, radius, elevation and easing inconsistency → **Polish**, so pre-existing drift cannot block a milestone. Rev 6 put off-scale spacing at Warning while claiming that kept it non-blocking — it does not, and on any project with existing drift that would have blocked the first milestone the gate ever ran on.

### 4.7 Downstream consumers

- **`agents/belmont/design-agent.md`** — mode keyed on Figma-URL presence in the PRD, never on load outcome. With Figma: unchanged. Without: derive per-task component specs *against the approved contract*. Reads `{base}/TECH_PLAN.md` (already at `:30`) **and the master `.belmont/TECH_PLAN.md`**, which it does not read today even though cross-cutting styling decisions are routed there. Writes nothing but MILESTONE.
- **`skills/belmont/_src/implement.md`** — the orchestrator for the phase this PR redefines. `:96` states Phase 2's purpose in Figma-only terms ("Analyze Figma designs (if provided)"), which on a no-Figma feature reads as a phase with nothing to do — and would skip the dispatch §7 forbids skipping. Restate as contract consumption; generalise the Visual Validation guidance beyond Figma.
- **`agents/belmont/implementation-agent.md`** — add a contract branch to the Visual Validation self-check. Correct the rev-4 justification: `:190-197` is *not* a no-op without Figma (`:196` handles other reference images, `:197` mandates a "No design reference available" note), and `:272` is mode-agnostic and needs no change.
- **`skills/belmont/_src/verify.md`** — three narrow edits, not one: Step 1b's collect list (`:62-72`) is Figma/image/URL-only; **Agent 1's dispatch prompt** (`:114-121`) steers the agent away from the new behaviour by enumerating only Figma/screenshot references and injecting "No design references found". `:109` and `:146` are the *same* line in two different agent blocks and are not the defect; and the focused-reverify clause at `:50` needs the contract exemption. *This file has Steps, not Phases — `verification-agent.md` has Phases. Never conflate them.*
- **`skills/belmont/_src/next.md`** — **stays in scope.** The feature contract is safe in TECH_PLAN, but *per-task* specs remain in MILESTONE, and `:162` overwrites the archive while `:117-118` stamps `[Not populated — lightweight mode skips the design agent]` over `## Design Specifications`. Carry forward populated `## Design Specifications` and `## Codebase Analysis` rather than clobbering. Coordinate with 0004, which edits this file and lands first.
- **`prompts/belmont/post-verify-triage.md`** — the Go-embedded prompt deciding whether verify findings block the loop (`executeTriageAction`, `main.go:7005`; parsed at `:6478-6488` with `defer_and_proceed` as the parse-failure default). Its blocking list is Figma-scoped, so contract failures could be silently deferred. Add a non-deferrable contract list. **Ships only via `scripts/build.sh` — embedded, never installed into a project.**

### 4.8 Model tier

**Rev 5's claim that tier guidance becomes unnecessary is false.** `planningTier = "high"` (`main.go:102`) is consumed only by `tierForAction` for `actionReplan` (`:249-252`), which fires **only inside the `belmont auto` shell-out**. A user typing `/belmont:tech-plan` in a Sonnet or Haiku REPL gets that session's model — Belmont pins no `model:` frontmatter anywhere. Since this PR's premise is that the contract is authored *interactively*, the forcing does not cover the path that matters.

1. Keep tier guidance, retargeted: **never downgrade the agent that authors or checks a design contract.** Note in the skill that contract authoring wants the strongest available session model.
2. `models-yaml-format.md:30` describes the `design` tier as "Figma extraction, token mapping, visual spec" and its profile heuristics at `:62-63` offer `design=low` for backend/infra. Both go stale; rewrite both locations.
3. `tech-plan` Phase 4.6 asks the planner to reason about "what will the design-agent actually do" (`:267`). That prose changes.

### 4.9 Pre-contract features

**This gate is not retroactive.** A feature planned before this PR has a `{base}/TECH_PLAN.md` with no `## Design Contract`, so §4.6 branch 3 applies and it keeps today's acceptance-criteria verification. That is deliberate: silently applying a new quality gate to already-approved plans would fail milestones on a standard their author never agreed to.

To adopt the contract on an existing feature, re-run `/belmont:tech-plan --feature <slug>`; Phase 3.5 derives the missing section and the user approves it. State this in `docs/workflow.md` and in the PR description.

## 5. Scope

### In scope

| File | Change |
|---|---|
| `skills/belmont/_src/tech-plan.md` | Phase 3.5; ALLOWED ACTIONS + preview; CRITICAL RULE 6; preserve rule at Phase 4; fix skill names `:160`; Phase 4.6 prose |
| `skills/belmont/_src/references/tech-plan-feature-format.md` | Generalise `## Design Tokens (from Figma)` → `## Design Contract` incl. all six subsections; contract branch in the Figma-only verification checklist; carry R4's preserve rule |
| `skills/belmont/_src/references/design-authority-baseline.md` | **New** — per-section ladder + baseline rules for all six contract sections, Belmont-original, crediting the four Yummy Labs skills by name and link |
| `skills/belmont/_src/product-plan.md` | Fix the same wrong skill names at `:165` |
| `skills/belmont/_src/implement.md` | Phase 2 purpose restated as contract consumption; Visual Validation generalised |
| `skills/belmont/_src/verify.md` | Step 1b collects the contract; dispatch prompt carries it; focused-reverify exemption at `:50` |
| `skills/belmont/_src/next.md` | Archive merges rather than clobbers `## Design Specifications` / `## Codebase Analysis` |
| `skills/belmont/_src/review-plans.md` | Treat `## Design Contract` as read-only — exclude it from drift/conflict rewrite options and emit a finding instead |
| `skills/belmont/_src/codex-plan-apply.md` | Carve planning artifacts out of the source-code prohibition so `design-preview.html` can be applied |
| `skills/belmont/_src/references/models-yaml-format.md` | Rewrite the `design` tier description (`:30`) and the profile heuristics (`:62-63`) |
| `agents/belmont/design-agent.md` | Consume the contract; replace `## Handling No Design`; read master TECH_PLAN. `FORBIDDEN ACTIONS` untouched |
| `agents/belmont/verification-agent.md` | Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; focused-reverify exemption; install-independent MCP names; **rework the `## Output Format` Visual Verification block (`:210-221`)** — Figma-shaped today, needs a contract branch and a Mechanism column |
| `agents/belmont/implementation-agent.md` | Contract branch in Visual Validation; install-independent MCP names |
| `prompts/belmont/post-verify-triage.md` | Non-deferrable contract list; de-Figma the blocking bullet |
| `plugin/` | Regenerate — `plugin/skills/{tech-plan,product-plan,implement,verify,next,review-plans,codex-plan-apply}/**` and `plugin/agents/*`. **Requires 0006** |
| `AGENTS.md` | Correct `:26`; add a design-authority line to the invariants |
| `knowledge/cross-cutting/dual-invocation-paths.md` | Correct `:9` |
| `knowledge/cross-cutting/codex-plan-handoff.md` | `Revisions` line — the packet gains its first non-Markdown payload |
| `knowledge/cross-cutting/design-authority.md` | **New** entry |

### Out of scope

Figma extraction behaviour · the NO FALLBACK rule · any Go change · a mechanical guard for the contract (deferred, §4.1) · the two `main` bugs this work uncovered (`writeWorktreeGitExcludes` writing to the wrong gitdir; `AGENTS.md:207`'s deleted master-tree shortcut) — file separately.

## 6. Both invocation paths

| Path | Exercise |
|---|---|
| **Auto** | `belmont auto` → `actionReplan` may invoke `/belmont:tech-plan` headlessly (R4) → implement dispatches design-agent → verify dispatches verification-agent |
| **Interactive** | `/belmont:tech-plan` with structured approval → `/belmont:implement` → `/belmont:verify` in a live REPL |
| **Plugin** | `plugin/agents/*` and `plugin/skills/{tech-plan,product-plan,implement,verify,next,review-plans,codex-plan-apply}/**` are a third tracked surface; `release.sh:41` regenerates and `:96` stages it |

**Correct the record on auto mode, per tool.** `AGENTS.md:26` and `dual-invocation-paths.md:9` say auto mode bypasses the tool's skill discovery and that Belmont injects the skill body, agent files and project state. For **five of the seven CLIs `belmont auto` can shell out to** that is false: `buildLoopPrompt` emits a bare slash reference (`main.go:7168`, `:7189`) and Belmont injects **only** steering text (`:6887`) and milestone-scope prose; the agent reads everything else from disk. For **Pi and opencode**, `adaptPromptForTool` (`main.go:7143-7157`) deliberately rewrites the reference into a read-the-file instruction — so for those two, discovery *is* bypassed, on purpose. Correct both statements with that split, not with a blanket claim.

**Do not conflate the two CLI counts.** The **install** surface is eight (`AGENTS.md:197`); the **auto** surface is seven. `belmont auto --tool windsurf` is rejected outright (`runAutoCmd`'s tool switch) and `toolHeadlessArgs` has no windsurf case — Windsurf is install-only. `docs/supported-tools.md`'s headless table already lists exactly seven rows and is the correct reference. This matters here: it is why the tier-1 ladder works under `claude -p`, which runs inside Claude Code's full skill runtime with `Skill` allowlisted (`main.go:7927`).

**No open-PR conflict.** PRs #10 and #11 were closed 2026-08-06. The only open PR is #21 (plugin generator), which is disjoint. Re-measure with `gh pr list --state open` at branch time.

## 7. Docs and knowledge

- `docs/agent-pipeline.md` — design authority now originates in tech-plan
- `docs/skills-reference.md` — the new tech-plan reference
- `docs/workflow.md` — Phase 3.5 in the flow; the §4.9 re-run instruction for pre-contract features
- `docs/prd-format.md` — Figma is one input among several, not the design source
- `README.md` — reframe the "Figma-first design workflow" bullet: Figma-first where Figma exists, contract-derived where it does not
- `AGENTS.md` — invariants line + the `:26` correction
- **New** `knowledge/cross-cutting/design-authority.md`, `Domains: agents, skills`, full skeleton. `Don't re-do` must record: inferring values on a *failed* load (rejected — NO FALLBACK stands); skipping the design dispatch (rejected); storing the contract in MILESTONE (rejected — `next.md` clobbers the archive); a separate `DESIGN-CONTRACT-<M>.md` (rejected — same resume profile); relaxing `design-agent.md:5-15` (rejected twice); relying on `--assume-unchanged` as a write guard (rejected — the merge copy defeats it); applying the gate retroactively (rejected — §4.9)
- Routing row in `knowledge/KNOWLEDGE.md`; `Revisions` lines on `model-tier-economics.md`, `dual-invocation-paths.md` and `codex-plan-handoff.md`

## 8. Author smoke test

Real project, disposable branch, a UI-bearing feature with no Figma URLs.

**Preconditions.** `requireCleanWorkingTree` aborts `auto` on any dirty or untracked path, and `install` writes files without committing — commit after every install. `belmont auto` runs `belmont validate` at startup; on a TTY it prompts y/N rather than hard-failing.

```bash
cd ~/path/to/real-project && git checkout -b smoke/pr1
belmont install --source ~/belmont --no-prompt
git add -A && git commit -m "belmont install (PR1 smoke)"
BASE=$(git rev-parse HEAD)
grep -c "Figma" .belmont/features/<slug>/PRD.md              # expect 0
grep -c "vercel-react-best-practices\|^- Load relevant skills.*security" \
  .agents/skills/belmont/tech-plan/SKILL.md                  # expect 0 — name fixes landed
```

**Step 1 — interactive tech-plan authors the contract.** `claude` → `/belmont:tech-plan --feature <slug>`.
Expect a structured question presenting the contract for approval; on approval `{base}/TECH_PLAN.md` carries `## Design Contract` with `**Mode**: derived — UI, no Figma`, a populated Token Contract, `**Source**` naming a real rung, an `**Authorities**` line, `**Approval**: approved <date>`, and all six subsections; `{base}/design-preview.html` exists and opens. Both committed by the skill's commit step.
Then record the baseline: `PLAN=$(git rev-parse HEAD)`.
Fail: no approval prompt → R3 not wired. Missing `**Authorities**` → ladder not recording.

**Step 1b — interactive implement + verify.** From a clean tree: `/belmont:implement --feature <slug>` then `/belmont:verify --feature <slug>`. Expect per-task `### Design:` sections referencing the contract, and a verify report with contract checks. *This is the only step exercising `verify.md`'s and `implement.md`'s interactive path.*

**Step 1c — a pre-contract feature adopts the contract (§4.9).** A second UI-bearing, no-Figma feature whose `TECH_PLAN.md` predates this branch and has no `## Design Contract`. Record `PRE=$(git rev-parse HEAD)`, then `/belmont:tech-plan --feature <slug3>` and approve.
Expect the contract added and `git diff $PRE..HEAD` confined to that feature's `TECH_PLAN.md` and `design-preview.html`. Then confirm the **non**-adoption path: on a third such feature, run `belmont auto` without re-planning and expect verify to reach branch 3 with no contract created — R4 must not adopt it headlessly.

**Step 2 — the contract survives `[r]`-resume.**
*Precondition:* the worktree path engages only when a milestone **in range** carries `(depends: …)`; otherwise `hasExplicitDeps` is false (`main.go:5441-5453`) and `runAutoCmd` routes to `runLoop` — master tree, no worktree, no `[r]` prompt. Confirm `{base}/PROGRESS.md` has `### M2: … (depends: M1)` first.
`belmont auto --feature <slug> --from M1 --to M2`; Ctrl-C during wave 1; re-run the identical command and answer `r` at the `Branch … exists from a previous run` prompt (`handleStaleWorktree`, `main.go:6100`).
Expect the banner `Belmont Auto (parallel)` — a serial fallback means the precondition was not met.
Then compare the worktree's `TECH_PLAN.md` against master's **in a second shell while wave 1 is still running**. The window is narrow and must be named: the worktree exists only between `Resuming with existing worktree at …` and the wave-1 merge, which runs `syncFeatureStateAfterMerge` and then removes the worktree. Check after the run and there is nothing left to inspect.
The interrupted run may leave `.belmont/auto.json` untracked, which `requireCleanWorkingTree` rejects on the re-run; delete it or pass `--allow-dirty`.

**Step 3 — design-agent consumes, never writes.** Run from the **master repo root, after `belmont auto` exits**, not after an individual wave merges — a run that aborts mid-wave leaves the check valid-looking and vacuous.
```bash
# positive control — fails loudly if the contract is untracked or .belmont/ is gitignored,
# in which case the diff below would pass for the wrong reason
git ls-files --error-unmatch .belmont/features/<slug>/TECH_PLAN.md
git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md   # expect empty
```
Do **not** use a bare `git diff`: `.belmont/` is `--assume-unchanged` in a worktree, so `git diff`, `git diff -- .belmont/` and `git status --porcelain` all report nothing even after the file has been completely rewritten (measured). The real loss point is `syncFeatureStateAfterMerge`, which lands the worktree's copy on master — which is why this compares committed revisions.

**Step 4 — the gate fires.** *Observe this in the Step 1b interactive run.* The per-row table and the attestation exist only in the verification sub-agent's returned report (`verification-agent.md` `## Output Format`); `verify.md` collects it into context and never writes it to disk, and the orchestrator's combined summary collapses it to a single `Visual Verification: [PASS/FAIL/N/A]` line. Expect per-row pass/fail/UNVERIFIABLE with a Mechanism column, and `Contract checks performed: YES against {base}/TECH_PLAN.md`. Do not go looking for a file.

**Step 5 — per-task specs survive a fix round.** Force one FWLUP round so `/belmont:next` runs and re-archives MILESTONE. Expect the re-archived `MILESTONE-M1.done.md` to still carry populated per-task design sections.
The second verify is *focused* by default and legitimately reports `Contract checks performed: NO — focused re-verification, no UI follow-ups` unless the follow-ups touched UI.

**Step 6 — headless replan preserves the contract.** `actionReplan` **cannot be forced**: after two verify failures `decideLoopActionSmart` returns nil (`main.go:7642-7644`) and the AI decider may pick DEBUG instead. Test R4 in isolation rather than driving the loop: run `claude -p "/belmont:tech-plan --feature <slug>"` from the project root with the contract already present.
Expect the `## Design Contract` byte-identical except `**Approval**` rewritten to `unreviewed (headless replan <date>)`; no regeneration of approved sections.
Fail: a fresh contract → R4/R8 not implemented, and the approved design is silently gone.

**Step 7 — Figma regression.** A feature **with** Figma URLs. Expect `**Mode**: N/A — Figma present`, **no** Token Contract, per-task Figma sections and the Figma Sources table exactly as today, and a verify verdict of PASS (not FAIL) — this asserts the fourth enforcement rule does not fire without a contract.

**Step 7b — backend regression (the other exclusion).** A feature with **no user interface** — API, pipeline, infra. `claude` → `/belmont:tech-plan --feature <backend-slug>`.
Expect the interview to ask **no design questions**, and `## Design Contract` to carry `**Mode**: N/A — no UI` with no Token Contract, UX Strategy, State Inventory, Microcopy or Motion subsections, and no `design-preview.html`. Verify must reach branch 3 and the fourth rule must not fire.
Without this, only one of R1's two exclusions is ever tested.

**Step 8 — failed-load regression (must not break).** Deliberately invalid node id. Expect task BLOCKED, status FAILED, nothing invented, and **no fallback to a contract**.

**Step 9 — other tools.**
9a: `belmont auto --tool codex --feature <slug>` from `$BASE`.
9b: interactive `codex` → `/plan $belmont:tech-plan` → confirm the structured interview runs and a `BELMONT_PLAN_PACKET` is emitted carrying **both** `TECH_PLAN.md` and `design-preview.html`; apply with `$belmont:codex-plan-apply` and confirm the `.html` is not rejected as source code.
9c: interactive `opencode` on a **second** UI-bearing no-Figma feature → `/belmont/tech-plan --feature <slug2>`. Expect a contract schema-identical to Step 1's with `**Authorities**: baseline only`. *`belmont auto` cannot author a contract — the only auto path to tech-plan is `actionReplan` — so this must be the interactive path.*

**Step 10 — plugin surface.** Run in the **Belmont source repo**, not the smoke project, and use `--check` — passing a version argument rewrites `plugin/.claude-plugin/plugin.json` and dirties a tracked file:
```bash
cd ~/belmont
./scripts/generate-plugin.sh --check && echo "plugin/ up to date"
test -s plugin/skills/tech-plan/references/design-authority-baseline.md
! find plugin/agents -name '*.md' -size 0 | grep -q .    # 0006's fix
```

**Diagnostics.** Steps 1–6 need no Figma access. If 7 or 8 fail, test the Figma MCP directly before blaming this PR.

## 9. Definition of Done

**Placement and durability**
- [ ] Contract written to `{base}/TECH_PLAN.md` by `tech-plan` only; no other phase writes that path, asserted by smoke Step 3's committed-revision diff
- [ ] `design-agent.md` `## FORBIDDEN ACTIONS` (`:5-15`) byte-identical in the diff
- [ ] §4.1 states plainly that the rule is prose-enforced and unguarded, and does **not** claim `--assume-unchanged` as a backstop
- [ ] Contract survives `[r]`-resume (Step 2) and headless replan (Step 6)
- [ ] R4's preserve rule stated at **both** Phase 3.5 and the Phase 4 write instruction (R8)

**Gating and scope**
- [ ] Contract created only when the feature has UI **and** no Figma URLs; `**Mode**` is the single signal, with `N/A` values for both exclusions
- [ ] One contract per feature, covering all milestones
- [ ] Pre-contract features fall to branch 3 and are documented as non-retroactive (§4.9)

**Authority and portability**
- [ ] `references/design-authority-baseline.md` created, cited as a literal `references/…` path, and present in `skills/belmont/tech-plan/references/` after `./scripts/generate-skills.sh`
- [ ] Tier 2 runs underneath tier 1; per-section enrichment; schema identical across tiers; no consumer branches on tier
- [ ] `**Authorities**` names the skill per section, never omitted
- [ ] `### Motion Contract` carries an `**Applies**` field; an unanimated feature records `N/A — no motion in this feature` rather than an empty section, and verification records those three rows `N/A`, never `UNVERIFIABLE`
- [ ] All four Yummy Labs skills credited with links; **no file from `~/.claude/skills/` copied into the repo**
- [ ] `design-authority-baseline.md` carries the **obtaining table** (§4.3) — source link and `~/.claude/skills/<name>/` install path per skill, and a plain statement that none is bundled and tier 2 works without them
- [ ] Every link in that table checked by **opening it in a browser** at branch time — they are third-party URLs this PR does not control, and a status code proves nothing on a client-rendered host (§4.3)
- [ ] WCAG conformance levels stated wherever criteria are cited (two are AAA)
- [ ] `vercel:react-best-practices`, the bogus `security` entry, and `frontend-design:frontend-design` corrected in `tech-plan.md` and `product-plan.md`

**Gate**
- [ ] Phase 2 three-way, with branch 1 performing contract checks when a contract is present; `:131` narrowed; fourth enforcement rule present and satisfiable
- [ ] Every check row names a mechanism; unavailable ⇒ `UNVERIFIABLE`; MCP tools resolved by suffix match, not a pinned prefix
- [ ] `verification-agent.md`'s `## Output Format` Visual Verification block reworked with a contract branch and a Mechanism column
- [ ] Focused-reverify behaviour stated and matched by Step 5
- [ ] `post-verify-triage.md` lists contract failures as non-deferrable
- [ ] `next.md` archive merges rather than clobbers per-task sections

**Non-regression**
- [ ] Figma feature: no contract, verdict PASS, behaviour unchanged (Step 7)
- [ ] Failed load still BLOCKS and does not fall back to a contract (Step 8)

**Corrections carried by this PR**
- [ ] `AGENTS.md:26` and `dual-invocation-paths.md:9` corrected **per tool** — bare slash reference for five of the seven auto-capable CLIs, deliberate rewrite for Pi and opencode, and Windsurf named as install-only with no auto path at all

**Mechanics**
- [ ] `./scripts/generate-skills.sh && test -f skills/belmont/tech-plan/references/design-authority-baseline.md` — `--check` compares SKILL.md only and cannot see references
- [ ] `plugin/` regenerated and committed; verified with `./scripts/generate-plugin.sh --check` (version-agnostic after 0006), not with a version argument that mutates `plugin.json`
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green — trivially true given no Go changes, retained as a regression guard on the embedded prompt
- [ ] Auto, interactive and plugin surfaces exercised, including Codex packet (9b), one non-Claude CLI (9c) and the plugin build (Step 10)
- [ ] `design-authority.md` created with `Don't re-do`; routing row added; three `Revisions` lines
- [ ] 0004's post-contract re-baseline executed and reported (§11)

## 10. Risks

| Risk | Mitigation |
|---|---|
| A worktree phase overwrites the approved contract | Prose rule (§4.1), **explicitly unguarded**. Detection is smoke Step 3's committed-revision diff, not git status. A mechanical guard is deferred to its own proposal |
| Headless replan regenerates an approved contract | R4 preserve-verbatim + R8 placement at the write instruction; Step 6 tests it in isolation because the loop cannot be driven to `actionReplan` deterministically |
| Gate never fires after the first failure | Focused-reverify exemption; Step 5 asserts the attestation wording |
| Gate fires on a Figma feature and fails it | Contract not created when Figma URLs exist (R1); branch 1 performs contract checks when one *is* present; Step 7 asserts PASS |
| Contract licenses a guess on a failed Figma load | Mode keyed on URL presence only (§4.7); §2's NO FALLBACK rule untouched; Step 8 |
| Contract invents a scale that already exists | Derivation ladder rungs 0–2, Storybook first among project sources; `**Source**` must name the rung |
| Sibling features mint different scales | Ladder rungs 0 and 0b consult master and sibling contracts |
| Preview drifts from the contract | Both authored in the same step by interactive tech-plan. R4's headless replan is the exception and must regenerate and stamp the preview |
| Contract never reaches the PR diff | Stated: `commit-belmont-changes.md` short-circuits when `.belmont/` is gitignored. Durability is unaffected; reviewability is not promised |
| Per-task specs still clobbered by `next.md` | `next.md` in scope; coordinate with 0004, which lands first |

## 11. Interaction with PR 2 (0004)

0004 lands first and measures its reduction against a pre-contract baseline, reporting it as an upper bound. **This PR owes the re-baseline.** Run 0004's Tier-2 gate after this lands and report the post-contract figure, naming the number it supersedes.

The restructure plausibly *reduces* the fan-out cost 0004 defers: derivation moves from per-milestone to per-feature, and per-task sections consume a contract rather than re-deriving one. Hypothesis, not result — measure it.

**Shared files.** Both edit `_src/next.md` and `_src/references/models-yaml-format.md` — real overlaps; 0003 rebases onto 0004. The `_src/verify.md` "guaranteed conflict" predicted by earlier revisions **does not occur**: tested at git 2.50.1, a three-line insertion three lines from 0004's edit rebases cleanly and stays clean when widened. Default diff context governs `git am`/`git apply`, not the three-way merge rebase uses. Resolve `plugin/` conflicts by regeneration, never by hand.

## 12. Changes from v6

Rev 6 was attacked by six lenses: 69 findings, **all 69** individually refuted (no cap, unlike the rev 5 round). **53 confirmed, 16 refuted (23%).** Full record: [`rev6-CONFIRMED-DEFECTS.md`](rev6-CONFIRMED-DEFECTS.md).

**The blocker — the same failure, one layer down**

| v6 | v7 |
|---|---|
| §4.6 branches on "a contract is present" and never defines it | **Defined:** `**Mode**` is `derived — UI, no Figma`. Both `N/A` modes and an absent section are "no contract" |
| — | **Never key on the `## Design Contract` heading.** §4.4 generalises the feature template's section, so a *Figma* feature's plan carries the heading too. Discriminating on presence would demand contract checks on every Figma feature, find none, and fire the fourth rule — reinstating the unconditional-failure blocker v6's own §12 claimed to have fixed |

The regression traces to v6's own minor fix: moving the mode value *inside* the heading voided the premise an earlier refutation had relied on. A fix that reintroduced the bug it fixed — the pattern this series keeps producing, and the reason each revision is re-attacked rather than trusted.

**Majors fixed**

| v6 | v7 |
|---|---|
| "six of eight CLIs" | **Five of seven.** `belmont auto --tool windsurf` is rejected outright — Windsurf is install-only. Install surface 8, auto surface 7; never conflate them |
| Fourth rule fires whenever a contract exists | Adds the **"checks were required"** clause, with focused re-verification named as the single exemption. Without it the rule fired on every legitimate focused re-verify |
| Write rule covers the whole file, forbidding `debug` | Scoped to the **`## Design Contract` section**, with `debug-manual`'s documented prose-editing permission preserved and `review-plans` added to scope as read-only |
| R4 may "derive sections that are absent" | **Never creates a contract.** Otherwise a headless replan silently adopts a pre-contract feature into a gate its author never approved, contradicting §4.9 |
| Mixed Figma coverage undefined | **Any** task with a Figma URL makes it a Figma feature; uncovered tasks keep today's behaviour, so `## Handling No Design` keeps a no-contract arm |
| Off-scale spacing at Warning "so drift cannot block" | Warning **does** block and generates follow-ups. Moved to Polish, with the ladder's real semantics stated |
| `verify.md` dispatch prompt cited at `:109`/`:146` | Those are the same line in two different agent blocks. The defect is Agent 1's prompt at `:114-121` |
| State Inventory unassigned after `interactive-prototype` was dropped | Split explicitly: `ux-designer` owns component states, `ux-motion` owns transitions — in the §3 table and the `Authorities` schema, not just the ladder |
| Smoke Steps 2, 3, 4, 10 | Step 2 gains the resume window (the worktree is gone after the merge); Step 3 runs after `auto` exits and gains a positive control; Step 4 says the report lives in the sub-agent's return, not on disk; Step 10 runs in the source repo with `--check`, since a version argument dirties `plugin.json` |
| Only one of R1's two exclusions tested | **Step 7b** added for the no-UI exclusion; **Step 1c** for §4.9 adoption and non-adoption |
| — | Easing check row added; `N/A` mode contents specified; `runAutoCmd` corrected; plugin enumerations completed; link-checking DoD requires opening a browser, since a status code proves nothing on a client-rendered host |

## 12b. Changes from v5

Rev 5 was reviewed by six adversarial lenses producing 72 findings; each was then attacked by an independent skeptic. **19 were refuted and are not acted on**; 53 survived. Full record: [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md).

**Author decisions, 2026-08-07**

| v5 | v6 |
|---|---|
| Contract for every UI feature, Figma or not | **Only UI features without Figma.** Figma features untouched — which also removes the unconditional-failure trap below |
| Scope ambiguous between feature and milestone | **One conversation per feature**, covering all milestones |
| Durability implied to be mechanical | **Prose-enforced and unguarded, stated as such.** Mechanical guard deferred to its own proposal |
| — | **Storybook added as the first project-local derivation rung**, read statically — planning sessions may not run build commands |
| Five tier-1 skills, `interactive-prototype` → State Inventory | **Dropped to four.** `interactive-prototype` is a *builder*: its later steps produce React code against a named animation library, and its own description excludes static specs. Loading it inside `tech-plan` — a session forbidden from writing code — invites the behaviour that skill instructs. Its one relevant contribution (interactions implied but not drawn) overlaps `ux-designer` and `ux-motion`, which now own State Inventory between them. It belongs to implementation, not planning |
| Motion Contract unconditional | **Conditional on an `**Applies**` field.** A feature that animates nothing records `N/A — no motion in this feature` rather than carrying an empty section, and verification records its three rows `N/A` rather than `UNVERIFIABLE` — a check that does not apply and a mechanism that was unavailable are different findings. `prefers-reduced-motion` stays in the Accessibility Floor either way, since it is about honouring a user preference |

**Blockers fixed**

| v5 | v6 |
|---|---|
| "Content authored inside a worktree does not survive" | **False and inverted.** `syncFeatureStateAfterMerge` copies worktree → master after every merge, so such an edit *replaces* the approved contract. §4.1 rewritten bidirectionally |
| `--assume-unchanged` "fails loudly" | It fails **silently** — the merge copy is not git-aware, and it covers only already-tracked files. Mitigation replaced with a content-hash check |
| Hard rule "no auto phase writes" vs R4 requiring `actionReplan` to write | Restated: only the `tech-plan` skill writes; R4 constrains it when headless. DoD items no longer mutually unsatisfiable |
| Every Figma milestone fails verification | Contract not created for Figma features; branch 1 performs contract checks when one is present; Step 7 asserts PASS |
| Smoke Step 2 never forks a worktree | Needs `(depends: …)` in range; `--from M1 --to M2` with the expected parallel banner |
| Smoke Step 3's `git diff` could not fail | Measured blind. Replaced with `git diff $PLAN..HEAD` over committed revisions |

**Majors fixed** — `.belmont/` git-invisibility claim corrected and re-cited; State Inventory section added (four places referenced a section that did not exist); preview regeneration defined for headless replan; focus-visible mechanism corrected (`browser_snapshot` cannot see computed styles); reduced-motion mechanism corrected (needs `browser_run_code_unsafe`); elevation check row added; MCP tool names made install-independent rather than pinned to a plugin namespace; `implement.md`, `codex-plan-apply.md` and `post-verify-triage.md` added to scope; `verify.md` scoped to three edits including the dispatch prompt; `verification-agent.md`'s Figma-shaped output block added to scope; derivation ladder extended with master and sibling contracts; Step 9c moved to the interactive path; Step 6 replaced with an isolated R4 test; migration story added (§4.9); dual-invocation correction split per tool.

**Minors fixed** — file count corrected to 13; `**Design contract**` phantom field removed in favour of `**Mode**`; `user-questions.md:4` → `:8`; `frontend-design` → `frontend-design:frontend-design`; WCAG levels stated; two broken §10 cross-references repaired; plugin enumerations completed; `--check` DoD items replaced with checks that can actually fail; two vacuous DoD items deleted; `models-yaml-format.md` second location cited; portability paragraphs updated to match the tier-1 roster; `docs/` list extended to five pages; gitignored-`.belmont/` configuration handled.
