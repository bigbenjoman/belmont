# Proposals

Design specifications written before implementation. Each proposal states the problem, the design, the scope, both invocation paths, an author smoke test, and a Definition of Done — so the approach can be reviewed and agreed before any code is written.

Proposals that argue for a change in approach carry a rationale section. Bug fixes do not — a defect justifies itself, and framing one as a design argument only obscures it.

## Numbering

Sequential, four digits, never reused. `0001` and `0002` are claimed by the design RFCs in PRs #12 and #13 — both closed unmerged, but the numbers stay spent.

## Index

| # | Title | Status |
|---|---|---|
| 0003 | [Design quality without Figma](0003-design-quality-without-figma.md) | Draft — revision 5. Design moved from `implement` Phase 2 into `tech-plan`. Awaits one adversarial pass |
| — | [0003 restructure brief](0003-DESIGN-RESTRUCTURE.md) | Background for rev 5. **Superseded in part** — see its header for the facts rev 5 corrected |
| 0004 | [Context budget, with evidence](0004-context-budget-with-evidence.md) | Draft — revision 5 |
| 0005 | [Split `main.go`, add CI](0005-maintainability.md) | Draft — rewritten for the reviewer. No revision history in the document; use `git log` for it |
| 0006 | [Plugin generator correctness](0006-plugin-generator-correctness.md) | **Done** — PR #21 merged 2026-08-07, shipped in v0.10.15 |

## Gate — cleared

PR #21 merged on 2026-08-07 (`ae26e29`, shipped as v0.10.15). It fixed `scripts/generate-plugin.sh`, which had been publishing four of six plugin agents as 0-byte files since 2026-06-04. Verified on `main`: all six are now source + 3 lines. Nothing in this series is blocked.

## Sequencing

**0005 → 0004 → 0003.** One per session; 0005 in particular is large.

- **0005 first.** It owns `.github/workflows/ci.yml`, which 0004's eval harness wires into, so building it first means 0004 extends a file that exists rather than creating one 0005 will rewrite. Its `main.go` split also touches every later diff's line numbers, so going first spares the others a rebase.

  0005's two halves are independently landable — see its §10. The CI half is what 0004 depends on and what the shipped plugin defect argues for; the `main.go` split is a judgement call the reviewer can refuse separately. If the split is rejected or deferred, the sequencing above still holds on the CI half alone.
- **0004 second.** It touches no non-test Go, so it cannot conflict with 0005's relocation of `executeLoopAction`.
- **0003 last**, and only after its rev 5 restructure — moving UX/UI design out of `implement` Phase 2 and into `tech-plan`.

### The 0003 ↔ 0004 baseline, and why the order changed

Through 0004 rev 4 this order was **reversed**: 0003 landed first so that 0004's measured token baseline already included Mode B, and both specs said so — 0003 §11 ("landing 0003 first") and 0004 §Sequencing ("baseline must include Mode B"). The author set the current order on 2026-08-06, which inverts that dependency. The specs have been amended to match; the cost is explicit:

- 0004's reduction is measured **pre-Mode-B**, so it is an **upper bound**, and must be reported in those terms.
- A **re-baseline after 0003** is owed. It sits in 0004's DoD as a follow-up obligation and in 0003's as an inherited one — it cannot be a merge gate for 0004, because the code it measures will not exist yet.

**This dependency may dissolve entirely.** 0003's rev 5 restructure derives the design contract once per feature in `tech-plan`, instead of once per milestone in `implement` Phase 2. If it lands that way, Mode B no longer adds per-task cost to the sub-agent fan-out path that 0004 defers — and the re-baseline shows no delta. That is a reason the deferral is cheap, not a reason to skip the measurement: rev 5 is not written yet, so its token profile is unknown. Do not treat the dissolution as settled.

### Shared files

0003 and 0004 both edit `skills/belmont/_src/verify.md`. Revisions up to and including 0004 rev 4 claimed these edits "will conflict" because they sit four lines apart, inside default diff context. **That claim was tested on the real file and is false** — the rebase is clean at a three-line gap, and stays clean when the edits are widened. Default diff context governs `git am`/`git apply` of a patch file; rebase and merge use the three-way content merge, which ignores it. A conflict needs both sides to touch the same line. The claim has been removed from both specs.

Genuine overlaps: 0003 and 0004 both edit `_src/next.md`; 0003 and 0005 both edit `_src/references/models-yaml-format.md` and append to `model-tier-economics.md`; all three append to `KNOWLEDGE.md`'s cross-cutting table — expect a trivial conflict there and keep every row. Resolve `plugin/` conflicts by regeneration, never by hand — `generate-plugin.sh:35` does `rm -rf`.

## Revision history

0003 is at revision 4 and 0004 at revision 5, after three adversarial reviews against the codebase. Round 1 found design-level blockers in all three. Round 2 found that several v2 fixes had not landed — including a replacement proof mechanism that was broken in both directions — and surfaced 0006's shipping defect as a side effect. Round 3 (`redteam-round3.md`) produced rev 4, finding four blockers in 0003 alone. 0004's rev 5 is an ordering amendment only, not a review outcome. Those two proposals carry per-revision change tables.

**0005 no longer does.** It went through the same four rounds, but the revision tables and the "an earlier draft claimed X" asides were removed on 2026-08-07: they made the reader work through the document's history before reaching what it proposed. The history is in `git log -- docs/proposals/0005-maintainability.md`, which is where it belongs. Prefer that treatment for the others when they are next revised — a proposal is read by someone deciding whether to accept it, not by someone auditing how it was written.

Three lessons are worth keeping.

A specification that reads as coherent is not the same as one that is correct about the system it describes — verify claims against the code before building on them. A review agent asserted that `cmd/belmont/tools.go` already existed; it never has, on any branch, and the claim was accepted unchecked and shipped into two revisions.

A verification mechanism is itself a claim. The purity proof for 0005's `main.go` split was specified twice without being run, and was impossible both times — first `cmp` on `-trimpath` binaries, then a `sed`/`sort` recipe that failed in both directions. The replacement is tested against a legitimate-move control and a deliberately-broken one before appearing in a Definition of Done. Note the control run itself has not been reproduced in-repo: `scripts/declsum/` does not exist yet, so re-verify before relying on it.

And fixes introduce defects at the same rate as first drafts. Several round-3 findings were created by round-2 repairs. Re-verify what the previous round changed, not only what it left alone.
