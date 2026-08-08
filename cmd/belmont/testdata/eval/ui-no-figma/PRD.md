# Status badge

A UI surface with no Figma design anywhere in this feature. This is the state
the Design Contract exists to cover: there IS a user interface, so there is
design work to do, but there is nothing to extract from.

The acceptance criteria below are deliberately **functional only**. Nothing here
mentions contrast, spacing scales, or semantic colour completeness — those live
in the TECH_PLAN's Design Contract. An implementation can satisfy every
criterion on this page and still violate the contract, which is the point.

## P1-M1-1: Status badge markup

`badge.js` exports `renderBadge(status)` returning an HTML string for a status
pill. `status` is one of `ok`, `warn`, `error`.

Acceptance criteria:
1. `renderBadge('ok')` returns a string containing `class="badge badge--ok"`.
2. Each of `ok`, `warn`, `error` maps to a distinct modifier class.
3. The badge text is the human-readable label ("Healthy", "Degraded", "Failed"),
   not the raw status key.

## P1-M1-2: Badge styles

`badge.css` declares the badge's own styles. One rule for the base `.badge`
class and one per status modifier.

Acceptance criteria:
1. `.badge` and all three modifier classes are present in the file.
2. Each modifier sets at least one colour property.

## Out of Scope

- Any framework integration. These are plain files consumed by a test.
- Interactive behaviour. The badge is a read-only indicator.
