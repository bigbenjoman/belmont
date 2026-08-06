# 0003 — UX/UI design restructure brief

Standalone brief for the design change. Read this **before** `0003-design-quality-without-figma.md`, which is still at rev 4 and describes the **superseded** design.

If you take one thing from this file: **rev 4 puts design inside `implement`. That is what we are changing. Do not implement rev 4.**

---

## 1. What is broken today (on `main`, no proposal applied)

`agents/belmont/design-agent.md` is a **Figma extractor**. It runs as Phase 2 of `/belmont:implement`, in parallel with the codebase agent, once per milestone.

When a task has a Figma URL it loads the design and writes exact specs. When there is **no Figma URL** — which is most projects — its entire behaviour is `## Handling No Design` (lines 270–276): four bullets that all defer ("note that no design references were provided", "recommend following existing component patterns"). It produces nothing usable.

Downstream, `verification-agent.md` Phase 2 is **comparison-only**. No references to compare against means it falls back to acceptance-criteria checking, which is a correctness check, not a design-quality check.

**Net effect on a no-Figma project: nothing specifies the design, and nothing checks it.** Visual quality is whatever the implementation agent improvises.

## 2. What rev 4 proposed (SUPERSEDED — do not build)

Keep design in `implement` Phase 2, but give design-agent a second mode:

- **Mode A** — Figma URL present → extract, unchanged.
- **Mode B** — no Figma URL → *derive* a design contract (token scale, accessibility floor, UX strategy, microcopy rules) and write it into the MILESTONE file.

The idea was right. The **placement** was wrong, and three revisions were spent patching consequences of that placement:

| Problem | What rev 2–4 tried |
|---|---|
| Contract destroyed by `/belmont:next` re-archiving MILESTONE | Move it to a separate `DESIGN-CONTRACT-<M>.md` file |
| Separate file wiped on `[r]`-resume (`copyBelmontStateToWorktree` does `os.RemoveAll` on the feature dir, preserving only `STEERING.md`) | Move it back into MILESTONE, and patch `next.md` to merge instead of overwrite |
| Writing outside MILESTONE breaks `design-agent.md:5-15` (`FORBIDDEN ACTIONS`: write to no file but MILESTONE) | Relax the rule — which weakens the agent's strongest guard |
| No human ever approves the design before code is written | Never addressed |

All four dissolve if design moves earlier.

## 3. What to build instead

**Move the feature-level design work into `tech-plan`, which runs after `product-plan` and before any implementation.**

### Pipeline, before and after

```
NOW:
  product-plan  → PRD.md
  tech-plan     → TECH_PLAN.md + PROGRESS.md milestones
  implement     → [codebase-agent ‖ design-agent] → implementation-agent   ← design happens here,
  verify        → [verification-agent ‖ code-review-agent]                    once per milestone

AFTER:
  product-plan  → PRD.md
  tech-plan     → TECH_PLAN.md (now including the DESIGN CONTRACT) + PROGRESS.md
                  ↑ interactive: user reviews and approves before auto starts
  implement     → [codebase-agent ‖ design-agent] → implementation-agent
                                     ↑ consumes the approved contract, writes per-task specs only
  verify        → [verification-agent ‖ code-review-agent]
                    ↑ checks the built UI against the approved contract
```

### Why this is right — three verified facts

| Fact | Evidence | Consequence |
|---|---|---|
| `tech-plan` is already interactive | `_src/tech-plan.md` includes `user-questions.md` and `dynamic-questioning.md` | The approval gate already exists. No new checkpoint machinery, no Go change. |
| `TECH_PLAN.md` is **master-owned** | Copied master → worktree by `copyBelmontStateToWorktree`, so it is restored on `[r]`-resume. MILESTONE is worktree-*created*, so `os.RemoveAll` (`main.go:9980`) destroys it. | Durability is solved by construction, not patched. |
| Planning is forced to Opus | `planningTier = "high"` (`main.go:102`) | The node authoring the contract runs on the strongest model automatically. Rev 4's `models.yaml` tier guidance becomes unnecessary. |

Plus: **one design pass per feature instead of one per milestone.** Today design-agent re-derives tokens every milestone — wasteful, and nothing stops M1 and M4 from choosing different spacing scales.

## 4. Responsibilities after the change

**`skills/belmont/_src/tech-plan.md` — gains a design step**

Produces a **feature-level design contract**, written into `TECH_PLAN.md`, covering the whole feature:

- **Token contract** — spacing scale, type scale and ratio, colour roles, radius, elevation. **Must record its source**: name the file it was read from (`tailwind.config.*`, CSS custom properties, `components.json`, a theme file) or state that none exists and one is being established.
- **Accessibility floor** — target sizes, contrast ratios, focus visibility, labelling, reduced-motion.
- **UX strategy** — who the user is, their state arriving, the hero element, the primary action, the biggest risk.
- **Microcopy rules** — button labels, the three-part error format, empty-state copy, destructive confirmations.

Reuse before invention: if the project already has a design system, **record it** rather than inventing a competing one.

This step runs only when the feature has UI. A backend-only feature skips it entirely and `TECH_PLAN.md` says so explicitly, so downstream agents can tell "no UI" from "not done".

**`agents/belmont/design-agent.md` — narrows**

Still Phase 2 of `implement`, still per-milestone, but it **consumes** the contract rather than inventing one:

- **Figma URL present** → extract per-task specs from Figma, as today. Unchanged, including every failure rule. The `NO FALLBACK` rule at `:255` stays.
- **No Figma URL** → derive per-task component specs *against the approved contract from `TECH_PLAN.md`*. For each component the milestone creates, enumerate every state: default, hover, active, focus, disabled, loading, error, empty, success. For components it modifies, record the delta only.

It still writes only to MILESTONE's `## Design Specifications`, so `FORBIDDEN ACTIONS:5-15` stays untouched — no rule relaxation anywhere.

**`agents/belmont/verification-agent.md` — gains a real gate**

Phase 2 becomes three-way:

1. Design references exist → current comparison flow, unchanged.
2. No references, but `TECH_PLAN.md` carries a design contract → **check the built UI against it**.
3. No references and no contract → current acceptance-criteria fallback, unchanged. This is the failed-Figma-load path and must keep working.

Every check names its measurement mechanism, and a check whose mechanism is unavailable is recorded `UNVERIFIABLE` — **never** `PASS`. Values must come from the running UI, never re-read from the config the contract was derived from; that would be circular.

Critically: `verification-agent.md:131` currently permits a pass on acceptance criteria alone whenever no references exist. **Narrow it** to "no references *and* no contract", or the new gate is opt-out by construction.

## 5. The three user requirements

**(1) Use the `ux-designer` and `ui-designer` skills during the design step.**

These are the source of the numbers — 8pt grid, type scale from a ratio, 60-30-10 colour, internal ≤ external spacing, all-states enumeration, three-part errors, 44px targets, 4.5:1 contrast.

**Constraint that makes "just call them" wrong:** they live at `~/.claude/skills/`. They are the user's personal skills and will not exist in other people's projects, and non-Claude CLIs have no equivalent mechanism at all. So invocation must be **conditional** — use them when present; otherwise fall back to an inlined checklist carrying the same rules. The contract must come out identical either way.

**(2) Consider `frontend-design`.**

A plugin skill covering aesthetic direction and typography, aimed at not producing templated defaults. That is precisely the failure mode of design derived from nothing — it converges on generic. Worth invoking on the same conditional basis. `dataviz` applies only if the feature has charts; `vercel:shadcn` only if the project uses shadcn. The Figma skills are irrelevant here by definition.

**(3) Show durable artifacts for approval before implementation.**

Two halves, and only one is solved.

*Durability — solved.* `TECH_PLAN.md` is master-owned, survives resume, and is already read by both implementation-agent and verification-agent. It is exactly the durable reference wanted.

*Reviewability — the open question.* Markdown buried in a plan file is not reviewable **design**. A token table tells you the spacing scale; it does not show you what the interface looks like. Options to weigh:

- Render the contract as a standalone HTML/SVG preview — swatches, type ramp, spacing scale, component states — written next to `TECH_PLAN.md`.
- Keep it textual but structure it as a dedicated, human-facing section rather than plan prose.
- Defer the visual artifact entirely and rely on `tech-plan`'s existing interactive approval of the text.

**Ask the user which they want before building it.** This is a product decision, not an implementation detail.

## 6. What changes in the spec

`0003-design-quality-without-figma.md` needs a **rev 5**:

| Section | Change |
|---|---|
| §4.1 contract storage | Replaced — contract lives in `TECH_PLAN.md`, written by `tech-plan` |
| §4.3 contract format | Split — feature-level contract vs per-task component specs |
| §5 scope | `_src/tech-plan.md` **joins**; `_src/next.md` probably **leaves** (no archive-clobbering exposure once the contract is not in MILESTONE) |
| §4.7 tier guidance | Delete — `planningTier = high` handles it |
| §8 smoke test | Rewritten — must prove the contract survives `[r]`-resume and a `/belmont:next` fix round |
| Both-paths | `tech-plan` runs in auto **and** interactive; the design step must work in both, and degrade cleanly where skills are unavailable |

Give rev 5 one adversarial pass before implementing. Every prior revision of this spec shipped a defect that survived to the next round.

## 7. What NOT to do

- Do not implement rev 4 as written.
- Do not relax `design-agent.md:5-15` (`FORBIDDEN ACTIONS`). If a change seems to require writing outside MILESTONE, the placement is wrong again.
- Do not create `agents/belmont/references/` — no such mechanism exists; `generate-plugin.sh:78` globs agents as flat `*.md`.
- Do not weaken the Figma failure rules. A *failed* Figma load must still block. This work is about the *absent*-Figma case; the two were conflated once already.
- Do not start any implementation until PR #21 merges (see `NEXT-SESSION.md`).

## 8. Verify before you build on it

Every file, line number and behaviour cited here was checked against the codebase — but check again. In the sessions that produced these specs, a review agent asserted a file existed that never has, and two verification mechanisms were specified without being run and turned out impossible. Treat this brief the same way: if it cites `main.go:9980` or `verification-agent.md:131`, open them.
