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
| `0003-design-quality-without-figma.md` | rev 4 — **needs a rev 5 restructure, see below** |
| `0004-context-budget-with-evidence.md` | rev 4 — GO |
| `0005-maintainability.md` | rev 4 — GO |
| `0006-plugin-generator-correctness.md` | shipped as PR #21 |
| `redteam-round3.md` | the review that produced rev 4 |

## Hard gate

**PR #21 must merge before any implementation.** It fixes `scripts/generate-plugin.sh`, which currently publishes four of six plugin agents as **0-byte files**. Branch off `main` before it merges and any regeneration drops those empties into your PR.

```bash
gh pr view 21 --repo blake-simpson/belmont --json state
```

## Build order

`0005 → 0004 → 0003`. **One per session** — 0005 in particular is large.

---

## Task A — housekeeping (2 minutes, do first)

`docs/proposals/README.md` is stale: it says the specs are at revision 3 and orders them `0003 → 0004 → 0005`. All three are rev 4 and the order is `0005 → 0004 → 0003`. Fix and commit on `docs/pr-proposals`.

## Task B — implement 0005

Follow `docs/proposals/0005-maintainability.md`. Split `cmd/belmont/main.go` (12,613 lines) into domain files **in the same package**, then add `go vet` + staticcheck + CI.

- One commit per §4.3 table row. The table is a **proposal** — re-cut it if the boundaries are wrong, but every declaration needs a stated home before commit 1.
- Build `scripts/declsum/main.go` (§4.1) and **re-verify it against both controls** — a real extraction and a deliberately broken one — before the DoD depends on it.
- Decl-set diff empty at every extraction commit.

---

## Task C — 0003 needs restructuring first (do NOT implement rev 4)

The user proposed moving UX/UI design from `implement` Phase 2 into **`tech-plan`, after `product-plan`**. This is better than rev 4 and dissolves problems rev 4 patches. Verified:

| Fact | Evidence |
|---|---|
| `tech-plan` is already interactive | `_src/tech-plan.md` includes `user-questions.md` + `dynamic-questioning.md` |
| `TECH_PLAN.md` is **master-owned** | Copied master → worktree, so restored on `[r]`-resume — unlike MILESTONE, which is worktree-created and wiped by `copyBelmontStateToWorktree` (`main.go:9980`, `os.RemoveAll`, preserves only `STEERING.md`) |
| Planning is forced to Opus | `planningTier = "high"` (`main.go:102`) |

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
- Two purity-proof mechanisms were specified before being run, and both were impossible. The current one (0005 §4.1) *is* verified, but re-verify before the DoD depends on it.

## Open decision for the user — ask, don't assume

0004 currently measures its token baseline **before** 0003's Mode B lands, then re-baselines. That contradicts what 0003 §11 originally argued. Confirm which is wanted before implementing 0004 (not needed for 0005).
