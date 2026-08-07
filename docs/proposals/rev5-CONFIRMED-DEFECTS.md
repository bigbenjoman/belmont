# 0003 rev 5 — confirmed defect list

Every entry below survived an independent skeptic instructed to refute it. 72 findings were
raised across six lenses; **19 were refuted and are excluded**. What remains is 53.

## Blockers

### B1 — false-behavioural-claim @ §4.1 lines 64 + 66, and §10 risk row line 369
Both are false. The spec's own §4.1 table cites `syncFeatureStateAfterMerge` (`main.go:10169`) but records its consequence only as "any mechanical durability change must patch both sites" — it never notices that this second site copies in the OPPOSITE direction (worktree → master). After a successful merge, `syncFeatureStateAfterMerge` wipes master's feature dir and replaces it with the worktree's on-disk copy; `commitBelmontState` then commits that to main under the message "belmont: update state files". A worktree-authored edit to `{base}/TECH_PLAN.md` therefore does NOT die — it silently ov

**Fix.** Rewrite §4.1's durability claim. The correct statement is: worktree-local edits to `{base}/TECH_PLAN.md` are destroyed only by a *resume* wipe (`main.go:9042`, unconditional in the single-feature milestone path); on the normal non-resumed path they survive to merge and are copied back over master by `syncFeatureStateAfterMerge` (`main.go:10169-10170`) and committed by `commitBelmontState` (`main.go:10230-10234`). Correspondingly, replace the §10 mitigation — `--assume-unchanged` is not a backsto

### B2 — self-contradiction @ docs/proposals/0003-design-quality-without-figma.md:78 (§4.1 hard rule) vs :93 (R4), :326 (DoD) and :307-309 (smoke Step 6)
`actionReplan` **is** an auto-mode phase and it runs `/belmont:tech-plan` (verified: `cmd/belmont/main.go:7188-7189` `case actionReplan: return fmt.Sprintf("/belmont:tech-plan --feature %s", feature)`). R4 (:93) then *requires* that phase to derive missing sections and rewrite the `**Approval**` line, and smoke Step 6 (:308) *expects* to observe that write. So the hard rule at :78 and DoD :326 are unsatisfiable alongside R4 and Step 6; the proposal itself concedes at :82 that "'Authored once, before auto starts' is **not** guaranteed". DoD :326 and DoD :328 ("Contract survives … headless repla

**Fix.** Restate the hard rule as "only the `tech-plan` skill may write `{base}/TECH_PLAN.md`; `implement`, `verify`, `next` and `debug` only read it. When `tech-plan` runs headlessly as `actionReplan`, R4 constrains what it may change." Amend DoD :326 to "no phase other than `tech-plan` writes that path; `actionReplan`'s tech-plan write is R4-constrained" and make smoke Step 3's assertion explicitly about implement/verify/next phases.

### B3 — false-premise @ docs/proposals/0003-design-quality-without-figma.md:64 and :74 (§4.1) and :369 (§10 risk row)
Worktree-authored `{base}/TECH_PLAN.md` content DOES survive — it overwrites master's approved copy on every successful merge. `syncFeatureStateAfterMerge` copies worktree → master wholesale (`cmd/belmont/main.go:10169-10170`: `os.RemoveAll(dstFeature)` then `copyDir(srcFeature, dstFeature)` where `srcFeature` is the **worktree** path, :10161). It is called after every successful merge in both worktree paths (:9157 and :6297). The `.belmont/` assume-unchanged flag is precisely why this function exists — main.go:6294-6296 comments "`.belmont/` was excluded from the merge (assume-unchanged), so 

**Fix.** Rewrite :64 to: "Master-authored content is re-copied into the worktree on every resume, so it survives. Worktree-authored content is not committable by git in the worktree (`--assume-unchanged`), but `syncFeatureStateAfterMerge` (`main.go:10169`) copies the whole feature dir back over master on merge — so a worktree amendment silently *replaces* the approved master contract rather than being lost." Replace the §10:369 mitigation with a detection that actually exists (e.g. smoke Step 3's post-ru

### B4 — unrunnable-smoke-step @ §8 Author smoke test, Step 2 (line 293); DoD line 328
That command produces no worktree and never shows an `[r]` prompt, so the step exercises nothing. Parallel-vs-serial is chosen by `hasExplicitDeps` over the milestones *in range*. With `--from M1 --to M1` the range is `[M1]`, and M1 (the first milestone) carries no `(depends: …)` annotation, so `hasExplicitDeps` is false and the run goes to `runLoop` — master tree, no worktree, no `copyBelmontStateToWorktree`, no `os.RemoveAll(dstFeature)`. `handleStaleWorktree` (the only source of the `[r]` prompt) is never reached from `runLoop`; its only callers are the multi-feature path (`:5879`, `:5926`)

**Fix.** Change Step 2 to a range that actually forks a worktree: ensure `{base}/PROGRESS.md` has `### M2: … (depends: M1)`, run `belmont auto --feature <slug> --from M1 --to M2`, Ctrl-C during Wave 1, then rerun the identical command and answer `r` at the `⚠ Branch 'belmont/auto/<slug>/m1' exists from a previous run` prompt. State the expected banner (`Belmont Auto (parallel)`) and the worktree path so a serial fallback is visibly a failure. Also warn that the interrupted run may leave `.belmont/auto.js

### B5 — durability-model-false @ 0003-design-quality-without-figma.md:62-66 (the durability claim), :72-74 (mechanics table), :369 (Risk row 1)
`syncFeatureStateAfterMerge` is not a second copy of the same wipe — it runs in the OPPOSITE direction. It deletes MASTER's feature dir and re-copies it from the WORKTREE, by plain filesystem copy, completely outside git. So a worktree-authored `{base}/TECH_PLAN.md` is uncommittable AND still lands in master at merge time, silently overwriting the approved contract. It does not fail loudly; it arrives in master as an ordinary unstaged modification that the next `git add .belmont/` (which `commit-belmont-changes.md` performs) commits. The proposal's entire safety argument — and the mitigation f

**Fix.** Rewrite §4.1's claim to the true axis: content in `{base}/` is bidirectional — master seeds the worktree at creation/resume (`main.go:10010-10011`) and the worktree overwrites master at merge (`main.go:10169-10170`). State the real invariant the PR needs ("no auto phase may write `{base}/TECH_PLAN.md`") without claiming a mechanical backstop, and delete the `--assume-unchanged` mitigation from Risk row 1 — it protects the git history, not the file. If a mechanical backstop is wanted, it is a Go 

### B6 — false-failure-gate @ 0003-design-quality-without-figma.md:181 (Phase 2 branch 1), :187 (fourth enforcement rule), :130 (contract Mode line)
These two rules contradict each other on every milestone that has both a design reference and a contract — which is every Mode A milestone by construction, since §4.4's contract carries `**Mode**: [A — Figma-derived | …]` and Mode A is defined by Figma URLs being present. Branch 1 runs comparison and explicitly does NOT perform contract checks ("unchanged"); the fourth rule then observes a contract with no contract checks and forces FAIL/INCOMPLETE. Every Figma-using milestone fails verification unconditionally. The same trap catches Mode B features that carry any non-Figma reference, because 

**Fix.** Make the three branches mutually exclusive on their real discriminator and make the fourth rule branch-aware. Either (a) branch 1 becomes "references exist → comparison flow PLUS contract checks" (the contract is orthogonal to a reference and should be checked in both cases), or (b) the fourth rule is scoped: "if branch 2 was selected and contract checks were not performed ⇒ FAIL/INCOMPLETE". Option (a) is the better design — a Figma-derived contract still needs its a11y floor checked. Whichever

### B7 — unrunnable-verification @ 0003-design-quality-without-figma.md:293-295 (Step 2), :297-298 (Step 3)  *(idx 0)*
Smoke Steps 2 and 3 are both inert. Step 2 (`belmont auto --feature <slug> --from M1 --to M1`) cannot engage the worktree path: `inRange` is [M1], M1 carries no `(depends: …)` in the shipped template, so `hasExplicitDeps` is false (main.go:5441-5447) and `runAuto` falls through to `runLoop` (:5453) — a master-tree serial loop that creates no worktree or branch. The `[r]` prompt is the single occurrence at main.go:6100 inside `handleStaleWorktree`, reachable only from `runWaveParallel` and multi-feature mode, so "interrupt mid-run, resume with `[r]`" and "in the worktree" are both unperformable

**Fix.** §8 Author smoke test — replace Steps 2 and 3.

Step 2 (:293-295) becomes:
"**Step 2 — the contract survives `[r]`-resume.** *Precondition: the worktree path engages only when some milestone in range carries `(depends: …)`; otherwise `main.go:5441-5453` routes to `runLoop`, which runs in the master tree with no worktree and no `[r]` prompt. Confirm `{base}/PROGRESS.md` has `### M2: … (depends: M1)` before running.* `belmont auto --feature <slug> --from M1 --to M2`; Ctrl-C during wave 1 (the tracker preserves the worktree); re-run the same command and answer `r` at the `Branch '<branch>' exists 


## Majors

### M1 — durability-model-incomplete @ §4.1 (durability table, row 3) and §10 (risk row 1) of docs/proposals/0003-design-quality-without-figma.md  *(idx 1)*
§4.1 states the durability rule in only one direction. It is correct that master-authored content survives `[r]`-resume (copyBelmontStateToWorktree wipes and re-copies from master), but the unqualified companion claims — line 64 "Content authored inside a worktree does not [survive]" and line 66 "A worktree-local edit to that same path is destroyed exactly like MILESTONE" — are false for the merge path. `syncFeatureS

**Fix.** Four prose edits, no design change.

1. §4.1, replace the sentence at line 66 ("A worktree-local edit to that same path is destroyed exactly like MILESTONE.") with: "That holds for the resume direction only. The opposite direction also exists: after every successful merge, `syncFeatureStateAfterMerge` (`main.go:10160`, called at `:6297` and `:9157`) does `os.RemoveAll` on **mas

### M2 — git-mechanics-falsified @ §4.5 ("The reviewable artifact") and §10 (risk row 1) of docs/proposals/0003-design-quality-without-figma.md  *(idx 2)*
§4.5's "anything written under `.belmont/` inside a worktree is invisible to git (`main.go:10104-10122`)" is false for the exact file it is about. `untrackBelmontInWorktree` (main.go:10104-10122) only walks `git ls-files .belmont/`, so it covers tracked files and nothing else; a NEWLY CREATED `.belmont/` file in a worktree is untracked, is not ignored (the companion `writeWorktreeGitExcludes` writes `.belmont/` to th

**Fix.** 1. §4.5, line 173 — replace the sentence "It is **not** written during an auto wave — anything written under `.belmont/` inside a worktree is invisible to git (`main.go:10104-10122`), so an auto-authored preview would never reach a diff." with the measured behaviour, e.g.: "It is **not** written during an auto wave — and nothing mechanical enforces that. `untrackBelmontInWorktr

### M3 — wrong-citation @ §4.5 line 173  *(idx 3)*
§4.5 (line 173) makes a false and mis-cited behavioural claim. `main.go:10104-10122` is `untrackBelmontInWorktree`, which only marks files already present in the worktree index as `--assume-unchanged`; it cannot make a *new* `.belmont/` file invisible. The function intended to cover new files, `writeWorktreeGitExcludes` (`main.go:10057-10098`, never cited in §4.5), writes to the per-worktree `$GIT_DIR/info/exclude`, 

**Fix.** In §4.5, replace the second sentence of line 173 (from "It is **not** written during an auto wave" to "would never reach a diff.") with:

"It is **not** written during an auto wave — that is the §4.1 rule (tech-plan owns the path), not a property git enforces. `untrackBelmontInWorktree` (`main.go:10104-10123`) only marks `.belmont/` files **already in the worktree index** as `-

### M4 — unsatisfiable-test @ §8 smoke Step 3 (line 298); DoD line 326  *(idx 4)*
Smoke Step 3's `git diff` assertion (line 298) cannot detect the failure it exists to detect, and the DoD item at line 326 names no mechanism at all. A rogue auto-phase edit to the worktree's `{base}/TECH_PLAN.md` is not committed from the worktree (`--assume-unchanged`), but `syncFeatureStateAfterMerge` (`main.go:10160`, `os.RemoveAll` + `copyDir` at `:10169`, called from `:9157` and `:6297`) copies the worktree's f

**Fix.** 1. §8 preconditions block: after Step 1 add a baseline capture. Append to Step 1 (`:290`) a final line: "Then record the post-approval baseline: `PLAN=$(git rev-parse HEAD)`."
2. §8 Step 3 (`:298`): replace "and `git diff` shows **no** modification to `{base}/TECH_PLAN.md` from any auto phase" with: "and `git diff $PLAN..HEAD -- .belmont/features/<slug>/TECH_PLAN.md` is **empty

### M5 — missing-section @ docs/proposals/0003-design-quality-without-figma.md:128-163 (§4.4 contract schema) vs :105, :171, :200, :211, :334  *(idx 9)*
§4.4's contract schema (:128-163) has no state/interaction section, but four other places in the spec assume one exists: §4.3:105 maps `interactive-prototype` to "interaction and gesture states" (a section that does not exist) and `ux-designer` to "UX Strategy **and state inventory**" (half of which does not exist); §5:238 calls the baseline's coverage "all five contract sections (tokens, strategy, microcopy, motion,

**Fix.** Four edits. (1) §4.4, inside the fenced schema, insert a new section between `### UX Strategy` (:148-149) and `### Microcopy Rules` (:151):

### State Inventory
Per interactive component: default · hover · focus · active · disabled · loading · empty · error,
plus any gesture or transition state the interaction introduces (drag, swipe, long-press, optimistic
update). A component

### M6 — unenforced-assertion @ docs/proposals/0003-design-quality-without-figma.md:173 (§4.5) vs :96 (R7), :93 (R4) and :377 (§10)  *(idx 10)*
§4.5:173's two assertions about `{base}/design-preview.html` are both unsupported. (i) "It is **not** written during an auto wave" is produced by no rule in the design: R7 (:96) adds the preview to tech-plan's ALLOWED ACTIONS unconditionally, R4 (:93) is silent on it, and `actionReplan` invokes `/belmont:tech-plan` inside the worktree (main.go:7188-7189) — R5 (:94) even requires the preview in the Codex plan packet, 

**Fix.** Take the regenerate branch (R5 already requires the preview in the Codex packet, and smoke 9a runs auto under codex). Three edits. (1) §4.2 R4 — append to the rule's sentence list: "; and, whenever any contract section is derived or amended, regenerate `{base}/design-preview.html` in the same step and stamp it `<!-- unreviewed: headless replan <ISO date> -->`. If no section cha

### M7 — auto-path-false-mechanism @ docs/proposals/0003-design-quality-without-figma.md:173 (§4.5); risk row :369  *(idx 14)*
§4.5:173 over-generalises §4.1's own correctly-qualified row. `--assume-unchanged` (main.go:10104-10122) covers only files already in the worktree index, so it cannot make a *new* `{base}/design-preview.html` uncommittable; the mechanism that would — `writeWorktreeGitExcludes` — writes to the linked worktree's own gitdir, which git does not read, and the spec already lists that as an out-of-scope `main` bug (:253). V

**Fix.** Three edits.

(1) §4.5, replace the sentence at :173 ("It is **not** written during an auto wave — anything written under `.belmont/` inside a worktree is invisible to git (`main.go:10104-10122`), so an auto-authored preview would never reach a diff.") with:

"It is **not** written during an auto wave — and that is a **prose rule, not a mechanism**. `--assume-unchanged` (`main.

### M8 — vacuous-test-step @ docs/proposals/0003-design-quality-without-figma.md:318 (§8 Step 9c)  *(idx 17)*
Smoke Step 9c (:318) is unfalsifiable as written: `belmont auto` never invokes `/belmont:tech-plan` except via `actionReplan` (the only producer is main.go:7188-7189, reachable only after two verify failures plus an AI REPLAN decision at main.go:7642-7644), and the Tier 1/Tier 2 ladder lives only in the tech-plan reference (§4.3, DoD :332). So a normal `belmont auto --tool opencode` run cannot "confirm Tier 2 runs", 

**Fix.** In §8, replace line 318 with a two-part step. (1) Interactive authoring on opencode, on a SECOND UI-bearing feature with no Figma URLs: "9c: interactive `opencode` → type `/belmont` and pick `/belmont/tech-plan --feature <slug2>` (opencode namespaces commands with `/`, see docs/supported-tools.md:176). Expect `{base2}/TECH_PLAN.md` to carry a `## Design Contract` with the same 

### M9 — unrunnable-smoke-step @ §8 Step 6 (line 307); DoD line 328; §4.2 R4; §10 risk row 2  *(idx 20)*
§8 Step 6 is unrunnable as written, which leaves R4 — the only defence of an approved contract under headless replan, and prose-enforced only per §10:370 — with no executable check, and DoD line 328 unsatisfiable. Two independent reasons: (a) `actionReplan` is never produced by a deterministic rule; at main.go:7642-7644 two verify failures make `decideLoopActionSmart` return nil, handing the choice to the AI decider,

**Fix.** Replace §8 Step 6 (lines 307-309) with a deterministic recipe that tests R4 in isolation, and state why the loop cannot be driven there:

'**Step 6 — headless replan preserves the contract.** `actionReplan` cannot be forced: after two *non-zero-exit* verify runs `decideLoopActionSmart` returns nil (`main.go:7642-7644`) and the AI decider may pick DEBUG instead (`prompts/belmont

### M10 — wrong-mechanism @ §4.6 verification table, row at line 197; severity ladder at line 211  *(idx 24)*
§4.6 line 197 pairs a visual-style assertion ("Focus visible on all interactive") with `browser_snapshot`, whose schema exposes only the accessibility tree plus optional `getBoundingClientRect` boxes and carries no computed-style data. The mechanism can establish tab order and focus location but never whether a focus indicator renders, so the row returns PASS for a component with `outline: none` and no replacement ri

**Fix.** In §4.6, replace the table row at line 197 with:

| Focus visible on all interactive | `browser_press_key Tab` per stop to walk the tab order, then `browser_evaluate` → `getComputedStyle(document.activeElement)` asserting non-`none` `outline-style` with non-zero `outline-width`, or a `box-shadow` differing from the same element's unfocused computed style |

Do NOT write `getCom

### M11 — environment-hardcoding @ 0003-design-quality-without-figma.md:205 ("Fix the stale MCP tool names"), :345 (DoD item)  *(idx 31)*
§4.6 :205 asserts `mcp__playwright__browser_*` "does not exist" and DoD :345 mandates rewriting both agents to `mcp__plugin_playwright_playwright__*`. That prefix is not a property of the Playwright MCP server — it is the namespace Claude Code synthesises for a server registered through the plugin system (`~/.claude/plugins/cache/claude-plugins-official/playwright/unknown/.mcp.json` declares server `"playwright"` run

**Fix.** §4.6: replace the paragraph at :205 with — "**Make the MCP tool references install-independent.** `verification-agent.md:93-94` and `implementation-agent.md:192-193` hardcode `mcp__playwright__browser_*` while `verification-agent.md:69`/`:129` and `implementation-agent.md:195` hardcode `mcp__plugin_figma_figma__get_screenshot`. Neither prefix is canonical: Claude Code names MCP

### M12 — unlisted-consumer @ §5 In scope table; §7 Docs and knowledge — neither lists `prompts/belmont/`  *(idx 34)*
`prompts/belmont/post-verify-triage.md` is an unlisted consumer of verification findings that decides whether they survive (`executeTriageAction`, cmd/belmont/main.go:7005; decision parsed at :6478-6488, with `defer_and_proceed` as the parse-failure default; the prompt itself deletes deferred tasks from PROGRESS.md/PRD.md and appends them to NOTES.md at :63-74). Its blocking list is Figma-scoped (:37 "Significant vis

**Fix.** Four edits.

(1) §5 In-scope table — add a row after `skills/belmont/_src/references/models-yaml-format.md`:
| `prompts/belmont/post-verify-triage.md` | Generalise the Figma-scoped blocking bullet; add a non-deferrable contract list. **Ships only via `scripts/build.sh` (embedded, never installed into a project)** |

(2) §5 header line :5-6 — update the counts: "prose only" now 

### M13 — unlisted-edit @ §5 row `agents/belmont/verification-agent.md`: "Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; focused-reverify exemption; fix Playwright MCP names"  *(idx 35)*
§5's `agents/belmont/verification-agent.md` row omits two edits that §4.6 and smoke Step 4 require. (1) Report shape (the substantive half): the `## Output Format` block's `## Visual Verification` table (:210-221) is Figma-shaped — six fixed Aspect rows, columns `Expected (from design)` / `Actual (from Playwright)` / `Status`, MATCH/MISMATCH values, no Mechanism column — and its :212 preamble admits only two sources 

**Fix.** §5 In-scope table — replace the `agents/belmont/verification-agent.md` row's Change cell with: "Three-way Phase 2; narrow `:131`; fourth enforcement rule; attestation field; focused-reverify exemption; fix Playwright MCP names; **rework the `## Output Format` Visual Verification block (`:210-221`)** — add a contract branch to the `:212` \"Expected\" preamble, and replace the si

### M14 — unlisted-edit @ §5 In scope table — `skills/belmont/_src/implement.md` and `_src/references/implement-milestone-template.md` are absent  *(idx 37)*
§5's In-scope table omits `skills/belmont/_src/implement.md`, which is the orchestrator for the very phase rev 5 redefines. `:96` states Phase 2's purpose in Figma-only terms ("Analyze Figma designs (if provided) for ALL tasks") — on a no-Figma feature that reads as a phase with nothing to do, which would skip the Mode-B dispatch that §7:272 explicitly forbids skipping. `:126` tells the orchestrator the implementatio

**Fix.** 1) §5 In scope — add one row after the `product-plan.md` row: `| skills/belmont/_src/implement.md | Phase 2 Purpose restated as contract consumption (Mode A/B); Visual Validation generalised beyond Figma |`. 2) §4.7 Downstream consumers — add a bullet: "- **`skills/belmont/_src/implement.md`** — the orchestrator prose for Phase 2. `:96` must be restated as: '**Purpose**: Produc

### M15 — unlisted-edit @ §4.7 bullet 3 and §5 row `skills/belmont/_src/verify.md`: "Step 1b recognises the contract shape" / "The edit is therefore small: tell it what shape to look for (`## Design Contract`), not where to look."  *(idx 38)*
§4.7:217 ("The edit is therefore small") and §5:243 ("Step 1b recognises the contract shape") under-scope `_src/verify.md` to a single edit, and §11:385 contradicts them by pricing its rebase-safety test on "a three-line insertion before `:114`" — inside the dispatch block. Three edits are needed: (a) Step 1b (:62-72), since :72's collect list is Figma/image/URL-only and its sole delivery channel is the Step 2 prompt

**Fix.** 1) §5 — replace the verify.md row's Change cell with: `Step 1b collects the contract; dispatch prompt carries it and the no-reference fallback names it; focused-reverify contract exemption at :50`. 2) §4.7 bullet 3 — replace "The edit is therefore small: tell it what shape to look for (`## Design Contract`), not where to look." with "The edit is therefore three narrow ones, not

### M16 — cross-feature-drift @ §4.4 "Derivation order — reuse before invention"; §5 omits `_src/references/tech-plan-master-format.md`  *(idx 39)*
§4.4's derivation order probes only project config (`tailwind.config.*`, CSS custom properties, `components.json`, theme files) and, failing that, mints new defaults with `**Source**: none`. It never consults the master `.belmont/TECH_PLAN.md` or a sibling feature's already-approved contract, and the `**Source**` enum at :131 has no value for either. On a greenfield multi-feature project — the case this PR exists for

**Fix.** 1. §4.4, replace line 165 wholesale with a rung ladder: "**Derivation order — reuse before invention.** Walk in order and stop at the first rung that supplies a token family: (0) a `## Design Contract` in the master `.belmont/TECH_PLAN.md`; (0b) an approved `## Design Contract` in a sibling `.belmont/features/*/TECH_PLAN.md` (most recently approved wins; name which); (1) projec


## Minors

- **m1** (false-internal-claim @ §4.6 line 189 and the table at lines 191-203; DoD line 342) — §4.6's bold lead-in at line 189 promises "Named tooling per check", but rows 200 and 203 name no tool, and for row 203 no read-only tool in the surface the spec pins at line 205 (`mcp__plugin_playwright_playwright__browser_*`) can  
  *Fix:* In §4.6: (1) Replace the mechanism cell of the row at line 200 with "`browser_click` / `browser_hover` / `browser_press_key` to enter each state + `browser_take_screenshot`". (2) Replace the mechanism
- **m2** (miscount @ docs/proposals/0003-design-quality-without-figma.md:6 (header) vs :236-245 (§5 In scope) and :269-273 (§7)) — The header's file count is off by one and its accounting is incomplete. :6 ("9 tracked source files") and :410 ("~520 / 9 tracked sources + 2 corrections") both say nine, but the §5 In-scope table names ten tracked source files (:  
  *Fix:* Header, line 6 — replace with: "**Size:** ~520 lines net across 10 tracked source files (§5) + regenerated `plugin/` + 2 docs pages + 1 new knowledge entry + 1 `KNOWLEDGE.md` routing row + 2 corrected
- **m3** (schema-mismatch @ docs/proposals/0003-design-quality-without-figma.md:90 (R1) vs :130 (contract schema)) — R1 (:90) instructs the template to record `**Design contract**: N/A — no UI in this feature`, but that field name is defined nowhere — the contract schema (:129-163) carries the identical value under `**Mode**:` (:130), and `**Des  
  *Fix:* §4.2, rule R1 (:90) — replace the sentence "A backend-only feature skips it and the template records `**Design contract**: N/A — no UI in this feature`, so downstream agents can distinguish \"no UI\" 
- **m4** (broken-cross-reference @ docs/proposals/0003-design-quality-without-figma.md:372 and :376 (§10 risk table)) — Two §10 risk-table mitigations cite the wrong location. :372 credits §4.4 with forbidding inference on a failed Figma load, but §4.4 (:124-167) is the contract-format section and contains no such prohibition — it lives at §4.7:215  
  *Fix:* §10 risk table, two rows. (a) :372 — replace the mitigation cell "Mode keyed on URL presence only; §4.4 forbids the inference; smoke Step 8" with "Mode keyed on URL presence only (§4.7); §2's NO FALLB
- **m5** (portability-understated @ docs/proposals/0003-design-quality-without-figma.md:118 and :120 (§4.3)) — §4.3 :118 and :120 are stale leftovers from the pre-rev-5 two-skill era: commit d1236f1 expanded tier 1 from two skills to five at :50, :105, :113 and :132 but left both portability paragraphs saying "`ux-designer` and `ui-designe  
  *Fix:* Two sentence edits in §4.3, nothing else.

1. Line 118, replace "`ux-designer` and `ui-designer` are user-scope personal skills at `~/.claude/skills/`. They exist for one person." with: "All five tier
- **m6** (dual-path-test-gap @ docs/proposals/0003-design-quality-without-figma.md:260 (§6 Interactive row) vs §8 Steps 1–6, and :319) — §8 contains no interactive step beyond Step 1 (tech-plan) and 9b (Codex tech-plan), so §6:260's Interactive row ("… → `/belmont:implement` → `/belmont:verify`"), :319's "Steps 1–6 are the Claude-in-both-modes check" and DoD :361 (  
  *Fix:* Close the §6↔§8 inconsistency. Preferred (cheaper, and satisfies AGENTS.md:56 literally): in §8, insert after Step 1 —

"**Step 1b — interactive implement + verify.** From a clean tree after Step 1: `
- **m7** (correction-is-itself-wrong @ docs/proposals/0003-design-quality-without-figma.md:354 (§9 DoD, Corrections carried by this PR)) — The DoD checkbox at :354 states the correction tool-blind and mis-attributes it across the two files, and the same two errors recur at §6:263 and §12:408. (a) "auto mode does not bypass skill discovery" holds for six CLIs but not   
  *Fix:* Three edits. (1) §9 DoD — replace the single checkbox at :354 with two tool-scoped items: "- [ ] `dual-invocation-paths.md:9` corrected — Belmont injects **only** steering text and milestone-scope pro
- **m8** (approval-gate-portability @ docs/proposals/0003-design-quality-without-figma.md:92 (§4.2 R3), with the schema at :133) — Minor citation error only. §4.2 R3 (line 92) cites `user-questions.md:4` for the sentence 'NEVER ask questions as plain inline text when a structured question tool exists'; that sentence is at `user-questions.md:8` (line 4 is blan  
  *Fix:* In §4.2, line 92, change the single token `user-questions.md:4` to `user-questions.md:8`. Make no other edit to R3. Do NOT restructure R3 into three branches and do NOT add a third value to the `**App
- **m9** (unsatisfiable-dod @ §9 Definition of Done → Mechanics, line 359) — DoD line 359's plugin half is ambiguous rather than unsatisfiable: `./scripts/generate-plugin.sh <ver> && git diff --exit-code plugin/` is green only when `<ver>` is the version already committed in plugin/.claude-plugin/plugin.js  
  *Fix:* In §9 Definition of Done → Mechanics, replace line 359 with: "- [ ] `./scripts/generate-skills.sh --check` and `./scripts/generate-plugin.sh --check` pass — the version-agnostic `--check` delivered by
- **m10** (stale-by-construction-citations @ §Sequencing (line 7); §4.1 evidence table (lines 60-74); §4.2 R4; §6; §8 Preconditions (line 279); §12) — 0003 anchors ~25 `main.go:NNNN` citations to a line-numbering scheme its own §Sequencing schedules for destruction: 0005 lands first and takes main.go from 12,613 lines to under ~1,500, relocating copyBelmontStateToWorktree, runSc  
  *Fix:* Two edits to 0003. (1) §Sequencing (:7) — append: "All `main.go:NNNN` line numbers in this document were resolved against HEAD d1236f1 (main.go = 12,613 lines) and are **pre-0005**. 0005's split takes
- **m11** (rule-placement @ 0003-design-quality-without-figma.md:93 (R4), :86 (Phase 3.5 insertion point), :236 (§5 scope for tech-plan.md)) — R4's preserve-verbatim rule is stated only at the derivation step (Phase 3.5) and at outcome level (§8 Step 6, §9 DoD, §10 Risks); the instruction that actually rewrites the file — tech-plan.md:310 / tech-plan-feature-format.md:3,  
  *Fix:* Two edits to 0003. (1) §5 In-scope table, `skills/belmont/_src/references/tech-plan-feature-format.md` row — extend to: "Generalise `## Design Tokens (from Figma)` → `## Design Contract`; add contract
- **m12** (artifact-drift @ 0003-design-quality-without-figma.md:171-175 (§4.5), :93 (R4), :377 (Risk row 9)) — Risk row 9 (:377) overstates its mitigation. "Both authored in the same step from the same source" is true for interactive tech-plan but not for R4's headless replan (:93), which may rewrite `**Approval**` and derive missing secti  
  *Fix:* Two edits, no behaviour change.

1. §10 Risks, row 9 — replace the mitigation cell with: "Both authored in the same step by interactive `tech-plan`. The one exception is R4's headless replan, which ma
- **m13** (ungradeable-checks @ 0003-design-quality-without-figma.md:201-203 (three motion check rows), :211 (severity ladder), :342 (DoD "Every check row names a mechanism")) — Narrow residue only: the reduced-motion row (:203) is the sole check whose mechanism cannot be executed with any tool the Playwright MCP plugin exposes — media emulation requires `browser_run_code_unsafe` (`page.emulateMedia({ red  
  *Fix:* §4.6 check table only; leave :211 alone.

1. Replace row :203 with: "| Reduced motion removes movement, not function | `browser_run_code_unsafe` → `page.emulateMedia({ reducedMotion: 'reduce' })`, the
- **m14** (missing-migration-story @ §4.6 (three-way Phase 2), §4.2 R1, §9 Definition of Done) — The spec never states what happens to a feature planned before this PR. Such a feature's `{base}/TECH_PLAN.md` has no `## Design Contract`, so §4.6 branch 3 applies and it keeps today's acceptance-criteria verification — the gate   
  *Fix:* Two edits, both small.

(1) Add a §4.9 immediately after §4.8, titled "Pre-contract features":

"### 4.9 Pre-contract features

This gate is **not retroactive**. A feature planned before this PR has a
- **m15** (wrong-line-number @ §4.2 rule R3, line 92) — §4.2 rule R3 (spec line 92) cites `user-questions.md:4` for the rule "NEVER ask questions as plain inline text when a structured question tool exists". Line 4 of `skills/belmont/_partials/user-questions.md` is blank; that rule is   
  *Fix:* In §4.2, rule R3 (line 92): change `user-questions.md:4` to `user-questions.md:8`. Leave the trailing `and the strict Codex fallback at `:11`` unchanged — it is correct.
- **m16** (wrong-skill-name @ §4.3 line 107 (Tier 1b)) — §4.3 line 107 writes `frontend-design`, but that skill is installed as a plugin (`frontend-design@claude-plugins-official`) and its registered invocable name is `frontend-design:frontend-design`. The same wrong name already sits i  
  *Fix:* Three coordinated edits: (1) line 107 — replace "If `frontend-design` is available" with "If `frontend-design:frontend-design` is available"; (2) line 122 — extend the sentence to "...currently name `
- **m17** (self-inconsistent-count @ Header lines 5-6) — The header's file counts are wrong and self-inconsistent: line 5's breakdown sums to 7 while line 6 claims 9, and §5's in-scope table actually names 10 tracked source files (3 agents, 4 skill sources, 2 existing skill references,   
  *Fix:* Header, lines 5-6. Line 5 → "**Type:** prose only (3 agents, 4 skill sources, 2 skill references, 1 new skill reference). No Go changes." Line 6 → "**Size:** ~520 lines net across 10 tracked source fi
- **m18** (imprecise-citation @ §4.8 point 2, line 227) — §4.8 point 2 (line 227) attaches a single citation, `models-yaml-format.md:30`, to a two-part claim. `:30` carries only the tier description ("`design` — Figma extraction, token mapping, visual spec"); the profile heuristics that   
  *Fix:* §4.8, point 2 (line 227): replace the first sentence with "`models-yaml-format.md:30` describes the `design` tier as \"Figma extraction, token mapping, visual spec\", and the profile heuristics at `:6
- **m19** (stale-text @ docs/proposals/0003-design-quality-without-figma.md:118 and :120 (§4.3) vs :104-106 and :334) — §4.3's portability paragraphs at :118 and :120 still name only `ux-designer` and `ui-designer` as the user-scope skills, stale text left over from when tier 1 had two members. Since d1236f1 all five tier-1 skills are load-bearing   
  *Fix:* §4.3, two edits. (1) At :118 replace "`ux-designer` and `ui-designer` are user-scope personal skills at `~/.claude/skills/`. They exist for one person." with "All five tier-1 skills — `ui-designer`, `
- **m20** (unsatisfiable-check @ docs/proposals/0003-design-quality-without-figma.md:359 (DoD Mechanics)) — DoD §9 Mechanics :359 is ambiguous rather than unsatisfiable: `./scripts/generate-plugin.sh <ver> && git diff --exit-code plugin/` passes only when `<ver>` is the version already committed in `plugin/.claude-plugin/plugin.json` (0  
  *Fix:* §9 Definition of Done → Mechanics, replace line 359 with: "- [ ] `./scripts/generate-skills.sh --check` passes; `plugin/` regenerated and committed, verified with `./scripts/generate-plugin.sh --check
- **m21** (coverage-gap @ docs/proposals/0003-design-quality-without-figma.md:211 (severity ladder) vs :191-203 (check table)) — §4.6's check table (:191-203) has no elevation row, yet the Token Contract declares elevation with a behavioural rule (:141 "Elevation — [levels]. Interactive elements rise one level on hover."), design-preview.html renders "radiu  
  *Fix:* In §4.6, insert one row into the check table immediately after the `Radius consistent, nested smaller` row (:199):

| Elevation levels as declared; interactive elements rise one level on hover | `brow
- **m22** (coverage-gap @ docs/proposals/0003-design-quality-without-figma.md:361 (DoD) vs :275-321 (§8 smoke test)) — DoD :361 asserts the plugin surface is exercised, but §8 contains no step that installs or runs the plugin build (grep for "plugin" over :275-321 returns nothing), so the item has no procedure behind it. The only plugin mechanic a  
  *Fix:* Add to §8 after Step 9, before **Diagnostics**:

**Step 10 — plugin surface.** `./scripts/generate-plugin.sh <ver>`; confirm `plugin/skills/tech-plan/references/design-authority-baseline.md` exists an
- **m23** (scope-incomplete @ docs/proposals/0003-design-quality-without-figma.md:246 (§5 plugin row) and :261 (§6 Plugin row)) — Both plugin enumerations are incomplete by exactly one skill. §5:246 and §6:261 list `tech-plan`, `product-plan` and `verify`, but §5:244 also puts `skills/belmont/_src/next.md` in scope, and generate-plugin.sh:61-75 copies every   
  *Fix:* §5:246 — replace with: "| `plugin/` | Regenerate — now includes `plugin/skills/{tech-plan,product-plan,verify,next}/**` as well as `plugin/agents/*`. **Requires 0006** |"
§6:261 — replace "`plugin/age
- **m24** (wrong-skill-name @ docs/proposals/0003-design-quality-without-figma.md:107 (§4.3 Tier 1b) and :132 (§4.4 schema example)) — §4.3 writes the bare name `frontend-design` at :107 and :132, but it is a plugin skill (`~/.claude/plugins/cache/claude-plugins-official/frontend-design/…/skills/frontend-design/SKILL.md`) whose registered name is `frontend-design  
  *Fix:* §4.3 :107 — replace "If `frontend-design` is available" with "If `frontend-design:frontend-design` is available (Anthropic plugin skill; the bare `frontend-design` is not a registered name)".
§4.4 :13
- **m25** (untested-changed-surface @ docs/proposals/0003-design-quality-without-figma.md:239 (§5 scope row for product-plan) vs §8) — §8 contains no assertion of any kind for `skills/belmont/_src/product-plan.md`, whose only invocation path is interactive (`buildLoopPrompt`, main.go:7165-7204, has no product-plan case). Per AGENTS.md:29 a skill-behaviour change   
  *Fix:* In §8, inside the Preconditions bash block (after the `grep -c "Figma" .belmont/features/<slug>/PRD.md   # expect 0` line, ~:286), add:

```bash
# both name fixes landed on the generated surface (DoD 
- **m26** (weak-gate @ §9 Definition of Done, line 359 (first half) paired with line 332) — `./scripts/generate-skills.sh --check` cannot verify DoD :332's reference-file assertion: it compares `$DEST_DIR/$n/SKILL.md` against `$ROOT/skills/belmont/$n/SKILL.md`, which are the same path (`generate-skills.sh:24`), so it onl  
  *Fix:* In §9 "Mechanics", replace line 359 with two checkboxes:

- [ ] `./scripts/generate-skills.sh && test -f skills/belmont/tech-plan/references/design-authority-baseline.md` — the copy at `generate-skill
- **m27** (vacuous-dod @ §9 Definition of Done → "Corrections carried by this PR", lines 355-356) — DoD :355 and :356 describe edits rev 5 already made to this proposal document, not deliverables of the PR — `docs/proposals/` appears in neither §5's in-scope table (:234-249) nor §7. Both are already recorded in §12 at :404 and :  
  *Fix:* In §9, under "**Corrections carried by this PR**", delete these two lines verbatim:

- `- [ ] v4's dead `#10`/`#11` conflict block and its DoD item deleted`
- `- [ ] v4's `agents/belmont/references/` 
- **m28** (missing-scope-item @ §4.2 R5; §8 Step 9b (line 317); §5 In-scope table) — R5 (:94) reasons about `codex-plan-apply`'s path constraint but not its content constraint. The skill carries three source-code prohibitions — rule 3 (`:20`), workflow validation step 3 (`:60`) and a hard refusal case (`:81` "The   
  *Fix:* In §5 In-scope, add a row after the `skills/belmont/_src/product-plan.md` row: "| `skills/belmont/_src/codex-plan-apply.md` | Carve planning artifacts out of the source-code prohibition: after rule 3,
- **m29** (docs-incomplete @ §7 "Docs and knowledge" — names only `docs/agent-pipeline.md` and `docs/skills-reference.md`) — §7 "Docs and knowledge" enumerates only `docs/agent-pipeline.md` and `docs/skills-reference.md`, but rev 5 also falsifies or leaves incomplete three further user-facing pages: `README.md:13` (the "Figma-first design workflow" bull  
  *Fix:* In §7 "Docs and knowledge", add three bullets after the `docs/skills-reference.md` line: (1) "`README.md` — reframe the `:13` "Figma-first design workflow" bullet: Figma-first where a Figma exists, an
- **m30** (unhandled-configuration @ §4.5: "authored in the master tree, staged by `commit-belmont-changes.md`'s `git add .belmont/`, and therefore present in the PR diff") — §4.5:173 states unconditionally that the contract and preview are "staged by `commit-belmont-changes.md`'s `git add .belmont/`, and therefore present in the PR diff", but that partial short-circuits entirely when `.belmont/` is gi  
  *Fix:* In §4.5, replace the first sentence of the paragraph at :173 with: "This is durable and reviewable on the same terms as the contract: authored in the master tree and staged by `commit-belmont-changes.

## Refuted — do not act on these

- docs/proposals/0003-design-quality-without-figma.md:93 (R4) and :308 (smoke Step 6): The finding's quotes are locationally accurate (:93, :129-133, :308 all verified verbatim against the file), but its evidence is a selective quote whose ellipsis removes the clause
- docs/proposals/0003-design-quality-without-figma.md:187 (fourth enforcement rule) vs :207 and :305 (focused re-verification): QUOTES: all accurate — the finding does not fail on misquotation. Proposal :187 does say "add a fourth enforcement rule mirroring `:129-130`: *a contract exists but contract checks
- 0003-design-quality-without-figma.md:64 (master-vs-worktree axis), :369 (Risk row 1), :82 ("Replan is the live threat"): The finding's CODE facts are all correct — I verified every one — but its SPEC claim is a misread, and the gap it says is missing is already stated in the spec, twice, in the exact
- idx 5 @ §3 line 40; §4.4 lines 144 and 162; §4.3 line 108: STEP 1 (quote check) — the two anchor quotes are accurate but the load-bearing framing is not. Line 40 reads: "...are Belmont-original prose stating published standards: WCAG 2.1 S
- idx 8 @ docs/proposals/0003-design-quality-without-figma.md:132 (contract template) vs :112-113 (§4.3 ladder) and :335 (DoD): The finding misquotes the spec — it quotes the PREVIOUS revision. At HEAD (d1236f1, the revision under review) line 132 reads: "**Authorities**: baseline[; ui-designer (tokens); ux
- idx 13 @ docs/proposals/0003-design-quality-without-figma.md:94 (§4.2 R5); scope table :246; smoke :317: MISREADS R7 by truncating the disambiguating clause. The finding's evidence says the proposal "acknowledges the same ambiguity and declines to resolve it" and quotes 0003:96 as onl
- idx 21 @ §8 Step 9c (line 318); DoD line 361 ("one non-Claude CLI"): The finding's own evidence misquotes R4 by ellipsis, removing the clause that settles it. Spec line 93 in full: "The step must: **preserve an existing `## Design Contract` verbatim
- idx 23 @ §4.6 verification table, row at line 203; DoD line 336: Quote is accurate (line 203 reads exactly as claimed; line 189 reads "**Named tooling per check.** Every row names its mechanism; a check whose mechanism is unavailable is recorded
- idx 27 @ 0003-design-quality-without-figma.md:78 (the hard rule of the PR), §5 in-scope table :236-249, §7 :269-273: The quote is accurate but the reading is not. Spec :78 in full: "> **`tech-plan` writes the contract. `implement`, `verify`, `next` and `debug` only read it.** No auto-mode phase m
- idx 28 @ 0003-design-quality-without-figma.md:187 (fourth enforcement rule), :182 (branch 2), :90 (R1), :130 (Mode line), :207 (the only stated exemption): Both halves fail on the actual text. Claim (a): R1 at spec :90 reads "A backend-only feature skips it and the template records `**Design contract**: N/A — no UI in this feature`" —
- idx 32 @ 0003-design-quality-without-figma.md:91 (R2), :215 (§4.7 design-agent), :253 (§5 out of scope), :311 (smoke Step 7): STEP 1 (quote check) — the quotes are accurate (:91 R2, :215 §4.7, :253 out of scope, :311 Step 7, and tech-plan.md:158 / product-plan.md:92 verbatim). The failure is in the infere
- idx 36 @ §4.1 hard rule: "`tech-plan` writes the contract. `implement`, `verify`, `next` and `debug` only read it.": The spec quote is accurate (line 78) and the debug-file evidence is accurate (debug-phase1-design.md:1,3; debug-auto.md:30,60-61; the Phase 3 verification dispatch at :126-155 with
- idx 44 @ §1 point 2 (line 18) and §4.6 (line 185): Quotes verified verbatim (0003:18 "`verification-agent.md` Phase 2 is *comparison-only*: its check target is a loaded Figma screenshot or image (`:53-60`)"; 0003:185 "verification-
- idx 45 @ docs/proposals/0003-design-quality-without-figma.md:403 (§12 row): Line 403 reads (raw, no bold — the finding's quote adds `**` emphasis that is not in the file): "| \"nothing runs the project's lint/typecheck\" | False — implementation-agent runs
- idx 52 @ docs/proposals/0003-design-quality-without-figma.md:132 (§4.4 schema) vs :112-113 (§4.3) and :335 (DoD): The finding misquotes rev 5 by pasting v4's text. It claims "§4.4's canonical schema block at :132 renders it as `**Authorities**: baseline[; ux-designer; ui-designer; frontend-des
- idx 57 @ 0003-design-quality-without-figma.md:105 (tier-1 mapping), :102 ("Load **every** tier-1 skill that is present"), :50 (credits table): The quotes are accurate — ~/.claude/skills/interactive-prototype/SKILL.md:3 does end "Do NOT use for static mockups, dashboards, landing pages, or marketing sites", spec :102 does 
- idx 58 @ §5 row `skills/belmont/_src/product-plan.md`: "Fix the same wrong skill names at `:165`" — the only product-plan change: The three quotes are real — product-plan.md:191 is "- Design intent (if no Figma: what should it look and feel like?)" under "Always ASK about", product-plan-prd-format.md has no d
- idx 59 @ §7 "Docs and knowledge" — lists design-authority.md (new), a KNOWLEDGE.md routing row, and Revisions on model-tier-economics.md and dual-invocation-paths.md: Locations verified: §7 (:269-273) lists only design-authority.md, the KNOWLEDGE.md routing row, and Revisions on model-tier-economics.md and dual-invocation-paths.md; codex-plan-ha
- idx 61 @ §4.5 (`{base}/design-preview.html` as a new durable artifact): The finding's load-bearing premise — "both lists are closed" — is a misreading of both quoted lines. skills/belmont/_src/reset.md:65 actually reads: "2. Delete **all files** in `.b
