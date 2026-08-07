# 0005 — Split `main.go`, add CI

**What this does.** Adds a GitHub Actions workflow that builds, tests and lints on every push and pull request. Deletes eight functions nothing calls. Splits `cmd/belmont/main.go` into fifteen files in the same package.

**Why now.** Belmont has no automated checks of any kind. Two defects shipped undetected as a result — the plugin distribution published empty agent files for two months, and a worktree function was a no-op for its entire life. Both were found by a person reading code for an unrelated reason; both have since been fixed by hand. This PR installs the first mechanism that would have found either, and that would stop the next one recurring.

**What it does not do.** No behaviour changes. No renames. No reformatting. No new dependencies. No new packages — the files move, the package does not.

**Sequencing.** Unblocked. 0006 merged as PR #21 on 2026-08-07 and shipped in v0.10.15, so `generate-plugin.sh --check` is now an honest CI gate. Branch from a `main` containing `ae26e29`.

**If you only want part of this,** the CI and the split are independent and land in separate commits. §10 says how to take one without the other.

---

## 1. Why

### 1.1 The plugin distribution was broken for two months

As published on `main` from 2026-06-04 until PR #21 merged on 2026-08-07:

| Published file | Lines | Source |
|---|---|---|
| `plugin/agents/code-review-agent.md` | **0** | 268 |
| `plugin/agents/codebase-agent.md` | **0** | 215 |
| `plugin/agents/reconciliation-agent.md` | **0** | 128 |
| `plugin/agents/verification-agent.md` | **0** | 363 |
| `plugin/agents/design-agent.md` | 118 | 276 |
| `plugin/agents/implementation-agent.md` | 102 | 427 |

For sixty-four days, anyone installing Belmont through the marketplace got no verification agent, no code-review agent, no codebase agent and no reconciliation agent.

It broke on 2026-06-04. `ad5f84c` removed frontmatter from the source agents — correctly, since agent frontmatter is not read at runtime — and that exposed a latent flaw in `scripts/generate-plugin.sh`. `f69e784` cut v0.10.12 the same day. Every release since has shipped it. Sixty-four days, no bug report, because the failure is silent: the orchestrator dispatches a sub-agent, tells it to read an empty file, and the sub-agent improvises. The output looks plausible and skips every actual verification phase.

**0006 fixed this once. This PR is what makes the fix stick.**

That distinction matters more than the bug, and it is the case for this PR in one paragraph. `generate-plugin.sh --check` cannot catch this class of failure by itself — it regenerates using the same code and diffs the result against the committed tree, so both sides are wrong in the same way. 0006 added a real assertion: published line count must equal source plus three. Verified on `main` at `ae26e29` — 271/366/218/131/279/430 against sources of 268/363/215/128/276/427.

**Nothing runs that assertion.** It is a script somebody has to remember to invoke. The commit that broke the generator, `ad5f84c`, touched neither `generate-plugin.sh` nor `plugin/`, so there was no review-time signal; the damage appeared at the next release. The next commit of that shape re-breaks it, and nobody finds out for another two months.

### 1.2 The same shape, twice more

**A worktree function never worked.** `writeWorktreeGitExcludes` wrote `.belmont/` into a linked worktree's own `$GIT_DIR/info/exclude`. Git resolves that path from `$GIT_COMMON_DIR`, so it never read the file. It was a no-op from the day it was written. Found by inspection; fixed in a separate PR.

**Eight functions are dead.** `cleanupAll` lost both call sites in `b85f448` when they moved to `gracefulShutdown`; `excludeBelmontInWorktree` lost both in `cd0b82f`. Both commits are from 26 March — over four months dead. `staticcheck` names all eight in under a second, and has never been run.

| Function | Line |
|---|---|
| `parseMasterFeatureStatuses` | 2227 |
| `upsertMarkedSection` | 4683 |
| `containsTool` | 4782 |
| `isCriticalConfig` | 7383 |
| `hasPendingTasks` | 8264 |
| `(*worktreeTracker).cleanupAll` | 8799 |
| `prepareWorktreesGitignore` | 9914 |
| `excludeBelmontInWorktree` | 9940 |

Each of these three is the same shape: a change in one place quietly broke or orphaned something in another, and nothing spanned the gap. That is precisely what a linter and a test run catch.

### 1.3 Belmont has tests. Nothing runs them.

2,989 lines across eight files, and no automation touches them:

- `.github/workflows/release.yml` is the only workflow. It triggers on tag push and only builds. There is no `go test` in it.
- `scripts/release.sh` and `scripts/build.sh` don't run tests either.
- There are no git hooks.

The suite runs when someone remembers to type the command. A pull request can merge with a broken suite; a release can ship with one.

`go vet ./...` is already clean, so half the lint work is wiring only. `staticcheck ./...` returns exactly nine findings — the eight dead functions plus one `SA4006` at `:10839`.

### 1.4 You cannot see a subsystem

This is a different kind of argument from the three above. No defect has shipped because of it. It is about what the code costs to change safely.

`main.go` being 12,613 lines is not itself the problem. Go's unit of encapsulation is the package, not the file, and the compiler does not care. The problem is that **the file is not laid out by subsystem** — each one is scattered through it:

| Subsystem | First function | Last function | Span |
|---|---|---|---|
| Worktree | `buildWorktreeEnv` :572 | `copyBelmontStateToWorktree` :9980 | **9,408 lines** |
| Tool execution | `resolveModelFlags` :119 | `buildToolCommand` :7984 | **7,865 lines** |
| State parsing | `milestoneAllDone` :299 | `parseMilestones` :2523 | **2,224 lines** |
| Monorepo | `seedWorkspaceEnv` :681 | `resolveWorkspaces` :1509 | **828 lines** |

`executeLoopAction` (:6862) calls `snapshotProgress` (:11988) — caller and callee 5,126 lines apart.

So to change worktree behaviour safely, you have to already know the code sits in two clumps 9,400 lines apart: `buildWorktreeEnv` near the top, `rebaseWorktreeOnMain` and `copyBelmontStateToWorktree` near the bottom. **That knowledge is in your head. It is not in the repository, and nothing keeps it true.** Anyone else — a new contributor, or an agent working from grep — finds one clump, changes it, and never learns the second exists.

**After the split, `worktree.go` is the worktree subsystem.** Opening the file is how you find out what is in it. The layout stops being oral history and becomes something the file listing states and a reviewer can check.

Two smaller consequences, worth noting but not worth arguing over:

- All forty of the last forty commits on `main` touch `cmd/belmont/main.go`. So `git log cmd/belmont/main.go` is just `git log`, and any two branches in flight conflict by construction. Both unmerged branches on this fork touch it.
- The file is 419,726 bytes — very roughly 105,000 tokens. Only people developing Belmont ever pay that; people using it never load the file. Treat it as a contributor cost, not a user-facing one.

`CONTRIBUTING.md:81` describes the file as "All CLI logic (single file, no external deps)". The no-external-deps half is the valuable half, and this PR leaves it untouched — the files move, the package does not, and nothing new is imported. The single-file half buys nothing in Go that a single package does not already buy.

If you disagree with this section, §10 is written so you can take §1.1–1.3 and refuse it.

---

## 2. How to review a 12,000-line diff safely

Two mechanisms. Both exist because "trust me, it's a pure move" is not reviewable.

### 2.1 Proof that nothing changed but location

`scripts/declsum/main.go` — a small tool in its own `package main`, so it never enters `cmd/belmont`. It parses the package with `go/parser` and, for every top-level declaration in the non-test files, prints `name → sha256(canonical printer output)`. Then it sorts.

```bash
git worktree add /tmp/pr3-base main
declsum /tmp/pr3-base/cmd/belmont > /tmp/base.txt
declsum ./cmd/belmont > /tmp/after.txt
diff /tmp/base.txt /tmp/after.txt && echo "PURE MOVE"
```

- Sorting makes it blind to which file a declaration lives in — that is the point.
- Hashing the printed AST makes it sensitive to any statement reorder, pasted line or flipped operator inside a declaration.
- Import declarations are skipped; they legitimately duplicate across a split.
- Test files are excluded by filename, not by line filtering.

Verified against two controls on a 398-declaration baseline of the real `cmd/belmont`:

| Control | Result |
|---|---|
| Move four `milestoneAll*` functions verbatim into a new `state.go` | identical → `PURE MOVE` |
| Same move, then invert two predicates (`== taskBlocked` → `!=`, `!= taskTodo` → `==`) — compiles clean, preserves the declaration set | differs → caught |

The second control is the case that matters: a change that a naive text or symbol comparison would miss.

Use `git worktree add` for the baseline, not `git stash`. On a clean tree the stash is a no-op and both sides build from the split tree, which prints a false pass. Add `-buildvcs=false` to any comparison build.

Two approaches were tried and do not work, so they aren't proposed here:

- **`cmp` on `-trimpath` binaries.** A verbatim move produces a same-size binary with ~40% of bytes differing — function layout plus pclntab and DWARF basenames. `-trimpath` strips directories, not basenames. `-ldflags="-w -s"` still leaves 1.82 MB differing.
- **`go tool nm` symbol name+size sets.** Misses the predicate inversion.

### 2.2 One commit per extracted file

Each commit moves exactly one domain out of `main.go`, so `git show --stat` reads as a legible pair: `main.go -N / newfile.go +N`. The declsum check runs at every commit, not only at the end.

`git diff --color-moved=zebra` helps when reading an individual commit.

Note that `git diff -M` will **not** render this as renames — tested at full scale, it reports zero. `main.go` survives the split so it can never be a rename source, and each new file is 9–12% of it. Forcing `--find-copies=1%` produces copy markers but inflates the patch from 22k to 118k lines. Hence per-commit review rather than rename detection.

---

## 3. The split

Files move; the package does not. The stdlib-only rule in `AGENTS.md` constrains **dependencies, not file count**.

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
| `reverify.go` | `runReverifyCmd` |
| `monorepo.go` | `detectWorkspaces`, `resolveWorkspaces`, `seedWorkspaceEnv` |
| `multifeature.go` | `runAutoMultiFeature`, `computeFeatureWaves`, readiness scanning |
| `reconcile.go` | `runReconciliationAgent`, `runReconciliationAnalysis`, `recover` |

**Rule for anything not named above:** a declaration goes to the file owning its nearest caller. If callers span domains, it stays in `main.go`. Every declaration gets a stated home before the first commit — an unassigned symbol means the table needs re-cutting, not a judgement call mid-split.

`decideLoopAction*` and `checkHardGuardrails` are ~498 lines and contain two of the file's ten largest functions, which is why they get their own file rather than a third of `main.go`'s remaining budget.

**The boundaries are the substance of this PR.** Treat the table as a proposal — re-cut on request, and say so before the first commit rather than after the tenth.

---

## 4. CI

New `.github/workflows/ci.yml`, on push and pull request:

- `go build ./cmd/belmont`
- `go test ./cmd/belmont`
- `go vet ./...`
- `GOOS=windows go vet ./...`
- `staticcheck ./...`
- `scripts/generate-skills.sh --check`
- `scripts/generate-plugin.sh --check`

Plus a five-platform build matrix: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64.

The matrix is not padding. `go build`, `go test` and `scripts/build.sh` all cover host GOOS/GOARCH only, and `release.sh` verifies host-only *before* it commits and tags. Demonstrated: deleting the `//go:build !windows` line from `process_unix.go` leaves every existing check green while `GOOS=windows go build` fails with four redeclaration errors. That file is a plausible casualty of a "tidy the platform files" re-cut, since `_unix` is not an implicit-constraint suffix.

The embed axis is already covered — plain `go build` is `!embed`, `build.sh` is `-tags embed` — so it's deliberately excluded from the matrix.

`staticcheck` is a CI-time tool. It does not affect the stdlib-only rule, which governs what the binary imports.

### 4.1 The nine findings, confirmed

Run against `main` at `5f1c2c2` with `staticcheck 2026.1 (v0.7.0)`:

```
main.go:2227:6   func parseMasterFeatureStatuses is unused (U1000)
main.go:4683:6   func upsertMarkedSection is unused (U1000)
main.go:4782:6   func containsTool is unused (U1000)
main.go:7383:6   func isCriticalConfig is unused (U1000)
main.go:8264:6   func hasPendingTasks is unused (U1000)
main.go:8799:28  func (*worktreeTracker).cleanupAll is unused (U1000)
main.go:9914:6   func prepareWorktreesGitignore is unused (U1000)
main.go:9940:6   func excludeBelmontInWorktree is unused (U1000)
main.go:10839:4  this value of progressContent is never used (SA4006)
```

The `SA4006` is a dead assignment inside `runReverifyCmd`: after the rewrite, `progressContent = []byte(newContent)` is set but never read, because `milestones` is recomputed from `newContent` directly on the next line. Deleting the one assignment is the whole fix.

**A tenth deletion is already on its way to `main` separately.** PR #22 removes `writeWorktreeGitExcludes` — a function that wrote `.belmont/` into a linked worktree's own `$GIT_DIR/info/exclude`, which git resolves from `$GIT_COMMON_DIR` and therefore never reads. `staticcheck` does not flag it, because it *was* called; only the call site kept it alive.

It is deliberately **not** folded into this PR. Landing it first keeps this PR's lint commit a clean set of nine, so the declsum expectation in §8 stays exact — which matters in a PR whose argument is that the diff is mechanically verifiable. Branch from a `main` that already contains #22. If #22 has not landed by then, expect the declsum diff at the lint commit to carry the extra removal plus an edited `copyBelmontStateToWorktree`, and say so in the PR description rather than letting the check trip.

## 5. What this touches outside Go

**Skill sources hard-code `main.go`.** Three committed sources reference it:

- `_partials/tier-registry.md:15` and `_src/references/models-yaml-format.md:38` — both point at the `modelTiers` map, which moves to `toolexec.go`
- `_partials/debug-scope-rules.md:7` — points at the `actionDebug` wiring, which moves to `auto_decide.go` and `auto_loop.go`

These are inlined into the tracked plugin tree at `plugin/skills/{implement,verify}/SKILL.md:201`, `plugin/skills/debug-manual/SKILL.md:123`, `plugin/skills/tech-plan/references/models-yaml-format.md:38`.

Editing either location by hand evaporates: `generate-plugin.sh:35` does `rm -rf "$PLUGIN_DIR"`, and `skills/belmont/*/` is gitignored. Edit the three `_src`/`_partials` sources, regenerate both, commit `plugin/`. Change them to **symbol-only references** so they can't go stale again.

**Docs.** `AGENTS.md` Architecture (no longer single-file) and Verify. The "There is no linter configured" sentence at `AGENTS.md:81` also claims "a small focused test file exists at `cmd/belmont/commit_update_test.go`" — that's stale, there are eight test files. `CLAUDE.md` is a symlink to `AGENTS.md`, so one edit covers both. Also `CONTRIBUTING.md:81` and `:138` ("All CLI logic is in `cmd/belmont/main.go`" — already imprecise, `local_llms.go` is 385 lines) and `docs/directory-structure.md`.

**Knowledge.** `grep -rln 'main\.go' knowledge/` returns 15 entries. Drive the update from the grep, not a hand-written list. One `Revisions` line each; confirm each routing trigger still matches.

---

## 6. Both invocation paths

Belmont runs from `belmont auto` and from a user typing `/belmont:implement`. The CLI backs both, so a regression here breaks everything. §5 shows skill sources are touched too, so the plugin surface needs exercising as well. All three are covered in §7.

---

## 7. Author smoke test

**Precondition.** `requireCleanWorkingTree` aborts `auto` on any dirty or untracked path, and `install` never commits. Commit after every install.

**Step 0 — record the starting point.**

```bash
cd ~/path/to/real-project && git checkout -b smoke/pr3
belmont install --source ~/belmont --no-prompt && git add -A && git commit -m "install"
BASE=$(git rev-parse HEAD)      # M1 still pending here — every run forks from this
```

This matters. If you branch from a HEAD where M1 is already `[x]` or `[v]`, `decideLoopActionSmart` returns `actionComplete` on the first iteration, before any shell-out, and the run exercises none of the relocated code. `milestoneAllDone` accepts `[x]`, so this fires even without verification.

**Step 1 — purity, at every commit.** Run the §2.1 declsum diff. Expect `PURE MOVE` and an empty diff. Paste the output into the PR description.

**Step 2 — auto end-to-end, at the final commit.** From `$BASE`, run `belmont auto --feature <slug> --from M1 --to M1` on pre- and post-split binaries. Assert behaviourally — phases executed, guard fired, worktrees and ports created — not by JSON byte-equality.

**Step 3 — scope guard.** `snapshotProgress` runs at the top of `executeLoopAction`, so a heading added between phases is baked into the baseline and the guard correctly finds nothing. Wait for the phase line to appear, then append `### M99: Injected` plus one task, then assert three things:

```bash
# the guard line is ANSI-coloured, so a literal grep will not match. Strip first:
sed 's/\x1b\[[0-9;]*m//g' run.log | grep -F '[SCOPE-GUARD] reverted'
grep -c 'M99' PROGRESS.md || true      # expect 0; bare grep -c exits 1 under set -e
grep -F '(pending)' STEERING.md | grep -F 'M99'
```

If the `[SCOPE-GUARD]` line is absent, the edit landed outside the window — retry, don't record a pass.

This step is the only coverage of the read → revert → write → amend → STEERING wiring. No test calls `runScopeGuard`. Keep it.

**Step 4 — interactive.** `git checkout -b smoke/pr3-interactive $BASE`, then `claude` → `/belmont:implement` → `/belmont:verify`.

**Step 5 — parallel + worktrees.** From `$BASE`: `belmont auto --feature <slug> --max-parallel 2`. Expect a distinct `BELMONT_PORT` per worktree, sequenced merges, cleanup on exit. Exercises `auto_parallel.go` and `worktree.go`.

**Step 6 — install path.** `belmont install --source ~/belmont --project /tmp/pr3-install --no-prompt`. Confirm `.agents/skills/belmont/`, the per-skill slash commands, and that `loop` is a real file rather than a symlink.

**Step 7 — a second tool.**
- 7a: `belmont auto --feature <slug> --from M1 --to M1 --tool codex` from `$BASE`. The `--feature` flag is required — `runAutoCmd` rejects a bare invocation before any shell-out.
- 7b: interactive `codex` → `$belmont` popup → `$implement`. (`--tool` is a CLI flag with no REPL meaning, and Codex has no `/belmont:` command.)

Step 2 omits `--tool`, so `detectTool()` picks claude — Steps 2 and 4 are the Claude both-modes check.

**Step 8 — cross-platform.** `GOOS=windows go build ./cmd/belmont` plus the other four targets, and `GOOS=windows go vet ./...` (passes today).

---

## 8. Definition of done

**Split**
- [ ] `main.go` under ~1,500 lines; no new file over ~2,000
- [ ] Package unchanged — files moved, no new packages
- [ ] One commit per §3 row; `git show --stat` shows a legible `main.go -N / newfile.go +N` pair
- [ ] Declsum diff empty at every extraction commit
- [ ] `scripts/declsum/main.go` re-verified against both controls before the DoD is relied on
- [ ] Zero logic edits — no renames, no signature changes, no reformatting
- [ ] Every declaration assigned a home before commit 1
- [ ] No new filename collides with an existing file. Verify with `ls cmd/belmont/*.go` at branch time — today the six non-test files are `main.go`, `local_llms.go`, `embed.go`, `embed_dev.go`, `process_unix.go`, `process_windows.go`
- [ ] Boundaries align with the knowledge-tree domains

**Lint**
- [ ] All nine staticcheck findings resolved, each deletion carrying a one-line `git log -S` justification
- [ ] Suppression count stated in the PR description — no silent scope truncation
- [ ] At the lint commit the declsum diff shows exactly the eight deletions plus the SA4006 line, and nothing else

**CI**
- [ ] `.github/workflows/ci.yml` added with the §4 job list
- [ ] Five-platform matrix green; `GOOS=windows go vet ./...` green
- [ ] `AGENTS.md:81` corrected — both the linter claim and the stale test-file sentence

**Skills / plugin**
- [ ] Three `_src`/`_partials` sources changed to symbol-only references
- [ ] `generate-skills.sh` and `generate-plugin.sh` run; `plugin/` committed
- [ ] Both `--check` scripts pass (`plugin.json` STALE is the only legitimate diff)

**Verification**
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green at every commit
- [ ] `./scripts/build.sh <version>` produces a working embedded binary
- [ ] Scope guard verified live within the timing window (Step 3)
- [ ] Auto, interactive and plugin surfaces exercised
- [ ] All 15 knowledge entries updated from the grep, with `Revisions` lines
- [ ] `AGENTS.md`, `CONTRIBUTING.md:81,138`, `docs/directory-structure.md` updated

---

## 9. Risks

| Risk | Mitigation |
|---|---|
| A behaviour change hides in 12.6k moved lines | Declsum hashing at every commit, verified against a legitimate move and a predicate inversion |
| Reviewer rejects on the stdlib-only rule | Pre-empted in paragraph one: it constrains dependencies, not file count |
| A platform-file re-cut breaks Windows silently | Five-platform matrix plus `GOOS=windows go vet` in CI and DoD |
| Deleting live code as "unused" | `git log -S` justification per deletion |
| Knowledge routing goes stale | Driven from grep output, not a hand list |
| Archaeology after the split | `git blame -C -C` resolves through the move with no config. `.git-blame-ignore-revs` does **not** help — tested; it needs the line to persist in the same file |
| Rollback | `git revert` conflicts once any later commit touches a new file. Rollback is re-concatenation, not revert. The lint work is a separate commit so doc corrections aren't entangled |
| In-flight branches conflict | Any unmerged branch touching `main.go` becomes hard to rebase after the split. Rebase or land them first |

---

## 10. If you only want part of this

The CI and the split are independent. They land in separate commits and neither needs the other.

**To take the CI and refuse the split.** The lint-and-CI work is the final commit. It adds `.github/workflows/ci.yml`, deletes the eight dead functions and corrects the docs — roughly 100 lines of YAML against 200 deleted lines of Go, and nothing in it depends on the file layout. Say so on the PR and I will re-cut it as a standalone branch rather than making you cherry-pick.

This is a reasonable thing to want. §1.1–1.3 are about defects that have already shipped; §1.4 is a judgement call about how the code is laid out. They are not the same kind of claim and you should not have to accept the second to get the first.

**To take the split and refuse the CI.** Possible, but there is no reason to — the split is the part that most needs a safety net underneath it.

**To re-cut the boundaries.** §3's table is a proposal, not a decision. Say which rows you would change *before* commit 1. Re-cutting mid-split means re-running the purity check on every commit already made.

---

## 11. Relationship to the other proposals

- **0006** — **done.** Merged as PR #21 on 2026-08-07, shipped in v0.10.15. The `generate-plugin.sh --check` CI gate is now honest.
- **0003** — overlaps. Both edit `_src/references/models-yaml-format.md` and both append to `model-tier-economics.md`'s `Revisions` footer. All the proposals also append to `KNOWLEDGE.md`'s cross-cutting table, so expect a trivial conflict — keep every row.
- **0004** — no Go conflict. This PR's CI consumes 0004's eval entry point (`go test -tags eval ./cmd/belmont`); if 0004 hasn't landed, drop that one CI line.
