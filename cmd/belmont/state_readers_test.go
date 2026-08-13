package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Issue #39 — one answer to "which milestone does this task ID name"
// ---------------------------------------------------------------------------

// detectFwlupTasksForMilestone used to carry its own `P\d+-M(\d+)` regex,
// unanchored and blind to `FWLUP-`. It disagreed with taskIDMilestoneRefRe —
// the definition `belmont validate` reports against and `belmont repair` moves
// toward — in both directions at once.
func TestFwlupMilestoneAttributionAgreesWithTheLint(t *testing.T) {
	for _, tc := range []struct {
		name     string
		taskID   string
		filedIn  string
		asked    string
		want     bool
		wasWrong string
	}{
		{
			name:     "FWLUP infix is attributed, as the lint already believes",
			taskID:   "P1-FWLUP-M2-3",
			filedIn:  "M9",
			asked:    "M2",
			want:     true,
			wasWrong: "the old regex required P<n>-M and FWLUP- sits between, so it returned \"\" and this task belonged to no milestone here while validate put it in M2",
		},
		{
			name:     "an ID embedded mid-token is NOT attributed",
			taskID:   "SWEEP-P1-M4-2",
			filedIn:  "M9",
			asked:    "M4",
			want:     false,
			wasWrong: "the old regex was unanchored, so it found P1-M4 anywhere in a longer ID and claimed a milestone the anchored definition does not",
		},
		{
			name:    "the ordinary shape is unchanged",
			taskID:  "P1-M5-FWLUP-1",
			filedIn: "M9",
			asked:   "M5",
			want:    true,
		},
		{
			name:    "position still wins when it agrees",
			taskID:  "P1-M3-1",
			filedIn: "M3",
			asked:   "M3",
			want:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := statusReport{Tasks: []task{{
				ID:          tc.taskID,
				Name:        "FWLUP: a follow-up",
				Status:      taskTodo,
				MilestoneID: tc.filedIn,
			}}}
			got := detectFwlupTasksForMilestone("", "", report, tc.asked)
			if got != tc.want {
				t.Errorf("detectFwlupTasksForMilestone(%s filed in %s, asked %s) = %v, want %v\n%s",
					tc.taskID, tc.filedIn, tc.asked, got, tc.want, tc.wasWrong)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Issue #40 — skipping a milestone must not answer a question asked of a person
// ---------------------------------------------------------------------------

func TestSkipMilestoneLeavesBlockedTasksAlone(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "auth")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	progress := `# Progress

### M1: Work
- [ ] P1-M1-1: todo becomes done
- [>] P1-M1-2: in progress becomes done
- [!] P1-M1-3: rotate the credentials — waiting on a person
- [v] P1-M1-4: already verified, untouched

### M2: Other
- [ ] P1-M2-1: a different milestone, untouched
`
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(progress), 0644); err != nil {
		t.Fatal(err)
	}

	blockersLeft, err := skipMilestoneInProgress(root, "auth", "M1")
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if blockersLeft != 1 {
		t.Errorf("blockersLeft = %d, want 1 — the caller cannot warn about what it is not told", blockersLeft)
	}

	got := readFile(t, filepath.Join(dir, "PROGRESS.md"))
	for _, want := range []string{
		"- [x] P1-M1-1",
		"- [x] P1-M1-2",
		"- [!] P1-M1-3", // the whole point
		"- [v] P1-M1-4",
		"- [ ] P1-M2-1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q after skip:\n%s", want, got)
		}
	}
	if strings.Contains(got, "- [x] P1-M1-3") {
		t.Errorf("a blocked task was laundered to [x]. It is a question for the user, and [x] reads as finished to every stop condition:\n%s", got)
	}
}

// A milestone whose only outstanding work is blocked has nothing to skip. It
// must not report success, because success here means "this milestone is done".
func TestSkipMilestoneRefusesWhenOnlyBlockedRemains(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "auth")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	progress := "### M1: Work\n- [!] P1-M1-1: waiting on a person\n- [v] P1-M1-2: done\n"
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(progress), 0644); err != nil {
		t.Fatal(err)
	}

	blockersLeft, err := skipMilestoneInProgress(root, "auth", "M1")
	if err == nil {
		t.Fatal("skip reported success on a milestone whose only pending task is blocked")
	}
	if blockersLeft != 1 {
		t.Errorf("blockersLeft = %d, want 1", blockersLeft)
	}
	if !strings.Contains(err.Error(), "waiting on a person") {
		t.Errorf("error does not say why there was nothing to skip: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "PROGRESS.md")); got != progress {
		t.Errorf("the file was rewritten despite nothing to skip:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Issue #38 — a hand-written ID must reconcile across a worktree merge
// ---------------------------------------------------------------------------

func TestMergeProgressStateReconcilesHandWrittenTaskIDs(t *testing.T) {
	master := "### M1: Work\n- [x] FWLUP-SWEEP-1: a cross-cutting follow-up\n- [x] P1-M1-1: an ordinary task\n"
	worktree := "### M1: Work\n- [ ] FWLUP-SWEEP-1: a cross-cutting follow-up\n- [ ] P1-M1-1: an ordinary task\n"

	got, _ := mergeProgressState(master, worktree)

	if !strings.Contains(got, "- [x] FWLUP-SWEEP-1") {
		t.Errorf("a hand-written ID was not reconciled — it took the worktree's stale [ ]. `.belmont/` is --assume-unchanged, so this is the only transport home:\n%s", got)
	}
	if !strings.Contains(got, "- [x] P1-M1-1") {
		t.Errorf("the P<n>- path regressed; that alternative must behave exactly as before:\n%s", got)
	}
}

// A hand-written ID is now ranked, so withdrawal must still win from either
// side — the property that made #27 expensive when a marker silently changed
// meaning.
func TestMergeProgressStateKeepsWithdrawalOnHandWrittenIDs(t *testing.T) {
	master := "### M1: Work\n- [-] FWLUP-SWEEP-1: dropped deliberately\n"
	worktree := "### M1: Work\n- [ ] FWLUP-SWEEP-1: dropped deliberately\n"

	got, _ := mergeProgressState(master, worktree)
	if !strings.Contains(got, "- [-] FWLUP-SWEEP-1") {
		t.Errorf("widening the ID shape revived cancelled work:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Issue #43 — a task only "theirs" has must not be dropped
// ---------------------------------------------------------------------------

// conflictFixture builds a repo whose PROGRESS.md genuinely conflicts, with
// `ours` on main and `theirs` on the merged-in branch.
func conflictFixture(t *testing.T, base, ours, theirs string) (root, rel string) {
	t.Helper()
	root = t.TempDir()
	rel = "PROGRESS.md"
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@t.t")
	gitRun(t, root, "config", "user.name", "t")

	write := func(content, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, root, "add", "-A")
		gitRun(t, root, "commit", "-q", "-m", msg)
	}
	write(base, "base")
	gitRun(t, root, "checkout", "-q", "-b", "sib")
	write(theirs, "sibling side")
	gitRun(t, root, "checkout", "-q", "main")
	write(ours, "main side")

	merge := exec.Command("git", "merge", "--no-commit", "sib")
	merge.Dir = root
	if out, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("fixture merged cleanly — the resolver under test never runs\n%s", out)
	}
	return root, rel
}

func TestConflictResolverCarriesTasksPresentOnlyInTheirs(t *testing.T) {
	base := "### M1: Work\n- [>] P1-M1-1: as first written\n"
	ours := "### M1: Work\n- [x] P1-M1-1: build the weekly roll-up\n"
	theirs := "### M1: Work\n- [x] P1-M1-1: build the weekly roll-up\n- [!] P1-M1-2: rotate the reporting credentials — needs console access\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined a file it should have resolved")
	}

	got := readFile(t, filepath.Join(root, rel))
	if !strings.Contains(got, "P1-M1-2") {
		t.Errorf("a task present only in the incoming version was dropped under a green auto-resolve. `.belmont/` is --assume-unchanged, so it is in no commit and unrecoverable:\n%s", got)
	}
	if !strings.Contains(got, "- [!] P1-M1-2") {
		t.Errorf("the carried task lost its marker — losing a [!] looks exactly like the question was answered:\n%s", got)
	}
	if !strings.Contains(got, "- [x] P1-M1-1") {
		t.Errorf("the existing task regressed:\n%s", got)
	}
}

// The carried line goes inside the list, after the last content line of its
// milestone — not between a task and its own indented body (the #33 stranding),
// and not past the blank line that ends the block.
func TestConflictResolverPlacesCarriedTaskInsideItsMilestone(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: as first written\n\n### M2: Two\n- [ ] P1-M2-1: other\n"
	ours := "### M1: One\n- [x] P1-M1-1: first\n  **Evidence**: commit abc123\n\n### M2: Two\n- [ ] P1-M2-1: other\n"
	theirs := "### M1: One\n- [x] P1-M1-1: first\n  **Evidence**: commit abc123\n- [ ] P1-M1-9: carried\n\n### M2: Two\n- [ ] P1-M2-1: other\n"

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
	carried, evidence, m2 := idxOf("P1-M1-9"), idxOf("**Evidence**"), idxOf("### M2:")
	if carried == -1 {
		t.Fatalf("carried task missing:\n%s", got)
	}
	if carried < evidence {
		t.Errorf("carried task was spliced between P1-M1-1 and its own body, stranding the evidence:\n%s", got)
	}
	if m2 != -1 && carried > m2 {
		t.Errorf("carried task landed outside M1, in or past M2:\n%s", got)
	}
}

// Structure disagreement is a reconciliation-agent question. Appending the task
// to a plausible-looking block would be the silent normalisation of #27.
func TestConflictResolverDeclinesWhenCarriedTaskHasNoMilestoneHere(t *testing.T) {
	base := "### M1: Work\n- [>] P1-M1-1: as first written\n"
	ours := "### M1: Work\n- [x] P1-M1-1: build it\n"
	theirs := "### M1: Work\n- [x] P1-M1-1: build it\n\n### M7: Added elsewhere\n- [!] P1-M7-1: a question filed in a milestone this side never had\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Errorf("resolver auto-resolved a file whose sides disagree about milestone structure; it must escalate:\n%s",
			readFile(t, filepath.Join(root, rel)))
	}
}

// ---------------------------------------------------------------------------
// Issue #42 — the listing must not collapse a parallel run to one worktree
// ---------------------------------------------------------------------------

func TestFeatureListingOverlaysEachMilestoneFromItsOwnWorktree(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, ".belmont", "features")
	masterDir := filepath.Join(featuresDir, "auth")
	if err := os.MkdirAll(masterDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeProgress := func(dir, content string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Master is the fork-point baseline: nothing done yet.
	writeProgress(masterDir, "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n")

	// Two milestone worktrees, each having advanced only its own milestone.
	// Each carries a STALE copy of the other's, which is the trap: reading one
	// worktree as the whole feature reports the other's stale state as current.
	wt1 := filepath.Join(root, "wt-m1", ".belmont", "features", "auth")
	wt2 := filepath.Join(root, "wt-m2", ".belmont", "features", "auth")
	writeProgress(wt1, "### M1: One\n- [v] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n")
	writeProgress(wt2, "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [v] P1-M2-1: b\n")

	// The collapsed map auto would build: one representative directory.
	collapsed := map[string]string{"auth": wt1}
	perMilestone := map[string]string{"M1": wt1, "M2": wt2}

	features := listFeaturesWithOverrides(featuresDir, 40, collapsed, "auth", perMilestone)
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	if got := features[0].TasksVerified; got != 2 {
		t.Errorf("TasksVerified = %d, want 2 — the listing read one representative worktree and reported the other milestone's stale state. `belmont blockers` reads per-milestone, and the listing points readers at it, so the two disagreed mid-run", got)
	}
}

// Serial and multi-feature runs keep the whole-feature override; only the
// parallel case changes.
func TestFeatureListingStillHonoursWholeFeatureOverride(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, ".belmont", "features")
	masterDir := filepath.Join(featuresDir, "auth")
	wtDir := filepath.Join(root, "wt", ".belmont", "features", "auth")
	for _, d := range []string{masterDir, wtDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(masterDir, "PROGRESS.md"), []byte("### M1: One\n- [ ] P1-M1-1: a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, "PROGRESS.md"), []byte("### M1: One\n- [v] P1-M1-1: a\n"), 0644); err != nil {
		t.Fatal(err)
	}

	features := listFeaturesWithOverrides(featuresDir, 40, map[string]string{"auth": wtDir}, "", nil)
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	if got := features[0].TasksVerified; got != 1 {
		t.Errorf("TasksVerified = %d, want 1 — the serial-mode override regressed", got)
	}
}
