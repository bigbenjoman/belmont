# Proposals

Design specifications written before implementation. Each proposal states the problem, the design, the scope, both invocation paths, an author smoke test, and a Definition of Done — so the approach can be reviewed and agreed before any code is written.

Proposals that argue for a change in approach carry a rationale section. Bug fixes do not — a defect justifies itself, and framing one as a design argument only obscures it.

## Numbering

Sequential, four digits, never reused. `0001` and `0002` are claimed by the design RFCs in PRs #12 and #13 — both closed unmerged, but the numbers stay spent.

## Index

| # | Title | Status |
|---|---|---|
| 0003 | [Design quality without Figma](0003-design-quality-without-figma.md) | Draft — revision 4. **Do not implement as written**; needs a rev 5 restructure first |
| 0004 | [Context budget, with evidence](0004-context-budget-with-evidence.md) | Draft — revision 5 |
| 0005 | [Maintainability](0005-maintainability.md) | Draft — revision 4 |
| 0006 | [Plugin generator correctness](0006-plugin-generator-correctness.md) | Standalone bug fix — unrelated to the series. Shipped as PR #21 |

## Gate

**PR #21 must merge before any implementation in this series.** It fixes `scripts/generate-plugin.sh`, which currently publishes four of six plugin agents as 0-byte files. Branch off `main` before it merges and any regeneration drops those empty files into an unrelated PR, where a 12k-line diff will hide them.

```bash
gh pr view 21 --repo blake-simpson/belmont --json state
```

## Sequencing

**0005 → 0004 → 0003.** One per session; 0005 in particular is large.

- **0005 first.** It owns `.github/workflows/ci.yml`, which 0004's eval harness wires into, so building it first means 0004 extends a file that exists rather than creating one 0005 will rewrite. Its `main.go` split also touches every later diff's line numbers, so going first spares the others a rebase.
- **0004 second.** It touches no non-test Go, so it cannot conflict with 0005's relocation of `executeLoopAction`.
- **0003 last**, and only after its rev 5 restructure — moving UX/UI design out of `implement` Phase 2 and into `tech-plan`.

### The 0003 ↔ 0004 baseline, and why the order changed

Through 0004 rev 4 this order was **reversed**: 0003 landed first so that 0004's measured token baseline already included Mode B, and both specs said so — 0003 §11 ("landing 0003 first") and 0004 §Sequencing ("baseline must include Mode B"). The author set the current order on 2026-08-06, which inverts that dependency. The specs have been amended to match; the cost is explicit:

- 0004's reduction is measured **pre-Mode-B**, so it is an **upper bound**, and must be reported in those terms.
- A **re-baseline after 0003** is owed. It sits in 0004's DoD as a follow-up obligation and in 0003's as an inherited one — it cannot be a merge gate for 0004, because the code it measures will not exist yet.

**This dependency may dissolve entirely.** 0003's rev 5 restructure derives the design contract once per feature in `tech-plan`, instead of once per milestone in `implement` Phase 2. If it lands that way, Mode B no longer adds per-task cost to the sub-agent fan-out path that 0004 defers — and the re-baseline shows no delta. That is a reason the deferral is cheap, not a reason to skip the measurement: rev 5 is not written yet, so its token profile is unknown. Do not treat the dissolution as settled.

### Shared files

0003 and 0004 both edit `skills/belmont/_src/verify.md` four lines apart — under the current order 0004 lands first, so 0003 rebases onto it. 0003 and 0005 both edit `_src/references/models-yaml-format.md` and append to `model-tier-economics.md`. 0003, 0004 and 0005 all append to `KNOWLEDGE.md`'s cross-cutting table; expect a trivial conflict and keep every row. Resolve `plugin/` conflicts by regeneration, never by hand — `generate-plugin.sh:35` does `rm -rf`.

## Revision history

0003 and 0005 are at revision 4 and 0004 at revision 5, after three adversarial reviews against the codebase. Round 1 found design-level blockers in all three. Round 2 found that several v2 fixes had not landed — including a replacement proof mechanism that was broken in both directions — and surfaced 0006's shipping defect as a side effect. Round 3 (`redteam-round3.md`) produced rev 4, finding four blockers in 0003 alone. 0004's rev 5 is an ordering amendment only, not a review outcome. Each proposal carries per-revision change tables.

Three lessons are worth keeping.

A specification that reads as coherent is not the same as one that is correct about the system it describes — verify claims against the code before building on them. A review agent asserted that `cmd/belmont/tools.go` already existed; it never has, on any branch, and the claim was accepted unchecked and shipped into two revisions.

A verification mechanism is itself a claim. 0005's purity proof was specified twice without being run, and failed both times. It is now tested against a legitimate-move control and a deliberately-broken control before appearing in a Definition of Done.

And fixes introduce defects at the same rate as first drafts. Several round-3 findings were created by round-2 repairs. Re-verify what the previous round changed, not only what it left alone.
