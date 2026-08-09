# Troubleshooting

## `belmont` command not found

Ensure `~/.local/bin` is in your PATH:

```bash
echo $PATH | tr ':' '\n' | grep local
# If missing:
export PATH="$HOME/.local/bin:$PATH"
```

Or re-install:

```bash
# Via Homebrew
brew reinstall belmont

# Or via curl
curl -fsSL https://raw.githubusercontent.com/blake-simpson/belmont/main/install.sh | sh
```

## No AI tools detected during install

If your project doesn't have a `.claude/`, `.codex/`, `.cursor/`, etc. directory yet, the installer will ask which tool you're using and create the directory for you.

## Skills not showing up in Claude Code

Verify the agent symlink + per-skill slash-command symlinks:

```bash
ls -la .claude/agents/belmont
# Should show: belmont -> ../../.agents/belmont

ls -la .claude/commands/belmont/
# Should show one .md symlink per skill, each → ../../../.agents/skills/belmont/<skill>/SKILL.md

ls .agents/skills/belmont/
# Should list one folder per skill, each with SKILL.md inside
```

If you're upgrading from Belmont 0.10.x and `claude -p`'s init message shows zero `belmont:*` entries, you're likely still on the dead `.claude/skills/belmont` (or briefly `.claude/plugins/belmont`) layout — Claude Code 2.1.x never discovered either of those. Run `belmont install` to migrate; the legacy cleanup pass removes the dead paths and writes the per-skill `.claude/commands/belmont/<skill>.md` symlinks. If symlinks are missing after re-install, re-run with `belmont install --source /path/to/belmont` and select Claude Code.

## `/belmont` shows "No matching items" in opencode

opencode's TUI `/` autocomplete lists **commands**, not skills — Belmont's skills are discovered (the model can load them via the `skill` tool) but they never appear in the slash menu on their own. Belmont installs per-skill slash commands to bridge this. Verify:

```bash
ls -la .opencode/command/belmont/
# Should show one regular .md file per skill (generated wrappers, NOT symlinks)

head -3 .opencode/command/belmont/implement.md
# Should show frontmatter with description: only — no name: key
```

If the directory is missing, re-run `belmont install` and select opencode (it's auto-selected when the `opencode` binary is on PATH or a root `opencode.json`/`opencode.jsonc` exists). Commands register as `/belmont/<skill>` — opencode namespaces with `/`, not Claude Code's `:`. Restart opencode (or start a new session) after installing so the command list refreshes.

If the files are **symlinks** to SKILL.md (an early integration approach), the commands silently register under the wrong names: opencode merges frontmatter over the path-derived command name, so SKILL.md's `name:` key makes the command register as bare `implement` instead of `belmont/implement`. Re-run `belmont install` — it replaces the symlinks with generated wrapper files.

## Skills not showing up in Cursor

Cursor uses per-file symlinks with `.mdc` extension. Verify:

```bash
ls -la .cursor/rules/belmont/
# Should show .mdc symlinks pointing to .agents/skills/belmont/*.md
```

If you need to manually refresh, restart Cursor or reload the window.

## PRD is empty / template only

Run the product-plan skill first to create your PRD interactively. The tech-plan and implement skills require a populated PRD.

## Task marked as blocked

Blocked tasks show as `[!]` in `.belmont/PROGRESS.md`. Common causes:
- Figma URL not accessible
- Missing context or dependencies
- Build/test failures that can't be auto-resolved

Fix the underlying issue, change the task's checkbox from `[!]` back to `[ ]` in PROGRESS.md, and re-run implement.

## `belmont status` reports unrecognised markers, or `belmont validate` exits 1

The file has task lines Belmont cannot act on: a checkbox marker outside
`[ ] [>] [x] [v] [!] [-]`, a task line below a column-zero `## ` heading (so it
belongs to no milestone), or a task whose ID names a different milestone from
the one it is filed under. On the single-feature path `belmont auto` will refuse
to start; on `--features` / `--all` it will not, and the loop can alternate
phases without making progress.

```bash
belmont repair --feature <slug> --dry-run          # what is wrong, and the evidence for each
belmont repair --feature <slug> --mechanical-only  # apply what the commit log settles, free
belmont repair --feature <slug>                    # then have an agent read the rest
belmont validate --feature <slug>                  # confirm
```

Repair never asks you what a marker meant — it checks the commit log first, then
reads the remaining tasks against the current code, and only puts to you what
neither settles. It stops at `[x]`; `belmont reverify --feature <slug>` earns
the `[v]`. Nothing is committed, so review with `git diff` first.

If a task was deliberately dropped, the marker is `[-]` withdrawn, with the
reason in `## Decisions Log`. **Do not delete the line** — a deletion does not
survive a sibling worktree merge, so the task comes back as outstanding work.

## `belmont auto` pauses saying it cannot parse a milestone header

A milestone header must be `### M<n>: Name` at column zero, with no emoji.
Several readers accept `### ✅ M2: Name`; the parser that produces the counts
does not, so such a milestone exists for some parts of Belmont and not others.
Remove the emoji and re-run.

## Want to start fresh

Run the reset skill (`/belmont:reset` in Claude Code) to reset all state files. Alternatively, delete `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/TECH_PLAN.md`, `.belmont/MILESTONE.md`, and any `.belmont/MILESTONE-*.done.md` files manually, then re-run `belmont install` (or `belmont install --source /path/to/belmont`) to recreate templates.

## `belmont auto` refuses to start: "working tree is not clean"

`auto` requires a clean working tree because worktree merges back into the starting branch will fail if uncommitted changes overlap the merged paths. The error lists the offending paths.

Most common cause: a recent `belmont update` rewrote files under `.agents/belmont/` or `.agents/skills/belmont/` and the user never committed them. Recent versions auto-commit these files; older versions did not. To resolve:

```bash
git stash -u                       # stash everything (incl. untracked)
git commit -am "Update Belmont"    # or commit your changes
belmont auto --feature ...         # then retry

# Last resort:
belmont auto --feature ... --allow-dirty
```

`--dry-run` also bypasses the check (no merges happen).

## `belmont update` auto-commit failed (pre-commit hook)

`belmont update` runs `git commit` with hooks enabled. If a hook fails, the Belmont-managed files are left staged. Fix whatever the hook complained about (e.g. lint, formatting) and re-run the printed `git commit -m "Update Belmont to vX.Y.Z"` manually. To skip auto-commit on the next update, run `belmont update --no-commit` and commit the files yourself.

## Monorepo: `npm install` fails with `PrismaConfigEnvError: Cannot resolve environment variable: DATABASE_URL`

You're hitting this if `belmont auto` runs in a monorepo (e.g. `packages/<app>/`) and Prisma's TS config loader can't find `.env`. Belmont copies `.env*` from the project root into the worktree root and into qualifying workspace dirs (those with `postinstall` scripts or Prisma deps). If your workspace doesn't trigger the heuristic, declare it explicitly in `.belmont/worktree.json`:

```json
{
  "workspaces": {
    "web": {
      "path": "packages/web",
      "env_files": [".env", "packages/web/.env.local"]
    }
  }
}
```

See [Monorepo Support](monorepo-support.md) for the full schema.

## Monorepo: agent runs commands at the repo root instead of the workspace

If the implementation/verification agent is running `pnpm run build` instead of `pnpm --filter <id> run build`, confirm `belmont status` shows a `Monorepo:` line at the top. If not, Belmont didn't detect your monorepo (e.g. unusual layout, no signal file). Add an explicit `workspaces` map and `primary_workspace` to `.belmont/worktree.json` — the override always wins over auto-detection.

## Monorepo: warning that `.env` is not gitignored

Belmont prints `⚠ Seeded packages/web/.env but it is not gitignored` after seeding env files into a workspace dir whose path isn't covered by `.gitignore`. Belmont won't commit the seeded file (worktrees only commit their feature branch), but the warning is a heads-up for interactive workflows. Common fix:

```
**/.env
**/.env.*
!**/.env.example
```

## Monorepo: setup hooks differ from CI

`worktree.json` setup hooks run only in Belmont worktrees. CI doesn't run Belmont. If your CI is failing where local Belmont runs succeed, mirror critical setup (env wiring, install commands, `dotenv -e` shims) in your CI configuration as well.
