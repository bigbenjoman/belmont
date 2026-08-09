# Progress: [Feature Name]

## PRD Reference
.belmont/PRD.md

## Milestones

### M1: [Milestone Name]
- [ ] P0-1: Task description
- [ ] P0-2: Task description

### M2: [Milestone Name] (depends: M1)
- [ ] P1-1: Task description

### M3: [Milestone Name] (depends: M1)
- [ ] P1-1: Task description

### M4: [Milestone Name] (depends: M2, M3)
- [ ] P1-1: Task description

> **Dependency syntax**: Add `(depends: M1)` or `(depends: M1, M3)` after the milestone name to declare dependencies. When dependencies are present, `belmont auto` will run independent milestones in parallel via git worktrees. If no milestones have `(depends: ...)`, they run sequentially (default behavior).

> **Task states**: `[ ]` todo, `[>]` in_progress, `[x]` done, `[v]` verified, `[!]` blocked, `[-]` withdrawn. The letter markers are case-insensitive (`[X]`, `[V]`). Nothing else parses — Belmont will not guess a state it cannot read.
>
> **Withdrawn** is for work that was planned and then deliberately dropped: superseded, duplicated by another task, relocated to another feature, or descoped. Mark it `[-]` and record *why* in `## Decisions Log`. Do **not** delete the line — a deletion does not survive a sibling worktree merge and the task comes back as outstanding work.
>
> Milestone status is computed from its tasks — do not add status emoji to milestone headers.

## Session History

| Date | Action | Details |
|------|--------|---------|

## Decisions Log

(none yet)
