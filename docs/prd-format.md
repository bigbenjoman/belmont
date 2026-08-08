# PRD & Progress Format

## PRD.md

The PRD is a **living specification** -- purely requirements, no status markers. It is actively curated as the source of truth for *what* to build. Task headers have no status emoji.

```markdown
# PRD: Feature Name

## Overview
Brief description of the feature.

## Problem Statement
What problem does this solve?

## Success Criteria (Definition of Done)
- [ ] Criterion 1
- [ ] Criterion 2

## Acceptance Criteria (BDD)

### Scenario: User logs in
Given a registered user
When they enter valid credentials
Then they see the dashboard

## Technical Approach
High-level implementation strategy.

## Target Workspace(s) — optional, monorepos only
If `belmont status` shows a `Monorepo:` line, list the workspaces this feature touches by ID and path. Mark the primary (the one whose dev server uses `$BELMONT_PORT`):

- `web` (`packages/web`) — primary
- `api` (`apps/api`)

Tasks may additionally use `[WEB]` / `[API]` prefixes (e.g. `[WEB] Render the new shell`) so implementation agents know which workspace each task targets. Omit this section entirely for single-package projects.

## Tasks

### P0-1: Set up authentication
**Severity**: CRITICAL
**Task Description**: Users can sign in to the product and return to a protected dashboard.
**Solution**: A sign-in screen accepts a Google account; on success the user lands on the dashboard with their name and avatar. Signed-out users hitting a protected page are redirected to sign-in.
**Verification**: New user completes Google sign-in and reaches the dashboard; signed-out visit to `/dashboard` redirects to sign-in.

### P1-1: Create dashboard layout
**Severity**: HIGH
**Task Description**: Build the main dashboard page
**Figma**: https://figma.com/file/xxx/node-id=123
**Solution**: Responsive layout matching the Figma node at mobile, tablet, and desktop breakpoints. Sidebar is collapsible on mobile.
**Verification**: Visual parity with Figma at all three breakpoints; sidebar collapses below md.
```

**Figma is one design input among several, not the design source.** A `**Figma**:` field gives the design-agent an exact reference to extract from, and gives verification something to compare screenshots against. It is not required. Where a feature has a UI but no Figma URL on any task, `/belmont:tech-plan` derives a **Design Contract** into the feature's `TECH_PLAN.md` instead, and that becomes the design authority — see [workflow.md](workflow.md). Where a feature has no UI at all, neither applies.

Note the granularity: a feature counts as a Figma feature if **any** task carries a URL. Mixing covered and uncovered UI tasks in one feature means no contract is derived, because there is no single authority to reconcile the two against — so prefer either annotating every UI task or none.

**Key points:**
- No status markers (emoji) on task headers -- status lives in PROGRESS.md only
- Follow-up tasks discovered during implementation are added as plain tasks (no special tag)
- The `**Verification**:` field lists *criteria* for the task, not a separate task. Do not create standalone "Verification", "QA", or "Unit Tests" tasks — verification runs automatically via `/belmont:verify` after each milestone.
- PRD tasks describe **WHAT** the user sees or experiences, not HOW it's implemented. Technical decisions (libraries, file paths, wrapper components, endpoint names, regex syntax) belong in `TECH_PLAN.md`. The tech-plan step reconciles PRD and TECH_PLAN at the end of its session — see `skills/belmont/_partials/plan-separation.md` for the boundary rules.

## Priority Levels

| Priority | Severity | Meaning                               |
|----------|----------|---------------------------------------|
| P0       | CRITICAL | Must be done first, blocks other work |
| P1       | HIGH     | Core functionality                    |
| P2       | MEDIUM   | Important but not blocking            |
| P3       | LOW      | Nice to have                          |

## PROGRESS.md

**Single source of truth for all state.** Tracks task status, milestones, session history, and decisions. Milestone status is computed from tasks (no emoji on milestone headers). There is no separate `## Blockers` section or `## Status:` line -- blocked tasks use the `[!]` checkbox and overall status is computed.

### Task States

| Checkbox | State       | Meaning                                                |
|----------|-------------|--------------------------------------------------------|
| `[ ]`    | Todo        | Not yet started                                        |
| `[>]`    | In Progress | Currently being worked on                              |
| `[x]`    | Done        | Task finished, not yet verified                        |
| `[v]`    | Verified    | Task finished and verified                             |
| `[!]`    | Blocked     | Cannot proceed (missing info, Figma unavailable, etc.) |
| `[-]`    | Withdrawn   | Planned, then deliberately dropped                     |

**The letter markers are case-insensitive** — `[X]` and `[V]` mean exactly what
`[x]` and `[v]` mean. One rule, easy to remember; a state reachable by a shift
key has to parse.

**`[x]` is not the finish line.** A feature whose tasks are all `[x]` reports `Complete`, not `Verified`, and `belmont status` warns that verification never recorded a result — recover with `belmont reverify --feature <slug>`. Only `[v]` means verified.

**`[-]` withdrawn is a state, not a deletion.** It covers work that was
superseded, duplicated by another task, relocated to another feature, or
descoped. It is neither outstanding nor done: excluded from both counts, never
offered as next work, and it does not stop a milestone reading complete. Record
*why* in `## Decisions Log` — a marker cannot carry a reason.

Do **not** express withdrawal by deleting the line. Belmont's merge takes the
worktree's document as its base and carries the other side's missing lines back
in, so a deleted task is resurrected by the next sibling sync — in either
direction. A marker survives, and a withdrawal wins from either side of a merge,
so a stale worktree cannot silently revive dropped work.

**Headings inside a milestone.** A `## ` heading at column zero ends the milestones
region — that is how `## Session History` works. Inside a task's write-up, indent any
quoted `##` by two spaces so it reads as part of that task's body; `###` and deeper are
always safe. A task line that ends up outside every milestone is counted by nothing, so
`belmont status` and `belmont validate` both report it rather than dropping it silently.

**Any other marker is an error, not a state.** Belmont does not guess: an
unrecognised marker is excluded from the counts, rendered `[?]`, never offered
as the next task, prevents its milestone reading as complete, and makes
`belmont validate` exit 1. Merge auto-resolution also refuses to touch a file
containing one. Earlier versions silently treated it as `[ ]` todo, so cancelled
or relocated work was counted as outstanding and handed to an agent to build.

Fix it with `belmont repair`, which checks each one against the commit log and
then against the current code — see [cli-commands.md](cli-commands.md). Editing
the marker by hand works too. Do **not** delete the line: if the work was
dropped, that is `[-]` withdrawn. There is no in-tool override, and skipping the
milestone will not clear it.

> **Upgrading?** If a PROGRESS.md already contains a stray marker, this is a
> behaviour change. `belmont validate` will now exit 1 on it, and
> `belmont auto` (single feature, non-interactive) will refuse to start where it
> previously ran and quietly treated the entry as outstanding work. Run
> `belmont status` to see every offending line with its line number, then
> `belmont repair` to fix them. Note the lint does **not** run for
> `belmont auto --features` / `--all`, and on a TTY it asks before aborting.
>
> `[-]` and `[V]` are the other half of that change, in the opposite direction:
> both used to be unrecognised and now parse, as withdrawn and verified. A file
> that already contains either **changes meaning on upgrade**, with no
> migration. If yours does, run `belmont status --feature <slug>` before your
> next `belmont auto` and check the reading is the one you meant: a `[-]` you
> wrote to mean something else (parked, not applicable) is now excluded from the
> counts and can let its milestone read complete, and a hand-written `[V]` now
> claims verified without any commit evidence behind it.

### Example

```markdown
# Progress: Feature Name

## PRD Reference
.belmont/PRD.md

## Milestones

### M1: Foundation
- [v] P0-1: Set up authentication
- [v] P0-2: Database schema

### M2: Core Features
- [>] P1-1: Dashboard layout
- [ ] P1-2: User settings

## Session History
| Session | Date/Time           | Context Used    | Milestones Completed |
|---------|---------------------|-----------------|----------------------|
| 1       | 2026-02-05 10:00:00 | PRD + TECH_PLAN | M1                   |

## Decisions Log
[Numbered list of key decisions with rationale]
```

### Master PROGRESS.md

The master PROGRESS.md (at `.belmont/PROGRESS.md` in multi-feature projects) contains a features table with these columns:

| Feature | Priority | Dependencies | Status | Milestones | Tasks |
|---------|----------|--------------|--------|------------|-------|

Feature-level status is computed from task states. There is no separate status line.

### Master PRD.md and TECH_PLAN.md

- **Master PRD.md** -- Living global document covering vision, constraints, and cross-cutting decisions. No features table (that lives in master PROGRESS.md).
- **Master TECH_PLAN.md** -- Living global document for cross-cutting architecture decisions.
