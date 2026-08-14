package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Issue #46 — the two merge paths must agree about a blocker
// ---------------------------------------------------------------------------

// `mergeProgressState` lets `[!]` win from either side. `resolveProgressConflict`
// kept ours, so a blocker the incoming side raised was discarded whenever this
// side held a more advanced state — and which of the two functions runs depends
// only on whether git happened to register a conflict.
//
// `.belmont/` is `--assume-unchanged` inside a worktree, so the discarded state
// is in no commit and unrecoverable. Losing a blocker looks exactly like the
// question was answered.
func TestConflictResolverKeepsBlockersFromEitherSide(t *testing.T) {
	for _, tc := range []struct{ ours, theirs, want string }{
		{"x", "!", "!"}, // the direction that was broken: an incoming blocker lost
		{"v", "!", "!"}, // …including against the strongest claim Belmont makes
		{">", "!", "!"},
		{" ", "!", "!"},
		{"!", "x", "!"}, // the direction that already worked, kept honest
		{"!", "v", "!"},
		{"!", " ", "!"},
	} {
		base := "### M1: Work\n- [>] P1-M1-1: retry logic, as first written\n- [ ] P1-M1-2: other\n"
		doc := func(marker string) string {
			return "### M1: Work\n- [" + marker + "] P1-M1-1: retry logic\n- [ ] P1-M1-2: other\n"
		}
		root, rel := conflictFixture(t, base, doc(tc.ours), doc(tc.theirs))
		if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
			t.Errorf("ours=[%s] theirs=[%s]: the resolver declined a file it should have resolved", tc.ours, tc.theirs)
			continue
		}
		got := readFile(t, filepath.Join(root, rel))
		if !strings.Contains(got, "- ["+tc.want+"] P1-M1-1") {
			t.Errorf("ours=[%s] theirs=[%s] resolved to something other than [%s] — "+
				"the two merge paths must agree that a blocker wins from either direction:\n%s",
				tc.ours, tc.theirs, tc.want, got)
		}
	}
}

// A withdrawal still beats a blocker, from both directions. The blocked fix must
// not reorder the two decisions: an unblocked route through cancelled work is not
// something anyone needs (#27's rule, applied to `[-]`).
func TestConflictResolverStillChecksWithdrawnBeforeBlocked(t *testing.T) {
	for _, tc := range []struct{ ours, theirs, want string }{
		{"-", "!", "-"},
		{"!", "-", "-"},
	} {
		base := "### M1: Work\n- [>] P1-M1-1: retry logic, as first written\n- [ ] P1-M1-2: other\n"
		doc := func(marker string) string {
			return "### M1: Work\n- [" + marker + "] P1-M1-1: retry logic\n- [ ] P1-M1-2: other\n"
		}
		root, rel := conflictFixture(t, base, doc(tc.ours), doc(tc.theirs))
		if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
			t.Errorf("ours=[%s] theirs=[%s]: the resolver declined", tc.ours, tc.theirs)
			continue
		}
		if got := readFile(t, filepath.Join(root, rel)); !strings.Contains(got, "- ["+tc.want+"] P1-M1-1") {
			t.Errorf("ours=[%s] theirs=[%s] resolved to something other than [%s]:\n%s",
				tc.ours, tc.theirs, tc.want, got)
		}
	}
}

// A decision winning is the rule. A decision winning SILENTLY is the bug —
// `mergeProgressState` reports the state it displaced in all four such
// directions, and this resolver reported none of them.
func TestConflictResolverReportsWhatADecisionDisplaced(t *testing.T) {
	for _, tc := range []struct {
		ours, theirs string
		wantWarning  bool
	}{
		{"x", "!", true},  // ours' completion is being discarded
		{"!", "x", true},  // theirs' completion is being discarded
		{"v", "-", true},  // …and a withdrawal displacing a verification
		{"-", "v", true},  //    from either side
		{" ", "!", false}, // nothing lost — do not cry wolf
		{"!", " ", false},
		{" ", "-", false},
		{"-", " ", false},
		{"!", "!", false}, // both sides agree
		{"-", "-", false},
	} {
		base := "### M1: Work\n- [>] P1-M1-1: retry logic, as first written\n- [ ] P1-M1-2: other\n"
		// The two sides differ in TEXT as well as marker, so a row where both
		// markers are the same still produces a genuine conflict — otherwise the
		// "both sides agree, say nothing" row merges cleanly and the resolver
		// under test never runs.
		doc := func(marker, text string) string {
			return "### M1: Work\n- [" + marker + "] P1-M1-1: " + text + "\n- [ ] P1-M1-2: other\n"
		}
		root, rel := conflictFixture(t, base, doc(tc.ours, "retry logic"), doc(tc.theirs, "retry logic, revised"))
		var resolved bool
		stderr := captureStderr(t, func() {
			resolved = resolveProgressConflict(root, rel, filepath.Join(root, rel))
		})
		if !resolved {
			t.Errorf("ours=[%s] theirs=[%s]: the resolver declined", tc.ours, tc.theirs)
			continue
		}
		got := strings.Contains(stderr, "P1-M1-1")
		if got != tc.wantWarning {
			t.Errorf("ours=[%s] theirs=[%s]: warned=%v, want %v — state was dropped without telling anyone.\nstderr: %s",
				tc.ours, tc.theirs, got, tc.wantWarning, stderr)
		}
	}
}

// ---------------------------------------------------------------------------
// Issue #47 — a task bullet nested inside another task's body is a real task
// ---------------------------------------------------------------------------

// #43's carry keyed on TOP-LEVEL bullets only: a nested one travelled solely
// inside its parent's block, so when the parent already existed here the nested
// child that only the incoming side had was dropped entirely. A nested task is a
// real task everywhere else in Belmont — `parseMilestones` counts it,
// `taskBodyEnd` bounds it, `moveTaskLines` moves it — so the merge treating it as
// invisible was the odd one out.
func TestCarriesANestedTaskWhoseParentAlreadyExistsHere(t *testing.T) {
	base := "### M1: Work\n- [>] P1-M1-1: parent\n"
	ours := "### M1: Work\n- [x] P1-M1-1: parent, built\n"
	theirs := "### M1: Work\n- [x] P1-M1-1: parent, built and logged\n" +
		"  - [!] P1-M1-2: nested — needs a person\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined a file it should have resolved")
	}
	got := readFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, "P1-M1-2") {
		t.Fatalf("a nested task present only in the incoming version was dropped under a green auto-resolve:\n%s", got)
	}
	if !strings.Contains(got, "[!] P1-M1-2") {
		t.Errorf("the carried nested task lost its marker — losing a [!] looks exactly like the question was answered:\n%s", got)
	}
	// It has to arrive as a task the parser sees, under the right milestone.
	var found bool
	for _, m := range parseMilestones(got) {
		for _, task := range m.Tasks {
			if task.ID != "P1-M1-2" {
				continue
			}
			found = true
			if m.ID != "M1" {
				t.Errorf("carried nested task parsed under %s, not M1:\n%s", m.ID, got)
			}
			if task.Status != taskBlocked {
				t.Errorf("carried nested task parsed as %v, not blocked:\n%s", task.Status, got)
			}
		}
	}
	if !found {
		t.Errorf("the carried line is in the file but parses as no task at all:\n%s", got)
	}
}

// Placement is under the parent it was written under, not appended to the end of
// the milestone: a nested bullet's meaning is positional, and re-parenting it to
// whatever task happens to sit last is the #33 mis-attribution in a new place.
func TestCarriedNestedTaskLandsUnderItsParent(t *testing.T) {
	base := "### M1: Work\n- [>] P1-M1-1: parent\n- [ ] P1-M1-7: a later sibling\n"
	ours := "### M1: Work\n- [x] P1-M1-1: parent, built\n- [ ] P1-M1-7: a later sibling\n"
	theirs := "### M1: Work\n- [x] P1-M1-1: parent, built and logged\n" +
		"  - [!] P1-M1-2: nested under the parent\n- [ ] P1-M1-7: a later sibling\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined")
	}
	got := readFile(t, filepath.Join(root, rel))
	lines := strings.Split(got, "\n")
	idxOf := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	parent, child, sibling := idxOf("P1-M1-1"), idxOf("P1-M1-2"), idxOf("P1-M1-7")
	if child == -1 {
		t.Fatalf("nested task dropped:\n%s", got)
	}
	if child < parent || (sibling != -1 && child > sibling) {
		t.Errorf("carried nested task landed outside its parent's body (parent=%d child=%d sibling=%d):\n%s",
			parent, child, sibling, got)
	}
	if !strings.HasPrefix(lines[child], " ") && !strings.HasPrefix(lines[child], "\t") {
		t.Errorf("carried nested task arrived unindented, so it is no longer nested:\n%s", got)
	}
}

// The nested bullet must not be carried twice: when its parent is itself carried,
// the child travels inside the parent's block. A duplicate ID is durable damage —
// `countIDs` in mergeProgressState refuses to reconcile that task on every later
// merge and tells the user to de-duplicate by hand.
func TestNestedTaskIsNotCarriedTwiceWhenItsParentIsAlsoCarried(t *testing.T) {
	base := "### M1: Work\n- [>] P1-M1-1: anchor\n"
	ours := "### M1: Work\n- [x] P1-M1-1: anchor, built\n"
	theirs := "### M1: Work\n- [x] P1-M1-1: anchor, built and logged\n" +
		"- [ ] P1-M1-8: new parent\n  - [!] P1-M1-9: new nested child\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined")
	}
	got := readFile(t, filepath.Join(root, rel))
	for _, id := range []string{"P1-M1-8", "P1-M1-9"} {
		if n := strings.Count(got, id+":"); n != 1 {
			t.Errorf("%s appears %d times, want 1 — a duplicated ID makes every later merge refuse to reconcile it:\n%s", id, n, got)
		}
	}
}

// Structure disagreement stays a reconciliation-agent question. If this side
// files the parent under a different milestone from the one the incoming child
// was written in, placing the child under the parent silently moves it — and a
// task whose ID names another milestone is `cross_milestone_task_id`,
// severityError.
func TestCarryDeclinesWhenTheParentSitsInADifferentMilestoneHere(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: parent\n\n### M2: Two\n- [ ] P1-M2-1: other\n"
	ours := "### M1: One\n- [ ] P1-M1-5: unrelated\n\n### M2: Two\n- [x] P1-M1-1: parent, filed under M2 here\n"
	theirs := "### M1: One\n- [x] P1-M1-1: parent, still under M1\n  - [!] P1-M1-2: nested child\n\n### M2: Two\n- [ ] P1-M2-1: other\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Errorf("resolver silently moved a carried nested task into another milestone rather than escalating:\n%s",
			readFile(t, filepath.Join(root, rel)))
	}
}
