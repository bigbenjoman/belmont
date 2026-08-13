Domains: skills, agents

# Brand foundation: designing the empty ladder

## Why this matters

The derivation ladder's bottom rung used to be a shrug. Rung 2 read four named
config files; rung 3 said *"nothing exists — take the tier-2 defaults"*. So a
repo full of real decisions with no `tailwind.config.ts` was treated as a blank
page, and a genuine blank page got defaults instead of a design. Neither is what
a designer would do with either situation, and the second one matters more than
it looks: a greenfield project's **first feature silently decides the product's
entire visual identity**, and it was deciding it by falling back to a list.

That is now two rungs — [`2b`](#) sweeps the repo, and rung 3 runs a *Brand
foundation* stage before the feature interview. This entry records what that
stage must produce and, more importantly, the constraints on **how** it chooses,
because the how is where this problem is genuinely hard.

## Invariant

- **A brand foundation for a digital product spans a gap almost nothing else
  spans.** Counted across nine published design systems and fourteen classic
  brand books, only five topics appear in nearly all of both — logo, colour,
  typography, accessibility, voice — and the edges invert perfectly: **logo is
  14/14 in brand books and 1/9 in design systems; tokens are 7/9 in design
  systems and 4/14 in brand books.** A tool that emits tokens has produced a
  design system, not a brand; a tool that emits positioning and voice has
  produced a brand nobody can build from.
- **Eight sections, in order, because each constrains the next.** Brand
  foundation → voice and tone → colour → typography → accessibility contract →
  geometry → logo and digital identity assets → token architecture. Voice sits
  above pixels because it fixes product naming and message anatomy, which are
  the most expensive things to retrofit. Stop there and draw one real screen.
- **Accessibility is a construction rule, not an audit.** Space the ramp so a
  stated distance between steps *guarantees* the ratio, then assign every role
  and its `on-` pair at that distance. This survives a change of anchor hue and
  a theme flip; a per-pair audit survives neither. Verify numerically afterwards
  as a check, never as the method.
- **The blocklist has a half-life of roughly 12–18 months, so the mechanism
  cannot be the blocklist.** Prefer gates that check themselves: an overuse
  gate against the live Google Fonts popularity ranking, and a licence gate
  against the `google/fonts` OFL path. Both are single HTTP requests whose
  answers update without anyone editing a file.
- **Divergence is process, not instruction.** Generate directions in parallel
  with independent briefs, require them to be *structurally incompatible*, and
  kill rather than blend when choosing. A single prompt asked for three options
  returns three shades of one idea.
- **No item on the tells list is banned outright.** Each is legitimate where the
  product's evidence leads there, said out loud on the page. What is forbidden
  is arriving at one *by default*. A rule that rejects legitimate work is worse
  than no rule.

## Evidence

- **The tell moved, and it moved because of the advice.** The 2024–25 signature
  (violet-indigo primary, gradients on dark, one sans for everything) has been
  displaced by warm cream grounds with a high-contrast display serif and a
  terracotta accent — which is what the first generation of anti-slop guidance
  recommended as the cure. An automated scan of 1,590 Show HN landing pages now
  flags that cluster, and a major vendor's own design skill bans it by name as a
  default. Two independent sources, one measured and one theoretical, reach the
  same place: *"any instruction specific enough to produce a distinctive result
  is specific enough to become a new center."*
- **Ingredients are not the meal.** The sharpest statement of the problem comes
  from an author who ran his own brand skill on three brands and got three
  interchangeable pages: *"Tokens define what materials to use. They don't define
  what to build with those materials... the composition comes from the
  distribution, not the tokens."* A brand foundation that emits only colour,
  type and spacing is predicted to produce interchangeable products however good
  those values are. Composition and structural identity are the unbuilt layer.
- **Parallel divergence is the one mechanism with peer-reviewed support.** A
  *Design Science* study across three models and seven design problems found
  independent parallel generations beat no-persona prompting on diversity, and
  that putting all the perspectives in one prompt produced the **lowest**
  diversity of the strategies tested. Related work shows instruction-tuned
  models homogenise across repeated queries inside one context.
- **The accessibility problem is getting worse, not better.** The WebAIM Million
  found low-contrast text on **83.9%** of home pages in 2026, up from 79.1% in
  2025 — the most common detected failure by roughly thirty points. A palette
  that ships hexes without valid pairs is the defect, and construction-time
  guarantees are the only approach that scales.
- **The "free font" assumption is wrong often enough to gate.** The most popular
  anti-slop skill in this space recommends eight Fontshare/ITF families as free;
  they are not SIL OFL, and secondary sources contradict each other on whether
  self-hosting is even permitted. One `HEAD` request against the `google/fonts`
  OFL path settles it. Our artifacts embed the face, so an unclear licence
  disqualifies a candidate outright.
- **Nobody in the ecosystem spans the gap.** Projects with real token pipelines
  (DTCG output, OKLCH mandates, executable WCAG validators) generate from sector
  presets or archetypes and have no brand; projects that genuinely derive a
  brand stop at markdown and human selection. Being the tool that authors a
  valid, approvable foundation from nothing sits upstream of both.

## Don't re-do

- **A bigger curated list.** The list is a *filter* on candidates — reject the
  overused and the unlicensed — not a menu that supplies taste. Growing it does
  not reduce convergence; it relocates it, which is exactly what sector presets
  and named-philosophy menus do elsewhere.
- **Hard-banning the tells.** Several are precision-poor. Coloured left borders
  were widely claimed as a tell on one person's impression; gradients, centred
  heroes and all-caps labels all predate generative tools and are at least as
  attributable to popular CSS kits. Weight them.
- **Claiming a perceptual colour space produces better-looking palettes.** It
  makes contrast targets easier to hit — that is well supported. There is no
  controlled evidence it makes palettes more attractive or distinctive. The
  "designed" quality comes from the constraints: one anchor hue, tinted
  neutrals, accent scarcity, explicit semantic roles.
- **Citing design-system URLs as authorities.** Three canonical ones moved
  within a year (Polaris, Paste, Salesforce). Cite the *rule*, name the system,
  and let the reader find it.
- **Presenting any of this as validated.** No anti-slop skill or ruleset
  published anywhere has a before/after evaluation — not the popular ones, not
  the vendor ones. The honest posture, which the reference states out loud:
  checkable rules are enforced, everything else is stated as intent, and nobody
  has demonstrated the uncheckable half works.

## Revisions

- 2026-08-13 — created when the empty ladder gained rung 2b and the Brand
  foundation stage. Records the eight-section shape and its ordering, the
  brand/product inversion that makes the gap real, accessibility as a
  construction rule, the two self-checking gates that replace a rotting
  blocklist, parallel structural divergence, and the honest evidence posture.
  The open question it does not settle: composition and structural identity are
  named as the unbuilt layer but not yet produced by the stage, which emits
  ingredients only.
