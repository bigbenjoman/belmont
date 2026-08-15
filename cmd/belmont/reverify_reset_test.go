package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The reset `belmont reverify` performs is a DESTRUCTIVE step that runs before
// any verification has happened: it rewrites `[v]` back to `[x]` so the
// verification agent picks the tasks up again. Nothing in the command restores
// it — there is no backup, no signal handler, and the per-milestone failure
// branch records the error and moves on.
//
// So the only thing bounding the loss is WHEN the reset happens. Doing it for
// every milestone up front means a run interrupted during M1 has already
// destroyed M7's verified marks, and nothing re-derives them: no Go code ever
// promotes a `[v]`, so each one costs another verification agent run to earn
// back (see knowledge/cross-cutting/verified-flip-recording.md). Resetting a
// milestone immediately before it is dispatched bounds the loss to the one
// milestone actually in flight.
//
// That distinction is invisible to a test that only inspects the file after the
// command returns — by then every milestone has been dispatched either way.
// What separates the two is what the file looks like DURING an earlier
// milestone's dispatch, so these tests stand a fake tool binary on PATH that
// snapshots PROGRESS.md each time it is invoked. See issue #49.

// reverifyFixture writes a throwaway project root holding one feature with two
// milestones, each carrying one `[x]` and one `[v]` task.
func reverifyFixture(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	featureDir := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	progress := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
- [v] P0-M1-2: Parse the body

### M2: Rendering
- [x] P0-M2-1: Render the header
- [v] P0-M2-2: Render the body
`
	if err := os.WriteFile(filepath.Join(featureDir, "PROGRESS.md"), []byte(progress), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "PRD.md"), []byte("# PRD: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// stubToolSnapshotting puts a fake `claude` on PATH that copies PROGRESS.md into
// snapDir on every invocation, named by call order. Returns snapDir.
//
// exit 0 on purpose: a failing tool would abort the comparison this test needs,
// and the bug reproduces just as well on a clean run — the reset is destructive
// before it is justified regardless of how the agent then fares.
func stubToolSnapshotting(t *testing.T, root string) (snapDir string) {
	t.Helper()
	binDir := t.TempDir()
	snapDir = t.TempDir()
	progressPath := filepath.Join(root, ".belmont", "features", "demo", "PROGRESS.md")
	script := "#!/bin/sh\n" +
		"n=$(find \"" + snapDir + "\" -type f | wc -l | tr -d ' ')\n" +
		"cp \"" + progressPath + "\" \"" + snapDir + "/snap-$n\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepended, not replaced: the stub is a /bin/sh script and needs the real
	// coreutils on PATH to run at all. binDir first is what shadows any genuine
	// `claude` the developer has installed.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return snapDir
}

func readSnapshot(t *testing.T, snapDir string, n int) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(snapDir, "snap-"+itoa(n)))
	if err != nil {
		entries, _ := os.ReadDir(snapDir)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("no snapshot %d (the stub tool was invoked %d time(s): %v): %v",
			n, len(entries), names, err)
	}
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestReverifyDoesNotResetAMilestoneItHasNotReached is the core of #49. While M1
// is being verified, M2 has not been dispatched and must still hold its `[v]`.
func TestReverifyDoesNotResetAMilestoneItHasNotReached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub tool is a /bin/sh script")
	}
	root := reverifyFixture(t)
	snapDir := stubToolSnapshotting(t, root)

	if err := runReverifyCmd([]string{"--root", root, "--feature", "demo", "--tool", "claude"}); err != nil {
		t.Fatalf("reverify returned an error: %v", err)
	}

	duringM1 := readSnapshot(t, snapDir, 0)
	if !strings.Contains(duringM1, "[v] P0-M2-2") {
		t.Errorf("M2's verified task was already reset while M1 was still being verified.\n"+
			"An interrupted run would have destroyed it having verified nothing.\n"+
			"PROGRESS.md as the agent saw it during M1:\n%s", duringM1)
	}
}

// TestReverifyStillResetsTheMilestoneItIsAboutTo Verify pins the other half: the
// reset must still happen, and must be visible to the agent handling that
// milestone. A "fix" that simply stopped resetting would pass the test above and
// break the command — the agent looks for `[x]`, so an unreset `[v]` is skipped
// and `reverify` silently verifies nothing.
func TestReverifyStillResetsTheMilestoneItIsAboutToVerify(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub tool is a /bin/sh script")
	}
	root := reverifyFixture(t)
	snapDir := stubToolSnapshotting(t, root)

	if err := runReverifyCmd([]string{"--root", root, "--feature", "demo", "--tool", "claude"}); err != nil {
		t.Fatalf("reverify returned an error: %v", err)
	}

	duringM1 := readSnapshot(t, snapDir, 0)
	if !strings.Contains(duringM1, "[x] P0-M1-2") {
		t.Errorf("M1's verified task was not reset before M1 was dispatched, so the "+
			"verification agent has nothing to pick up.\nPROGRESS.md as the agent saw it during M1:\n%s", duringM1)
	}

	duringM2 := readSnapshot(t, snapDir, 1)
	if !strings.Contains(duringM2, "[x] P0-M2-2") {
		t.Errorf("M2's verified task was never reset, so its dispatch verifies nothing.\n"+
			"PROGRESS.md as the agent saw it during M2:\n%s", duringM2)
	}
}

// TestReverifyRereadsProgressBetweenMilestones pins the subtlety the issue
// flagged as worth settling first. The agent commits its own edits to
// PROGRESS.md between milestones, so a per-milestone reset that kept working
// from the copy read at startup would write M2's reset on top of a stale
// document and silently revert whatever M1's agent recorded.
func TestReverifyRereadsProgressBetweenMilestones(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub tool is a /bin/sh script")
	}
	root := reverifyFixture(t)
	progressPath := filepath.Join(root, ".belmont", "features", "demo", "PROGRESS.md")

	// A stub that behaves like a verification agent: it snapshots what it was
	// given, then records its own result by flipping M1's tasks to [v].
	binDir := t.TempDir()
	snapDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"n=$(find \"" + snapDir + "\" -type f | wc -l | tr -d ' ')\n" +
		"cp \"" + progressPath + "\" \"" + snapDir + "/snap-$n\"\n" +
		"sed -i.bak 's/\\[x\\] P0-M1-/[v] P0-M1-/' \"" + progressPath + "\"\n" +
		"rm -f \"" + progressPath + ".bak\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := runReverifyCmd([]string{"--root", root, "--feature", "demo", "--tool", "claude"}); err != nil {
		t.Fatalf("reverify returned an error: %v", err)
	}

	duringM2 := readSnapshot(t, snapDir, 1)
	if !strings.Contains(duringM2, "[v] P0-M1-1") || !strings.Contains(duringM2, "[v] P0-M1-2") {
		t.Errorf("M1's verification result was lost when M2's reset was written — the reset "+
			"worked from a stale in-memory copy instead of re-reading from disk.\n"+
			"PROGRESS.md as the agent saw it during M2:\n%s", duringM2)
	}
}
