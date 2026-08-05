# Proposals

Design specifications written before implementation. Each proposal states the problem, the design, the scope, both invocation paths, an author smoke test, and a Definition of Done — so the approach can be reviewed and agreed before any code is written.

## Numbering

Sequential, four digits, never reused. `0001` and `0002` are claimed by the open design RFCs in PRs #12 and #13.

## Index

| # | Title | Status |
|---|---|---|
| 0003 | [Design quality without Figma](0003-design-quality-without-figma.md) | Draft — revision 2 |
| 0004 | [Context budget, with evidence](0004-context-budget-with-evidence.md) | Draft — revision 2 |
| 0005 | [Maintainability](0005-maintainability.md) | Draft — revision 2 |

## Sequencing

0003 → 0004 → 0005.

- **0003 lands first.** It *adds* tokens on the sub-agent fan-out path, so 0004's measured baseline must already include it.
- **0004 blocks nothing.** It touches no non-test Go, so it does not conflict with 0005's relocation of `executeLoopAction`.
- **0005 lands last** and owns `.github/workflows/ci.yml`, which 0004's eval harness wires into.

## Revision history

All three are at revision 2. Revision 1 of each was adversarially reviewed against the codebase; that review found design-level blockers in every one — including two proof mechanisms that were empirically impossible and several claims about Belmont internals that were wrong. Each proposal carries a "Changes from v1" table recording what moved and why.

The lesson is worth keeping: a specification that reads as coherent is not the same as one that is correct about the system it describes. Verify claims against the code before building on them.
