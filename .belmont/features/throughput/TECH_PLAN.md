# Technical Plan: Belmont Throughput

> **Master Tech Plan**: See `.belmont/TECH_PLAN.md` for workspace conventions. **Read the note there about the two workspaces** — this feature is the first in this repo whose build artefacts are Go source and skill prose in a *different* repository, not markdown deliverables in this one.

Written 2026-08-18. All measurements in this document were taken on that date against `bigbenjoman/belmont` at `17897aa2` (= `upstream/main`, v0.11.0, released 2026-08-17) unless stated otherwise.

## Overview

Eleven milestones that cut tokens and wall-clock per *verified* milestone by restructuring what Belmont reads, bounding what it spends on failure, and making its state writes safe under parallel execution. Nothing here weakens verification: `[x]` → `[v]` still requires two independently-lensed agents, milestone immutability is untouched, and no acceptance-criterion check is removed or narrowed.

The work is Go (`cmd/belmont/*.go`, package `main`, zero external dependencies) plus skill prose (`skills/belmont/_src/*.md`, `skills/belmont/_partials/*.md`) plus agent definitions (`agents/belmont/*.md`), executed against the fork with Belmont running on itself.

---

## Prerequisites — before M1 can start

These cannot be milestone tasks: `belmont auto` cannot bootstrap the state directory it reads from. Do them by hand, in this order.

0. **Refresh the user-level install — this one had already failed silently.** `.claude/` is gitignored in the fork, so a fresh worktree carries no project-local commands and the **user-level install at `~/.agents/skills/belmont/` is the only thing resolving `/belmont:*` inside a worktree**. Measured 2026-08-18 during `/belmont:review-plans`: it was last written **2026-05-04** and was missing `loop`, `repair` and `codex-plan-apply` — i.e. v0.10.x against a v0.11.0 orchestrator. Left alone, all six wave-2 milestones would have executed 3½-month-old prose, including the `verify.md` that M2 rewrites, and the M1 baseline would have been captured on different skill prose from the M11 re-measurement — the same failure mode that sequences P0-12 ahead of P0-3. Refresh with `belmont install --project ~ --tools claude`, then confirm `loop`, `repair` and `codex-plan-apply` are present.

   **This must happen before M1 and must not happen again until after M11.** Refreshing skill prose mid-feature is exactly what the pinning rule in §Self-hosting hazards forbids.
1. **Move the feature.** `mv /Users/benlavender/belmont/.belmont/features/throughput /Users/benlavender/repos/belmont/.belmont/features/throughput` — after this session's writes land. The research workspace keeps the master PRD/PROGRESS/TECH_PLAN and the `framework-evaluation` feature; the master PROGRESS row for `throughput` gains a pointer to the fork.
2. **Ignore `_research/` in the fork before the move lands.** `_research/` is 436 KB of the feature's 572 KB, and the fork's `.gitignore` has no rule for it. The master TECH_PLAN says research notes are never committed. Add `_research/` to `/Users/benlavender/repos/belmont/.gitignore` in the same commit that lands the feature directory; the files stay on disk so implementation agents can still read the measurement evidence they cite.
3. **Install Belmont into the fork.** `cd ~/repos/belmont && belmont install --source . --tools claude`. This creates `.agents/`, `.claude/commands/belmont/`, and `.belmont/`.
4. **Confirm the orchestrating binary is pinned.** The loop must run the Homebrew `belmont` (v0.11.0, `/opt/homebrew/bin/belmont`), *not* a build from the worktree. Go changes made inside a milestone take effect only on an explicit rebuild — this is the isolation that makes self-hosting safe.
5. **Set concurrency.** Run with `--max-parallel 2` or `3`, not the default 5. Wave 2 holds seven milestones; the measured local speedup is ~1.4×, and overcommitting is worse than serial.

## Prerequisites — before **wave 2** can start

*Added 2026-08-19. These four are gates on wave 2, not on M1, and every one of them is an owner action: two touch a public git remote and two decide what the loop runs on. No agent performs any of them.*

6. **Redact the product estate from this repository, then force-push.** `github.com/bigbenjoman/belmont` is `visibility: PUBLIC` and a fork of `blake-simpson/belmont`. `origin/main` has carried `.belmont/` since the P0-13a push on 2026-08-18: `census.json` alone names **82 feature slugs across all five products** (repo-4 14, repo-3 29, repo-2 18, repo-5 11, repo-1 10) with per-feature byte, milestone and task counts, and `MILESTONE-M1.done.md` carries 107 further product references across 245 KB.

   Rewrite every committed occurrence to opaque identifiers — `repo-1`…`repo-5`, `feature-a`… — preserving every byte count, milestone count, task count and conclusion, because the census's findings are about *distributions*, not names. Twelve committed files are affected; **two of them are product source and would travel in an upstream PR regardless of `.belmont/`**: `cmd/belmont/extract.go:31` (names repo-4 and a byte count) and `knowledge/auto-mode/clean-tree-preflight.md:61` (names `~/repo-3`).

   The alias↔slug mapping lives in the private planning workspace at `~/belmont/`, never in this repository — without it CENSUS.md's conclusions stop being actionable against real repos, and with it committed the redaction is decorative.

   **Accepted residual risk, recorded rather than argued.** Objects already pushed to a public fork stay reachable through the parent repository's network, so a force-push does not undo the publication; only deleting the fork would. Decision by Ben Lavender, 2026-08-19: redact in place and accept that. This step reduces future exposure and the exposure already taken stands.

   **DONE 2026-08-19.** 625 identifier substitutions across the twelve files — five repo names to `repo-1`…`repo-5`, 77 feature slugs to `feat-001`…`feat-077`. `main` rewritten with `git filter-repo` (`--replace-text` **and** `--replace-message`; commit messages carried the names too, which this section had not anticipated) and force-pushed: `d13f0fbf` → `ff11815`. Every ref on `origin` was audited afterwards, not just `main` — `origin/main` and `origin/HEAD` were the only dirty ones, so no stale branch keeps the estate published. Map at `~/belmont/private/estate-alias-map.json`; pre-rewrite bundle beside it, gitignored.

   **Scope the rewrite to `17897aa2..main`, and here is why that is not optional.** The first attempt ran `filter-repo` across all refs. It produced a clean history and destroyed the fork relationship: `17897aa2`, the merge base with `blake-simpson/belmont`, ceased to exist, so every future `git merge upstream/main` would have seen two unrelated histories with identical content. Restored from the bundle, re-run with `--refs 17897aa2..main`, and the merge base is preserved and verified an ancestor of `HEAD`. **The upstream half of the history contains none of these identifiers** — measured before either attempt: `git grep` for all five names over `upstream/main` returns nothing. So there is never a reason to rewrite past the merge base, and doing so costs the periodic upstream re-merge the master PRD commits to.

   *A figure-preservation guard is worth writing even for a one-off.* The redaction script masked identifiers on both sides and refused to write any file whose measured digits did not survive byte-identically. It fired twice — once correctly (aliases carry digits of their own) and once on this very section's literal `repo-1`…`repo-5` examples. Both were guard bugs, not data loss, but a silent run would have given no reason to believe that.

7. **Do not start wave 2 before step 6 lands.** Seven milestones commit into this repository concurrently. Redacting after they start means redacting a moving target across seven worktrees.

   **SATISFIED 2026-08-19** — step 6 landed before any wave-2 milestone started.

   **Merged 2026-08-19.** Not cleanly: five conflicts, all created by M1's own fix batch moving `AGENTS.md`, `docs/cli-commands.md` and the CLI surface hours after the verdict measured zero. The collision was real, it just came from our side. Two of its changes are directly relevant here — a table-driven `printUsage` (which `metrics` and `extract` were added to) and a **per-milestone** reverify reset replacing the bulk one (issue #49), which is half of the defect this feature had recorded that morning.

   ⚠️ **This invalidates any `belmont-pre` built before the merge, and that is a measurement problem, not a rebuild chore.** `issue-sweep` rewrites `_partials/dispatch-strategy.md`, `_partials/loop-recipe.md` and `_src/{implement,loop,next,status}.md` — the same prose M2/P1-5 and M8/P1-11 change. A frozen binary carrying *pre*-merge prose, compared against a post-change build carrying *post*-merge prose, folds `issue-sweep`'s effect into this feature's measured result. The two arms must differ **only** by this feature's work. Rebuild `belmont-pre` from the merge commit before gate 9, and record its SHA.

8. **Reverify M1, then freeze the orchestrator.** `belmont reverify --feature throughput` promotes M1's remaining `[x]` tasks (P0-3, P0-M1-FIX-12). Then build the pinned artefact from M1's merge commit and keep it:

   ```bash
   cd ~/repos/belmont && git rev-parse HEAD > /tmp/belmont-pre.sha
   bash scripts/build.sh && cp ./belmont ~/bin/belmont-pre
   ```

9. **Repoint the pin to `belmont-pre` — this changes master TECH_PLAN rule 1.** Put `belmont-pre` on PATH ahead of Homebrew, for this repository's own loop *and* as the daily driver in the five product repos. It is one artefact serving both: there is one `belmont` on PATH, and the fork's loop and the passive arm want the same frozen build. See §The passive arm below for why, and §Risks for what it costs.

### Self-hosting hazards, and what contains them

| Hazard | Containment |
|---|---|
| A milestone edits the Go code the orchestrator is running | Orchestrator uses a **frozen** binary; worktree builds are never installed mid-run. **Changed 2026-08-19**: the frozen binary is `belmont-pre`, built from M1's merge commit, not Homebrew v0.11.0. Isolation is identical — both are frozen — but Homebrew v0.11.0 contains no metrics code, so pinning to it meant the feature that exists to measure Belmont recorded *nothing* across its own twelve remaining milestones. Revert is one command: put Homebrew back on PATH |
| A milestone edits skill prose the next phase reads | Skill prose resolves from `~/.agents/skills/belmont/` (user-level) until an explicit `belmont install` is run. **Do not run `belmont install` — in the fork or at user level — between M1 and M11.** The user-level refresh is prerequisite 0 and happens once, before M1. Verified at v0.11.0 at that point; it was v0.10.x before. |
| A milestone changes `PROGRESS.md` format while the loop parses it | The register's line grammar does not change (see Decision Log). Only the detail tier is new, and it is opt-in per feature — `throughput` itself stays un-extracted until M11 |
| M11 migrates features while the loop reads them | M11 runs serially, last, and `throughput`'s own register is migrated only after P3-3 has taken its final measurement |

---

## PRD Task Mapping

| Area | Files | PRD Tasks | Priority |
|---|---|---|---|
| Toolchain & branch hygiene | `go.mod`, git branches | P0-12, P0-13 | HIGH |
| Atomic state writes | `cmd/belmont/fsutil.go` (new helper) + 9 PROGRESS writers | P0-1 | CRITICAL |
| Instrumentation | `cmd/belmont/metrics.go` (new), `toolexec.go`, `auto_loop.go` | P0-2, P0-3 | CRITICAL |
| Census | `cmd/belmont/extract.go` (new, `--dry-run` path) | P0-4 | HIGH |
| Verify read path | `skills/belmont/_src/verify.md`, `agents/belmont/verification-agent.md`, `agents/belmont/code-review-agent.md` | P0-5, P0-6, P0-7 | CRITICAL |
| Register/detail split | `cmd/belmont/extract.go`, `state.go`, `validate.go` | P1-1, P1-2, P1-3 | CRITICAL |
| Merge safety | `cmd/belmont/merge_conflict.go`, `worktree.go`, `guards.go` | P1-14, P1-15 | CRITICAL |
| Slice | `cmd/belmont/slice.go` (new), `main.go` dispatch | P1-4 | CRITICAL |
| Cheap path mandatory | `_src/implement.md`, `_src/verify.md`, `_src/next.md`, `_src/status.md`, `_partials/loop-recipe.md` | P1-5 | CRITICAL |
| Ceilings | `cmd/belmont/config.go` (new), `validate.go`, `autocmd.go`, `state.go` | P1-6, P1-7, P1-8 | HIGH |
| Bounded loop | `cmd/belmont/auto_decide.go`, `auto_loop.go`, `guards.go` | P0-8, P0-9, P0-10, P0-11 | CRITICAL |
| Skill payload | `scripts/generate-skills.sh`, `cmd/belmont/install.go`, `_partials/*` | P1-9, P1-10 | HIGH |
| Cacheable dispatch | `agents/belmont/*.md` (frontmatter), `_partials/dispatch-strategy.md`, `cmd/belmont/eval_harness_test.go` | P1-11, P1-13 | HIGH |
| ~~Steering position~~ | — | ~~P1-12~~ **withdrawn** | — |
| Master file & journal | `cmd/belmont/journal.go` (new), `multifeature.go`, `_partials/commit-belmont-changes.md` | P2-1, P2-2, P2-3 | MEDIUM |
| Scheduler correctness | `cmd/belmont/feature.go`, `validate.go`, `autocmd.go`, `docs/feature-auto.md` | P2-4, P2-5, P2-6 | HIGH |
| Measurement controls | `cmd/belmont/toolexec.go`, `local_llms.go`, `metrics.go`, `auto_loop.go` | P0-14, P0-15, P0-16 | CRITICAL |
| Seeded paired suite | `cmd/belmont/bench_harness_test.go` (new, `//go:build bench`), `testdata/bench/` | P0-17, P0-18, P0-19, P0-20 | CRITICAL |
| Rollout | all five repos, `scripts/build.sh` | P3-1, P3-2, P3-3 | CRITICAL |

---

## File Structure

New and changed files in `~/repos/belmont`. Existing files not listed are untouched.

```
cmd/belmont/
├── fsutil.go              # CHANGED: + writeStateFile() atomic helper       (P0-1)
├── metrics.go             # NEW: usage parsing, JSONL record, summary       (P0-2, P0-3)
├── metrics_test.go        # NEW
├── extract.go             # NEW: `belmont extract` + census dry-run         (P0-4, P1-1, P3-1)
├── extract_test.go        # NEW: round-trip proof, corrupted fixtures       (P1-2)
├── slice.go               # NEW: `belmont slice`                            (P1-4)
├── slice_test.go          # NEW
├── config.go              # NEW: project .belmont/config.json               (P1-6, P2-6)
├── config_test.go         # NEW
├── journal.go             # NEW: append-only JSONL + master render          (P2-1, P2-2, P2-3)
├── journal_test.go        # NEW
├── state.go               # CHANGED: detail-tier aware read, write ceiling  (P1-1, P1-8)
├── validate.go            # CHANGED: + tier integrity, dangling dep, dup ID (P1-2, P2-4)
├── feature.go             # CHANGED: computeWaves dangling-dep fix          (P2-4)
├── merge_conflict.go      # CHANGED: accounting check, total order          (P1-14, P1-15)
├── worktree.go            # CHANGED: merge accounting on progress sync      (P1-14)
├── guards.go              # CHANGED: pre-dispatch gates, lane-scoped pause  (P0-9, P0-11)
├── auto_decide.go         # CHANGED: verify-round cap, lane-scoped pause    (P0-9)
├── auto_loop.go           # CHANGED: trajectory signal, clean-context retry (P0-8, P0-10)
├── autocmd.go             # CHANGED: ceiling preflight, slot cap, messages  (P1-7, P2-6)
├── toolexec.go            # CHANGED: usage extraction; model chain for all  (P0-2, P0-14)
├── local_llms.go          # CHANGED: chain generalised beyond pi/opencode   (P0-14)
├── bench_harness_test.go  # NEW: //go:build bench — seed, run, analyse      (P0-17..P0-20)
├── testdata/bench/gen.go  # NEW: //go:build bench — calibrated seed emitter (P0-17)
├── install.go             # CHANGED: per-host skill files, not symlinks     (P1-9)
├── main.go                # CHANGED: subcommand dispatch + help             (P1-4, P0-4)
└── eval_harness_test.go   # CHANGED: + slice fixture, + bounded-loop fixture (P1-4, P0-9, P1-13)

skills/belmont/
├── _src/verify.md         # CHANGED: read-once, VERIFY.md, slice           (P0-5, P0-6, P0-7, P1-5)
├── _src/implement.md      # CHANGED: slice, clean-context retry            (P1-5, P0-10)
├── _src/next.md           # CHANGED: slice                                 (P1-5)
├── _src/status.md         # CHANGED: drop --format json recommendation     (P0-6)
├── _src/repair.md         # CHANGED: drop --format json recommendation     (P0-6)
├── _src/reset.md          # CHANGED: drop --format json recommendation     (P0-6)
├── _src/loop.md           # CHANGED: host-conditional recipe include       (P1-9)
├── _partials/loop-recipe.md        # CHANGED: corrected scale factor       (P0-6)
├── _partials/dispatch-strategy.md  # CHANGED: registered agent names       (P1-11)
└── _partials/commit-belmont-changes.md # CHANGED: journal, not master edit (P2-1)

agents/belmont/*.md        # CHANGED: + YAML frontmatter (6 files)          (P1-11)

docs/
├── feature-auto.md        # CHANGED: correct milestone syntax              (P2-5)
├── cli-commands.md        # CHANGED: + slice, + extract
└── proposals/
    ├── 0007-detail-tier.md      # NEW  (M3 contract change)
    ├── 0008-size-ceilings.md    # NEW  (M5 contract change)
    └── 0009-master-and-journal.md # NEW (M9 contract change)

knowledge/cross-cutting/
├── context-budget.md      # NEW: the read-path contract, slice-is-not-cache
└── state-atomicity.md     # NEW: why truncate-then-write was the real cause

go.mod                     # CHANGED: toolchain directive                   (P0-12)
```

---

## File-Format Specifications

### The register (`{base}/PROGRESS.md`) — grammar unchanged

The task-line and milestone-heading grammar is **not changing**. It is parsed at seventeen sites across three dialects (`state.go:484` and `guards.go:458` carry the same `depsRe`), and changing it buys nothing measurable. Every change below is additive or extractive.

- Milestone heading: `### M<N>: <name>` optionally followed by `(depends: M<n>[, M<n>]*)`
- Task line: `- [<marker>] <task-id>: <title>`
- Markers: `[ ] [>] [x] [v] [!] [-]` — fixed vocabulary, unchanged

### The detail tier (new, opt-in) — `{base}/details/M<N>.md`

One file per milestone. **Absent directory means exactly today's behaviour** — this is a hard requirement; 139 existing feature directories across the five repos must be unaffected until opted in.

```markdown
# Detail: M3 — Split the register safely

## P1-1
The register's indented body for P1-1, verbatim as it appeared under the task line.
Free markdown. No length limit.

## P1-2
> Superseded 2026-09-02: acceptance criteria narrowed after the M1 census.

The current body only. The previous text is in git history, which is where
history belongs — this is what stops the detail tier becoming the diary the
register became (207 in-place rewrite markers in one file today).
```

- One `## <task-id>` heading per task, in register order.
- A rewritten body **replaces** the old one; a `> Superseded YYYY-MM-DD: <reason>` line is recorded above it. Both copies are never kept inline.
- Two branches adding different tasks never conflict (different lines). Two branches rewriting the same task conflict loudly — the correct outcome.

### Project config — `.belmont/config.json` (new, optional)

Stdlib `encoding/json`, same `config` struct family as the existing user-level `~/.config/belmont/config.json` (`loadConfigSource`, `main.go:501`; `configPaths`, `main.go:~520`). Resolution order: project → user → built-in default. **JSON, not YAML**: the binary has zero dependencies and no `go.sum`; `models.yaml` is already hand-parsed and a second hand-rolled parser for two integers is not worth its maintenance surface.

```json
{
  "progress_warn_bytes": 51200,
  "progress_error_bytes": 102400,
  "task_line_max_chars": 400,
  "parallel_slots": 2
}
```

**Absent file = no limits = today's behaviour.** An absent individual key means that limit is off, not that it takes a default.

Threshold rationale, from the measured distribution of all 139 feature `PROGRESS.md` files (p50 8.5 KB, p75 23 KB, p90 60 KB, p95 100 KB, p99 360 KB, max 1.86 MB):

- `progress_error_bytes: 102400` — 100 KB is 25,000 tokens at the 4-bytes-per-token convention every measurement in the PRD uses. The error threshold **is** the success criterion, not a number sitting beside it. Trips 7 of 139 files today.
- `progress_warn_bytes: 51200` — trips 20 of 139, the top 14%. A real signal, not noise.
- `task_line_max_chars: 400` — p75 of 5,138 measured task lines is 436, p50 is 142. Rejects narrative prose; leaves every plausible title alone.

Bytes rather than tokens because a zero-dependency Go binary has no tokenizer.

### Metrics — `.belmont/metrics/<feature>.jsonl` (new, **gitignored**)

Added via the existing `ensureGitignoreEntry` mechanism in `fsutil.go`, alongside `.belmont/auto.json` and `.belmont/worktrees/`. Local-only: bookkeeping commits are already 41–46% of commit volume in these repos.

```json
{"run":"...","feature":"throughput","milestone":"M3","phase":"implement","tool":"claude","wall_ms":2814000,"input":30903,"output":8871,"cache_creation":22387,"cache_read":8506,"critical_path":true,"input_semantics":"excludes_cache_read","model_requested":"claude-opus-4-8-20260112","model_served":"claude-opus-4-8-20260112"}
{"run":"...","feature":"throughput","milestone":"M3","phase":"verify","tool":"claude","wall_ms":1102000,"input":null,"output":null,"cache_creation":null,"cache_read":null,"note":"tool reports no usage","model_requested":"claude-opus-4-8-20260112","model_served":null,"model_note":"served ID not verified against a live run"}
{"run":"...","feature":"throughput","milestone":"M3","phase":"__terminal__","tool":"claude","wall_ms":0,"outcome":"completed","exit_reason":"all tasks verified","tasks_verified":5,"tasks_total":5}
```

#### Model identity — `model_requested` / `model_served` (new, P0-14)

`model_requested` is the exact ID Belmont passed to the tool. `model_served` is the ID the tool reports it actually used, where it reports one; `null` plus a `model_note` where it does not, following the `usageUnavailableNote` precedent already set for gemini and cursor (`metrics.go:185`). **Do not assume a tool's self-reporting schema — verify it against a live run**, exactly as `P0-M1-FIX-4` did for codex's `cached_input_tokens` semantics.

Recording only the request is not sufficient, and this is the whole point of the control. A vendor serving something other than what was asked, silently, over a months-long passive window, is indistinguishable from your own improvement if the record holds the request alone. `belmont metrics` **refuses to aggregate across differing model IDs** and reports `null` with a stated reason plus the per-model breakdown — the same fail-closed rule already applied to `input_semantics`, for the same reason: a figure spanning two models is neither model's figure.

#### Run outcome — the `__terminal__` record (new, P0-15)

One record per run, appended when the loop exits, in the same JSONL. Not a separate file: a run's phases and its outcome belong to one append-only stream, and splitting them invites the two to disagree.

`outcome` is `completed` only when the run's milestone reaches **all `[v]`** — the feature's own success bar, where `[v]` and not `[x]` is the unit. Anything else is `incomplete`, with `exit_reason` carrying which: `max-iterations`, `blocked`, `circuit-breaker`, `tool-error`, `session-limit`.

Completion rate = `completed / attempted`. **Failed runs count in the denominator and are excluded from the ratio computation, and the harness never retries.** A retry would silently repair the completion rate, which is precisely the number published to stop a run that terminated cheaply from flattering a median.

Usage source per tool — no extra API calls, exact figures rather than estimates:

| Tool | Invocation today | Usage available |
|---|---|---|
| claude | `-p --output-format stream-json --verbose` (`toolexec.go`) | Yes — `result` event `usage` block |
| codex | `exec --json` | Yes |
| gemini | `-p --output-format json` | Yes |
| cursor | `-p --force --output-format json` | Yes |
| copilot | `-p --yolo` | No — wall-clock only |
| pi | `-p` (plain text) | No — wall-clock only |
| opencode | `run` (plain text, deliberately not `--format json`) | No — wall-clock only |

`input`/`output`/`cache_*` are `null` with a stated `note` where the tool cannot report. **This degradation is load-bearing**: opencode is the Oct 2026 local-LLM target, and the plan must not pretend it will report tokens.

`input_semantics` records what that tool's `input_tokens` counts, written at ingest: `excludes_cache_read`
(claude — the uncached remainder only) or `includes_cache_read` (codex — the whole prompt, verified against a
live codex-cli 0.147.0 run on 2026-08-18, not assumed). A tool with no recorded definition writes `unknown`.
**`belmont metrics` reports a combined `input` only when every contributing record shares one definition**;
where they differ it reports `null` with a stated reason plus a per-tool breakdown (`tools_detail`), the same
way it reports a host that cannot measure. Summing the two definitions would give a figure that is neither the
uncached total nor the prompt total — an undefined number in the baseline every later milestone is scored
against, which is the "never estimate" rule failing by a different door.

`critical_path` marks phases on the longest dependency chain. Latency correlates with critical-path tokens at r = 0.901 — attributing tokens to the chain, not merely summing them, is what makes P3-3's verdict meaningful.

### Journal — `.belmont/journal/<feature>/M<N>.jsonl` (new, **committed**)

One file per milestone plus `main.jsonl` for entries outside any milestone. Structurally conflict-free: two worktrees on different milestones write different paths, so git unions them with no `merge=union` driver, no `.gitattributes`, and no rule anyone has to remember. A union driver would silently interleave lines out of order — the exact class of silent loss P1-14 exists to prevent.

```json
{"at":"2026-08-19T09:14:02Z","feature":"throughput","milestone":"M3","event":"implemented","detail":"P1-1 extraction landed; round-trip proof green on 4 fixtures"}
```

Rotation by file count, not bytes: when a milestone's journal exceeds the configured line count it rolls to `M<N>.1.jsonl`.

---

## Command Specifications

### `belmont slice` (new) — P1-4

```
belmont slice --feature SLUG --milestone M3 [--root PATH] [--format text|json]
```

Emits, to stdout only, exactly the material one phase needs for one milestone: its heading and dependency annotation, its task lines with identifiers and markers, and its detail (from `details/M<N>.md` when present, from the register's indented bodies when not). Nothing else — no monorepo section, no global counts, no archived features, no other milestone.

- **Default `--format text`.** JSON measured 682 KB against 57 KB for the default text output on the worst feature — 12× — which is the very defect P0-6 removes. JSON stays available for programmatic callers.
- **Writes nothing to disk.** No temp file, no cache, no artefact to invalidate.
- **A distinct verb, not a `status` flag.** This is what makes P1-5 auditable: you can grep the skills and prove the cheap path is mandatory. You cannot grep for "used `status` correctly".

**State this explicitly in `knowledge/cross-cutting/context-budget.md`, because it will be read by someone who remembers the rejection**: Belmont's institutional memory rejected *caching the status command's output*. `slice` computes fresh from the files on every invocation and leaves nothing behind. Computing on demand is not caching.

### `belmont extract` (new) — P0-4, P1-1, P3-1

```
belmont extract --feature SLUG [--dry-run] [--all] [--root PATH]
```

Moves each milestone's indented task bodies out of the register into `details/M<N>.md`, leaving the task line, identifier, marker and title in place.

- `--dry-run` reports what would move and the resulting register size, and writes nothing. **`--all --dry-run` is the census** (P0-4): it runs over every feature directory in the repo and prints the size distribution plus an explicit list of any feature whose extracted register would still exceed `progress_error_bytes`.
- **Round-trip proof before any write.** The real run re-inlines its own output in memory and compares byte-for-byte against the original. Mismatch aborts with the offending milestone named and nothing written. This is the one operation in the feature that destroys information if it is wrong, on the files holding the most of it — a proof, not a promise.
- One commit per migrated feature, so revert is one command.

### `belmont validate` (extended) — P1-2, P2-4

New deterministic checks, each naming its specific offender. Agent judgement is not involved.

| Check | Reports | Task |
|---|---|---|
| Task in register, no detail record | task id, milestone | P1-2 |
| Detail record, no task in register | task id, file | P1-2 |
| Orphaned `details/M<N>.md` — no such milestone | file, milestone id | P1-2 |
| Register exceeds `progress_error_bytes` | file, size, threshold | P1-6 |
| Duplicate task identifier | id, both line numbers | P1-14 |
| Dependency naming a milestone that does not exist | both milestone ids | P2-4 |

### `belmont status` (unchanged behaviour, changed advice) — P0-6

No code change. The four skills recommending `--format json` as a quick summary stop doing so; each names the cheapest path that answers its question. The single existing warning in `_partials/loop-recipe.md` cites ~3× — corrected to the measured **12×** at 529 tasks (682,148 B JSON against 57,145 B text).

### Model resolution — the chain generalised (new) — P0-14

`modelTiers["claude"]` is `haiku` / `sonnet` / `opus` (`toolexec.go:402`) — the drifting aliases the measurement controls forbid. Pi and opencode already resolve through a layered chain (`local_llms.go`, `resolvePiModelFlags` / `resolveOpencodeModelFlags`); **generalise that chain to every tool** rather than hardcoding IDs, which is what `AGENTS.md` says to do when extending this area.

```
BELMONT_<TOOL>_MODEL_<TIER>  /  BELMONT_<TOOL>_PROVIDER_<TIER>   env, per tier
  > BELMONT_<TOOL>_MODEL     /  BELMONT_<TOOL>_PROVIDER          env, all tiers
  > <project>/.belmont/local-llms.json
  > ~/.belmont/local-llms.json
  > modelTiers[tool][tier]                                        unchanged defaults
```

**Default behaviour for every existing user is unchanged** — an absent chain falls through to today's aliases. The aliases are deliberately *not* replaced: they exist so users track the current frontier, and pinning them globally would commit the project to editing that table on every vendor release, or to shipping a build that silently holds users on a superseded model. The measurement pins itself; it does not pin everybody else.

### The bench harness (new, never shipped) — P0-17 … P0-20

Not a subcommand. Build-tagged test files, mirroring `eval_harness_test.go` exactly — the pattern this repo already uses for "drives a real tool, spends real tokens, must never run in CI by accident".

```bash
go test -tags bench ./cmd/belmont                              # offline, free, in CI
BELMONT_BENCH_LIVE=1 go test -tags bench -timeout 0 \
    -run TestBenchLive ./cmd/belmont                           # spends; owner-run
```

`-timeout 0` is required for the same reason Tier 2 requires it (`AGENTS.md:59`): without it Go's ten-minute default orphans the live tool child, and these runs are measured in tens of minutes.

| Test | Tag only | Live | What it proves |
|---|:-:|:-:|---|
| `TestBenchSeed` | ✅ | | The generated seed reproduces byte-for-byte from a fixed seed value |
| `TestBenchAnalysis` | ✅ | | Median, Hodges–Lehmann interval and completion rate against fixture pairs |
| `TestBenchGuards` | ✅ | | The testbed-location refusal fires; the prose-sentinel assertion fires |
| `TestBenchLive` | | ✅ | The paired suite itself |

Three of the four run offline and free. That split is deliberate and load-bearing: **P3-3's verdict is computed by `TestBenchAnalysis`'s code**, and a statistic that decides whether this feature succeeded cannot be the one piece with no test behind it.

**Refusals, both hard.** The harness aborts if the resolved testbed path lies inside any of the five product repositories or inside this one — a measurement instrument that can write into what it measures is the 2026-08-18 failure with a different entry point. And it aborts if the arm's own installed prose did not resolve: each arm installs from a **fixed commit checkout** into the testbed, and a sentinel string from that arm's `SKILL.md` must be observed before the timed run begins. Without that check, both arms silently resolve prose from user-level `~/.agents/skills/belmont/`, and M2, M4 and M7 — whose entire deliverable is prose — measure as zero improvement.

### The passive arm

No commands and nothing scheduled. Once `belmont-pre` is on PATH (prerequisite 9) every normal run in the five product repos and in this repository records metrics as a side effect, at zero extra token cost and with no interference.

Three properties make it genuinely pre-change data, and all three are consequences of the binary being frozen rather than of anything anyone remembers to do:

1. **The binary does not move.** `belmont-pre` is built once at M1's merge commit and is not refreshed as M2–M13 land. §Distribution's "do not run this between M1 and M11" is unchanged and is what enforces it.
2. **The prose does not move either.** `.claude/` is gitignored here, so worktrees resolve `/belmont:*` from user-level `~/.agents/skills/belmont/`, frozen at v0.11.0 by prerequisite 0 and master rule 2. Prose changes merging into `main` do not reach the running loop.
3. **`.belmont/metrics/` cannot dirty the tree it measures.** `P0-M1-FIX-12` guarantees the ignore rule on the `auto` path itself, not only at install time — which is why this arm needs no per-repo setup step at all.

The window opens at prerequisite 9 and closes at the M11 rollout. `P0-16` verifies it is genuinely open and records the date and the frozen commit; it writes nothing into any product repository, because a step that both opens a measurement and reports on it is free to report what it wishes were true.

---

## Go Implementation Notes

### P0-12 — Go toolchain (M1, first task)

`go.mod` declares `go 1.21` with no `go.sum` and zero requires. Bump the directive, run the full suite, commit alone. **Sequence it before P0-2 and P0-3** so the baseline is captured on the toolchain everything after it is built with — otherwise the before and after were built differently and P3-3's comparison is contaminated.

### P0-13 — Branch triage (M1, second task)

35 unmerged remote branches. Classify every one as merge / rebase / abandon and record the verdict in `docs/proposals/NEXT-SESSION.md` **before** any milestone touches shared files. Five sit directly on ground this feature rewrites:

| Branch | Last commit | Collides with |
|---|---|---|
| `origin/feat/loop-efficiency` | 2026-08-14 | M6 (bounded loop) |
| `origin/fix/wave-merge-state-loss` | 2026-08-07 | M3 (P1-14, P1-15) |
| `origin/fix/state-readers-and-live-views` | 2026-08-14 | M4 (read path) |
| `origin/fix/unrecognised-task-markers` | 2026-08-11 | M3 (merge, duplicate IDs) |
| `origin/feat/verify-dedup` | 2026-04-12 | M2 (P0-5) |

Also note: `origin/main` is at v0.9.1 (2026-04-10) while local `main` is at v0.11.0. Push `main` to `origin` as part of this task so the fork's default branch stops lying about what it contains.

### P0-1 — Atomic state writes (M1)

Add one helper to `fsutil.go`:

```go
// writeStateFile writes data to path so that a concurrent reader observes
// either the complete previous content or the complete new content, never a
// partial file. Writes to a sibling temp file, fsyncs, then renames — rename
// is atomic within a filesystem, which is why the temp file must be a sibling
// and not in os.TempDir().
func writeStateFile(path string, data []byte, perm os.FileMode) error
```

Replace **every** bare `os.WriteFile` that writes Belmont state. The PRD's figure of nine is the count of **`PROGRESS.md` writers**, which is correct:

| # | Site | File |
|---|---|---|
| 1 | `feature.go` | PROGRESS.md |
| 2–3 | `guards.go` ×2 (`pre.Path`, in `runScopeGuard` and `runEvidenceCheck`) | PROGRESS.md |
| 4 | `merge_conflict.go` | resolved state file |
| 5 | `reverify.go` | PROGRESS.md |
| 6 | `state.go` | PROGRESS.md |
| 7–8 | `repair.go` ×2 | PROGRESS.md |
| 9 | `worktree.go` (merged progress) | PROGRESS.md |

**Refinement on the PRD**: seven further sites write other Belmont state — `reconcile.go` ×2, `steer.go` (STEERING.md), `worktree.go` (STEERING.md, feature marker, `auto.json` ×2). P0-1 covers all sixteen. The PRD says "every state writer, not a subset"; sixteen is what that resolves to on v0.11.0.

Proof: a concurrent read-during-write test with a negative control. **An atomicity claim with no concurrent test is an assertion, not a result.** *(Corrected 2026-08-19, `P0-M1-FIX-32`: this said "under `go test -race`". The test compares file contents and asserts filesystem atomicity, which `-race` does not instrument; the negative control — the same harness against the old `os.WriteFile` — is what makes it a result rather than a test that cannot fail.)*

### P0-2 / P0-3 — Instrumentation and baseline (M1)

`toolexec.go` already assembles the invocation; add a per-tool usage extractor beside it and a `metrics.go` that appends one JSONL record per phase. Wall-clock is measured around the `exec.Command` in `auto_loop.go`. Critical-path marking comes from `computeWaves` — a milestone is on the critical path if it lies on a longest chain through the DAG.

**P0-3 was re-scoped on 2026-08-19 and now covers the read-path half only** — the register-size and `status` output figures in `BASELINE.md`, which are captured and carry commit evidence at `ff06d45`. It records the Belmont commit it was taken against, and — since `P0-M1-FIX-14` — the revisions of the two repositories it measured.

*Softened 2026-08-19 by `P0-M1-FIX-14`; this read "reproduce exactly", unqualified.* What reproduces exactly is **the capture**, and 40 of the 43 measured registers on re-run one day later. The **procedure** did not, because it pinned the Belmont binary but not the working trees it measured, and three registers had already grown. Both halves are now pinned, and the distinction is stated rather than collapsed: a figure being exact against its own capture is not the same claim as a command being re-runnable.

The cost half — tokens and wall-clock per verified milestone — moved to **M12 and M13**, because it turned out to be a milestone's worth of work rather than a task's: a model-pinning mechanism that does not exist, an outcome field that does not exist, a seeded testbed, a paired runner and an analysis path. `P0-3a`, the placeholder that carried it, is withdrawn `[-]`. See §M12 and §M13 below.

### P1-1 / P1-2 / P1-3 — The split (M3)

Extraction is mechanical: 62.1% of the worst register's bytes are indented bodies beneath task lines, and 25.0% is the task head lines themselves. Indentation is the separator; no re-modelling is required and none should be attempted.

`state.go`'s reader becomes detail-tier aware: when `details/M<N>.md` exists, task bodies are read from there; when it does not, from the register

**Extraction alone does not meet the 25,000-token read budget, and M3 must not be reported as if it did.** *(Carried forward from `P0-4a`'s resolution, 2026-08-18; given a destination 2026-08-19 by `P0-M1-FIX-29(c)` — until then it existed only inside `P0-4a`'s own task body, which is not a home.)* The census measured **five registers still over the 100 KB / 25,000-token ceiling after extraction**, and three of the five gain essentially nothing from it (0%, 0%, 7.6%) because their bytes are task *head* lines, not indented bodies — `CENSUS.md` §The open question, answered has the per-register figures. What closes the gap for those five is **M4's `belmont slice`**, serving one milestone's slice on demand, two waves later.

Two consequences for how M3 is verified. Its acceptance criteria are about **round-trip integrity** — no task lost, no state lowered, byte-for-byte reconstruction — and deliberately **not** about any register landing under the ceiling; a criterion M3 cannot meet alone would fail it for M4's absence. And no M3 report may claim the read budget is met: the honest statement is that extraction removes 32.6% of estate bytes and leaves five registers needing the slice. exactly as today. Status output must be **byte-identical** before and after extraction — that is the acceptance criterion, and it is checkable mechanically.

### P1-14 / P1-15 — Merge safety (M3)

`merge_conflict.go` and `worktree.go` gain an accounting check that must hold before a merged register is written:

1. Every task identifier present in either input appears **exactly once** in the output.
2. No resulting state is **lower** in the total order than either input's.
3. Every input task's body either survives or is reported as replaced, with both versions named.

A merge that cannot satisfy all three is **refused and escalated**, never written. The escalation path already exists.

**State precedence — a single total order, applied uniformly in both directions so merging is order-independent:**

```
[-] withdrawn  >  [!] blocked  >  [v] verified  >  [x] done  >  [>] in-progress  >  [ ] todo
```

Blocked outranks verified. This is what makes the PRD's requirement true rather than aspirational — a blocker raised in one worktree survives a merge against a stale verified from another, which is exactly the four measured blocked-to-verified regressions. It is also the conservative direction: a wrongly surfaced blocker costs someone a look; a silently cleared one costs the invariant the framework rests on. Withdrawn sits top because a withdrawn task no longer exists to have a state.

**Laundering is closed before the order is applied.** A `[v]` arriving by merge with no commit naming that task is demoted to `[x]` and reported. `runEvidenceCheck` (`guards.go:227`) already implements exactly this rule *after a local phase* via `findEvidenceMissingFlips` / `revertEvidenceMissing` — P1-15 is that same rule moved to the merge path. Reuse the functions; do not write a second implementation of the same predicate.

### P0-8 / P0-9 / P0-10 / P0-11 — Bounding the loop (M6)

**P0-8, early-failure signal — deterministic only, no judge.** Fire on conditions visible in the stream Belmont already consumes:

- N consecutive tool errors of the same kind within a phase;
- zero file changes past a configurable fraction of that phase's historical median wall-clock or tokens (P0-2 supplies the medians for free);
- the same file rewritten more than N times within one phase.

Warn always, naming what triggered it. Pause under `--policy autonomous`. **No LLM judge**: across 67 frontier models an LLM-as-router captured *exactly zero* of the available oracle gain, so a judge would add cost, latency and a correlated failure mode to the one mechanism whose entire job is to stop spending. The signal must stay advisory enough not to abort recoverable work — the pilot behind this preserved 8 of 10 usable outcomes.

**P0-9, verify-round cap.** `milestoneLoopState.VerifyFailures` already exists and is already surfaced to `decideLoopActionAI`. After the second failure on the same milestone: mark that milestone's failing tasks `[!]` with a reason naming both failures, and stop that milestone.

*This requires a scheduler change.* `checkHardGuardrails` (`auto_decide.go:346`) currently PAUSEs the **entire run** the moment any task is blocked, which would stop five healthy siblings. Narrow it: under parallel execution a blocked task pauses **only that milestone's lane**; siblings finish and merge. Serial runs keep today's whole-run pause — there is no sibling to protect. This is a correctness fix the wave scheduler needs regardless of P0-9.

**P0-10, clean-context retry.** Prose-level, in `_src/implement.md` and the fix-all path. A fix attempt receives the specification and the failure report only, never the failed attempt's transcript. Forwarding a failed reasoning chain costs up to 34.8 percentage points of accuracy against a fresh attempt.

**P0-11, deterministic gates before dispatch.** Before spawning the two review agents, run: the project's build, test and typecheck commands; `belmont validate`; and a check that commits exist for the milestone's tasks. Any failure writes follow-up tasks deterministically and skips the dispatch entirely.

> **The invariant is untouched, and the plan must say why:** a *passing* gate promotes nothing. `[x]` → `[v]` still requires both review agents and every acceptance-criterion check they run today. The gates can only fail work earlier and more cheaply. This is `runEvidenceCheck`'s principle moved from after the phase to before it.

### P1-9 / P1-10 — Host-conditional builds (M7)

Today one physical `.agents/skills/belmont/<skill>/SKILL.md` serves all six CLIs, and `.claude/commands/belmont/<skill>.md` is a **symlink to it** (verified in repo-3). So `loop.md` carries both the `/loop` recipe (Claude) and the `/goal` recipe (Codex) — `_partials/loop-recipe.md` is 17,075 bytes and is `@include`d twice in one skill, 99.92% identical. Every reader on every host pays for both.

**Design**: convert the per-host symlinks into generated per-host files. `.claude/commands/belmont/`, `.opencode/command/belmont/`, `.cursor/rules/belmont/` and `.windsurf/rules/belmont/` are already distinct install paths. Generate each from `_src` with only that host's `@include` branches; leave `.agents/skills/belmont/<skill>/SKILL.md` as the generic fallback for hosts with no dedicated path. Multi-host support is retained — only what each host's build contains changes.

Mechanism: extend the `<!-- @include name.md key="value" -->` directive in `scripts/generate-skills.sh` with a `host="claude"` attribute; the generator emits per-host variants; `install.go` writes the variant matching each `--tools` entry instead of creating a symlink. `install`/`update` idempotency (`filesEqual`, `install.go:265`) must be updated for file-instead-of-symlink.

**P1-10 measures the right thing.** Total duplication across `_partials` is ~128 KB ≈ 32,000 tokens, but most of that is *across different skills* — and only one skill is loaded per invocation, so aggregate size is disk, not context. Measure **per-skill, per-host built size**: that is what a reader actually pays. Claude's `loop.md` shedding a full 17 KB recipe is the headline number; report it as such rather than as a total.

### P1-11 / P1-13 — Cacheable dispatch (M8, depends on M7)

Root cause, established: the six files in `agents/belmont/` carry **no YAML frontmatter**. `.claude/agents/belmont` is symlinked into place by `install.go`, but without frontmatter nothing registers as a dispatchable subagent — which is precisely why `_partials/dispatch-strategy.md` falls back to `subagent_type: "general-purpose"` plus `_partials/identity-preamble.md`'s "**MANDATORY FIRST STEP**: Read the file `.agents/belmont/<agent>.md` NOW". The definition (up to 24,460 bytes for the implementation agent, 91,443 bytes across all six) therefore arrives inside the sub-agent's own tool-result stream, after the variable prompt — where no cache prefix can hold it. A live dispatch measured 30,903 input tokens with 22,387 cache-creation against 8,506 cache-read: a 72% miss on a trivial task.

**P1-11 opens with a spike.** Add frontmatter to one agent, dispatch it by its registered name, and confirm three things: it retains file-edit and shell access; `models.yaml` tier overrides still land via the dispatch `model:` parameter; and the registered system prompt does not displace the FORBIDDEN ACTIONS blocks the research agents rely on. **If any of those fails, P1-11 is withdrawn as `[-]`** exactly as the PRD instructs — worked around, never.

Two constraints on the frontmatter: omit `model:` (it would fight `models.yaml`), and omit `tools:` unless the spike shows otherwise (an omitted `tools:` key inherits full access; a partial list would silently strip the shell the implementation agent needs). Registration is Claude Code-only, so the dispatch prose becomes host-conditional — which is why M8 depends on M7.

**P1-13** runs the change through `cmd/belmont/eval_harness_test.go` (`-tags eval`, six fixtures on `testdata/eval/`) and against measured cache-read versus cache-creation figures. **Reverting on a negative result is the expected outcome path, not a failure** — the framework already contains one optimisation that measurement disproved, which is why the harness exists.

### ~~P1-12~~ — withdrawn

Marked `[-]`. The mechanism it names is real — steering is prepended at `auto_loop.go:259` and `auto_loop.go:404`. The saving is not. Proposal `0004-context-budget-with-evidence.md` rev 2 measured prefixed against suffixed steering on `claude -p` and got **identical** cache figures both ways (13,233 creation / 21,689 read), because Belmont assembles a ~549-byte user message and the cache breakpoint sits at its end; intra-message reordering recovers nothing. Recorded here so the reasoning is not rediscovered.

### P2-1 / P2-2 / P2-3 — Master file and journal (M9)

repo-3's master `.belmont/PROGRESS.md` is 498,879 bytes, of which 420,417 is a 330-row hand-appended narrative table nobody authors as a human artefact — yet four skills instruct agents to hand-edit it. Generate it from the feature registers, mark it clearly as generated, validate it as current during preflight, and stop instructing agents to edit it.

**The feature-level register stays authored.** Commit-history measurement showed regenerating it produces large spurious diffs. Only the master file becomes generated.

### P2-4 / P2-5 / P2-6 — Scheduler correctness (M10)

**P2-4** — `computeWaves` (`feature.go:494`) computes in-degree as:

```go
for _, dep := range m.Deps {
    if dm, ok := byID[dep]; ok && !milestoneAllDone(dm) {
        count++
    }
}
```

An unknown `dep` fails the `ok` test and contributes nothing, so a typo silently promotes that milestone into wave 1. The feature-level equivalent already errors correctly at `validate.go:28` (`"feature %q depends on %q which does not exist"`). Add the milestone-level check with the same shape, and do not treat the milestone as unblocked.

**P2-5** — `docs/feature-auto.md:192-197` documents milestone syntax as list items (`- [x] M1: Project scaffolding`). The parser requires `### M<N>:` headings (`state.go:484`, `guards.go:458`). Anyone following the docs gets zero parsed milestones and a confusing halt. Documentation-only fix.

**P2-6** — Two defects. Parallel execution engages only when at least one milestone declares a dependency (`autocmd.go:280`, `if len(m.Deps) > 0`); otherwise a run with `--max-parallel` set falls through to serial with no message. And `--max-parallel` defaults to 5 (`autocmd.go:74`) with no notion of backend capacity.

Effective concurrency becomes `min(--max-parallel, parallel_slots)`, printed on every run. A run that requested parallelism but found no declared dependencies says so and why. Absent `parallel_slots` means today's behaviour. **Configured, not auto-detected**: the binding constraint is memory bandwidth and API rate limits, which no portable probe can see, and the measured penalty for overcommitting is worse than running serially — the same graph yields 14.31× on an unbounded backend and 1.43× on a two-slot local one, with overcommit oscillating between near-two-slot and near-sequential.

### P0-14 / P0-15 / P0-16 — Trustworthy measurement (M12)

**P0-14** — Generalise the model-resolution chain (§Command Specifications) and add `model_requested` / `model_served` / `model_note` to `metricsRecord` (`metrics.go:149`). `summariseMetrics` gains the same fail-closed refusal it already applies to `input_semantics`: no combined figure across differing model IDs, `null` plus a stated reason and a per-model breakdown instead. Verify each tool's self-reporting schema against a live run before writing a parser for it; where it is unverified, say so in the record rather than estimating.

**P0-15** — The `__terminal__` record (§Metrics). `recordPhaseMetrics` (`auto_loop.go:614`) already knows the run ID and the metrics root; the terminal record is appended where the loop exits, reading the milestone's final task states through `canonicalMarker` rather than comparing raw bytes. Watch the wave path: `P0-M1-FIX-1` established that `MetricsRoot` and `Root` differ under worktrees, and the terminal record must follow `MetricsRoot` like every other record or it is written into a tree that is about to be deleted.

**P0-16** — Verify the passive window is open, and record it. Report-only: for this repository and each of the five product repos, confirm the resolved `belmont` is the frozen commit (via `belmont version`), that `.belmont/metrics/` is ignored, and that records are accruing. Write the window-open date and the frozen commit into `BASELINE.md` and `baseline.json`. **It writes nothing into any product repository** — per §Constraints in the PRD, measurement there is observational only, and `P0-M1-FIX-12` already guarantees the ignore rule from the `auto` path so there is nothing to install.

### P0-17 / P0-18 / P0-19 / P0-20 — The seeded paired suite (M13)

**P0-17 — the seed.** A generator (`testdata/bench/gen.go`, tag `bench`) emitting a register calibrated to the measured pathological profile: 1,860,979 total bytes, 30 milestones, 529 tasks, 458 with detail, and the head-versus-indented split recorded in `census.json`. Deterministic from a fixed seed value, so `TestBenchSeed` asserts the exact byte count offline and free, forever.

**Generated, never copied.** The register it models is a pre-launch product's complete feature planning, and this repository is a public fork of a third party's. Read-path cost is a function of bytes and structure, not of words, so a calibrated generator is byte-equivalent for the purpose while carrying no product content — and git stores ~100 lines instead of a 1.86 MB blob. The same generator emits a median-sized 17 KB register from a different parameter set, which is what makes the typical case measurable later at no extra cost.

The testbed itself is a trivial buildable project under `os.MkdirTemp` — no dependency install, no database, no product test suite. Those would be three variance sources bolted onto a measurement whose entire design is isolating one, and the quantity being measured does not read them.

**P0-18 — the runner.** Both arms from one `baseline-seed` tag, `git reset --hard` between every run, arm order alternating across replicates (A/B, B/A, A/B).

**Strictly serial, and this is not a preference.** Wall-clock is one of the two measured outcomes, and this feature's own research is that concurrent local runs are memory-bandwidth bound — the same graph yields 14.31× unbounded and 1.43× on two slots. Running the suite in parallel would corrupt the very number it exists to produce.

Arm B migrates before its timed run: `belmont extract` on the testbed, then the run on the migrated register. That is the post-M11 world, and measuring arm B on an un-migrated pathological register would measure a state that will never exist. The migration's own cost is recorded and reported **separately**, never amortised into the per-run figure. **Acceptance criterion on P0-17, not a hope**: the seed must be chosen such that extraction lands it under `progress_error_bytes`, because `CENSUS.md` records five of 82 registers still exceeding the ceiling after extraction — and if the seed is one of those, arm B refuses to start and the suite produces nothing.

Per run, recorded: arm, seed SHA, Belmont commit, `model_requested`/`model_served`, the `belmont metrics --format json` payload, and the terminal outcome.

**P0-19 — the A/A pilot.** Both "arms" on the same pre-change build, ~3 seeded milestones × 3 replicates. It measures two things nobody currently knows: the **within-task sd** of per-pair log-ratios, which sets the pair count (6 at sd 0.3, 12 at 0.7, 20 at 1.0, per the 2026-08-19 research), and the **per-run wall-clock** of the seeded design, which is what makes the M11 block schedulable. It is also an honesty check: an A/A whose median ratio departs materially from 1.0 means the harness is not measuring what it believes it is, and that must be resolved before any real pair is run.

**When sd and the clock conflict, sample size wins.** If the derived pair count will not fit the available block, cut in this order: the seeded milestone's work, then verify rounds (`--from M<n> --to M<n>`), then the phases measured. **The pair count is the last thing to give, and if it gives, the report says so.** The reason is this feature's own record: `P0-M1-FIX-6` through `FIX-11` are five consecutive rounds correcting claims stated beyond their evidence, in these very files. A headline "50% reduction" published with an interval too wide to support it is that same failure with a statistic instead of a sentence.

**P0-20 — the analysis, one path for both consumers.** Per-pair log-ratios, **blocked by task and never pooled** — with heterogeneous tasks that is the only defensible treatment at this n ([Miller 2024](https://arxiv.org/html/2411.00640v1)).

- **Point estimate**: the sample median of per-pair reductions. This is what criterion 26 says the pass/fail rule is, worded literally, so it is what gets computed.
- **Interval**: the exact Wilcoxon signed-rank / Hodges–Lehmann interval. Exact at n = 6–20, where a percentile bootstrap is unstable, and it matches the signed-rank power calculation that sized the suite in the first place.
- **Completion rate**, published beside every median.

The same function serves P0-19's pilot and P3-3's verdict, so the two halves of the comparison cannot diverge — the same reasoning that put `belmont metrics` behind both P0-3 and P3-3.

---

## Prior Art

`~/repos/belmont` carries a proposals process and 35 unmerged branches. Both are load-bearing inputs.

### `docs/proposals/0004-context-budget-with-evidence.md` (rev 5, on `origin/docs/pr-proposals`)

Six adversarial review passes deep. **M2 treats it as its specification** for the two halves that are additive here:

- the **conditional archive preference** — prefer `MILESTONE-*.done.md` over re-deriving from specs, *but only where its sections are populated*, because a `/belmont:next` run archives `[Not populated — …]` placeholders (`next.md:114-118`). Preserve the `[Not populated —` **prefix match**; a rewrite matching exact wording silently breaks the design-skip path.
- the **read-once discipline** across `verify.md`'s four archive-read sites (`:30`, `:75`, `:119`, `:156` — the orchestrator, one step, and both sub-agent dispatch prompts).

**0004's Optimisation A (lazy Setup) is superseded, not rejected.** It reorders the Setup block to read `PROGRESS.md` first and then only the selected milestone's PRD sections. P1-5 in M4 replaces that first read with `belmont slice` two waves later. Implementing an ordering fix that a later milestone deletes is churn with a merge cost. Recorded here so 0004's reasoning survives the decision.

Also inherited from 0004, and still true: the NOTES conflict must be **resolved, not stepped around**. `references/implement-milestone-template.md:45-47` requires copying `{base}/NOTES.md` and `.belmont/NOTES.md` into the MILESTONE's Learnings section, and `implement.md:52` makes that template mandatory. Keep NOTES eager — they are small and feed a mandatory section — and take the saving from TECH_PLAN / master TECH_PLAN instead.

### Proposals owed by this feature

Three milestones change a contract that 139 existing feature directories and seventeen parsing sites depend on. Each gets a proposal in the repo's own format before implementation; the other eight ship as ordinary PRs.

| Proposal | Milestone | Contract changed |
|---|---|---|
| `0007-detail-tier.md` | M3 | `details/M<N>.md`, extraction, merge accounting, state precedence |
| `0008-size-ceilings.md` | M5 | `.belmont/config.json`, write-time rejection, preflight refusal |
| `0009-master-and-journal.md` | M9 | Generated master file, `.belmont/journal/` |

Open upstream PRs opportunistically. Never block a milestone on one.

---

## Decision Log

| Decision | Alternatives considered | Rationale |
|---|---|---|
| Self-host in the fork at `~/repos/belmont` | Manual implementation from this workspace; long-lived integration branch | `belmont auto`, worktrees and the wave structure are the only way the success bar (tokens/wall-clock per *verified* milestone) can be measured at all. Contained by pinning the orchestrating binary |
| Absorb 0004; triage 35 branches in M1 | Land 0004 as a prerequisite PR; build from the PRD alone | Five branches sit on files M3/M4/M6 rewrite. Finding that at merge time is the expensive way |
| Detail tier as `details/M<N>.md`, one file per milestone | Single `DETAIL.md`; one file per task | Parallel worktrees own different milestones, so concurrent writes hit different files and union with no conflict — the hazard P1-14 exists to close. Per-task would spawn 529 files for one feature |
| Supersession = replace + one-line note; old text in git | Keep both copies under a Superseded heading | Keeping both is the 207-marker diary pattern with a tidier label. Git already holds history |
| `belmont slice` as a new subcommand | `belmont status --milestone M3` | A distinct verb makes P1-5 auditable by grep, and keeps status's global sections out of a milestone-scoped read |
| Parse each tool's own JSON usage; `null` where unavailable | Wall-clock only; estimate from prompt bytes | Zero extra calls, exact figures. Estimating from what Belmont *sends* would score M2/M4 — the largest wins — as zero, because the problem is what the agent *reads* |
| Per-host generated files replacing symlinks | Install-time block stripping; per-host embedded skill sets | Block stripping cannot help the shared `.agents/` path when a project installs multiple tools, which is most of them |
| `.belmont/config.json` (stdlib JSON) | `.belmont/config.yaml`; keys inside `models.yaml` | Zero-dependency binary with no `go.sum`; a second hand-rolled YAML parser for two integers is a permanent maintenance surface |
| Warn 50 KB / error 100 KB / 400-char task lines | Tighter (25/50/200); looser (100/250/1000) | 100 KB *is* 25,000 tokens — the error threshold is the success criterion. Tighter warns on 39 of 139 healthy features and gets configured away; looser permits the state the feature exists to prevent |
| Deterministic failure signals, no judge | LLM judge on trajectory; token-budget cap | An LLM-as-router captured exactly zero oracle gain across 67 models. A budget cap fires after the budget is gone — the failure mode P0-8 exists to avoid |
| Gates may fail, never promote | Add a diff-size/no-op gate; `belmont validate` only | Preserves the `[x]`→`[v]` invariant exactly. A no-op gate would misfire on this feature's own three prose milestones |
| Journal as per-milestone JSONL | Per-feature JSONL + `merge=union` driver; markdown bullets | Different paths union with no driver and no rule to remember. A union driver interleaves silently — the loss class P1-14 prevents |
| State precedence `[-] > [!] > [v] > [x] > [>] > [ ]` | Progress-wins ordering; accounting without a total order | Blocked above verified is the only ordering that makes "a blocker is never overwritten" true. Without a total order, merging stays order-dependent |
| `[!]` escalation + lane-scoped pause | Escalate without marking; keep whole-run pause | Only option that makes "sibling milestones continue" literally true. An escalation invisible in the register is one the next session cannot see |
| `parallel_slots` configured, not detected | Auto-derive from CPU/backend; report without capping | The binding constraint is memory bandwidth and rate limits; no portable probe sees either, and a wrong guess is worse than serial |
| Round-trip proof + `--dry-run` + per-feature commit | Git history alone; branch-and-diff | The only operation here that destroys information. A 1.86 MB diff review is theatre; a byte-for-byte round-trip is a proof |
| Proposals for M3, M5, M9 only | One per milestone; none | These three change what 139 directories depend on. Eleven specs is a second feature's worth of writing |
| P1-12 withdrawn `[-]` | Keep as measure-first; implement as written | Already measured in-repo: identical cache figures prefixed and suffixed. The mechanism is real; the saving is not |
| Go toolchain bumped as M1's first task | Out of scope; bump during M11 | **User decision, overriding the recommendation.** Sequenced before P0-2/P0-3 so the baseline is captured on the bumped toolchain and the comparison stays clean |

### Added during tech-plan, 2026-08-19 — placing the seeded harness

| Decision | Alternatives considered | Rationale |
|---|---|---|
| Both arms run at P3-3, against a binary frozen at M1 | Capture arm A now and arm B at M11; run the full pair twice | Alternating arm order needs both builds to exist at once, and at M1 the post-change build does not. Arms months apart confound cache warmth, vendor drift and machine state *with the arm* — the three things the 2026-08-19 controls exist to remove. A frozen binary is also immune to sibling merges, so the harness milestone need not serialise a wave behind it |
| Two new milestones, M12 (wave 2) and M13 (wave 3) | One 7-task milestone; fold everything into M11 | Free on wave count — still four waves, and theoretical parallelism rises 2.75× → 3.25×. One milestone would mix shipped Go, a bench harness and a five-repo ops step, which is the sizing rules' own "too big" test. Folding into M11 discovers harness infeasibility at wave 4, after everything else is spent, and loses the passive window entirely |
| P0-3 re-scoped to `[x]`; P0-3a withdrawn `[-]` | Promote P0-3 to `[v]` here; withdraw both | M1's `[!]` PAUSEs the whole feature, so this had to resolve. `[x]` lets `belmont reverify` judge the *new* criterion; a planning session writing `[v]` against a criterion it rewrote in the same session is the failure `[x]` vs `[v]` exists to prevent, and nothing mechanical would have caught it — commit `ff06d45` names the task |
| Harness as build-tagged test files | `scripts/*.sh` + a Go helper; a shipped `belmont bench` subcommand | Exactly the `eval_harness_test.go` pattern, so CI wiring, gating and conventions already exist. A subcommand puts a one-off instrument permanently into a surface 139 feature directories depend on. Shell cannot compute a Hodges–Lehmann interval without a dependency, and this repo has none |
| Generated seed, calibrated to the measured distribution | Copy the 1.86 MB register verbatim (committed, or gitignored) | Read-path cost is a function of bytes and structure, not words, so a generator is byte-equivalent for the purpose. Committing the copy would publish a pre-launch product's plans into a public fork; gitignoring it makes the seed unreproducible on any other machine, which undercuts the whole design |
| Arm B migrates before its timed run | Raise `progress_error_bytes` in the testbed; run all three variants | The post-M11 world is migrated; measuring arm B un-migrated measures a state that will never exist. Three variants would decompose the win informatively but cost 50% more runs in a strictly serial budget where the run *is* the unit of statistical power |
| Pin repointed to `belmont-pre`, not Homebrew v0.11.0 | Keep the Homebrew pin; repoint only after M12 | Isolation is identical — both frozen — but Homebrew v0.11.0 has no metrics code, so the feature that exists to measure Belmont would record nothing across its own twelve remaining milestones. Reverts in one command. Waiting for M12 would lose all of wave 2, more than half the remainder |
| Extend the model chain; record requested **and** served | Record the request only; replace `modelTiers` aliases with pinned IDs | Recording the request cannot see a vendor serving something else, which over a months-long window is the exact confound. Replacing the aliases pins every user in every repo to serve one experiment, and commits the project to editing that table on every vendor release |
| Sample median + exact Wilcoxon/Hodges–Lehmann interval | Percentile bootstrap; report significance against 50% | The median is what criterion 26 literally names as the rule. The exact interval is stable at n = 6–20 where a bootstrap is not, and it matches the signed-rank calculation that sized the suite. Significance against a 50% threshold would need a true effect near 65% — the reason criterion 26 was rewritten around the point estimate |
| No retries on a failed run | Retry on infrastructure failure, recorded as such | The completion rate exists to stop a cheaply-failed run flattering a median. A retry repairs that rate silently, which is the same distortion arriving through the fix rather than the fault |
| Redact the product estate in place; accept the residual exposure | Move to a private repo and delete the public fork; evict `.belmont/` from the fork | **User decision, 2026-08-19, against the recommendation and recorded as such.** Objects already pushed to a public fork stay reachable via the parent network, so a force-push reduces future exposure without undoing the publication already made. The cheapest option, and the one that keeps `.belmont/` as the state of record and the census evidence intact |

---

## Verification Checklist

### Per-milestone

- [ ] Unit tests extend the existing per-area `*_test.go` file, not a new parallel suite
- [ ] Any behaviour crossing a full loop iteration adds an eval fixture under `cmd/belmont/testdata/eval/`
- [ ] Every fixture `PROGRESS.md` passes `belmont validate` — `auto` lints at startup
- [ ] Prose milestones (M2, M7) report measured per-skill, per-host built size before and after
- [ ] `absent-means-today` proven: with the new optional file removed, behaviour is byte-identical to v0.11.0
- [ ] No change to the task-line or milestone-heading grammar
- [ ] No acceptance-criterion check removed or narrowed; no new path can promote `[x]` → `[v]`

### Commands

```bash
cd ~/repos/belmont
go build ./cmd/belmont               # NB: plain build does not embed skills/agents
go test ./cmd/belmont
go test -race ./cmd/belmont          # required for P0-1
go test -tags eval ./cmd/belmont     # Tier 1, offline, free, in CI
go test -tags bench ./cmd/belmont    # seed determinism + analysis math, offline, free, in CI
go vet -tags bench ./cmd/belmont     # tagged files are invisible to plain `go vet ./...`
go vet ./...
staticcheck ./...                    # runs in CI and is currently clean — keep it so
gofmt -l cmd/                        # must print nothing
bash scripts/generate-skills.sh --check
belmont validate --root ~/repos/belmont
```

**Tier 2 is mandatory on every prose milestone, not just the expensive ones.** `AGENTS.md:59` is explicit: *"Tier 1 **cannot** license a prose change — nothing in it reads a `SKILL.md`."* Tier 1 does not even compile under plain `go test`, and it never reads skill prose at all.

| Milestone | What it changes | Tier 2 |
|---|---|---|
| M2 | `_src/verify.md`, both review agent files — **pure prose** | **required** |
| M4 | `slice.go` plus four `_src/*.md` and a partial — spans both | required |
| M6 | `auto_decide.go`, `auto_loop.go`, `guards.go` — behavioural | required |
| M7 | the per-host skill build — **prose payload** | **required** |
| M8 | agent frontmatter + dispatch partial (P1-13) | required |
| M12 | `toolexec.go`, `local_llms.go`, `metrics.go` — Go only, no prose | not required |
| M13 | the bench harness — test files only, no prose | not required |

`ci.yml` already carries the exact wiring to copy: `go test -tags eval` at line 35 and `go vet -tags eval ./cmd/belmont` at line 44, with a comment explaining that the plain `go vet ./...` above cannot see build-tagged files. Add the `bench` pair beside them. **The live suite stays out of CI** — it is gated twice, by the tag and by `BELMONT_BENCH_LIVE=1`, exactly as `TestEvalLive` is.

M2 and M7 were previously Tier-1-only; corrected 2026-08-18 during `/belmont:review-plans`. They are the two milestones whose entire deliverable is prose, so they are exactly the ones Tier 1 cannot speak to.

```bash
BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont
```

`-timeout 0` is **required**, not optional — without it Go's 10-minute default orphans the live tool child (`AGENTS.md:59`). Tier 2 drives a real tool and costs tokens; record that spend against P0-2's instrumentation rather than letting it sit outside the measurement.

---

## Edge Cases

| Scenario | Handling |
|---|---|
| Feature has no `details/` directory | Read bodies from the register exactly as today. This is the default for 139 of 139 directories until opted in |
| `details/M<N>.md` exists for a milestone not in the register | `belmont validate` reports an orphaned detail record naming the file; nothing is deleted automatically |
| Task in register with no detail record | Reported by validation. Not an error at read time — a task may legitimately have no body |
| Register already over `progress_error_bytes` when M5 lands | `belmont auto` refuses to start, before the first read, naming the file and pointing at `belmont extract`. Existing files are not rewritten |
| Register already holds task lines over `task_line_max_chars` when M5 lands | **P1-8 checks only the lines a write introduces or modifies, never the whole file.** A pre-existing over-length line is left alone and reported by `belmont validate`, not by refusing the write. Whole-file checking would make every legacy register unwritable the moment M5 ships — including this feature's own, measured 2026-08-19 at **13 lines over 400 chars, longest 2,925** (the `P0-M1-FIX-8`…`FIX-11` narrative), with M11 and `belmont reverify` still to write to it at wave 4. The ceiling exists to stop *new* narrative entering the register; refusing writes over lines already there stops the migration that would fix them. Acceptance criterion on P1-8, not a note |
| Two worktrees rewrite the same task's detail | Git conflict on that one `## <task-id>` block. Correct — it is a genuine disagreement |
| Merge would drop a task | Refused, escalated, task named. Never written |
| `[v]` arrives by merge with no supporting commit | Demoted to `[x]` and reported, before the total order is applied |
| Both a blocker and a stale `[v]` for one task | `[!]` wins. Conservative by design |
| Tool reports no token usage (copilot, pi, opencode) | Record wall-clock, `null` tokens, and a stated reason. Never estimate |
| Registration spike fails any of its three checks | P1-11 marked `[-]`; M8 keeps P1-13 only. Not worked around |
| Extraction round-trip mismatch | Abort, name the milestone, write nothing |
| Milestone fails verification twice | Its tasks go `[!]` with both failures named; that lane stops; siblings continue and merge |
| Parallelism requested, no dependencies declared | Run says it is serial and why, then runs serially |
| `--max-parallel` above `parallel_slots` | Capped to the lower value, and the cap is reported |
| Skill built for a host with no dedicated install path | Falls back to the generic `.agents/skills/belmont/<skill>/SKILL.md`. Every supported host keeps a complete build |
| Testbed path resolves inside a product repo or this one | Harness **refuses to run**, naming the path. A measurement instrument able to write into what it measures is the 2026-08-18 failure with a different entry point |
| Arm's own prose did not resolve in the testbed | Harness aborts before the timed run. Otherwise both arms silently use user-level prose and M2/M4/M7 measure as zero |
| A seeded run fails or hits the session limit | Recorded with its `exit_reason`, counted in the completion-rate denominator, excluded from the ratio. **Never retried** |
| Extraction does not bring the seed under `progress_error_bytes` | Arm B cannot start. Prevented at seed-selection time by a P0-17 acceptance criterion, not discovered at run time — five of 82 real registers have this property |
| Tool reports a `model_served` differing from `model_requested` | Recorded as-is and flagged. Not corrected, not dropped — a divergence is the finding |
| `belmont metrics` spans two model IDs | Combined figure is `null` with a stated reason plus a per-model breakdown. Same fail-closed rule as `input_semantics` |
| A/A pilot's median ratio departs materially from 1.0 | The harness is not measuring what it believes. Resolve before running any real pair; no suite size can fix a biased instrument |

---

## Implementation Order

`(depends: …)` annotations in `PROGRESS.md` match this section exactly.

- **M1: Toolchain, atomic writes & baseline** — independent (wave 1). Root of the graph: every other milestone writes state more often than today and is unsafe without P0-1, and every acceptance criterion downstream is stated against P0-3's baseline. **Six tasks, deliberately at the soft ceiling** — P0-12 and P0-13 are prerequisites that gate every later milestone, and since every milestone already depends on M1 transitively, housing them here costs no extra wave.
- **M2: The verify read** — depends: M1 (wave 2). Needs instrumentation to evidence its saving. No file overlap with M3/M6/M7/M9/M10.
- **M3: Split the register safely** — depends: M1 (wave 2). Needs atomic writes before it doubles the number of state files being written.
- **M6: Bound the loop** — depends: M1 (wave 2). Needs P0-2's historical medians for the trajectory signal.
- **M7: Skill payload** — depends: M1 (wave 2). Independent of the state work entirely; touches only the generator and install path.
- **M9: Master file & journal** — depends: M1 (wave 2). Needs atomic writes; independent of the detail tier.
- **M10: Scheduler correctness** — depends: M1 (wave 2). Self-contained dependency-layer fixes. Land early — parallel work amplifies every defect here.
- **M12: Trustworthy measurement** — depends: M1 (wave 2). Needs P0-2's record to extend, and nothing else. Placed early on purpose: the passive window cannot open until the model and outcome fields exist, and every week it stays shut is pre-change observation that cannot be recovered afterwards.
- **M4: The read path** — depends: M3 (wave 3). `slice` must serve the detail tier, so the tier has to exist first.
- **M5: Growth ceilings** — depends: M3 (wave 3). A ceiling is only meaningful once extraction gives a way back under it.
- **M8: Cacheable dispatch** — depends: M7 (wave 3). Registration is Claude-Code-only, so the dispatch prose must already be host-conditional.
- **M13: The seeded paired suite** — depends: M12 (wave 3). The runner records the `model_requested` / `model_served` fields M12 adds, so the record schema has to exist first. Its A/A pilot runs here; the paired suite it sizes runs at M11.
- **M11: Roll out & re-measure** — depends: M2, M4, M5, M6, M8, M9, M10, M13 (wave 4). Migrates the pathological features, brings all five repos onto one build, and runs the paired suite M13 built, reporting against the M1 baseline including anything that missed.

**Wave structure**: W1 = M1 · W2 = M2, M3, M6, M7, M9, M10, M12 · W3 = M4, M5, M8, M13 · W4 = M11.

Thirteen milestones in four waves — **3.25× theoretical parallelism**, up from 2.75×, because both new milestones landed inside waves that already existed. **Expect ~1.4× in practice.** Run with `--max-parallel 2` or `3`: wave 2 now holds seven milestones, and seven against seven slots is precisely the overcommit the measurements warn about.

**The measurement is not on the critical path, and that is deliberate.** M12 and M13 sit inside existing waves, so neither adds a wave; the only ordering they impose is on M11, which already waited for seven other milestones.

---

## Distribution (P3-2)

`belmont` currently comes from `blake-simpson`'s Homebrew tap, and `scripts/update-homebrew.sh` defaults to `blake-simpson/homebrew-belmont`. Work landing in the fork reaches the five repos by **source-mode install**:

```bash
cd ~/repos/belmont && bash scripts/build.sh
for r in ~/repo-1 ~/repo-5 ~/repo-4 ~/repo-3 ~/repo-2; do
  (cd "$r" && belmont install --source ~/repos/belmont)
done
```

No tap, no release pipeline, no token. `belmont version` still reports a commit, so P3-2's version check stays meaningful. Reversible by re-installing the Homebrew build.

**Do not run this between M1 and M11** — it would install mid-flight skill prose into the repos the loop is measuring.

---

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Self-hosting: a milestone's change reaches the running loop early | Medium | Pinned orchestrating binary; no `belmont install` in the fork between M1 and M11; `throughput`'s own register stays un-extracted until after P3-3 |
| Wave 2's six milestones conflict on shared skill files | Medium | P0-13 triage first; M2/M4 both touch `_src/verify.md` but in different waves; per-milestone file ownership recorded in the File Structure section |
| Extraction loses information on a 1.86 MB file | Low | Byte-for-byte round-trip proof before any write; `--dry-run`; per-feature commit |
| P1-11 spike fails and M8 shrinks to one task | Medium | Explicitly planned for — withdraw as `[-]`, do not work around |
| Baseline contaminated by the toolchain bump | Low | P0-12 sequenced before P0-2 and P0-3, so before and after are built identically |
| Upstream diverges while the fork carries thirteen milestones | Medium | Formats stay backward-compatible; three proposals give upstream reviewable specs rather than one large diff; merge `upstream/main` between waves |
| Two verify agents share a model and co-fail | Known | Not addressed by this feature, and deliberately so — pairwise error correlation underprices joint failure by 2.5–8.3×, which argues for keeping the two lenses genuinely *different*, not for collapsing them |
| Success criteria not met at P3-3 | Medium | A criterion that was not met is reported as not met. The baseline exists so this cannot be argued either way |
| `belmont-pre` carries an M1 defect and breaks twelve milestones | Low-Medium | M1 is the most-verified milestone here — six tasks, twelve fix rounds, three of them in code. Revert is one command: Homebrew back on PATH, and the passive arm is lost rather than the run |
| The M11 paired-suite block runs longer than a sitting | **High** | Measured: 43–45 min per milestone on *real* code work (implement 27–38, verify 5–18). The seeded design does trivial work so implement should collapse, but nobody knows by how much until P0-19's pilot measures it. That is the pilot's second job, and the cut order (work → verify rounds → phases → pair count) is decided in advance so it is not improvised at 2 a.m. |
| The A/A pilot shows sd ≥ 1.0, demanding 20 pairs | Medium | Sample size wins; per-run cost gives first. If it cannot give enough, the achieved n and its consequence for the interval are published rather than quietly absorbed |
| Product names leak into a new artefact | Medium | The seed is generated, not copied; per-pair records name repos by alias. The redaction wordlist is gitignored, so the local guard script skips where it is absent — **CI cannot enforce this and does not pretend to** |
| Redaction leaves the already-published objects reachable | **Known, accepted** | Recorded in prerequisite 6 as an accepted residual risk on the owner's explicit decision. Not mitigated — only repository deletion would, and that option was declined |
| M12 and M6 both touch `auto_loop.go` in wave 2 | Medium | Different functions: M6 rewrites the decision and retry paths, M12 appends one terminal record beside the existing `recordPhaseMetrics` call at `:614`. Recorded here so the merge is expected rather than discovered |

---

## Notes for Implementing Agent

- **Read the code before trusting a line number.** Every reference in this document was checked against `17897aa2` on 2026-08-18, but M1's toolchain bump and P0-13's branch merges will move them. The repo's own recorded lesson: a specification that reads as coherent is not the same as one that is correct about the system it describes.
- **Follow existing patterns**: `guards.go` for deterministic checks, `validate.go:28` for the shape of a dangling-reference error, `fsutil.go` for filesystem helpers, `eval_harness_test.go` for fixture structure.
- **Never add an external dependency.** `go.mod` has zero requires and no `go.sum`. That is a property worth keeping.
- **Resolve `plugin/` conflicts by regeneration, never by hand** — `generate-plugin.sh:35` does `rm -rf`.
- **Skills to load**: `context-audit` for M1's global-surface measurement and M7's skill-payload work. No frontend, design, database or Figma skills apply anywhere in this feature.
- **The design agent is skipped on every milestone.** No task here has design input.
- **`[x]` → `[v]` is the unit.** A milestone that implements cleanly and fails verification has not shipped, and its tokens count against the same denominator.

---

## References

- `docs/proposals/0004-context-budget-with-evidence.md` rev 5 (`origin/docs/pr-proposals`) — lazy Setup, eval harness, and the measurement that withdrew the steering optimisation.
- `docs/proposals/README.md`, `docs/proposals/NEXT-SESSION.md` — the repo's proposal convention and its three recorded lessons on specification correctness.
- SPOQ, arXiv:2606.03115 — 14.31× unbounded against 1.43× on a two-slot local backend.
- arXiv:2605.26297 — LLM inference is 71–98% of agent runtime; decode 91–98.6% of that, memory-bandwidth bound.
- PASTE, arXiv:2603.18897 — generation time growing 17× from 1 to 192 concurrent sessions.
- LAMaS, arXiv:2601.10560 — Pearson r = 0.901 between critical-path tokens and wall-clock.
- arXiv:2606.01365 — 58% of a failing run's tokens spent after failure was detectable (165 GAIA traces, median 61%).
- Sherlock, arXiv:2511.00330 — verification adding up to 28.9× latency and 53.2× cost; verifier-preserved output rates of 0.6–0.8.
- arXiv:2608.13571 — forwarding a failed reasoning chain costs up to 34.8 percentage points of accuracy.
- arXiv:2604.02460 — single-agent beating multi-agent at matched thinking-token budgets on multi-hop reasoning.
- arXiv:2606.27288 — pairwise error correlation underpricing joint failure 2.5–8.3×; LLM-as-router capturing zero oracle gain.
- arXiv:2506.14852 — plan caching, 50.31% cost and 27.28% latency reduction. Noted as a future lever; not scoped here.
