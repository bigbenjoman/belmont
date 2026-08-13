# `[!]` Is a Decision Queue, Not a Failure

**Domains:** skills, cli, state, auto-mode

**Why this matters.** `[!]` is the only marker in Belmont's set whose resolution requires a person. Every other state describes work an agent can move: `[ ]` can be built, `[x]` can be verified, `[-]` is already settled. `[!]` means the missing input is a human — an approval, a product or architecture ruling, a credential, a console action nobody automated, a spec change that belongs to `/belmont:tech-plan`.

The state model has always protected it. `mergeProgressState` refuses to rank over `[!]` from either direction, `resolveProgressConflict` treats it as a signal rather than a value to reconcile, and no skill may flip one. But protecting a signal is not the same as surfacing it, and *nothing decided what a running loop should do when it hits one*. Two opposite failures followed from that gap, and both were observed on the same feature.

**Failure one: the loop treats a blocker as a stall and stops.** A `[!]` in M1 stops a feature whose M2–M17 have nothing to do with it. On the observed feature that would have stranded 16 milestones behind one product question.

**Failure two: the loop treats a blocker as a bug and grinds at it.** A human-gated follow-up classified as *blocking* burns fix rounds that cannot succeed, then trips the circuit breaker, which defers everything remaining — so the security item that needed a password rotation lands in a polish bucket. Attempting it harder is the one thing guaranteed not to work.

Underneath both: **the queue was never legible.** The observed feature banked 19 blocked follow-ups over one overnight run, every one a question for the same person, each carrying a paragraph of explanation, and the only place they appeared was the tail of `belmont status` — one truncated line each, interleaved with progress counts. 87 tasks verified, 0 of 17 milestones readable as done, and no single surface that said what the person actually had to answer.

## Invariant

- **A `[!]` never stops the loop by itself.** The loop moves past it to the next milestone whose dependencies are satisfied. It stops for the user only when *every* remaining pending task in the feature is `[!]` — at that point no agent action can change the file.
- **No agent clears, answers, or converts a `[!]`.** Not to `[-]` to make a milestone read complete, not to `[ ]` to retry it, not to `[v]` because the surrounding work passed. Writing one is allowed; clearing one is the user's.
- **Human-gated is classified before blocking, not after.** The test is not difficulty or severity — it is whether the missing thing is a *person*. A security item or an unmet acceptance criterion that needs an approval is human-gated, not blocking. Getting the order wrong is failure two.
- **A human-gated task is not a fix round.** An attempt that cannot succeed must not count toward the circuit breaker, or two of them defer the real work.
- **The circuit breaker never sweeps `[!]`.** It defers what is still classified *blocking*.
- **The queue is reportable in one command.** `belmont blockers` prints every `[!]` across every feature, grouped, with the indented body intact.

## How it's enforced

Mechanically (cannot be dropped by an agent):

- `belmont blockers` (`cmd/belmont/blockers.go`) — `buildBlockers` walks every feature (or one), reads live worktree state via `loadAutoWorktrees` exactly as `status` does, and pulls each blocked task's indented body with `taskDetail` → `taskBodyEnd`. Reports only; it has no write path, because a command that both raises a question and resolves it is free to guess the answer.
- `renderStatus` names it after the blocked list in the `--feature` detail view.
- `renderFeatureListing` caps blockers at `blockedListingCap` (3) per feature and prints the withheld count plus the exact command. The listing is the whole-project overview; one feature's 19-item queue used to bury every other feature. Nothing is hidden — the count and the command print on the next line, and the detail view still lists every one.
- `mergeProgressState` / `canonicalMarker` already refuse to rank over `[!]`; see [`progress-md-parsing.md`](progress-md-parsing.md).
- Tests in `cmd/belmont/blockers_test.go`, including that `[-]` never enters the queue and that a blocker with no body does not borrow the next task's lines.

By prose (reduces the chance of the failure, cannot prevent it):

- `_src/loop.md` — triage's three-class split with human-gated checked first; the `[!]`-does-not-stop rule and the `belmont blockers` handoff, both **inside the fenced recipe** (see below); a `Scope rules` bullet stating `[!]` belongs to the user.

## Failure mode if you break it

- Classify human-gated as blocking → two dead fix rounds per milestone, then the circuit breaker defers a real issue as polish.
- Stop the loop on the first `[!]` → every independent milestone behind it is stranded, and the run reports a stall rather than a question.
- Let an agent clear a `[!]` to make a milestone read complete → the feature computes Verified over work nobody approved. This is the `[v]`-without-evidence failure ([`verified-flip-recording.md`](verified-flip-recording.md)) with a different marker and no guard, because no Go code audits a `[!]` that disappeared.
- Leave the queue visible only in `belmont status` → it is read one truncated line at a time, in the middle of a report about something else, and the questions go unanswered for as long as the run lasts.

## Don't re-do

- **"Add an `[a]` awaiting-approval marker."** Rejected. `[!]` already means exactly this, every reader already handles it, and adding a marker changes the meaning of files already in the wild — the expensive lesson from `[-]` and `[V]` (see [`progress-md-parsing.md`](progress-md-parsing.md)). The gap was never the marker; it was that nothing surfaced or routed around it.
- **"Let the loop ask the user inline and continue."** Rejected for the delegated `/loop` path. The loop's premise is that it runs unattended across compaction; a question asked into a transcript nobody is watching is worse than a queue, because it leaves no record on disk.
- **"Have `belmont blockers` clear a task once answered."** Rejected. The answer arrives as prose from a person; encoding it means guessing which task it settles. Editing the marker by hand, or letting `/belmont:next` pick the task up once the input exists, keeps the human in the one loop that needs them.
- **"Exclude blocked milestones from the feature's milestone count so it can read Complete."** Rejected. A milestone holding an unanswered question is not done, and `[-]` already exists for work genuinely dropped. Withdraw it deliberately or answer it.
- **"Put the blocked-stop rule in the skill's `## Stop conditions` prose."** Rejected — that is where it was, and it is the section most likely to be summarised away mid-run. Every stop condition lives inside the fenced recipe handed to `/loop`, which is the only text guaranteed to travel with the delegated task. The prose section restates them for a reader.

## Evidence

`ai-agents` / `robin-pm-upgrade`, overnight run of 2026-08-12→13 on Belmont 0.10.17: 165 tasks, of which 92 were verification-generated follow-ups. Final state 87 `[v]`, 9 `[x]`, 50 `[ ]`, **19 `[!]`** — the blocked tasks spread across M1–M10, every one of which therefore computed as "has blockers", giving `0/17 milestones done` against `92/165 tasks done`. Sample: *"the apply needs Ben's approval"*, *"Operator — populate the roster"*, *"Rotate the database passwords found during the studia-portal scan"*, *"Rule on the P2-default probe"*, *"Decide whether the completeness signal propagates to the voice pipeline"*. None is a bug; none can be closed by an agent; all 19 were visible only as truncated lines at the bottom of `belmont status`.

## Revisions

2026-08-13 — Created. Adds `belmont blockers`, the three-class triage split in `loop.md`, and the rule that `[!]` neither stops the loop nor gets swept by the circuit breaker. Motivated by the `robin-pm-upgrade` overnight run above.
