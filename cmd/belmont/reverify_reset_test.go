package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
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

// `belmont reverify --format json` builds its document by hand. That is fine for
// the numbers and wrong for every string, twice over: the follow-up list was
// hand-concatenated (`["`+Join(`","`)+`"]`), which broke on a `"`; replacing that
// with `%q` fixed the easy cases and still emits `\xNN` for a control byte or
// invalid UTF-8, which JSON has no escape for. Follow-up labels are `t.ID`
// falling back to `t.Name`, so free text a human wrote reaches the writer
// verbatim — a name pasted out of coloured terminal output carries ESC.
//
// This asserts against `jsonString`, the production encoder. The version of this
// test written alongside the `%q` fix rebuilt the document inside the test with
// `%q` and unmarshalled THAT — so it asserted on its own construction and could
// not fail whatever `reverify.go` did. It passed while the binary emitted
// unparseable JSON.
func TestReverifyJSONEncodesHostileFollowUpNames(t *testing.T) {
	hostile := []string{
		`Add the "verified" marker check`,
		`Handle a back\slash in paths`,
		"A tab\tand a newline\nin one name",
		"Handle the \x1b[31mred\x1b[0m case",
		"an invalid UTF-8 byte \xff here",
	}
	for _, name := range hostile {
		encoded := jsonString(name)
		var got string
		if err := json.Unmarshal([]byte(encoded), &got); err != nil {
			t.Errorf("jsonString(%q) = %s, which is not valid JSON: %v", name, encoded, err)
			continue
		}
		// Invalid UTF-8 is replaced with U+FFFD rather than preserved byte for
		// byte; everything else must round-trip exactly.
		if !utf8.ValidString(name) {
			continue
		}
		if got != name {
			t.Errorf("jsonString round-trip changed the name:\n got %q\nwant %q", got, name)
		}
	}

	// And the whole record, assembled the way runReverifyCmd assembles it.
	quoted := make([]string, len(hostile))
	for i, f := range hostile {
		quoted[i] = jsonString(f)
	}
	doc := fmt.Sprintf(`{"feature":%s,"results":[{"id":%s,"name":%s,"fwlups":[%s],"error":%s}]}`,
		jsonString("demo"), jsonString("M1"), jsonString("First \x1b[0m"),
		strings.Join(quoted, ","), jsonString(`reset failed: unexpected " in path`))
	var parsed map[string]any
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		t.Fatalf("the assembled reverify document is unparseable: %v\n%s", err, doc)
	}
}

// `belmont repair` moving a task into a milestone whose final bullet is a NESTED
// child used to strand that child's parent's `**Evidence**` — the moved task was
// spliced above it and the evidence re-attached. Same defect the two merge paths
// were fixed for, in the third writer, while three comments asserted repair
// anchored identically to them.
func TestRepairMoveLandsBelowAParentsEvidence(t *testing.T) {
	lines := strings.Split(`### M1: First
- [ ] P0-M1-1: Wrong place

### M2: Second
- [x] P0-M2-1: Parent
  - [x] P0-M2-2: Child
  **Evidence**: commit aaa111 proves P0-M2-1
`, "\n")

	out, _, _ := moveTaskLines(lines, map[int]string{1: "M2"})
	doc := strings.Join(out, "\n")

	moved, evidence := -1, -1
	for i, l := range out {
		if strings.Contains(l, "P0-M1-1") {
			moved = i
		}
		if strings.Contains(l, "**Evidence**") {
			evidence = i
		}
	}
	if moved == -1 {
		t.Fatalf("the task was not moved at all:\n%s", doc)
	}
	if evidence == -1 {
		t.Fatalf("the evidence line vanished:\n%s", doc)
	}
	if moved < evidence {
		t.Errorf("moved task spliced ABOVE P0-M2-1's own **Evidence**, which now reads as the moved task's:\n%s", doc)
	}
}
