# Brand foundation: the curated set

Read this **only** on the empty-ladder branch — no contract, no Storybook, no
config, no code, no Figma — where `/belmont:ux-design` runs its *Brand
foundation* stage. A feature with any rung populated has nothing to take from
here, and reuse always beats invention.

## Why a curated set exists at all

An unconstrained choice lands on the same handful of answers every time, and
those answers now read as *generated* rather than *designed*. A named starting
set makes the first move deliberate. It is a **menu, not a cage**: departing
from it is allowed, and the only requirement is that you say on the page what
in the product pushed you there. A departure with a reason is design; a
departure without one is the thing this file exists to prevent.

The set is deliberately small. It is not a survey of good typefaces, and it is
not a ranking — every entry is a defensible default for a different kind of
product, and none of them is the "best" one.

## Typefaces

**Prefer one family with a full weight range over a pairing.** It is harder to
get wrong, it ages better, it costs one embed rather than two, and it is what
most well-designed products actually do. Reach for a second family only when a
role genuinely needs it — a display face for a marketing surface, a monospace
for data — and justify it by that role rather than by mood.

Every candidate below is open-licensed (SIL OFL or similar) and embeddable as a
`@font-face` with a `data:` URI, which is a hard requirement: a face the
reviewer cannot see is not a candidate.

| Family | Character | Reach for it when |
|---|---|---|
| **Source Sans 3** | Neutral humanist sans, wide weight range, unusually legible at small sizes | The product is dense, formal or read under pressure — admin, finance, health, anything with tables |
| **Public Sans** | Plain, civic, slightly stiff; drawn for government use | Trust and plainness matter more than personality — regulated, institutional, public-facing |
| **Work Sans** | Warm geometric sans with a soft lower case | Consumer products that should feel approachable without being childish |
| **Libre Franklin** | Grotesque with real editorial authority in its heavy weights | Content-led products, where headlines carry the page |
| **Newsreader** | Serif with a proper reading texture at body sizes | Long-form reading is the actual product — never for UI chrome |
| **IBM Plex Sans** | Technical, engineered, ships with a matched mono and serif | Developer and data products, where a matched mono earns its place |

**Not on the list, and why.** Inter, Geist, Space Grotesk, Poppins, Montserrat
and DM Sans are all fine typefaces and all heavily overused as the default
answer. Choosing one is not wrong — choosing one *because it is the safe
option* is. If you land on one, name what in the product led you there.

## Palette starting points

Each entry is a **starting point for a primary hue**, not a palette. Build the
ramp yourself: the primary at the weight that meets the floor on white, a
light tint for surfaces, a dark step for text on tint, plus a neutral ramp and
the three semantic hues.

| Direction | Primary hue family | Reads as |
|---|---|---|
| **Ink** | Near-black or very dark neutral as the brand colour, with one restrained accent | Serious, editorial, confident; the accent does all the work |
| **Deep blue** | Blue at high depth, away from the default-link blues | Institutional trust without corporate blandness |
| **Forest** | Desaturated green, dark enough for text use | Growth, care, patience; strong for education and health |
| **Clay** | Warm earthy red-browns and terracottas | Human, tactile, non-technological |
| **Slate + one signal** | An almost-monochrome neutral system with a single saturated signal colour | The interface should recede and the data or content should carry it |

**Assign semantics explicitly.** Every direction must state which value is
action, which is danger, which is success, and what the neutral ramp is. A
palette without semantic roles is decoration, and downstream verification has
nothing to measure against.

**Derive the ramp against the floor, not after it.** Fix the Accessibility
Floor first, then choose hues that can meet it. A primary that cannot reach
4.5:1 on white is not a decision available to you.

## The tells

These are the marks of design that was assembled rather than chosen. None is
banned outright — each is legitimate when the product's own evidence leads
there, and then you say so on the page. What is not allowed is arriving at
them **by default**:

- An indigo or violet primary around `#6366f1` / `#7c3aed`
- Purple-to-pink or blue-to-purple gradients used as brand furniture
- Glassmorphism, blurred colour blobs, a dark hero with one neon accent
- Inter, or a near-clone, chosen for everything because it is the safe answer
- A single large radius applied uniformly to every surface
- Emoji standing in for iconography
- Five weights of the same family used to fake a hierarchy that the scale
  should be carrying

## Two tests before you present anything

1. **Could you give the reason out loud to a client?** Every value should have
   one. "It felt modern" is not one.
2. **Would this still make sense if the trend that produced it disappeared?**
   A direction that only works right now is one you assembled, not one you
   chose.

Present two or three complete, internally coherent directions — each named,
each with its rationale tied to something the product actually is, each drawn
as **the same real screen from this feature** so the comparison means
something. Then publish, hand over the link, and ask.
