# Design Authority Baseline

Read this when deriving a **Design Contract** in `/belmont:tech-plan` Phase 3.5 —
that is, when the feature has a user interface and **no** task in its PRD carries
a Figma URL.

Two things live here:

1. **The authority ladder** — which design skills to load, per contract section.
2. **The tier-2 baseline rules** — Belmont's normative floor, used when a
   tier-1 skill is unavailable for a section. This is the tested path.

---

## Attribution

The rules below are Belmont-original prose stating **published standards**: WCAG
2.1 success criteria, the 8pt spacing grid, ratio-derived type scales, and the
60-30-10 colour convention. Belmont adopts them as *its own* floor — that is a
project choice, not a WCAG requirement. **State the conformance level wherever
you cite one**; two of the four criteria below are AAA, and presenting an AAA
criterion as a mandatory floor without saying so misrepresents the standard.

The *curation* — which rules matter for a checkable contract, and how they
compose into one — is drawn from the [Yummy Labs](https://www.yummy-labs.com/)
Claude design skills. Credit them by name when the contract cites this file.

| Skill | Contract section it feeds |
|---|---|
| [ui-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | Token Contract; "an existing design system is LAW"; the banned-defaults list |
| [ux-designer](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) | UX Strategy; the component-state half of State Inventory |
| [ux-copywriter](https://drive.google.com/drive/folders/1lSwUatVOzOX5TGWgBDjKA820RiUsVLNr) | Microcopy Rules |
| [ux-motion](https://drive.google.com/drive/folders/1gYrK4aT4A-LYr4GbM88-PaKu0wYyY4C6) | Motion Contract; the transition half of State Inventory |

**Their files are not vendored into Belmont.** All four are distributed as
downloads with no LICENSE, no repository, and no redistribution grant. Belmont
states the same published standards in its own words and credits the source.

---

## The authority ladder

Check for these skills **by name, in your available skills**. Do not probe the
filesystem. Load **every** tier-1 skill that is present — they cover different
contract sections and are not alternatives to one another.

### Tier 1 — enrichment, per section

| Skill | Enriches |
|---|---|
| `ui-designer` | Token Contract |
| `ux-designer` | UX Strategy **and** the component-state half of State Inventory |
| `ux-copywriter` | Microcopy Rules |
| `ux-motion` | Motion Contract **and** the transition half of State Inventory |

A section whose skill is absent falls back to the tier-2 baseline **for that
section alone**. Never for the whole contract.

### Tier 1b — conditional

- `frontend-design:frontend-design` — aesthetic direction.
- `dataviz` — **only** if the feature renders charts.
- `vercel:shadcn` — **only** if the project already depends on shadcn. Evidence
  is `components.json` at the repo root or a shadcn entry in the package
  manifest, never a guess.

### Tier 2 — normative floor, all tools, always

The baseline rules in this file. Tier 2 runs *underneath* tier 1, not instead of
it: a tier-1 skill may add detail, or argue a deviation and record it, but may
never lower the accessibility floor.

### The schema does not vary by tier

**The contract has the same six sections whichever tiers ran, and no downstream
consumer may branch on which ones did.** Record what ran on an `**Authorities**`
line, naming the skill per section:

```
**Authorities**: baseline; ui-designer (tokens); ux-copywriter (microcopy)
**Authorities**: baseline only (no design skills available in this session)
```

**Never omit that line.** A single available/unavailable switch would give a user
holding three of the four skills the baseline for all four sections, and would
hide which sections were actually enriched — which is why it is per-section.

### Portability — state it exactly

All four tier-1 skills are **user-scope Claude Code skills** under
`~/.claude/skills/`. None ships with Belmont, and none is present for most
users. What is absent on other tools is the **content**, not the mechanism —
opencode exposes a native `skill` tool, Gemini has `activate_skill`, and
Codex / Cursor / Windsurf / Copilot / Pi auto-activate on `description:`. On the
seven non-Claude CLIs every section runs tier 2.

**Tier 2 is therefore the tested path, not the fallback.**

### Obtaining the tier-1 skills

None of these is bundled with Belmont; all are optional; tier 2 works without
them. If you want the enrichment:

| Skill | Source | Archive | Installs to |
|---|---|---|---|
| `ui-designer`, `ux-designer` | [Claude UX & UI Design Skills](https://yummy-design.notion.site/Claude-UX-UI-Design-Skills-31462791470981a99fe1c993b08c5347) (Notion) | `ux-ui designer skill Jul 26.zip` | `~/.claude/skills/{ui,ux}-designer/` |
| `ux-copywriter` | [Drive folder](https://drive.google.com/drive/folders/1lSwUatVOzOX5TGWgBDjKA820RiUsVLNr) | `ux-copywriter.zip` | `~/.claude/skills/ux-copywriter/` |
| `ux-motion` | [Drive folder](https://drive.google.com/drive/folders/1gYrK4aT4A-LYr4GbM88-PaKu0wYyY4C6) | `ux-motion.zip` | `~/.claude/skills/ux-motion/` |

Vendor homepage, always valid: <https://www.yummy-labs.com/>.

The designer pair ships as `.skill` bundles (themselves archives) inside one zip;
the other two ship as plain directories. Either way the on-disk result is one
directory per skill under `~/.claude/skills/`.

> **These are unauthenticated share links the vendor controls.** They can be
> revoked, re-pointed, or renamed without notice. If one 404s, start from the
> vendor homepage. And never "verify" one with an HTTP status code — Notion and
> Google Drive are client-rendered and return 200 with an empty shell for almost
> any path, so a status check cannot tell a real page from a fabricated one.

---

## Derivation order — reuse before invention

Walk this ladder in order. **Stop at the first rung that supplies a token
family, and name that rung in the contract's `**Source**` field.** Never invent
a competing scale where one already exists.

0. **Master contract** — a `## Design Contract` in `.belmont/TECH_PLAN.md`.
0b. **Sibling contract** — an approved `## Design Contract` in another
    `.belmont/features/*/TECH_PLAN.md`. Most recently approved wins; name which
    feature. *Without rungs 0 and 0b, feature 1 and feature 4 of a greenfield
    project mint different scales with nothing to reconcile them.*
1. **Storybook, if present** — the strongest source below a contract, because
   stories enumerate components *and their states*, which is exactly what State
   Inventory needs. It comes in two forms and **the hosted one is better**:

   **1a. A deployed Storybook.** Detect from a URL in the master `TECH_PLAN.md`,
   the feature PRD, `package.json` (`homepage`, `repository`, or a `storybook`
   script's `--url`), or simply because the user named one during the interview —
   ask if the project has one and you have not found it. Then fetch:

   ```
   <storybook-url>/index.json      # the story index — components and their stories
   <storybook-url>/project.json    # framework and addon metadata (optional)
   ```

   `index.json` is a **static build artifact**. Fetching it runs nothing, needs no
   port, and starts no server — so `forbidden-actions`' prohibition on build and
   package-manager commands does not apply. Use `WebFetch`, which this skill is
   already permitted to use.

   Its `entries` map is the richest inventory available anywhere: each entry has a
   `title` (the component path, e.g. `Bookings/BookingPicker`) and a `name` (the
   story, e.g. `Empty`, `Loading`, `Pending`, `Declined`, `Long Headline`).
   **Component titles give you `## Existing Components to Reuse`; story names give
   you the State Inventory directly** — they are a real team's enumeration of the
   states that component actually has, which is strictly better than your guess.
   Ignore `Docs` entries (`type: "docs"`); they are generated, not states.

   If the URL 404s or the JSON will not parse, say so and fall through to 1b or
   rung 2 — never invent an inventory and never claim a Storybook source you could
   not read.

   **1b. Local story files.** Detect via `.storybook/`, a `storybook` entry in the
   package manifest, or `*.stories.@(js|jsx|ts|tsx|mdx)` files. **Read the story
   files and `.storybook/preview.*` statically — do NOT run Storybook.** A planning
   session may not run build or package-manager commands, and booting it would
   collide with worktree port isolation. A local build's `storybook-static/index.json`,
   if one is already committed, counts as 1a and is preferable to parsing sources.

   Either way: record the component inventory into the feature template's
   `## Existing Components to Reuse`, treat those components as **LAW** — the
   contract records them, it does not redesign them — and set `**Source**` to
   `storybook (<url>)` or `storybook (local)` so a reader knows which you used.
2. **Project config** — `tailwind.config.*`, CSS custom properties in
   `globals.css`, `components.json`, theme files.
3. **Nothing exists** → establish the tier-2 defaults below and set
   `**Source**: none — established here`.

---

## Tier-2 baseline rules

These are the values to commit to when no higher rung supplies them. Every one
of them is a *decision the contract records*, not a range the implementation may
pick from at will — write concrete numbers into the contract.

### Token Contract

- **Spacing** — 8pt grid: 4, 8, 12, 16, 24, 32, 48, 64, 96. A component's
  internal padding is ≤ its external margin, always.
- **Typography** — one ratio (1.2 minor third / 1.25 major third / 1.333 perfect
  fourth), sizes derived from it, line-height 1.4–1.6 for body and 1.1–1.3 for
  headings. **Max 4 sizes** in one feature.
- **Colour** — 60/30/10 distribution (dominant / secondary / accent). Max 3 hues
  plus neutrals. **Never pure `#000` or `#FFF`.** Body text ≥ 4.5:1 against its
  background. Each semantic colour (success / error / warning / info) declares
  all four of background, border, text, icon — a semantic that only declares a
  text colour produces unreadable alert boxes.
- **Radius** — one committed value (8px is a reasonable default). A nested
  child's radius is strictly smaller than its parent's.
- **Elevation** — a named, ordered set of levels. Interactive elements rise
  exactly one level on hover.

**Banned defaults.** Unstyled browser focus rings removed without replacement;
`border-radius: 0` on every surface as a non-decision; pure-black text on
pure-white; a shadow on everything; system-font-only typography where the
project already ships a typeface.

### Accessibility Floor

This is the one section a tier-1 skill may never lower.

- Touch targets ≥ 44×44px — **WCAG 2.1 SC 2.5.5, Level AAA**, adopted as
  Belmont's floor.
- Contrast ≥ 4.5:1 for body text, ≥ 3:1 for large text — **SC 1.4.3, Level AA**.
- A visible focus indicator on every interactive element — **SC 2.4.7, Level AA**.
- Every input has a visible label. **Placeholder-only is a failure**, not a style.
- `prefers-reduced-motion: reduce` is respected — **SC 2.3.3, Level AAA**.
- No meaning conveyed by colour alone.

### UX Strategy

Five lines, no more: the user · their emotional state on arrival · the hero
element · the primary action · the biggest UX risk.

### State Inventory

Per interactive component the feature **builds**: default · hover · focus ·
active · disabled · loading · empty · error, plus any gesture or transition state
the interaction introduces (drag, swipe, long-press, optimistic update).

Components the feature only **consumes** get no entry. Any omission carries a
one-line reason.

### Microcopy Rules

- **Buttons** — a verb matching the outcome. Never "Submit".
- **Errors** — what happened / why / what to do next. Never blame the user, never
  a joke.
- **Empty states** — what appears here, why it is empty, the action that fills it.
- **Destructive confirmations** — name the thing being destroyed.
- Possessive framing for user-owned objects ("your projects", not "the projects").

### Motion Contract

**This section is conditional.** If the feature introduces no transition,
animation, or micro-interaction, record `**Applies**: N/A — no motion in this
feature` and leave the four rules unfilled. Do not leave it silently empty — a
downstream agent must be able to tell "does not apply" from "not done".

- **Duration bands** — instant feedback ≤ 100ms · state change 150–200ms ·
  context shift 250–300ms · page transition 300–400ms. Anything above 400ms must
  justify itself in the contract.
- **Easing** — one documented curve per class (enter / exit / move). Bounce and
  overshoot are opt-in per product voice, never a default.
- **Performance** — animate `transform` and `opacity` only. Animating layout
  properties (`width`, `height`, `top`, `margin`) is a defect, not a trade-off.
- **Reduced motion** — `prefers-reduced-motion: reduce` removes movement, never
  functionality. The element must still reach its end state.

`prefers-reduced-motion` stays in the Accessibility Floor regardless of this
section's `**Applies**` value: it is a requirement about honouring a user
preference, and a feature with no motion satisfies it trivially rather than
being exempt from it.
