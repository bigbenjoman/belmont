All spot-checks confirm. Report follows.

---

# ROUND-3 REVIEW REPORT — Belmont proposals 0003 / 0004 / 0005 (rev 3)

## 1. DANGLING DEPENDENCIES ON 0006

0006 was deliberately narrowed (commit 50d7685, "Make 0006 fully standalone") to just the awk fix + version-aware `--check`. The agents `references/` copy branch was **deleted from 0006 and never re-homed**. PR #21 is **OPEN, unmerged** — the only open PR in the repo.

| Dependent site | What it expects | Reality |
|---|---|---|
| 0003 DoD :288 `test -f plugin/agents/references/design-no-figma.md` "(needs 0006's agents references/ branch)" | 0006 copies `agents/belmont/references/` into `plugin/agents/references/` | No such branch exists in PR #21 or on main. Agent loop is a flat `*.md` glob (generate-plugin.sh:78). DoD :288 and :285 (`--check` passes) are **mutually exclusive** — verified empirically: committing the file yields `EXTRA: ... (should be removed)`, exit 1; and `rm -rf "$PLUGIN_DIR"` (:35) deletes it on every regeneration. |
| 0003 :338 (v2→v3 changelog) | Same assertion recorded as an improvement | Same dangling. |
| 0006 :4 Size line | "~15 lines of awk, **one new copy loop**, one `--check` argument" | Still advertises the deleted copy loop (verified just now). §4 scope, §8 DoD, and the actual diff contain no copy loop. |
| 0003 :285 / 0005 :199 "plugin.json STALE is the only legitimate diff" | Pre-0006 `--check` behaviour | Post-0006, `--check` ignores the version field and exits 0 on clean main; a STALE plugin.json would mean real drift. 0003's "restore from main before committing" would actively revert a re-stamped version in a tracked release artifact. |
| README :24 "0003 next" | 0006 already landed | #21 unmerged; branching 0003 off main today regenerates plugin agents with the **broken** generator (4 files → 0 bytes). |

50d7685's own commit message says the copy branch "rides with the change that needs it" — i.e. **0003 must carry it**. 0003 does not: its §5 scope table omits `scripts/generate-plugin.sh` entirely, and 0003:5 says "No Go changes" without mentioning shell.

## 2. BLOCKERS (must fix before implementation)

### 0003 — four blockers
1. **DoD :288 unsatisfiable / contradicts :285** (above). Fix: move the ~6-line agents-`references/` copy branch (mirroring the skills loop) into 0003's own scope; add `scripts/generate-plugin.sh` to the §5 table; delete the "needs 0006's branch" parenthetical. Alternative: drop the plugin references dir and inline Mode B detail into design-agent.md.
2. **Write-rule fix incomplete and the DoD forbids completing it.** §5 amends design-agent.md `:137`/`:260` but leaves `## FORBIDDEN ACTIONS (HARD RULES)` `:5-15` (`:8` "write to ANY file except the MILESTONE file", `:15` "ONLY writable output is the ## Design Specifications section") — read first, and stronger. Worse: DoD :274 confines the permitted diff so an implementer who fixes `:5-15` **fails the DoD**. Fix: add `:5-15` to the write-rule row (sole permitted extra write = `{base}/DESIGN-CONTRACT-<MilestoneID>.md`) and widen DoD :274 accordingly.
3. **(Same root as #1, second DoD site)** — the `test -f` row vs `--check` row conflict is independently fatal even if the parenthetical is deleted; the generator change must land somewhere.
4. **Contract file not durable in parallel/resume mode.** `copyBelmontStateToWorktree` (main.go:9980) wipes the feature dir on every `[r]`-resume, preserving only STEERING.md; `syncFeatureStateAfterMerge` (:10160) lets a sibling milestone's merge clobber `DESIGN-CONTRACT-M1.md` at default `--max-parallel=5`. §4.1's "durable" claim is false in exactly the modes the redesign targets, and §5's "no Go change" bars the clean fix. Fix options: (A) Go — preserve `DESIGN-CONTRACT-*.md` in both functions (requires lifting §5's scope line); (B) prose-only — write the contract to a git-tracked path outside `.belmont/` (worktree git exclusion covers only `.belmont/`; `git add -A` carries it through merges). Add a smoke step: two milestones, one wave, `--max-parallel=2`, assert both contracts survive; plus a resume case.

### 0004 — two blockers
1. **Tier 2 DoD command fails as written.** `executeLoopAction` has no timeout/watchdog; Go's default test `-timeout` is 10m for the whole binary; DoD :211 mandates verbatim `BELMONT_EVAL_LIVE=1 go test -tags eval ./cmd/belmont` at N≥3, twice. It panics mid-run, and on panic `killProcessGroup` never fires so the live `claude -p` child is orphaned, still burning budget. Fix: `-timeout 0` (or explicit budget), name which fixtures run live, state wall-clock/cost.
2. **Smoke Steps 0/2 corrupt tracked `plugin/`, and the post-0006 `--check` is version-blind to it.** `build.sh 0.0.0-smoke` rewrites `plugin/.claude-plugin/plugin.json`; 0006's `--check` greps out `"version"` on both sides, so both mechanics DoD boxes go green over a committed `"0.0.0-smoke"`/`"dev"`. Exposure is the marketplace git surface (marketplace.json `"source": "./plugin"`) between merge and next release — not the release tarball (release.yml regenerates from tag). Fix: DoD → `./scripts/generate-plugin.sh 0.10.14 && git diff --exit-code plugin/`; add `git checkout -- plugin/` after Steps 0 and 2.

### 0005 — one blocker
1. **Smoke Step 7a not executable.** `belmont auto --tool codex` with no `--feature` is rejected by runAutoCmd's unconditional validation (main.go:5311-5312) before any shell-out. The Codex auto-mode leg — the point of splitting 7a/7b — never runs. Fix: `belmont auto --feature <slug> --from M1 --to M1 --tool codex`.

## 3. MAJOR (should fix), by spec

**0004**
- §5 :116 "only open PRs are #10–#13… #10/#11 sit on `_src/next.md`" — **empirically false**; all four CLOSED unmerged 2026-08-06T06:43Z. The headline v3 fix records an overlap that cannot occur, and §5 disagrees with DoD :219. Rewrite against measured state: only open PR is #21, disjoint from 0004's scope and its declared dependency anyway.

**0005**
- §4.3/§12/DoD :193 assert `cmd/belmont/tools.go` **already exists**. It does not and never has — any branch, any commit (`git log --all` empty). A fabricated empirical claim introduced in v2 as a "correction," driving the `toolexec.go` name and a vacuous DoD checkbox, in a spec whose premise is "verified against controls." Delete all three references; restate collision check against the 14 real files.
- §4.3 boundary table omits `reverify` (runReverifyCmd, 288 lines) and monorepo/workspace detection outright; recover/reconciliation/runAutoMultiFeature have only inferable homes; §5 hard-codes "commits 1–10." Not fatal (the table invites re-cutting; Contents column assigns by domain), but underdetermined. Fix: explicit rows + "one commit per §4.3 row" + a placement rule for unnamed declarations.
- Smoke Step 3 hardening (downgraded from blocker in verification): the guard line has `ESC[0m` between `]` and ` reverted`, so any scripted literal grep fails on a working guard; AGENTS.md requires copy-paste-ready smoke steps. Specify ANSI-strip + `grep -F`; also `grep -c 'M99'` exits 1 on success under `set -e` — use `|| true`.

## 4. CROSS-CUTTING (judged; weak ones discarded)

**Kept — verified or corroborated:**
1. **README/series contradiction on 0006** (major). README:18 calls 0006 "Standalone bug fix — unrelated to the series" (verified just now) while 0003:7, 0004:7, 0005:7 each declare a hard dependency on it; README's Sequencing never mentions 0006, and README:34 contradicts its own index row. Fix: "0006 (PR #21) → 0003 → 0004 → 0005," one line per downstream need.
2. **0006 unshipped; "until 0006 lands" hedges live** (major). "0003 next" is premature; write both hedge sites in 0004 to read correctly pre- and post-merge.
3. **Stale PR states in all four docs** (major). #10–#13 closed; 0003:203 warns of a conflict with dead #11; README:9 reserves 0001/0002 against now-closed RFCs. Rewrite all four sites.
4. **The one real conflict is unnamed** (major). PR #21 rewrites all six `plugin/agents/*.md` (+1,511/−36); 0003 regenerates three of them and its conflict section points at the dead PR instead. Fix: branch 0003 off #21; resolve plugin conflicts by regeneration only.
5. **plugin.json STALE parentheticals in 0003:285 / 0005:199** (major) — wrong post-0006, and 0003's "restore from main" is actively harmful. Delete; invert the caveat.
6. **Model-tier doctrine** (major). Verified just now: 0004:125 says #18 removed pins "precisely because downgrading is a false economy"; 0006:48 says the same commit was correct because "frontmatter genuinely is not read at runtime" — two contradictory motives for one merged commit, and 0006's mechanical one is right. The 57%/77% figure is uncited in both specs, yet 0003:170 would ship "never downgrade design-agent" into a user-facing reference and 0003:209 into AGENTS.md invariants, and 0004:125 declares default tiering "deferred permanently" on its strength. Fix: cite or drop the figure; align 0004 to 0006's mechanical rationale; split the policy edit out of 0003's DoD into an explicit yes/no ask.
7. **0003 relaxes the design agent's single-writer guardrail as a scope-table row** (major). Interacts with 0003 blocker #2; the guards (`runScopeGuard`/`runEvidenceCheck`) only watch PROGRESS.md, so the new writable path is unguarded. Promote to an explicit §2 decision + Risks row + `Don't re-do` line (rejected alternative: orchestrator writes the contract, preserving single-writer).
8. **0005 adopts staticcheck (blocking CI gate) and a permanent `scripts/declsum` tool without asking** (major). AGENTS.md:81 states "There is no linter configured" (verified) — that's a project-policy change smuggled in as refactor detail. Add a "Decisions this PR needs" block with fallbacks (advisory-only lint; declsum built in temp dir, not committed).
9. **Naming schemes** (minor, downgraded from major). H1s say "PR 1/2/3," which collide with real merged/closed PRs #1–#3; two sentences mix both schemes. Standardise on 0003/0004/0005 to match 0006's H1.
10. **0006:4 Size line still advertises the deleted copy loop** (minor, verified live). One-word edit — worth doing before Blake reads PR #21.
11. **KNOWLEDGE.md overlap claims wrong** (minor). Only 0003 (+0004 partially) touch the cross-cutting table; 0005/0006 amend entries only. Correct README:30 and 0005:231.
12. **Unstated live-run cost** (minor but material given post-nerf limits). 0004's Tier 2 gate is ≥6 live runs plus full `belmont auto` smoke runs; 0003 needs live Figma MCP for 2 of 7 steps with no fallback. Add a cost header + minimum defensible N per spec.

**Discarded as weak:** the graph-engineering-rationale objection (§3 sections citing Isenberg/Cherny). It's a persuasion-style concern, not a correctness defect; the specs' §4s largely stand without it. Worth a one-line "framing, not evidence" disclaimer at most — not tracking as a finding.

## 5. BUILD ORDER

**0006 (PR #21) first — merge it, it is materially sound** (one-line Size fix aside). Then **0005 → 0004 → 0003**, inverting the README's order:

- **0005 is safest first of the three.** Its blocker is a one-line smoke-command fix; its majors are documentation deletions (tools.go fabrication) and table extensions. It's a pure-move refactor with mechanical controls, touches no agent prose, and doesn't depend on the design-contract or eval semantics. Landing it first also gives 0004 its promised `ci.yml` home instead of a forward reference.
- **0004 second.** Fixes are bounded (timeout flag, plugin cleanup, stale-PR paragraph rewrite) but its baseline should be measured on the final code layout — and its own §2 argument that 0003 must precede the baseline is now moot if the baseline is taken after 0003, which brings us to:
- **0003 last, and it needs a rev 4.** Four blockers, two of which (write-rule contradiction locked in by its own DoD; contract durability in parallel mode) are design decisions, not edits. The sequencing note in 0003:7 ("PR 2's baseline must include Mode B") should be re-decided explicitly — either 0003 before 0004's baseline as written, or 0004 measures pre-Mode-B and re-baselines. That is Blake's call, not the implementer's.

## 6. GO / NO-GO

**0003 — NO-GO.** Four verified blockers, two requiring decisions (single-writer relaxation scope; parallel-mode durability). Minimum edit set for GO: (a) pull the agents-`references/` copy branch into its own §5 scope and fix DoD :285/:288; (b) add design-agent.md `:5-15` to the write-rule row and widen DoD :274; (c) pick durability option A (Go preserve, lift §5's no-Go-change line) or B (git-tracked contract path) and add the parallel/resume smoke step; (d) rewrite §6 conflict analysis against PR #21. That is a rev 4, not a patch.

**0004 — NO-GO as written, but one edit-session away.** Minimum edits: add `-timeout 0` + fixture list + cost line to the Tier 2 gate; change the plugin DoD to `generate-plugin.sh 0.10.14 && git diff --exit-code plugin/` and add `git checkout -- plugin/` after smoke Steps 0/2; replace §5:116 and DoD :219 with the measured PR state (only #21 open); align :125's #18 rationale with 0006:48. Otherwise materially sound — the eval-harness design itself survived three rounds without a structural finding.

**0005 — NO-GO as written, closest to GO.** Minimum edits: fix Step 7a's command; delete the three `tools.go` fabrication sites and restate the collision DoD against real files; add rows for reverify + monorepo detection and change §5/§9 to "one commit per §4.3 row"; add the staticcheck/declsum decision block. The split plan itself is sound and the mechanical controls (declsum before/after) are the right idea — the defects are in the spec's self-description, not the refactor.

**Convergence note:** rounds 1–3 have monotonically narrowed. 0004 and 0005's remaining defects are edits an author fixes in an hour; 0003 is the only spec with open design questions. Nothing in this round suggests a fourth adversarial pass would find new structure — the remaining risk is execution, covered by the (now-corrected) smoke tests.