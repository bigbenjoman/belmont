# UX Design: Status badge

## Design Contract
**Mode**: derived — UI, no Figma
**Source**: none — established here
**Authorities**: baseline only (no design skills available in this session)
**Approval**: approved 2026-08-08

### Token Contract
Spacing — 8pt grid: 4, 8, 12, 16, 24, 32. Internal ≤ external on every component.
Typography — ratio 1.25, sizes 13px / 16px / 20px / 25px, line-height 1.4 body. Max 4 sizes.
  One family carries display and body; no outlier slot. Headings roman.
Colour — 60/30/10. Neutrals plus one accent hue. Never pure #000/#FFF.
  Each semantic (ok/warn/error) declares bg, border, text.
Radius — committed value 8px. Nested children strictly smaller than parent.
Elevation — level 0 (flat) and level 1 (`0 1px 2px rgba(0,0,0,0.12)`) only.

### Accessibility Floor
Targets ≥ 44×44px (SC 2.5.5, AAA — adopted as Belmont's floor) · contrast ≥ 4.5:1 text / 3:1 large
(SC 1.4.3, AA) · visible focus on every interactive element (SC 2.4.7, AA) · every input has a visible
label, never placeholder-only · reflow at 320px with no horizontal scroll (SC 1.4.10, AA) ·
`prefers-reduced-motion` respected (SC 2.3.3, AAA) · no meaning by colour alone.

| Pair (fg on bg) | Computed | Floor |
|-----------------|----------|-------|
| `#1f2421` on `#fbfbfa` | 15.22:1 | 4.5:1 |
| `#6b7280` on `#fbfbfa` | 4.67:1 | 4.5:1 |
| `#2f6f5e` on `#fbfbfa` | 5.7:1 | 4.5:1 |

### UX Strategy
User: an operator scanning a list of jobs. Arrives wanting to spot failures fast.
Hero element: the badge's colour band. Primary action: none — this is a read-only
indicator. Biggest UX risk: conveying status by colour alone, which fails anyone
who cannot distinguish the hues.

### State Inventory
`badge` is a non-interactive display component. States: default (per status
variant: ok, warn, error). No hover, focus, active, disabled, loading, empty or
error states — the component is not interactive and always renders with a status.

### Microcopy Rules
The badge label is the human-readable status word ("Healthy", "Degraded",
"Failed"), never the raw key. No abbreviations. No jokes.

### Motion Contract
**Applies**: N/A — no motion in this feature
