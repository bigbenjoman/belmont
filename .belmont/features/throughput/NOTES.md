# Notes

## 2026-08-18

### Environment
- Go stable at the time of P0-12 is **1.26.6** (1.27 was at rc3); 1.21 and 1.24 are both out of Go's two-release support window. `go.mod` now declares `go 1.26.0`, so `GOTOOLCHAIN=auto` auto-downloads go1.26.0 on any machine whose installed toolchain is older (this machine has go1.24.4 installed).
- **`staticcheck` must be built with a toolchain >= the module's `go` directive.** After the bump, the locally installed staticcheck (built with go1.25, because staticcheck's own `go.mod` requires 1.25 and the machine toolchain was 1.24.4) failed with `file requires newer Go version go1.26 (application built with go1.25)` on both `scripts/declsum/main.go` and a vendored stdlib file. Fix is to rebuild it, not to pin or revert: `GOTOOLCHAIN=go1.26.0 go install honnef.co/go/tools/cmd/staticcheck@latest`. CI is unaffected — `setup-go` installs 1.26 *before* the `go install`, so CI's staticcheck is already built with 1.26.

### Discovery
- `go build -tags embed ./cmd/belmont` fails from a clean checkout with `pattern all:agents: no matching files found`. This is **pre-existing and unrelated to the toolchain** (verified by stashing the bump and reproducing on `go 1.21`): `//go:embed` paths in `cmd/belmont/embed.go` resolve relative to the package directory, and `scripts/build.sh` stages `agents/`, `skills/` and `prompts/` into `cmd/belmont/` before building. Use `scripts/build.sh`, never a bare `-tags embed` build.
- `go test -race ./cmd/belmont` was **already clean** before any P0-1 change, on both go1.24.4 and go1.26.0. The MILESTONE's warning 6 (expect pre-existing races from `worktreeTracker`) did not materialise.
