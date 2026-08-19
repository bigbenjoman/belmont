# CLI Commands

Belmont ships a small Go CLI (`belmont`) for status checks, automated feature implementation, and self-updating. Install via the [curl one-liner](../README.md#quick-start), or on Windows use `./bin/install.ps1` for a project-local helper.

## Usage

```bash
belmont install                         # Install skills/agents into current project
belmont update                          # Update to latest release (auto-commits Belmont-managed files)
belmont update --check                  # Check for updates without installing
belmont update --no-commit              # Update without auto-committing
belmont status                          # View project progress
belmont status --format json            # Machine-readable status
belmont status --feature auth           # Feature-specific status
belmont status --color always           # Force ANSI-coloured markers (auto|always|never; auto honors NO_COLOR + TTY)
belmont status --show-archived          # Include archived features in the listing (default: collapsed to a footer count)
belmont metrics --feature auth          # What a feature cost: tokens and wall-clock per phase
belmont extract --dry-run --feature auth # Census: what splitting narrative out of PROGRESS.md would yield
belmont auto --feature auth              # Run feature auto (auto-detect tool)
belmont auto --feature auth --tool codex # Use specific tool
belmont auto --feature auth --tool pi    # Run with Pi (local LLM via ~/.belmont/local-llms.json)
belmont auto --feature auth --tool opencode  # Run with opencode (anthropic/* tiers by default)
belmont auto --feature auth --from M2 --to M4  # Milestone range
belmont auto --features auth,payments    # Run multiple features in parallel
belmont auto --all                       # Run all pending features in parallel
belmont auto --all --max-parallel 2      # Cap concurrent features (parallel within wave, merges batched post-wave)
belmont auto --all --max-parallel 1      # Strict serial: each feature merges before the next starts
belmont auto --feature auth --allow-dirty # Skip clean-working-tree preflight (not recommended)
belmont reverify --feature my-feature     # Re-verify all completed milestones
belmont reverify --feature my-feature --from M3 --to M10  # Re-verify specific range
belmont reverify --feature my-feature --tool codex  # Use specific tool
belmont repair --feature my-feature       # Repair task states that no longer parse
belmont repair --feature my-feature --dry-run          # Report findings + commit evidence, write nothing
belmont repair --feature my-feature --mechanical-only  # Apply only what the commit log settles (no agent, no tokens)
belmont repair --feature my-feature --yes # Apply reviewed proposals without prompting
belmont repair --feature my-feature --apply-proposal fixes.json  # Validate + apply a proposal file
belmont blockers                         # Every [!] task across every feature, with its detail
belmont blockers --feature my-feature    # One feature's decision queue
belmont blockers --summary               # One line per blocker, no task body
belmont blockers --format json           # Machine-readable decision queue
belmont sync                             # Sync master PROGRESS.md with feature states (explicit only, no longer auto-hooked)
belmont recover                          # List preserved worktrees from failed merges
belmont recover --list                   # Same as above
belmont recover --merge auth             # Retry merge for a preserved worktree
belmont recover --clean auth             # Delete worktree and branch
belmont recover --clean-all              # Clean all preserved worktrees
belmont steer --message "pin all axes"   # Inject instructions into an in-flight auto run
belmont steer --milestone M5 --file fix.md   # Scope to one milestone, read from file
belmont steer -                          # Read steering text from stdin
belmont steer                            # Opens $EDITOR when a TTY is attached
belmont validate                         # Lint PROGRESS.md for milestone-structure violations
belmont validate --feature about         # Scope lint to one feature
belmont validate --strict                # Also exit 1 on warnings (CI)
belmont version                         # Show version, commit, build date
# Note: "belmont loop" still works as an alias for "belmont auto"
# If a previous run was interrupted, auto detects stale branches and prompts to resume or restart
```

## The decision queue: `belmont blockers`

`[!]` usually means the work is waiting on a person — an approval, a product
ruling, a credential, a console action, a spec change that belongs to
`/belmont:tech-plan` — and that kind no skill may clear. (Two `[!]`s carry a
condition an agent *can* check: one raised against a later milestone, and one
the reconciliation agent raised over a merge. The reason line says which.)
`mergeProgressState` never ranks over `[!]` from either direction, but
protecting a signal is not the same as surfacing it.

A long run banks them up. Each `[!]` task carries a paragraph explaining what is
being asked and of whom — and that paragraph is exactly what `belmont status`
never showed. It listed every blocker's headline, in both views, interleaved
with progress counts and spread across however many features are in flight; the
person who had to answer them read them one headline at a time, in the middle of
a report about something else.

`belmont blockers` prints them together, grouped by feature and milestone, with
the indented body intact:

```bash
belmont blockers                          # every feature
belmont blockers --feature auth           # one feature
belmont blockers --summary                # one line each — the scannable view
belmont blockers --format json            # machine-readable
```

It reports and never writes — a command that both raised a question and resolved
it would be free to guess at the answer. Answering a blocker means flipping the
marker to `[ ]` yourself in the file the output names; `/belmont:next` only ever
selects a `[ ]` task, so it cannot act on one while it is still `[!]`.

During an active `belmont auto` run the paths printed are inside the run's
worktrees, and the output says so: master's copy has different line numbers, and
an edit made there is overwritten by `mergeProgressState` when the worktree
merges. Use `belmont steer`, or edit the worktree file.

Blocked task lines sitting **outside** every milestone are reported in their own
section. They are invisible to `parseMilestones` and therefore to every count in
Belmont, and a decision queue that printed "nothing is waiting on you" about a
file it could not fully read would be worse than no queue at all.

`belmont status` names it: the `--feature` detail view lists every blocker and
then points here, and the multi-feature listing prints the first three per
feature before deferring to this command with an exact count of what it withheld.

## Repairing a PROGRESS.md that no longer parses

`belmont validate` tells you a file is broken. `belmont repair` fixes it.

The three findings it acts on are the ones Belmont cannot act on itself: a
checkbox marker outside `[ ] [>] [x] [v] [!] [-]`, a task line sitting outside
every milestone (below a column-zero `## ` heading), and a task whose ID names a
different milestone from the one it is filed under.

It also runs one **audit** that is not a repair: every task marked `[v]` that the
commit log does not settle. That is wider than "no commit names it" — a commit
naming a task ID another feature also claims is not evidence about *this*
feature, because task IDs are feature-local and the log is not, so those are
reported too with the commit named. Nothing else checks any of it: the
commit-evidence guard only ever compares one phase's before and after, so a `[v]`
already on disk when a run started was never audited by anything. It is reported separately, never
acted on mechanically, and `leave` is a legitimate answer: documentation-only and
configuration-only tasks routinely leave no commit naming them. When the claim
does not hold, the move is `set_marker "x"` — `belmont reverify` then re-earns
the `[v]` under its own contract.

It works in two tiers, the same shape as `belmont reverify`:

| Tier | What it does | Cost |
|---|---|---|
| **Commit evidence** | Runs `git log` for each task ID, using the same word-boundary match the auto loop's evidence guard uses. A commit naming the ID proves the work happened, so the marker becomes `[x]`. Applied automatically, and each change names the commit it relied on. | zero tokens |
| **Review** | Dispatches an agent to read whatever survived against the current code — usually enough to separate "still outstanding" from "superseded". It proposes; you confirm each edit. | one agent run |

**It never asks you what a marker meant.** A damaged file carries dozens of
these at once and nobody remembers six weeks later, so the answer comes from the
repository. Only what survives both tiers reaches a question, and by then it is
grounded in what the code says today.

```bash
belmont repair --feature auth --dry-run          # report findings + evidence, write nothing
belmont repair --feature auth --mechanical-only  # apply only what the commit log settles
belmont repair --feature auth                    # both tiers, confirming each reviewed edit
belmont repair --feature auth --yes              # apply reviewed proposals without prompting
belmont repair --feature auth --format json      # machine-readable findings
```

What it will not do, whoever proposes it:

- **Write `[v]`.** Repair stops at `[x]`. The verified flip has its own evidence
  contract; `belmont reverify` is its only legitimate writer.
- **Delete a task line.** Dropped work is `[-]` withdrawn, with the reason
  written to `## Decisions Log`. A deletion does not survive a sibling worktree
  merge, so the task would come back as outstanding work.
- **Create, rename or remove a milestone.** That is `/belmont:tech-plan` alone.
  Moving a task between milestones that already exist is allowed here, because
  repair runs outside the auto loop where the scope guard would revert it.
- **Split a task from its body.** A move carries the whole block — the bullet
  plus the indented `**Verification**` / `**Evidence**` lines beneath it. Moving
  the bullet alone would credit the wrong task with the evidence and leave the
  moved one asserting done with nothing behind it, with the file still parsing
  and `belmont validate` still clean. A bullet nested inside another task being
  moved is refused rather than moved twice.
- **Touch a line it did not flag**, or a line that changed since it was scanned.
- **Run against an ambiguous file.** A repeated `### M<n>:` heading makes every
  milestone-keyed lookup arbitrary; repair refuses, as both runtime guards do.

Nothing is committed — review with `git diff` and commit yourself.

`--apply-proposal FILE` validates and applies a proposal written by something
else — the CLI-dispatched agent writes one, and `/belmont:repair` in a REPL
writes one too rather than editing `PROGRESS.md` itself. That keeps the Go
writer the only writer, so the bounds above are enforced rather than merely
remembered. The JSON shape is `{"repairs": [{"line": N, "task_id": "...",
"action": "...", "reason": "..."}]}`, with `line` as reported by `--dry-run`.

## What a run cost: `belmont metrics`

```bash
belmont metrics --feature SLUG [--root PATH] [--format text|json]
```

Summarises `.belmont/metrics/<feature>.jsonl` — one append-only record per phase, written by `belmont auto`. Records are **local-only and gitignored**; `auto` guarantees the ignore rule at startup (see §Clean-working-tree preflight, below).

The governing rule is that **a figure is either reported by the tool that spent it, or it is null.** Nothing here estimates a token count — not from character counts, not from a ratio, not from a previous run. A host that cannot report usage records `null` with a stated reason and wall-clock only. That is **five** of the seven: `toolReportsUsage` (`metrics.go`) returns true for `claude` and `codex` only, so copilot, pi, opencode, **gemini and cursor** all record null. *(Corrected 2026-08-19 by `P0-M1-FIX-28(c)`, which said three: gemini and cursor were listed as reporting in the feature's own plan table and record null in the shipped code, because their usage schema was never verified against a live run and Belmont does not ship a parser written against a guess.)* A plausible invention would silently contaminate the baseline every later comparison is judged against.

Two distinctions the summary keeps rather than flattening:

- **`input_semantics` is per tool and is not aggregated across tools.** Claude's `input_tokens` *excludes* `cache_read_input_tokens`; codex's (OpenAI lineage) *includes* `cached_input_tokens`. Summing them yields a plausible wrong number, so a figure spanning two tools is reported as `null` with the reason plus a per-tool breakdown.
- **Zero-usage phases are flagged, never dropped.** A phase that failed before the tool produced usage (a session-limit exit at 3 s) still appended a record. It counts as a phase and is surfaced as `phases_with_zero_usage`, because silently excluding failures is how a cheap failure flatters a median. `ZeroUsage` ("the host said zero") and `Unreported` ("the host said nothing") are disjoint and answer different questions.

## Register size census: `belmont extract`

```bash
belmont extract --dry-run [--feature SLUG] [--root PATH] [--roots PATH,PATH] [--allow-unreadable-roots] [--format text|json]
```

Measures what moving indented narrative detail out of `PROGRESS.md` would yield, per feature. **`--dry-run` is required, not optional** — this release ships the census only; nothing is written.

- **`--roots` paths must be absolute.** The shell expands `~` only at the start of a word, so every path after the first comma stays literal and resolves under the current directory. The documented command silently returned 43 registers instead of 65 before this was caught.
- **A root or register that cannot be read is a reported failure, never a skip.** The run refuses and names everything it could not read, rather than quietly reporting a smaller denominator — an incomplete census reads exactly like a complete one once its numbers are quoted elsewhere. `--allow-unreadable-roots` overrides; the report then states what it missed, above the figures and again below them. `coverage_complete` in the JSON is false whenever either `unreadable_roots` or `unreadable_registers` is non-empty.
- Feature directories with **no** register are counted separately (`dirs_without_register`) from registers that **exist but could not be read** (`unreadable_registers`). The first is a fact about the estate; the second is a hole in the measurement.

## Milestone-structure validation

`belmont validate` lints `PROGRESS.md` for milestone-structure violations — the class of bug documented in [`knowledge/cross-cutting/milestone-immutability.md`](../knowledge/cross-cutting/milestone-immutability.md).

**Errors** (exit code `1`, and `belmont auto` will not start):

- **Polish / follow-up milestone names.** Milestones whose name matches `polish`, `follow-ups`, `cleanup`, `verification fixes`, `deviations from M<N>`, `from M<N> implementation`, `fwlup(s)`. These violate the rule that follow-ups stay in the milestone that discovered them.
- **Cross-milestone task IDs.** Task IDs like `P3-FWLUP-M2-1` that embed a milestone number should live under that milestone; when they're found under a different one, the milestone structure is lying about ownership and parallel merges will collide.
- **Unrecognised task markers.** Anything outside `[ ] [>] [x] [v] [!] [-]` (letters are case-insensitive, so `[X]` and `[V]` are fine). Belmont will not guess a state, so the milestone can never read as complete. Work that was deliberately dropped is `[-]` withdrawn, with the reason recorded in `## Decisions Log`.
- **Duplicate milestone IDs.** Two `### M<n>:` headings sharing a number — including a session note written in that shape. Milestones are keyed by ID throughout, so a collision makes the scope guard and the evidence guard switch off.

**Warnings** (exit code `0`, auto continues):

- **Task lines outside any milestone.** A `## ` heading at column zero ends the milestones region, so a checkbox line below it is counted by nothing and never scheduled. Reported with its line number because losing it silently is the bug this exists to prevent — but a `- [ ]` bullet in a retro is not work, so it does not stop a run.

  The advice on each one is **conditional**, because "move it under its `### M<n>:` heading" only helps when there is such a heading. If the task's ID names a milestone the file already has, the warning says so and points at `belmont repair`, which moves between existing milestones. If it names none — the usual shape of a follow-up produced by a cross-cutting audit — the warning gives the destination rule instead: file it under the **highest-numbered existing milestone whose work it touches**, or the last milestone in the plan if it is genuinely global. Never a new milestone. Escalating that case to `/belmont:tech-plan` used to be a dead end, since that skill forbids a new milestone for follow-ups and the task came back unscheduled.

- **A milestone whose live worktree state could not be read.** During a parallel run each milestone's state is overlaid from the worktree that owns it. When that worktree's `PROGRESS.md` is missing or unreadable — a half-cleaned worktree, a failed merge that left the directory behind, a `.belmont/` copy interrupted mid-write — the view falls back to master's fork-point copy. That fallback used to be silent, which meant `belmont validate` printed a clean bill of health for a file it had only partly seen, and `belmont auto` started on it. It is now stated, by `validate`, `status` and `blockers` alike, and the path that could not be read is printed so you can look at it directly. The same warning covers the serial / multi-feature shape, where the whole feature is read from one worktree.

  It deliberately does **not** tell you to run `belmont recover`: this condition only arises while a run is active, and `belmont recover --list` scans the directory the live wave worktrees are in with no active-run filter, so mid-run it lists them as preserved and offers to clean them.

```bash
belmont validate                            # Scan every feature
belmont validate --feature about            # One feature
belmont validate --strict                   # Exit 1 on warnings too (for CI)
belmont validate --format json              # Machine-readable output; each entry carries "severity"
```

Most violations also carry a **remedy**, shown in the text output and in the JSON:

- `needs_evidence` — the correct answer is a fact about the repository, not a question for someone's memory. Whether a commit carries the task ID, whether the code it describes exists, whether a test covers it. A project can accumulate dozens of these, and "was this withdrawn or did it ship?" is not something anyone recalls months later; guessing is what issue #27 was.
- `tech_plan` — the fix changes milestone structure, which is immutable outside `/belmont:tech-plan` and enforced at runtime. An agent that edits it anyway has the change reverted by the scope guard.

`unreadable_live_milestone` carries neither: both remedies answer "what should this line say", and that finding is not about a line in the file at all. Its fix is to the worktree, and the command is in the message.

The report ends with the expected structure — header shape, task-line shape, the marker set, and where a `## ` heading ends the milestones region — so the output is enough on its own to conform a file.

`belmont auto` runs this lint at startup on the single-feature path: interactive runs get a `[y/N]` override prompt on errors, non-interactive runs abort, and warnings print without stopping anything. Restructure via `/belmont:tech-plan` before rerunning.

Duplicate milestone IDs are the exception — they are refused before the loop starts on **every** path, including `--features` / `--all`, and the refusal cannot be overridden. Proceeding would run the whole feature with both runtime guards absent.

## `--max-parallel` semantics

`belmont auto`'s `--max-parallel` flag controls how many units (features in multi-feature mode, milestones in single-feature parallel-milestones mode) run concurrently within a wave.

- `--max-parallel=1` (strict serial): each unit runs to completion and **merges to main inline** before the next unit's worktree is created. Subsequent units fork from a main that already includes prior merges, so implicit cross-unit task deps — e.g. one feature's home screen calling another feature's new route — resolve at the fork point rather than producing `[!]` blockers. Each unit still runs in its own worktree (no master-tree shortcut).
- `--max-parallel >= 2` (default 5): wave units run in parallel up to the cap; merges are collected and applied **post-wave** in dependency / milestone-ID order with overlap reporting. Use when units in the same wave are truly independent.

If a paused worktree is resumed in a later invocation, Belmont automatically `git rebase`s it onto current main so any sibling merges since the pause are picked up. Conflicts abort the rebase and warn — they are never auto-resolved. See [`knowledge/auto-mode/resume-rebase.md`](../knowledge/auto-mode/resume-rebase.md).

## Clean-working-tree preflight

`belmont auto` refuses to start when the working tree has uncommitted, unstaged, or untracked changes. Worktree merges write back to the same branch the user started auto on, and dirty files in that branch will block the merge — leaving the user with a preserved worktree and a half-finished run to recover. The preflight catches this before any worktree is created.

The most common trigger is `belmont update` having rewritten files under `.agents/belmont/` or `.agents/skills/belmont/` without a follow-up commit. When the dirty paths overlap a Belmont-managed subtree, the error message names that situation explicitly. `belmont update` now auto-commits these files by default (use `--no-commit` to opt out) so this scenario should be rare.

Resolve via `git stash -u`, `git commit -am "..."`, or, as a last resort, bypass with `belmont auto --allow-dirty`. The `--dry-run` mode skips the check (no merges happen).

Immediately **after** the check passes, `auto` guarantees its own artefacts cannot trip it next time: if `.gitignore` does not already ignore `.belmont/metrics/`, the rule is appended and committed on its own (pathspec-scoped, so nothing else is swept in). Without it, a project installed by a Belmont older than the metrics feature has no rule — the first instrumented run leaves `?? .belmont/metrics/` and the next run is refused. This happens at most once per project; a repo that already has the rule sees no edit and no commit, and `--dry-run` writes nothing at all.

## Update auto-commit

`belmont update` follows a successful self-update + skill reinstall by staging and committing only Belmont-managed paths:

- `.agents/belmont/`, `.agents/skills/belmont/`
- `.claude/agents/belmont/`, `.claude/commands/belmont/`, `.opencode/command/belmont/`
- Legacy (no longer created, but staged for deletion if leftover from older Belmont versions): `.claude/skills/belmont/`, `.claude/plugins/belmont/`, `.codex/belmont/`, `.cursor/rules/belmont/`, `.windsurf/rules/belmont/`, `.gemini/rules/belmont/`, `.copilot/belmont/`
- `AGENTS.md` (Codex + Copilot routing section), `GEMINI.md` (Gemini `@import` section)

Unrelated user changes (staged or unstaged) are not swept up — the `git commit` carries an explicit pathspec. Repo pre-commit hooks run normally; if a hook fails, Belmont leaves the files staged and prints the manual `git commit` to retry.

`belmont update --no-commit` skips the commit and prints the equivalent manual command. The commit step is also skipped silently when the cwd isn't a git repo.

## Steering a running auto run

`belmont steer` is the way to hand new instructions to an `auto` run that's
already in progress — headless agent invocations never see stdin, so typing
into the terminal does nothing. The command appends a pending entry to
`STEERING.md` inside each active worktree (or the master feature directory
for non-parallel runs). Before the next agent phase fires, the auto loop
reads any matching entries and prepends them to the agent's prompt as a
high-priority block (higher than `NOTES.md`).

Lifecycle:

- Consumed entries are **dropped from disk** — they don't accumulate inside
  `STEERING.md`. When the last pending entry is consumed the file is
  deleted, so agents that explore `.belmont/features/<slug>/` never
  re-read steering text that's already been injected into the prompt.
- The durable audit trail lives in the auto run's stderr stream — look for
  `[feature][milestone]: [STEERING] injected N instruction(s) — "…"`
  lines with their timestamps.

Rules:

- Only works while `belmont auto` has an active `.belmont/auto.json`.
  Manual CLI sessions are steered by typing directly into the running
  terminal.
- With no `--milestone`, writes to every active worktree for the feature.
- With `--milestone M5`, writes only to that milestone's worktree.
- Exactly one input source is required: `--message "text"`, `--file PATH`,
  `-` (stdin), or no source with `$EDITOR` set and a TTY attached.
- `copyBelmontStateToWorktree` preserves `STEERING.md` across the
  resume-time state refresh, so steering you drop before resuming a
  preserved worktree survives.

## Worktree Environment Variables

When `belmont auto` runs features or milestones in parallel worktrees, the following environment variables are automatically set for each worktree:

| Variable | Description |
|----------|-------------|
| `PORT` | Unique free port assigned to this worktree |
| `BELMONT_PORT` | Same value as `PORT` |
| `BELMONT_WORKTREE` | Set to `1` in worktree context |
| `BELMONT_MONOREPO` | Set to `1` when a monorepo is detected (otherwise unset) |
| `BELMONT_MONOREPO_TYPE` | Detected type (`turborepo`, `nx`, `pnpm`, `npm`, `yarn`, `bun`, `cargo`, `go`, `uv`, etc.) |
| `BELMONT_PRIMARY_WORKSPACE` | Workspace ID hosting the primary dev server |
| `BELMONT_PRIMARY_WORKSPACE_PATH` | Workspace path relative to the worktree root |
| `BELMONT_WORKSPACES` | JSON array `[{"id","path"}, ...]` of all workspaces |

Dependencies are auto-installed by detecting your lock file (e.g., `package-lock.json` → `npm install`). Configure custom worktree lifecycle hooks via `.belmont/worktree.json`. See [Worktree Isolation](worktree-isolation.md) for full documentation, and [Monorepo Support](monorepo-support.md) for monorepo-specific behavior (including how to override auto-detected workspaces).

`belmont status` reports a `Monorepo: <type> (<N> workspaces, primary=<id>)` line when auto-detection fires; `belmont status --format json` includes a `monorepo` object alongside the existing fields.

## Local-LLM configuration (Pi)

When `--tool pi` is used, Belmont resolves Pi's `--provider <p> --model <m>` flags from a chain that supports both global and per-project setups. From highest priority to lowest:

| Source | Notes |
|--------|-------|
| `BELMONT_PI_PROVIDER_<TIER>` / `BELMONT_PI_MODEL_<TIER>` env vars | Per-shot overrides (e.g. `BELMONT_PI_MODEL_HIGH=deepseek-coder-v3`). Tier is uppercased. |
| `BELMONT_PI_PROVIDER` / `BELMONT_PI_MODEL` env vars | Single value applied to every tier. |
| `<project>/.belmont/local-llms.json` | Per-project mapping; per-tier-per-field. |
| `~/.belmont/local-llms.json` | User-level mapping. |
| _(none)_ | Belmont passes no flags; Pi uses the default model from `~/.pi/agent/models.json`. |

Each layer is consulted independently for `provider` and `model`, so you can override one field without touching the other. See [Supported Tools → Pi](supported-tools.md#pi-local-llm-workflow) for the full schema and an LM Studio + Ollama example, and [docs/local-llms.example.json](local-llms.example.json) for a copy-paste starter.

## Model overrides (opencode)

When `--tool opencode` is used, Belmont resolves the `--model <provider/model>` flag from a similar chain. Unlike Pi, opencode has built-in defaults (the Anthropic provider), so the chain ends in a working mapping instead of "no flags". From highest priority to lowest:

| Source | Notes |
|--------|-------|
| `BELMONT_OPENCODE_MODEL_<TIER>` env var | Per-shot override (e.g. `BELMONT_OPENCODE_MODEL_HIGH=opencode/gpt-5.1-codex`). Tier is uppercased. |
| `BELMONT_OPENCODE_MODEL` env var | Single value applied to every tier. |
| `<project>/.belmont/local-llms.json` `opencode.tiers.<tier>` | Per-project mapping. `model` takes a full `provider/model` ID, or split across `provider` + `model` and Belmont joins them with `/`. |
| `~/.belmont/local-llms.json` `opencode.tiers.<tier>` | User-level mapping. |
| Built-in `modelTiers` defaults | `anthropic/claude-haiku-4-5` / `anthropic/claude-sonnet-4-6` / `anthropic/claude-opus-4-8`. |
| _(no tier)_ | When the feature has no `models.yaml`, Belmont passes no `--model` flag and opencode uses the default model from its own config. |

## How Skills Use the CLI

Skills prefer these helpers when available:
- `status` uses `belmont status` first
- `implement`, `next`, `verify`, and `reset` may use `belmont status --format json` for summaries (still read `.belmont` files for full context)

## Windows

Build example (project-local helper):

```powershell
go build -o .belmont\\bin\\belmont.exe ./cmd/belmont
```

Helper install script:

```powershell
pwsh ./bin/install.ps1
```
