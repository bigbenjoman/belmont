package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #31's root cause is one structural question — "where does the milestones
// region end?" — answered independently by every reader. 8d7f82b routed five of
// them through isSectionBreak and 6833347 called mergeProgressState "the last
// reader that disagreed". It was not: rebuildAfterScopeGuard, the merge's own
// duplicate counter, resolveProgressConflict, resetVerifiedTasks and
// pendingTasksInRange all still walked every line.
//
// A task-shaped line past a column-zero `## ` is the shared trigger. It is not
// hypothetical: agents log completions into `## Session History`, and the
// project that reported #31 had 85 such lines.
//
// These tests are controls. Each one fails if its reader stops honouring the
// boundary.

// ---------------------------------------------------------------- resolver

// resolveDocConflict builds a real merge conflict from two full documents and
// returns whether the resolver claimed success plus the file it left behind.
// buildConflict (marker_readers_test.go) varies a single line inside a fixed
// document; these cases need the region boundary itself to vary.
func resolveDocConflict(t *testing.T, base, ours, theirs string) (bool, string) {
	t.Helper()
	d := t.TempDir()
	rel := filepath.Join(".belmont", "features", "demo", "PROGRESS.md")
	abs := filepath.Join(d, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	git := func(a ...string) {
		c := exec.Command("git", a...)
		c.Dir = d
		if out, err := c.CombinedOutput(); err != nil && a[0] != "merge" {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	write := func(s string) {
		if err := os.WriteFile(abs, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t.t")
	git("config", "user.name", "t")
	git("config", "commit.gpgsign", "false")
	write(base)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	git("checkout", "-q", "-b", "side")
	write(theirs)
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "main")
	write(ours)
	git("commit", "-qam", "ours")
	git("merge", "side") // conflicts

	resolved := resolveProgressConflict(d, rel, abs)
	b, _ := os.ReadFile(abs)
	return resolved, string(b)
}

// inRegionMarker returns the marker on the in-region task line, which is the
// one that carries state. The history quote below the break is decoration.
func inRegionMarker(t *testing.T, doc string) string {
	t.Helper()
	ms := parseMilestones(doc)
	for _, m := range ms {
		for _, task := range m.Tasks {
			if task.ID == "P1-M1-1" {
				return task.Marker
			}
		}
	}
	t.Fatalf("P1-M1-1 not found in any milestone:\n%s", doc)
	return ""
}

func TestResolverIgnoresTaskLinesOutsideTheRegion(t *testing.T) {
	const base = "# Progress\n\n### M1: Only\n- [ ] P1-M1-1: real task\n\n## Session History\n- base note\n"

	t.Run("a [v] quoted in the history must not mint a verified flip", func(t *testing.T) {
		ours := "# Progress\n\n### M1: Only\n- [ ] P1-M1-1: real task\n\n## Session History\n- note from ours\n"
		theirs := "# Progress\n\n### M1: Only\n- [ ] P1-M1-1: real task\n\n## Session History\n- [v] P1-M1-1: real task (logged as verified)\n"

		_, got := resolveDocConflict(t, base, ours, theirs)
		if m := inRegionMarker(t, got); m == "v" {
			// `[v]` requires commit evidence, and runEvidenceCheck runs
			// post-phase inside the worktree — it never inspects a post-merge
			// file, so a flip minted here is never audited.
			t.Errorf("resolver minted [v] on the in-region task from a Session History quote:\n%s", got)
		}
	})

	t.Run("a quote on OUR side must not be rewritten to theirs' state", func(t *testing.T) {
		// Exercises the write-side gate. Without it the resolver rewrites ours'
		// history quote and commits the mutated log line as auto-resolved.
		//
		// Both sides must touch the SAME line or git merges them cleanly and
		// the resolver is never called — which is how this gate went uncovered.
		ours := "# Progress\n\n### M1: Only\n- [>] P1-M1-1: real task\n\n## Session History\n- [ ] P1-M1-1: quoted while it was still open\n"
		theirs := "# Progress\n\n### M1: Only\n- [x] P1-M1-1: real task\n\n## Session History\n- base note\n"

		resolved, got := resolveDocConflict(t, base, ours, theirs)
		if !resolved {
			t.Fatalf("resolver declined a fixture of known markers, so the write-side gate is untested:\n%s", got)
		}
		if !strings.Contains(got, "- [ ] P1-M1-1: quoted while it was still open") {
			t.Errorf("ours' history quote was rewritten to theirs' state:\n%s", got)
		}
		if m := inRegionMarker(t, got); m != "x" {
			t.Errorf("in-region task should have taken theirs' [x], got [%s]:\n%s", m, got)
		}
	})

	t.Run("a stale quote must not shadow theirs' real completion", func(t *testing.T) {
		ours := "# Progress\n\n### M1: Only\n- [ ] P1-M1-1: real task\n\n## Session History\n- note from ours\n"
		theirs := "# Progress\n\n### M1: Only\n- [x] P1-M1-1: real task\n\n## Session History\n- [ ] P1-M1-1: recorded copy of the task line\n"

		resolved, got := resolveDocConflict(t, base, ours, theirs)
		if !resolved {
			// Never Skip here: a fixture that stops conflicting makes the test
			// vacuous without failing, which is how the write-side gate above
			// went uncovered.
			t.Fatalf("resolver declined a fixture of known markers:\n%s", got)
		}
		if m := inRegionMarker(t, got); m != "x" {
			t.Errorf("theirs' completed [x] was lost to its own history quote (marker = %q):\n%s", m, got)
		}
	})
}

// ---------------------------------------------------------------- reverify

func TestReverifyLeavesLinesPastASectionBreakAlone(t *testing.T) {
	doc := "# Progress\n\n### M1: Only\n- [v] P1-M1-1: real task\n\n## Session History\n- [v] reviewed the deploy checklist in session 3\n"

	got, changed := resetVerifiedTasks(doc, map[string]bool{"M1": true})
	if !changed {
		t.Fatal("resetVerifiedTasks reported no change; the in-region [v] should have reset")
	}
	if !strings.Contains(got, "- [x] P1-M1-1: real task") {
		t.Errorf("the in-region task did not reset to [x]:\n%s", got)
	}
	if !strings.Contains(got, "- [v] reviewed the deploy checklist in session 3") {
		t.Errorf("reverify rewrote a history entry that belongs to no milestone:\n%s", got)
	}
}

// ---------------------------------------------------------------- scheduling

func TestPendingTasksIgnoresBulletsOutsideTheRegion(t *testing.T) {
	for name, doc := range map[string]string{
		"past a section break": "# Progress\n\n### M1: Only\n- [x] P1-M1-1: done\n\n## Session History\n- [ ] retro: write this up next week\n",
		"before the first header": "# Progress\n- [ ] stray preamble bullet\n\n## Milestones\n\n" +
			"### M1: Only\n- [x] P1-M1-1: done\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFeature(t, root, "demo", doc)
			if pendingTasksInRange(root, "demo", "", "") {
				t.Error("a `- [ ]` bullet outside every milestone was counted as pending work — " +
					"the feature is finished, so the loop can never reach actionComplete")
			}
		})
	}

	// …and real outstanding work must still register.
	root := t.TempDir()
	writeFeature(t, root, "demo", "# Progress\n\n## Milestones\n\n### M1: Only\n- [x] P1-M1-1: done\n- [ ] P1-M1-2: outstanding\n\n## Session History\n")
	if !pendingTasksInRange(root, "demo", "", "") {
		t.Error("a genuine in-region [ ] task was not reported as pending")
	}
}

func TestFwlupTasksIgnoreBulletsPastASectionBreak(t *testing.T) {
	root := t.TempDir()
	writeFeature(t, root, "demo",
		"# Progress\n\n### M1: Only\n- [v] P1-M1-1: done\n\n## Session History\n- [ ] FWLUP: retro bullet, not work\n")

	if fwlupTasksInRange(root, "demo", statusReport{}, "M1", "M1") {
		t.Error("a FWLUP-shaped bullet under ## Session History was counted as outstanding follow-up work")
	}

	// The control. fwlupTasksInRange returns false on any read error, so the
	// negative above is satisfied by a path typo or an unconditional `return
	// false` — it proves nothing on its own.
	real := t.TempDir()
	writeFeature(t, real, "demo",
		"# Progress\n\n### M1: Only\n- [ ] P1-M1-1: FWLUP: a genuine follow-up\n\n## Session History\n")
	if !fwlupTasksInRange(real, "demo", statusReport{}, "M1", "M1") {
		t.Error("a genuine in-region FWLUP task was not detected")
	}
}

// A repeated `### M<n>:` heading makes every milestone-keyed lookup ambiguous.
// Both guards refuse rather than rewrite the wrong block; `belmont validate` is
// what tells the user to fix it.
func TestDuplicateMilestoneIDIsRefusedAndReported(t *testing.T) {
	doc := "### M1: A\n- [ ] P1-M1-1: a\n\n### M2: B\n- [x] P1-M2-1: b\n\n" +
		"## Session History\n\n### M2: retry notes for milestone two\n- attempt log line\n"

	snap := parseProgressSnapshot("P", doc)
	if len(snap.DupIDs) == 0 {
		t.Fatal("parseProgressSnapshot did not flag the repeated M2 heading, so ByID silently names one of them")
	}

	// The rebuild path is what deleted live task lines when it guessed.
	v := detectViolations("demo", parseMilestones(doc))
	var reported bool
	for _, x := range v {
		if x.Rule == "duplicate_milestone_id" {
			reported = true
		}
	}
	if !reported {
		t.Errorf("belmont validate does not report the duplicate milestone ID; violations=%+v", v)
	}
}

// ---------------------------------------------------------------- merge

func TestMergeDuplicateCountIsRegionScoped(t *testing.T) {
	// A sibling logged its completion into the history. That line is not a task,
	// so the in-region ID is unique and the merge must proceed.
	master := "### M1: Only\n- [x] P1-M1-1: thing\n\n## Session History\n- [x] P1-M1-1: thing (done, commit abc123)\n"
	wt := "### M1: Only\n- [ ] P1-M1-1: thing\n\n## Session History\n- [x] P1-M1-1: thing (done, commit abc123)\n"

	got, warnings := mergeProgressState(master, wt)
	for _, w := range warnings {
		if strings.Contains(w, "appears more than once") {
			t.Errorf("false duplicate reported for an ID that occurs once in the region: %s", w)
		}
	}
	if !strings.Contains(got, "- [x] P1-M1-1: thing\n") {
		t.Errorf("master's recorded completion was dropped — the #24 loss, via the #31 root cause:\n%s", got)
	}
}

// An ID the worktree holds ONLY as an orphan — a fork-stale line an agent
// stranded behind a new `## ` heading, or a sibling's completion quoted into a
// log — must not suppress the carry-over. Master's in-region copy is the only
// counted one, and `.belmont/` moves only by this copy, so skipping it deletes
// the task irrecoverably. Carry it and say so.
func TestMergeCarriesOverWhenWorktreeHoldsTheIDOnlyAsAnOrphan(t *testing.T) {
	// Sibling A completed P1-M1-2 and merged. Sibling B stranded its fork-time
	// copy behind a heading its agent wrote.
	master := "### M1: One\n- [x] P1-M1-1: a\n- [x] P1-M1-2: b\n\n## Session History\n"
	wt := "### M1: One\n- [x] P1-M1-1: a\n\n## What I learned\n- [ ] P1-M1-2: b\n\n## Session History\n"

	got, warnings := mergeProgressState(master, wt)

	var inRegion *task
	for _, m := range parseMilestones(got) {
		for i, task := range m.Tasks {
			if task.ID == "P1-M1-2" {
				inRegion = &m.Tasks[i]
			}
		}
	}
	if inRegion == nil {
		t.Fatalf("sibling A's completion was deleted — it exists in no commit, so this is unrecoverable:\n%s", got)
	}
	if inRegion.Marker != "x" {
		t.Errorf("in-region P1-M1-2 reads [%s], want [x]", inRegion.Marker)
	}
	// The stray copy is left exactly as written, and reported.
	if !strings.Contains(got, "- [ ] P1-M1-2: b") {
		t.Errorf("the orphan line was rewritten instead of left alone:\n%s", got)
	}
	var warned bool
	for _, w := range warnings {
		if strings.Contains(w, "P1-M1-2") && strings.Contains(w, "outside every milestone") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("carrying a line whose ID is also stranded outside the region must be reported; warnings=%v", warnings)
	}
	// And the result must not be refused by the next sibling merge.
	if _, next := mergeProgressState(got, got); len(next) != 0 {
		t.Errorf("merging the result against itself warns, so a later sibling would refuse it: %v", next)
	}
}

func TestMergeCarriesOverIntoAnEmptyMilestone(t *testing.T) {
	// M3 exists on both sides but holds no tasks here yet. The anchor used to
	// come only from an existing task line, so the first follow-up a sibling
	// added to it had nowhere to land and was dropped from its only copy.
	master := "### M2: B\n- [ ] P1-M2-1: b\n\n### M3: Follow-ups\n- [ ] P1-M3-1: first follow-up, added by a sibling\n"
	wt := "### M2: B\n- [x] P1-M2-1: b\n\n### M3: Follow-ups\n"

	got, warnings := mergeProgressState(master, wt)
	if !strings.Contains(got, "P1-M3-1") {
		t.Errorf("the sibling's follow-up was dropped; warnings=%v\n%s", warnings, got)
	}
	ms := parseMilestones(got)
	for _, m := range ms {
		if m.ID != "M3" {
			continue
		}
		if len(m.Tasks) != 1 {
			t.Errorf("M3 holds %d tasks, want 1 — the carried line landed outside its milestone:\n%s", len(m.Tasks), got)
		}
	}
}

func TestMergeDoesNotWarnAboutMasterOrphans(t *testing.T) {
	// A legitimate history line on master is not an unmerged task. The orphan
	// must exist on the MASTER side only: with the same line on both sides the
	// worktree's copy masks the carry-over pass, and the test passes whether or
	// not that pass iterates master's tasks or rescans master's raw lines.
	master := "### M1: One\n- [x] P1-M1-1: a\n\n## Session History\n- [x] P9-LOG-9: logged by a sibling\n"
	wt := "### M1: One\n- [ ] P1-M1-1: a\n\n## Session History\n"

	got, warnings := mergeProgressState(master, wt)
	for _, w := range warnings {
		if strings.Contains(w, "P9-LOG-9") {
			t.Errorf("false alarm about a line that was never dropped: %s", w)
		}
	}
	if strings.Count(got, "P9-LOG-9") > 0 {
		t.Errorf("master's history line was spliced into a milestone; orphans belong to none:\n%s", got)
	}
}

// Several master-only tasks carrying into one anchor must land in master's
// document order. Iterating the masterTasks map instead is nondeterministic, so
// repeated merges would rewrite PROGRESS.md with the lines shuffled.
func TestMergeCarryOverOrderIsDeterministic(t *testing.T) {
	master := "### M1: One\n- [ ] P1-M1-1: a\n- [ ] P1-M1-2: b\n- [ ] P1-M1-3: c\n- [ ] P1-M1-4: d\n"
	wt := "### M1: One\n"

	want := []string{"P1-M1-1", "P1-M1-2", "P1-M1-3", "P1-M1-4"}
	for i := 0; i < 20; i++ {
		got, _ := mergeProgressState(master, wt)
		var order []string
		for _, m := range parseMilestones(got) {
			for _, task := range m.Tasks {
				order = append(order, task.ID)
			}
		}
		if strings.Join(order, ",") != strings.Join(want, ",") {
			t.Fatalf("run %d: carried tasks out of document order: got %v, want %v", i, order, want)
		}
	}
}

// The worktree walk must leave orphan lines alone. Master's marker outranks the
// orphan's here, so a missing region guard rewrites a historical log entry.
func TestMergeLeavesWorktreeOrphanMarkerAlone(t *testing.T) {
	master := "### M1: One\n- [x] P1-M1-1: a\n\n## Session History\n"
	wt := "### M1: One\n- [x] P1-M1-1: a\n\n## Session History\n- [ ] P1-M1-1: quoted while it was still open\n"

	got, _ := mergeProgressState(master, wt)
	if !strings.Contains(got, "- [ ] P1-M1-1: quoted while it was still open") {
		t.Errorf("a Session History quote was rewritten to master's in-region state:\n%s", got)
	}
}

// Mutation control: the merge's WORKTREE walk must reset at a break too. Only
// the master walk was pinned, so deleting the reset from the second loop passed
// the whole suite while insertions landed below `## Session History`.
func TestMergeWorktreeWalkRespectsSectionBreak(t *testing.T) {
	master := "### M1: One\n- [x] P1-M1-1: a\n- [ ] P1-M1-2: follow-up from a sibling\n"
	wt := "### M1: One\n- [ ] P1-M1-1: a\n\n## Session History\n\n- [ ] P9-ORPHAN-1: a logged line\n"

	got, _ := mergeProgressState(master, wt)
	idx := strings.Index(got, "## Session History")
	if idx < 0 {
		t.Fatalf("section heading vanished:\n%s", got)
	}
	if at := strings.Index(got, "P1-M1-2"); at < 0 {
		t.Fatalf("the carried follow-up is missing:\n%s", got)
	} else if at > idx {
		t.Errorf("the carried follow-up was spliced BELOW the section break, where nothing counts it:\n%s", got)
	}
}

// Mutation control: equal rank must not rewrite the line. `>=` respells the
// worktree's marker to master's variant and forces a spurious write.
func TestMergeKeepsWorktreeMarkerAtEqualRank(t *testing.T) {
	master := "### M1: One\n- [x] P1-M1-1: a\n"
	wt := "### M1: One\n- [X] P1-M1-1: a\n"

	got, _ := mergeProgressState(master, wt)
	if !strings.Contains(got, "- [X] P1-M1-1: a") {
		t.Errorf("equal-rank markers must be left alone; the worktree's [X] was rewritten:\n%s", got)
	}
}

// ---------------------------------------------------------------- guards

// Mutation control: revertEvidenceMissing's conversion in 8d7f82b had no test.
// With the trimmed check an indented heading clears currentMS, so an
// un-evidenced [v] after it is silently not reverted — findEvidenceMissingFlips
// still reports it, the rebuild is a no-op, and runEvidenceCheck returns before
// logging or steering. A bypass of the whole verify-evidence invariant.
func TestEvidenceRevertRespectsSectionBreak(t *testing.T) {
	doc := `### M1: Work
- [x] P1-M1-1: first

  ## a heading quoted in the write-up

- [v] P1-M1-2: flipped with no commit behind it
`
	snap := parseProgressSnapshot("P", doc)
	got := revertEvidenceMissing(snap, []evidenceMissing{
		{Milestone: "M1", TaskID: "P1-M1-2", FromState: "x"},
	})
	if !strings.Contains(got, "- [x] P1-M1-2:") {
		t.Errorf("the un-evidenced [v] survived because an indented heading ended the milestone early:\n%s", got)
	}
}

// ---------------------------------------------------------------- diagnostics

func mkFeature(t *testing.T, root, slug, progress string) {
	t.Helper()
	dir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# PRD: "+slug+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(progress), 0644); err != nil {
		t.Fatal(err)
	}
}

// A task line past the region end, i.e. the thing #31 made visible.
const orphanDoc = "# Progress: Demo\n\n## Milestones\n\n### M1: Work\n- [x] P1-M1-1: done\n\n" +
	"## Session History\n- [ ] P1-M1-2: outstanding work stranded outside the region\n"

// Mutation controls for the three wires from orphan detection to a user-visible
// surface. Each pure function was covered; every wire out of it could be cut
// with a green suite — which is exactly the "belmont validate exits 0" symptom
// that made #31 invisible for two releases.

func TestValidateReportsOrphans(t *testing.T) {
	root := t.TempDir()
	mkFeature(t, root, "demo", orphanDoc)

	v, err := validateFeature(root, "demo")
	if err != nil {
		t.Fatalf("validateFeature: %v", err)
	}
	var found bool
	for _, x := range v {
		if x.Rule == "task_outside_milestone" {
			found = true
		}
	}
	if !found {
		t.Errorf("belmont validate reports no violation for a stranded task line; violations=%+v", v)
	}
}

func TestBuildStatusExposesOrphans(t *testing.T) {
	root := t.TempDir()
	mkFeature(t, root, "demo", orphanDoc)

	report, err := buildStatus(root, 0, "demo")
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}
	if len(report.Orphans) != 1 {
		t.Errorf("statusReport.Orphans = %d, want 1 — the detail view cannot warn about what it never collected", len(report.Orphans))
	}
}

func TestListingCountsOrphansFromTheRealPipeline(t *testing.T) {
	root := t.TempDir()
	mkFeature(t, root, "demo", orphanDoc)

	// Drive the real listing pipeline. Hand-building a featureSummary pins the
	// renderer and leaves the population site free to return zero.
	features := listFeaturesWithOverrides(filepath.Join(root, ".belmont", "features"), 0, nil)
	if len(features) != 1 {
		t.Fatalf("features = %d, want 1", len(features))
	}
	if features[0].TasksOrphaned != 1 {
		t.Errorf("featureSummary.TasksOrphaned = %d, want 1 — listing mode would show a clean feature",
			features[0].TasksOrphaned)
	}
}
