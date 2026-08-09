# Tech Plan: Feature TECH_PLAN.md Format

Write to `{base}/TECH_PLAN.md` with this structure.

> **Related output**: per-feature model tiers live in a separate file, `{base}/models.yaml`. The tech-plan skill writes it after Phase 4.6 (Model Tier Assignment). See `references/models-yaml-format.md` for the schema and tier semantics.

> **CRITICAL — `## Design Contract` is not yours to regenerate.** This is a
> whole-file template, so writing it naively destroys an approved contract.
> Reproduce any existing `## Design Contract` section **byte-for-byte**,
> including its `**Approval**` line, unless Phase 3.5 derived and the user
> approved a new one in this session. See the tech-plan skill's Phase 3.5 for
> the full rule and its headless clause.

```markdown
# Technical Plan: [Feature Name]

> **Master Tech Plan**: See `.belmont/TECH_PLAN.md` for stack, conventions, and cross-cutting architecture decisions.

## Overview
[2-3 sentences on what we're building]

## PRD Task Mapping
| Code Section                          | Relevant PRD Tasks | Priority |
|---------------------------------------|--------------------|----------|
| src/components/feature/ComponentA.tsx | P0-1, P1-2         | CRITICAL |

---

## File Structure
```
src/
├── app/
│   └── feature/
│       ├── page.tsx              # Main page (Tasks: P0-1)
│       └── layout.tsx            # Layout wrapper
├── components/
│   └── feature/
│       ├── ComponentA.tsx        # [description] (Tasks: P1-1)
│       └── index.ts              # Barrel export
├── lib/
│   └── feature/
│       ├── api.ts                # API functions (Tasks: P0-2)
│       ├── types.ts              # TypeScript types
│       └── utils.ts              # Helper functions
└── hooks/
    └── useFeature.ts             # Custom hook (Tasks: P1-4)
```

---

## Design Contract
**Mode**: [derived — UI, no Figma | N/A — no UI | N/A — Figma present]
**Source**: [storybook (<url>) | storybook (local) | tailwind.config.ts | globals.css |
             components.json | master TECH_PLAN | sibling feature <slug> |
             none — established here]
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

### Design Tokens (from Figma)
[Only when **Mode** is `N/A — Figma present`. The exact values extracted from Figma in
Phase 1 — colours, spacing, typography, radius, effects. Omit this subheading entirely in
the other two modes.]

### Motion Contract
**Applies**: [yes | N/A — no motion in this feature]
Duration bands — instant feedback ≤ 100ms · state change 150–200ms · context shift 250–300ms ·
page transition 300–400ms. Above 400ms must justify itself.
Easing — one documented curve per class (enter / exit / move). Bounce and overshoot are opt-in per
product voice, never a default.
Performance — animate `transform` and `opacity` only. Animating layout properties is a defect.
Reduced motion — `prefers-reduced-motion: reduce` removes movement, never functionality.

---

## Component Specifications
### ComponentA.tsx
**PRD Tasks**: P1-1, P1-2
**Figma Node**: [node-id if applicable]
**Reuses**: ExistingComponent from src/components/ui

[TypeScript interface and skeleton code]

**Styling Notes**: [Tailwind classes, responsive behavior]
**State Management**: [Local state, server state approach]
**Error Handling**: [Empty, loading, error states]

---

## API Integration
### Endpoints Used
| Endpoint | Method | Purpose | Tasks |
|----------|--------|---------|-------|

### Data Types
[TypeScript interfaces for API data]

---

## Existing Components to Reuse
| Component | Location | Usage |
|-----------|----------|-------|

---

## State Management
[Server state approach, client state approach]

---

## Verification Checklist
### Per-Component Checks
- [ ] **If `**Mode**` is `N/A — Figma present`**: matches Figma design pixel-perfect
- [ ] **If `**Mode**` is `derived — UI, no Figma`**: spacing, type sizes, colours, radius and
      elevation all on the declared scales; contrast and touch targets meet the Accessibility
      Floor; every State Inventory entry renders; motion within the declared bands
- [ ] Responsive: mobile, tablet, desktop
- [ ] Accessibility: keyboard nav, screen reader
- [ ] Loading/error/empty states implemented

### Commands
Use the project's package manager (detect via lockfile: `pnpm-lock.yaml` → pnpm, `yarn.lock` → yarn, `bun.lockb`/`bun.lock` → bun, `package-lock.json` → npm):
```bash
<pkg> run lint:fix
npx tsc --noEmit
<pkg> run test
<pkg> run build
```

---

## Edge Cases
| Scenario | Handling |
|----------|----------|

---

## Implementation Order

List each milestone from PROGRESS.md with its dependency declaration. PROGRESS `(depends: ...)` annotations MUST match this section — if they drift, auto-mode will serialize parallelizable work or vice versa.

- **M1: [Name]** — independent (wave 1)
- **M2: [Name]** — independent (wave 1, can run in parallel with M1)
- **M3: [Name]** — depends: M1, M2 (wave 2)
- **M4: [Name]** — depends: M3 (wave 3)

Brief rationale per milestone (one line): why it depends on what it depends on, or why it's independent.

---

## Notes for Implementing Agent
- Follow existing patterns in [reference file path]
- Skills to load: [relevant skills list]
- When in doubt about design, check Figma node [id]
```
