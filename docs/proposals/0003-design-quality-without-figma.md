# PR 1 — Design quality without Figma

**Revision 5.** v1–v4 all placed design derivation inside `/belmont:implement` Phase 2 and spent three revisions patching the consequences of that placement. v5 moves feature-level design into **`/belmont:tech-plan`**, where the approval gate, the durability and the model tier already exist. Four v4 defects that survived round 3 are also fixed. Changes in §12.

**Type:** prose only (2 agents, 2 skill sources, 2 skill references, 1 new skill reference). No Go changes.
**Size:** ~520 lines net across 9 tracked source files + regenerated `plugin/` + 1 new knowledge entry + 2 corrected knowledge/AGENTS statements. (v4 said "~450 / 6 files" and undercounted — its own §5 table names seven, and §7 adds three more.)
**Sequencing:** depends on **0006** (PR #21, open) — the plugin generator must work before edited agents and skills ship intact. Lands **after 0005 and 0004** per the build order set 2026-08-06. This PR owes 0004's post-Mode-B re-baseline (§11).

---

## 1. Problem

`agents/belmont/design-agent.md` is a Figma extractor. It runs as Phase 2 of `/belmont:implement`, once per milestone, in parallel with the codebase agent. With a Figma URL it extracts exact specs. Without one, its entire behaviour is `## Handling No Design` (`:270-276`) — four bullets that all defer.

Consequences on a project with no Figma:

1. **Phase 2 is a paid no-op.** A full sub-agent reads its agent file, MILESTONE and the PRD to return "no design references were provided."
2. **No design quality gate exists.** `verification-agent.md` Phase 2 is *comparison-only*: its check target is a loaded Figma screenshot or image (`:53-60`). With nothing to compare against it falls back to acceptance criteria — a correctness check, not a design-quality check.
3. **The fallback is explicitly permissive.** `verification-agent.md:131` lets visual verification pass on acceptance criteria alone whenever no references exist. "No references" is the defining condition of the no-Figma case, so this clause would make any new gate opt-out by construction.

Net: visual quality is whatever the implementation agent improvises, and nothing checks it.

## 2. Root cause — state this first in the PR description

Two failures, conflated:

- **Figma load *failed*** — a real design exists and we could not read it. Guessing is dangerous; the NO FALLBACK rule at `design-agent.md:61-68` and `:255` is **correct and untouched by this PR**.
- **Figma *absent*** — no design exists to be faithful to. Nothing to guess *against*, so the same prohibition leaves the pipeline with no design authority at all.

v1–v4 correctly separated these, then put the fix in the wrong place. **The second root cause is placement**: design authority was derived per-milestone, by a sub-agent, inside the implementation loop, into worktree-local state that nothing durable owns and no human ever approves.

## 3. Rationale

**Checking is its own job.** Belmont already separates implementation from verification, but with no Figma the checker has nothing to check against — the separation is structurally present and substantively empty. A design contract supplies the objective standard.

**Derive once, at the point where a human is already reviewing.** `tech-plan` is interactive by construction (`_src/tech-plan.md` includes `user-questions.md` and `dynamic-questioning.md`), already asks about "Design system details (colors, spacing, typography — **especially if no Figma**)" (`:197`), already lists "Styling & design system … design tokens; theme layer" as a domain (`:70`), and already loads design skills (`:160`). The capability is present and under-specified. This PR hardens it rather than building a new one.

**State as durable artifacts.** The contract must live where it survives worktree creation, `[r]`-resume and merge — see §4.1, which is now an evidence question rather than an assertion.

**Design content source.** The token scales, state inventory, microcopy rules and a11y floor in `references/design-authority-baseline.md` are Belmont-original prose stating published standards: WCAG 2.1 SC 1.4.3 (4.5:1 body, 3:1 large), SC 2.5.5 (44×44 targets), SC 2.4.7 (visible focus), the 8pt spacing grid, ratio-derived type scales, and the 60-30-10 colour convention. The curation was **inspired by the `ux-designer` and `ui-designer` skills by [Yummy Labs](https://www.yummy-labs.com/)**; credit them by name in the reference header and the PR description. **Do not vendor their files.** They are distributed as a download with no stated licence, no repository and no redistribution grant, so the default is all-rights-reserved and copying them into this repo is not ours to do. If written permission is later obtained, vendoring with attribution can replace the baseline file in a follow-up.

## 4. Design

### 4.1 Where the contract lives — and the exact durability claim

The feature-level design contract is written into **`{base}/TECH_PLAN.md`** (i.e. `.belmont/features/<slug>/TECH_PLAN.md`) by `/belmont:tech-plan`, **in the master tree, before `belmont auto` starts**.

**The durability claim, stated precisely.** `copyBelmontStateToWorktree` (`main.go:9980`) does `os.RemoveAll(dstFeature)` at **`main.go:10010`** and immediately re-copies from the master feature dir at **`:10011`**, preserving only `STEERING.md` (`:10004-10018`). Four earlier documents cited `:9980` for the `RemoveAll`; that line is the function signature, and the call is 30 lines later.

The consequence is not "TECH_PLAN.md is safe". It is:

> **Content authored in the master tree survives, because master is the copy source. Content authored inside a worktree does not.**

`{base}/TECH_PLAN.md` sits *inside* the wiped directory. It survives resume only because master holds the version that is copied back over it. A worktree-local edit to that same path is destroyed exactly like MILESTONE.

Three further mechanics rev 5 depends on, all verified:

| Mechanic | Evidence | Consequence |
|---|---|---|
| Tracked `.belmont/` files are `--assume-unchanged` in a worktree | `main.go:10104-10122` | An agent editing `{base}/TECH_PLAN.md` inside a worktree **cannot commit it** — `git add -A` does not stage it |
| Resume-wipe is unconditional in the single-feature milestone path, conditional in multi-feature | `main.go:9042` (unconditional) vs `:6178-6195` (`if !resumed`) | Worktree-local edits survive multi-feature resume and die in single-feature resume. A skill cannot tell which regime it is in |
| `syncFeatureStateAfterMerge` has a second `os.RemoveAll(dstFeature)` | `main.go:10169` | Any mechanical durability change must patch **both** sites |

**Therefore, the hard rule of this PR:**

> **`tech-plan` writes the contract. `implement`, `verify`, `next` and `debug` only read it.** No auto-mode phase may write or amend `{base}/TECH_PLAN.md`. A phase that "enriches the contract it consumes" reintroduces exactly the bug this PR removes.

**`design-agent.md`'s `## FORBIDDEN ACTIONS` (`:5-15`) is untouched.** Its only writable output stays MILESTONE `## Design Specifications`. v3 relaxed that rule and v4 had to revert it; a consuming design-agent needs no relaxation at all.

**Replan is the live threat.** `actionReplan` emits `/belmont:tech-plan --feature <slug>` headlessly mid-run (`main.go:7188-7189`), and `tech-plan.md:310` writes `{base}/TECH_PLAN.md` from a template. "Authored once, before auto starts" is **not** guaranteed. Mitigation in §4.2 (rule R4).

### 4.2 The tech-plan design step

Insert **`### Phase 3.5 — Design Contract`** into `skills/belmont/_src/tech-plan.md` between `:217` and `:219` — after Phase 3's interview (it consumes the `:197` design-system answers) and before Phase 4 writes the plan.

Rules, each of which exists because something in the codebase would otherwise break it:

- **R1 — UI gate.** Run only if the feature has UI. A backend-only feature skips it and the template records `**Design contract**: N/A — no UI in this feature`, so downstream agents can distinguish "no UI" from "not done". Silence is not allowed.
- **R2 — Figma stays inline.** If the PRD carries Figma URLs, load them **in-session**, never via a sub-agent — `tech-plan.md:158`: "sub-agents cannot get MCP tool permissions approved." The contract records Figma-derived tokens where they exist and derived tokens where they do not.
- **R3 — Structured questions or stop.** The approval step inherits `user-questions.md:4` ("NEVER ask questions as plain inline text when a structured question tool exists") and the strict Codex fallback at `:11`. A Markdown pick-list is not an approval gate.
- **R4 — Headless behaviour is defined, not implied.** Under `belmont auto` (`actionReplan`) there is no user and `AskUserQuestion` is unavailable. The step must: **preserve an existing `## Design Contract` verbatim** if one is present, deriving only what is missing; mark it `**Approval**: unreviewed (headless replan <ISO date>)`; and never silently regenerate an approved contract. `runScopeGuard` returns early for `actionReplan` (`main.go:12065-12068`), so nothing mechanical will catch a violation here — this is prose-enforced and must be stated as such.
- **R5 — Codex packet.** The contract and the preview must be enumerable as `BELMONT_PLAN_PACKET` operations (`tech-plan.md:37-45`). Every target path is under `.belmont/`, which `codex-plan-apply` requires. Without this, Codex users silently get a TECH_PLAN with no contract.
- **R6 — Update CRITICAL RULE 6.** `tech-plan.md:19` names Phase 4.5 as the last mandatory phase and is *already* stale with respect to Phase 4.6. Adding Phase 3.5 without fixing it leaves three contradictory statements of when the skill terminates.
- **R7 — Extend ALLOWED ACTIONS.** `tech-plan.md:96-105` enumerates the write surface exhaustively. Add `{base}/design-preview.html`. **Do not touch the shared `forbidden-actions.md` partial** — it is included by other skills, and widening it would let `product-plan` write HTML too. The preview is a planning artifact under `.belmont/`, never project source; CRITICAL RULE 2 ("no source code files") stands unchanged for `.tsx/.ts/.css`.

### 4.3 Design authority ladder

Placed in the new `skills/belmont/_src/references/design-authority-baseline.md` and cited from the tech-plan body as a literal `references/design-authority-baseline.md` path — `generate-skills.sh:140` greps the generated SKILL.md for exactly that pattern and ships only what it finds.

> Take the **first** tier whose inputs are available. Check by **name**, in your available skills. Do not probe the filesystem.
>
> - **Tier 1 — enrichment.** If `ux-designer` and/or `ui-designer` are available, load them and let them drive the contract.
> - **Tier 1b — enrichment, conditional.** If `frontend-design` is available, load it for aesthetic direction. Load `dataviz` **only** if this feature renders charts. Load `vercel:shadcn` **only** if the project already depends on shadcn — evidence being a `components.json` at repo root or a shadcn entry in the package manifest, not a guess.
> - **Tier 2 — normative floor, all tools.** Apply the baseline rules in this file.
>
> Tier 2 is normative. Tiers 1/1b only enrich. **The contract schema is identical either way and no downstream consumer may branch on which tier ran.**
>
> Record the outcome on an `**Authorities**:` line — e.g. `baseline; ux-designer; ui-designer`, or `baseline (no design skills available in this session)`. Never omit it and never silently skip, mirroring `verification-agent.md:93`.

**Why the fallback is normative and not the fallback.** `ux-designer` and `ui-designer` are user-scope personal skills at `~/.claude/skills/`. They exist for one person. Wording requirement (1) as "invoke the skills, falling back to a checklist" would make the shipped behaviour for essentially every user the untested branch.

**Correct the framing this PR inherits.** The claim that non-Claude CLIs "have no equivalent mechanism" is false: opencode exposes a native model-callable `skill` tool, Gemini has `activate_skill`, and Codex/Cursor/Windsurf/Copilot/Pi auto-activate on `description:` match. What is absent is the **content** — those two skills live under `~/.claude/skills/`, which no non-Claude CLI scans. Rev 5 must say the skills are absent, not the mechanism, or it designs the wrong fallback.

**Use exact registered names.** `tech-plan.md:160` and `product-plan.md:165` currently name `vercel-react-best-practices` and `security`; neither resolves — the real name is `vercel:react-best-practices`, and `security` is a slash command, not a skill. The wrong names are duplicated onto two git-tracked generated plugin surfaces. Fix them in this PR; do not copy that line's style.

### 4.4 Contract format

Written into `{base}/TECH_PLAN.md`. The feature template's `## Design Tokens (from Figma)` (`tech-plan-feature-format.md:44`) is **generalised**, not supplemented — it is Figma-scoped today, and a no-Figma run leaves it empty with the contract homeless. Its verification checklist hard-codes "Matches Figma design pixel-perfect" and must gain a contract branch.

```markdown
## Design Contract
**Mode**: [A — Figma-derived | B — derived, no Figma | N/A — no UI in this feature]
**Source**: [tailwind.config.ts | globals.css | components.json | Figma <file> | none — established here]
**Authorities**: baseline[; ux-designer; ui-designer; frontend-design]
**Approval**: [approved <ISO date> | unreviewed (headless replan <ISO date>)]

### Token Contract
Spacing — 8pt grid: 4, 8, 12, 16, 24, 32, 48, 64, 96. Internal ≤ external on every component.
Typography — ratio [1.25], sizes [list], line-height 1.4–1.6 body / 1.1–1.3 heading. Max 4 sizes.
Colour — 60/30/10. Max 3 hues + neutrals. Never pure #000/#FFF. Body ≥ 4.5:1.
  Each semantic (success/error/warning/info) declares bg, border, text, icon.
Radius — committed value [8px]. Nested children strictly smaller than parent.
Elevation — [levels]. Interactive elements rise one level on hover.

### Accessibility Floor
Targets ≥ 44×44px (WCAG 2.5.5) · contrast ≥ 4.5:1 text / 3:1 large (SC 1.4.3) · visible focus on
every interactive element (SC 2.4.7) · every input has a visible label, never placeholder-only ·
`prefers-reduced-motion` respected · no meaning by colour alone.

### UX Strategy
User · emotional state on arrival · hero element · primary action · biggest UX risk. Five lines.

### Microcopy Rules
Buttons: verb matching outcome. Errors: what happened / why / what next.
Empty states: what appears here, why it's empty, the action to fill it.
Destructive confirmations: name what is destroyed.
```

**Derivation order — reuse before invention:** probe `tailwind.config.*`, CSS custom properties, `components.json`, theme files; if found, **record** it and never invent a competing scale; only where nothing exists, establish the defaults and set `**Source**: none`.

**Per-task specs stay in MILESTONE, unchanged in shape.** design-agent still produces one `### Design: [Task ID]` section per active task (`design-agent.md:232`), because `implementation-agent.md` Step 1.1 looks its per-task spec up there.

### 4.5 The reviewable artifact

`tech-plan` also writes **`{base}/design-preview.html`** — a self-contained page rendering the contract: colour swatches with computed contrast ratios, the type ramp, the spacing scale, radius and elevation samples, and a state row per enumerated component state. No external assets, no network references.

This is durable and reviewable on the same terms as the contract: authored in the master tree, staged by `commit-belmont-changes.md`'s `git add .belmont/`, and therefore present in the PR diff. It is **not** written during an auto wave — anything written under `.belmont/` inside a worktree is invisible to git (`main.go:10104-10122`), so an auto-authored preview would never reach a diff.

The user opens it from the path printed at approval time. Publishing it to a hosted artifact is Claude-Code-specific and must not be a dependency; the file is the deliverable.

### 4.6 Verification

**`agents/belmont/verification-agent.md` Phase 2 becomes three-way:**

1. Design references exist → existing comparison flow, unchanged.
2. No references, but `{base}/TECH_PLAN.md` carries a `## Design Contract` → **contract checks** (below).
3. No references and no contract → existing acceptance-criteria fallback, unchanged. This is the failed-load path and must keep working.

Note the distinction rev 5 must not blur: verification-agent **already reads** TECH_PLAN (`:20`, `:60`), but only as a place to *search for references*. Making it the *check target* is new behaviour this PR specifies, not existing behaviour it inherits.

**Close the escape clause.** Narrow `:131` to "no design references **and no design contract**", and add a fourth enforcement rule mirroring `:129-130`: *a contract exists but contract checks were not performed ⇒ MUST be FAIL/INCOMPLETE*.

**Named tooling per check.** Every row names its mechanism; a check whose mechanism is unavailable is recorded **`UNVERIFIABLE`**, never `PASS`. `Actual` values must come from the running UI — sourcing them from `tailwind.config` or `globals.css` is forbidden, since that is the source the contract was derived from and the check would be circular.

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

**Fix the stale MCP tool names in the same PR.** `verification-agent.md:93-94` and `implementation-agent.md:192-193` name `mcp__playwright__browser_*`, which does not exist; the installed plugin exposes `mcp__plugin_playwright_playwright__browser_*`. The same files already use the correct `mcp__plugin_figma_figma__` prefix, so these are simply stale — and every row above depends on them.

**Focused re-verification must be handled explicitly.** After a fix-all round the default verify scope is *focused*, and both `verify.md:50` and the injected prompt (`main.go:7184`) instruct the agent to skip visual/design comparison. Contract checks are **exempt from that skip** when the milestone's follow-ups touched UI; otherwise the attestation records `Contract checks performed: NO — focused re-verification, no UI follow-ups`. Without this, the gate silently never fires after the first failure.

**Attestation gains a field:** `Contract checks performed: [YES against <path> | NO — <reason> | N/A]`.

**Severity** uses the existing Critical / Warning / Polish ladder: contrast failure, missing focus indicator, missing label → Critical; missing state, off-scale spacing → Warning; radius or elevation inconsistency → Polish. Off-scale spacing stays Warning so pre-existing drift cannot block a milestone.

### 4.7 Downstream consumers

- **`agents/belmont/design-agent.md`** — mode selection keyed on Figma-URL presence in the PRD, never on load outcome. Both modes now **consume** the contract: Mode A extracts per-task specs from Figma; Mode B derives per-task component specs *against the approved contract*. Reads `{base}/TECH_PLAN.md` (already in its input list at `:30`) **and the master `.belmont/TECH_PLAN.md`**, which it does not read today even though cross-cutting styling decisions are routed there by tech-plan's Scenario A/B rules. Writes nothing but MILESTONE.
- **`agents/belmont/implementation-agent.md`** — add a contract branch to the Visual Validation self-check. Correct the justification v4 carried: `:190-197` is *not* a no-op without Figma — `:196` handles other reference images and `:197` mandates a "No design reference available" note. The branch is still needed; the "keys entirely on the Figma Sources table" claim is not true. `:272` is mode-agnostic and needs no change.
- **`skills/belmont/_src/verify.md`** — Step 1b already reads `{base}/TECH_PLAN.md` for visual specifications (`:70`). The edit is therefore small: tell it what shape to look for (`## Design Contract`), not where to look. v4's "Step 1b recognises only Figma tables and images" was wrong.
- **`skills/belmont/_src/next.md`** — **stays in scope.** The feature contract is now safe in TECH_PLAN, but the *per-task* specs remain in MILESTONE, and `next.md:162` still overwrites the archive while `:117-118` stamps `[Not populated — lightweight mode skips the design agent]` over `## Design Specifications`. A `/belmont:next` fix round still destroys per-task design state. Carry forward populated `## Design Specifications` and `## Codebase Analysis` sections rather than clobbering them. Coordinate with 0004, which edits this file and lands first.

### 4.8 Model tier

**§4.7 of v4 does not become unnecessary, contrary to the restructure brief.** `planningTier = "high"` (`main.go:102`) is consumed only by `tierForAction` for `actionReplan` (`main.go:249-252`), which fires **only inside the `belmont auto` shell-out**. A user typing `/belmont:tech-plan` in a Sonnet or Haiku REPL gets that session's model — Belmont pins no `model:` frontmatter anywhere. Since rev 5's entire premise is that the contract is authored *interactively*, the forcing does not cover the path that matters.

Three consequences:

1. Keep tier guidance, retargeted: **never downgrade the agent that authors or checks a design contract.** For interactive tech-plan, note in the skill that contract authoring wants the strongest available session model.
2. `models-yaml-format.md:30` describes the `design` tier as "Figma extraction, token mapping, visual spec" and offers `design=low` heuristics for backend/infra features. Both go stale under rev 5 and must be rewritten — design-agent is now a contract consumer *and* per-task spec author.
3. `tech-plan` Phase 4.6 asks the planner to reason about "what will the design-agent actually do on this feature" (`:267`). That prose changes.

## 5. Scope

### In scope

| File | Change |
|---|---|
| `skills/belmont/_src/tech-plan.md` | Phase 3.5; ALLOWED ACTIONS + `design-preview.html`; CRITICAL RULE 6; fix skill names at `:160`; Phase 4.6 prose |
| `skills/belmont/_src/references/tech-plan-feature-format.md` | Generalise `## Design Tokens (from Figma)` → `## Design Contract`; add contract branch to the Figma-only verification checklist |
| `skills/belmont/_src/references/design-authority-baseline.md` | **New** — the ladder + baseline rules, Belmont-original, crediting Yummy Labs |
| `skills/belmont/_src/product-plan.md` | Fix the same wrong skill names at `:165` |
| `agents/belmont/design-agent.md` | Mode A/B both consume the contract; replace `## Handling No Design`; read master TECH_PLAN. `FORBIDDEN ACTIONS` untouched |
| `agents/belmont/verification-agent.md` | Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; focused-reverify exemption; fix Playwright MCP names |
| `agents/belmont/implementation-agent.md` | Contract branch in Visual Validation; fix Playwright MCP names |
| `skills/belmont/_src/verify.md` | Step 1b recognises the contract shape |
| `skills/belmont/_src/next.md` | Archive merges rather than clobbers `## Design Specifications` / `## Codebase Analysis` |
| `skills/belmont/_src/references/models-yaml-format.md` | Rewrite the `design` tier description and heuristics |
| `plugin/` | Regenerate — now includes `plugin/skills/tech-plan/**` and `plugin/skills/product-plan/**` as well as the agents and `plugin/skills/verify/`. **Requires 0006** |
| `AGENTS.md` | Correct `:26`; add a design-authority line to the invariants |
| `knowledge/cross-cutting/dual-invocation-paths.md` | Correct `:9` |
| `knowledge/cross-cutting/design-authority.md` | **New** entry |

### Out of scope

Mode A extraction behaviour · the NO FALLBACK rule · any Go change · the two `main` bugs this work uncovered (`writeWorktreeGitExcludes` writing to the wrong gitdir; `AGENTS.md:207`'s deleted master-tree shortcut) — file them separately, do not fold them in.

## 6. Both invocation paths

| Path | Exercise |
|---|---|
| **Auto** | `belmont auto` → `actionReplan` may invoke `/belmont:tech-plan` headlessly (R4) → implement dispatches design-agent → verify dispatches verification-agent |
| **Interactive** | `/belmont:tech-plan` in a live REPL, contract approved via structured questions → `/belmont:implement` → `/belmont:verify` |
| **Plugin** | `plugin/agents/*`, `plugin/skills/{tech-plan,product-plan,verify}/**` are a third tracked distribution surface; `release.sh:41` regenerates and `:96` stages it |

**Correct the record on how auto mode works.** `AGENTS.md:26` and `knowledge/cross-cutting/dual-invocation-paths.md:9` state that auto mode bypasses the tool's skill discovery and that Belmont injects the skill body, agent files and project state. That is false. `buildLoopPrompt` emits a bare slash reference (`main.go:7168`, `:7189`); Belmont injects **only** steering text (`:6887`) and milestone-scope prose; the agent reads everything else from disk. For Pi and opencode, `adaptPromptForTool` rewrites the reference into a read-the-file instruction — still not injection. Consequently a skill running under `claude -p` runs inside Claude Code's full skill runtime with `Skill` in the allowlist (`main.go:7927`), which is why the Tier-1 ladder works in auto mode on Claude. This correction is a scope item, not an aside: rev 5's requirement-(1) story depends on it.

**No open-PR conflict.** v4 warned about PRs #10 and #11 editing `verification-agent.md`; both were **closed on 2026-08-06**. The only open PR is #21 (plugin generator), which is disjoint. Re-measure with `gh pr list --state open` at branch time rather than trusting this paragraph.

## 7. Docs and knowledge

- `docs/agent-pipeline.md` — design authority now originates in tech-plan
- `docs/skills-reference.md` — the new tech-plan reference
- `AGENTS.md` — invariants line + the `:26` correction
- **New** `knowledge/cross-cutting/design-authority.md`, `Domains: agents, skills`, full skeleton. `Don't re-do` must record: inferring values on a *failed* load (rejected — NO FALLBACK stands); skipping the design dispatch (rejected — it now consumes a contract); **storing the contract in MILESTONE (rejected — `next.md` clobbers the archive, and worktree-local state does not survive resume)**; a separate `DESIGN-CONTRACT-<M>.md` (rejected — same resume profile, no gain); a separate `design-mode-b-agent` (rejected — one agent, two modes); relaxing `design-agent.md:5-15` (rejected twice — if a change seems to need it, the placement is wrong again)
- Routing row in `knowledge/KNOWLEDGE.md`; `Revisions` lines on `model-tier-economics.md` and `dual-invocation-paths.md`

## 8. Author smoke test

Real project, disposable branch, no Figma URLs in the PRD.

**Preconditions.** `requireCleanWorkingTree` (`main.go:9818`) aborts `auto` on any dirty or untracked path, and `install` writes files without committing — commit after every install. `belmont auto` runs `belmont validate` at startup; on a TTY it prompts y/N rather than hard-failing (`main.go:5391-5397`).

```bash
cd ~/path/to/real-project && git checkout -b smoke/pr1
belmont install --source ~/belmont --no-prompt
git add -A && git commit -m "belmont install (PR1 smoke)"
BASE=$(git rev-parse HEAD)
grep -c "Figma" .belmont/features/<slug>/PRD.md   # expect 0
```

**Step 1 — interactive tech-plan authors the contract.** `claude` → `/belmont:tech-plan --feature <slug>`.
Expect: a structured question presenting the derived contract for approval; on approval, `{base}/TECH_PLAN.md` contains `## Design Contract` with a populated Token Contract, `**Source**` naming a real file or `none`, an `**Authorities**` line, and `**Approval**: approved <date>`; `{base}/design-preview.html` exists and opens; both are committed by the skill's commit step.
Fail: no approval prompt → R3 not wired. Missing `**Authorities**` → ladder not recording.

**Step 2 — the contract survives `[r]`-resume.** `belmont auto --feature <slug> --from M1 --to M1`, interrupt mid-run, resume with `[r]`.
Expect `{base}/TECH_PLAN.md` in the worktree still carries the identical `## Design Contract` after resume.
This is the step that proves §4.1. Fail here and the placement is wrong again.

**Step 3 — design-agent consumes, never writes.** Same run.
Expect MILESTONE `## Design Specifications` carries per-task `### Design:` sections referencing the contract, and `git diff` shows **no** modification to `{base}/TECH_PLAN.md` from any auto phase.

**Step 4 — the gate fires.** Verify report `## Visual Verification` shows per-row pass/fail/UNVERIFIABLE and the attestation reads `Contract checks performed: YES against {base}/TECH_PLAN.md`.
Fail: `NO`, or an acceptance-criteria pass → `:131` was not narrowed.

**Step 5 — per-task specs survive a fix round.** Force one FWLUP round so `/belmont:next` runs and re-archives MILESTONE.
Expect the re-archived `MILESTONE-M1.done.md` still carries populated per-task design sections, not the `[Not populated — lightweight mode…]` placeholder.
Note the second verify is *focused* by default and legitimately reports `Contract checks performed: NO — focused re-verification, no UI follow-ups` unless the follow-ups touched UI. v4's Step 3 expected the opposite and was unsatisfiable.

**Step 6 — headless replan preserves the contract.** Drive the loop to `actionReplan` (two verify failures, `main.go:7642-7644`).
Expect the existing `## Design Contract` preserved verbatim, `**Approval**` rewritten to `unreviewed (headless replan <date>)`, and no regeneration.
Fail: a fresh contract → R4 not implemented, and the approved design is silently gone.

**Step 7 — Mode A regression.** Feature with Figma URLs, MCP connected. Expect Figma-derived tokens in the contract, per-task Mode A sections, Figma Sources table with LOADED/FAILED.

**Step 8 — failed-load regression (must not break).** Deliberately invalid node id. Expect task BLOCKED, status FAILED, nothing invented, and **no fallback to the contract** — the contract's existence must not license a guess at a failed node.

**Step 9 — other tools.**
9a: `belmont auto --tool codex --feature <slug>` from `$BASE`.
9b: interactive `codex` → `/plan $belmont:tech-plan` → confirm the structured interview runs and a `BELMONT_PLAN_PACKET` is emitted carrying both `TECH_PLAN.md` and `design-preview.html`; apply with `$belmont:codex-plan-apply`.
9c: `belmont auto --tool opencode` — confirm Tier 2 runs and the contract is schema-identical to Step 1's.
Steps 1–6 are the Claude-in-both-modes check.

**Diagnostics.** Steps 1–6 need no Figma access. If 7 or 8 fail, test the Figma MCP directly before blaming this PR.

## 9. Definition of Done

**Placement and durability**
- [ ] Contract written to `{base}/TECH_PLAN.md` by `tech-plan` only; no auto-mode phase writes that path
- [ ] `design-agent.md` `## FORBIDDEN ACTIONS` (`:5-15`) byte-identical in the diff
- [ ] Contract survives `[r]`-resume (smoke Step 2) and headless replan (Step 6)
- [ ] `{base}/design-preview.html` written, committed, and opens standalone with no network references

**Authority and portability**
- [ ] `references/design-authority-baseline.md` created, cited from the tech-plan body as a literal `references/…` path, and present in `skills/belmont/tech-plan/references/` after generation
- [ ] Tier 2 is normative; contract schema identical across tiers; no downstream consumer branches on tier
- [ ] `**Authorities**` line always present, never silently omitted
- [ ] Baseline is Belmont-original prose citing public standards; Yummy Labs credited; **no file from `~/.claude/skills/` copied into the repo**
- [ ] `vercel-react-best-practices` → `vercel:react-best-practices` and the bogus `security` entry fixed in both `tech-plan.md:160` and `product-plan.md:165`

**Gate**
- [ ] Phase 2 three-way; `:131` narrowed to "no references AND no contract"; fourth enforcement rule present
- [ ] Every check row names a mechanism; unavailable ⇒ `UNVERIFIABLE`, never `PASS`
- [ ] `Actual` sourced from the running UI; config-sourcing forbidden in prose
- [ ] Focused-reverify behaviour stated explicitly and matched by smoke Step 5
- [ ] Playwright MCP tool names corrected to `mcp__plugin_playwright_playwright__*` in both agents
- [ ] `next.md` archive merges rather than clobbers per-task sections

**Non-regression**
- [ ] Failed Figma load still BLOCKS and does not fall back to the contract (Step 8)
- [ ] `SKIPPED` still forbidden as a Figma node status
- [ ] Mode A structural assertion passes (Step 7)

**Corrections carried by this PR**
- [ ] `AGENTS.md:26` and `dual-invocation-paths.md:9` corrected — auto mode does **not** bypass skill discovery and Belmont injects only steering text
- [ ] v4's dead `#10`/`#11` conflict block and its DoD item deleted
- [ ] v4's `agents/belmont/references/` paragraph (§5) and its DoD item deleted — they contradicted the inlining decision and each other

**Mechanics**
- [ ] `./scripts/generate-skills.sh --check` and `./scripts/generate-plugin.sh <ver> && git diff --exit-code plugin/` pass
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green
- [ ] Auto, interactive and plugin surfaces all exercised, including Codex packet and one non-Claude CLI
- [ ] `design-authority.md` created with `Don't re-do`; routing row added; `Revisions` lines on `model-tier-economics.md` and `dual-invocation-paths.md`
- [ ] 0004's post-Mode-B re-baseline executed and reported (§11)

## 10. Risks

| Risk | Mitigation |
|---|---|
| Contract written from a worktree and silently lost | Hard rule in §4.1; smoke Steps 2 and 3; `--assume-unchanged` makes such a write uncommittable, so it fails loudly at review time rather than shipping |
| Headless replan regenerates an approved contract | R4 preserve-verbatim rule; smoke Step 6. **Prose-enforced only** — `runScopeGuard` exempts `actionReplan` |
| Gate never fires after the first failure | Focused-reverify exemption stated; smoke Step 5 asserts the attestation wording |
| Contract exists but licenses a guess on a failed Figma load | Mode keyed on URL presence only; §4.4 forbids the inference; smoke Step 8 |
| Two quality tiers — author's runs better than everyone's | Tier 2 normative, schema identical, `Authorities` recorded |
| Per-task specs still clobbered by `next.md` | `next.md` kept in scope; coordinate with 0004, which lands first |
| Checks unimplementable → silent pass | `UNVERIFIABLE` never counts as pass; enforcement rule makes a missing check a FAIL |
| Circular checking (`Actual` read from config) | Explicitly forbidden; smoke Step 4 spot-checks one row against the live DOM |
| Preview rots against the contract | Both authored in the same step from the same source; the preview is generated, never hand-edited |

## 11. Interaction with PR 2 (0004)

0004 lands first and measures its reduction against a **pre-Mode-B** baseline, reporting it as an upper bound. **This PR owes the re-baseline.** Run 0004's Tier-2 gate after this PR lands and report the post-contract figure, naming the number it supersedes.

The restructure plausibly *reduces* the fan-out cost 0004 defers: design derivation moves from once-per-milestone to once-per-feature, and the per-task sections consume a contract rather than re-deriving one. That is a hypothesis, not a result — measure it.

**Shared files.** 0004 edits `_src/verify.md` at `:110`/`:147`; this PR touches Step 1b at `:70` and the contract-shape recognition nearby. **The "guaranteed conflict" claim in earlier revisions is false** — it was reproduced on the real file at git 2.50.1: 0004 rewriting `:110` and a three-line insertion before `:114` rebases cleanly, and stays clean even when 0004's edit is widened to `:108-112`. The "default diff context" reasoning applies to `git am`/`git apply` of a patch file, not to the three-way merge that `git rebase` uses. A conflict arises only if both sides touch `:113`. Both PRs also edit `_src/next.md` and `models-yaml-format.md` — those are real overlaps. Resolve `plugin/` conflicts by regeneration, never by hand (`generate-plugin.sh:35` does `rm -rf`).

## 12. Changes from v4

| v4 | v5 |
|---|---|
| Contract derived in `implement` Phase 2, stored in MILESTONE | Derived in `tech-plan` Phase 3.5, stored in `{base}/TECH_PLAN.md`, authored in the master tree |
| "MILESTONE and a separate file have the same resume profile" | True, and irrelevant — the axis that matters is **master-authored vs worktree-authored**. `main.go:10011` re-copies from master |
| `main.go:9980` cited for `os.RemoveAll` | `:10010`. `:9980` is the function signature. Four documents carried the wrong line |
| No human approves the design | tech-plan's existing structured-question gate, plus a rendered `design-preview.html` |
| `next.md` out of scope (§5:201) **and** in scope (§5:192, §4.1) | Contradiction deleted. It stays **in** scope — per-task specs are still clobbered |
| Risk row mitigated by "separate durable file" (§10:307) | Stale v3 leftover, deleted |
| §5 specifies `agents/belmont/references/` (`:197`) while `:187` says no such mechanism exists | Both leftovers deleted; the new reference is a **skill** reference, where the mechanism does exist |
| DoD asserts the reference file exists (`:295`) **and** that no new agent files exist (`:296`) | Mutually exclusive. Both deleted |
| `Don't re-do`: "storing the contract in MILESTONE (rejected)" while v4 stores it in MILESTONE | Now consistent — MILESTONE storage is genuinely rejected |
| §4.7 tier guidance "becomes unnecessary — planning is forced to Opus" | False. `planningTier` applies to `actionReplan` only, i.e. auto mode; the interactive path this PR relies on inherits the session model. Guidance retained and retargeted |
| "verify.md Step 1b recognises only Figma tables and images" | False — `:70` already reads TECH_PLAN for visual specs. The edit shrinks |
| "implementation-agent `:190-195` is a no-op without Figma" | False — `:196`/`:197` handle non-Figma and no-reference cases. `:272` is mode-agnostic |
| "nothing runs the project's lint/typecheck" | False — implementation-agent runs both per task (`:120`, `:146`, `:149`). Restated as "verification never re-runs them"; §4.5 dropped |
| ⚠ open PRs #10/#11 conflict | Both closed 2026-08-06. Block and its DoD item deleted |
| `release.sh:39-41` "regenerates and `git add`s" | `git add` is at `:96` |
| Smoke Step 3 expects contract checks after a fix round | Unsatisfiable — focused re-verify skips design comparison by default. Rewritten as Step 5 |
| Non-Claude CLIs "have no skill mechanism" | False — opencode has a native `skill` tool, Gemini `activate_skill`, others auto-activate. The **skills** are absent, not the mechanism |
| Auto mode "bypasses skill discovery; Belmont injects the skill body" | False (`AGENTS.md:26`, `dual-invocation-paths.md:9`). Bare slash reference; only steering is injected. Correcting both is in scope |
| `_src/verify.md` "Phase 2" | verify.md has **Steps**, not Phases. Phase 2 belongs to `verification-agent.md`. Every edit now names its file |
| "~450 lines / 6 files" | ~520 / 9 tracked sources + 2 corrections |
| Design numbers sourced from `ux-designer`/`ui-designer` | Same rules, Belmont-original prose citing WCAG/public conventions, crediting Yummy Labs. Their files are unlicensed and must not be vendored |
