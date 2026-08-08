package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// PR #28 review follow-up. parseMilestones is not the only thing that reads a
// checkbox marker: the commit-evidence guard, the merge-conflict resolver and
// `belmont reverify` each interpret raw markers too. When they disagreed with
// the parser the failures were silent, so these tests pin the agreement rather
// than the individual behaviours.
//
// The original defect: `[V]` parsed as taskVerified while every raw-marker
// reader compared against a literal lowercase "v", so a `[V]` flip counted as
// verified everywhere and was invisible to runEvidenceCheck.

// canonicalMarker is the single source of truth. Anything it does not
// recognise must be taskUnknown, and nothing it does not recognise may be
// treated as a state by any reader.
func TestCanonicalMarkerContract(t *testing.T) {
	known := map[string]taskStatus{
		" ": taskTodo,
		">": taskInProgress,
		"x": taskDone,
		"X": taskDone, // GitHub renders a capital X as checked
		"v": taskVerified,
		"!": taskBlocked,
	}
	for m, want := range known {
		got, ok := canonicalMarker(m)
		if !ok || got != want {
			t.Errorf("canonicalMarker(%q) = (%v, %v), want (%v, true)", m, got, ok, want)
		}
	}
	// [V] is deliberately absent: no external convention emits it, and
	// accepting it cost enforcement. See canonicalMarker's doc comment.
	for _, m := range []string{"V", "?", "-", "~", "→", "", "y"} {
		got, ok := canonicalMarker(m)
		if ok || got != taskUnknown {
			t.Errorf("canonicalMarker(%q) = (%v, %v), want (taskUnknown, false)", m, got, ok)
		}
	}
}

// THE control for the original bypass. Every marker the parser calls verified
// must also be seen as verified by the evidence guard, which works on raw
// markers. Re-adding "V" to canonicalMarker without teaching the guard about
// it fails here.
func TestEveryVerifiedMarkerIsVisibleToEvidenceGuard(t *testing.T) {
	root := t.TempDir()
	git := func(a ...string) {
		c := exec.Command("git", a...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t.t")
	git("config", "user.name", "t")
	os.WriteFile(filepath.Join(root, "f.txt"), []byte("a"), 0644)
	git("add", "-A")
	// Deliberately no task ID in the message: the guard must find no evidence.
	git("commit", "-q", "-m", "work with no task attribution")

	// Every printable ASCII marker the parser accepts as verified.
	var verifiedMarkers []string
	for c := 32; c < 127; c++ {
		m := string(rune(c))
		if st, ok := canonicalMarker(m); ok && st == taskVerified {
			verifiedMarkers = append(verifiedMarkers, m)
		}
	}
	if len(verifiedMarkers) == 0 {
		t.Fatal("no marker maps to taskVerified — fixture is wrong")
	}

	pre := parseProgressSnapshot("P", "### M1: M\n- [x] P1-M1-1: alpha\n")
	for _, m := range verifiedMarkers {
		post := parseProgressSnapshot("P", "### M1: M\n- ["+m+"] P1-M1-1: alpha\n")
		missing := findEvidenceMissingFlips(root, pre, post, "M1")
		if len(missing) != 1 {
			t.Errorf("marker [%s] parses as verified but findEvidenceMissingFlips flagged %d flips, want 1 — "+
				"a verified state invisible to the guard is a silent bypass of the evidence contract", m, len(missing))
			continue
		}
		// And the revert must actually rewrite the line, not silently no-op.
		rebuilt := revertEvidenceMissing(post, pre, missing)
		if strings.Contains(rebuilt, "["+m+"]") {
			t.Errorf("marker [%s]: revertEvidenceMissing left the line untouched — the guard logs a revert it did not perform", m)
		}
	}
}

// reverify selects milestones by parsed status but rewrites by raw marker.
// A rewriter that recognises fewer spellings than the parser silently no-ops:
// the milestone is selected for reset and then nothing is reset.
func TestReverifyResetsEveryVerifiedMarker(t *testing.T) {
	// Guard against a vacuous pass: if canonicalMarker ever stopped calling
	// anything verified, the loop below would run zero times and report success.
	ran := 0
	defer func() {
		if ran == 0 {
			t.Error("no marker maps to taskVerified — this test asserted nothing")
		}
	}()
	for c := 32; c < 127; c++ {
		m := string(rune(c))
		if st, ok := canonicalMarker(m); !ok || st != taskVerified {
			continue
		}
		ran++
		body := "# Progress\n\n## Milestones\n\n### M1: M\n- [" + m + "] P1-M1-1: alpha\n"

		// This is how runReverifyCmd builds resetIDs: from parsed status.
		ms := parseMilestones(body)
		resetIDs := map[string]bool{}
		for _, mi := range ms {
			for _, tk := range mi.Tasks {
				if tk.Status == taskVerified {
					resetIDs[mi.ID] = true
				}
			}
		}
		if !resetIDs["M1"] {
			t.Fatalf("marker [%s] parses as verified but did not select its milestone for reset", m)
		}

		got, changed := resetVerifiedTasks(body, resetIDs)
		if !changed {
			t.Errorf("marker [%s]: milestone selected for reset but resetVerifiedTasks changed nothing — "+
				"reverify would report \"nothing to re-verify\" on a fully verified file", m)
			continue
		}
		if !strings.Contains(got, "- [x] P1-M1-1: alpha") {
			t.Errorf("marker [%s]: reset produced %q, want the task at [x]", m, strings.TrimSpace(got))
		}
	}
}

// A milestone with no verified tasks must not be rewritten at all.
func TestReverifyLeavesUnverifiedMarkersAlone(t *testing.T) {
	body := "# Progress\n\n## Milestones\n\n### M1: M\n- [x] P1-M1-1: a\n- [ ] P1-M1-2: b\n- [?] P1-M1-3: c\n"
	got, changed := resetVerifiedTasks(body, map[string]bool{"M1": true})
	if changed || got != body {
		t.Errorf("resetVerifiedTasks rewrote a milestone with no verified tasks:\n%s", got)
	}
}

// The merge resolver must rank by canonical state. A raw-marker map ranked
// anything it lacked a key for at 0 — tied with todo, below in-progress — so
// [X] (done) lost to [>].
func TestMergePriorityRanksCanonicalStates(t *testing.T) {
	cases := []struct{ ours, theirs, want string }{
		{"X", ">", "X"}, // done must not be downgraded to in-progress
		{" ", "X", "X"}, // a completion recorded on the other side must arrive
		{" ", "x", "x"}, // control: the ordinary case
		{"x", " ", "x"},
		{"x", "v", "v"},
		{"!", "x", "!"}, // blocked is deliberately preserved from ours
	}
	for _, c := range cases {
		got := resolveConflictFixture(t, "- ["+c.ours+"] P1-M1-1: alpha", "- ["+c.theirs+"] P1-M1-1: alpha")
		if !strings.Contains(got, "["+c.want+"]") {
			t.Errorf("ours=[%s] theirs=[%s] => %q, want marker [%s]", c.ours, c.theirs, got, c.want)
		}
	}
}

// An unrecognised marker must abort auto-resolution entirely. Normalising it
// away and committing under a green "conflicts auto-resolved" destroys the
// signal the whole taskUnknown feature exists to raise.
func TestMergeRefusesToResolveUnrecognisedMarker(t *testing.T) {
	for _, c := range []struct{ ours, theirs string }{
		{"?", "x"}, // unrecognised on ours
		{"x", "?"}, // unrecognised on theirs
		{"V", "x"}, // the capital-V case specifically
	} {
		if resolveConflictResolved(t, "- ["+c.ours+"] P1-M1-1: alpha", "- ["+c.theirs+"] P1-M1-1: alpha") {
			t.Errorf("ours=[%s] theirs=[%s]: resolver reported success — it must bail out so the "+
				"conflict escalates to the reconciliation agent", c.ours, c.theirs)
		}
	}
	// An ID-less task line is skipped by the resolver's own taskRe, so the
	// bail-out scan has to be broader than that regex.
	if resolveConflictResolved(t, "- [?] a task with no id", "- [x] a task with no id") {
		t.Error("ID-less line with an unrecognised marker: resolver reported success, want bail-out")
	}
}

// The status JSON is the only place an agent can learn that an unreadable entry
// exists — Marker/Line are json:"-", so the `unknown` bucket and the per-task
// Status string are the whole signal. Nothing drove buildStatus before, so both
// could be dropped by a payload-shrink pass with a green suite. PR #26 exists to
// shrink exactly this payload; this test is what makes that regression fail CI.
func TestBuildStatusExposesUnknownMarkers(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# PRD: Demo\n"), 0644)
	os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(
		"# Progress: Demo\n\n## Milestones\n\n### M1: M\n"+
			"- [x] P1-M1-1: done\n- [?] P1-M1-2: withdrawn\n- [-] P1-M1-3: cancelled\n"), 0644)

	report, err := buildStatus(root, 0, "demo")
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}
	if got := report.TaskCounts["unknown"]; got != 2 {
		t.Errorf(`TaskCounts["unknown"] = %d, want 2 — this key is how a JSON consumer learns an unreadable entry exists`, got)
	}
	if got := report.TaskCounts["todo"]; got != 0 {
		t.Errorf(`TaskCounts["todo"] = %d, want 0 — unknown markers must never be folded into todo`, got)
	}
	// The buckets must still account for every task.
	sum := 0
	for k, v := range report.TaskCounts {
		if k != "total" {
			sum += v
		}
	}
	if sum != report.TaskCounts["total"] {
		t.Errorf("task buckets sum to %d but total = %d — a consumer computing completion will be wrong", sum, report.TaskCounts["total"])
	}
	// Per-task Status is the only per-entry locator left in JSON.
	unknowns := 0
	for _, tk := range report.Tasks {
		if tk.Status == taskUnknown {
			unknowns++
		}
	}
	if unknowns != 2 {
		t.Errorf("tasks with Status=unknown = %d, want 2", unknowns)
	}
}

// buildConflict stages a real merge conflict on PROGRESS.md and runs the real
// resolver. Returns (resolved, merged task line).
func buildConflict(t *testing.T, oursLine, theirsLine string) (bool, string) {
	t.Helper()
	d := t.TempDir()
	rel := filepath.Join(".belmont", "features", "demo", "PROGRESS.md")
	abs := filepath.Join(d, rel)
	os.MkdirAll(filepath.Dir(abs), 0755)

	git := func(a ...string) {
		c := exec.Command("git", a...)
		c.Dir = d
		if out, err := c.CombinedOutput(); err != nil && a[0] != "merge" {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	write := func(line string) {
		os.WriteFile(abs, []byte("# Progress\n\n## Milestones\n\n### M1: M\n"+line+"\n"), 0644)
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t.t")
	git("config", "user.name", "t")
	write("- [ ] P1-M1-1: BASE")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "side")
	write(theirsLine)
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "main")
	write(oursLine)
	git("commit", "-qam", "ours")
	git("merge", "side") // conflicts

	resolved := resolveProgressConflict(d, rel, abs)
	b, _ := os.ReadFile(abs)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "P1-M1-1") || strings.Contains(l, "no id") {
			return resolved, strings.TrimSpace(l)
		}
	}
	return resolved, ""
}

func resolveConflictFixture(t *testing.T, ours, theirs string) string {
	t.Helper()
	resolved, line := buildConflict(t, ours, theirs)
	if !resolved {
		t.Fatalf("resolver declined a fixture of known markers: ours=%q theirs=%q", ours, theirs)
	}
	return line
}

func resolveConflictResolved(t *testing.T, ours, theirs string) bool {
	t.Helper()
	resolved, _ := buildConflict(t, ours, theirs)
	return resolved
}
