# 0003 rev 5 — adversarial review state

**Written 2026-08-06, mid-review, when the session had to end.** Read this before touching
`0003-design-quality-without-figma.md`.

> **Superseded — rev 6 shipped, then rev 7 after its own adversarial pass.** All 53 confirmed defects are addressed, plus three author
> decisions (contract only for UI-without-Figma; one conversation per feature; prose-enforced rule with the
> mechanical guard deferred) and a new Storybook derivation rung. This file remains the evidence record for
> *why* rev 6 says what it says. Rev 6 has **not** yet had its own adversarial pass.

**Why rev 5 was stopped.** Its central durability claim was not merely wrong, it was inverted, and the
safety net it relied on did not exist.

---

## Where this got to

| Stage | State |
|---|---|
| Rev 5 written and pushed | Done — `d1236f1` |
| Adversarial pass, round 1 (6 lenses, 72 findings) | Complete — `tasks/w8jhj34c3.output` |
| Round 1 refutation | 9 of 72 verified. 6 survived, 3 refuted |
| Round 2 refutation of the remaining 63 | **Complete — 63/63 judged.** 47 survived, 16 refuted |
| **Total** | **53 confirmed defects** of 72 raised; **19 refuted** |
| Rev 6 | Not started — blocked on one author decision (below) |

**Confirmed list: [`rev5-CONFIRMED-DEFECTS.md`](rev5-CONFIRMED-DEFECTS.md)** — every entry survived an
independent skeptic instructed to refute it, and carries a specific fix. That is the document rev 6 gets
written against. Raw data in `rev5-findings-round1.json`, `rev5-findings-pending.json`,
`rev5-verdicts-round2.json`.

The 7 blocker entries are **4 distinct defects** — the durability-direction bug was found by three lenses
independently and the inert smoke steps by two, and the dedup key (location + category) did not collapse
them because each lens described the location differently. Treat B1/B3/B5 as one, and B4/B7 as one.

Data preserved in-repo, all of it — nothing depends on a session or a scratch dir:

- `rev5-CONFIRMED-DEFECTS.md` — the 53 confirmed defects with fixes. **Start here.**
- `rev5-findings-round1.json` — round 1 in full: 72 findings, verdicts, per-lens coverage notes
- `rev5-findings-pending.json` — the 63 round-2 findings, stable `idx`
- `rev5-verdicts-round2.json` — all 63 verdicts

---

## The blocker that kills rev 5

Found independently by three lenses. One agent wrote a Go test calling `syncFeatureStateAfterMerge`
directly; another rebuilt a real git worktree at 2.50.1 and replayed the call sequence. This is measured,
not argued.

**`syncFeatureStateAfterMerge` (`main.go:10160-10173`) copies worktree → master — the opposite direction
from `copyBelmontStateToWorktree`.** After a successful merge it runs `os.RemoveAll(dstFeature)` on
*master* (`:10169`) and `copyDir` from the *worktree* (`:10170`), as a plain filesystem copy outside git.
`commitBelmontState` (`:10211`) then commits it to main as `belmont: update state files`. Called at
`:9157` (single-feature) and `:6297` (multi-feature).

Measured end to end:

```
master TECH_PLAN.md after merge:      APPROVED CONTRACT (human-approved)
after syncFeatureStateAfterMerge:     WORKTREE-EDITED CONTRACT (never approved)
committed to main:                    283365b belmont: update state files
```

So a worktree-authored edit to `{base}/TECH_PLAN.md` does **not** die. It silently overwrites the
human-approved contract on main. Rev 5 §4.1 asserts the opposite, and §10 offers `--assume-unchanged` as
a backstop that "fails loudly". The flag genuinely stops the *branch* from carrying the change — verified,
even explicit `git add` stages nothing — but the orchestrator's copy is not git-aware, so it ships anyway.

**The failure mode is worse than the one rev 5 documents: you do not lose the edit, you lose the
approval.**

Rev 5's own §4.1 table cites `:10169` and records its consequence only as "must patch both sites" — the
line was seen and its direction missed.

### What rev 6 must do about it

State the real invariant: content under `{base}/` is **bidirectional**. Master seeds the worktree at
creation and resume (`:10010-10011`); the worktree overwrites master at merge (`:10169-10170`). Then pick
one:

- **(a)** Accept that "no auto phase writes the contract" is **prose-enforced and unguarded**, and say so
  as plainly as R4 already does about `runScopeGuard` being exempt for `actionReplan`. Delete the
  `--assume-unchanged` mitigation from §10 — it protects git history, not the file.
- **(b)** Add a mechanical guard: snapshot the `## Design Contract` before the wave and restore it in
  `syncFeatureStateAfterMerge`. **This is a Go change**, which contradicts §5's "No Go changes" — so
  choosing (b) means re-scoping the PR, not patching a sentence.

Also unhandled: `recoverMerge` (`main.go:11156-11196`) never calls `syncFeatureStateAfterMerge` at all.

---

## Other confirmed blockers (round 1, all survived refutation)

1. **Rev 5 repeats rev 4's exact defect.** DoD `:326` ("no auto-mode phase writes that path") and `:328`
   ("contract survives headless replan") are mutually unsatisfiable, because `actionReplan` *is* an auto
   phase (`main.go:7188-7189`) and R4 requires it to write. Same shape as rev 4's `:295`/`:296`, which
   rev 5's own §12 mocks. **Fix:** restate the hard rule as "only the `tech-plan` skill may write
   `{base}/TECH_PLAN.md`; when it runs headlessly as `actionReplan`, R4 constrains what it may change."

2. **Every Mode A milestone would fail verification.** Branch 1 (`:181`) runs comparison and explicitly
   does *not* perform contract checks; the fourth enforcement rule (`:187`) then sees a contract with no
   checks performed and forces FAIL/INCOMPLETE. Figma projects fail unconditionally. Also catches Mode B
   features carrying any non-Figma reference, since verification-agent Step 2.0 counts linked screenshots
   as design references. **Fix (preferred):** branch 1 becomes "references exist → comparison flow **plus**
   contract checks" — a Figma-derived contract still needs its a11y floor checked. Add a verify-verdict
   assertion to smoke Step 7.

3. **Smoke Step 2 tests nothing.** `--from M1 --to M1` gives one in-range milestone; M1 carries no
   `(depends:)`, so `hasExplicitDeps` is false (`main.go:5440-5453`), `runAuto` takes `runLoop`, master
   tree, no worktree, no `copyBelmontStateToWorktree`, no `[r]` prompt (`handleStaleWorktree` is
   unreachable from `runLoop`). The step that exists to prove the load-bearing claim never exercises it.
   **Fix:** ensure `### M2: … (depends: M1)` exists, run `--from M1 --to M2`, Ctrl-C during Wave 1, rerun
   identically and answer `r`. State the expected `Belmont Auto (parallel)` banner so a serial fallback
   reads as failure. Warn that the interrupted run leaves `.belmont/auto.json` untracked, which
   `requireCleanWorkingTree` rejects on the rerun.

---

## Round 2 — 47 more confirmed

Full detail with per-finding fixes in `rev5-CONFIRMED-DEFECTS.md`. One further **blocker**, 16 majors,
30 minors. Recurring themes:

- **Citations go stale by construction.** 0005 lands first and splits `main.go` from 12,613 lines; every
  one of rev 5's ~18 `main.go:NNNN` citations will point somewhere else. Rev 6 should cite **function
  names** with line numbers as a secondary hint, not the reverse.
- **WCAG levels are unstated.** SC 2.5.5 (44×44) and SC 2.3.3 (animation) are **Level AAA**; the spec
  presents them as a baseline floor without naming a conformance level anywhere.
- **The Codex path may be broken.** `codex-plan-apply` applies a second validation rejecting source-code
  paths, and a self-contained `.html` with embedded CSS is source code by any ordinary reading. R5
  addresses only the first gate.
- **No migration story.** Grepping the proposal for `migrat|backfill|existing project|pre-rev|legacy`
  returns zero hits. Every already-planned feature has a `TECH_PLAN.md` with no `## Design Contract`.
- **Scope table is incomplete.** `_src/implement.md`, `implement-milestone-template.md`,
  `prompts/belmont/post-verify-triage.md` and `_src/references/tech-plan-master-format.md` all need edits
  and are unlisted. The header's "9 tracked source files" is 10 by the table's own rows.
- **`/belmont:debug-manual` contradicts the hard rule** — it is explicitly sanctioned to edit TECH_PLAN
  prose in place, with its own knowledge entry and an AGENTS.md callout.

---

## Next steps, in order

1. ~~Finish the refutation.~~ **Done 2026-08-07** — 63/63 judged, 47 survived, 16 refuted. Refutation
   earned its keep: 19 of 72 findings (26%) did not survive, and acting on those would have produced
   19 wrong edits.
2. **Decide (a) or (b)** on the durability blocker. This is an author decision, not an implementer's: (b)
   turns a prose-only PR into one with a Go change and re-scopes it.
3. **Write rev 6** once, against the confirmed set. Do not patch rev 5 incrementally — the durability
   claim is load-bearing for §4.1, §4.5, §10 and the smoke test, and partial edits will leave the same
   class of contradiction this review exists to catch.
4. **Re-run the adversarial pass on rev 6**, with no cap this time.

## Standing rule this review earned

**Verify a mechanism's direction, not just its existence.** The blocker was in plain sight — rev 5 cited
the exact function and line, and described what it does incompletely. Reading that a function copies
between two paths is not the same as establishing *which way*. The same applies to
`writeWorktreeGitExcludes`, which writes to `$GIT_DIR/info/exclude` while git reads
`$GIT_COMMON_DIR/info/exclude`, so it silently does nothing — and rev 5 §4.5 depends on the broken
behaviour for its justification.

## Unrelated bugs on `main` found along the way

Both deliberately out of scope for 0003; they want their own issues.

- `writeWorktreeGitExcludes` (`main.go:10057-10080`) writes to the per-worktree gitdir's `info/exclude`,
  which git never reads. New files under `.belmont/` in a worktree **are** staged by `git add -A`.
- `AGENTS.md:207` documents a `runAutoParallel` master-tree shortcut deleted 2026-04-22; every wave now
  routes through `runWaveParallel`.
- `main.go:10000`'s comment claims `STEERING.log.md` is preserved across the resume wipe. Only
  `STEERING.md` is — `STEERING.log.md` appears nowhere else in the file.
