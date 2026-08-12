# Design Authority Baseline

Read this when deriving a **Design Contract** in `/belmont:ux-design` — that is,
when the feature has a user interface and **no** task in its PRD carries a Figma
URL.

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
family**, and name in the contract's `**Source**` field every rung you actually
took content from. Never invent a competing scale where one already exists.

Rung 1 routinely supplies components and states while supplying **no** token
family — a story index carries component and story *names*, not colours or
spacing. That is not a reason to stop there for tokens: keep walking, and name
both rungs, one per family — e.g.
`storybook (<url>) — components & states; tailwind.config.ts — tokens`.

0. **Master contract** — the Token Contract and Accessibility Floor in
   `.belmont/UX_DESIGN.md`.
0b. **Sibling contract** — an approved `## Design Contract` in another
    `.belmont/features/*/UX_DESIGN.md`. Most recently approved wins; name which
    feature. *Without rungs 0 and 0b, feature 1 and feature 4 of a greenfield
    project mint different scales with nothing to reconcile them.*
1. **Storybook, if present** — the strongest source below a contract, because
   stories enumerate components *and their states*, which is exactly what State
   Inventory needs. It comes in two forms and **the hosted one is better**:

   **1a. A deployed Storybook.** Use only a URL you can *identify as a Storybook*:
   one named as such in the master `TECH_PLAN.md` or the feature PRD, a
   Storybook-named field or script in `package.json`, or one the user gives you
   during the interview. **`homepage` and `repository` are not Storybook URLs** —
   they are the product site and the source repo, and appending `/index.json` to
   either fetches something that is not a story index. If you have found no URL you
   can identify, ask — but see *Asking is interactive-only* below. Then fetch:

   ```
   <storybook-url>/index.json      # the story index — components and their stories
   ```

   `index.json` is a **static build artifact**. Fetching it runs nothing, needs no
   port, and starts no server — so `forbidden-actions`' prohibition on build and
   package-manager commands does not apply. Use whatever single-URL fetch your tool
   provides (`WebFetch` on Claude Code, which this skill's ALLOWED ACTIONS already
   permit for a user-provided URL). If your tool has none, skip to 1b.

   Its `entries` map is the richest inventory available anywhere: each entry has a
   `title` (the component path, e.g. `Bookings/BookingPicker`) and a `name` (the
   story, e.g. `Empty`, `Loading`, `Pending`, `Declined`, `Long Headline`).
   **Story names give you the State Inventory's state lists directly** — they are a
   real team's enumeration of the states that thing actually has, which is strictly
   better than your guess. Ignore `Docs` entries (`type: "docs"`); they are
   generated, not states.

   **Component titles are evidence, not names.** They tell you what already exists,
   which is how you tell a surface the feature *builds* from one it *reuses
   unchanged* — and only the former gets a State Inventory entry. They never become
   surface names: surfaces are named from the PRD's flows (see *State Inventory*
   below), and mapping a surface onto one or more components is `/belmont:tech-plan`'s
   job, in its `## Component Specifications`. A title copied into the `## Screens`
   table would enumerate the inventory per component file, which is exactly the
   framing this phase exists to move away from — components are chosen after this
   session, not during it.

   **You have read a story index only if** the response body parses as JSON *and*
   carries an `entries` object (Storybook 7+) or a `stories` object (6.x) whose
   values have `title` and `name` fields. Anything else is **not** one: a 404, a
   redirect to a login or marketing page, an HTML error page or SPA shell, a
   deployment-protection interstitial, JSON of some other shape, or a fetch you
   could not make. **A 200 proves nothing** — the same client-rendered trap as the
   download links above. On any of those, say which happened and fall through to 1b
   or rung 2. Never invent an inventory, and never write a `storybook` `**Source**`
   you could not read.

   **Asking is interactive-only.** A deployed Storybook is the one rung you cannot
   discover by reading the repo, so ask for it while the interview is live.
   `/belmont:ux-design` refuses to run headlessly at all, so this rung is never
   reached non-interactively — never ask outside a live session: use only a URL
   already present in the files above, and if there is none, go straight to 1b.

   **1b. Local story files.** Detect via `.storybook/`, a `storybook` entry in the
   package manifest, or `*.stories.@(js|jsx|ts|tsx|mdx)` files. **Read the story
   files and `.storybook/preview.*` statically — do NOT run Storybook.** A planning
   session may not run build or package-manager commands, and booting it would
   collide with worktree port isolation. A local build's `storybook-static/index.json`,
   if one is already committed, counts as 1a and is preferable to parsing sources.

   Either way: use the inventory to fill each flow-named surface's **States**
   column in `{base}/UX_DESIGN.md`'s `### State Inventory`, taking the story names
   of whichever component already realises that surface, and to mark as *reused
   unchanged* — no entry, one-line reason — any surface an existing component
   already covers. Treat what the stories record as **LAW**: the contract records
   the states the team built, it does not redesign or prune them. Do not write
   component titles into the `## Screens` table's *Surfaces it contains* column.
   Add `storybook (<url>)` or `storybook (local)` to `**Source**` so a reader
   knows which you used. **Adding, not replacing**: a story index supplies components
   and states, not tokens, so keep walking to rung 2 for the Token Contract and
   name that rung too. `**Source**` must always name the artifact each family of
   values actually came from — `verification-agent.md`'s anti-circularity rule is
   keyed on it, and a rung named there but not read from, or read from but not
   named, disarms it.
2. **Project config** — `tailwind.config.*`, CSS custom properties in
   `globals.css`, `components.json`, theme files.
3. **Nothing exists** → establish the tier-2 defaults below and set
   `**Source**: none — established here`.

On a greenfield project rungs 0–2 may all be empty — `tailwind.config.*` and
`globals.css` do not exist until code is written, and the styling approach is
chosen later, in `/belmont:tech-plan`. That is the expected first-feature case:
fall to rung 3 and set `**Source**: none — established here`. The contract
records token **values**, not their expression, so nothing is lost by fixing them
before the stack is chosen.

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
  headings. **Max 4 sizes** in one feature. Every size must actually sit on the
  ratio you named: a scale of 14 / 16 / 18 / 31 is not a 1.25 scale, and a ratio
  that does not describe the sizes beneath it is decoration.
- **Type families** — **one family carries display and body.** A second family
  appears only in named outlier slots, and you must enumerate them (a wordmark,
  numerals, code). A third family is never correct. Headings are roman —
  italic display type is a recognisable machine tell, not a decision.
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
project already ships a typeface; naming a typeface the project neither ships
nor loads, which silently falls back to the system stack and makes the contract
describe a design nobody will see.

### Accessibility Floor

This is the one section a tier-1 skill may never lower.

- Touch targets ≥ 44×44px — **WCAG 2.1 SC 2.5.5, Level AAA**, adopted as
  Belmont's floor.
- Contrast ≥ 4.5:1 for body text, ≥ 3:1 for large text — **SC 1.4.3, Level AA**.
  **Every ratio you write down is one you computed**, by WCAG 2.x relative
  luminance, on the actual pair of values in the contract — not an estimate, not
  a recollection of what that colour usually scores, and not a number carried
  over from the source you took the palette from. Convert to sRGB first where the
  palette is authored in another space; a lightness percentage is not a
  luminance. Show the pair adjacently in `design-preview.html` at body size so a
  human can check the claim by looking, which is what that artifact is for. A
  ratio nobody computed is the same defect as a component inventory nobody
  fetched: it reads as evidence, the verification agent treats it as authority,
  and it is worth less than saying nothing. If you cannot compute one, write the
  pair and say so.
- A visible focus indicator on every interactive element — **SC 2.4.7, Level AA**.
- Every input has a visible label. **Placeholder-only is a failure**, not a style.
- Content reflows at 320px with no horizontal scrolling — **SC 1.4.10, Level AA**.
  A control's label fits on one line at that width, or the label is too long: a
  button that wraps to two lines is a copy decision that was never made.
- `prefers-reduced-motion: reduce` is respected — **SC 2.3.3, Level AAA**.
- No meaning conveyed by colour alone.

### UX Strategy

Five lines, no more: the user · their emotional state on arrival · the hero
element · the primary action · the biggest UX risk.

### State Inventory

Per interactive **surface** the feature introduces — a control or region the user
acts on, named from the PRD's flows and the `## Screens` table, never from a file
path or a component name: default · hover · focus · active · disabled · loading ·
empty · error, plus any gesture or transition state the interaction introduces
(drag, swipe, long-press, optimistic update).

A surface the feature **reuses unchanged** gets no entry. Any omission carries a
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
