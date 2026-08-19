# Product: Belmont (self-hosted)

This repository is `bigbenjoman/belmont`, a tracking fork of `blake-simpson/belmont` — and, since 2026-08-18, the place where Belmont's own framework features are planned and executed by Belmont.

**The master product PRD is not here.** It lives at `~/belmont/.belmont/PRD.md`, in the planning workspace that governs all five repos (repo-1, repo-5, repo-4, repo-2, repo-3) plus this one. Read it there for vision, constraints and cross-cutting decisions. Do not restate it in this file, and do not run `/belmont:product-plan` here expecting to create one — this file replaced the placeholder that said exactly that.

What this repo carries instead:

- `.belmont/TECH_PLAN.md` — the seven Kind B rules that make self-hosting safe: binary pinning, the no-reinstall window between M1 and M11, barbell model tiers, zero Go dependencies, proposals-before-contract-change, Tier 2 evals for any prose change, and — added 2026-08-19 — **no product's confidential information in this repository**, which is a gate on `throughput`'s wave 2 and not merely advice.
- `.belmont/features/<slug>/` — the framework features themselves. Currently `throughput`.
- `AGENTS.md`, `CONTRIBUTING.md`, `knowledge/KNOWLEDGE.md` — the engineering conventions, authoritative for build, test and review.
