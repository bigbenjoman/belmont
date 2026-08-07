# PR 1 — Design quality for UI features without Figma

**Revision 6.** Rev 5 moved design into `/belmont:tech-plan` — the right place — but stated its central durability claim backwards, and an adversarial review confirmed **53 defects** in it (72 raised, 19 refuted). v6 fixes those, and narrows scope on three author decisions taken 2026-08-07. Changes in §12. Evidence: [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md).

**Type:** prose only — 3 agents, 6 skill sources, 2 existing skill references, 1 new skill reference, 1 Go-embedded prompt = **13 tracked source files**. **No Go changes.**
**Size:** ~560 lines net across those 13 (§5) + regenerated `plugin/` + 5 docs pages + 1 new knowledge entry + 1 `KNOWLEDGE.md` routing row + 3 corrected statements in `AGENTS.md`/knowledge. *(Counted from the §5 table, not asserted — rev 5 claimed 9 against a table of 10, and this figure was itself wrong at 11 on first draft.)*
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

The *curation* — which rules matter and how they compose into a checkable contract — is drawn from the **[Yummy Labs](https://www.yummy-labs.com/) Claude design skills**. Credit them by name and link in the reference file header (see §4.3 for which URLs are confirmed), in `design-authority.md`, and in the PR description:

| Skill | Contract section it feeds |
|---|---|
| [ui-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | Token Contract; the "existing design system is LAW" rule; the banned-defaults list |
| [ux-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | UX Strategy |
| `ux-copywriter` | Microcopy Rules |
| `ux-motion` | Motion Contract |
| `interactive-prototype` | State Inventory — gesture and transition states a static spec omits |

**Do not vendor their files.** All five are distributed as downloads with **no LICENSE, no repository and no redistribution grant** — verified against the actual packages, not the website. Belmont states the same published standards in its own words and credits the source. If written permission is obtained (`yummylabs@yummydesign.xyz`), vendoring with attribution can replace the baseline in a follow-up.

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

> **Only the `tech-plan` skill may write `{base}/TECH_PLAN.md`. `implement`, `verify`, `next` and `debug` read it and never write it.** When `tech-plan` itself runs headlessly as `actionReplan`, R4 constrains what it may change.

**This is prose-enforced and unguarded, and the PR says so plainly.** Nothing mechanical prevents a worktree phase from writing that path: `runScopeGuard` parses `PROGRESS.md` only and returns early for `actionReplan` (`main.go:12065-12068`), and the merge copy is a filesystem operation. The detection is smoke Step 3's content hash (§8), not git.

*Author decision, 2026-08-07: prose rule now, mechanical guard later.* A guard — snapshot `## Design Contract` before the wave, restore it in `syncFeatureStateAfterMerge` — would be a **Go change** on the merge path every feature depends on. It is deferred to its own proposal rather than smuggled into a design-quality PR. Also unhandled there: `recoverMerge` (`main.go:11156-11196`) never calls `syncFeatureStateAfterMerge` at all.

**`design-agent.md`'s `## FORBIDDEN ACTIONS` (`:5-15`) is untouched.** Its only writable output stays MILESTONE `## Design Specifications`. A consuming design-agent needs no relaxation.

### 4.2 The tech-plan design step

Insert **`### Phase 3.5 — Design Contract`** into `skills/belmont/_src/tech-plan.md` between `:217` and `:219` — after Phase 3's interview (it consumes the `:197` answers) and before Phase 4 writes the plan.

- **R1 — Gate: UI **and** no Figma.** Run only when the feature has a user interface **and** no active task carries a Figma URL. Record the outcome in the contract's `**Mode**` field, which is the single machine-read signal (§4.4) — there is no separate `**Design contract**` field. Backend features record `N/A — no UI`; Figma features record `N/A — Figma present`. **Silence is not allowed**, so downstream agents can distinguish "not applicable" from "not done".
  *Author decision, 2026-08-07: the contract exists only for UI-without-Figma.* Figma features keep today's behaviour untouched.
  **Scope is the whole feature, one conversation** (author decision): the contract covers every milestone, including ones that turn out to be backend-only. Milestone-level scoping was rejected — planning does not reliably know which milestones are UI-bearing before the work is broken down.
- **R2 — Figma detection only, no loading.** The step needs to know *whether* Figma URLs exist, not what is in them. Do not load Figma here. (If Figma is loaded elsewhere in tech-plan it must stay **in-session** — `tech-plan.md:158`: "sub-agents cannot get MCP tool permissions approved.")
- **R3 — Structured questions or stop.** The approval step inherits `user-questions.md:8` ("NEVER ask questions as plain inline text when a structured question tool exists") and the strict Codex fallback at `:11`. A Markdown pick-list is not an approval gate.
- **R4 — Headless behaviour is defined, not implied.** Under `belmont auto`, `actionReplan` invokes `/belmont:tech-plan` (`buildLoopPrompt`, `main.go:7188-7189`) with no user and no `AskUserQuestion`. The step must: **preserve an existing `## Design Contract` verbatim**, deriving only sections that are absent; mark it `**Approval**: unreviewed (headless replan <ISO date>)`; regenerate `design-preview.html` in the same step if and only if a section changed, stamping it `<!-- unreviewed: headless replan <ISO date> -->`; and never silently regenerate an approved contract. Nothing mechanical catches a violation — see §4.1.
- **R5 — Codex packet.** The contract and preview must be enumerable as `BELMONT_PLAN_PACKET` operations (`tech-plan.md:37-45`). `codex-plan-apply` applies **two** gates: a path constraint (satisfied — both are under `.belmont/`) and a **source-code content prohibition** which a self-contained `.html` would trip. `_src/codex-plan-apply.md` is therefore in scope (§5) to carve out planning artifacts explicitly.
- **R6 — Update CRITICAL RULE 6.** `tech-plan.md:19` names Phase 4.5 as the last mandatory phase and is *already* stale with respect to Phase 4.6. Adding Phase 3.5 without fixing it leaves three contradictory statements of when the skill terminates.
- **R7 — Extend ALLOWED ACTIONS.** `tech-plan.md:96-105` enumerates the write surface exhaustively; add `{base}/design-preview.html`. **Do not touch the shared `forbidden-actions.md` partial** — other skills include it. The preview is a planning artifact under `.belmont/`, never project source; CRITICAL RULE 2 stands unchanged for `.tsx/.ts/.css`.
- **R8 — The rule must be stated where the file is written.** R4 constrains Phase 3.5, but the instruction that actually writes the file is Phase 4 (`tech-plan.md:310`) via `tech-plan-feature-format.md`, which is an unconditional whole-file template write. Both must carry the preserve rule or R4 is bypassed by construction.

### 4.3 Design authority ladder

Placed in the new `skills/belmont/_src/references/design-authority-baseline.md`, cited from the tech-plan body as a literal `references/design-authority-baseline.md` path — `generate-skills.sh:140` greps the generated SKILL.md for exactly that pattern and ships only what it finds.

> Check by **name**, in your available skills. Do not probe the filesystem. Load **every** tier-1 skill present — they cover different contract sections and are not alternatives.
>
> - **Tier 1 — enrichment, per section.** `ui-designer` → Token Contract · `ux-designer` → UX Strategy · `ux-copywriter` → Microcopy Rules · `ux-motion` → Motion Contract · `interactive-prototype` → State Inventory. A section whose skill is absent falls back to the baseline **for that section alone**.
> - **Tier 1b — conditional.** `frontend-design:frontend-design` for aesthetic direction. `dataviz` **only** if the feature renders charts. `vercel:shadcn` **only** if the project already depends on shadcn — evidence being `components.json` at repo root or a shadcn entry in the package manifest, not a guess.
> - **Tier 2 — normative floor, all tools, always.** The baseline rules in this file. Tier 2 runs *underneath* tier 1, not instead of it: a tier-1 skill may add detail or argue a deviation and must record it, but may never lower the a11y floor.
>
> **The contract schema is identical whichever tiers ran, and no downstream consumer may branch on it.**
>
> Record on an `**Authorities**` line, naming the skill per section — e.g. `baseline; ui-designer (tokens); ux-copywriter (microcopy)`, or `baseline only (no design skills available in this session)`. Never omit it, mirroring `verification-agent.md:93`.

**Why per-section.** The five skills are complementary. A single available/unavailable switch would give a user holding four of the five the baseline for all five sections, and would hide which sections were actually enriched.

**Portability — state it exactly.** **All five tier-1 skills are user-scope Claude Code skills** under `~/.claude/skills/`; none ships with Belmont and none is present for most users. What is absent elsewhere is the **content**, not the mechanism — opencode exposes a native `skill` tool, Gemini has `activate_skill`, and Codex/Cursor/Windsurf/Copilot/Pi auto-activate on `description:`. On the seven non-Claude CLIs every section runs tier 2, which is the shipped behaviour for essentially every user. Tier 2 is therefore the tested path, not the fallback.

**Use exact registered names.** `tech-plan.md:160` and `product-plan.md:165` currently name `vercel-react-best-practices` and `security`; neither resolves (`vercel:react-best-practices` is the real one; `security` is a slash command). `frontend-design` is likewise a plugin skill registered as `frontend-design:frontend-design`. Fix all three; do not copy that line's style.

#### Obtaining the tier-1 skills

The ladder says "if `ui-designer` is available" — a reader's next question is how to make it available. **`design-authority-baseline.md` must carry this table**, so the answer ships with the skill rather than living only in this proposal. None of these is bundled with Belmont; all are optional, and tier 2 is the tested path without them.

| Skill | Notion page | Installs to |
|---|---|---|
| `ui-designer`, `ux-designer` | [Claude UX & UI Design Skills](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) — confirmed | `~/.claude/skills/{ui,ux}-designer/` |
| `ux-copywriter` | **TODO — get the published URL from the author** | `~/.claude/skills/ux-copywriter/` |
| `ux-motion` | **TODO — get the published URL from the author** | `~/.claude/skills/ux-motion/` |
| `interactive-prototype` | **TODO — get the published URL from the author** | `~/.claude/skills/interactive-prototype/` |

Vendor homepage, always valid: **<https://www.yummy-labs.com/>**.

> **Do not construct these URLs.** Three were previously written into this document by deriving a slug from the page title and were **wrong**. The page IDs are real (obtained from Notion's API, and all four report `publicAccessRole: reader`), but each page carries a distinct `site_id`, so they are published as separate Notion Sites and do not share one predictable domain. An HTTP 200 is **not** evidence a Notion URL is correct — Notion is client-rendered and returns 200 with an empty shell for any path under a workspace subdomain. Paste the URLs from a browser; do not derive them, and do not "verify" them with `curl`.

Link the **Notion pages**, not the storage URLs behind them. The pages are vendor-maintained and carry current install instructions; the download links they point at are unauthenticated share URLs that change without notice.

**Two packaging notes**, both verified against the downloaded packages:

- The designer pair ships as `.skill` bundles (themselves archives) inside one zip; the other two ship as plain directories. Either way the on-disk result is a directory per skill under `~/.claude/skills/`.
- **`interactive-prototype` is mis-packaged upstream.** Its `SKILL.md` loads `references/<file>.md`, but the archive places those files flat in a directory named `Prototyping v5`. Installed as downloaded, every progressive-disclosure load silently misses — you get the body and none of the ~73KB behind it. It must be unpacked into `interactive-prototype/` with the reference files moved into `references/`. Say so in the baseline file; a user who follows the vendor's instructions literally gets a broken skill and no error.

These are **user-scope Claude Code skills**. They do not exist on the other seven CLIs, and they are not installable into a project — which is precisely why tier 2 is normative and the contract schema may not vary by tier.

### 4.4 Contract format

Written into `{base}/TECH_PLAN.md`. The feature template's `## Design Tokens (from Figma)` (`tech-plan-feature-format.md:44`) is **generalised**, not supplemented — it is Figma-scoped today, and its verification checklist hard-codes "Matches Figma design pixel-perfect".

```markdown
## Design Contract
**Mode**: [derived — UI, no Figma | N/A — no UI | N/A — Figma present]
**Source**: [storybook | tailwind.config.ts | globals.css | components.json | master TECH_PLAN |
             sibling feature <slug> | none — established here]
**Authorities**: baseline[; ui-designer (tokens); ux-designer (strategy); ux-copywriter (microcopy);
             ux-motion (motion); interactive-prototype (states); frontend-design:frontend-design (aesthetic)]
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
Duration bands — instant feedback ≤ 100ms · state change 150–200ms · context shift 250–300ms ·
page transition 300–400ms. Above 400ms must justify itself.
Easing — one documented curve per class (enter / exit / move). Bounce and overshoot are opt-in per
product voice, never a default.
Performance — animate `transform` and `opacity` only. Animating layout properties is a defect.
Reduced motion — `prefers-reduced-motion: reduce` removes movement, never functionality.
```

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

**`agents/belmont/verification-agent.md` Phase 2 becomes three-way.** Because a contract now exists only when there are no Figma URLs (R1), the branches are near-disjoint — but *not* fully, since `Step 2.0` counts linked screenshots and mockups as design references. A feature can therefore have a contract *and* a reference. The branches must handle that rather than assume it away:

1. **Design references exist** → existing comparison flow, unchanged — **plus contract checks if a contract is present.** A contract is orthogonal to a reference and its a11y floor still applies.
2. **No references, contract present** → contract checks only.
3. **No references, no contract** → existing acceptance-criteria fallback, unchanged. This is the failed-load path and must keep working.

Rev 5's fourth enforcement rule combined with an unconditional branch 1 would have failed **every** milestone carrying both. Branch 1 now performs the checks, so the rule is satisfiable.

verification-agent **already reads** TECH_PLAN (`:20`, `:60`), but only as a place to *search for* references. Making it the *check target* is new behaviour this PR specifies.

**Close the escape clause.** Narrow `:131` to "no design references **and no design contract**", and add a fourth enforcement rule: *a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*.

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
| Only `transform`/`opacity` animated | `browser_evaluate` → `transitionProperty` / keyframe inspection |
| Reduced motion removes movement, not function | `browser_run_code_unsafe` → `page.emulateMedia({ reducedMotion: 'reduce' })`, then re-run the state pass and assert the element still reaches its end state. **This is the only row needing a write-capable tool**; if it is unavailable, record `UNVERIFIABLE` |

**Make MCP tool references install-independent.** `verification-agent.md:93-94` and `implementation-agent.md:192-193` hardcode `mcp__playwright__browser_*`; the same files hardcode `mcp__plugin_figma_figma__*` elsewhere. Neither prefix is canonical — Claude Code synthesises `mcp__plugin_<plugin>_<server>__` for plugin-registered servers and `mcp__<server>__` for directly-registered ones, and Belmont's own README recommends the direct install. **Instruct agents to resolve the browser tools by matching `*browser_navigate` / `*browser_evaluate` in their available tools rather than pinning a prefix**, and record `UNVERIFIABLE` when none matches.

**Focused re-verification.** After a fix-all round the default verify scope is *focused*, and both `verify.md:50` and the injected prompt (`main.go:7184`) instruct the agent to skip visual/design comparison. Contract checks are **exempt from that skip when the milestone's follow-ups touched UI**; otherwise the attestation records `Contract checks performed: NO — focused re-verification, no UI follow-ups`.

**Attestation gains a field:** `Contract checks performed: [YES against <path> | NO — <reason> | N/A]`.

**Severity** uses the existing Critical / Warning / Polish ladder: contrast failure, missing focus indicator, missing label → Critical; missing state, off-scale spacing, out-of-band duration → Warning; radius, elevation or easing inconsistency → Polish. Off-scale spacing stays Warning so pre-existing drift cannot block a milestone.

### 4.7 Downstream consumers

- **`agents/belmont/design-agent.md`** — mode keyed on Figma-URL presence in the PRD, never on load outcome. With Figma: unchanged. Without: derive per-task component specs *against the approved contract*. Reads `{base}/TECH_PLAN.md` (already at `:30`) **and the master `.belmont/TECH_PLAN.md`**, which it does not read today even though cross-cutting styling decisions are routed there. Writes nothing but MILESTONE.
- **`skills/belmont/_src/implement.md`** — the orchestrator for the phase this PR redefines. `:96` states Phase 2's purpose in Figma-only terms ("Analyze Figma designs (if provided)"), which on a no-Figma feature reads as a phase with nothing to do — and would skip the dispatch §7 forbids skipping. Restate as contract consumption; generalise the Visual Validation guidance beyond Figma.
- **`agents/belmont/implementation-agent.md`** — add a contract branch to the Visual Validation self-check. Correct the rev-4 justification: `:190-197` is *not* a no-op without Figma (`:196` handles other reference images, `:197` mandates a "No design reference available" note), and `:272` is mode-agnostic and needs no change.
- **`skills/belmont/_src/verify.md`** — three narrow edits, not one: Step 1b's collect list (`:62-72`) is Figma/image/URL-only; the **sub-agent dispatch prompt** (`:109`, `:146`) steers the agent away from the new behaviour by enumerating only Figma/screenshot references and injecting "No design references found"; and the focused-reverify clause at `:50` needs the contract exemption. *This file has Steps, not Phases — `verification-agent.md` has Phases. Never conflate them.*
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
| `skills/belmont/_src/references/design-authority-baseline.md` | **New** — per-section ladder + baseline rules for all six contract sections, Belmont-original, crediting the five Yummy Labs skills by name and link |
| `skills/belmont/_src/product-plan.md` | Fix the same wrong skill names at `:165` |
| `skills/belmont/_src/implement.md` | Phase 2 purpose restated as contract consumption; Visual Validation generalised |
| `skills/belmont/_src/verify.md` | Step 1b collects the contract; dispatch prompt carries it; focused-reverify exemption at `:50` |
| `skills/belmont/_src/next.md` | Archive merges rather than clobbers `## Design Specifications` / `## Codebase Analysis` |
| `skills/belmont/_src/codex-plan-apply.md` | Carve planning artifacts out of the source-code prohibition so `design-preview.html` can be applied |
| `skills/belmont/_src/references/models-yaml-format.md` | Rewrite the `design` tier description (`:30`) and the profile heuristics (`:62-63`) |
| `agents/belmont/design-agent.md` | Consume the contract; replace `## Handling No Design`; read master TECH_PLAN. `FORBIDDEN ACTIONS` untouched |
| `agents/belmont/verification-agent.md` | Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; focused-reverify exemption; install-independent MCP names; **rework the `## Output Format` Visual Verification block (`:210-221`)** — Figma-shaped today, needs a contract branch and a Mechanism column |
| `agents/belmont/implementation-agent.md` | Contract branch in Visual Validation; install-independent MCP names |
| `prompts/belmont/post-verify-triage.md` | Non-deferrable contract list; de-Figma the blocking bullet |
| `plugin/` | Regenerate — `plugin/skills/{tech-plan,product-plan,verify,next,implement}/**` and `plugin/agents/*`. **Requires 0006** |
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
| **Plugin** | `plugin/agents/*` and `plugin/skills/{tech-plan,product-plan,verify,next,implement}/**` are a third tracked surface; `release.sh:41` regenerates and `:96` stages it |

**Correct the record on auto mode, per tool.** `AGENTS.md:26` and `dual-invocation-paths.md:9` say auto mode bypasses the tool's skill discovery and that Belmont injects the skill body, agent files and project state. For **six of eight CLIs** that is false: `buildLoopPrompt` emits a bare slash reference (`main.go:7168`, `:7189`) and Belmont injects **only** steering text (`:6887`) and milestone-scope prose; the agent reads everything else from disk. For **Pi and opencode**, `adaptPromptForTool` (`main.go:7143-7157`) deliberately rewrites the reference into a read-the-file instruction — so for those two, discovery *is* bypassed, on purpose. Correct both statements with that split, not with a blanket claim. This matters here: it is why the tier-1 ladder works under `claude -p`, which runs inside Claude Code's full skill runtime with `Skill` allowlisted (`main.go:7927`).

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

**Step 2 — the contract survives `[r]`-resume.**
*Precondition:* the worktree path engages only when a milestone **in range** carries `(depends: …)`; otherwise `hasExplicitDeps` is false (`main.go:5441-5453`) and `runAuto` routes to `runLoop` — master tree, no worktree, no `[r]` prompt. Confirm `{base}/PROGRESS.md` has `### M2: … (depends: M1)` first.
`belmont auto --feature <slug> --from M1 --to M2`; Ctrl-C during wave 1; re-run the identical command and answer `r` at the `Branch … exists from a previous run` prompt (`handleStaleWorktree`, `main.go:6100`).
Expect the banner `Belmont Auto (parallel)` — a serial fallback means the precondition was not met — and the worktree's `TECH_PLAN.md` to hash-match master's after resume.
The interrupted run may leave `.belmont/auto.json` untracked, which `requireCleanWorkingTree` rejects on the re-run; delete it or pass `--allow-dirty`.

**Step 3 — design-agent consumes, never writes.** After the wave **merges**:
`git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md` — expect **empty**.
Do **not** use a bare `git diff`: `.belmont/` is `--assume-unchanged` in a worktree, so `git diff`, `git diff -- .belmont/` and `git status --porcelain` all report nothing even after the file has been completely rewritten (measured). The real loss point is `syncFeatureStateAfterMerge`, which lands the worktree's copy on master — which is why this compares committed revisions.

**Step 4 — the gate fires.** Verify report `## Visual Verification` shows per-row pass/fail/UNVERIFIABLE with a Mechanism column, and the attestation reads `Contract checks performed: YES against {base}/TECH_PLAN.md`.

**Step 5 — per-task specs survive a fix round.** Force one FWLUP round so `/belmont:next` runs and re-archives MILESTONE. Expect the re-archived `MILESTONE-M1.done.md` to still carry populated per-task design sections.
The second verify is *focused* by default and legitimately reports `Contract checks performed: NO — focused re-verification, no UI follow-ups` unless the follow-ups touched UI.

**Step 6 — headless replan preserves the contract.** `actionReplan` **cannot be forced**: after two verify failures `decideLoopActionSmart` returns nil (`main.go:7642-7644`) and the AI decider may pick DEBUG instead. Test R4 in isolation rather than driving the loop: run `claude -p "/belmont:tech-plan --feature <slug>"` from the project root with the contract already present.
Expect the `## Design Contract` byte-identical except `**Approval**` rewritten to `unreviewed (headless replan <date>)`; no regeneration of approved sections.
Fail: a fresh contract → R4/R8 not implemented, and the approved design is silently gone.

**Step 7 — Figma regression.** A feature **with** Figma URLs. Expect `**Mode**: N/A — Figma present`, **no** Token Contract, per-task Figma sections and the Figma Sources table exactly as today, and a verify verdict of PASS (not FAIL) — this asserts the fourth enforcement rule does not fire without a contract.

**Step 8 — failed-load regression (must not break).** Deliberately invalid node id. Expect task BLOCKED, status FAILED, nothing invented, and **no fallback to a contract**.

**Step 9 — other tools.**
9a: `belmont auto --tool codex --feature <slug>` from `$BASE`.
9b: interactive `codex` → `/plan $belmont:tech-plan` → confirm the structured interview runs and a `BELMONT_PLAN_PACKET` is emitted carrying **both** `TECH_PLAN.md` and `design-preview.html`; apply with `$belmont:codex-plan-apply` and confirm the `.html` is not rejected as source code.
9c: interactive `opencode` on a **second** UI-bearing no-Figma feature → `/belmont/tech-plan --feature <slug2>`. Expect a contract schema-identical to Step 1's with `**Authorities**: baseline only`. *`belmont auto` cannot author a contract — the only auto path to tech-plan is `actionReplan` — so this must be the interactive path.*

**Step 10 — plugin surface.** `./scripts/generate-plugin.sh <ver>`; confirm `plugin/skills/tech-plan/references/design-authority-baseline.md` exists and is non-empty, and that `plugin/agents/*.md` are all non-zero (0006's fix).

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
- [ ] All five Yummy Labs skills credited with links; **no file from `~/.claude/skills/` copied into the repo**
- [ ] `design-authority-baseline.md` carries the **obtaining table** (§4.3) — Notion page link and `~/.claude/skills/<name>/` install path per skill, the `interactive-prototype` repackaging note, and a plain statement that none is bundled and tier 2 works without them
- [ ] Every link in that table checked resolving at branch time — they are third-party URLs and this PR does not control them
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
- [ ] `AGENTS.md:26` and `dual-invocation-paths.md:9` corrected **per tool** — bare slash reference for six CLIs, deliberate rewrite for Pi and opencode

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

## 12. Changes from v5

Rev 5 was reviewed by six adversarial lenses producing 72 findings; each was then attacked by an independent skeptic. **19 were refuted and are not acted on**; 53 survived. Full record: [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md).

**Author decisions, 2026-08-07**

| v5 | v6 |
|---|---|
| Contract for every UI feature, Figma or not | **Only UI features without Figma.** Figma features untouched — which also removes the unconditional-failure trap below |
| Scope ambiguous between feature and milestone | **One conversation per feature**, covering all milestones |
| Durability implied to be mechanical | **Prose-enforced and unguarded, stated as such.** Mechanical guard deferred to its own proposal |
| — | **Storybook added as the first project-local derivation rung**, read statically — planning sessions may not run build commands |

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

**Minors fixed** — file count corrected to 11; `**Design contract**` phantom field removed in favour of `**Mode**`; `user-questions.md:4` → `:8`; `frontend-design` → `frontend-design:frontend-design`; WCAG levels stated; two broken §10 cross-references repaired; plugin enumerations completed; `--check` DoD items replaced with checks that can actually fail; two vacuous DoD items deleted; `models-yaml-format.md` second location cited; portability paragraphs updated from two skills to five; `docs/` list extended to five pages; gitignored-`.belmont/` configuration handled.
