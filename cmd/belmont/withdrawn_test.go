package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `[-]` withdrawn exists because its absence caused issue #27. Belmont's marker
// set had no way to say "we decided not to do this", so a user invented `[-]`,
// every unrecognised marker parsed as todo, and cancelled work was offered to an
// agent as the next thing to build. Repair tooling does not fix that on its own
// — without a legal way to express withdrawal the next person invents a marker
// too.

// gitRun runs one git command in root and fails the test on any error.
func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = root
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestWithdrawnAndCapitalVParse(t *testing.T) {
	for marker, want := range map[string]taskStatus{
		"-": taskWithdrawn,
		"v": taskVerified,
		"V": taskVerified, // case-insensitive, like x/X
		"x": taskDone,
		"X": taskDone,
	} {
		got, ok := canonicalMarker(marker)
		if !ok || got != want {
			t.Errorf("canonicalMarker(%q) = (%v, %v), want (%v, true)", marker, got, ok, want)
		}
	}
	// …and the set stays closed.
	for _, m := range []string{"?", "~", "→", "y", ""} {
		if _, ok := canonicalMarker(m); ok {
			t.Errorf("canonicalMarker(%q) was recognised; the set must stay closed", m)
		}
	}
}

// A withdrawn task is resolved: not outstanding, not done, never scheduled.
func TestWithdrawnIsResolvedNotOutstanding(t *testing.T) {
	ms := parseMilestones("### M1: Work\n- [x] P1-M1-1: shipped\n- [-] P1-M1-2: dropped\n")
	if !milestoneAllDone(ms[0]) {
		t.Error("a milestone of [x] + [-] has nothing outstanding; it must read done")
	}
	tasks := flattenTasks(ms, 0)
	if got := computeOverallStatus(tasks); got != "Complete" {
		t.Errorf("status = %q, want Complete", got)
	}
	if nt := nextTask(tasks); nt != nil {
		t.Errorf("withdrawn work was scheduled: %+v", nt)
	}

	verified := parseMilestones("### M1: Work\n- [v] P1-M1-1: shipped\n- [-] P1-M1-2: dropped\n")
	if !milestoneAllVerified(verified[0]) {
		t.Error("a milestone of [v] + [-] must read verified — the dropped task is not unverified work")
	}

	// All-withdrawn is resolved, not "not started" — otherwise the loop stalls
	// forever on work somebody deliberately cancelled.
	all := parseMilestones("### M1: Work\n- [-] P1-M1-1: dropped\n- [-] P1-M1-2: also dropped\n")
	if milestoneNotStarted(all[0]) {
		t.Error("an all-withdrawn milestone is not `not started`")
	}
	if got := computeOverallStatus(flattenTasks(all, 0)); got != "Complete" {
		t.Errorf("all-withdrawn status = %q, want Complete", got)
	}
}

// THE reason withdrawal is a marker and not a deletion. Deleting the line does
// not survive mergeProgressState: the worktree is the base, so master's missing
// line is carried straight back in — in both directions.
func TestWithdrawalSurvivesTheMergeThatDeletionDoesNot(t *testing.T) {
	full := "### M3: Webhooks\n- [x] P1-M3-1: send\n- [ ] P1-M3-2: add retry\n"
	deleted := "### M3: Webhooks\n- [x] P1-M3-1: send\n"
	withdrawn := "### M3: Webhooks\n- [x] P1-M3-1: send\n- [-] P1-M3-2: add retry\n"

	// The control, asserted rather than logged. This is the premise the whole
	// `[-]`-is-a-state design rests on, and a t.Log cannot fail — it read as
	// evidence while checking nothing.
	if got, _ := mergeProgressState(deleted, full); !strings.Contains(got, "P1-M3-2") {
		t.Fatalf("deletion now survives the merge, so the argument for [-] over deleting the line "+
			"needs re-deriving before this file is trusted:\n%s", got)
	}

	// The contract: a withdrawal wins from either side and is never revived.
	if got, _ := mergeProgressState(withdrawn, full); !strings.Contains(got, "- [-] P1-M3-2") {
		t.Errorf("master's withdrawal was reverted by a stale worktree:\n%s", got)
	}
	if got, _ := mergeProgressState(full, withdrawn); !strings.Contains(got, "- [-] P1-M3-2") {
		t.Errorf("the worktree's withdrawal was overwritten by master's copy:\n%s", got)
	}
	// Even against a more-advanced marker: reviving is a deliberate edit.
	done := "### M3: Webhooks\n- [x] P1-M3-1: send\n- [x] P1-M3-2: add retry\n"
	if got, _ := mergeProgressState(done, withdrawn); !strings.Contains(got, "- [-] P1-M3-2") {
		t.Errorf("a [x] from the other side silently revived withdrawn work:\n%s", got)
	}
}

// Withdrawn gets its own bucket. Folding it into done or todo would misreport
// the feature in exactly the way issue #27 did.
func TestWithdrawnCountedSeparately(t *testing.T) {
	root := t.TempDir()
	mkFeature(t, root, "demo", "# Progress\n\n## Milestones\n\n### M1: Work\n"+
		"- [x] P1-M1-1: shipped\n- [-] P1-M1-2: dropped\n- [ ] P1-M1-3: todo\n")

	report, err := buildStatus(root, 0, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := report.TaskCounts["withdrawn"]; got != 1 {
		t.Errorf("withdrawn count = %d, want 1", got)
	}
	if got := report.TaskCounts["done"]; got != 1 {
		t.Errorf("done count = %d, want 1 — withdrawn must not inflate done", got)
	}
	if got := report.TaskCounts["todo"]; got != 1 {
		t.Errorf("todo count = %d, want 1 — withdrawn must not inflate todo", got)
	}
	if got := taskStatusIcon(taskWithdrawn, false); got != "[-]" {
		t.Errorf("icon = %q, want [-]", got)
	}
	if !strings.Contains(statusLegend(false), "[-] withdrawn") {
		t.Error("the legend does not mention withdrawn, so nobody learns the state exists")
	}
}

// `[-]` and `[V]` must no longer be violations — that is the whole point.
func TestWithdrawnAndCapitalVAreNotViolations(t *testing.T) {
	doc := "### M1: Work\n- [v] P1-M1-1: a\n- [V] P1-M1-2: b\n- [-] P1-M1-3: c\n"
	for _, v := range detectViolations("demo", parseMilestones(doc)) {
		if v.Rule == "unrecognised_task_marker" {
			t.Errorf("false positive on a now-recognised marker: %s", v.Message)
		}
	}
}

// The trap accepting [V] re-armed: findEvidenceMissingFlips compared preState
// against a raw "v", so an already-verified task read as a fresh flip and the
// guard reverted it for lacking a commit it never needed.
func TestCapitalVIsNotAFreshFlip(t *testing.T) {
	// Must be a REAL git repo whose commits do NOT carry the task ID:
	// taskHasCommit returns true when the git query fails, so a bare TempDir
	// would pass this test no matter what findEvidenceMissingFlips did.
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@t.t"},
		{"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "-m", "unrelated work"},
	} {
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	doc := "### M1: Work\n- [V] P1-M1-1: verified a while ago\n"
	pre := parseProgressSnapshot("P", doc)
	post := parseProgressSnapshot("P", doc)
	if missing := findEvidenceMissingFlips(root, pre, post, "M1"); len(missing) != 0 {
		t.Errorf("an unchanged [V] read as a fresh flip and would be reverted: %+v", missing)
	}

	// Control: a genuine fresh flip with no commit behind it IS reported, so
	// the test above cannot pass by the guard being inert.
	preTodo := parseProgressSnapshot("P", "### M1: Work\n- [ ] P1-M1-1: not yet\n")
	if missing := findEvidenceMissingFlips(root, preTodo, post, "M1"); len(missing) != 1 {
		t.Errorf("an un-evidenced [ ]->[V] flip was not caught: %+v", missing)
	}
}

// ----------------------------------------------------------------------------
// Regressions the withdrawn/case-insensitive change introduced
//
// Making `[-]` and `[V]` parse changed the meaning of bytes already sitting in
// files in the wild. Before, both were unrecognised: loud, blocking, and
// impossible to act on by accident. After, they mean something — and every
// reader that classified markers with its own rule instead of canonicalMarker's
// silently started doing the wrong thing with them.
// ----------------------------------------------------------------------------

// resolveProgressConflict was the one markerRank consumer with no withdrawn
// case. taskWithdrawn ranks at -2, below every other state, so anything at all
// outranked a withdrawal and revived it — and the file was written back under a
// green "conflicts auto-resolved". Before `[-]` parsed, the unrecognised-marker
// bail-out caught this and escalated the whole file to the reconciliation
// agent; making the marker legal removed that safety net.
func TestMergeConflictResolverKeepsWithdrawalsFromEitherSide(t *testing.T) {
	for _, tc := range []struct {
		ours, theirs, want string
	}{
		{"-", " ", "-"}, // a stale sibling must not revive cancelled work…
		{"-", "x", "-"},
		{"-", "v", "-"}, // …least of all as an un-evidenced verification
		{" ", "-", "-"}, // …and a withdrawal recorded on the other branch must arrive
		{"x", "-", "-"},
	} {
		root := t.TempDir()
		gitRun(t, root, "init", "-q", "-b", "main")
		gitRun(t, root, "config", "user.email", "t@t.t")
		gitRun(t, root, "config", "user.name", "t")

		rel := "PROGRESS.md"
		doc := func(marker string) string {
			return "### M1: Work\n- [" + marker + "] P1-M1-1: retry logic\n- [ ] P1-M1-2: other\n"
		}
		writeAndCommit := func(content, msg string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			gitRun(t, root, "add", "-A")
			gitRun(t, root, "commit", "-q", "-m", msg)
		}
		// The base differs from BOTH sides in text as well as marker, so each
		// side is a real commit and the two genuinely conflict. A base equal to
		// one side produces an empty commit, or a clean merge in which the
		// resolver under test never runs at all.
		writeAndCommit("### M1: Work\n- [>] P1-M1-1: retry logic, as first written\n- [ ] P1-M1-2: other\n", "base")
		gitRun(t, root, "checkout", "-q", "-b", "sib")
		writeAndCommit(doc(tc.theirs), "sibling side")
		gitRun(t, root, "checkout", "-q", "main")
		writeAndCommit(doc(tc.ours), "main side")

		merge := exec.Command("git", "merge", "--no-commit", "sib")
		merge.Dir = root
		if out, err := merge.CombinedOutput(); err == nil {
			t.Fatalf("ours=%s theirs=%s merged cleanly — the fixture must actually conflict, or the resolver under test never runs\n%s",
				tc.ours, tc.theirs, out)
		}

		if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
			t.Errorf("ours=[%s] theirs=[%s]: the resolver declined; a legal marker must not stop auto-resolution",
				tc.ours, tc.theirs)
			continue
		}
		got := readFile(t, filepath.Join(root, rel))
		if !strings.Contains(got, "- ["+tc.want+"] P1-M1-1") {
			t.Errorf("ours=[%s] theirs=[%s] resolved to something other than [%s]:\n%s",
				tc.ours, tc.theirs, tc.want, got)
		}
	}
}

// A withdrawal winning is the decision. A withdrawal winning SILENTLY is the
// bug: `.belmont/` is assume-unchanged inside a worktree, so this copy is the
// only transport home and a `[v]` it overwrites exists in no commit anywhere.
func TestMergeProgressStateReportsWhatAWithdrawalDisplaced(t *testing.T) {
	for _, tc := range []struct {
		master, worktree string
		wantWarning      bool
	}{
		{"-", "v", true},  // a real completion is being discarded
		{"-", "!", true},  // a live blocker is being cleared
		{"-", ">", true},  // work in flight
		{"-", " ", false}, // nothing lost — do not cry wolf
		{"v", "-", true},  // the same in reverse: the worktree's withdrawal wins
	} {
		master := "### M3: W\n- [" + tc.master + "] P1-M3-1: x\n"
		worktree := "### M3: W\n- [" + tc.worktree + "] P1-M3-1: x\n"
		got, warnings := mergeProgressState(master, worktree)
		if !strings.Contains(got, "- [-] P1-M3-1") {
			t.Errorf("master=[%s] wt=[%s]: the withdrawal did not win:\n%s", tc.master, tc.worktree, got)
		}
		if hasWarning := len(warnings) > 0; hasWarning != tc.wantWarning {
			t.Errorf("master=[%s] wt=[%s]: warnings=%v, want any=%v — state was dropped without telling anyone",
				tc.master, tc.worktree, warnings, tc.wantWarning)
		}
	}
}

// Verified is the strongest claim Belmont makes and it must never be true
// vacuously. Skipping withdrawn tasks without counting the survivors made a
// milestone in which nothing was built and nothing was verified render `[v]` —
// while computeOverallStatus called the same data "Complete".
func TestAllWithdrawnMilestoneDoesNotReadVerified(t *testing.T) {
	all := parseMilestones("### M1: Dropped\n- [-] P1-M1-1: a\n- [-] P1-M1-2: b\n")[0]
	if milestoneAllVerified(all) {
		t.Error("a milestone where nothing was built reads as verified")
	}
	if !milestoneAllDone(all) {
		t.Error("an all-withdrawn milestone has nothing outstanding; the loop must not stall on it")
	}
	if got := milestoneStatusIcon(all, false); got != "[-]" {
		t.Errorf("icon = %q, want [-] — done and verified both claim work happened", got)
	}

	// The control: one live verified task and the milestone is genuinely
	// verified again.
	mixed := parseMilestones("### M1: Mixed\n- [v] P1-M1-1: a\n- [-] P1-M1-2: b\n")[0]
	if !milestoneAllVerified(mixed) {
		t.Error("a withdrawn task made a genuinely verified milestone read unverified")
	}
	if got := milestoneStatusIcon(mixed, false); got != "[v]" {
		t.Errorf("mixed icon = %q, want [v]", got)
	}
}

// Listing mode is the half an agent reads. Withdrawn tasks sit inside
// TasksTotal, so `TasksTotal - TasksVerified` counted deliberately dropped work
// as implementations awaiting verification — and the listing said nothing about
// the withdrawals at all, while the detail view reported both correctly.
func TestListingViewAccountsForWithdrawnTasks(t *testing.T) {
	ms := parseMilestones("### M1: Work\n- [x] P1-M1-1: shipped\n- [-] P1-M1-2: dropped\n- [-] P1-M1-3: also dropped\n")
	report := statusReport{
		Feature: "Demo Product", OverallStatus: "Complete", TaskCounts: map[string]int{},
		Features: []featureSummary{{
			Name: "Demo", Slug: "demo", Status: "Complete",
			TasksDone: 1, TasksVerified: 0, TasksWithdrawn: 2, TasksTotal: 3, Milestones: ms,
		}},
	}
	out := renderStatus(report, false, false)

	if !strings.Contains(out, "2 withdrawn") {
		t.Errorf("listing mode says nothing about withdrawn work, so total-minus-done is wrong by two:\n%s", out)
	}
	if !strings.Contains(out, "⚠ 1 task(s) implemented but never verified") {
		t.Errorf("the never-verified count is not the number of [x] tasks:\n%s", out)
	}
	if strings.Contains(out, "3 task(s) implemented but never verified") {
		t.Errorf("withdrawn tasks were counted as implementations awaiting verification:\n%s", out)
	}

	// …and the detail view has to agree with it, since disagreeing views are
	// what made this invisible.
	if n := len(doneNotVerifiedTasks(ms)); n != 1 {
		t.Errorf("doneNotVerifiedTasks = %d, want 1 — the two views disagree", n)
	}

	// A feature whose every task was withdrawn reads Complete with nothing
	// implemented. `TasksVerified < TasksTotal` is true there and prints
	// "0 task(s) implemented but never verified"; the count has to come from
	// the done tasks, and there are none.
	allDropped := parseMilestones("### M1: Dropped\n- [-] P1-M1-1: a\n- [-] P1-M1-2: b\n")
	report.Features[0] = featureSummary{
		Name: "Demo", Slug: "demo", Status: "Complete",
		TasksDone: 0, TasksVerified: 0, TasksWithdrawn: 2, TasksTotal: 2, Milestones: allDropped,
	}
	if out := renderStatus(report, false, false); strings.Contains(out, "never verified") {
		t.Errorf("an all-withdrawn feature was told to run reverify on nothing:\n%s", out)
	}
}

// The scope guard compared raw marker bytes, so a case-only rewrite read as a
// state change: it reverted a line, amended that revert into the agent's
// commit, and injected a steering correction for a flip that never happened.
func TestScopeGuardIgnoresACaseOnlyMarkerRewrite(t *testing.T) {
	for _, tc := range []struct{ before, after string }{
		{"v", "V"}, {"V", "v"}, {"x", "X"}, {"X", "x"},
	} {
		pre := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- ["+tc.before+"] P1-M2-1: b\n")
		post := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- ["+tc.after+"] P1-M2-1: b\n")
		if v := diffScopeViolations(pre, post, "M1"); len(v) != 0 {
			t.Errorf("[%s] → [%s] read as an out-of-scope flip: %+v", tc.before, tc.after, v)
		}
	}

	// The control: a real state change in a non-target milestone is still a
	// violation, so the fix cannot have simply switched the rule off.
	pre := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- [ ] P1-M2-1: b\n")
	post := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- [x] P1-M2-1: b\n")
	if v := diffScopeViolations(pre, post, "M1"); len(v) != 1 {
		t.Errorf("a genuine out-of-scope flip is no longer caught: %+v", v)
	}

	// Two markers Belmont cannot read are not "the same" just because both are
	// unknown — swapping one for another is still an edit.
	preU := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- [?] P1-M2-1: b\n")
	postU := parseProgressSnapshot("P", "### M1: A\n- [x] P1-M1-1: a\n### M2: B\n- [~] P1-M2-1: b\n")
	if v := diffScopeViolations(preU, postU, "M1"); len(v) != 1 {
		t.Errorf("an unreadable marker was swapped for a different one unnoticed: %+v", v)
	}
}

// `inRange` comes from parseMilestones (strict header regex) and
// `pendingInRange` from pendingTasksInRange (lenient, emoji-tolerant). Any
// header the lenient one accepts and the strict one rejects gave no milestones
// and pending work — and indexing the last element of an empty slice crashed
// `belmont auto` with a Go stack trace on iteration one.
func TestAutoDoesNotCrashWhenNoMilestoneParses(t *testing.T) {
	doc := "# P\n\n### ✅ M1: Emoji header the parser rejects\n- [ ] P1-M1-1: work\n"
	root := t.TempDir()
	writeFeature(t, root, "demo", doc)

	if ms := parseMilestones(doc); len(ms) != 0 {
		t.Skip("the parser now accepts this header; the disagreement this test guards is gone")
	}
	if !pendingTasksInRange(root, "demo", "", "") {
		t.Fatal("fixture: the lenient reader must see pending work, or there is no disagreement to test")
	}

	report := statusReport{Milestones: nil, TaskCounts: map[string]int{}, OverallStatus: "Not Started"}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("belmont auto crashed instead of reporting the problem: %v", r)
		}
	}()
	got := decideLoopActionSmart(report, nil, loopConfig{Feature: "demo", Root: root, MaxIterations: 5},
		false, false, true, false, map[string]*milestoneLoopState{})
	if got == nil || got.Type != actionPause {
		t.Fatalf("action = %+v, want a PAUSE naming the unparseable header", got)
	}
	if !strings.Contains(got.Reason, "milestone") {
		t.Errorf("the pause reason does not say what is wrong: %s", got.Reason)
	}
}

// A `[!]` blocker beating master's `[x]` is the fourth of four directions in
// which a decision beats progress. Three of them warned; this one was silent.
func TestMergeProgressStateReportsWhatABlockerDisplaced(t *testing.T) {
	got, warnings := mergeProgressState("### M3: W\n- [x] P1-M3-1: x\n", "### M3: W\n- [!] P1-M3-1: x\n")
	if !strings.Contains(got, "- [!] P1-M3-1") {
		t.Errorf("the blocker did not win:\n%s", got)
	}
	if len(warnings) == 0 {
		t.Error("master's [x] was discarded without telling anyone")
	}
	// Control: nothing lost, nothing said.
	if _, w := mergeProgressState("### M3: W\n- [ ] P1-M3-1: x\n", "### M3: W\n- [!] P1-M3-1: x\n"); len(w) != 0 {
		t.Errorf("warned when a blocker displaced only a todo: %v", w)
	}
}

// parseSectionLines was the last reader still ending a section on a TRIMMED
// "## " — the exact rule issue #31 removed everywhere else. It made the writer
// (appendDecisionLogEntry, which uses isSectionBreak) and the reader disagree.
func TestDecisionsLogReaderUsesTheSameBoundaryAsTheWriter(t *testing.T) {
	doc := "# P\n\n## Decisions Log\n\n- chose sqlite\n\n  ## quoted heading inside a decision's body\n\n- chose bun over node\n"
	got := parseDecisions(doc, 10)
	if len(got) != 2 {
		t.Errorf("an indented heading truncated the log: %v", got)
	}
	// A column-zero heading still ends it.
	doc2 := "# P\n\n## Decisions Log\n\n- chose sqlite\n\n## Session History\n\n- not a decision\n"
	if got := parseDecisions(doc2, 10); len(got) != 1 {
		t.Errorf("a real section break no longer ends the log: %v", got)
	}
	// …and so does a bare `##`, which the trimmed test did not recognise.
	doc3 := "# P\n\n## Decisions Log\n\n- chose sqlite\n\n##\n\n- not a decision\n"
	if got := parseDecisions(doc3, 10); len(got) != 1 {
		t.Errorf("a bare ## did not end the log: %v", got)
	}
}

// `## Decisions Log` is not always the last section, and `### M<n>:` is
// deliberately not a section break — so a log sitting ABOVE the milestones
// swallowed the whole milestones region and `belmont status` listed task lines
// as decisions. The reader and `appendDecisionLogEntry` now stop at the same
// boundary.
func TestDecisionsLogStopsAtTheMilestones(t *testing.T) {
	doc := "# P\n\n## Decisions Log\n\n- chose sqlite\n\n### M1: Work\n- [x] P1-M1-1: a real task\n- [ ] P1-M1-2: another\n"
	got := parseDecisions(doc, 10)
	if len(got) != 1 {
		t.Errorf("task lines were read as decisions: %v", got)
	}
	for _, d := range got {
		if strings.Contains(d, "P1-M1-") {
			t.Errorf("a task line appeared in the Decisions Log: %q", d)
		}
	}
}
