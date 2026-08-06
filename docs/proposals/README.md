# Proposals

Design specifications written before implementation. Each proposal states the problem, the design, the scope, both invocation paths, an author smoke test, and a Definition of Done — so the approach can be reviewed and agreed before any code is written.

## Numbering

Sequential, four digits, never reused. `0001` and `0002` are claimed by the open design RFCs in PRs #12 and #13.

## Index

| # | Title | Status |
|---|---|---|
| 0003 | [Design quality without Figma](0003-design-quality-without-figma.md) | Draft — revision 3 |
| 0004 | [Context budget, with evidence](0004-context-budget-with-evidence.md) | Draft — revision 3 |
| 0005 | [Maintainability](0005-maintainability.md) | Draft — revision 3 |
| 0006 | [Plugin generator correctness](0006-plugin-generator-correctness.md) | Draft — bug fix, ready |

## Sequencing

**0006 → 0003 → 0004 → 0005.**

- **0006 lands first.** It fixes a live shipping defect — four of six plugin agents are published as empty files — and every other proposal depends on a working generator. It also stands on its own merits, independent of the rest.
- **0003 next.** It *adds* tokens on the sub-agent fan-out path, so 0004's measured baseline must already include it.
- **0004 blocks nothing.** It touches no non-test Go, so it does not conflict with 0005's relocation of `executeLoopAction`.
- **0005 lands last** and owns `.github/workflows/ci.yml`, which 0004's eval harness wires into.

### Shared files

0003 and 0004 both edit `skills/belmont/_src/verify.md` four lines apart. 0003 and 0005 both edit `_src/references/models-yaml-format.md` and append to `model-tier-economics.md`. All four append to `KNOWLEDGE.md`'s cross-cutting table. Resolve `plugin/` conflicts by regeneration, never by hand — `generate-plugin.sh:35` does `rm -rf`.

## Revision history

0003–0005 are at revision 3, having been adversarially reviewed against the codebase twice. Round 1 found design-level blockers in all three. Round 2 found that several v2 fixes had not landed — including a replacement proof mechanism that was broken in both directions — and surfaced 0006's shipping defect as a side effect. Each proposal carries per-revision change tables.

Two lessons are worth keeping. A specification that reads as coherent is not the same as one that is correct about the system it describes — verify claims against the code before building on them. And a verification mechanism is itself a claim: 0005's purity proof was specified twice without being run, and failed both times. It is now tested against a legitimate-move control and a deliberately-broken control before appearing in a Definition of Done.
