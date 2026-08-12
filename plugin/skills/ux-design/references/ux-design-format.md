# UX Design: UX_DESIGN.md Format

Write to `{base}/UX_DESIGN.md` with this structure.

> **The ordering is load-bearing — do not reorder.** `## Design Contract` is first so its
> four header fields land in the first ~10 lines: `/belmont:implement`, `/belmont:verify`
> and the verification agent read **only** the `**Mode**` line, and a head-read must
> suffice. `## Review Artifacts` comes second so a human opening the file sees the two
> HTML links before scrolling. Screens and flows — the human-facing bulk — come last.

> **Figma tokens do not live here.** `/belmont:tech-plan` extracts them and writes them to
> `{base}/TECH_PLAN.md`'s `## Design Tokens (from Figma)`. Under `**Mode**: N/A — Figma
> present` this file is the header block and nothing else.

```markdown
# UX Design: [Feature Name]

> **Written and owned by `/belmont:ux-design`.** No other skill writes this file.
> `/belmont:tech-plan`, `/belmont:implement`, `/belmont:verify`, `/belmont:next`,
> `/belmont:debug-auto`, `/belmont:debug-manual` and `/belmont:review-plans` read it
> and never write it. To change it, re-run `/belmont:ux-design --feature <slug>`.

## Design Contract
**Mode**: [derived — UI, no Figma | N/A — no UI | N/A — Figma present]
**Source**: [storybook (<url>) | storybook (local) | tailwind.config.ts | globals.css |
             components.json | master UX_DESIGN | sibling feature <slug> |
             none — established here. Name EVERY rung you took content from, one
             per family where they differ — a story index gives components and
             states but no tokens, so that case reads
             `storybook (<url>) — components & states; tailwind.config.ts — tokens`]
**Authorities**: baseline[; ui-designer (tokens); ux-designer (strategy, states); ux-copywriter (microcopy);
             ux-motion (motion); frontend-design:frontend-design (aesthetic)]
**Approval**: [approved <ISO date> | unreviewed (headless replan <ISO date>) — legacy value,
             written only by pre-extraction `/belmont:tech-plan` Phase 3.5 runs. No current
             path writes it; preserve it byte-for-byte where you find it.]

[Under either `N/A` mode the four fields above are the whole file. Stop here.]

### Token Contract
Spacing — 8pt grid: 4, 8, 12, 16, 24, 32, 48, 64, 96. Internal ≤ external on every component.
Typography — ratio [1.25], sizes [list — every one on the named ratio], line-height 1.4–1.6 body /
  1.1–1.3 heading. Max 4 sizes. One family carries display and body; name any outlier slot and the
  family it uses. Headings roman, never italic.
Colour — 60/30/10. Max 3 hues + neutrals. Never pure #000/#FFF. Body ≥ 4.5:1.
  Each semantic (success/error/warning/info) declares bg, border, text, icon.
Radius — committed value [8px]. Nested children strictly smaller than parent.
Elevation — [levels]. Interactive elements rise one level on hover.

### Accessibility Floor
Targets ≥ 44×44px (SC 2.5.5, AAA — adopted as Belmont's floor) · contrast ≥ 4.5:1 text / 3:1 large
(SC 1.4.3, AA) · visible focus on every interactive element (SC 2.4.7, AA) · every input has a visible
label, never placeholder-only · reflow at 320px with no horizontal scroll (SC 1.4.10, AA) ·
`prefers-reduced-motion` respected (SC 2.3.3, AAA) · no meaning by colour alone.

| Pair (fg on bg) | Computed | Floor |
|-----------------|----------|-------|
[One row per text/background pair the Token Contract commits to. Every number here is one you
computed by WCAG 2.x relative luminance on those exact values — never estimated, never inherited
from the source you took the palette from. If you could not compute one, write `not computed` and
say why; a number nobody calculated reads as evidence and is worth less than nothing.]

[If the PRD names a target WCAG level, adopt it here and say so. This floor may be raised, never lowered.]

### UX Strategy
User · emotional state on arrival · hero element · primary action · biggest UX risk. Five lines.

### State Inventory
Per interactive **surface** the feature introduces — a control or region the user acts on, named
from the PRD's flows and `## Screens` below, never from a file path or a component name:
default · hover · focus · active · disabled · loading · empty · error, plus any gesture or
transition state the interaction introduces (drag, swipe, long-press, optimistic update).
A surface the feature **reuses unchanged** gets no entry. Omissions carry a one-line reason.

`/belmont:tech-plan` maps each surface to one or more components in its `## Component
Specifications`. It may not add, remove or rename a surface here.

| Surface | States | Omitted (+ reason) |
|---------|--------|--------------------|

### Microcopy Rules
Buttons: verb matching the outcome, never "Submit". Errors: what happened / why / what next, never
blame or jokes. Empty states: what appears here, why it's empty, the action to fill it.
Destructive confirmations: name what is destroyed. Possessive framing for user-owned objects.
[Rules only. The PRD already fixed the copy *content* — do not restate or re-decide strings here.]

### Motion Contract
**Applies**: [yes | N/A — no motion in this feature]
Duration bands — instant feedback ≤ 100ms · state change 150–200ms · context shift 250–300ms ·
page transition 300–400ms. Above 400ms must justify itself.
Easing — one documented curve per class (enter / exit / move). Bounce and overshoot are opt-in per
product voice, never a default.
Performance — animate `transform` and `opacity` only. Animating layout properties is a defect.
Reduced motion — `prefers-reduced-motion: reduce` removes movement, never functionality.

---

## Review Artifacts

Both are **self-contained** (no external assets, no network references, no scripts) and live
beside this file. Links are relative on purpose — `copyBelmontStateToWorktree` copies `{base}`
wholesale into every worktree, and an absolute path would break there.

- [design-preview.html](./design-preview.html) — colour swatches with computed contrast ratios,
  the type ramp, the spacing scale, radius and elevation samples, one state row per State
  Inventory entry.
- [ux-flows.html](./ux-flows.html) — one panel per screen with its states and microcopy in place,
  and an inline-SVG diagram per flow.

---

## Screens

| Screen / view | Reached from | Purpose (one line) | Surfaces it contains |
|---------------|--------------|--------------------|----------------------|

## Flows

[One subsection per flow. Key each to the PRD's `## Acceptance Criteria (BDD)` scenarios — this
section **renders** the flows the PRD already fixed; it does not re-elicit them.]

### Flow: [Name]  (PRD scenario: [name] · tasks: [IDs])

1. **Entry** — [where the user arrives from, and in what state]
2. [step] → [screen] → [surface acted on]
3. …

- **Success** — [end state the user sees]
- **Failure** — [what can go wrong] → lands in [surface]'s `error` / `empty` state

---

## Open Design Questions
[Anything the interview could not settle. Empty is fine — say "None".]
```

## Master UX_DESIGN.md Format

Write `.belmont/UX_DESIGN.md` **only on the first feature of a project for which you derive a
contract**, and only then. It is rung 0 of the derivation ladder. It carries **two subsections
and nothing else** — the ones that must be identical across every feature, or feature 1 and
feature 4 mint competing scales with nothing to reconcile them.

```markdown
# UX Design: [Product Name] (cross-cutting)

**Source**: [...]
**Authorities**: [...]
**Approval**: approved <ISO date>

### Token Contract
[as above]

### Accessibility Floor
[as above]
```

No `**Mode**` line — mode is per-feature. No flows, no screens, no State Inventory, no artifacts.
A feature that inherits from it sets `**Source**: master UX_DESIGN` and repeats the inherited
values inline, so a feature file is readable on its own and the verification agent's
anti-circularity rule — keyed on `**Source**` — stays armed.
