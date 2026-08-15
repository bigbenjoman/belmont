Domains: skills, agents, auto-mode

# Dispatch authorization: the host's rules outrank the skill's

## Why this matters

Belmont's orchestrator skills do not run on bare metal. They run *inside* a host CLI that has its own system prompt, and that prompt is not neutral about sub-agent dispatch. Claude Code sessions carry a standing line — quoted verbatim out of a live headless child — reading:

> Do not call the AgentTool unless the user requested it

Belmont's prompt is machine-assembled ("Run the belmont:implement skill against feature …"). Nothing in it looks like a human asking for delegation. So an orchestrator reading `dispatch-strategy.md` faces two instructions in genuine conflict: the skill says *prefer Approach A*, the host says *not unless asked*. Both are legitimate; neither cites the other.

The host wins, and it wins **silently**. A model resolving that conflict downgrades to sequential inline execution and, unless something forces it to say so, reports a completed milestone that reads exactly like a dispatched one.

The conflict is also **not deterministic**. It is a judgement call, so the same fixture on the same commit resolves differently run to run — which is worse than a consistent failure, because it defeats the "run it again" instinct that normally exposes a broken path.

## Invariant

Prose that tells an orchestrator to dispatch MUST also tell it that dispatch is **authorized**, not merely preferred. The skill decides *whether the tool exists*; it must never leave the orchestrator deciding *whether it is allowed*.

Approach selection is gated on **tool presence alone**. A runtime *refusal* of a specific call is the one legitimate reason to fall back with the tool present — and it must be reported as a refusal, not as an absence.

## How it's enforced

`skills/belmont/_partials/dispatch-strategy.md`, inlined at build time into every orchestrator skill (`implement`, `verify`, `debug-auto`, `debug-manual`):

- **"Running this skill is the request to dispatch."** States the authorization as fact and tells the orchestrator not to re-litigate it: a human ran `/belmont:<skill>`, or ran `belmont auto`, which shells out on their behalf. The skill they invoked *is* delegation; the prompt is the user asking, relayed through Belmont.
- **The choice is re-scoped to presence.** "Choose Approach B because a name below is missing from your tool list — never because dispatching felt unrequested."
- **Approach B's entry condition is stated negatively at its own heading**, so the fallback section refuses to apply to a session that has the tool.
- **The orchestrator must announce its selection in one line** (from #45). This is what makes a silent downgrade visible at all, and it is the mechanism that caught this.

This mirrors the host's own carve-out rather than fighting it: Claude Code's Workflow rule already lists "the user invoked a skill or slash command whose instructions tell you to call Workflow" as explicit opt-in. Belmont is asserting that same shape for the dispatch tool.

## Failure mode if you break it

- **Silent Approach B.** Every phase runs in the orchestrator's own context. A long `implement` burns context far faster than its phase count suggests, and nothing in the output says why.
- **`models.yaml` goes inert.** Tiers are applied through the dispatch call's `model:` parameter. With no dispatch call there is nowhere to put one, so every agent runs at the session model regardless of its configured tier. There is no runtime validation of `models.yaml` — nothing reports this.
- **Evals go nondeterministic instead of red.** With dispatch genuinely available, some runs dispatch and some do not, and the two paths file different numbers of follow-up tasks for the same seeded defect. The suite fails on a cross-run comparison rather than on any assertion, which reads as flakiness and invites someone to relax the check.

## Don't re-do

- **"Name the tool correctly and dispatch will happen."** That was #45's fix and it was necessary but not sufficient. Correcting `Task`/`TeamCreate` → `Agent` moved Claude Code from *never* dispatching to dispatching in roughly half of observed runs; the rest declined on authorization grounds while explicitly stating they had the tool.
- **"It must be leaking from the orchestrating session."** Measured and false. The directive survives a scrubbed environment: a bare `claude -p` with `CLAUDECODE`, `CLAUDE_CODE_CHILD_SESSION`, the session ID and the messaging socket all unset still reports carrying it. It is in no file on disk — `~/.claude/settings.json`, `~/.claude.json`, `~/.claude/CLAUDE.md`, project settings and managed-settings were all checked.
- **"Tell the model to ignore its system prompt."** Rejected, and it is not what the fix does. The host's rule is conditional — *unless the user requested it* — so the honest move is to establish that the condition is met, which it is. Prose instructing an agent to disregard its host would be both untrue and unstable.
- **"Set `--permission-mode bypassPermissions` and the problem goes away."** It does not. Belmont already passes it, and `Agent` is already in the `--allowedTools` list (`toolexec.go`). Permission was never the blocker; the blocker was an instruction about *when to choose* to call, which no permission flag addresses.
- **"Assert dispatch happened in the eval."** Tempting, and forbidden by `meta/evals.md` — that is an assertion on prose, and it breaks on the first model swap. The announcement line exists so a human reading a transcript can see it; it is not an assertion target.

## Evidence

- Issue #45. Found by *reading* a Tier 2 transcript, not by an assertion: children said "no Task/TeamCreate dispatch available here" in their own words.
- The follow-on, found the same way: after #45, children said *"I have the `Agent` tool, but this session carries a standing directive not to call it unless the user asks for it"* — and then took the fallback. Three fixtures, same reasoning, independently phrased.
- The `failing-acceptance` fixture, 3 runs per commit: **on `main`, Approach C three times out of three** — uniform, because the tool-name check could not pass, so the cross-run comparison was trivially satisfied. **On the fix branch, a mix of A and B**, and the suite failed on `P1-M1-FIX-2` (an agent-invented follow-up ID) present in run 1 and absent in run 2. The nondeterminism was not introduced by the branch; it was *uncovered* by it. Every `LiveExpect` transition held in all 9 runs.
- A live headless child, given Belmont's exact argv, quoted the directive back verbatim on request and confirmed `Agent` was in its tool list at the same time.

## Revisions

- 2026-08-15 — initial. Records the host-vs-skill dispatch conflict found while diagnosing a nondeterministic Tier 2 run after #45, the authorization prose that resolves it, and the measurements ruling out session leak and permission flags as causes.
