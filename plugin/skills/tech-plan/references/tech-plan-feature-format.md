# Tech Plan: Feature TECH_PLAN.md Format

Write to `{base}/TECH_PLAN.md` with this structure.

> **Related output**: per-feature model tiers live in a separate file, `{base}/models.yaml`. The tech-plan skill writes it after Phase 4.6 (Model Tier Assignment). See `references/models-yaml-format.md` for the schema and tier semantics.

> **CRITICAL — the Design Contract does not live here.** It lives in
> `{base}/UX_DESIGN.md`, written and owned by `/belmont:ux-design`. This
> template carries no `## Design Contract` section, and `/belmont:tech-plan`
> never creates one — not in this file, not in `.belmont/TECH_PLAN.md`, not in
> any mode. Read the design authority, plan within it, and report conflicts
> rather than editing it.

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

## Design Tokens (from Figma)
[Write this section **only when a PRD task carries a Figma URL**. The exact values extracted
from Figma in Phase 1 — colours, spacing, typography, radius, effects. **Do not drop them**:
this is where a Figma feature's exact values live, and downstream agents read them from here.
Omit the heading entirely when there are no Figma URLs — `{base}/UX_DESIGN.md` is the design
authority in that case.]

---

## Component Specifications
[Every surface in `{base}/UX_DESIGN.md`'s State Inventory maps to at least one entry here, naming
the states that entry must render. Never add, remove or rename a surface.]
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
[This table stays in TECH_PLAN.md — `Location` is a file path, which is technical. Fill it from
`{base}/UX_DESIGN.md`'s `## Screens` table: the design authority names the surfaces, this table
names the files that render them.]
| Component | Location | Usage |
|-----------|----------|-------|

---

## State Management
[Server state approach, client state approach]

---

## Verification Checklist
### Per-Component Checks
`**Mode**` is read from `{base}/UX_DESIGN.md`'s `## Design Contract` header, not from this file.
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
