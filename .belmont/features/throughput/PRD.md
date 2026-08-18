# PRD: Belmont Throughput

## Overview

Make Belmont ship a verified milestone in less wall-clock time and fewer tokens, without weakening the adversarial verification model that makes `[v]` mean something. The work is structural — smaller state in the hot path, cacheable prompts, bounded failure loops — not model downgrades and not wider fan-out.

## Problem Statement

Belmont's graph is already correct. It computes a real milestone dependency DAG, sorts it topologically into waves, and executes those waves in isolated git worktrees. Against the field it is ahead: GitHub Spec Kit's `[P]` marker computes nothing, Task Master returns one task at a time, BMAD declares "sequential execution only", and OpenSpec has no task-level dependency notation at all.

What is wrong is everything around the graph. Measured on the two live deployments (repo-3: 29 features, 318 milestones, 1,791 tasks, 9.96 MB of state; repo-4: 14 features, 199 milestones, 1,094 tasks, 8.6 MB):

1. **The mandatory first read has become unreadable.** A framework optimisation deliberately made `PROGRESS.md` the first read of every phase so everything else could be scoped from it. Nothing then capped it. The worst file is 1,860,979 bytes / ~465,000 tokens, of which **98% sits under already-verified tasks**. A 200k-context sub-agent cannot read it at all, so it truncates silently and scopes its work from a partial picture.

2. **Verification is the bottleneck, and most of its cost is duplication.** Three separate readers in `/verify` are each told to read every archived milestone file: median 59,562 tokens per feature, so ~178,000 tokens per verify with ~119,000 of it pure duplication; the worst feature holds 346,735 tokens of archives, which no reader can satisfy. 507 tasks across the two repos sit at `[x]`, done but never verified. On one measured day a milestone took 47 minutes to implement and 118 minutes to verify across three rounds.

3. **Failing runs keep spending.** Across 165 measured multi-agent traces, **58% of tokens in a failing run are burned after the failure was already detectable** (median 61%). An intervention pilot cut that to 30% while still producing 8/10 usable finals. Belmont has a circuit breaker but no early-failure signal, and its fix cycle re-enters with the failed context still attached — which independently costs up to 34.8 percentage points of accuracy versus a fresh attempt.

4. **The largest static block in every prompt sits where caching cannot reach it.** Sub-agents are dispatched as `general-purpose` with an instruction to read their own definition, so 91 KB of agent definitions arrives *after* the variable prompt has already broken the cache prefix. A live dispatch measured 30,903 input tokens with 22,387 cache-creation against 8,506 cache-read — a 72% miss on a trivial task.

5. **State writes are not atomic.** Nine call sites use bare truncate-then-write. The institutional memory blames a rejected symlink design for a worktree reading `PROGRESS.md` mid-flip; the actual cause is the non-atomic write, and it is unfixed.

## Success Criteria (Definition of Done)

- [ ] Tokens per verified milestone and wall-clock per verified milestone are both measured, per run, on repo-3 and repo-4, with a captured pre-change baseline
- [ ] Median tokens per verified milestone falls by at least 50% against that baseline
- [ ] No phase reads more than 25,000 tokens of Belmont state before it has selected its milestone
- [ ] No `PROGRESS.md` in any of the five repos exceeds its configured ceiling, and the ceiling is enforced at write time rather than warned about
- [ ] A failing milestone escalates after 2 failed verify rounds rather than looping
- [ ] The `[x]` → `[v]` distinction, the two-agent verify fan-out, and milestone immutability are unchanged — no acceptance-criterion check is removed or narrowed
- [ ] `[v]` cannot be reached by merge without a commit supporting it, and no merge of two registers can lose a task or lower its state
- [ ] Every change is reversible by deleting an optional file or reverting one commit; all 139 existing feature registers keep working untouched (measured 2026-08-18: 82 active + 57 archived across the five repos)
- [ ] All five repos run the same Belmont build, verified by a version check

## Acceptance Criteria (BDD)

### Scenario: A phase scopes itself without reading the whole register
Given a feature whose `PROGRESS.md` holds 529 tasks across 42 milestones
When any orchestrating phase begins and needs to select a milestone
Then it obtains the milestone list, task identifiers, task states and dependency annotations
And the material it reads to do so is under 25,000 tokens
And it reads the narrative detail only for the milestone it selected

### Scenario: Verification does not re-read the same archive three times
Given a feature with 22 archived milestone records totalling 346,735 tokens
When `/belmont:verify` runs
Then the archived records are read once
And both review agents receive the extracted material rather than an instruction to re-derive it
And neither agent is given a read instruction it cannot satisfy within its context window

### Scenario: A doomed milestone stops early
Given a milestone that has failed verification twice
When the second failure is recorded
Then the loop stops and escalates to the user, naming the milestone and the failures
And it does not begin a third fix-and-re-verify round unattended

### Scenario: A retry starts clean
Given a task whose verification failed
When the fix is attempted
Then the fix begins from the specification and the failure report
And it does not inherit the reasoning context of the failed attempt

### Scenario: The register cannot silently regrow
Given a configured ceiling on register size
When an agent attempts a write that would push `PROGRESS.md` past the error threshold
Then the write is refused with a message naming the offending content and the remedy
And `belmont auto` refuses to start against a feature already over the threshold

### Scenario: Concurrent writes cannot be torn
Given two worktrees writing Belmont state at the same moment
When one writes while the other reads
Then the reader observes either the complete previous content or the complete new content
And never a partially-written file

### Scenario: An unknown dependency is an error, not a silent promotion
Given a milestone annotated with a dependency on a milestone that does not exist
When the dependency graph is computed
Then validation reports the dangling reference and names both milestones
And the milestone is not silently treated as having no unmet dependencies

## Out of Scope

Scored out on the evidence rather than on preference; each is recorded with the number that disqualified it.

- **A code-graph / symbol index for file localisation.** The 58–97% reductions circulating for this come from vendor benchmarks. The peer-reviewed result (RepoGraph, ICLR 2025) is +2.3 percentage points of issue-resolve rate for 12–38% *more* spend, and two-hop retrieval scores *below* using no graph at all. It also commits Belmont to per-language symbol extraction and incremental sync, which sits badly against shipping as a dependency-free binary.
- **Speculative execution across the implement→verify edge.** Sherlock reports strong figures, but its rollback trigger relies on similarity metrics that "collapse to random performance for code and math" (AUC ≈ 0.5) — precisely this workload. Cannot be made safe here yet.
- **Blast-radius verification scoping.** Depends on the code-graph index, and a wrong radius silently skips a check.
- **A rolling/greedy wave scheduler.** Level-by-level waves already land within 2.6–11.2% of the critical-path lower bound; on a slot-limited local backend the scheduling discipline is measurably irrelevant.
- **Task-level dependency or touched-path metadata.** The milestone is the worktree boundary; a task DAG needs a new isolation unit, which is a much larger change than the file format.
- **Revalidating the three May contribution specs** (`contribution-fast-mode`, `contribution-prd-hygiene`, `contribution-new-skills`) against v0.11.0. Real work, but it belongs to `framework-evaluation`'s open follow-ups.
- **Model tier changes as a cost lever.** Settled the other way — see Clarifications.
- **Reordering steering text relative to the stable prompt.** Withdrawn during tech-planning on an existing in-repo measurement: prefixed and suffixed steering produce identical cache figures, because the assembled prompt is a single short message whose cache breakpoint sits at its end. The mechanism is real; the saving is not.
- **Accessibility, internationalisation, offline behaviour, notifications, monetization.** Not applicable: Belmont has no user-facing UI surface, no end users beyond developers, and no commercial surface.

## Open Questions

- **Still open** — whether the extraction census in M1 finds any feature whose *index alone* exceeds 25,000 tokens. If one does, the read-path work in M4 stops being an optimisation and becomes a prerequisite for M3, and the wave structure changes. M1's census answers it.
- **Root cause found, resolution deferred to a spike in M8** — whether registering agents natively preserves identical tool access. The cause is now known: the six agent definitions carry no YAML frontmatter, so nothing registers as a dispatchable sub-agent and the prose falls back to a general-purpose dispatch that reads its own definition afterwards. M8's first task confirms empirically that registration preserves tool access, per-agent model tiers, and the research agents' forbidden-action constraints. If any of the three fails, the task is withdrawn rather than worked around. See TECH_PLAN.md §Go Implementation Notes.

## Clarifications

Answers captured during the planning interview, 2026-08-18.

- **Repository and posture**: work lands in `bigbenjoman/belmont` as a **tracking fork** of `blake-simpson/belmont`, with the upstream remote retained and merged periodically. Blake is leaving Studia at the end of the week; upstream shipped v0.11.0 on 2026-08-17 and is still alive. File formats stay backward-compatible so a future re-merge stays cheap and all five repos' existing state keeps working.
- **Host support**: keep multi-host, but make host-specific paths **build-time conditional** — generate one skill per host instead of shipping every host's prose to every host. This preserves opencode for the Oct 2026 local-LLM move while unlocking Claude-Code-specific caching.
- **Sequencing**: cheapest-first by ICE score. One reasoned deviation — instrumentation sits in M1 despite a mid-table score, because the chosen success bar cannot be an acceptance criterion on later milestones if nothing measures it.
- **Success bar**: tokens **and** wall-clock per *verified* milestone. `[v]`, not `[x]`, is the unit.
- **Goal shape**: same spend, more shipped. Token efficiency buys throughput, not savings — a change that halves tokens per milestone is a win even if the milestone count then doubles.
- **Metrics storage**: local-only and gitignored, alongside `auto.json`. Bookkeeping commits are already 41–46% of commit volume in these repos; adding a metrics file per run would worsen that for no analytical gain.
- **Model tiers**: the master PRD's "Sonnet is the current default model" is superseded. Opus is the default, Haiku for genuinely mechanical work, the Sonnet/medium tier skipped for real coding work. Basis: measured rework rates of ~37% for Sonnet 4.6 against ~19% for Opus 4.7 — a milestone rebuilt, re-verified and re-fixed costs far more than the tier difference. Independently supported by routing research showing that routing down is nearly free on mechanical nodes and catastrophic on the one or two nodes carrying the task's real difficulty (a hard node fell from 0.711 to 0.462 when downgraded). This also settles it: input cost is cut through structure, never by downgrading the model that determines rework.
- **Verify cap**: escalate after 2 failed rounds.
- **Atomic writes**: fixed as the first task of M1 rather than as a separate PR, keeping the plan self-contained.
- **Migration**: opt-in per feature. A missing detail directory means today's behaviour. Bulk-run the extraction only on the pathological minority; the median feature is 14 KB and gains nothing.
- **Rollout**: all five repos (repo-1, repo-5, repo-4, repo-2, repo-3), proven on repo-3 and repo-4 first — they hold the measured baselines, are the worst offenders, and are the two whose installs have drifted.
- **Domains skipped as not applicable**: accessibility, internationalisation, offline/degraded states, notifications and cross-surface, monetization, onboarding-as-discovery. Belmont's surface is a CLI plus markdown read by agents; none of these have a product surface here.
- **Parallelism expectations were revised downward during planning.** See Technical Context.

### Added during tech-planning, 2026-08-18

- **Where the work happens**: Belmont runs on itself, in the fork. The feature moves out of the planning workspace into the fork before M1 begins, and the orchestrating binary stays pinned to the released build so a milestone's changes cannot reach the loop that is running it. Implementation detail in TECH_PLAN.md §Prerequisites.
- **Existing prior art is an input, not competition.** An earlier, heavily reviewed proposal in the same repository already specifies part of the verify read-path work; M2 adopts it rather than re-deriving it. The part of it that a later milestone supersedes is recorded as superseded, so its reasoning is not lost.
- **Two prerequisite tasks were added to M1** — a toolchain bump and a triage of the repository's unmerged branch backlog. Both gate every later milestone, and the toolchain bump is sequenced ahead of the baseline capture so the before-and-after comparison is made on one toolchain.
- **Size ceilings are expressed in bytes, and the error ceiling equals the stated read budget.** 100 KB is 25,000 tokens at the conversion every measurement in this document uses, so the ceiling and the success criterion are the same number rather than two numbers that can drift apart. Warning fires at half that. Chosen against the measured distribution of all 139 registers.
- **A blocker outranks a verified state when two registers merge.** A blocker raised during a run is never overwritten by a stale advanced state from elsewhere; a wrongly surfaced blocker costs someone a look, a silently cleared one costs the guarantee. Full precedence order in TECH_PLAN.md.
- **Escalation stops one milestone, not the run.** Two failed verification rounds block that milestone's tasks and stop that milestone alone; siblings continue and merge. Today any blocked task halts the whole run, so this required a scheduler change, recorded as such.
- **Deterministic gates can only fail work, never pass it.** Nothing added in this feature can promote a task to verified. That still requires both review agents and every check they run today.
- **Instrumentation reports nothing rather than guessing.** Where a host cannot report token usage, the record says so and carries wall-clock only. This matters because the planned local-model host is one of those hosts.
- **Three milestones owe a written specification before implementation**, being the three that change a file contract every existing feature directory depends on. The other eight ship as ordinary changes.

## Technical Context (for implementation agents)

### Research Notes

Findings that shaped this plan. Each is load-bearing for a scoping decision.

- **Local parallelism is far weaker than the theoretical numbers.** SPOQ (arXiv:2606.03115) ran the same DAGs on both backends: 14.31× unbounded but **1.43× on a two-slot local backend** — and the unbounded figure is a synthetic sleep simulation, not LLM work. Mechanism, measured independently: LLM inference is 71–98% of agent runtime and decode is 91–98.6% of that (arXiv:2605.26297); decode is memory-bandwidth bound, so concurrent branches share one budget. PASTE (arXiv:2603.18897) measured generation time growing 17× as concurrency rose from 1 to 192 sessions. **Uncapped parallelism on bounded hardware is worse than sequential in its bad mode**, oscillating between near-two-slot and near-sequential. Consequence: concurrency must be capped to slot count, and the planned six-agent local configuration should expect far less than 6×.
- **Critical-path tokens, not scheduling, drive latency.** LAMaS (arXiv:2601.10560) measures Pearson r = 0.901 between critical-path tokens and wall-clock. Reducing tokens on the longest dependency chain is worth more than scheduling the graph better. This is why instrumentation must attribute tokens to the critical path, not only sum them.
- **58% of a failing run's tokens are spent after failure was detectable** (arXiv:2606.01365, 165 GAIA traces; median 61%; intervention pilot cut it to 30% while still producing 8/10 usable finals). The researchers' own framing: a larger and more reliable saving than anything DAG parallelism offers. This is the basis for M6.
- **Naive verification is not a marginal tax.** Sherlock (arXiv:2511.00330) measures per-node verification adding up to 28.9× latency and 53.2× cost in the worst case, and finds verifier-preserved output rates above 0.6–0.8 for self-consistency and LLM-as-judge — i.e. 60–80% of those verification calls change nothing. Selective placement, and deterministic gates where a rule exists, are the levers.
- **Forwarding a failed reasoning chain costs up to 34.8 percentage points of accuracy** versus a fresh attempt (arXiv:2608.13571). Basis for the fresh-context-on-retry requirement.
- **Fan-out can destroy correct answers.** At matched thinking-token budgets, single-agent beat multi-agent on multi-hop reasoning (arXiv:2604.02460); the error decomposition shows the multi-agent system winning 72 cases and losing 124 — a net loss — by over-exploring and dropping already-correct answers at aggregation. Argues against widening Belmont's fan-out.
- **Correlated failure is underpriced 2–8×.** Across 67 frontier models, pairwise error correlation underprices joint failure by 2.5× on one benchmark and 8.3× on another, and an LLM-as-router captured *exactly zero* of the available oracle gain (arXiv:2606.27288). Belmont's two parallel verify agents share a model and will co-fail more than intuition suggests — which is an argument for keeping their lenses genuinely different, not for collapsing them.
- **Caching matters more in agent workloads than in chat.** Measured input-to-output ratios of 53.9×–559.8× with KV cache-hit ratios of 84.6–99.5%. Cache reads cost ~0.1× input against a 1.25× write on the five-minute TTL, so a cacheable prefix pays back on the second dispatch of a milestone.
- **Plan caching** (arXiv:2506.14852) reports 50.31% cost and 27.28% latency reduction from caching structured plan templates; quality delta not fully verifiable from the paper. Noted as a future lever, not scoped here.

### Measured baseline (2026-08-18, before any change)

| Read path | Tokens | Ratio |
|---|---|---|
| Raw `PROGRESS.md`, worst feature — what the skills currently mandate | 465,245 | 1:1 |
| `belmont status --format json` — what four skills currently recommend | 170,537 | 2.7:1 |
| `belmont status --feature` (default text) | 14,286 | 33:1 |
| `belmont status --feature --max-task-name 1` | 6,864 | 68:1 |
| Extraction-based index (IDs, markers, dependencies, truncated titles) | ~12,200 | 38:1 |
| Minimum viable index (IDs, markers, dependencies only) | 1,686 | 276:1 |

Composition of the 1,860,979-byte worst case: 91.8% in the milestones region, of which **62.1% is indented bodies beneath task lines** and 25.0% is the task head lines themselves. The detail is already mechanically separable by indentation. Task head lines run to 13,088 characters; 458 of 529 tasks carry an indented body, median 1,416 bytes, max 33,674.

By marker: 98.0% of bytes sit under `[v]`, 0.8% under `[-]`. The feature reads 523 verified / 1 done / 1 blocked / 0 todo of 529 — essentially finished, and still in the hot path of every phase.

Master `.belmont/PROGRESS.md` in repo-3 is 498,879 bytes, of which 420,417 is a 330-row hand-appended narrative activity table.

### Known defects to fix alongside

- Nine `PROGRESS.md` writers use non-atomic truncate-then-write. A further seven sites write other Belmont state the same way, so the fix covers sixteen in total — see TECH_PLAN.md §Go Implementation Notes for the enumeration.
- An unknown milestone dependency contributes nothing to in-degree, so a typo silently makes that milestone eligible in the first wave. The equivalent check for *feature* dependencies already errors correctly — the milestone path simply lacks it.
- `docs/feature-auto.md` documents milestone syntax the parser cannot read (list items rather than headings), which yields zero parsed milestones for anyone following it.
- Four skills recommend `belmont status --format json` as a quick summary. It is the largest text output of any read path, 12× the default at 529 tasks. A warning against it already exists in exactly one place (the loop recipe, which cites ~3× — an understatement at scale) and was never propagated.
- The two parallel research agents in `/implement` are both instructed to read and append to the same coordination file with no lock, so the later writer can drop the earlier one's section silently.

### Constraints inherited from the framework

- Markdown files, not chat history, are the source of truth. State must stay human-readable and git-diffable, and readable by any agent that can read a file.
- Task markers `[ ] [>] [x] [v] [!] [-]` are a fixed vocabulary enforced centrally. Only a different agent than the implementer may promote `[x]` to `[v]`.
- Only the tech-planning stage may create or restructure milestones.
- State merges across worktrees by matching task ID; duplicate IDs refuse to merge.
- Absent-means-today: any new optional file must, when missing, produce exactly current behaviour.
- Belmont's institutional memory has already rejected symlinked shared state between worktrees and caching the status command's output. Computing a slice on demand is not the same as caching one — any design in that area must say so explicitly and leave no artefact on disk.

### Effort profile (hint)

Mixed. M1, M3, M5, M9, M10 are Go-side work on the CLI. M2, M6, M7 are skill-prose work. M4 and M8 span both. M11 is operational. No design or Figma input anywhere in this feature — the design agent should be skipped on every milestone.

### Skills worth loading

`context-audit` for the M7 skill-payload work and the M1 global-surface measurement. No frontend, design, or database skills apply.

## Tasks

### P0-1: Atomic state writes
**Severity**: CRITICAL

**Task Description**: Belmont writes its state files with a truncate-then-write sequence. A reader in another worktree can observe a partially written file. This has already been blamed on a different design and is still present.

**Solution**: Every write of Belmont state completes atomically from a reader's point of view — a concurrent reader sees either the complete previous content or the complete new content, never a partial file. Applies to every state writer, not a subset.

**Notes**: Nine call sites. This is a correctness fix, not an optimisation; it is first because later milestones write more often and are unsafe without it.

**Verification**: A concurrent read-during-write test observes only complete files. Existing state-management tests pass unchanged.

### P0-2: Token and wall-clock instrumentation
**Severity**: CRITICAL

**Task Description**: Nothing currently measures what a milestone costs. The chosen success bar cannot be evaluated without this, and the framework's own history includes an optimisation that measurement later disproved.

**Solution**: Each run records tokens consumed and wall-clock elapsed, attributed to the milestone and phase that spent them, and separately identifies the spend on the critical dependency path. Records are written locally and are not committed. A summary can be printed for any feature, comparing runs.

**Notes**: Local-only and gitignored, alongside `auto.json`. Critical-path attribution matters because latency correlates with critical-path tokens at r = 0.901, not with total tokens.

**Verification**: Two consecutive runs on the same feature produce comparable records. The metrics location is gitignored and produces no working-tree changes.

### P0-3: Capture the pre-change baseline
**Severity**: CRITICAL

**Task Description**: Every later acceptance criterion is stated against a baseline that does not yet exist.

**Solution**: A recorded baseline of tokens and wall-clock per verified milestone for repo-3 and repo-4, taken before any change in this feature lands, stored where M11 can compare against it.

**Notes**: Must be taken after P0-2 and before any of M2–M10 merges.

**Verification**: A baseline record exists for both repos and names the Belmont version it was taken against.

### P0-4: Extraction census across all feature directories
**Severity**: HIGH

**Task Description**: The tiered design assumes the register is small once narrative is removed. That holds for the two files measured; it is unverified across the other 83.

**Solution**: A dry-run report over every feature directory in all five repos, giving the size distribution of the register once narrative detail is separated, and naming any feature whose register alone would still exceed 25,000 tokens.

**Notes**: This answers an open question that can change the wave structure — if any index alone exceeds the ceiling, M4 becomes a prerequisite of M3 rather than a successor.

**Verification**: A report exists covering every feature directory, with a distribution and an explicit list of any feature exceeding the threshold.

### P0-5: Read archived milestone records once
**Severity**: CRITICAL

**Task Description**: Three separate readers are each told to read every archived milestone record. At the median that is ~178,000 tokens per verify with ~119,000 duplicated; at the worst it is an instruction no reader can satisfy, so it is silently truncated.

**Solution**: The archived records are read once per verify run, and both review agents receive the extracted material directly rather than an instruction to re-derive it. No agent is given a read instruction that cannot fit its context.

**Notes**: Mirrors the coordination-document pattern the framework already uses for implementation hand-off. The extraction must preserve design references, which is what the orchestrator reads them for today.

**Verification**: A verify run on a feature with many archived records reads them once. Both agents' reports still cite implementation context from those records.

### P0-6: Stop recommending the largest read path
**Severity**: HIGH

**Task Description**: Four skills recommend a status output format that is the largest of any available read path — 12× the default at 529 tasks. A warning exists in one other place and understates the multiplier.

**Solution**: No skill recommends that format as a summary. Where a quick summary is useful, the skills name the cheapest path that answers the question, and the existing warning is corrected to reflect the measured scale factor.

**Notes**: Purely a prose fix across four skills plus one correction.

**Verification**: No skill recommends the expensive format. The corrected multiplier matches a measurement taken on a large feature.

### P0-7: Give verification a coordination document
**Severity**: HIGH

**Task Description**: Both verify agents independently re-read the specification documents, costing roughly 30,000 duplicated tokens per run on a median feature.

**Solution**: The verify orchestrator produces one coordination document holding the material both agents need — the tasks under verification, their acceptance criteria, the extracted archive context and the design references — and both agents read that.

**Notes**: The implementation side already works this way; verification is the half that was never given the equivalent.

**Verification**: A verify run produces the coordination document and both agents' reports trace to it. Neither agent re-reads the full specification set.

### P0-8: Detect failure before the budget is spent
**Severity**: CRITICAL

**Task Description**: In failing runs, most of the spend happens after the failure was already detectable. Belmont has a circuit breaker for repeated failure but no early signal within a run.

**Solution**: A run surfaces a warning as soon as its trajectory indicates it is unlikely to produce a usable result, naming what triggered the warning. Under autonomous operation the run pauses at that point rather than continuing to completion.

**Notes**: The measured opportunity is 58% of a failing run's tokens. The pilot that informed this preserved 8 of 10 usable outcomes while cutting post-warning spend by more than half — so the signal must be advisory enough not to abort recoverable work.

**Verification**: A deliberately failing milestone produces the warning before completion. A healthy milestone completes without one.

### P0-9: Cap the verify-and-fix cycle at two rounds
**Severity**: CRITICAL

**Task Description**: One milestone was re-entered eight times. Another consumed 118 minutes across three verify rounds. Nothing bounds the cycle per milestone.

**Solution**: After two failed verification rounds on the same milestone, the loop stops and escalates to the user, naming the milestone and each failure. It does not begin a third unattended round.

**Notes**: Escalation is not the same as a verdict — the milestone is reported incomplete, and other milestones are unaffected, consistent with the existing blocked-task rule.

**Verification**: A milestone failing verification twice escalates and does not start a third round. Sibling milestones continue.

### P0-10: Start every retry from clean context
**Severity**: HIGH

**Task Description**: When verification fails, the fix currently proceeds with the failed attempt's context attached. Carrying a failed reasoning chain forward costs up to 34.8 percentage points of accuracy against a fresh attempt.

**Solution**: A fix attempt begins from the specification and the failure report only. It does not inherit the reasoning context of the attempt that failed.

**Notes**: This is both a quality and a token improvement, and it is prose-level. It does not change what is verified, only what the fixer starts from.

**Verification**: A fix dispatched after a failed verification receives the specification and failure report and not the prior attempt's transcript.

### P0-11: Gate the expensive agents behind deterministic checks
**Severity**: HIGH

**Task Description**: Verification agents are dispatched even when a deterministic check would have failed the work immediately. Measured across verification research, 60–80% of judge-style verification calls change nothing.

**Solution**: Where a rule exists and can be evaluated without a model — the project builds, types check, the register parses, evidence is present — it is evaluated first, and failures become follow-up work without spending an agent's context on a tree that does not compile.

**Notes**: The framework already runs deterministic guards after each autonomous phase; this extends the same principle to the front of verification. Does not remove any acceptance-criterion check — it reorders when the expensive ones run.

**Verification**: A milestone with a failing build produces follow-up tasks without dispatching the review agents. A clean milestone dispatches them as before.

### P0-12: Bring the toolchain onto a supported version
**Severity**: HIGH

**Task Description**: The project declares a language version that is out of support. Eleven milestones of changes are about to land on it, and one of them asserts a concurrency property that needs the toolchain's race detector.

**Solution**: The declared toolchain version is current and supported, the full test suite passes on it, and the change lands on its own.

**Notes**: Sequenced ahead of the instrumentation and baseline tasks so the recorded baseline and the final re-measurement are taken on the same toolchain. A comparison whose two halves were built differently proves nothing.

**Verification**: The project builds and the full suite passes on the updated version. The change is a single commit touching nothing else.

### P0-13: Triage the unmerged branch backlog
**Severity**: HIGH

**Task Description**: The working repository carries a large backlog of unmerged branches, five of which change the same files that later milestones rewrite. Discovering that at merge time is the expensive way to find out.

**Solution**: Every unmerged branch is classified as merge, rebase or abandon, with the verdict and reason recorded where the next session will read it. The classification is complete before any milestone touches a shared file. The default branch is brought up to date so it stops misrepresenting what the repository contains.

**Notes**: This is the task that prevents wave 2's six parallel milestones from colliding with existing work.

Three branches carry a known dependency and must be triaged deliberately rather than swept:

- **`origin/docs/pr-proposals`** holds proposals 0003–0006, including `0004-context-budget-with-evidence.md` rev 5, which **M2 takes as its specification**. Verified 2026-08-18: there is no `docs/proposals/` directory on `main`, and neither `AGENTS.md` nor `CONTRIBUTING.md` documents the convention. A worktree cut off `main` cannot read M2's spec. This branch's verdict therefore gates M2, and the three proposals this feature owes (0007–0009) need a home on the mainline before M3, M5 and M9 begin.
- **`origin/proposal/0001-quick-mode`** and **`origin/proposal/0002-prd-hygiene`** carry `framework-evaluation`'s P6-1 and P6-2 specs, pushed 2026-05-23. They are prior art from the other feature in this workspace, not stray work.

**Verification**: Every unmerged branch carries a recorded verdict. The default branch matches the working state. `origin/docs/pr-proposals` has an explicit verdict, and if the verdict is anything other than merge, the note records where M2 is to read proposal 0004 from instead.

### P1-1: Separate narrative detail from the register
**Severity**: CRITICAL

**Task Description**: 62% of the worst register is indented narrative beneath task lines. It is already mechanically separable by indentation, and it is the reason the file cannot be read.

**Solution**: A command moves each milestone's narrative detail out of the register into a per-milestone record, leaving the task line, its identifier, its state marker and a short title in place. The register keeps every task and every marker, so status reporting is unchanged. Running it on a feature is opt-in and reversible.

**Notes**: The grammar of a task line and a milestone header must not change — that grammar is parsed in seventeen places and changing it buys nothing measurable. This is an extraction, not a re-modelling.

**Verification**: After extraction, the register lists every task with its original identifier and marker, status reporting is byte-identical to before, and the moved narrative is retrievable in full.

### P1-2: Validate index and detail integrity
**Severity**: HIGH

**Task Description**: Splitting a record into two files creates new ways for it to be wrong: a task in the register with no detail, detail with no task, a stale reference after a merge.

**Solution**: Validation reports any task present in one tier and absent from the other, any orphaned detail record, and any register that exceeds its configured size, naming each specific offender.

**Notes**: These must be deterministic checks, not agent judgement.

**Verification**: Each failure mode is detected on a deliberately corrupted fixture and reported by name.

### P1-3: A durable grammar for detail records
**Severity**: MEDIUM

**Task Description**: The current register rewrites task bodies in place and keeps the old text inline — 207 such markers in one file. That is how a tracker becomes a diary.

**Solution**: Detail records use a consistent block structure so they diff cleanly and merge safely, and a superseded body is recorded as a supersession rather than by keeping both copies inline.

**Notes**: Applies to the detail tier only. The register's line grammar stays untouched.

**Verification**: A superseded task body appears once with its supersession recorded. Two branches appending different detail to the same milestone merge without conflict.

### P1-14: Refuse a merge that loses state
**Severity**: CRITICAL

**Task Description**: Cross-worktree merges of the register are resolved by special-case rules with no accounting check. Two live duplicate task identifiers already exist in large files, detected only when a merge refuses; and the sync path drops task bodies from carried tasks.

**Solution**: A merge of two registers is written only if every task identifier present in either input appears exactly once in the output, no resulting state is lower than either input's, and every input task's text either survives or is reported as replaced with both versions named. A merge that cannot satisfy this is refused and escalated rather than written. Duplicate identifiers are reported by validation rather than discovered at merge time.

**Notes**: The failure this prevents is silent loss, which is the documented failure mode of every comparable system that merges structured text. The escalation path already exists.

**Verification**: A merge that would drop a task is refused and names it. A register with a duplicate identifier fails validation. A carried task retains its body.

### P1-15: Close the verified-state laundering path
**Severity**: CRITICAL

**Task Description**: The `[x]` versus `[v]` distinction is the framework's core quality guarantee. At merge, a `[v]` arriving from another worktree is accepted without checking that any commit supports it — so the one state an agent may not award itself can arrive by merge. Separately, the current state-precedence rules produce measured cases where a blocked task regresses to verified.

**Solution**: A verified state arriving by merge without a commit naming that task merges as done-not-verified and is reported. State precedence is a single total order applied uniformly, so merging is order-independent, and a blocker raised during a run is never overwritten by a stale advanced state.

**Notes**: This narrows what can be called verified — it does not remove any check. It closes the one point where the invariant is currently bypassable.

**Verification**: An unsupported verified state arriving by merge lands as done-not-verified and is reported. The four known blocked-to-verified regressions no longer occur. Merging two registers in either order gives the same result.

### P1-4: Serve a single milestone's slice on demand
**Severity**: CRITICAL

**Task Description**: Even an extracted register is paid in full by every phase. A phase needs one milestone.

**Solution**: A command emits exactly the material a phase needs for one named milestone — its tasks, their states, its dependencies and its detail — computed on demand from the files. No artefact is written to disk, so there is nothing to invalidate and nothing that can be stale.

**Notes**: This is explicitly not a status cache, and the distinction must be stated where it will be read. The framework has already rejected caching status output; computing a slice fresh is a different thing.

**Verification**: The slice for a milestone contains its tasks and no other milestone's, matches the register, and leaves no file behind.

### P1-5: Make the cheap path the mandatory one
**Severity**: CRITICAL

**Task Description**: The skills tell agents to read the register directly and list the CLI only as an optional helper. That ordering is what makes the expensive path the default.

**Solution**: Phases obtain their scope through the slice, and fall back to reading the register directly only where the CLI is unavailable. The fallback is stated, so an agent without the binary still works.

**Notes**: This is the change that realises most of the measured saving. It touches roughly a dozen prompts across the skills.

**Verification**: A phase run with the CLI present obtains its scope through the slice. The same phase with the CLI absent still completes by reading the files.

### P1-6: Configurable size ceilings
**Severity**: HIGH

**Task Description**: Without a ceiling, an extracted register regrows. The commit history on the worst file shows the rate — it reached 1.86 MB in a quarter.

**Solution**: A project may declare warning and error thresholds for register size. Absent configuration means no limits and today's behaviour exactly.

**Notes**: Absent-means-today is a hard requirement — 85 existing feature directories must be unaffected until opted in.

**Verification**: A project with no configuration behaves as it does today. A configured project reports its thresholds.

### P1-7: Refuse to start over the ceiling
**Severity**: HIGH

**Task Description**: A warning that fires after the expensive read has already happened saves nothing.

**Solution**: Autonomous operation refuses to start against a feature whose register is over its error threshold, before the first read, naming the file and the remedy.

**Notes**: Warnings alone are documented as insufficient for this class of problem. The refusal must fire ahead of the read it is protecting.

**Verification**: A feature over threshold refuses to start and names the remedy. A feature under threshold starts normally.

### P1-8: Reject oversized writes at the source
**Severity**: MEDIUM

**Task Description**: The register grows because agents write prose into it. Nothing stops them.

**Solution**: A write that would push a task line past the configured limit is rejected, with a message directing the content to the detail tier.

**Notes**: This is the mechanism that changes agent behaviour; the ceilings above only detect the result.

**Verification**: An oversized task-line write is rejected and names the detail tier. A normal write succeeds.

### P1-9: Stop shipping every skill's shared prose to every skill
**Severity**: HIGH

**Task Description**: Roughly 32,000 tokens of built skill content is the second-and-later copy of shared prose already emitted into another skill. One skill ships the same 17 KB recipe twice, 99.92% identical, one copy for a host the reader is not running.

**Solution**: Shared prose is emitted once per skill that needs it, and host-specific prose only into the build for that host. A reader on one host never pays for another host's instructions.

**Notes**: An existing unmerged branch already prototypes part of this. Multi-host support is retained — this changes what each host's build contains, not which hosts are supported.

**Verification**: Built skills for a given host contain no other host's instructions. Total built skill size falls measurably. Every supported host still has a complete build.

### P1-10: Prove the skill payload shrank
**Severity**: MEDIUM

**Task Description**: The payload change must be measured, not asserted.

**Solution**: A before-and-after comparison of built skill sizes per host, recorded against the M1 baseline.

**Notes**: Small task; exists so the claim is evidenced rather than inferred.

**Verification**: The comparison exists and shows per-host totals before and after.

### P1-11: Put agent definitions where the cache can hold them
**Severity**: HIGH

**Task Description**: Agent definitions arrive after the variable part of a prompt, so the cache prefix has already diverged. A live dispatch measured a 72% cache miss on a trivial task.

**Solution**: A dispatched agent receives its definition as part of its stable context rather than fetching it after variable content. The definition content itself is unchanged.

**Notes**: The registered-agent path already exists and is bypassed. Depends on an open question — if registration does not grant identical tool access, this task is dropped rather than worked around. Cache reads cost about a tenth of input against a 1.25× write, so this pays back on the second dispatch of a milestone.

**Verification**: Cache-read tokens exceed cache-creation tokens on the second and later dispatches within a milestone. Agents retain file-editing and shell access.

### P1-12: Stop invalidating the cache with steering text
**Severity**: MEDIUM — **WITHDRAWN 2026-08-18 during tech-planning**

**Withdrawal reason**: An existing measurement in the same repository compared the two orderings directly and found identical cache figures either way. The assembled prompt is a single short message and its cache breakpoint sits at the end, so reordering within it recovers nothing. The mechanism described below is accurate; the benefit claimed for changing it is not. Recorded rather than deleted so the reasoning is not rediscovered.

**Task Description**: Variable steering text is prepended ahead of an otherwise stable instruction, invalidating the cached prefix for the whole iteration whenever steering has been used.

**Solution**: Variable content follows stable content in every assembled prompt.

**Notes**: The milestone-scope text in the same path is already appended correctly; this brings the steering path into line with it.

**Verification**: A steered run and an unsteered run share a cached prefix up to the point where they genuinely differ.

### P1-13: Verify the caching change against the harness
**Severity**: HIGH

**Task Description**: The framework already contains an optimisation that measurement disproved, which is why its evaluation harness exists. This change must go through it.

**Solution**: The caching change is evaluated against the existing harness and against measured cache-read and cache-creation figures, and is reverted if it does not improve them.

**Notes**: Reverting on a negative result is the expected outcome path, not a failure.

**Verification**: A harness run records the before and after figures and states the verdict.

### P2-1: Generate the master progress file
**Severity**: MEDIUM

**Task Description**: The master file is 498,879 bytes, of which 420,417 is a hand-appended narrative table. Nobody authors it as a human artefact, yet four skills instruct agents to hand-edit it.

**Solution**: The master file is generated from the feature registers, marked clearly as generated, and validated as current during preflight. The skills stop instructing agents to hand-edit it.

**Notes**: This applies to the master file only. The feature-level register stays authored — measurement of its commit history shows that regenerating it would produce large spurious diffs.

**Verification**: The generated file matches the feature registers. Preflight detects a stale master file. No skill instructs an agent to edit it by hand.

### P2-2: Move the activity narrative to an append-only record
**Severity**: MEDIUM

**Task Description**: The activity table grows without bound and is edited in place by parallel agents, which is both a size problem and a merge hazard.

**Solution**: Activity is recorded append-only, one entry per line, in a form that merges by union rather than by conflict, and is rotated when it grows past a threshold. The master file's activity view is rendered from it.

**Notes**: Each worktree writes its own file on its own branch and the union happens at merge time — this is deliberately unlike the shared-state design already rejected.

**Verification**: Two branches recording activity concurrently merge without conflict and both entries survive. Rotation fires at the threshold.

### P2-3: Retire hand-edited session history
**Severity**: LOW

**Task Description**: Session history in the feature register is the same append-only narrative pattern at feature level.

**Solution**: Session history is recorded in the same append-only form, and the register no longer carries it.

**Notes**: Existing session-history sections must continue to parse for features that have not migrated.

**Verification**: A migrated feature records history in the new form. An unmigrated feature still parses.

### P2-4: Make a dangling dependency an error
**Severity**: HIGH

**Task Description**: An unknown milestone dependency contributes nothing to in-degree, so a typo silently promotes that milestone into the first wave. The equivalent feature-level check already errors correctly.

**Solution**: Validation reports a dependency naming a milestone that does not exist, identifying both, and the milestone is not treated as unblocked.

**Notes**: Small and self-contained. It is a correctness fix in the dependency layer that any parallel work would otherwise amplify.

**Verification**: A register with a dangling dependency fails validation and names both milestones. A correct register passes.

### P2-5: Correct the documented milestone syntax
**Severity**: MEDIUM

**Task Description**: The autonomous-mode documentation shows milestone syntax the parser cannot read. Anyone following it gets zero parsed milestones and a confusing halt.

**Solution**: The documentation shows the syntax the parser actually accepts.

**Notes**: Documentation-only.

**Verification**: The documented example parses.

### P2-6: Cap concurrency and say when parallelism is unavailable
**Severity**: HIGH

**Task Description**: Two problems. Parallel execution engages only when at least one milestone declares a dependency — otherwise a run with parallelism requested falls through to fully serial with no message. Separately, issuing more concurrent work than the backend has slots is measurably worse than running serially.

**Solution**: When parallelism is requested but cannot engage, the run says so and why. Concurrent work is capped to the available slot count rather than to the requested figure alone.

**Notes**: The local measurement behind the cap: the same graph yielding 14.31× on an unbounded backend yields 1.43× on a two-slot local one, and overcommitting the slot pool oscillates between near-two-slot and near-sequential. Expectations for the planned local configuration should be set accordingly.

**Verification**: A run with parallelism requested and no dependencies declared reports why it is serial. A run exceeding available slots is capped and reports the cap.

### P3-1: Migrate the pathological features
**Severity**: HIGH

**Task Description**: Four files account for most of the measured pain. The median feature is 14 KB and gains nothing from migration.

**Solution**: The features identified by the census as over threshold are migrated to the tiered layout. Everything else is left alone.

**Notes**: Opt-in per feature; a feature without the new layout behaves exactly as today.

**Verification**: Each migrated feature's register is under threshold, its markers are unchanged, and its narrative is retrievable.

### P3-2: Bring all five repos onto one build
**Severity**: HIGH

**Task Description**: The installs have drifted. One repo carries a dispatch path deleted upstream; another runs a vendored copy differing from upstream on ten of eleven skills, whose hand-written substitute for a core skill caused a data-integrity incident.

**Solution**: All five repos run the same Belmont build, confirmed by a version check rather than by inspection.

**Notes**: This is the task that makes every other change in this feature actually reach the work.

**Verification**: A version check across all five repos reports the same build with no local divergence.

### P3-3: Re-measure and report against the baseline
**Severity**: CRITICAL

**Task Description**: The feature's success criteria are stated against the M1 baseline and are otherwise unevidenced.

**Solution**: Tokens and wall-clock per verified milestone are re-measured on repo-3 and repo-4 and compared against the captured baseline, with a stated verdict against each success criterion, including any that were not met.

**Notes**: A criterion that was not met is reported as not met. The point of the baseline is that this cannot be argued either way.

**Verification**: The comparison exists for both repos and states a verdict against every success criterion.
