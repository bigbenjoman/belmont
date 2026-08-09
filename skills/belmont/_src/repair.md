---
description: Repair a PROGRESS.md whose task states no longer parse — unreadable markers, task lines outside every milestone, tasks filed under the wrong milestone. Uses commit evidence and the current code, never memory.
alwaysApply: false
---

# Belmont: Repair

You are the repair reviewer. A PROGRESS.md has entries Belmont cannot act on, and
your job is to work out what each one should say — **from the repository, not
from anyone's recollection**.

This skill runs in two situations. Read the next two sections and decide which
one you are in before doing anything else.

## Mode A — the CLI called you (proposal mode)

If the prompt you were invoked with contains a **`REPAIR BRIEF`** block:

- The mechanical tier has already run. Anything the commit log settled has
  already been applied and is not in your brief.
- **Do not edit `PROGRESS.md`, or any other file.** Investigate, then write the
  JSON proposal to the path the brief names. The CLI validates every action,
  applies what passes, and shows the user each edit before making it.
- The brief carries the full rules and the exact JSON shape. Follow them; they
  are the same ones repeated under *How to decide* below.

## Mode B — a person typed `/belmont:repair` (interactive mode)

Run the two tiers yourself, in this order. **Do not skip tier 1** — it costs
nothing and answers most of the findings without you reading a line of code.

<!-- @include feature-detection.md feature_action="ask which feature to repair, or auto-select the only one" -->

### Tier 1 — the CLI, at zero token cost

```bash
belmont repair --feature <slug> --dry-run --format json
```

That reports every finding with its line number, the marker as written, and
whether a commit names the task. Nothing is written.

Then apply what the commit log settles:

```bash
belmont repair --feature <slug> --mechanical-only
```

This marks `[x]` — and only `[x]` — where a commit names the task ID. It prints
the commit it relied on for each change.

If the CLI is not installed, do the same by hand: read
`.belmont/features/<slug>/PROGRESS.md`, find the task lines Belmont cannot read
(see *What counts as a finding*), and check each ID with
`git log --oneline --all --grep '<task-id>'`.

### Tier 2 — read the survivors against the code

For everything still outstanding, follow *How to decide* below. Then present
your conclusions to the user **as a list, with the evidence for each**, and ask
them to confirm before anything is written. Apply the confirmed ones by editing
`PROGRESS.md` directly, obeying every rule under *Hard limits*.

Finish by running `belmont validate --feature <slug>` and reporting the result.

## What counts as a finding

Only these three, and only the lines that exhibit them:

| | What it looks like | Why Belmont cannot act on it |
|---|---|---|
| Unreadable marker | `- [?] P1-M2-3: …` — anything outside `[ ] [>] [x] [v] [!] [-]` | excluded from every count, never scheduled, and its milestone can never read complete |
| Outside every milestone | a task line below a column-zero `## ` heading | counted by nothing, rendered nowhere, never scheduled |
| Filed under the wrong milestone | `P1-M3-9` sitting under `### M2:` | a dependency-graph lie: the parallel scheduler believes the wrong thing about what blocks what |

Nothing else. A task that is merely old, or wrong, or badly worded is not a
repair finding.

## How to decide

**Evidence, never memory.** Do not ask the user "did this ship?", "was this
withdrawn?", "was this blocked?" A damaged file carries dozens of these at once
and nobody remembers what a marker meant six weeks ago — you will get "I don't
know" followed by a guess, which is precisely the failure that put the file in
this state. Ask the repository instead.

For each finding, read the task text, then look for what it names in the
**current** code: the route, component, table, endpoint, flag, migration, test.

| What you find | Action |
|---|---|
| The thing exists and does what the task describes | `set_marker` `"x"` |
| The thing does not exist and nothing supersedes it | `set_marker` `" "` |
| Something replaced it, it was folded into other work, or the feature it belonged to is gone | `withdraw` (+ reason) |
| It is waiting on something you can name | `set_marker` `"!"` |
| It is not a task at all — a retro bullet, a quoted log line, a table row shaped like a checkbox | `leave` |
| The code does not settle it | `escalate` |

`escalate` is a real answer and is always better than a marker you cannot
justify. A wrong state here is the exact bug this command exists to fix.

**A commit naming the task ID proves the work happened.** That is what tier 1
already used. It cannot prove the reverse — no commit means no commit, not "not
done" — so a task with no commit still needs the code read.

## Hard limits

In Mode A the CLI enforces every one of these and refuses a proposal that breaks
them. In Mode B nothing enforces them but you, and they matter more, not less.

- **Never write `[v]`.** Repair stops at `[x]`. The verified flip has its own
  evidence contract and `belmont reverify` is the only thing allowed to write
  it. Tell the user to run that afterwards.
- **Withdrawal is `[-]` plus the reason in `## Decisions Log`.** A marker cannot
  carry why. Never express withdrawal by deleting the line: a deletion does not
  survive a sibling worktree merge, so the task comes back as outstanding work
  and the next person deletes it again.
- **Never create, rename or remove a `### M<n>:` milestone.** That is
  `/belmont:tech-plan`'s job alone. Moving a task between milestones that
  already exist is fine here — repair runs outside the auto loop, where the
  scope guard would revert it — but the destination must already exist and must
  not already hold that task ID.
- **Only touch the lines that were flagged.** Every other task in the file is
  none of this skill's business, however wrong it looks.
- **Never change a task's text**, only its marker or its milestone. If the text
  is wrong, say so in your summary and leave it.

## Report

State, in this order:

1. What the commit log settled, and the commit each conclusion rests on.
2. What you read the code for, with the evidence per finding — name the file or
   route you looked at, not just the verdict.
3. Anything escalated, and precisely what you could not determine.
4. `belmont validate --feature <slug>` output.
5. If any task is now `[x]`: `belmont reverify --feature <slug>` earns the `[v]`.

Do not report a file as repaired while findings remain escalated. Say how many
are left.
