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
