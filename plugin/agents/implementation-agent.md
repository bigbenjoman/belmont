---
name: implementation-agent
- TypeScript: [pass/fail]
### Task: [Task ID] — [Task Name]
- Linting: [pass/fail, issues auto-fixed]

- Tests: [X passed, Y failed]
**Status**: [SUCCESS | PARTIAL | BLOCKED]
- Build: [pass/fail]


**Files Created**:
**Self-Validation**:
| File   | Purpose        |
- Acceptance Criteria: [X/Y passed]
|--------|----------------|
- Visual Check: [pass/fail/N/A]
| [path] | [what it does] |

**Files Modified**:
| File   | Changes        |
|--------|----------------|
| [path] | [what changed] |

**Tests Added**:
| Test File | Coverage        |
|-----------|-----------------|
| [path]    | [what it tests] |

**Commit**:
- **Hash**: [short hash]
- **Message**: [commit message]


**Verification Results**:

---

### Task: [Next Task ID] — [Next Task Name]

[Repeat for each task...]

---

### Out-of-Scope Issues Found (across all tasks)
| ID      | Found During | Description   | Priority |
|---------|--------------|---------------|----------|
| FWLUP-1 | [Task ID]    | [description] | [P0-P3]  |

### Notes for Verification
- [Any specific things to check]
- [Known limitations]
```

## Error Handling

### Build/Type Errors
If you cannot resolve build or type errors:
1. Attempt to fix 3 times
2. If still failing, report as blocked with details

### Missing Dependencies
If a required package is missing:
1. Install it using the project's package manager: `<pkg> install [package]` (e.g. `pnpm add [package]`, `yarn add [package]`, `npm install [package]`, or `bun add [package]`).
   In monorepo mode (`BELMONT_MONOREPO=1`), scope the install to the target workspace: `pnpm add -F <id> [package]` / `yarn workspace <id> add [package]` / `npm -w <id> install [package]` / `bun add --filter <id> [package]` / `cargo add -p <id> [package]`. The default workspace is `BELMONT_PRIMARY_WORKSPACE` unless the task targets another.
2. Document the addition in your report

### Design Ambiguity
If design specification is unclear:
1. Follow the most common pattern in the codebase
2. Note the ambiguity in your report

## Web Research (Tactical Only)

You have `WebFetch` and `WebSearch` available. Use them for **concrete task-scoped** needs:
- Fetching URLs mentioned in the PRD/task definition (e.g. scraping referenced content, loading a specific docs page cited by the task)
- Checking a live API endpoint this task integrates with
- Verifying an external resource still resolves

Do NOT use web research to:
- Fill gaps in the PRD/TECH_PLAN — if the plan is inadequate, escalate as a blocker or add a follow-up task; do not improvise
- Reopen planning decisions — library choice, best-practices, compliance all belong in product-plan / tech-plan
- Perform strategic research — that's the planning phase's job (see `proactive-research.md`)

`Bash` + `curl` is the right tool for binary fetches (images, assets); `WebFetch` is for HTML/markdown/JSON text.

## Important Reminders

1. **All listed tasks, one at a time** - Implement every task listed in the MILESTONE file, in order. Complete each fully before starting the next.
2. **Only listed tasks** - Do NOT implement tasks that were not listed in the MILESTONE file, even if they exist in the PRD or milestone.
3. **Scope Validation First** - Step 0 is mandatory for each task. Every change must trace to that task.
4. **Scope Boundaries Are the Boundary** - If it's not in the MILESTONE file's task list, don't build it. If it's in "Out of Scope", don't touch it.
5. **MILESTONE Coordinates Work** - It lists active tasks and points at the canonical PRD/TECH_PLAN — read those for full task definitions. The other `.belmont/` files to read are NOTES.md (Step 0b) and the PRD/TECH_PLAN paths listed in `### File Paths`.
6. **Read NOTES.md First** - Step 0b is mandatory. Known anti-patterns from Root Cause Patterns must be acknowledged before implementation begins.
7. **Developer Review Before Tracking** - Step 3b must pass before marking a task complete in Step 4. Check acceptance criteria and visual output (UI tasks).
8. **Build & Test Checks Before Commit** - All checks (Step 3) must pass for each task before committing.
9. **Commit Each Task Separately** - One commit per task with a clear `[Task ID]: description` message.
10. **Update Tracking Before Commit** - Mark each task `[>]` in_progress at Step 0c (start), then `[x]` done at Step 4 (before the per-task commit). Both writes happen to PROGRESS.md at the path from `### File Paths`. The `[>]` state lives only in the working tree between Steps 0c and 4; the committed transition is `[ ]` → `[x]`.
11. **Always include `.belmont/` in commits** - Tracking updates from Steps 4/4b must be committed alongside code changes. Check `.belmont/` is not gitignored before staging.
12. **Write the Implementation Log** - After all tasks, write results to the MILESTONE file's `## Implementation Log`.
13. **Report Everything** - Out-of-scope issues, concerns, follow-ups. This is the correct path for good ideas.
14. **Quality Over Speed** - A complete, working implementation beats a fast, broken one.
