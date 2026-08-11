# Full Workflow

A step-by-step walkthrough of a complete Belmont session, from product vision to iteration.

## 0. Define Product Vision (optional)

If you're building a product with multiple features, start with a PR/FAQ to define the strategic vision.

```
Claude Code:  /belmont:working-backwards
Cursor:       Enable the belmont working-backwards rule, then: "Let's define the product vision"
Other:        Load skills/belmont/working-backwards.md as context
```

**What happens:**
- You describe the product idea and target customer
- AI asks focused questions about the problem, solution, and key benefit
- AI writes a PR/FAQ: press release + customer/stakeholder FAQs + appendix
- AI writes `.belmont/PR_FAQ.md`

## 1. Install

```bash
cd ~/projects/my-app
belmont install
```

## 2. Plan

Start an interactive planning session. Describe what you want to build. The AI will ask clarifying questions, then write a structured PRD with prioritized tasks organized into milestones.

```
Claude Code:  /belmont:product-plan
Cursor:       Enable the belmont product-plan rule, then: "Let's plan a new feature"
Other:        Load skills/belmont/product-plan.md as context
```

**What happens:**
- You describe the feature
- AI asks questions one at a time (edge cases, dependencies, Figma URLs, etc.)
- You finalize the plan together
- AI writes `.belmont/PRD.md` and `.belmont/PROGRESS.md`

It is strongly recommended you read the PRD created yourself. You can manually make edits before tech plan/implementation or you can run `belmont:product-plan` again and tell it what to refine.

## 3. UX Design (recommended before the tech plan)

Derive the feature's design authority before any technical decision is taken. This is the step that gives a UI feature with no Figma something for verification to measure against.

```
Claude Code:  /belmont:ux-design
Cursor:       Enable the belmont ux-design rule, then: "Let's design the UX for this feature"
Other:        Load .agents/skills/belmont/ux-design/SKILL.md as context
```

**What happens:**
- AI reads the PRD — flows, copy, edge cases and design intent are already settled there and are rendered, never re-elicited
- **If the feature has a UI and no Figma URLs anywhere in its PRD**, the AI walks a derivation ladder (master `.belmont/UX_DESIGN.md` → sibling contract → Storybook → `tailwind.config.*` / CSS custom properties / `components.json` → baseline defaults), asks only what no rung supplies, and presents a **Design Contract** for approval: design tokens, an accessibility floor, UX strategy, a state inventory per interactive surface, microcopy rules, and a motion contract
- On approval it writes `.belmont/features/<slug>/UX_DESIGN.md`, plus two self-contained review pages beside it — `design-preview.html` (tokens, contrast, states) and `ux-flows.html` (screens, real microcopy, a diagram per flow)
- The file is written for **every** feature the skill runs against. Features with Figma URLs record `**Mode**: N/A — Figma present`; features with no UI record `**Mode**: N/A — no UI`. Only `derived — UI, no Figma` is a contract, and `**Mode**` — never the file existing — is what downstream agents read
- On a project's first UI feature it also writes the master `.belmont/UX_DESIGN.md` (Token Contract + Accessibility Floor only), so later features inherit one scale
- **Interactive only.** `belmont auto` never invokes it — a contract is an approval artifact and there is nobody to approve it in a headless run. Run non-interactively, the skill writes nothing and says so
- **Only `/belmont:ux-design` writes these files.** Tech-plan, implement, verify, next, debug and `review-plans` read them and never write them; inside an auto run, a phase that edits either file has the change reverted by the CLI and the next phase told why
- **It is not retroactive.** A feature planned before contracts existed keeps its existing acceptance-criteria verification — applying a new quality standard to an already-approved plan would fail milestones on a bar their author never agreed to. To adopt it, run `/belmont:ux-design --feature <slug>`. If that feature carries a `## Design Contract` in its `TECH_PLAN.md` from an older Belmont, the skill offers to move it across verbatim, approval stamp untouched, and delete the old section — never two contracts

## 4. Tech Plan (recommended)

Have a senior architect agent review the PRD and produce a detailed technical plan. This step is optional but strongly recommended -- it produces the TECH_PLAN.md that guides the implementation agents.

You may add any additional context to the tech plan agent that you want to include.

```
Claude Code:  /belmont:tech-plan
Cursor:       Enable the belmont tech-plan rule, then: "Let's review the technical plan"
Other:        Load skills/belmont/tech-plan.md as context
```

**What happens:**
- AI reads the PRD and explores the codebase
- Interactive discussion about architecture, patterns, edge cases
- **Where the PRD carries Figma URLs**, the AI loads the designs and records the extracted values in the feature's `TECH_PLAN.md` under `## Design Tokens (from Figma)`. Figma extraction lives here, not in the UX design step.
- **`{base}/UX_DESIGN.md` is read-only for this skill**, in every mode. Token values, the accessibility floor, the state inventory and the motion bands are fixed there; the tech plan decides only how to express them — theme layer, CSS variable naming, animation library. A technical constraint that contradicts the contract is reported, and you re-run `/belmont:ux-design` to change it.
  - If the feature has visible UI and no `UX_DESIGN.md`, the AI says so and offers to stop so you can run `/belmont:ux-design` first. Advisory, never blocking — and it never derives a contract itself, in interactive or headless mode.
- AI writes `.belmont/TECH_PLAN.md` with file structures, component specs, API types
- AI assesses per-agent effort (codebase / design / implementation / verification / code-review / reconciliation) and proposes **model tiers** — `low` / `medium` / `high` per agent. You confirm or adjust; the choice is written to `.belmont/features/<slug>/models.yaml`. If you accept Belmont defaults, no file is written and each agent inherits the session model (auto mode forces the high tier only for planning and reconciliation). See `skills/belmont/references/models-yaml-format.md` for the schema and tier → model mapping per CLI.

## 5. Implement

Run the implementation pipeline. The AI finds the next incomplete milestone and works through each task using the 4-phase agent pipeline.

```
Claude Code:  /belmont:implement
Cursor:       Enable the belmont implement rule, then: "Implement the next milestone"
Other:        Load skills/belmont/implement.md as context
```

**What happens:**
1. Orchestrator creates `.belmont/MILESTONE.md` with task list, PRD context, and TECH_PLAN context
2. `codebase-agent` reads MILESTONE, scans codebase, writes patterns to MILESTONE *(parallel with 3)*
3. `design-agent` reads MILESTONE and derives design specs — from Figma where it exists, otherwise against the feature's Design Contract in `{base}/UX_DESIGN.md` — and writes them to MILESTONE *(parallel with 2)*
4. `implementation-agent` reads MILESTONE (only), writes code, tests, verification, commits
5. PROGRESS.md task states are updated, follow-up tasks added as plain `[ ]` entries
6. MILESTONE file is archived (`MILESTONE-M2.done.md`)

**After all tasks in the milestone:**
- Milestone is marked complete in PROGRESS.md
- MILESTONE file is archived
- Summary is reported

## 6. Quick Fix (optional)

If verification created follow-up tasks or there's a small task to knock out, use `next` to implement just one task without the full pipeline overhead.

```
Claude Code:  /belmont:next
Cursor:       Enable the belmont next rule, then: "Implement the next task"
Other:        Load skills/belmont/next.md as context
```

**What happens:**
- Finds the next unchecked task in the current milestone
- Creates a minimal MILESTONE file with the task's context (skips analysis sub-agents)
- Dispatches the single task to the implementation agent
- Task is implemented, verified, committed, and marked complete
- MILESTONE file is archived
- Reports a brief summary

## 7. Verify

Run comprehensive verification on all completed work.

```
Claude Code:  /belmont:verify
Cursor:       Enable the belmont verify rule, then: "Verify the completed tasks"
Other:        Load skills/belmont/verify.md as context
```

**What happens:**
- Verification agent checks acceptance criteria, visual fidelity, i18n
- Code review agent runs build, tests, reviews code quality
- Issues become follow-up tasks (plain `[ ]` entries) in PROGRESS.md
- Combined report is produced

## 8. Review Alignment (recommended periodically)

After implementing milestones or making significant changes, review the alignment between your plans and the codebase.

```
Claude Code:  /belmont:review-plans
Cursor:       Enable the belmont review-plans rule, then: "Review document alignment"
Other:        Load skills/belmont/review-plans.md as context
```

**What happens:**
- Reads all planning documents (PR/FAQ, master PRD, feature PRDs, tech plans, PROGRESS files)
- Scans codebase for implemented features and compares against plans
- Presents each discrepancy interactively with resolution options:
  - Update the planning document to match reality
  - Create a follow-up task to address the gap
  - Mark as intentional deviation
  - Skip
- Produces a summary of all findings and actions taken

## 9. Check Progress

Check where things stand at any point.

```
Claude Code:  /belmont:status
Cursor:       Enable the belmont status rule, then: "Show belmont status"
Other:        Load skills/belmont/status.md as context
```

## 10. Iterate

After implementing a milestone:
- Run `/belmont:verify` to catch issues
- Run `/belmont:debug` for targeted fixes on specific issues found by verification (routes to auto or manual mode)
- Run `/belmont:next` to quickly fix follow-up tasks from verification
- Run `/belmont:review-plans` to check alignment between plans and codebase
- Run `/belmont:implement` again for the next milestone
- Run `/belmont:status` to check progress
- Continue until all milestones are complete

### 10a. Fixing spec drift (the bug is in the docs)

If a bug exists because the PRD, TECH_PLAN, or NOTES drifted from shipped reality — acceptance criteria don't match behaviour, a TECH_PLAN decision was abandoned mid-implementation, or the docs describe an old approach — use `/belmont:debug-manual` (or `/belmont:debug` and choose "manual"). It loads the full Belmont context (master PR_FAQ + master/feature PRD + UX_DESIGN + TECH_PLAN + NOTES + latest shipped MILESTONE) up front, then after the fix is confirmed walks the specs and offers diffs to correct drift in place. `UX_DESIGN.md` is context only — design drift is surfaced to you and fixed by re-running `/belmont:ux-design`, never edited in place. Code edits and spec edits land in one atomic commit so `git log` tells the complete story. Supports multi-feature debugging (bugs that span two or more features). Interactive only; never invoked from `belmont auto`. See [skills-reference.md#debug-manual](skills-reference.md) for the full behaviour.

## 10b. Steering an auto run mid-flight

When `belmont auto` is running and you want to hand the agent a new piece
of context — a fix direction, a "don't keep retrying this" guard, a
reminder about project conventions — use `belmont steer`:

```bash
belmont steer --message "pin the ital and MONO axes too when regenerating fonts"
belmont steer --milestone M5 --file fix-notes.md
cat instructions.md | belmont steer --feature my-feature -
belmont steer   # no source → opens $EDITOR (when attached to a TTY)
```

Each call appends a pending entry to `STEERING.md` inside the relevant
worktree(s). Before the next agent phase fires, the auto loop consumes
matching entries and prepends them to the agent prompt as an URGENT
block (higher priority than NOTES.md). Consumed entries are dropped
from disk and `STEERING.md` is deleted once nothing pending remains —
the live file only exists while there's something an agent hasn't
seen yet, so skills exploring `.belmont/features/<slug>/` never
re-read steering text that's already been injected into the prompt.
The durable audit lives in the auto run's stderr stream (the
`[STEERING] injected …` line with timestamps). Entries with no
`--milestone` broadcast across every active worktree; `--milestone
M5` targets one.

Steering only works while an auto run is active (i.e. `.belmont/auto.json`
is present). Manual skill sessions are steered by typing directly into
the terminal — no sidecar needed.

## 11. Cleanup

When your project has accumulated completed features and stale state:

```
Claude Code:  /belmont:cleanup
Cursor:       Enable the belmont cleanup rule, then: "Clean up completed features"
Other:        Load skills/belmont/cleanup.md as context
```

**What happens:**
- Scans all state files and identifies completed features, archived milestones, stale notes
- Presents each item individually — you choose to archive, keep, delete, or skip
- Archives completed features into slim summaries, removes milestone archives, trims notes
- Audits CLAUDE.md and AGENTS.md for stale file paths and outdated conventions
- Checks tool directories for stale copies or broken symlinks

## 12. Start Fresh

When you're done with a feature and want to plan something new:

```
Claude Code:  /belmont:reset
Cursor:       Enable the belmont reset rule, then: "Reset belmont state"
Other:        Load skills/belmont/reset.md as context
```

**What happens:**
- Agent reads current state and shows what will be cleared (feature name, tasks, milestones)
- Asks for explicit "yes" confirmation
- Resets PRD.md and PROGRESS.md to blank templates
- Deletes TECH_PLAN.md, UX_DESIGN.md, `design-preview.html` and `ux-flows.html`
- Prompts you to start fresh with `/belmont:product-plan`
