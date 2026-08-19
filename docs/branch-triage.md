# Branch triage — the unmerged backlog

*Produced by `throughput` P0-13 on 2026-08-18, against local `main` at `4f71915c`.
Every verdict below is measured, not recalled. Re-run the commands in
§How this was measured before trusting a row that looks stale.*

## Why this file is here and not in `docs/proposals/NEXT-SESSION.md`

The feature TECH_PLAN nominated `docs/proposals/NEXT-SESSION.md` as the home for
this triage. That is the wrong file: `NEXT-SESSION.md` exists **only** on
`origin/docs/pr-proposals`, and this triage's headline verdict is that that
branch should be merged. Writing the triage into a file the merge also carries
would manufacture a conflict in the one branch we most want to land cleanly.
`docs/branch-triage.md` is on the mainline, collides with nothing, and is where
the next session will look.

## Headline

**36 unmerged remote branches. 30 are already dead, 3 should merge, 3 need a rebase.**

**Scope: `origin/*` only.** `upstream/*` is deliberately excluded and always was —
the measurement command below has carried `grep -v upstream/` since the first
triage — but that decision lived only in the command, so a later reader running
`git branch -r --no-merged main` without it counts 38 and concludes the file is
three verdicts short. It is one short. `upstream/belmont` (2026-02-05) and
`upstream/v1-rebuild` (2026-06-04) are the third party's own branches: this fork
tracks `upstream/main` and merges it periodically, and nothing here proposes a
verdict on branches we do not own. Stated in prose now rather than implied by a
`grep`, per `P0-M1-FIX-17`.

The backlog is far less dangerous than the plan assumed, and dangerous in a
different place than the plan predicted.

- **The two branches the milestone analysis flagged as most hazardous carry no
  code `main` does not already have.** `origin/fix/unrecognised-task-markers`
  (24 commits, 64 files, described as "the widest collision in the backlog")
  and `origin/feat/maintenance-ci` (the only branch said to collide with P0-12,
  P0-1 *and* P0-2 at once) appear in `git branch -r --no-merged` only because
  they were rebased before landing, so their SHAs differ from the commits that
  carry their patches. `git cherry main <branch>` reports 0 outstanding patches
  for the first and 2 for the second. **Corrected 2026-08-18 by
  `P0-M1-FIX-3`:** of `maintenance-ci`'s 2, one — `1129cf84`, a 23-line
  knowledge note — was **not** on `main` and has now been cherry-picked onto
  local `main` as `24db490`; the other is superseded. The two changes this
  bullet originally named as the leftovers are on `main`, but neither is what
  those commits contain. Evidence in the row below.
- **Four of the five collisions named in the TECH_PLAN are also already on
  `main`**: `feat/loop-efficiency`, `fix/wave-merge-state-loss`,
  `fix/state-readers-and-live-views` and `fix/unrecognised-task-markers`. Only
  `feat/verify-dedup` is genuinely outstanding, and it is unmergeable for an
  unrelated reason (see below). **M3, M4 and M6 are not standing on contested
  ground.**
- **The real hazard the plan did not name is `origin/fix/post-51-triage`** —
  the newest branch in the backlog (2026-08-17), 15 genuinely unlanded commits,
  six new regression test files, and it merges into `main` with **zero
  conflicts** today. It touches `reconcile.go` and `worktree.go`, which P0-1
  rewrites. Merge it before P0-1 or pay for it afterwards.

## Verdicts — all 36 branches

Legend: **merge** = land as-is; **rebase** = content is wanted but the branch
cannot apply where it sits; **abandon** = nothing left to recover, delete the
remote branch.

### Merge (3)

| Branch | Last commit | Verdict | Reason |
|---|---|---|---|
| `origin/docs/pr-proposals` | 2026-08-08 | **merge** | 15 files under `docs/proposals/` exist nowhere on `main`, including `0004-context-budget-with-evidence.md` rev 5 — **M2's specification**. `git merge-tree` yields exactly one conflict, in `.gitignore`. Highest-value, lowest-risk merge in the backlog. |
| `origin/fix/post-51-triage` | 2026-08-17 | **merge** | 15 unlanded commits and 6 new regression tests (`merge_carry_placement`, `recover_active_run`, `reverify_reset`, `wave_integration`, `listing_unreadable_worktree`, `subcommand_help`). Merges with **zero conflicts**. Touches `reconcile.go`/`worktree.go` — **merge before P0-1**. |
| `origin/fix/issue-sweep` | 2026-08-18 | **merge** | Added by `P0-M1-FIX-17`; pushed after the initial triage, which is why it had no verdict. 9 unlanded commits (`git cherry` reports 9 `+`, 0 landed), 33 files, +1,336/−211, closing #54, #56, #58, #59, #61. `git merge-tree` yields **zero conflicts** against `main` today. **Merge before wave 2**: it rewrites `_partials/dispatch-strategy.md` — M8/P1-11's own deliverable — plus `_partials/loop-recipe.md` and `_src/{implement,loop,next,status}.md`, which M2/P1-5 rewrites. Resolving that across seven concurrent worktrees costs far more than landing it at a fork point, which is the same argument prerequisite 7 makes for the redaction. |

### Rebase (3)

| Branch | Last commit | Verdict | Reason |
|---|---|---|---|
| `origin/feat/design-contract` | 2026-08-13 | **rebase** | 34 unlanded commits and 18 new files — a whole `ux-design` phase (skill, three knowledge entries, `design_guard_test.go`, a `ui-no-figma` eval fixture). Real work, but 85 files across `toolexec.go`, `guards.go`, `auto_loop.go`, `worktree.go`, `steer.go`; it cannot merge and it collides with M1 and M6. Rebase deliberately, after M6. |
| `origin/proposal/0001-quick-mode` | 2026-05-23 | **rebase** | Content absent from `main` and merges cleanly, but lands at root `proposals/` — the wrong path under the convention settled below. Rebase to relocate into `docs/proposals/`. Carries `framework-evaluation`'s P6-1 spec; it is prior art, not stray work. |
| `origin/proposal/0002-prd-hygiene` | 2026-05-23 | **rebase** | Same: absent from `main`, merges cleanly, wrong path. Carries `framework-evaluation`'s P6-2 spec. |

### Abandon — content already on `main` (15)

Verified by `git cherry main <branch>` reporting **zero** outstanding patches.
Each was rebased or squashed before landing, so only the SHAs differ.

| Branch | Last commit | Reason |
|---|---|---|
| `origin/fix/unrecognised-task-markers` | 2026-08-11 | 24/24 patches on `main`. The plan's "widest collision" is fully landed. |
| `origin/fix/dropped-state-and-dispatch` | 2026-08-16 | 11/11 on `main`; its tip is `main`'s own `17897aa2`. |
| `origin/fix/state-readers-and-live-views` | 2026-08-14 | 7/7 on `main`. Named as an M4 collision; is not one. |
| `origin/feat/eval-harness` | 2026-08-08 | 7/7 on `main`; `eval_harness_test.go` and all fixtures present. |
| `origin/backup/state-readers-pre-main-rebase` | 2026-08-14 | 7/7 on `main`. Explicit pre-rebase backup, purpose served. |
| `origin/fix/repair-move-and-guidance` | 2026-08-13 | 3/3 on `main`. |
| `origin/codex-model-tier-overrides` | 2026-06-04 | 3/3 on `main`. |
| `origin/feat/loop-efficiency` | 2026-08-13 | 2/2 on `main`. Named as an M6 collision; is not one. |
| `origin/codex-plan-apply-handoff` | 2026-06-09 | 2/2 on `main`. |
| `origin/feat/declsum` | 2026-08-07 | 1/1 on `main`; `scripts/declsum/main.go` is present. |
| `origin/feat/claude-loop-skill` | 2026-06-08 | 1/1 on `main`. |
| `origin/codex-skill-display-names` | 2026-06-03 | 1/1 on `main`. |
| `origin/fix/plugin-generator-empty-agents` | 2026-08-06 | 1/1 on `main`. |
| `origin/opencode-support` | 2026-06-03 | 1/1 on `main`. |
| `origin/opencode-slash-commands` | 2026-06-03 | 1/1 on `main`. |

### Abandon — content verified on `main` despite an apparent outstanding commit (5)

`git cherry` compares patch-ids, which context drift defeats. Each of these was
checked by hand against the working tree.

**Caveat added 2026-08-18 (`P0-M1-FIX-3`).** That hand-check failed on the first
row: one of `feat/maintenance-ci`'s two leftovers was never on `main`, and the
reason given named two changes those commits do not contain. The bar this
section is now held to — and the bar an `abandon` verdict has to clear, being a
recommendation to destroy — is that an *"already on `main`"* claim names the
commit **and** shows its content on `main` (grep the heading, cite the lines),
and a *"superseded"* claim names the mechanism on `main` and cites the lines
implementing it. **Two of the rows below `feat/maintenance-ci` have since been
re-audited to that bar.** `origin/fix/worktree-git-excludes` was re-checked by
`P0-M1-FIX-3` and holds (`writeWorktreeGitExcludes` is absent from `cmd/`, with
the three removal comments at `worktree.go:286`, `worktree_state_test.go:60`,
`steer_test.go:148`, and `main`'s `worktree-state-isolation.md` is a superset of
the branch's version). `origin/fix/wave-merge-state-loss` was re-checked by
`P0-M1-FIX-5`: the abandon verdict holds, but its stated reason was wrong and is
replaced in the row itself. **The two rows that remain — the pre-rebase backup
branches `origin/backup/loop-efficiency-pre-rebase` and
`origin/backup/state-readers-pre-rebase` — have not been re-audited to this
bar.**

*Sentence corrected 2026-08-18 (`P0-M1-FIX-8`).* This paragraph previously closed
*"The rows below it were not re-audited to that bar"*, which stopped being true
the moment `P0-M1-FIX-5` re-audited the immediately following row. No verdict was
re-opened in making the correction, and the rows in this document's other
sections remain a known, deliberately deferred gap.

| Branch | Last commit | Reason |
|---|---|---|
| `origin/feat/maintenance-ci` | 2026-08-07 | 29/31 landed. **The original reason here was wrong and is replaced — `P0-M1-FIX-3`, 2026-08-18.** It claimed both leftovers were already present, naming the `worktree-state-isolation` row in `knowledge/KNOWLEDGE.md` and the `main.go` → `autocmd.go` repointing in `clean-tree-preflight.md`. Those two changes *are* on `main` (`knowledge/KNOWLEDGE.md:25`; `knowledge/auto-mode/clean-tree-preflight.md:13` reads ``In `cmd/belmont/autocmd.go`:``) — but neither is what either leftover commit contains. The leftovers are: **(1) `1129cf84`**, 23 lines adding §*Gotcha — `--max-parallel` does not by itself produce worktrees* to `knowledge/auto-mode/parallel-wave-orchestration.md`. This was **genuinely absent** (`git grep 'does not by itself produce worktrees' main` returned nothing) and is now **cherry-picked onto local `main` as `24db490`**, landing at `parallel-wave-orchestration.md:43-62`. The gate it documents is live on `main`: `hasExplicitDeps` at `cmd/belmont/autocmd.go:278-284`, dispatching to `runAutoParallel` only under `if hasExplicitDeps` at `:286`. **(2) `7a9b7b56`**, 26 lines adding §*Confirmed defect — the second merge in a wave clobbers the first's state* to `knowledge/auto-mode/worktree-state-isolation.md`. **Superseded on `main`**, which records the same last-writer-wins defect *and its fix* under §*PROGRESS.md is merged on sync, never replaced* (`worktree-state-isolation.md:44-58`, issue #24, revision `2026-08-08`), implemented by `mergeProgressState` (`cmd/belmont/worktree.go:504`, called from `syncFeatureStateAfterMerge` at `worktree.go:410`). The branch's text records the defect as **open**, so applying it would regress the file — do not cherry-pick it. With (1) landed and (2) superseded, nothing on this branch is unrecovered: **abandon stands**. **This is the branch the milestone flagged as colliding with P0-12, P0-1 and P0-2 simultaneously. It does not.** |
| `origin/fix/worktree-git-excludes` | 2026-08-07 | `writeWorktreeGitExcludes` is already removed from `main` (three source comments record the removal), and `knowledge/auto-mode/worktree-state-isolation.md` exists. |
| `origin/fix/wave-merge-state-loss` | 2026-08-07 | One unlanded commit, `1fcbc7d4` (418 insertions). **The original reason here was wrong and is replaced — `P0-M1-FIX-5`, 2026-08-18.** It claimed `main` *"already scopes the sync per milestone and refuses duplicate headings"*, and that *"only the branch's own test filename is absent"*. `main` does **not** scope the sync per milestone: `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` (`cmd/belmont/worktree.go:370`) takes no milestone ID, still runs `os.RemoveAll(dstFeature)` + `copyDir` (`:397-398`), and neither `resolveMilestoneProgress` nor `milestoneBlockRange` exists anywhere in `cmd/`. Nor is a filename all that is absent — the commit carries a 251-line `cmd/belmont/sync_feature_state_test.go` (11 tests) **and an entire alternative implementation**: a `milestoneID` parameter on `syncFeatureStateAfterMerge`, plus `resolveMilestoneProgress` and `milestoneBlockRange` (a line-exact splice of the worktree's own milestone block onto master's file), with both `auto_parallel.go` call sites updated. **Abandon still stands, but by a different mechanism.** `main` addresses the same defect (issue #24) by reading master's `PROGRESS.md` **before** the wipe (`worktree.go:395`) and union-merging it back afterwards via `mergeProgressState` (defined `worktree.go:504`, called at `:410`) — matched per task ID rather than per milestone block, and stricter than the branch's splice: most-advanced-state-wins by `markerRank` (`state.go:260`), `[!]` wins in both directions, unrecognised markers never ranked, master-only tasks carried into their milestone. The *"refuses duplicate headings"* half of the old reason is true, but of `mergeProgressState`, not of any milestone scoping — duplicate `### M<n>:` headings are counted at `worktree.go:569-585` and refused with a warning at `:586-589`; duplicate task IDs get the same refusal (`:530-558`, warned at `:618-621`). Those rules are documented on `main` at `knowledge/auto-mode/worktree-state-isolation.md:44-58`, a superset of the branch's 28-line edit to the same file. Coverage is `worktree_sync_test.go` (15 tests, of which `TestSyncPreservesSiblingCompletions` is the branch's own two-sibling clobber scenario), `merge_symmetry_test.go` and `worktree_state_test.go`. The branch also no longer applies where it sits: `git merge-tree main origin/fix/wave-merge-state-loss` conflicts in four files, `worktree.go` among them. **One thing the branch has that `main` does not**: in milestone mode it skipped the `RemoveAll` entirely, so a non-`PROGRESS.md` file left in the feature dir by an earlier sibling merge survived (`TestWaveMergeDoesNotDeleteSiblingFiles`). `main` still wipes the whole directory at `worktree.go:397`, so every file in the feature dir except `PROGRESS.md` is still last-writer-wins across a wave. That is a residual gap to close on `main` — not a reason to resurrect a branch whose `PROGRESS.md` mechanism `main` has superseded and whose code no longer applies. |
| `origin/backup/loop-efficiency-pre-rebase` | 2026-08-13 | Pre-rebase backup of `feat/loop-efficiency`, which is itself fully landed. |
| `origin/backup/state-readers-pre-rebase` | 2026-08-14 | Pre-rebase backup of `fix/state-readers-and-live-views`, which is fully landed. |

### Abandon — unmergeable against the current layout (7)

These edit `skills/belmont/<skill>.md`, the **pre-`_src/` layout**. Those paths
are now **gitignored generated output** — `scripts/generate-skills.sh` builds
them from `_src/` and `_partials/`. A merge would either conflict or write into
an ignored file. None of these can be rebased mechanically; the content has to
be re-authored into `_src/` if it is still wanted.

| Branch | Last commit | Reason |
|---|---|---|
| `origin/feat/verify-dedup` | 2026-04-12 | Edits `skills/belmont/verify.md` (generated). The only genuine survivor of the TECH_PLAN's five named collisions — but it is a **re-author, not a merge**. Its subject matter is M2/P0-5; fold the idea in there rather than resurrecting the branch. |
| `origin/feature/agent-dedup-fix` | 2026-04-12 | Same file, same defect, near-duplicate of the above. |
| `origin/milestone-dereference-prd` | 2026-04-21 | Edits `skills/belmont/implement.md` (generated). Intent already on `main`: the implementation agent dereferences PRD/TECH_PLAN from the MILESTONE rather than inlining them. |
| `origin/skill-progressive-disclosure` | 2026-04-21 | Introduces a `references/` convention that `main` already has as `skills/belmont/_src/references/`. |
| `origin/skill-progressive-disclosure-rest` | 2026-04-21 | Same convention applied more widely; same reason. 21 of its 50 files do not exist on `main`. |
| `origin/trim-dispatch-strategy` | 2026-04-21 | Pre-`_src/` edits to the dispatch-strategy partial. That partial is **M8/P1-11's** deliverable; do it there. |
| `origin/status-colors` | 2026-04-23 | Edits `skills/belmont/status.md` (generated). Its single patch is also already on `main`, so there is nothing to recover either way. |

### Abandon — superseded by a later architectural decision (2)

| Branch | Last commit | Reason |
|---|---|---|
| `origin/fix-gemini-ask-user` | 2026-05-04 | Its multi-framework work writes `AGENTS.md` / `GEMINI.md` routing sections, which `main` **deliberately stopped writing** (`cmd/belmont/install.go:118`, "Phase 2: ... are no longer written"). The `ask_user` support never landed and is not recoverable from this branch's shape — refile as a fresh task if still wanted. |
| `origin/framework-awareness` | 2026-05-04 | A strict subset of the branch above (both its commits are in it); same superseding decision. |

### Abandon — stale against current policy (1)

| Branch | Last commit | Reason |
|---|---|---|
| `origin/opus-default-model-selection` | 2026-06-04 | Wants all agents defaulted to the high tier. That intent is now policy, but it is expressed per-feature in `<feature>/models.yaml` under the barbell rule (high or low, never medium), and the tier→model map in `toolexec.go` has changed underneath the branch. Re-apply as a fresh change if the *repo* default should move. |

## The proposals-path convention — settled

Two conventions were in flight:

- root `proposals/` — `origin/proposal/0001-quick-mode`, `origin/proposal/0002-prd-hygiene` (2026-05-23)
- `docs/proposals/` — `origin/docs/pr-proposals`, holding 0003–0006 (2026-08-08)

**`docs/proposals/` is canonical.** It carries the larger and far more heavily
reviewed set (36 commits, 15 files, including the rev5/rev6 adversarial review
artefacts), it is the path named by the master TECH_PLAN's rule 5, and it is
where this feature's owed proposals `0007-detail-tier`, `0008-size-ceilings` and
`0009-master-and-journal` will be written. 0001 and 0002 relocate into it on
rebase; their numbers are free and do not collide.

## Where M2 reads its specification

**`origin/docs/pr-proposals` → merge.** Once merged,
`docs/proposals/0004-context-budget-with-evidence.md` is on the mainline and
every worktree cut from `main` can read it. This is the answer M2 is blocked on.

Until that merge happens, M2 must read the file directly off the remote branch
rather than from a worktree checkout:

```bash
git show origin/docs/pr-proposals:docs/proposals/0004-context-budget-with-evidence.md
```

Both readings are recorded because the merge is not this task's to perform —
see below.

## Not done here: bringing the default branch up to date

`origin/main` is **157 commits behind** local `main`. The task's acceptance
clause "the default branch matches the working state" is **deliberately not
satisfied by this task**, and is tracked as a blocked task in
`.belmont/features/throughput/PROGRESS.md` under M1.

Satisfying it means pushing 157 commits to a public GitHub fork. That is a
publication decision belonging to the repository owner, not to an implementation
agent, and it has been escalated to them. Nothing in this triage was pushed, no
remote branch was deleted, merged or rebased, and no PR was opened. Every
`abandon` verdict above is a **recommendation to delete**, not a deletion.

## How this was measured

```bash
# the 36 in scope — note `grep -v upstream/`: the backlog is origin/* only,
# and without that filter this returns 38 (see §Headline)
git branch -r --no-merged main | grep -v upstream/ | grep -v HEAD

# supersession — the load-bearing one. '-' = patch already upstream, '+' = outstanding
git cherry -v main <branch>

# content genuinely absent from main (git rev-parse needs --verify --quiet;
# without it, it echoes unresolvable args back and every file looks present)
git rev-parse --verify --quiet "main:$f"

# mergeability without touching the working tree
git merge-tree --write-tree main <branch>
```

Three traps worth recording. The first two produced a wrong answer first; the
third is a consequence of correcting one of them:

1. **`git branch --no-merged` over-reports badly in a rebase-heavy repo.** It
   compares commit ancestry, so a rebased-and-landed branch still lists as
   unmerged. 21 of these 36 branches are fully or effectively landed. Use
   `git cherry`, and hand-check the residue — patch-ids are defeated by context
   drift, which is what produced the false positives on `feat/maintenance-ci`
   and `fix/worktree-git-excludes`.
2. **Two-dot `git diff main..<branch>` is meaningless here.** With `main` 157
   commits ahead it reports main's own additions as the branch's deletions —
   every branch looked like a 30,000-line deletion. Use three-dot
   (`main...<branch>`) for a branch's own changes.
3. **A cherry-picked commit still shows as `+` under `git cherry`.** `1129cf84`
   was cherry-picked onto `main` as `24db490` on 2026-08-18 and `git cherry -v
   main origin/feat/maintenance-ci` *still* lists it as outstanding, because the
   conflict resolution changed its context lines and therefore its patch-id.
   Re-running the commands above will contradict the row for
   `feat/maintenance-ci`; the file content is the authority, not the count.

## Revisions

- 2026-08-19 — re-measured against the remote after `git fetch --prune`
  (`throughput` `P0-M1-FIX-17`). **35 → 36 in-scope branches.** One genuinely new:
  `origin/fix/issue-sweep`, pushed 2026-08-18 after the initial triage, verdict
  **merge**, and flagged to land before wave 2 because it rewrites M8/P1-11's and
  M2/P1-5's own files. The other two branches a naive re-run surfaces
  (`upstream/belmont`, `upstream/v1-rebuild`) were never in scope — the method's
  `grep -v upstream/` excluded them from the start — so the scope is now stated in
  §Headline instead of being implied by a shell filter. Count corrected at all
  four sites plus this footer. Note for anyone re-deriving these: `main` was
  rewritten by `git filter-repo` on 2026-08-19, so every pre-rewrite SHA quoted
  in this file resolves only as an unreachable object; match commit **subjects**
  instead (`P0-M1-FIX-25`).
- 2026-08-18 — initial triage, all 35 branches, `throughput` P0-13.
- 2026-08-18 — `origin/feat/maintenance-ci` re-verdicted (`throughput`
  P0-M1-FIX-3). One of its two leftovers, `1129cf84`, was **not** on `main`;
  cherry-picked as `24db490`. The other, `7a9b7b56`, is superseded and named as
  such. Abandon still stands, now on evidence rather than on a commit count.
  Headline bullet and the section preamble corrected to match.
- 2026-08-18 — `origin/fix/wave-merge-state-loss` re-verdicted (`throughput`
  P0-M1-FIX-5). **Abandon still stands, but by a different mechanism.** `main`
  does not scope the sync per milestone, as the old reason claimed; it reads
  master's `PROGRESS.md` before the wipe and union-merges it back via
  `mergeProgressState`. The branch's 251-line test file and its entire
  alternative implementation are named, as is the residual gap `main` still has
  — `syncFeatureStateAfterMerge` wipes every file in the feature dir except
  `PROGRESS.md`. Row reason replaced. *(This entry was omitted at the time and is
  added retrospectively by `P0-M1-FIX-8`, 2026-08-18.)*
- 2026-08-18 — §*Abandon — content verified on `main`* preamble corrected
  (`throughput` P0-M1-FIX-8). Its closing caveat still said the rows below
  `worktree-git-excludes` were unaudited, which `P0-M1-FIX-5` had already made
  false. The two rows re-audited to the stated bar are now named, and so are the
  two that still are not. No verdict was re-opened.
