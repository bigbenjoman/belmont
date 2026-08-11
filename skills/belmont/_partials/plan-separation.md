## PRD ↔ UX_DESIGN ↔ TECH_PLAN Boundary

Belmont's planning workflow splits concerns across three documents, one per phase: `product-plan` writes the PRD, `ux-design` writes UX_DESIGN, `tech-plan` writes TECH_PLAN. Keeping the boundary clean prevents drift — the most common failure mode is two of them disagreeing after tech-plan refinements, which confuses the implementation agent.

### What belongs in the PRD (product surface)

- User goals, target audience, problem statement
- User flows and journeys (what the user does, step by step)
- Acceptance criteria and success criteria (measurable outcomes)
- Content, copy, and tone decisions
- UX behavior and product-level invariants (e.g. "hide the logo slot when no university matches"; "LSEGroup must NOT be matched as LSE")
- Out-of-scope statements
- Priority and scope
- Figma URLs / node IDs (by reference only — never the implementation path they render to)

### What belongs in the UX_DESIGN (design surface)

- Token *values*: the spacing scale, the type ratio and its sizes, the colour distribution and hues, the committed radius, the elevation levels
- The Accessibility Floor — contrast ratios, target sizes, focus and label rules. The PRD's target WCAG level is adopted here and may be raised, never lowered
- The per-**surface** State Inventory (default / hover / focus / active / disabled / loading / empty / error, plus gesture and transition states), and any omission's reason
- Screens, and the PRD's flows rendered screen-by-screen with each failure edge's landing state
- Microcopy *rules* — button verb style, error shape, empty-state shape, destructive confirmations. Never the strings themselves; the PRD already fixed those
- Motion duration bands, easing classes, and the reduced-motion rule — not the animation library
- Written by `/belmont:ux-design` only. Every other skill, `tech-plan` included, reads it and never writes it. **Design tokens extracted from Figma are the exception and stay in the TECH_PLAN** (below) — `tech-plan` owns Figma extraction

### What belongs in the TECH_PLAN (implementation surface)

- File paths (`src/...`) and directory structure
- Component architecture (wrapper components, sub-components, composition patterns)
- Direct-usage vs wrapper decisions (e.g. "use `<UniversityLogo>` wrapper" or "use `<Image>` directly")
- Icon / library imports and specific import identifiers (e.g. `Play` vs `PlayCircle` from `lucide-react`)
- Regex *syntax* and implementation patterns (`/\blse\b/i` vs `/lse/i`)
- tRPC / REST endpoint names the implementation commits to
- State management choices, styling approach, data-fetching approach
- Design tokens extracted from Figma, file-level TypeScript interfaces
- How the UX_DESIGN's token *values* are expressed — CSS variable names, theme layer, config files — and which components each of its surfaces maps to. Tech-plan may not add, remove or rename a surface

### Grey-zone rule

When a decision has both a product-visible invariant AND an implementation detail, split them:

- PRD keeps the **behavior**: "LSEGroup must not match as LSE."
- TECH_PLAN keeps the **implementation**: "Use `/(\blse\b|london school of economics)/i` — the `\b` word-boundary is what enforces the invariant."

If the user volunteers an implementation idiom during product-plan ("just use `<Image>` directly"), record it as an **open question for the tech-plan step** — do NOT commit to it in the PRD. Tech-plan may well decide otherwise, and baking it into the PRD creates drift.

## Milestone Sizing Rules

Milestones are the unit of execution for `belmont auto`. Each milestone runs in a single AI session inside an isolated worktree. Small milestones are load-bearing:

- Keep each within a single context window so the implementation agent can hold all relevant files, designs, and task detail at once.
- Keep verification tractable — the verify-agent and code-review-agent re-check the whole milestone; oversized milestones lead to shallow verification.
- Enable parallelism — independent small milestones can run concurrently in separate worktrees.

### Sizing targets

- **Target**: 3–5 tasks per milestone.
- **Soft ceiling**: 6. Going above should be rare and deliberate.
- A milestone is "too big" if any of:
  - More than ~5 tasks
  - Mixes unrelated domains (UI + backend + infra)
  - Requires loading multiple Figma files *and* a separate backend surface
  - Would force the agent to juggle context the verify-agent can't reasonably re-check in one pass

### Growing vs splitting

When new work is discovered during planning or review:

- **Default**: create a NEW milestone for the new work rather than inflating an existing one. Use `(depends: M<n>)` when the new milestone genuinely builds on another — only when there is a real file/API/data dependency, not just "related topic."
- **Favor parallelism**: if two clusters of new tasks don't share files or APIs, split them into separate milestones so `belmont auto` can run them in parallel worktrees.
- **Never** merge two small milestones just because they're topically similar — the cost of a too-large context is much higher than the cost of an extra milestone.

## Tech-plan's Back-update Contract

The tech-plan step is responsible for keeping the PRD and PROGRESS in sync with its own decisions. Implementation agents read the PRD — if it disagrees with TECH_PLAN, the implementation will be confused or wrong.

Rules for updating PRD/PROGRESS from the tech-plan session:

1. **Contradictions**: when a tech-plan decision contradicts PRD prose, correct the PRD in the same session. Don't leave two sources of truth.
2. **Refinements**: when a tech-plan decision disambiguates the PRD (e.g. "endpoint A or B" → "endpoint A"), update the PRD to commit to the resolved version. The orchestrator extracts context from the PRD — it must reflect the final decision.
3. **Leaked tech detail**: when the PRD contains technical prose that shouldn't be there (file paths, wrapper choices, icon imports, regex syntax), replace it with a behavior-only description or a short pointer: `See TECH_PLAN.md §<section>.`
4. **New Clarifications**: add product-facing decisions that crystallized during tech-plan (e.g. resolved ambiguities, confirmed invariants) into the PRD's `## Clarifications` section.
5. **PROGRESS dependency annotations**: ensure `(depends: ...)` annotations on milestone headings in PROGRESS.md match the TECH_PLAN's §Implementation Order. Mismatches cause auto-mode to serialize work that could run in parallel, or vice versa.
6. **Non-destructive**: always use Edit to modify specific sections. Never replace entire files. Preserve all existing content, task IDs, and completion status.
