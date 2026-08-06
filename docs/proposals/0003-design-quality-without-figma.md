# PR 1 — Design quality without Figma

**Revision 3.** v2 was re-reviewed against the codebase; v3 resolves what that found. Changes in §12.

**Type:** prose only (agents, one skill source, one new reference). No Go changes.
**Size:** ~450 lines net across 6 tracked files + regenerated `plugin/` + 1 knowledge entry. (v1 understated this at ~300 / 3 files.)
**Sequencing:** depends on **0006** — the plugin generator must work before edited agents can ship to the plugin surface. Land **first of the three** thereafter. PR 2's token baseline must already include Mode B (see §11).

---

## 1. Problem

`agents/belmont/design-agent.md` is a Figma extractor, not a designer. Its entire no-Figma path is `## Handling No Design` (lines 270–276) — four bullets that all defer.

Consequences on a project with no Figma:

1. **Phase 2 of `/belmont:implement` is a paid no-op.** A full sub-agent reads its agent file, MILESTONE and the PRD to return "no design references were provided."
2. **No design quality gate exists.** `verification-agent.md` Phase 2 has a real acceptance-criteria fallback (`:73`, `:112-114`, `:125`) — dev server, mandatory screenshot, `N/A` attestation — but that is a *correctness* check, not a design-quality check. Nothing evaluates spacing, states, contrast or hierarchy.
3. **The fallback is explicitly permissive.** `verification-agent.md:131`: with no design references at all, visual verification *can pass on acceptance criteria alone*. Mode B is defined by the absence of references, so this clause would apply to every Mode B run and make the new gate opt-out by construction.

Net: for a no-Figma project, visual quality is whatever the implementation agent improvises, and nothing checks it.

## 2. Root cause — state this first in the PR description

The agent conflates two situations:

- **Figma load *failed*** — a real design exists and we could not read it. Guessing is dangerous; the NO FALLBACK rule at `design-agent.md:255` is **correct and untouched by this PR**.
- **Figma *absent*** — no design exists to be faithful to. Nothing to guess *against*, so the same prohibition leaves the pipeline with no design authority at all.

This PR separates the two and changes nothing about the failed-load path.

## 3. Rationale from the graph-engineering method

**Checking is its own job (§3).** Isenberg's framing: research fails because "the same model that writes the answer also grades the answer." Belmont already separates implementation from verification, but with no Figma the checker has nothing to check *against* — the separation is structurally present and substantively empty. Mode B supplies the contract that makes verification objective.

**Give it a goal and tools, not a script (§4).** Boris Cherny on how model use changed at Anthropic: you no longer say "do step one, then step two" — "you give it a goal, you give it the starting point, you give it access to tools, and then it will figure it out." Mode B hands the agent design *authority* plus its inputs and a return format. The fixed edges are only what a model under time pressure would skip: enumerate every state, declare the token source, meet the a11y floor.

**State as durable artifacts (§5).** The contract is written to a file the checker reads later. v1 put it somewhere that gets overwritten — see §4.1.

**Design content source.** Token scales, state inventory, microcopy rules and a11y floor come from the `ui-designer` and `ux-designer` skills — 8pt grid, type scale from a ratio, 60-30-10 colour, internal ≤ external spacing, all-states enumeration, three-part error messages, 44px targets, 4.5:1 contrast. Credit them in the PR description so the numbers read as sourced rather than arbitrary.

## 4. Design

### 4.1 Contract storage — the v1 blocker

v1 put the contract in MILESTONE's `## Design Specifications`. That is destroyed on the first fix round: `skills/belmont/_src/next.md:162` archives to the **same** `MILESTONE-<ID>.done.md` path and overwrites the section with `[Not populated — lightweight mode skips the design agent]` (`next.md:114-118`). Both `actionFixAll` (`main.go:7197`) and `actionImplementNext` (`:7174`) emit `/belmont:next`, so the contract dies before the re-verify that most needs it — and also disappears for code-review-agent, `belmont reverify` and debug-manual.

**Split the output by lifetime:**

| Artifact | Path | Lifetime | Consumer |
|---|---|---|---|
| **Design contract** — shared tokens, a11y floor, UX strategy, microcopy rules | `{base}/DESIGN-CONTRACT-<MilestoneID>.md` | durable; never written by `next.md` | verification-agent Phase 2, code-review-agent, reverify |
| **Per-task design sections** — `### Design: [Task ID]` with component specs | MILESTONE `## Design Specifications`, as today | per-run | implementation-agent (`:79` looks up its per-task section) |

This preserves the existing downstream contract (`design-agent.md:232` mandates one section per active task; `implementation-agent.md:79` reads it), so v1's claim that "downstream consumers need no structural change" becomes true rather than assumed.

MILESTONE's `## Design Specifications` opens with a pointer line — `**Design Contract**: {base}/DESIGN-CONTRACT-<M>.md` — so any agent reading MILESTONE can find it.

### 4.2 Mode selection

Determined once, before any other work, keyed on **URL presence in the PRD** and never on load outcome:

- Any active task has a Figma URL → Mode A for those tasks.
- No active task has a Figma URL → Mode B for the milestone.
- Mixed → per-task; record the mode per task.

Mode A is unchanged, including every failure rule.

### 4.3 Mode B contract format

`{base}/DESIGN-CONTRACT-<M>.md`:

```markdown
# Design Contract — <MilestoneID>
**Mode**: B (derived — no Figma references)
**Generated**: <ISO date>

## Token Contract
**Source**: [tailwind.config.ts | globals.css | components.json | none — established below]

**Spacing** — 8pt grid: 4, 8, 12, 16, 24, 32, 48, 64, 96. Internal ≤ external on every component.
**Typography** — ratio [1.2], sizes [list], line-height 1.4–1.6 body / 1.1–1.3 heading. Max 4 sizes.
**Colour** — 60/30/10. Max 3 hues + neutrals. Never pure #000/#FFF. Body ≥ 4.5:1.
  Each semantic (success/error/warning/info) declares bg, border, text, icon.
**Radius** — committed value [8px]. Nested children strictly smaller than parent.
**Elevation** — [levels]. Interactive elements rise one level on hover.

## Accessibility Floor
Targets ≥ 44×44px · contrast ≥ 4.5:1 text / 3:1 large · visible focus on every interactive
element · every input has a visible label (never placeholder-only) · `prefers-reduced-motion`
respected · no meaning by colour alone.

## UX Strategy
User · emotional state · hero element · primary action · biggest UX risk. Five lines.

## Microcopy Rules
Buttons: verb matching outcome. Errors: what happened / why / what next.
Empty states: what appears here, why it's empty, the action to fill it.
Destructive confirmations: name what is destroyed.
```

MILESTONE per-task section, unchanged shape:

```markdown
### Design: [Task ID] — [Task Name]
**Mode**: B
**Contract**: {base}/DESIGN-CONTRACT-<M>.md

#### [ComponentName] — [NEW | MODIFIED]
| State | Spec |
```

**Output cap (cost control, see §11).** Full nine-state table (default · hover · active · focus · disabled · loading · error · empty · success) **only for components the milestone creates**. For components it modifies, record the **state delta** only. Components merely consumed get no entry. Omissions carry a reason; silence is not allowed.

**Derivation order — reuse before invention:**

1. Read `## Codebase Analysis` (written in parallel by codebase-agent).
2. Probe for an existing system: `tailwind.config.*`, CSS custom properties, `components.json`, theme files.
3. If found, **record it** — do not invent a competing scale.
4. Only where nothing exists, establish the defaults above and set `**Source**: none`.

### 4.4 Verification Phase 2, Mode B branch

Three-way, replacing v1's two-way:

- **References found** → existing comparison flow, unchanged.
- **No references, `DESIGN-CONTRACT-<M>.md` exists** → contract checks (below).
- **No references, no contract** → existing acceptance-criteria fallback, unchanged. This is the failed-load path and it must keep working.

**Close the escape clause.** `verification-agent.md:131` currently permits a pass on acceptance criteria alone whenever no references exist. Narrow it to "no design references **and no Mode B contract**", and add a fourth enforcement rule mirroring `:129-130`: *a Mode B contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*.

**Named tooling per check.** v1 specified measurements no tool in the pipeline provides. Each row names its mechanism:

| Check | Mechanism |
|---|---|
| Spacing on declared scale; internal ≤ external | `browser_evaluate` → `getComputedStyle` |
| Type sizes from declared scale | `browser_evaluate` |
| Contrast ≥ 4.5:1 / 3:1 | `browser_evaluate` computing fg/bg luminance |
| Touch targets ≥ 44×44 | `browser_evaluate` → `getBoundingClientRect` |
| Focus visible on all interactive | `browser_press_key Tab` + `browser_snapshot` per stop |
| Labels present, not placeholder-only | `browser_evaluate` DOM inspection |
| Radius consistent, nested smaller | `browser_evaluate` |
| Enumerated states present | exercise each state, screenshot |

Reconcile the bare `mcp__playwright__*` names in `verification-agent.md` against the plugin-prefixed names actually available; if a Bash-driven Playwright script is the fallback, say so explicitly.

**Degradation rule**, mirroring `:93`: a check whose mechanism is unavailable is recorded **`UNVERIFIABLE`**, never `PASS`. **`Actual` values must come from the running UI — sourcing them from `tailwind.config` or `globals.css` is forbidden**, since that is the same source the contract was derived from and would make the check circular.

Findings use the **existing** Critical / Warning / Polish ladder. Suggested mapping: contrast failure, missing focus indicator, missing label → Critical; missing state, off-scale spacing → Warning; radius or elevation inconsistency → Polish. Off-scale spacing stays Warning so pre-existing drift cannot block a milestone.

**Attestation gains a field:** `Contract checks performed: [YES against <path> | NO — <reason> | N/A]`.

Note Phase 5 (Lighthouse/axe) already covers four of these rows but is gated on "if public page" and degrades to SKIPPED — it is not a substitute.

### 4.5 Project lint / typecheck gate

Mode B increases hand-written UI volume, and nothing in Phases 0–6 runs the target project's own checks (`code-review-agent.md:228` assumes a linter runs elsewhere; nothing does). Add to Phase 4: *run the project's lint/typecheck scripts if defined; failures introduced by this milestone are Critical, pre-existing failures are noted and not blocking.*

### 4.6 Downstream consumers

- **`implementation-agent.md`** — `:190-195` self-check keys entirely on the Figma Sources table, so under Mode B it is a no-op, and `:272` assumes Mode A snippets. Add a Mode B branch: self-check against the contract's token scale and state inventory before reporting a task done. Without this, every violation survives to verify and costs a triage → fix-all → re-verify cycle.
- **`skills/belmont/_src/verify.md`** — Step 1b (`:62-72`) recognises only Figma tables and images, so a Mode B milestone falls through to `:119` and the orchestrator injects "No design references found" into the sub-agent prompt. Extend Step 1b to detect the contract file and pass a Mode B block at `:114-121`.

### 4.7 Model tier interaction

Agents inherit the session model; `models.yaml` is the single visible override (PR #18, `ad5f84c`). The governing doctrine is **errors cost more than tokens**, measured: Sonnet 4.x produces 57% first-pass-correct code against Opus 4.x at 77% — a 43% vs 23% rework rate, ~1.9× as often — and Sonnet 5.x is no better. In Belmont a rework is a triage → fix-all → re-verify cycle: three extra cold-start phases each reloading full context, so it costs multiples of the phase it replaces.

Mode B sharpens this. Today design-agent reads as mechanical extraction, which invites a `low` (haiku) pin. Under Mode B it **authors the contract that mechanically gates the milestone** — the highest-blast-radius output in the pipeline. A cheap contract produces expensive rework in every downstream phase.

Add to `references/models-yaml-format.md` and the tech-plan tier heuristics: **never downgrade design-agent on a milestone with UI and no Figma; leave it inheriting the session model, or pin `high`.** Add a `Revisions` line to `model-tier-economics.md`.

## 5. Scope

### In scope

| File | Change |
|---|---|
| `agents/belmont/design-agent.md` | Mode A/B selection; replace `## Handling No Design` with Mode B; write contract file + per-task sections |
| `agents/belmont/references/design-no-figma.md` | **New.** Mode B detail. Note this is the **first** `agents/belmont/references/` dir — confirm `scripts/build.sh` copies it |
| `agents/belmont/verification-agent.md` | Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; lint/typecheck in Phase 4 |
| `agents/belmont/implementation-agent.md` | Mode B self-check branch |
| `skills/belmont/_src/verify.md` | Step 1b detects the contract |
| `skills/belmont/_src/references/models-yaml-format.md` | Mode B tier guidance |
| `agents/belmont/design-agent.md` `## Important Rules` | `:260` ("DO NOT create, edit, or write to any file other than the MILESTONE file") and `:137` forbid the contract write. Permit the `DESIGN-CONTRACT-<M>.md` path in both. Leave `:259` alone — it is not violated |
| `skills/belmont/_src/implement.md` | Phase 2 purpose line (`:96`) is stale under Mode B |
| `plugin/` | Regenerate — `plugin/agents/{design,verification,implementation}-agent.md` and `plugin/skills/verify/` are git-tracked. **Requires 0006**; before it, four plugin agents generate as 0 bytes |
| `knowledge/cross-cutting/design-authority.md` | **New** entry |

**Reference file placement (v1 blocker).** v1 put it under `skills/belmont/_src/references/`, where `generate-skills.sh:134-148` copies a reference only when a generated **SKILL.md body** names it. The consumer here is an agent file, so it would ship nowhere — verified empirically. It goes under `agents/belmont/references/`, which the `hasReferences` branch of `syncEmbeddedDir` (`main.go:4861-4872`) already mirrors, and is cited from `design-agent.md` by the explicit project-root path `.agents/belmont/references/design-no-figma.md` (sub-agent CWD is the project root).

### Out of scope

Mode A behaviour · the NO FALLBACK rule · skipping the design dispatch (Mode B gives it real work) · `next.md` (the contract now survives it by living elsewhere) · any Go change.

## 6. Both invocation paths

| Path | Exercise |
|---|---|
| **Auto** | `belmont auto` → implement dispatches design-agent → verify dispatches verification-agent |
| **Interactive** | `/belmont:implement` then `/belmont:verify` in a live REPL |
| **Plugin** | `plugin/agents/*` and `plugin/skills/verify/` are a third tracked distribution surface; `release.sh:39-41` regenerates and `git add`s it, `release.yml:66-70` ships it. Uncorrected, plugin users get the old design-agent. |

⚠️ Open PR #11 edits `plugin/agents/verification-agent.md` — guaranteed conflict. #10 and #11 both edit `agents/belmont/verification-agent.md` at the intro block (`@@ -6,6 +6,14 @@`), which does **not** overlap this PR's Phase 2 edits (`:49-134`). Both are stale against HEAD. State this in the PR description with regions cited rather than claiming "conflicts with nothing."

## 7. Docs and knowledge

- `docs/agent-pipeline.md` — Mode A / Mode B
- `docs/skills-reference.md` — the new agent reference dir
- `AGENTS.md` — add a design-authority line under the invariants list (`CLAUDE.md` is a symlink; edit `AGENTS.md`)
- **New** `knowledge/cross-cutting/design-authority.md`, `Domains: agents, skills`, full skeleton. `Don't re-do` must record: inferring values on a *failed* load (rejected — NO FALLBACK stands); skipping the dispatch (rejected — Mode B is cheaper than Mode A, no MCP round-trips); storing the contract in MILESTONE (**rejected — `next.md` overwrites the archive**); a separate `design-mode-b-agent` (rejected — one agent, two modes, stable downstream contract).
- Routing row in `knowledge/KNOWLEDGE.md`; `Revisions` line on `model-tier-economics.md`.

## 8. Author smoke test

Real project, disposable branch, no Figma URLs in the PRD.

**Preconditions (apply to every step).** `requireCleanWorkingTree` (`main.go:9818`) aborts `auto` on any dirty or untracked path, and `install` writes `.agents/`, `.claude/commands/belmont/` and `.belmont/` without committing. Commit after every install. `belmont auto` also runs `belmont validate` at startup, so any hand-edited PROGRESS.md must pass the milestone-structure lint.

```bash
cd ~/path/to/real-project && git checkout -b smoke/pr1
belmont install --source ~/belmont --no-prompt
git add -A && git commit -m "belmont install (PR1 smoke)"
BASE=$(git rev-parse HEAD)          # every later branch forks from here
grep -c "Figma" .belmont/features/<slug>/PRD.md   # expect 0
```

**Step 1 — auto, Mode B.** `belmont auto --feature <slug> --from M1 --to M1`
Expect `.belmont/features/<slug>/DESIGN-CONTRACT-M1.md` exists with a populated Token Contract and `**Source**` naming a real file or `none`; the archived `MILESTONE-M1.done.md` (implement archives before auto returns — `implement.md:167-168`, `main.go:6593`) contains per-task `### Design:` sections with `**Mode**: B` and a contract pointer.
Fail: only "no design references were provided" → Mode B did not trigger.

**Step 2 — the gate fires.** Same run's verify report `## Visual Verification` shows per-row pass/fail/UNVERIFIABLE and the attestation reads `Contract checks performed: YES against …`.
Fail: `NO` or the acceptance-criteria fallback → `:131` was not narrowed.

**Step 3 — contract survives a fix round (the v1 blocker).**
Force one FWLUP round so `/belmont:next` runs and re-archives MILESTONE.
Expect `DESIGN-CONTRACT-M1.md` still present and unmodified; the second verify still reports contract checks.
Fail: contract missing → it is being written to a clobbered path.

**Step 4 — interactive.** `git checkout -b smoke/pr1-interactive $BASE`, then `claude` → `/belmont:implement --feature <slug>` → `/belmont:verify --feature <slug>`.
Branching from `$BASE` matters: after Step 1, M1 is `[v]` and committed, and `implement.md:42-46` selects the first *pending* milestone, so a branch off Step 1's HEAD would silently run M2.

**Step 5 — Mode A regression.** Feature with Figma URLs, MCP connected. Expect Mode A per-task sections, Figma Sources table with LOADED/FAILED, **no** contract file.
Verify Mode A is unchanged by `git diff` on `design-agent.md` confined to the mode-selection block and the replaced `## Handling No Design` section, plus this structural assertion. (v1's "byte-identical MILESTONE" DoD was unsatisfiable — the section is LLM prose and the template embeds a timestamp and a `git rev-parse HEAD` baseline, `implement-milestone-template.md:14-15`.)

**Step 6 — failed-load regression (the one that must not break).** Deliberately invalid node id. Expect task BLOCKED, status FAILED, no values invented, **no contract file created**.

**Step 7 — other tools.**
7a: `belmont auto --tool codex` from `$BASE` — same contract structure.
7b: interactive `codex` → `$belmont` popup → confirm `belmont:implement` / `belmont:verify` listed → select `$implement`. (`--tool` is a Go-CLI flag only; Codex's `/` menu is built-ins only — `docs/supported-tools.md:75`.)
Steps 1–4 already constitute the Claude-in-both-modes sanity check.

**Diagnostics.** Steps 1–4 need no Figma access. If 5 or 6 fail, test the Figma MCP directly before blaming this PR.

## 9. Definition of Done

**Behaviour**
- [ ] Mode keyed on PRD URL presence, never on load outcome; recorded per task
- [ ] Contract written to `{base}/DESIGN-CONTRACT-<M>.md`; per-task sections stay in MILESTONE
- [ ] Contract survives a `/belmont:next` fix round (smoke Step 3)
- [ ] Contract reuses an existing token source and names it in `**Source**`
- [ ] Nine-state table for created components; state-delta for modified; omissions carry a reason
- [ ] Phase 2 is three-way; acceptance-criteria fallback retained unchanged for the no-contract case
- [ ] `verification-agent.md:131` narrowed to "no references AND no contract"
- [ ] Fourth enforcement rule present: contract exists + checks not performed ⇒ FAIL/INCOMPLETE
- [ ] Every check row names a mechanism; unavailable ⇒ `UNVERIFIABLE`, never `PASS`
- [ ] `Actual` sourced from the running UI; sourcing from config forbidden in prose
- [ ] Attestation carries `Contract checks performed:`
- [ ] Phase 4 runs the project's lint/typecheck; new failures Critical, pre-existing noted
- [ ] `implementation-agent` self-checks against the contract under Mode B
- [ ] `verify.md` Step 1b detects the contract and passes a Mode B block
- [ ] Findings use the existing Critical/Warning/Polish ladder — no new severity

**Non-regression**
- [ ] `git diff` on `design-agent.md` confined to mode selection, the replaced `## Handling No Design` section, **and the `## Important Rules` write-rule amendment at `:137`/`:260`**
- [ ] Mode A structural assertion passes (smoke Step 5)
- [ ] Failed load still BLOCKS, invents nothing, creates no contract (smoke Step 6)
- [ ] `SKIPPED` still forbidden as a Figma node status

**Measurement (per C10 — structure only, not quality)**
- [ ] One eval fixture asserts contract *structure*: all sections present, `**Source**` non-empty, created components enumerate nine states
- [ ] Spec and PR description state plainly that design **quality** is not measured, only contract **presence and conformance**
- [ ] MILESTONE + contract byte size recorded pre/post for PR 2's baseline (§11)

**Mechanics**
- [ ] `./scripts/generate-skills.sh --check` and `./scripts/generate-plugin.sh --check` pass (`plugin.json` STALE is the only legitimate diff; restore from main before committing)
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green
- [ ] `test -f <project>/.agents/belmont/references/design-no-figma.md` after install
- [ ] `test -f plugin/agents/references/design-no-figma.md` after regeneration (needs 0006's agents `references/` branch; `scripts/build.sh` already handles the install surface)
- [ ] Auto, interactive and plugin surfaces all exercised
- [ ] `design-authority.md` created with `Don't re-do`; routing row added; `model-tier-economics.md` amended
- [ ] `AGENTS.md` invariants list updated (edit `AGENTS.md`, not the `CLAUDE.md` symlink)
- [ ] PR description opens with the failed-vs-absent distinction and cites the #10/#11 region analysis

## 10. Risks

| Risk | Mitigation |
|---|---|
| Mode B activating on a failed load | Selection keys on URL presence only. Smoke Step 6. |
| Contract lost to `next.md` | Separate durable file. Smoke Step 3 exists solely for this. |
| Checks unimplementable → silent pass | `UNVERIFIABLE` never counts as pass; enforcement rule makes a missing check a FAIL. |
| Circular checking (Actual read from config) | Explicitly forbidden; smoke Step 2 spot-checks one row against the live DOM. |
| Contract invents a competing scale | Derivation order mandates reuse; `**Source**` must name a file or say `none`. |
| haiku authoring the gating contract | §4.7 tier guidance biases `high` for UI-without-Figma. |
| Verbosity inflates sub-agent cost | Output cap in §4.3; byte size recorded for PR 2. |

## 11. Interaction with PR 2

These two PRs push token cost in opposite directions. Mode B *adds* to `## Design Specifications` and a contract file, both read on the sub-agent fan-out path PR 2 explicitly defers; PR 2 cuts only the orchestrator reads. On a UI-heavy no-Figma milestone the net could be higher.

Mitigations: the §4.3 output cap; recording MILESTONE + contract byte size in this PR's DoD; and **landing 0003 first** so 0004's measured baseline already includes Mode B. 0004 names this in its Problem section.

**Shared files.** 0004 edits `_src/verify.md` at `:110`/`:147`, four lines from this PR's Mode B insertion at `:114` — inside default diff context, so they will conflict. Both regenerate tracked `plugin/skills/verify/SKILL.md`. 0005 edits `_src/references/models-yaml-format.md`, which this PR also touches. Resolve plugin conflicts by regeneration, never by hand — `generate-plugin.sh:35` does `rm -rf`.

## 12. Changes from v1

| v1 | v2 |
|---|---|
| Contract in MILESTONE `## Design Specifications` | Own durable file — `next.md` overwrites the archive |
| Reference under `skills/belmont/_src/references/` | Under `agents/belmont/references/` — the generator ships references only for skill bodies |
| Nine checks, no mechanisms | Mechanism named per row + `UNVERIFIABLE` rule + circularity ban |
| `verification-agent.md:131` untouched | Narrowed; fourth enforcement rule added |
| "Phase 2 skips with no references" | False — there is an acceptance-criteria fallback; reworded |
| "downstream needs no structural change" | Per-task sections retained explicitly; implementation-agent + verify.md added to scope |
| `plugin/` unmentioned | In scope with `--check` gate |
| "conflicts with nothing" | #10/#11 regions cited |
| Smoke asserts live `MILESTONE.md`, no clean-tree commit, branches from wrong SHA, `--tool` for interactive codex | All corrected |
| DoD "Mode A byte-identical" | Unsatisfiable — replaced with confined `git diff` + structural assertion |
| ~300 lines / 3 files | ~450 lines / 6 files + plugin |

### v2 → v3

| v2 | v3 |
|---|---|
| Contract write vs `design-agent.md:260` | `:137`/`:260` now in scope; write-rule permits the contract path |
| Non-regression DoD forbade the fix | Reworded to include the rules amendment |
| `_src/implement.md` Phase 2 purpose line stale | In scope |
| Plugin generator assumed working | Depends on **0006**; four plugin agents currently generate as 0 bytes |
| DoD checked `build.sh` (already worked) | Asserts `plugin/agents/references/design-no-figma.md` |
| "conflicts with nothing" | Shared-file rows for `verify.md` (0004) and `models-yaml-format.md` (0005) |
