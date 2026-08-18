# Notes

## 2026-08-18

### Environment
- Go stable at the time of P0-12 is **1.26.6** (1.27 was at rc3); 1.21 and 1.24 are both out of Go's two-release support window. `go.mod` now declares `go 1.26.0`, so `GOTOOLCHAIN=auto` auto-downloads go1.26.0 on any machine whose installed toolchain is older (this machine has go1.24.4 installed).
- **`staticcheck` must be built with a toolchain >= the module's `go` directive.** After the bump, the locally installed staticcheck (built with go1.25, because staticcheck's own `go.mod` requires 1.25 and the machine toolchain was 1.24.4) failed with `file requires newer Go version go1.26 (application built with go1.25)` on both `scripts/declsum/main.go` and a vendored stdlib file. Fix is to rebuild it, not to pin or revert: `GOTOOLCHAIN=go1.26.0 go install honnef.co/go/tools/cmd/staticcheck@latest`. CI is unaffected — `setup-go` installs 1.26 *before* the `go install`, so CI's staticcheck is already built with 1.26.

### Discovery
- `go build -tags embed ./cmd/belmont` fails from a clean checkout with `pattern all:agents: no matching files found`. This is **pre-existing and unrelated to the toolchain** (verified by stashing the bump and reproducing on `go 1.21`): `//go:embed` paths in `cmd/belmont/embed.go` resolve relative to the package directory, and `scripts/build.sh` stages `agents/`, `skills/` and `prompts/` into `cmd/belmont/` before building. Use `scripts/build.sh`, never a bare `-tags embed` build.
- `go test -race ./cmd/belmont` was **already clean** before any P0-1 change, on both go1.24.4 and go1.26.0. The MILESTONE's warning 6 (expect pre-existing races from `worktreeTracker`) did not materialise.

### Debugging
- **`git branch -r --no-merged main` over-reports in a rebase-heavy repo.** It compares ancestry, so a rebased-and-landed branch still lists as unmerged. 21 of Belmont's 35 "unmerged" branches were fully or effectively landed. Use `git cherry -v main <branch>` (patch-equivalence: `-` = already upstream, `+` = outstanding), then hand-check the residue — patch-ids are defeated by context drift, which produced false positives on `feat/maintenance-ci` and `fix/worktree-git-excludes`.
- **`git rev-parse "main:$path"` echoes the argument back when it cannot resolve it**, so a missing file reads as present. Use `git rev-parse --verify --quiet` or `git cat-file -e`. This silently inverted a whole triage pass.
- **Two-dot `git diff main..<branch>` is meaningless when main is far ahead** — it reports main's own additions as the branch's deletions (every branch looked like a 30,000-line deletion against a 157-commit-ahead main). Use three-dot `main...<branch>` for a branch's own changes.

### Discovery
- The MILESTONE's two "most dangerous" branches are phantoms: `origin/fix/unrecognised-task-markers` (64 files, "widest collision in the backlog") is 24/24 landed, and `origin/feat/maintenance-ci` (said to collide with P0-12, P0-1 and P0-2 at once) is 29/31 landed with both leftovers verified present. Four of the TECH_PLAN's five named M3/M4/M6 collisions are already on `main`. **M3, M4 and M6 are not standing on contested ground.**
- The real hazard is `origin/fix/post-51-triage` (2026-08-17, newest, 15 unlanded commits, 6 new regression tests) — it merges with **zero** conflicts and touches `reconcile.go` + `worktree.go`, which P0-1 rewrites.
