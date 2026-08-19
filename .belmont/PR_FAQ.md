# PR/FAQ: not here

This repository is `bigbenjoman/belmont`, a tracking fork of `blake-simpson/belmont`, and — since 2026-08-18 — the place where Belmont's own framework features are executed by Belmont.

**There is no PR/FAQ for this repository, deliberately.** The strategic layer it would carry lives at `~/belmont/`, the private planning workspace governing all five product repositories plus this one. This file replaced the `belmont install` stub on 2026-08-19; do not run `/belmont:working-backwards` here expecting to fill it.

Read instead, in this order:

- `.belmont/PRD.md` — what this repo carries and what it deliberately does not.
- `.belmont/TECH_PLAN.md` — the seven Kind B rules that make self-hosting safe. Rule 7 (no product's confidential information) is a gate on `throughput`'s wave 2, not advice.
- `.belmont/features/<slug>/` — the framework features themselves. Currently `throughput`.
- `AGENTS.md`, `CONTRIBUTING.md`, `knowledge/KNOWLEDGE.md` — engineering conventions, authoritative for build, test and review.

**Why a pointer rather than an empty file.** `worktree.go` copies `PR_FAQ.md` into every worktree as a context file, and seven planning skills instruct agents to read it. The stub told each of them to run `/belmont:working-backwards` — an instruction to create a strategic document mid-feature, delivered to agents that must not create one. `belmont status` reads this file as having real content, so `PRFAQReady` now reports true; that is a statement about the pointer existing, not about a PR/FAQ existing.
