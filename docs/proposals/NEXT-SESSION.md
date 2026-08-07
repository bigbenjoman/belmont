# Next session — handoff

Written at the end of a long planning session. Assumes you know nothing about it.

**Repo:** `/Users/benlavender/belmont` · fork remote `bigbenjoman` · upstream `origin` = `blake-simpson/belmont`
**Specs branch:** `docs/pr-proposals` (pushed)

## Read first

1. `AGENTS.md` (`CLAUDE.md` is a symlink to it) — especially "Both invocation paths or it's not done" and **Verify**.
2. `knowledge/KNOWLEDGE.md` — the routing index. **Open the entries that match what you touch**; don't work from the routing table alone. Twice in the prior session the answer was already in an entry that went unread.
3. The specs in `docs/proposals/`:

| File | State |
|---|---|
| `0003-design-quality-without-figma.md` | **rev 7 — written 2026-08-07.** Two full adversarial passes done (rev 5: 53 fixed; rev 6: 53 fixed, 1 blocker). Evidence: [`rev6-CONFIRMED-DEFECTS.md`](rev6-CONFIRMED-DEFECTS.md), [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md) |
| `0003-DESIGN-RESTRUCTURE.md` | background brief; four of its facts were refuted — see its header |
| `0004-context-budget-with-evidence.md` | rev 5 — GO (v5 = ordering amendment, see "Resolved decision" below) |
| `0005-maintainability.md` | **GO — rewritten for the reviewer 2026-08-07.** No revision tables; history is in `git log`. Sections renumbered — old §4.1 is now §2.1, old §4.3 is now §3 |
| `0006-plugin-generator-correctness.md` | shipped as PR #21 |
| `redteam-round3.md` | the review that produced rev 4 |

## Hard gate — CLEARED 2026-08-07

PR #21 **merged** (`ae26e29`), shipped as v0.10.15 (`8aa84db`). Verified on `origin/main`: all six `plugin/agents/*.md` are now source + 3 lines — 271/366/218/131/279/430 — exactly the assertion 0006 specified. The "do not run `generate-plugin.sh`" rule is retired.

Branch off a `main` that contains `ae26e29`.

## Build order

`0005 → 0004 → 0003`. **One per session** — 0005 in particular is large.

---

## Task A — housekeeping (2 minutes, do first)

**DONE — 2026-08-06.** `README.md` said revision 3 and ordered the specs `0003 → 0004 → 0005`; both were wrong. Fixed, plus two further stale claims it did not name (PRs #12/#13 described as open — both closed; "reviewed twice" — three rounds). The ordering fix cascaded into `0003` §11, `0004` (bumped to rev 5) and this file — see "Resolved decision" below. Revisions are now 0003 rev 4, **0004 rev 5**, 0005 rev 4.

## Task B — implement 0005

Follow `docs/proposals/0005-maintainability.md`. Two independent halves — read its §10 before starting, because they can land separately and the reviewer may want only one.

**The CI half** (final commit): `.github/workflows/ci.yml`, delete the eight dead functions, fix the docs. ~100 lines of YAML against ~200 deleted lines of Go. This is the half that answers a defect which has already shipped — four 0-byte plugin agents in every release since 2026-06-04 — and the half 0004's eval harness wires into. Low risk, ten-minute review.

**The split half** (commits 1–N): `cmd/belmont/main.go`, 12,613 lines, into domain files **in the same package**. Bigger, and a judgement call the reviewer can decline.

- One commit per **§3** table row. The table is a **proposal** — re-cut it if the boundaries are wrong, but every declaration needs a stated home before commit 1, and re-cutting mid-split means re-running the purity check on every commit already made.
- Build `scripts/declsum/main.go` (**§2.1**) and **re-verify it against both controls** — a real extraction and a deliberately broken one — before the DoD depends on it. The tool does not exist in the repo; the 398-declaration control run quoted in the spec came from a scratch directory and has not been reproduced.
- Decl-set diff empty at every extraction commit.

**`fix/worktree-git-excludes` is now PR #22**, opened separately against `main` — not folded in. Branch from a `main` that contains it. If it has not landed, 0005 §4.1 says what changes about the declsum expectation.

**staticcheck's nine findings are confirmed** (`staticcheck 2026.1 (v0.7.0)` against `main` at `5f1c2c2`) — the eight unused functions plus one dead assignment in `runReverifyCmd`. Line numbers and symbols are listed in 0005 §4.1. Install with `go install honnef.co/go/tools/cmd/staticcheck@latest`.

If the split is deferred, the CI half still stands alone and the build order below is unaffected.

---

## Task C — 0003 needs restructuring first (do NOT implement rev 4)

> **Read [`0003-DESIGN-RESTRUCTURE.md`](0003-DESIGN-RESTRUCTURE.md) first.** It is the
> full standalone brief for this change — current pipeline vs proposed, what each agent
> and skill does afterwards, where artifacts live, the three user requirements with their
> constraints, and what not to do. The summary below is orientation only.


**DONE — rev 5 written 2026-08-06.** The spec now describes the restructure. Remaining: one adversarial pass, then implement (after PR #21). What follows is the original orientation, with its errors marked.

The user proposed moving UX/UI design from `implement` Phase 2 into **`tech-plan`, after `product-plan`**. This is better than rev 4 and dissolves problems rev 4 patches. The three facts below were checked against the code when rev 5 was written — two needed correcting:

| Fact | Evidence | Verdict |
|---|---|---|
| `tech-plan` is already interactive | `_src/tech-plan.md:53,55` includes `user-questions.md` + `dynamic-questioning.md` | ✅ holds |
| `TECH_PLAN.md` is **master-owned** | ~~`main.go:9980`~~ → the `os.RemoveAll(dstFeature)` is at **`main.go:10010`**, the restoring `copyDir` at `:10011` | ⚠️ conclusion right, mechanism wrong. The axis is **master-authored vs worktree-authored**. `{base}/TECH_PLAN.md` is *inside* the wiped dir and survives only because master is the copy source; a worktree-local write to it dies exactly like MILESTONE |
| Planning is forced to Opus | `planningTier = "high"` (`main.go:102`), consumed by `tierForAction` for `actionReplan` only (`:249-252`) | ❌ **auto-mode only.** The interactive path this restructure relies on inherits the session model. The tier guidance does **not** become unnecessary |

**Why it is better**

- **Durability solved, not patched.** Rev 3 put the contract in a separate file, rev 4 moved it back to MILESTONE — both wipe on resume. `TECH_PLAN.md` does not. The problem was never the filename; it was writing to worktree-created state.
- **The approval gate comes free.** `tech-plan` already stops and asks. Design becomes part of the plan reviewed before `belmont auto` starts, with no new checkpoint machinery and no Go change.
- **One design pass per feature, not per milestone.** Today design-agent re-derives tokens every milestone — a token cost and a coherence risk (M1 and M4 could pick different scales with nothing to catch it).
- **Tier doctrine lines up by construction.** Planning is already Opus, so the node authoring the contract runs on the right model. §4.7's `models.yaml` guidance becomes unnecessary.

**The split to draw**

- **`tech-plan`** — derive and record the *feature-level* design contract (token contract + source, a11y floor, UX strategy, microcopy rules) into `TECH_PLAN.md`, reviewed and approved interactively.
- **`implement` Phase 2** — design-agent stays but **consumes** the approved contract rather than inventing one. Mode A extracts per-task specs from Figma; Mode B derives per-task component specs against the contract. Both stay per-task because Figma URLs are per-task in the PRD.
- **`verify` Phase 2** — checks against `TECH_PLAN.md`, which it already reads. No archive-clobbering exposure, so `_src/next.md` probably leaves scope entirely.

**Three user requirements to fold in**

1. **Invoke `ux-designer` and `ui-designer`** during the design phase. Constraint: these live at `~/.claude/skills/` — they are the user's personal skills, absent in other people's projects. Must be conditional: use when present, fall back to the inlined checklist otherwise.
2. **Consider `frontend-design`** (plugin skill) — aesthetic direction, typography, avoiding templated defaults. Targets Mode B's real failure mode: derived-from-nothing design converges on generic. `dataviz` only if the milestone has charts; `vercel:shadcn` only if the project uses it. Figma skills are irrelevant here by definition. Note skill availability differs per tool — Codex/Gemini/etc. have no equivalent mechanism, so it must degrade cleanly.
3. **Show durable artifacts for approval** before implementation. Largely solved by the restructure: `TECH_PLAN.md` is durable and already read by implementation-agent and verification-agent. What still needs designing is a **human-viewable** form — markdown inside a plan file is not reviewable design. That is the open question.

**Scope of the restructure:** §4.1, §4.3, §5 and the smoke test all change; `_src/tech-plan.md` joins scope; `_src/next.md` probably leaves. Call it **rev 5**, and give it one adversarial pass before implementing.

---

## Non-negotiables (from `AGENTS.md`)

- Address **both invocation paths** — auto (CLI shells out to `claude -p`) and interactive (`/belmont:<skill>`) — in the plan *and* the tests.
- `go build ./cmd/belmont` and `go test ./cmd/belmont` green at **every** commit.
- Author smoke test against a real project on a disposable branch, with exact expected output. Note 0005's was corrected twice: Step 7a needs `--feature` (`runAutoCmd` rejects a bare call at `main.go:5311`), and Step 3's grep needs an ANSI strip (`main.go:12247` emits `ESC[33m[SCOPE-GUARD]ESC[0m`).
- 5-platform build matrix including `GOOS=windows` — a deleted `//go:build` line passes every host-only check while breaking Windows.
- Knowledge entries: use `grep -rln 'main\.go' knowledge/` (15 entries), not a hand list. One `Revisions` line each.
- Also stale after 0005: `AGENTS.md` Architecture + Verify, `CONTRIBUTING.md:81` and `:138`, `docs/directory-structure.md`.

## Verify claims against the code — this matters here

These specs went through three adversarial review rounds. Every round found defects, and several were **introduced by the previous round's fixes**. Two specific traps:

- A review agent claimed `cmd/belmont/tools.go` already existed. **It never has** — zero commits, any branch. The claim was accepted without checking and shipped into two revisions. Do not trust a finding — from a review, a spec, or a previous session — without running the command yourself.
- **When a claim spans two documents, grep both and diff them.** Keep this rule permanently; it is not specific to one round. A claim gets written once, cited elsewhere, and the citation outlives the correction. Both instances so far were found this way and neither was visible from one file: the `0003`↔`0004` build order disagreed across *three* documents with no single "current" one, and `main.go:9980` — the wrong line for an `os.RemoveAll` that lives at `:10010` — had propagated into **four**. A document that reads as authoritative is not evidence; the code is.
- **A predicted failure is a claim too, and cheaper to test than to reason about.** Three revisions asserted that 0003 and 0004 "will conflict" in `_src/verify.md` because their edits sit inside default diff context. Reproducing it took one scratch repo and showed the rebase is clean — the context rule governs `git am`/`git apply`, not the three-way merge. Nobody had run it.
- Two purity-proof mechanisms were specified before being run, and both were impossible. The current one (0005 **§2.1**) is described as verified against two controls — but `scripts/declsum/` is not in the repo, so that run has never been reproduced here. Build it and re-run both controls before the DoD depends on it.

## Resolved decision — 0003 ↔ 0004 baseline order (2026-08-06)

**This section previously mislocated the defect.** It said 0004 "currently measures its token baseline before 0003's Mode B lands, then re-baselines." That plan did exist — but in **`0003:7`**, not in 0004. 0004 rev 4 said the opposite in four places (`:7`, `:36`, `:238`, `:243`), and so did **0003 §11**, which contradicted 0003's own header. The round-3 order was applied to 0003's Sequencing line and nowhere else, leaving one spec self-contradictory and the other stale. Three files disagreed; none of them was simply "current".

Lesson for the next session: when a claim spans two documents, grep **both** and diff them against each other. This was found by grepping 0004 for a claim attributed to it, finding the opposite, and only then grepping 0003.

**Resolution:** the author confirmed `0005 → 0004 → 0003`. The specs were amended to match, and 0004 bumped to rev 5:

- 0004 baselines **pre-Mode-B**; its reduction is an **upper bound** and must be reported as one.
- A **re-baseline after 0003** is owed — a follow-up obligation in 0004's DoD, inherited into 0003's. Not a merge gate for 0004; the code it measures will not exist yet.
- 0003's rev 5 restructure may dissolve the dependency (design derived once in `tech-plan`, so Mode B stops touching 0004's deferred fan-out path). **Unverified** — rev 5 is unwritten. Do not assume it.

Nothing here affects 0005.
