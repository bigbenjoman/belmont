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

// Two milestones carrying at once exercises the descending-index splice: insert
// into M1 first and every later insertion point shifts, so the loop goes
// highest-first. Also covers a milestone whose block in "ours" is a bare header
// with no tasks, where the insertion point is the header line itself.
func TestConflictResolverCarriesIntoSeveralMilestonesAtOnce(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: as first written\n\n### M2: Two\n\n### M3: Three\n- [>] P1-M3-1: as first written\n"
	ours := "### M1: One\n- [x] P1-M1-1: first\n\n### M2: Two\n\n### M3: Three\n- [x] P1-M3-1: third\n"
	theirs := "### M1: One\n- [x] P1-M1-1: first\n- [!] P1-M1-9: carried into one\n\n### M2: Two\n- [ ] P1-M2-9: carried into an empty block\n\n### M3: Three\n- [x] P1-M3-1: third\n- [ ] P1-M3-9: carried into three\n"

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
	for _, id := range []string{"P1-M1-9", "P1-M2-9", "P1-M3-9"} {
		if idxOf(id) == -1 {
			t.Fatalf("%s was dropped:\n%s", id, got)
		}
	}
	// Each carried task must land inside its own milestone's block.
	m2, m3 := idxOf("### M2:"), idxOf("### M3:")
	if got1 := idxOf("P1-M1-9"); got1 > m2 {
		t.Errorf("P1-M1-9 escaped M1 into a later block:\n%s", got)
	}
	if got2 := idxOf("P1-M2-9"); got2 < m2 || got2 > m3 {
		t.Errorf("P1-M2-9 did not land inside M2's empty block:\n%s", got)
	}
	if got3 := idxOf("P1-M3-9"); got3 < m3 {
		t.Errorf("P1-M3-9 landed before its own milestone:\n%s", got)
	}
	if !strings.Contains(got, "- [!] P1-M1-9") {
		t.Errorf("the carried blocker lost its marker:\n%s", got)
	}
}

// The insertion index must survive anything else that appends to `merged` during
// the same iteration.
//
// msInsertAfter was recorded at the TOP of the walk body while the line itself is
// appended at the BOTTOM, and the activity-merge block in between can inject
// theirs' unseen activity rows. Its trigger is `strings.HasPrefix(line, "##")`,
// and "##" is a prefix of "###" — so it fires on a milestone header. For a
// milestone whose block in "ours" is a bare header there is no later line to
// correct the index, and the carried task was spliced BEFORE that header, i.e.
// into the previous milestone. That produces a cross-milestone task ID, which
// `belmont validate` rates severityError and which blocks `belmont auto` — all
// under a green "conflicts auto-resolved".
func TestCarriedTaskSurvivesAnActivityFlushOnTheSameIteration(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: base text\n\n### M2: Two\n\n## Recent Activity\n\n| a | b |\n"
	// The M1 task mentions "## Activity" in its own text, which is all it takes:
	// inActivitySection is set by an unanchored strings.Contains.
	ours := "### M1: One\n- [x] P1-M1-1: add a \"## Activity\" section to the report\n\n### M2: Two\n\n## Recent Activity\n\n| a | b |\n"
	theirs := "### M1: One\n- [x] P1-M1-1: add a \"## Activity\" section to the report — revised\n\n### M2: Two\n- [!] P1-M2-9: carried into empty M2\n\n## Recent Activity\n\n| a | b |\n| c | d |\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined")
	}
	got := readFile(t, filepath.Join(root, rel))

	// The authoritative check is what the parser makes of the result, not where
	// the bytes landed: a misplaced carry shows up as the task belonging to the
	// wrong milestone.
	for _, m := range parseMilestones(got) {
		for _, task := range m.Tasks {
			if task.ID == "P1-M2-9" && m.ID != "M2" {
				t.Errorf("carried task P1-M2-9 parsed under %s, not M2 — a cross-milestone task ID that `belmont validate` rates severityError:\n%s", m.ID, got)
			}
		}
	}
	if !strings.Contains(got, "- [!] P1-M2-9") {
		t.Errorf("carried task lost or mangled:\n%s", got)
	}
}

// A task is a bullet plus its indented body, and anything that moves one moves
// both — the invariant `taskBodyEnd` exists for (#33). Carrying only the bullet
// leaves the **Evidence** behind on the side it came from.
func TestCarriedTaskBringsItsBody(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: base\n"
	ours := "### M1: One\n- [x] P1-M1-1: build it\n"
	theirs := "### M1: One\n- [x] P1-M1-1: build it\n- [!] P1-M1-9: rotate the credentials\n  **Verification**: console shows the new key\n  **Evidence**: none yet — waiting on ops\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("resolver declined")
	}
	got := readFile(t, filepath.Join(root, rel))
	for _, want := range []string{"- [!] P1-M1-9", "**Verification**", "**Evidence**"} {
		if !strings.Contains(got, want) {
			t.Errorf("carried task lost %q — the bullet arrived without its body:\n%s", want, got)
		}
	}
}

// Ambiguous structure is refused, not guessed — the policy `mergeProgressState`
// already applies via dupMS and `requireUnambiguousMilestones` applies at startup.
func TestCarryRefusesWhenTheTargetMilestoneIDIsDuplicated(t *testing.T) {
	base := "### M1: One\n- [>] P1-M1-1: base\n"
	ours := "### M1: One\n- [x] P1-M1-1: build it\n\n### M1: One again\n- [ ] P1-M1-2: other\n"
	theirs := "### M1: One\n- [x] P1-M1-1: build it\n- [!] P1-M1-9: which M1 does this belong to?\n\n### M1: One again\n- [ ] P1-M1-2: other\n"

	root, rel := conflictFixture(t, base, ours, theirs)
	if resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Errorf("resolver placed a carried task into a duplicated milestone ID rather than escalating:\n%s",
			readFile(t, filepath.Join(root, rel)))
	}
}

// Widening the ID SHAPE without the `<ID>:` delimiter left both merge readers
// still disagreeing with parseTaskID — a bullet merely beginning with an
// uppercase-initial hyphen-number token took that token as its identity. Two
// such bullets on opposite sides then swapped markers.
func TestMergeReadersDoNotTreatALeadingProseTokenAsATaskID(t *testing.T) {
	master := "### M1: Work\n- [x] OAuth-2 migration for the API\n"
	worktree := "### M1: Work\n- [ ] OAuth-2 migration for the API\n- [ ] OAuth-2 scope cleanup in admin\n"

	// parseTaskID is the definition; it reports no ID for either line.
	if _, _, ok := parseTaskID("OAuth-2 migration for the API"); ok {
		t.Fatal("fixture invalid: parseTaskID should report no ID here")
	}

	got, _ := mergeProgressState(master, worktree)
	if strings.Contains(got, "- [x] OAuth-2 scope cleanup") {
		t.Errorf("a marker was applied across two different bullets that share a leading prose token:\n%s", got)
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

// The exact scenario from the #36 review thread on status.go:552, which was
// answered there by documenting the gap rather than closing it:
//
//	"with worktrees for M1 and M2 and a [!] only in M2's, the listing shows no
//	 Blocked section at all, while --feature and belmont blockers both show it.
//	 The listing you just taught to defer to `blockers` disagrees with it."
//
// The Blocked section is driven by TasksBlocked, so the cap and its pointer
// line could never fire for a blocker raised outside the representative
// worktree. Pinned as a regression test, not just as a count.
func TestListingSeesABlockerRaisedOutsideTheRepresentativeWorktree(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, ".belmont", "features")
	write := func(dir, content string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(featuresDir, "auth"), "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n")

	// M1's worktree is the alphabetically first, so it is the representative
	// the collapsed map picks. The blocker is in M2's.
	wt1 := filepath.Join(root, "wt-m1", ".belmont", "features", "auth")
	wt2 := filepath.Join(root, "wt-m2", ".belmont", "features", "auth")
	write(wt1, "### M1: One\n- [x] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n")
	write(wt2, "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [!] P1-M2-1: b — waiting on a person\n")

	features := listFeaturesWithOverrides(featuresDir, 40, map[string]string{"auth": wt1}, "auth",
		map[string]string{"M1": wt1, "M2": wt2})
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	if got := features[0].TasksBlocked; got != 1 {
		t.Errorf("TasksBlocked = %d, want 1 — the listing read only M1's worktree, so the Blocked section never fired for a blocker raised in M2's, while --feature and `belmont blockers` both showed it", got)
	}
}

// Master must remain the BASELINE, not just a starting point that gets fully
// replaced. Both other overlay tests put every milestone in perMilestoneLive, so
// the overlay covers 100% of the file and the baseline is unobservable — a
// mutant that restored the whole-feature swap while keeping overlayLiveMilestones
// would survive them. A milestone with no live worktree pins the other half.
func TestFeatureListingKeepsMasterForMilestonesWithNoWorktree(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, ".belmont", "features")
	write := func(dir, content string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// M3 has no worktree. Master says it is verified; wt1's stale copy says todo.
	write(filepath.Join(featuresDir, "auth"),
		"### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n\n### M3: Three\n- [v] P1-M3-1: c\n")

	wt1 := filepath.Join(root, "wt-m1", ".belmont", "features", "auth")
	wt2 := filepath.Join(root, "wt-m2", ".belmont", "features", "auth")
	write(wt1, "### M1: One\n- [v] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n\n### M3: Three\n- [ ] P1-M3-1: c\n")
	write(wt2, "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [v] P1-M2-1: b\n\n### M3: Three\n- [ ] P1-M3-1: c\n")

	features := listFeaturesWithOverrides(featuresDir, 40, map[string]string{"auth": wt1}, "auth",
		map[string]string{"M1": wt1, "M2": wt2})
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	// M1 and M2 live (verified), M3 from master (verified) = 3.
	if got := features[0].TasksVerified; got != 3 {
		t.Errorf("TasksVerified = %d, want 3 — M3 has no worktree, so it must come from MASTER. Reading it from the representative worktree reports master's verified task as todo", got)
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
