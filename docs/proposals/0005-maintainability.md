# PR 3 — Maintainability

**Revision 2.** Rewritten after adversarial review. **Both v1 proof mechanisms were impossible** and have been replaced. Changes in §12.

**Type:** mechanical refactor (zero behaviour change) + lint + CI.
**Size:** very large diff, near-zero semantic content. Reviewed per-commit, one extracted file at a time.
**Sequencing:** no PR dependency (see §1). Consumes PR 2's eval entry point in CI — rebase onto PR 2 if it has landed, otherwise drop that CI line.

---

## 1. What changed and why (read first)

v1 rested on two proofs, **both verified impossible**:

**`cmp` of `-trimpath` binaries can never show IDENTICAL.** Reproduced twice with controls (same-source rebuild *is* byte-identical; revert-to-pristine *is* byte-identical). A verbatim move of `main.go:299-339` into a new `state.go`, gofmt-clean and tests green, produces **4,871,998 of 12,097,522 bytes differing (~40%)** — same size, different build IDs. It survives `-ldflags="-w -s"` (1.82 MB still differ): this is function layout plus pclntab/DWARF base filenames, and `-trimpath` strips directories, not basenames. `-buildvcs=auto` adds a second independent failure. v1 wired this check into **seven** places including the inference rule "non-identical = not a pure move" and the instruction "find the edit before going further" — which would have sent the implementer hunting an edit that does not exist.

**`git diff -M` will not render the split as renames.** Simulated at full scale (main.go 1,499 lines + 8 domain files): `10 files changed, ~11,334 insertions, ~11,319 deletions`, `--summary` shows only `create mode`, zero renames. `-C -C`, `-M1%`, `-M5%` are identical. `main.go` survives, so it can never be a rename *source*, and each new file is 9–12% of it. Forcing `--find-copies=1%` produces copy markers but inflates the patch from 22,288 to 118,226 lines.

Also corrected: **there is no PR dependency.** `gh pr list` shows #4–#7 closed unmerged 2026-04-21 (superseded by merged #8), #18 merged 2026-06-04, and the only open PRs — #10–#13 — touch **no `.go` file**. v1's "land after the open Go PRs" and its Risks row are deleted.

## 2. Problem

`cmd/belmont/main.go` is **12,613 lines** — 78% of the Go in the repo. Every agent that touches the CLI drags it through context; every reviewer scrolls it. For work *on Belmont*, this is the bottleneck.

Separately, `AGENTS.md:81` states there is no linter configured, so the code-review-agent rediscovers mechanically-detectable issues on every run.

## 3. Rationale from the graph-engineering method

**Bottleneck-at-a-time (§9).** Boris Cherny's account of Anthropic's 8× code output per engineer is "systematically improve a bottleneck at a time" — coding, then code review, then GTM. Here the bottleneck is not agent capability but that the unit of context is a 12.6k-line file.

**Checking is its own job (§3).** A linter is the cheapest possible skeptic: mechanical, unarguable, never rubber-stamps. Every finding moved from code-review-agent to `go vet` is caught deterministically instead of probabilistically.

**Rare gates stay read (§8).** Boris on approval fatigue — a human deciding yes/no every time eventually "stops reading"; he admits doing it with bash commands. An agent surfacing style findings beside real defects trains the reader to skim.

**The tool comes after the workflow (Isenberg, §7).** Split first, then enforce the new boundaries. Linting a 12.6k-line file and then moving everything is churn.

## 4. Design

### 4.1 The new proof mechanism — normalized declaration-set diff

Replaces `cmp`. Concatenate the package's non-test `.go` files, strip `package` / `import` / blank lines, sort top-level declarations, diff before against after. **Must be empty.**

```bash
decls() {  # $1 = git ref or worktree path
  cat "$1"/cmd/belmont/*.go 2>/dev/null | grep -v '_test\.go' \
    | sed '/^package /d; /^import /d; /^\s*$/d' | sort
}
git worktree add /tmp/pr3-base main
diff <(decls /tmp/pr3-base) <(decls .) && echo "PURE MOVE"
```

Verified to prove a pure move **and** to catch an injected `!=`→`==` change. Do **not** substitute `go tool nm` symbol name+size sets — tested, and it misses that injected bug.

Use `git worktree add`, not v1's `git stash` recipe: on a clean tree the stash is a no-op and both builds come from the split tree, printing a **false pass** (reproduced); on a dirty tree `git stash` does not stash untracked files, so the "before" build fails with `redeclared in this block`, `git stash pop` never runs, and the split is stranded in `stash@{0}`. Add `-buildvcs=false` to any comparison build.

### 4.2 The new reviewability mechanism — one commit per extracted file

Replaces the rename claim. Each commit moves exactly one domain out of `main.go`, so `git show --stat` reads `main.go -N / newfile.go +N` — a legible pair. The decl-set check runs at **every** commit, not just at the end. Suggest `git diff --color-moved=zebra` for reviewing an individual commit.

This makes the review a sequence of ten small, verifiable steps rather than one 22k-line diff.

### 4.3 Boundaries

Files move; the package does not. **The stdlib-only rule in `AGENTS.md` constrains dependencies, not file count** — state this in the first paragraph of the PR description, as it is the first objection a careful reviewer raises.

| File | Contents |
|---|---|
| `main.go` | entry point, command dispatch, flag parsing |
| `state.go` | `parseMilestones`, `flattenTasks`, `milestoneAll*`, task-state enum, `parseMasterDeps` |
| `install.go` | installer, updater, `setupTool`, `runLegacyCleanup`, `syncSkillsFolderDir` |
| `auto_loop.go` | `executeLoopAction`, `executeTriageAction`, `buildLoopPrompt`, checkpoint policy |
| `auto_decide.go` | `decideLoopAction`, `decideLoopActionSmart`, `decideLoopActionAI`, `checkHardGuardrails` |
| `auto_parallel.go` | `runAutoParallel`, `runWaveParallel`, `runMilestoneInWorktree`, merge sequencing |
| `guards.go` | `runScopeGuard`, `runEvidenceCheck`, `diffScopeViolations`, `rebuildAfterScopeGuard` |
| `worktree.go` | worktree lifecycle, `buildWorktreeEnv`, `rebaseWorktreeOnMain`, `copyBelmontStateToWorktree` |
| `steer.go` | `belmont steer`, `consumePendingSteering`, STEERING.md lifecycle |
| `toolexec.go` | `toolHeadlessArgs`, `buildToolCommand`, `adaptPromptForTool`, `resolveModelFlags`, `modelTiers` |
| `status.go` | `status`, `sync`, `validate`, JSON output |

Two v1 errors corrected. **`decideNextAction` does not exist** — the real functions are `decideLoopAction` (`:6563`), `decideLoopActionSmart` (`:7465`), `decideLoopActionAI` (`:7724`) and `checkHardGuardrails` (`:7707`), ~498 lines containing the file's 3rd- and 7th-largest functions; they get their own `auto_decide.go` rather than a third of `main.go`'s 1,500-line budget. **`executeTriageAction` (`:6996`, ~146 lines)** was unassigned in v1 and now sits with `executeLoopAction`. **`cmd/belmont/tools.go` already exists**, hence `toolexec.go`.

Treat the table as a proposal. The reviewer's opinion on boundaries *is* the substance of this PR; re-cut on request.

### 4.4 Lint — enumerate, don't estimate

`go vet ./...` is **already clean**; that half is CI wiring only. `staticcheck ./...` returns **exactly 9**:

| Finding | Symbol |
|---|---|
| U1000 | `parseMasterFeatureStatuses:2227` |
| U1000 | `upsertMarkedSection:4683` |
| U1000 | `containsTool:4782` |
| U1000 | `isCriticalConfig:7383` |
| U1000 | `hasPendingTasks:8264` |
| U1000 | `(*worktreeTracker).cleanupAll:8799` |
| U1000 | `prepareWorktreesGitignore:9914` |
| U1000 | `excludeBelmontInWorktree:9940` |
| SA4006 | `:10839` |

Nine is not "a large volume", so v1's suppress-and-ticket risk row does not apply and would have routed straight to unconditional deletion — while "resist adjacent cleanups" discouraged the very check that justifies deleting. **Require a one-line `git log -S` justification per deletion.** Three are already resolved as dead-by-design: b85f448 swapped both `cleanupAll` call sites to `gracefulShutdown`; cd0b82f removed both `excludeBelmontInWorktree` sites; `prepareWorktreesGitignore` is superseded by `ensureStateFiles` (`:4611-4612`).

### 4.5 CI — this PR owns it

No test CI exists; `.github/workflows/` holds only tag-triggered `release.yml`. PR 2 assumes a CI surface it does not create. Add `.github/workflows/ci.yml` on push and PR:

`go build ./cmd/belmont` · `go test ./cmd/belmont` · `go vet ./...` · `GOOS=windows go vet ./...` · `staticcheck ./...` · `generate-skills.sh --check` · `generate-plugin.sh --check` · PR 2's Tier-1 evals (`go test -tags eval ./cmd/belmont`) if PR 2 has landed.

### 4.6 Cross-platform coverage

`go build`, `go test` and `scripts/build.sh` (`:18-19` defaults to `go env`) all cover host GOOS/GOARCH only; `release.sh` verifies host-only *before* `git commit`/`git tag`. Demonstrated: deleting the `//go:build !windows` line from `process_unix.go` leaves the entire v1 DoD green while `GOOS=windows go build` fails with four redeclaration errors — and that file is a natural casualty of a "tidy the platform files" re-cut, since `_unix` is **not** an implicit-constraint suffix.

Add the 5-platform build matrix (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64) to the DoD. The embed axis is already covered (`go build` = `!embed`, `build.sh` = `-tags embed`) — exclude it explicitly and say why.

## 5. Scope

### In scope

| Commit | Change |
|---|---|
| 1–10 | One extracted file each, decl-set check per commit |
| 11 | `go vet` + staticcheck + `.github/workflows/ci.yml` + doc corrections |

**Skill sources hard-code `main.go` (v1 claimed no skill impact).** Three committed sources reference it: `_partials/tier-registry.md:15` and `_src/references/models-yaml-format.md:38` (`modelTiers` at `main.go:42` → `toolexec.go`), and `_partials/debug-scope-rules.md:7` (`actionDebug`, `main.go:364/405`). These are inlined into the **tracked** plugin tree — `plugin/skills/{implement,verify}/SKILL.md:201`, `plugin/skills/debug-manual/SKILL.md:123`, `plugin/skills/tech-plan/references/models-yaml-format.md:38`. Hand-editing either location evaporates: `generate-plugin.sh:35` does `rm -rf "$PLUGIN_DIR"` and `skills/belmont/*/` is gitignored. Edit the three `_src`/`_partials` sources, regenerate both, commit `plugin/`. **Prefer symbol-only references** so they cannot go stale again.

### Out of scope

Behaviour changes · package restructuring · new dependencies · reformatting or renaming · any "while I'm here" cleanup.

## 6. Both invocation paths

The CLI backs both, so a regression breaks everything — and §5 shows skill sources *are* touched, contradicting v1's "no interactive-mode behaviour to change." Both paths plus the plugin surface are exercised in §8.

## 7. Docs and knowledge

**Docs.** `AGENTS.md` Architecture (main.go no longer single-file) and Verify (lint + CI); `AGENTS.md:81` — the "no linter" sentence appears **once**, not twice as v1 claimed (`CLAUDE.md` is a symlink to `AGENTS.md`), and the same sentence's "A small focused test file exists at `cmd/belmont/commit_update_test.go`" is stale (8 test files, 2,989 lines). Also `CONTRIBUTING.md:81` and `:138` ("All CLI logic is in `cmd/belmont/main.go`" — already imprecise; `local_llms.go` is 385 lines), `AGENTS.md:124`, and `docs/directory-structure.md`.

**Knowledge.** v1 hand-listed 8 entries; `grep -rln 'main\.go' knowledge/` returns **15**. Use the grep output, not a hand list. Missing from v1: `auto-mode/clean-tree-preflight.md:13`, `auto-mode/multi-feature-scheduling.md:16`, `cross-cutting/milestone-immutability.md:25`, `cross-cutting/skill-format.md:16,25`, `cross-cutting/codex-plan-handoff.md:36`, `cross-cutting/debug-spec-reconciliation.md:11,44`, `cross-cutting/dual-invocation-paths.md:42`. One `Revisions` line each; confirm every routing trigger still matches.

## 8. Author smoke test

**Preconditions.** `requireCleanWorkingTree` (`main.go:9818`) aborts `auto` on any dirty/untracked path and `install` never commits — commit after every install.

**Step 0 — record the true starting point.** v1's differential was vacuous: Step 2 branched from post-Step-0 HEAD where M1 was already `[x]`/`[v]` and committed (`runLoop` never switches branches, `:5449-5453`), so `decideLoopActionSmart`'s first-iteration branch (`:7466-7485`) returns `actionComplete` **before any shell-out** — exercising none of the relocated code. Note `milestoneAllDone` (`:299`) accepts `[x]`, so this fires even without verification.

```bash
cd ~/path/to/real-project && git checkout -b smoke/pr3
belmont install --source ~/belmont --no-prompt && git add -A && git commit -m "install"
BASE=$(git rev-parse HEAD)      # M1 still pending here — every run forks from this
```

**Step 1 — purity, at every commit.** Run the §4.1 decl-set diff. Expect `PURE MOVE`, empty diff. Paste the output in the PR description.

**Step 2 — auto end-to-end, at commit 11.** From `$BASE`, run `belmont auto --feature <slug> --from M1 --to M1` on pre- and post-split binaries. Assert **behaviourally** — phases executed, guard fired, worktrees and ports created — not by JSON byte-equality.

**Step 3 — scope guard, with the timing window respected.** `snapshotProgress` runs at the *top* of `executeLoopAction` (`:6867`), so a heading added between phases is baked into the baseline and `diffScopeViolations` finds nothing by design. Wait for the phase line to appear, then append `### M99: Injected` plus one task, then assert three things: stderr `[SCOPE-GUARD] reverted 1 violation(s)`; `grep -c 'M99' PROGRESS.md` = 0; a `(pending)` STEERING.md entry naming M99.
**Absence of the `[SCOPE-GUARD]` line means the edit landed outside the window — retry, do not record a pass.**
This manual step is the **only** coverage of the read → revert → write → amend → STEERING wiring; no test calls `runScopeGuard`. Keep it; do not substitute unit tests.

**Step 4 — interactive.** `git checkout -b smoke/pr3-interactive $BASE`, then `claude` → `/belmont:implement` → `/belmont:verify`.

**Step 5 — parallel + worktrees.** From `$BASE`: `belmont auto --feature <slug> --max-parallel 2`. Expect distinct `BELMONT_PORT` per worktree, sequenced merges, cleanup on exit. Exercises `auto_parallel.go` + `worktree.go`.

**Step 6 — install path.** `belmont install --source ~/belmont --project /tmp/pr3-install --no-prompt`; confirm `.agents/skills/belmont/`, per-skill slash commands, and `loop` as a real file not a symlink.

**Step 7 — other tools.** 7a: `belmont auto --tool codex` from `$BASE`. 7b: interactive `codex` → `$belmont` popup → `$implement`. (`--tool` is a CLI flag — `:5287`, `:10755`, `:11081` — with no REPL meaning, and Codex has no `/belmont:` command; v1's "repeat Step 4 with `--tool codex`" was not executable. Steps 2 and 4 already are the Claude-both-modes check, so v1's extra clause was redundant.) Note Step 2 omits `--tool`, so `detectTool()` (`:6316`) picks claude.

**Step 8 — cross-platform.** `GOOS=windows go build ./cmd/belmont` plus the other four targets, and `GOOS=windows go vet ./...` (passes today).

## 9. Definition of Done

**Split**
- [ ] `main.go` under ~1,500 lines; no new file over ~2,000
- [ ] Package unchanged — files moved, no new packages
- [ ] One commit per extracted file; `git show --stat` shows a legible `main.go -N / newfile.go +N` pair
- [ ] Decl-set diff empty **at every commit**; output pasted in the PR description
- [ ] Zero logic edits — no renames, no signature changes, no reformatting
- [ ] `decideLoopAction*` + `checkHardGuardrails` in `auto_decide.go`; `executeTriageAction` assigned
- [ ] No collision with the existing `cmd/belmont/tools.go`
- [ ] Boundaries align with knowledge-tree domains

**Skills / plugin**
- [ ] Three `_src`/`_partials` sources updated to symbol-only references
- [ ] `generate-skills.sh` and `generate-plugin.sh` run; `plugin/` committed
- [ ] Both `--check` scripts pass (`plugin.json` STALE is the only legitimate diff)

**Lint / CI**
- [ ] All 9 staticcheck findings resolved, each deletion carrying a one-line `git log -S` justification
- [ ] Suppression count stated in the PR description — no silent scope truncation
- [ ] `.github/workflows/ci.yml` added with the §4.5 job list
- [ ] `AGENTS.md:81` corrected — both the linter claim and the stale `commit_update_test.go` sentence

**Verification**
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green at every commit
- [ ] 5-platform build matrix green; `GOOS=windows go vet ./...` green; embed axis exclusion stated
- [ ] `./scripts/build.sh <version>` produces a working embedded binary
- [ ] Scope guard verified live within the timing window (Step 3)
- [ ] Auto, interactive and plugin surfaces exercised
- [ ] All 15 knowledge entries updated from `grep -rln 'main\.go' knowledge/` + `Revisions` lines
- [ ] `AGENTS.md`, `CONTRIBUTING.md:81,138`, `docs/directory-structure.md` updated
- [ ] PR description opens with: pure move, stdlib-only unaffected, decl-set proof, no PR dependency

## 10. Risks

| Risk | Mitigation |
|---|---|
| A behaviour change hides in 12.6k moved lines | Decl-set diff at every commit — proven to catch an injected `!=`→`==` |
| Reviewer rejects on the stdlib-only rule | Pre-empt in paragraph one: it constrains dependencies, not file count |
| A platform-file re-cut breaks Windows silently | 5-platform matrix + `GOOS=windows go vet` in DoD and CI |
| Knowledge routing goes stale | Driven from grep output, not a hand list |
| Deleting live code as "unused" | `git log -S` justification per deletion |
| Archaeology after the split | `git blame -C -C` resolves through the move with no config. `.git-blame-ignore-revs` does **not** work here — tested; it needs the line to persist in the same file. Document `-C -C` as the path. |
| Rollback | `git revert` conflicts once any later commit touches a new file. Rollback is **re-concatenation**, not revert. Commits 1–10 and 11 are separate, so doc corrections are not entangled. |

## 11. Interaction with PR 1 and PR 2

- **PR 1** — no overlap.
- **PR 2** — v1 conflicted because PR 2 edited `executeLoopAction`, which this PR relocates. PR 2 v2 dropped Optimisation B, so it touches no non-test Go and the conflict is gone. This PR's CI consumes PR 2's eval entry point; if PR 2 has not landed, drop that one CI line.

## 12. Changes from v1

| v1 | v2 |
|---|---|
| `cmp` of `-trimpath` binaries proves purity | Impossible — ~40% of bytes differ on a verbatim move. Replaced with normalized decl-set diff |
| `git stash` recipe for the baseline build | False pass on clean trees, strands the split on dirty ones. Replaced with `git worktree add` |
| `git diff -M` renders renames | It does not — zero renames at full scale. Replaced with one commit per extracted file |
| `decideNextAction` | Does not exist — `decideLoopAction`/`Smart`/`AI` + `checkHardGuardrails`, own file |
| `executeTriageAction` unassigned | Assigned to `auto_loop.go` |
| `tools.go` | Collides with an existing file — `toolexec.go` |
| "Neither commit touches skills" | Three sources hard-code `main.go`; plugin tree mirrors them |
| 8 knowledge entries | 15, from grep |
| "no linter" appears twice in CLAUDE.md | Once, at `AGENTS.md:81` (`CLAUDE.md` is a symlink) |
| "fix what the linters flag" | `go vet` already clean; staticcheck returns exactly 9, enumerated |
| Suppress-and-ticket risk row | Inapplicable at n=9; replaced with `git log -S` per deletion |
| Lands after #4–#7 and #18 | No dependency — #4–#7 closed, #18 merged, no open PR touches Go |
| Smoke differential from post-run HEAD | Vacuous (`actionComplete`, zero shell-outs). Forks from `$BASE` with M1 pending |
| Scope-guard step with no timing guidance | Snapshot window explained; three assertions; retry rule |
| "Repeat Steps 2 and 4 with `--tool codex`" | Step 4 half not executable — split 7a/7b |
| Host-only build coverage | 5-platform matrix + `GOOS=windows go vet` |
| No CI | This PR owns `.github/workflows/ci.yml` |
| No rollback or blame story | `git blame -C -C`; rollback is re-concatenation |
