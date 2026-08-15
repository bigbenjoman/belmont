package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A REAL wave, on a REAL git repository.
//
// Everything else covering `mergeProgressState` and `syncFeatureStateAfterMerge`
// passes plain directories and string fixtures. That proves the merge rules, and
// it is what the #53 unit tests do. It does not prove the thing this file is
// for: that the rules still hold once `git worktree add` has made the copies,
// `--assume-unchanged` is actually armed on the tracked `.belmont/` files, and a
// real `git merge` has run in between.
//
// That distinction is not academic here. The whole reason a lost task is
// unrecoverable is `--assume-unchanged`: the state exists in no commit, so the
// disk copy is the only transport. A test that never arms it is testing the one
// condition under which the bug would not have mattered.
//
// AGENTS.md: "All unit tests pass" is necessary but not sufficient. These are as
// close to the real thing as is reachable without spending an agent — and the
// agent is not what any of this changed.

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// realRepo builds a git repository with a committed Belmont feature.
func realRepo(t *testing.T, progress string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")

	fd := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(fd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fd, "PROGRESS.md"), []byte(progress), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fd, "PRD.md"), []byte("# PRD: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "seed")
	return root
}

const realWaveDoc = `# Progress: demo

## Milestones

### M1: Parsing
- [ ] P0-M1-1: Parse the header
- [ ] P0-M1-2: Parse the body

### M2: Rendering
- [ ] P0-M2-1: Render the header
- [ ] P0-M2-2: Render the body
`

// TestRealWavePreservesBothSiblingsAndTheirBodies is the end-to-end shape of
// #24 and #53 together. Two real worktrees, each completing its own milestone
// and adding a follow-up with an indented body, merged one after the other.
func TestRealWavePreservesBothSiblingsAndTheirBodies(t *testing.T) {
	root := realRepo(t, realWaveDoc)

	type wt struct {
		path, branch, milestone string
	}
	wts := []wt{
		{filepath.Join(t.TempDir(), "m1"), "belmont/auto/demo-m1", "M1"},
		{filepath.Join(t.TempDir(), "m2"), "belmont/auto/demo-m2", "M2"},
	}

	// Create both worktrees the way the wave does, through the real entry point.
	for _, w := range wts {
		if err := createWorktreeIfNeeded(root, w.path, w.branch, "demo", false); err != nil {
			t.Fatalf("createWorktreeIfNeeded(%s): %v", w.milestone, err)
		}
		git(t, w.path, "config", "user.email", "test@example.com")
		git(t, w.path, "config", "user.name", "Test")
	}

	// The isolation this whole entry depends on must actually be armed, or the
	// rest of the test proves nothing about the unrecoverable case.
	for _, w := range wts {
		out := git(t, w.path, "ls-files", "-v", ".belmont")
		if !strings.Contains(out, "h ") {
			t.Fatalf("%s: --assume-unchanged is NOT armed on .belmont/ — this test would "+
				"pass for the wrong reason.\ngit ls-files -v:\n%s", w.milestone, out)
		}
	}

	// Each worktree does its milestone's work and files a follow-up WITH a body.
	edits := map[string]string{
		"M1": `# Progress: demo

## Milestones

### M1: Parsing
- [v] P0-M1-1: Parse the header
  - [ ] P0-M1-1a: Handle the BOM
    **Verification**: byte-order mark stripped before parse
- [v] P0-M1-2: Parse the body

### M2: Rendering
- [ ] P0-M2-1: Render the header
- [ ] P0-M2-2: Render the body
`,
		"M2": `# Progress: demo

## Milestones

### M1: Parsing
- [ ] P0-M1-1: Parse the header
- [ ] P0-M1-2: Parse the body

### M2: Rendering
- [v] P0-M2-1: Render the header
- [v] P0-M2-2: Render the body
  **Evidence**: commit feedfac
`,
	}
	for _, w := range wts {
		p := filepath.Join(w.path, ".belmont", "features", "demo", "PROGRESS.md")
		if err := os.WriteFile(p, []byte(edits[w.milestone]), 0o644); err != nil {
			t.Fatal(err)
		}
		// A real code change too, so the branch has something git will merge.
		if err := os.WriteFile(filepath.Join(w.path, w.milestone+".txt"), []byte(w.milestone+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := commitWorktreeChanges(w.path, w.milestone); err != nil {
			t.Fatalf("commitWorktreeChanges(%s): %v", w.milestone, err)
		}
	}

	// Merge them into main in sequence, syncing state after each — the exact
	// order runWaveParallel's merge loop uses.
	for _, w := range wts {
		git(t, root, "merge", "--no-ff", "-q", "-m", "merge "+w.milestone, w.branch)
		syncFeatureStateAfterMerge(root, w.path, "demo")
	}

	final := readFile(t, filepath.Join(root, ".belmont", "features", "demo", "PROGRESS.md"))
	t.Logf("final master PROGRESS.md after both merges:\n%s", final)

	// #24: the second merge must not revert the first.
	for _, want := range []string{
		"[v] P0-M1-1", "[v] P0-M1-2", "[v] P0-M2-1", "[v] P0-M2-2",
	} {
		if !strings.Contains(final, want) {
			t.Errorf("sibling completion lost across the wave: %q missing.\n%s", want, final)
		}
	}

	// #53: both follow-ups arrived, each with its own body, and the nested one
	// is still nested under its parent.
	if !strings.Contains(final, "P0-M1-1a") {
		t.Fatalf("the nested follow-up never arrived:\n%s", final)
	}
	if !strings.Contains(final, "byte-order mark stripped before parse") {
		t.Errorf("the nested follow-up lost its **Verification** body:\n%s", final)
	}
	if !strings.Contains(final, "**Evidence**: commit feedfac") {
		t.Errorf("M2's evidence body did not survive the wave:\n%s", final)
	}

	lines := strings.Split(final, "\n")
	at := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	child, parent, next := at("P0-M1-1a"), at("P0-M1-1:"), at("P0-M1-2:")
	if child < 0 || parent < 0 || lineIndentWidth(lines[child]) <= lineIndentWidth(lines[parent]) {
		t.Errorf("the nested follow-up is no longer nested under P0-M1-1 after a real merge.\n%s", final)
	}
	// Its parent is deliberately NOT the last task in its milestone, so a carry
	// anchored at the milestone's end lands it AFTER P0-M1-2 and re-parents it.
	// With the parent last, the milestone anchor is accidentally correct and the
	// placement defect hides.
	if next >= 0 && child > next {
		t.Errorf("the follow-up was carried past P0-M1-2 to the milestone's end, re-parenting it.\n%s", final)
	}

	// No ID may appear twice — a duplicate poisons every later merge.
	for _, id := range []string{"P0-M1-1a", "P0-M1-2", "P0-M2-1", "P0-M2-2"} {
		if n := strings.Count(final, id+":"); n != 1 {
			t.Errorf("%s appears %d times after the wave, want 1.\n%s", id, n, final)
		}
	}

	// And the document the wave produced must be one `belmont validate` accepts,
	// or the next run refuses to start on it.
	v, vErr := validateFeature(root, "demo")
	if vErr != nil {
		t.Fatalf("validateFeature on the merged document: %v", vErr)
	}
	for _, one := range v {
		t.Errorf("the merged document does not validate: [%s/%s] %s", one.Severity, one.Rule, one.Message)
	}
}

// TestRecoverRefusesTheRealLiveWorktree drives the #52 guard against worktrees
// that genuinely exist as git worktrees, with auto.json written by the tracker
// rather than by hand.
func TestRecoverRefusesTheRealLiveWorktree(t *testing.T) {
	root := realRepo(t, realWaveDoc)
	t.Setenv("HOME", t.TempDir())

	wtPath := filepath.Join(worktreeBasePath(root), "demo-m1")
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createWorktreeIfNeeded(root, wtPath, "belmont/auto/demo-m1", "demo", false); err != nil {
		t.Fatalf("createWorktreeIfNeeded: %v", err)
	}

	// auto.json exactly as the tracker writes it during a live wave.
	tracker := &worktreeTracker{
		root:    root,
		feature: "demo",
		mode:    "single-feature-parallel",
		entries: map[string]worktreeEntry{},
	}
	tracker.add("M1", wtPath, "belmont/auto/demo-m1")

	if err := runRecover([]string{"--root", root, "--clean-all"}); err == nil {
		t.Error("recover --clean-all deleted a REAL live worktree")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("the real live worktree was removed: %v", err)
	}
	out := captureStdout(t, func() {
		if err := runRecover([]string{"--root", root, "--list"}); err != nil {
			t.Fatalf("recover --list refused: %v", err)
		}
	})
	t.Logf("recover --list against a real live worktree:\n%s", out)
	if !strings.Contains(out, "still in flight") {
		t.Errorf("the real live worktree was not marked in flight.\n%s", out)
	}
}
