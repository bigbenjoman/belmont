# Progress: Belmont (self-hosted)

## Features

| Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
|---------|------|----------|-------------|--------|------------|-------|
| Belmont Throughput | throughput | P0 | — | In Progress | 0/13 | 20/73 |

Status, Milestones and Tasks are **computed** from `.belmont/features/<slug>/PROGRESS.md` — run `belmont sync` to refresh them rather than editing by hand. Priority and Dependencies are set during planning. Figures above are `belmont status --format json` after verify round 7 (2026-08-19): 20 done of 73, of which **12 verified**, plus 2 withdrawn.

> **This file was created on 2026-08-19 by `/belmont:verify`; it had never existed in this repository** (no git history for `.belmont/PROGRESS.md`).
>
> **The `[v]` → `[x]` demotion noted here on round 6 is resolved.** The working tree had carried an uncommitted, unexplained demotion of 18 M1 tasks from `[v]` to `[x]`. Round 7 re-verified all 20 M1 `[x]` tasks from first principles rather than restoring the markers: 12 were re-established and promoted, 8 are held at `[x]` against confirmed Critical or Warning findings. The Tasks column is no longer disputed.

## Recent Activity

| Date | Feature | Activity |
|------|---------|----------|
| 2026-08-19 | throughput | **Verify round 7 on M1 — full re-verification of all 20 `[x]` tasks** after an unexplained working-tree demotion of 18 `[v]` markers. Evidence-first: task-body self-narration of prior rounds was ignored and every measured figure re-derived. **12 promoted `[v]`** (P0-12, P0-13a, P0-4a, FIX-1…FIX-6, FIX-9, FIX-10, FIX-11). **8 held `[x]`**: P0-13 (3 unmerged branches carry no verdict, incl. `origin/fix/issue-sweep` which edits M8's own deliverable), P0-2 (unsynchronised concurrent `tailWriter` writes introduced by `attachUsageCapture`; undocumented `belmont metrics`), P0-1 (`-race` precondition never wired into CI and mis-stated), P0-4 (undocumented `belmont extract`), P0-3 + FIX-12 (unchanged from round 6), FIX-7 (unreadable *register* still swallowed while `coverage_complete` reports true), FIX-8 (`CENSUS.md:172` over-assertion contradicting a clause 11 lines below). Nine follow-ups filed into M1: `P0-M1-FIX-17` … `P0-M1-FIX-25`. Gates all green: `go build`, `go test -count=1 ./cmd/belmont`, `go test -race`, `go vet ./...`, `gofmt -l`, Tier 1 evals, `belmont validate`. |
| 2026-08-19 | throughput | Verify round 6 on M1 — scoped to `P0-3` and `P0-M1-FIX-12`, the only two M1 tasks never through a verification pass. **Neither promoted.** `P0-M1-FIX-12`: verification 7/7 PASS incl. an end-to-end run on a real install, code review clean on every scope/security/pattern check — held at `[x]` over one Warning inside its own clause-(c) surface (`metrics.go` header records three zero-usage phases; measured count is two). `P0-3`: 7 criteria met / 1 partial / 1 not met, 40 of 43 registers reproduce byte-for-byte, but the procedure pins the binary and not the measured repos, "the estate" is 2 repos of 5, and `baseline.json` still carries the withdrawn capture procedure (Critical). Four follow-ups filed into M1: `P0-M1-FIX-13` … `P0-M1-FIX-16`. Gates all green. |
