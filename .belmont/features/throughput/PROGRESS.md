# Progress: Belmont Throughput

## PRD Reference
.belmont/features/throughput/PRD.md

## Milestones

### M1: Toolchain, atomic writes & baseline
*Bump the toolchain and clear the branch backlog, fix the non-atomic state write, then stand up the measurement the whole feature is judged against. Everything else depends on this. Six tasks, deliberately at the soft ceiling — see TECH_PLAN §Implementation Order for why the two prerequisites live here rather than in a milestone of their own.*
- [v] P0-12: Bring the toolchain onto a supported version
- [v] P0-13: Triage the unmerged branch backlog
- [!] P0-13a: Bring the default branch up to date — BLOCKED, awaiting the repository owner
  - P0-13's acceptance clause "the default branch matches the working state" is deferred, not done. `origin/main` is 157 commits behind local `main`, so satisfying it means publishing 157 commits to a public GitHub fork (`bigbenjoman/belmont`).
  - **Asked of**: Ben Lavender, as the repository owner. **The decision**: whether to push local `main` to `origin`, and whether that happens before or after the fork is re-synced with `upstream` (`blake-simpson/belmont`).
  - Not an implementation-agent decision: publication is irreversible and public. No push, remote-branch deletion, merge, rebase or PR was performed by P0-13 — every verdict in `docs/branch-triage.md` is a recommendation.
  - Unblocks nothing in M1. It does not gate M2: `docs/branch-triage.md` records both the merge verdict for `origin/docs/pr-proposals` and the `git show` command that reads proposal 0004 rev 5 off the remote branch without it.
- [v] P0-1: Atomic state writes
- [v] P0-2: Token and wall-clock instrumentation
- [!] P0-3: Capture the pre-change baseline — read-path half verified and reproduces exactly; the tokens/wall-clock half is recorded NOT CAPTURED in BASELINE.md. Not promoted to `[v]`: the criterion the task exists to satisfy is unmet, and a task that reads verified while its stated Solution is undelivered is the exact failure `[x]` vs `[v]` exists to prevent. Resolution path decided 2026-08-18 — see `[!] P0-3a`.
- [!] P0-3a: Capture the per-milestone token and wall-clock half of the baseline — BLOCKED, needs instrumented runs
  - `BASELINE.md` + `baseline.json` record the **read-path** half in full for both repos (per-feature register bytes, `status` text and JSON costs, the six registers over the 100 KB ceiling), plus the Belmont version, repo commit and Go toolchain. The **tokens/wall-clock per verified milestone** half is recorded as `NOT CAPTURED`.
  - **Why**: the orchestrating binary is pinned to Homebrew v0.11.0, which carries no instrumentation — P0-2's code exists only in this working tree — and neither `~/repo-3/.belmont/metrics/` nor `~/repo-4/.belmont/metrics/` exists (verified, not assumed). No figure was invented to fill the gap; the PRD is explicit that instrumentation reports nothing rather than guessing, and a baseline is the last place to break that.
  - **Asked of**: Ben Lavender. **The decision**: whether to run at least one milestone to `[v]` in each repo using a build of this tree, which is live agent work costing real tokens against two production repositories.
  - `BASELINE.md` §How to capture it holds the exact commands. Until then M11/P3-3 compares the read-path half and reports the cost half as unevidenced.
- [v] P0-4: Extraction census across all feature directories
- [v] P0-4a: Wave structure revisited — RESOLVED 2026-08-18, no restructure. **Decision (Ben Lavender)**: leave the graph at four waves. The PRD's rule said M4 becomes a prerequisite of M3, but the consequence that rule exists to guarantee is already satisfied — the migration needing both is P3-1, inside M11, and M11 depends on M4 which depends on M3, so both land before any pathological register is migrated. Restructuring was measured and costs a wave: W1=M1 · W2=M2,M4,M6,M7,M9,M10 · W3=M3,M8 · W4=M5 · W5=M11 — five waves and 2.20× theoretical parallelism, against four and 2.75× today, buying an ordering already held transitively. **Carried forward to M3**: extraction alone does not meet the 25,000-token read budget for the five registers listed in `CENSUS.md`; M4's slice is what closes that, and M3 should say so rather than implying the split suffices.
  - The PRD's open question: *"whether the extraction census in M1 finds any feature whose index alone exceeds 25,000 tokens. If one does, the read-path work in M4 stops being an optimisation and becomes a prerequisite for M3, and the wave structure changes."* **Five do** (`CENSUS.md`), against a pre-estimate of three.
  - The pre-estimate was wrong in mechanism as well as magnitude: it assumed size implies indented narrative. **Three of the five gain essentially nothing from extraction — 0%, 0% and 7.6%.** Two of those three carry zero indented lines; the third, `repo-3/feat-015`, is 805,397 B of task *head* lines with a single 11,542-character task line, so extraction moves 7.6% of it. Per-register counts and their derivation: `CENSUS.md` §Why the estimate was wrong — canonical, not restated here. *(Corrected by `P0-M1-FIX-6`: an earlier version of this line counted those two zero-detail registers as three — retraction in `CENSUS.md`. The escalation below is unchanged: five registers still exceed the ceiling after extraction.)*
  - **Asked of**: Ben Lavender / a tech-planning session. **The decision**: whether M4 moves ahead of M3 and the wave structure is re-cut. Only the tech-planning stage may create or restructure milestones (PRD §Constraints), so this is recorded rather than applied.
  - Two knock-ons recorded in `CENSUS.md`: M5/P1-8's 400-char task-line limit is load-bearing rather than defensive (it is the only mechanism that would have prevented `feat-015`), and P3-1's migration list is seven features, not four.

- [v] P0-M1-FIX-1: Metrics are written to the worktree root and destroyed with it, so parallel-wave mode records nothing. `recordPhaseMetrics` (`auto_loop.go:614-628`) passes `cfg.Root` to `appendMetricsRecord`, but `auto_parallel.go:74` and `:474` set `mCfg.Root = wtPath` before calling `runLoop`; `.belmont/metrics/` is gitignored, `syncFeatureStateAfterMerge` copies only `PROGRESS.md` back, and `removeWorktree` deletes the tree. `autocmd.go:284` routes to `runAutoParallel` whenever any milestone declares `(depends: …)` — which this feature's own PROGRESS.md does for M2–M11 — so the instrumentation the whole feature is judged by produces nothing in the mode the feature will actually run in, and M11/P3-3 would have no data. Add `loopConfig.MetricsRoot` defaulting to `Root`, set it to the originating root at both `auto_parallel.go` sites, propagate `RunID` from the parent so one wave is one run, and add a test pinning `MetricsRoot != Root` in worktree mode. Breaks P0-2's acceptance criterion "two consecutive runs produce comparable records".
- [v] P0-M1-FIX-2: The census never walked `repo-2`, so P0-4's acceptance criterion "every feature directory in all five repos" is unmet — and the report uses the incomplete walk to publish a correction to the PRD that is itself wrong. `census.json` `roots` covers repo-3, repo-4, repo-5, repo-1 and repos/belmont; the PRD's five are repo-1, repo-5, repo-4, **repo-2**, repo-3. `repo-2` is at `/Users/benlavender/repo-2/.belmont/` with 31 feature directories and 18 further live registers. Re-run over the PRD's exact five: **168 dirs / 82 live / 32.6% estate reduction**, not 138/65/34.8%. Then **retract** CENSUS.md's claim that *"the PRD's phrase 'the other 83' does not match disk"* — the PRD's "82 active" reproduces exactly once repo-2 is included. The PRD was right. The five-over-threshold conclusion is unaffected and `[!] P0-4a` still stands.
- [v] P0-M1-FIX-3: `docs/branch-triage.md:99` abandons `origin/feat/maintenance-ci` on the claim that both apparent leftovers are already on `main`. Only one is (`7a9b7b56`). Commit **`1129cf84` is not on `main`** — 23 lines adding the "Gotcha — `--max-parallel` does not by itself produce worktrees" section to `knowledge/auto-mode/parallel-wave-orchestration.md`; `grep` for the heading returns nothing and `git diff --stat main...branch` shows the file differing by 26 lines. The triage also mis-describes that leftover as a `main.go`→`autocmd.go` repointing, which is a different change. Acting on the abandon verdict would delete documented knowledge that a smoke fixture without `(depends: …)` proves nothing about the parallel path — directly relevant to M6 and M10. Cherry-pick `1129cf84`, then re-verdict.
- [v] P0-M1-FIX-4: `summariseMetrics` (`metrics.go:296-341`) sums `Input` across tools whose `input_tokens` mean different things — claude's **excludes** `cache_read_input_tokens`, codex's (OpenAI lineage) **includes** `cached_input_tokens`, as the file's own comment notes. Any cross-tool summary, or a P3-3 comparison whose two halves used different tools, yields a plausible wrong number in the baseline every later milestone is scored against. Either normalise on ingest or break the summary out per tool and refuse to aggregate across tools; record the semantics in `metrics.go`'s header either way. Do not resolve by assuming — confirm codex's inclusion semantics against a live run.
- [v] P0-M1-FIX-5: `docs/branch-triage.md:101` abandons `origin/fix/wave-merge-state-loss` because *"`main`'s `worktree.go` already scopes the sync per milestone"*. It does not — `syncFeatureStateAfterMerge(mainRoot, wtPath, slug)` takes no milestoneID, still does `os.RemoveAll(dstFeature)` + `copyDir`, and there is no `resolveMilestoneProgress`. The defect class *is* addressed on `main`, but by a different mechanism: read master's PROGRESS.md before the wipe, then union-merge via `mergeProgressState` (`worktree.go:370-419`). The verdict is defensible; the stated reason is wrong, and "only the branch's own test filename is absent" understates a 251-line test file and an alternative implementation. Correct the reason.
- [v] P0-M1-FIX-6: *"Three of the five contain no indented lines at all"* is wrong — only **two** do (`feat-058` 0, `feat-031` 0). `repo-3/feat-015` has 167 indented lines / 84,593 B, and every occurrence contradicts itself in the next clause ("only 83,500 are indented"). The intended claim is that three of the five gain essentially nothing from extraction (0%, 0%, 7.6%). Fix all four copies — `CENSUS.md:48`, `NOTES.md:43`, `MILESTONE.md:566`, `PROGRESS.md:28`. The PROGRESS.md copy sits directly beneath the human-gated `[!] P0-4a`, so the owner making the wave-restructuring call reads it as fact.
- [v] P0-M1-FIX-7: `CENSUS.md` §Reproducing this does not reproduce. `--roots ~/repo-4,~/repo-5,…` — the shell expands `~` only at word start, so every path after the first comma stays literal and resolves under CWD. The documented command returns **43 live registers instead of 65**, silently, because `runCensus` swallows an unreadable root with `os.IsNotExist → continue`. Both reviewers hit this independently. Use absolute paths (or `$HOME`) in the doc, and make `runCensus` report a root it could not read rather than skipping it.
- [!] P0-M1-FIX-8: Reconcile the three documentation inconsistencies the fix batch left behind — DELIVERED but NOT PROMOTED. Verification 2026-08-18 returned PARTIAL: all five acceptance criteria pass, every gate green, no Critical. It proved the archive annotation is byte-identical (192 added, 0 deleted; sha256 of line 460 unchanged) and independently re-derived the (c) decomposition, confirming `moved = indented_in_body + blank_in_body` exhaustively across three registers. **The objection**: the replacement sentence in `CENSUS.md` §Why the estimate was wrong introduced a *new* claim asserted beyond its evidence — "usually the larger term, and why detail moved normally comes in below the indented total". Measured over all 82 live registers: **75 (91.5%) have moved == indented exactly**; 5 come in below (`feat-020`, `feat-016`, `feat-071`, `feat-075`, `feat-015`); 2 come in above (`feat-063` +1 B, `feat-070` +44 B). The normal case is equality, not "below". **The fix is one sentence** — replace that clause with the measured distribution above, as an annotation in the FIX-6/FIX-8 blockquote style, changing nothing else. No re-derivation needed; the measurement is recorded here. **Not attempted** because the M1 circuit breaker fired at two fix rounds this session, and any further edit needs its own verification pass to reach `[v]` — which is the thrash the breaker exists to prevent. Changes no figure, table or conclusion; `[!] P0-4a` is unaffected.

### M2: The verify read (depends: M1)
*Highest-scoring item in the backlog. Stop reading the same archives three times and stop recommending the largest read path.*
- [ ] P0-5: Read archived milestone records once
- [ ] P0-6: Stop recommending the largest read path
- [ ] P0-7: Give verification a coordination document

### M3: Split the register safely (depends: M1)
*Separate narrative from register — 62% of the worst file is indented detail already separable by indentation, so an extraction rather than a re-modelling — and close the merge holes that splitting would otherwise widen.*
- [ ] P1-1: Separate narrative detail from the register
- [ ] P1-2: Validate index and detail integrity
- [ ] P1-3: A durable grammar for detail records
- [ ] P1-14: Refuse a merge that loses state
- [ ] P1-15: Close the verified-state laundering path

### M4: The read path (depends: M3)
*Serve one milestone's slice on demand and make it the mandatory path. This is where most of the measured saving is realised.*
- [ ] P1-4: Serve a single milestone's slice on demand
- [ ] P1-5: Make the cheap path the mandatory one

### M5: Growth ceilings (depends: M3)
*Without a write-time ceiling an extracted register regrows within a quarter. Warnings alone are documented as insufficient.*
- [ ] P1-6: Configurable size ceilings
- [ ] P1-7: Refuse to start over the ceiling
- [ ] P1-8: Reject oversized writes at the source

### M6: Bound the loop (depends: M1)
*58% of a failing run's tokens are spent after the failure was detectable. Largest reliable saving available, and none of it costs a quality check.*
- [ ] P0-8: Detect failure before the budget is spent
- [ ] P0-9: Cap the verify-and-fix cycle at two rounds
- [ ] P0-10: Start every retry from clean context
- [ ] P0-11: Gate the expensive agents behind deterministic checks

### M7: Skill payload (depends: M1)
*Stop shipping every host's prose to every host. Multi-host support is retained; only the per-host build contents change.*
- [ ] P1-9: Stop shipping every skill's shared prose to every skill
- [ ] P1-10: Prove the skill payload shrank

### M8: Cacheable dispatch (depends: M7)
*Move 91 KB of agent definitions into the cacheable prefix. Goes through the eval harness, and reverts on a negative result.*
- [ ] P1-11: Put agent definitions where the cache can hold them
- [-] P1-12: Stop invalidating the cache with steering text — withdrawn during tech-plan: measured identical cache figures prefixed and suffixed (proposal 0004 rev 2)
- [ ] P1-13: Verify the caching change against the harness

### M9: Master file & journal (depends: M1)
*The master file is 420 KB of hand-appended narrative nobody authors. Generate it; move activity to an append-only record that merges by union.*
- [ ] P2-1: Generate the master progress file
- [ ] P2-2: Move the activity narrative to an append-only record
- [ ] P2-3: Retire hand-edited session history

### M10: Scheduler correctness (depends: M1)
*Three defects in the dependency layer. Not a throughput play — a correctness play that any parallel work would otherwise amplify.*
- [ ] P2-4: Make a dangling dependency an error
- [ ] P2-5: Correct the documented milestone syntax
- [ ] P2-6: Cap concurrency and say when parallelism is unavailable

### M11: Roll out & re-measure (depends: M2, M4, M5, M6, M8, M9, M10)
*Migrate the pathological minority, bring all five repos onto one build, and report against the M1 baseline including anything that missed.*
- [ ] P3-1: Migrate the pathological features
- [ ] P3-2: Bring all five repos onto one build
- [ ] P3-3: Re-measure and report against the baseline

> **Dependency syntax**: `(depends: M1)` after the milestone name declares dependencies. Independent milestones run in parallel via git worktrees under `belmont auto`.
>
> **Wave structure**: W1 = M1 · W2 = M2, M3, M6, M7, M9, M10 · W3 = M4, M5, M8 · W4 = M11. Four waves for eleven milestones — 2.75× theoretical parallelism. Expect roughly 1.4× in practice on a single machine; see PRD §Research Notes on why local parallelism does not scale with slot count.
>
> **Task states**: `[ ]` todo, `[>]` in_progress, `[x]` done, `[v]` verified, `[!]` blocked, `[-]` withdrawn. Milestone status is computed from its tasks — do not add status emoji to milestone headers.
>
> **Model tiers**: Opus for implementation, verification and planning; Haiku only for genuinely mechanical work; skip the Sonnet tier. Per PRD §Clarifications — measured rework rates make the middle tier a false economy.
>
> **Design agent**: skip on every milestone. No task in this feature has design input.

## Session History

| Date | Action | Details |
|------|--------|---------|
| 2026-08-18 | Tech plan | Six interview rounds. Self-hosting in the `bigbenjoman/belmont` fork at `~/repos/belmont` confirmed; proposal 0004 rev 5 absorbed into M2 and its lazy-Setup half recorded as superseded by M4's slice; P1-12 withdrawn on an in-repo measurement; P0-12 (Go toolchain) and P0-13 (branch triage) added to M1. Detail tier, slice command, config home, thresholds, journal format, state precedence, gate set and slot cap all specified in TECH_PLAN.md. |
| 2026-08-18 | Created | Product plan session. Eleven milestones, 35 tasks, four dependency waves. (37 now — P0-12 and P0-13 were added during tech-plan.) Preceded by measurement of repo-3 and repo-4 state files and four research passes (graph engineering practice, arXiv literature, GitHub ecosystem and rival spec-driven frameworks, milestone-tracker architecture). |
| 2026-08-18 | Plan review | `/belmont:review-plans`. Task-count history corrected 33→35 (37 now, less P0-12/P0-13 added at tech-plan). P0-12 and P0-13 task lines aligned with their PRD titles. **P0-13 scope made explicit**: `origin/docs/pr-proposals` must carry its own verdict, because verification found no `docs/proposals/` directory on the fork's `main` at `17897aa2` and no mention of the convention in `AGENTS.md` or `CONTRIBUTING.md` — M2's specification (proposal 0004 rev 5) and the homes for the three proposals this feature owes are all off-mainline. Also recorded: `origin/proposal/0001-quick-mode` and `origin/proposal/0002-prd-hygiene` carry `framework-evaluation`'s P6-1/P6-2 specs. Verified accurate and unchanged: the 35-branch count, `go.mod` at `go 1.21`, the six agent files' absent YAML frontmatter, the symlinked `.claude/commands/belmont/`, `eval_harness_test.go` present on `main`, the wave structure against the declared dependencies, and `belmont validate` clean. |
| 2026-08-18 | Prerequisites | Two further findings from the review's Layer 4 measurement, both fixed. **(a) The user-level install was stale and it mattered.** `~/.agents/skills/belmont/` was last written 2026-05-04 and lacked `loop`, `repair` and `codex-plan-apply` — v0.10.x against a v0.11.0 orchestrator. Because the fork gitignores `.claude/`, that install is the sole resolver of `/belmont:*` inside worktrees, so all six wave-2 milestones would have run 3½-month-old prose (including the `verify.md` M2 rewrites) and the M1 baseline would not have been comparable with the M11 re-measurement. Refreshed, and recorded as prerequisite 0 with an explicit do-not-repeat-until-M11 note. **(b) `_research/` is 436 KB of the feature's 572 KB and the fork has no ignore rule for it**; added as prerequisite 2 so it lands with the move rather than being committed. Prerequisites renumbered 0–5. |

## Decisions Log

- **Repository**: work lands in `bigbenjoman/belmont` as a tracking fork of `blake-simpson/belmont`, upstream remote retained, file formats kept backward-compatible. Blake leaves Studia at the end of this week; upstream shipped v0.11.0 on 2026-08-17 and remains active.
- **Host support**: multi-host retained, host-specific prose made build-time conditional. Preserves opencode for the Oct 2026 local-LLM move while unlocking Claude-Code-specific caching.
- **Success bar**: tokens **and** wall-clock per *verified* milestone, against a baseline captured in M1. `[v]`, not `[x]`, is the unit.
- **Goal shape**: same spend, more shipped. Token efficiency buys throughput, not savings.
- **Sequencing**: cheapest-first by ICE score, with one deviation — instrumentation sits in M1 despite a mid-table score, because the success bar cannot be an acceptance criterion on later milestones if nothing measures it.
- **Model tiers**: Opus default, Haiku for mechanical work, Sonnet tier skipped. Supersedes the master PRD's previous "Sonnet is the current default". Basis: measured rework ~37% (Sonnet 4.6) against ~19% (Opus 4.7), plus routing research showing downgrades are catastrophic on the one or two nodes carrying real difficulty. Input cost is cut through structure, never by downgrading the model that determines rework.
- **Verify cap**: escalate after 2 failed rounds.
- **Atomic writes**: fixed as the first task of M1 rather than shipped as a separate PR, keeping the plan self-contained.
- **Migration**: opt-in per feature; bulk-run only on the pathological minority identified by the M1 census. Median feature is 14 KB and gains nothing.
- **Rollout**: all five repos, proven on repo-3 and repo-4 first.
- **Metrics**: local-only and gitignored. Bookkeeping already accounts for 41–46% of commits in these repos.
- **Feature-level register stays authored, master file becomes generated.** Commit-history measurement showed regenerating the feature register produces large spurious diffs; the master file is 420 KB of narrative nobody authors as a human artefact.
- **Register line grammar is not changing.** It is parsed in seventeen places across three dialects and changing it buys nothing measurable. Only the detail tier gets a new grammar.
- **Scored out of scope on evidence**: code-graph index (peer-reviewed result is +2.3pp resolve for 12–38% more spend; two-hop retrieval scores below no graph at all), speculative execution (rollback trigger collapses to AUC ≈ 0.5 on code and math), blast-radius verification (depends on the code graph; a wrong radius silently skips a check), rolling wave scheduler (level-by-level is already within 2.6–11.2% of the critical-path bound, and scheduling discipline is irrelevant when slot-limited), task-level dependency metadata (the milestone is the worktree boundary; a task DAG needs a new isolation unit).
- **Parallelism expectations revised down during planning.** Belmont's wave scheduler is correct and already ahead of the field. The available speedup is real but bounded by memory bandwidth on a single machine, not by graph structure — which is why this feature spends its effort on token structure rather than on widening fan-out.

### Added during tech-plan, 2026-08-18

- **Work location**: self-host in the fork at `/Users/benlavender/repos/belmont` (`origin=bigbenjoman`, `upstream=blake-simpson`, local `main` at `17897aa2` = upstream v0.11.0). The feature directory moves there before M1; the research workspace keeps the master files and `framework-evaluation`. Contained by pinning the orchestrating binary to the Homebrew v0.11.0 build — see TECH_PLAN §Prerequisites.
- **Prior art**: proposal `0004-context-budget-with-evidence.md` rev 5 is M2's specification for its conditional-archive and read-once halves. Its lazy-Setup optimisation is **superseded, not rejected** — M4's `belmont slice` replaces that first read two waves later.
- **P1-12 withdrawn** `[-]`. The mechanism is real (steering prepended at `auto_loop.go:259` and `:404`); the saving is not. 0004 rev 2 measured prefixed against suffixed steering on `claude -p` and got identical cache figures both ways (13,233 creation / 21,689 read) — Belmont assembles a ~549-byte user message and the cache breakpoint sits at its end.
- **P0-12 and P0-13 added to M1.** Go toolchain bump sequenced *before* P0-2/P0-3 so the baseline is captured on the toolchain everything after is built with. Branch triage before any milestone touches shared files — five of the 35 branches sit on ground M3/M4/M6 rewrite.
- **Detail tier**: `{base}/details/M<N>.md`, one file per milestone, `## <task-id>` blocks, supersession as a one-line note with the old text left to git. Absent directory = today's behaviour.
- **Read path**: new `belmont slice --feature X --milestone M3` subcommand, text by default, nothing written to disk. Explicitly not a status cache — stated in `knowledge/cross-cutting/context-budget.md`.
- **Config home**: project-level `.belmont/config.json` (stdlib JSON), not YAML — the binary has zero dependencies and no `go.sum`.
- **Thresholds**: warn 50 KB, error 100 KB, task lines rejected above 400 chars. 100 KB is 25,000 tokens at 4 bytes/token, so the error threshold *is* the success criterion. Measured against all 139 feature registers across the five repos.
- **Instrumentation**: parse each tool's own JSON usage; record `null` with a stated reason where the tool cannot report (copilot, pi, opencode). Never estimate.
- **Host builds**: per-host generated files replace the symlinks at `.claude/commands/belmont/`, `.opencode/command/belmont/`, `.cursor/rules/belmont/`, `.windsurf/rules/belmont/`. P1-10 measures per-skill, per-host built size — not aggregate, which is disk rather than context.
- **P1-11** opens with a spike: the six agent files carry no YAML frontmatter, which is why nothing registers and the prose falls back to `general-purpose`. If registration does not preserve tool access, `models.yaml` overrides and the FORBIDDEN ACTIONS blocks, P1-11 is withdrawn rather than worked around.
- **State precedence**: single total order `[-] > [!] > [v] > [x] > [>] > [ ]`, applied uniformly so merging is order-independent. Blocked above verified is what makes "a blocker is never overwritten" true. Unsupported `[v]` is demoted to `[x]` before the order is applied, reusing `runEvidenceCheck`'s existing predicate.
- **Escalation**: two failed verify rounds mark that milestone's tasks `[!]`; `checkHardGuardrails` is narrowed so under parallel execution a blocked task pauses only that lane. Serial runs keep today's whole-run pause.
- **Gates may fail, never promote.** `[x]` → `[v]` still requires both review agents and every check they run today.
- **Journal**: `.belmont/journal/<feature>/M<N>.jsonl`, committed, one file per milestone — structurally conflict-free with no merge driver. Metrics stay at `.belmont/metrics/`, gitignored.
- **Slot cap**: `parallel_slots` in `.belmont/config.json`; effective concurrency = `min(--max-parallel, parallel_slots)`, always reported. Run this feature at 2–3, not the default 5.
- **Proposals owed**: `0007-detail-tier` (M3), `0008-size-ceilings` (M5), `0009-master-and-journal` (M9) — the three milestones that change a contract 139 feature directories depend on. The other eight ship as ordinary PRs.
- **Distribution**: source-mode install (`belmont install --source ~/repos/belmont`) into all five repos. No fork tap, no release pipeline. Not run between M1 and M11.
- **Migration safety**: `belmont extract` proves a byte-for-byte round-trip before writing, supports `--dry-run`, and commits once per feature.
- **Model tiers** (`models.yaml`, profile `framework-go-and-prose`): every agent `high` except `design: low`. Rationale per agent — codebase must find all 16 state writers and all 17 parse sites (a miss is a torn file); implementation touches atomicity, merge precedence and scheduling where errors are silent data loss; verification checks invariants rather than features; code-review reads concurrency semantics, not style; reconciliation is always high and this feature changes merge semantics *while* merging; design is never dispatched. Consistent with the barbell rule — high or low, never medium.
