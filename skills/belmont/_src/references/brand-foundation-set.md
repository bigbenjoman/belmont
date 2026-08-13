# Brand foundation: what to produce, and how to choose

Read this **only** on the empty-ladder branch — no contract, no Storybook, no config, no code,
no Figma — where `/belmont:ux-design` runs its *Brand foundation* stage. A feature with any rung
populated has nothing to take from here: reuse always beats invention.

---

## 1. What a brand foundation contains

Counted across nine published digital design systems (Polaris, Atlassian, Carbon, Material 3,
Primer, Paste, Spectrum, Salesforce, GOV.UK) and fourteen classic brand books (NASA, Mozilla,
Firefox, Spotify, Uber, Airbnb, Slack, Starbucks, Duolingo, Asana and others), only five topics
appear in nearly all of both: **logo, colour, typography, accessibility, voice**.

The edges invert perfectly, and that inversion is the trap. **Logo appears in 14/14 brand books
and 1/9 design systems. Tokens appear in 7/9 design systems and 4/14 brand books.** A brand
foundation *for a digital product* has to span that gap deliberately, which is what almost
nothing does — the projects with real token pipelines have no brand, and the projects with real
brand derivation stop at prose.

**Produce these eight, in this order.** Each is a constraint on the next, so the order is not
cosmetic — reordering means redoing work.

| # | Section | What it records |
|---|---|---|
| 1 | **Brand foundation** | Positioning, audience, personality, promise, and 3–5 design principles — the arguments every later decision appeals to. One page, not an essay. |
| 2 | **Voice and tone** | Attributes, each bounded by a negation; tone registers keyed to *situation*; a we-say/we-don't word list; a reading-level target. Before pixels, because voice fixes product naming and message anatomy, which are expensive to retrofit. |
| 3 | **Colour** | Anchor hue → ramp built to contrast thresholds → semantic roles with their `on-*` pairs → neutrals split into surface and outline families. |
| 4 | **Typography** | Families per role, the ramp with its usage-per-rung, weight/line-height/tracking, the full fallback stack, and licensing. |
| 5 | **Accessibility contract** | The *construction rule* that makes §3 and §4 accessible by default, plus the sanctioned pairs and the focus treatment. Not an audit — see §4 below. |
| 6 | **Geometry** | Spacing scale, radius, elevation, breakpoints. The dimensional vocabulary components consume. |
| 7 | **Logo and digital identity assets** | Mark, clear space, minimum **pixel** size, misuse, colour variants — and the asset set that actually blocks shipping: favicon 16/32/48, touch icon 180, PWA 512, 1:1 avatar, OG image 1200×630. |
| 8 | **Token architecture and naming** | The chain — palette → alias → semantic → component — and the naming grammar everything above exports into. Components never reference a palette step directly. |

Stop there and draw one real screen. Iconography, illustration, motion, theming, content patterns
and governance are the next pass, not this one.

**Deliberately excluded, for a digital product**: stationery, forms, signage, vehicle livery,
packaging, uniforms, print production specs, Pantone/CMYK, paper stock, deck templates. Roughly
two thirds of a classic manual is output specification that a screen collapses into one
substrate. Two conditional exceptions: if merchandise is planned, a one-colour mark variant
becomes necessary; if anything is ever printed, colour gains CMYK values. Neither is a default.

**One thing no classic brand book has, and this one must: a decision log.** Record why each
choice was made, at the moment it was made. It is the most-cited missing artefact in practitioner
critiques of design systems, it costs nothing at authoring time, and it is what decides whether
the foundation survives its second year.

---

## 2. Why the curated set below is a filter, not a menu

There is a real argument that a curated set *relocates* convergence rather than escaping it, and
you should know it before using this file. It is best put by the author of a brand skill who ran
his own tool on three different brands and got three interchangeable pages:

> Any instruction specific enough to produce a distinctive result is specific enough to become a
> new center. "Be bold" converges to the average of bold. "Break the grid" converges to the
> average of grid-breaking. You cannot instruct your way out of a statistical property of the
> system.

This is not theory. The 2024–25 signature of machine-made design (a violet-indigo primary,
gradients on dark, one sans for everything) has already been displaced, and it was displaced by
the aesthetic the *first generation of anti-slop advice recommended as the cure*: warm cream
grounds with a high-contrast display serif and a terracotta accent. A 1,590-page automated scan
of Show HN submissions now flags that cluster, and a major vendor's own design skill bans it by
name as a default.

**So a blocklist has a half-life of roughly twelve to eighteen months, and this file will go
stale.** Three consequences, and they govern everything below:

1. **Prefer gates that check themselves over lists that rot.** §3 and §5 are HTTP calls whose
   answers update without anyone editing this file. Prefer them.
2. **The list below is a filter on candidates, not a menu to pick from.** Derive from the product
   first (§6); use the list to reject what is overused or unlicensed, not to supply taste.
3. **Divergence is process, not instruction** — see §7.

---

## 3. The typeface gates — check, don't guess

Run both before proposing any family. Both are single requests, and both survive fashion.

**Overuse gate.** `GET https://fonts.google.com/metadata/fonts` returns JSON with a `popularity`
rank per family (1 = most used). A family ranked inside the top ~40 is a default, not a decision:
propose it only with a stated reason, on the page. Verified ranks at time of writing — **Inter 3,
Montserrat 5, Rubik 7, Poppins 8, Work Sans 10, Raleway 10, DM Sans 13, Plus Jakarta Sans 13,
JetBrains Mono 18, Manrope 22, Outfit 23, Instrument Serif 27, Figtree 32, Bricolage Grotesque 33**.
The ranking has ties and it moves; fetch it rather than trusting these numbers.

**Licence gate.** `HEAD https://raw.githubusercontent.com/google/fonts/main/ofl/<slug>/OFL.txt`
returning 200 is definitive proof of SIL OFL 1.1. This is not pedantry: a widely-used anti-slop
skill recommends Satoshi, General Sans, Switzer, Clash Display, Cabinet Grotesk, Sentient, Erode
and Tanker as "free", and they are Fontshare/ITF licence, **not OFL**, with secondary sources
contradicting each other on whether self-hosting is even permitted. One HTTP call catches it.
The artifacts must embed the face, so an unclear licence disqualifies a candidate outright.

**Currently overused, in two distinct ways.** Broadly overused: Inter, Poppins, Montserrat,
DM Sans, Work Sans, Rubik, Raleway, Plus Jakarta Sans, Manrope, Outfit, Figtree. Overused
*specifically in machine-generated work*, which is worse for our purposes: **Space Grotesk,
Geist, Instrument Serif, Bricolage Grotesque, Fraunces, Syne**. None is banned. Each requires a
reason stated on the page.

---

## 4. Candidate families — all SIL OFL, all embeddable

A filter, not a ranking. Prefer **one family with a full weight range** over a display pairing:
harder to get wrong, ages better, one embed rather than two. Where you propose a pairing, justify
it by role, never by mood.

| Family | Character | Reach for it when |
|---|---|---|
| **Schibsted Grotesk** | Neo-grotesque with humanist terminals, built for a news group | The grown-up alternative to Inter — editorial authority without novelty |
| **Public Sans** | Plain, civic, deliberately unexciting | Trust and plainness matter more than personality: regulated, institutional, public |
| **Host Grotesk** | Contemporary grotesk, slightly narrow, very low uptake | Maximum differentiation while staying entirely conventional in shape |
| **Recursive** | Five axes including casual↔linear and a mono axis | One family covering sans, mono and a warmer register; the highest-character option here |
| **Geologica** | Sharpness and cursive axes | Personality tuned parametrically rather than by swapping families |
| **Atkinson Hyperlegible Next** | Letterforms disambiguated by design (I/l/1, 0/O) | Education, health, public sector — its character *comes from* its accessibility engineering, which makes it a defensible choice rather than an arbitrary one |
| **Source Serif 4** | Neutral text serif with an optical-size axis | A body serif that is not Merriweather or Lora |
| **Newsreader** | Screen-first news serif, optical sizes | Content-heavy products, docs, editorial |
| **Literata** | Warm, sturdy, engineered for long reading | Reading and learning products |
| **Faustina** | Warm, slightly calligraphic humanist serif | Friendly institutional register without going Playfair |
| **Young Serif** | Chunky high-personality display slab | Display only, paired with a plain sans |
| **Eczar** | High-contrast, energetic, strong Devanagari | Multilingual products, or intellectual heat |
| **Martian Mono** | Wide, engineered, width and weight axes | Data and code surfaces, instead of the default mono |
| **Sometype Mono** | Softer, more typographic mono | An outlier face for captions and figures |

---

## 5. Colour — construct it so it cannot fail

**Derive the anchor hue from the product's own world** — its materials, instruments, artifacts,
vernacular — never from a colour wheel. This is the only step that produces a non-generic result;
everything below is the craft that stops the derived choice being ruined.

**One anchor hue.** A palette with several equal accents reads as assembled. Accent occupies no
more than 3–5% of any viewport: scarcity is what makes it read as composed.

**Build the ramp in OKLCH**, hue held constant, lightness stepped, chroma peaking mid-ramp and
falling at both ends. **Never `#000`, never `#fff`, and never a zero-chroma grey** — flat greys
are the strongest "generated" signal in colour, and they are trivially detectable. Tint every
neutral toward the anchor.

**Ten to fourteen steps, and the number is derived from a contrast rule rather than chosen.**
Carbon uses ten because black text passes on steps 10–50 and white passes on 60–100, so the flip
point lands cleanly. Spectrum places fourteen steps at target contrast ratios, so the *index* is
a contrast promise.

**Encode accessibility in the construction — this is the single best idea in the field.**
Material 3 spaces its tones so that a **tone distance ≥40 guarantees 3:1** and **≥50 guarantees
4.5:1**. Roles are then assigned by distance: a role at tone 40 with its `on-` pair at tone 100
is 60 apart, so it cannot fail. Nothing is eyeballed, and it survives both a change of anchor hue
and a theme flip — which per-pair audits do not. Adopt the rule, then verify the pairs
numerically as a check, not as the method.

This matters more than it sounds: the WebAIM Million found low-contrast text on **83.9%** of home
pages in 2026, up from 79.1% in 2025 — the most common detected failure by roughly thirty points,
and getting worse. A palette that ships hexes without valid pairs is the defect.

**Assign semantic roles explicitly** — action, danger, success, warning, plus neutrals — each
declaring background, border, text and icon. A palette without roles is decoration, and
verification has nothing to measure. Split neutrals into a **surface** family and an **outline**
family: every good system does, and outlines need a different lightness band from backgrounds.

**Name stably, vary the values.** Light and dark are two values under one name, never two
palettes. Say explicitly which roles deliberately do *not* invert.

Tools worth naming when you need to generate rather than reason: Leonardo (specify target
contrast, get the colour), Huetone and Harmonizer (OKLCH with contrast checking), and Radix
Colors for the twelve-step semantic role model.

**One honest caveat to carry.** There is good evidence that a perceptual colour space makes
contrast targets *easier to hit*, and no controlled evidence that it makes palettes more
attractive or more distinctive. The "designed" quality comes from the constraints — one anchor
hue, tinted neutrals, accent scarcity, explicit roles — not from the colour space. Do not claim
otherwise on the page.

---

## 6. The tells

Marks of design that was assembled rather than chosen. **None is banned.** Each is legitimate
where the product's own evidence leads there, and then you say so on the page. What is not
allowed is arriving at them *by default*.

**Current cluster** (displaced the previous one, and will itself be displaced): warm cream or
beige ground with a high-contrast display serif and a terracotta or orange accent · serif italic
on one accent word in an otherwise-sans headline · shimmer/streaming text as a loading device ·
sparkle iconography for anything AI · undersized thin icons against a heavier system UI.

**Previous cluster, still live**: indigo or violet primary around `#6366f1`/`#7c3aed` ·
purple-to-pink and blue-to-purple gradients as brand furniture · glassmorphism and blurred colour
blobs · permanent dark mode with mid-grey body text · large coloured glows.

**Structural, and the most durable of the three**: a centred hero stack with a pill above the
headline · a grid of identically-rounded rectangles · three feature cards with an icon on top ·
numbered 1-2-3 step sequences where the order carries no information · stat banner rows · emoji
standing in for iconography · a uniform large radius on every surface · untouched component-kit
defaults.

**Counter-evidence, recorded deliberately.** Several of these are precision-poor. Coloured left
borders were widely claimed as a tell on the strength of one person's impression, and others have
used them for years. Gradients, centred heroes and all-caps labels all predate generative tools
and are at least as attributable to popular CSS kits. **Weight these, do not hard-ban them** — a
rule that rejects legitimate work is worse than no rule.

---

## 7. Divergence is process, not instruction

Asking one prompt for three options produces three shades of one idea. The only mechanism in this
field with peer-reviewed support is **parallel independent generation**: separate generations,
each with its own brief, compared afterwards. The same study found that asking a single prompt to
cover all the perspectives produced the *lowest* diversity of the strategies tested, and related
work shows instruction-tuned models homogenise across repeated queries inside one context.

So: **generate the directions independently — one sub-agent per direction where your tool has
them** — and hold them to two rules borrowed from the practitioner who diagnosed this best:

- **Structural incompatibility.** Each direction must be one you could not blend with the others.
  If a header from one would sit happily on a body from another, they are one direction wearing
  two palettes. At least one direction should feel slightly uncomfortable.
- **Kill, don't blend.** Combining the safe half of one with the safe half of another is
  averaging, which is the exact failure being avoided. Choose one and discard the rest. When in
  doubt, kill the safe ones.

**Record what you chose.** Keep a short log — structure, anchor hue band, ground lightness,
type voice — and require the next foundation in this project to differ on at least one named
axis. This is the only mechanism that resists *becoming your own new default*, which is
empirically what happened to the last generation of advice.

---

## 8. Two tests before you present anything

1. **Could you give the reason out loud to a client?** Every value should have one. "It felt
   modern" is not one.
2. **Would this still make sense if the trend that produced it disappeared?** A direction that
   only works right now is one you assembled, not one you chose.

Present two or three complete, internally coherent directions — each named, each with its
rationale tied to something the product actually is, each drawn as **the same real screen from
this feature** so the comparison means something. Then publish, hand over the link, and ask.

---

## 9. What is not proven

No anti-slop skill or ruleset published anywhere has a before/after evaluation. Mechanisms are
plausible and evidence is absent, with one exception — the parallel-divergence result in §7. The
honest posture, and the one this file takes: **checkable rules are enforced, everything else is
stated as intent, and nobody has demonstrated that the uncheckable half works.** Say so if the
user asks how confident the guidance is.
