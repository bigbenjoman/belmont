package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// `belmont repair` is the healer for the damage #27 and #31 only made legible.
// The tests below are grouped by the property each one protects, and every one
// of them fails under a specific mutation of the code it names.

// gitFixture builds a REAL git repository holding a feature's PROGRESS.md, and
// commits whatever messages it is given.
//
// A real repository is not optional. `taskHasCommit` returns true when the git
// query fails, so a bare t.TempDir() reports "evidence present" for every task
// ever asked about — a test written against one proves nothing about the
// evidence path, in either direction.
func gitFixture(t *testing.T, progress string, commitMsgs ...string) string {
	t.Helper()
	root := t.TempDir()
	writeFeature(t, root, "demo", progress)
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.email", "t@t.t")
	gitRun(t, root, "config", "user.name", "t")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "init")
	// Everything lands on `main`, on purpose: that is where repair actually
	// runs, and it is precisely the shape in which merge-base scoping yields an
	// empty range.
	for _, m := range commitMsgs {
		gitRun(t, root, "commit", "-q", "--allow-empty", "-m", m)
	}
	return root
}

func progressOf(t *testing.T, root string) string {
	t.Helper()
	return readFile(t, filepath.Join(root, ".belmont", "features", "demo", "PROGRESS.md"))
}

// ----------------------------------------------------------------------------
// The mechanical tier
// ----------------------------------------------------------------------------

// The commit log is the whole point of the mechanical tier, and it is scoped to
// the FULL history reachable from HEAD rather than to `mergeBase..HEAD` the way
// runEvidenceCheck scopes it.
//
// That difference is load-bearing and easy to "tidy" away. Repair runs on the
// default branch, where merge-base(HEAD, main) is HEAD itself — so the scoped
// range is empty and every finding reports "no commit names it". The tier would
// appear to run and settle nothing, which is indistinguishable from a project
// where nothing was ever committed.
func TestRepairEvidenceIsNotScopedToTheMergeBase(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: the admin route\n",
		"P1-M1-1: add the admin route")

	findings, available := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if !available {
		t.Fatal("commit evidence reported unavailable inside a real git repository")
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(findings), findings)
	}
	ev := findings[0].Evidence
	if !ev.Checked || !ev.Found {
		t.Fatalf("the commit naming P1-M1-1 was not found (checked=%v found=%v) — "+
			"on the default branch a merge-base-scoped log is empty, so the mechanical tier settles nothing",
			ev.Checked, ev.Found)
	}
	if ev.Subject != "P1-M1-1: add the admin route" {
		t.Errorf("evidence subject = %q — repair prints this so a human can audit what it relied on", ev.Subject)
	}
}

// The mirror, and the one the whole design turns on: repair must NOT inherit
// taskHasCommit's fail-open. Outside a git repository there is no evidence, and
// "could not look" must never read as "the work happened".
func TestRepairDoesNotInheritFailOpen(t *testing.T) {
	root := t.TempDir()
	writeFeature(t, root, "demo", "### M1: Work\n- [?] P1-M1-1: unreadable\n")

	findings, available := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if available {
		t.Error("repair claimed commit evidence was available with no git repository present")
	}
	if findings[0].Evidence.Found {
		t.Error("repair reported commit evidence for a task in a directory with no git history at all")
	}
	if plans := mechanicalRepairs(findings); len(plans) != 0 {
		t.Errorf("repair auto-applied %d change(s) on evidence it could not gather: %+v", len(plans), plans)
	}

	// The control: taskHasCommit's fail-open is deliberate and must stay, or
	// the auto loop's evidence guard starts reverting real work on a shallow
	// clone. Repair opts out of it; nobody else does.
	if !taskHasCommit(root, "P1-M1-1", "") {
		t.Error("taskHasCommit stopped failing open — runEvidenceCheck will now revert [v] flips on any repo it cannot read")
	}
}

// Capped at [x]. `[v]` has its own evidence contract and `belmont reverify` is
// the only thing allowed to write it.
func TestRepairNeverMintsVerified(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: the thing\n",
		"P1-M1-1: implement and verify the thing")

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	plans := mechanicalRepairs(findings)
	if len(plans) != 1 {
		t.Fatalf("plans = %d, want 1", len(plans))
	}
	if plans[0].Action.Marker != "x" {
		t.Errorf("commit evidence produced [%s]; repair is capped at [x]", plans[0].Action.Marker)
	}

	// …and an agent proposing it is refused, in both spellings.
	for _, marker := range []string{"v", "V"} {
		_, rejected := validateRepairPlans(progressOf(t, root), findings,
			[]repairAction{{Line: findings[0].Line, Action: repairSetMarker, Marker: marker, Reason: "trying it on"}})
		if len(rejected) != 1 {
			t.Fatalf("marker %q was accepted; repair must never mint verified", marker)
		}
		if !strings.Contains(rejected[0].Reason, "reverify") {
			t.Errorf("the refusal for %q does not name the command that CAN write [v]: %s", marker, rejected[0].Reason)
		}
	}
}

// Only an unreadable marker already inside a milestone is applied without
// review. The commonest orphan is a sibling's completion quoted into a session
// log — it names a task ID that certainly has a commit, and moving it into the
// milestone would duplicate a live ID.
func TestRepairNeverAutoMovesAnOrphanEvenWithEvidence(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [x] P1-M1-1: the thing\n\n## Session History\n\n- [x] P1-M1-1: done, commit abc123\n",
		"P1-M1-1: implement the thing")

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if len(findings) != 1 || findings[0].Rule != ruleTaskOutsideMilestone {
		t.Fatalf("want one orphan finding, got %+v", findings)
	}
	if !findings[0].Evidence.Found {
		t.Fatal("fixture is wrong — the quoted line's ID must have a commit for this test to mean anything")
	}
	if plans := mechanicalRepairs(findings); len(plans) != 0 {
		t.Errorf("repair auto-acted on an orphaned log line: %+v", plans)
	}
}

// A cross-milestone ID is a filing question, not an evidence question.
func TestRepairNeverAutoMovesACrossMilestoneTask(t *testing.T) {
	root := gitFixture(t,
		"### M2: Surface\n- [ ] P1-M3-9: the export button\n\n### M3: Later\n- [ ] P1-M3-1: other\n",
		"P1-M3-9: build the export button")

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if len(findings) != 1 || findings[0].Rule != ruleCrossMilestoneTaskID {
		t.Fatalf("want one cross-milestone finding, got %+v", findings)
	}
	if findings[0].NamedMilestone != "M3" {
		t.Errorf("named milestone = %q, want M3 — repair and validate must agree on which milestone an ID names", findings[0].NamedMilestone)
	}
	if plans := mechanicalRepairs(findings); len(plans) != 0 {
		t.Errorf("repair relocated a task on commit evidence alone: %+v", plans)
	}
}

// ----------------------------------------------------------------------------
// What repair is allowed to touch
// ----------------------------------------------------------------------------

// Repair may only act on lines it flagged. Without this an agent's proposal is
// an unbounded edit of PROGRESS.md — the out-of-scope flip runScopeGuard exists
// to revert, except that the guard is not running here.
func TestRepairRefusesActionsOnLinesItDidNotFlag(t *testing.T) {
	doc := "### M1: Work\n- [ ] P1-M1-1: untouched\n- [?] P1-M1-2: unreadable\n"
	root := gitFixture(t, doc)
	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(doc))

	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M1-1", Action: repairSetMarker, Marker: "x", Reason: "not a finding"},
	})
	if len(plans) != 0 {
		t.Fatalf("repair accepted an edit to a line it never reported: %+v", plans)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "not one of the findings") {
		t.Errorf("refusal did not say why: %+v", rejected)
	}
}

// The line must still be byte-for-byte what was scanned. An editor, a
// concurrent agent, or a stale proposal file all mean the judgement was made
// about a different line.
func TestRepairSkipsALineThatChangedSinceTheScan(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: as scanned\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, TaskID: "P1-M1-1", Action: repairSetMarker, Marker: "x", Reason: "shipped"}})

	edited := "### M1: Work\n- [?] P1-M1-1: somebody rewrote this line\n"
	got, applied, warnings := applyRepairPlans(edited, plans, "2026-08-09")
	if got != edited {
		t.Errorf("repair wrote to a line it had not read:\n%s", got)
	}
	if len(applied) != 0 {
		t.Errorf("repair reported applying a change it skipped: %v", applied)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "changed since") {
		t.Errorf("the skip was silent, or unexplained: %v", warnings)
	}
}

// Milestone structure is immutable outside /belmont:tech-plan. Moving a task
// between milestones that already exist is repair's business; conjuring the
// destination is not.
func TestRepairNeverCreatesAMilestoneToReceiveAMove(t *testing.T) {
	doc := "### M2: Surface\n- [ ] P1-M3-9: the export button\n"
	findings := collectRepairFindings(doc)
	_, rejected := validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, TaskID: "P1-M3-9", Action: repairMoveMilestone, Milestone: "M3", Reason: "its ID names M3"}})
	if len(rejected) != 1 {
		t.Fatalf("a move into a milestone that does not exist was accepted")
	}
	if !strings.Contains(rejected[0].Reason, "tech-plan") {
		t.Errorf("the refusal does not route the user anywhere: %s", rejected[0].Reason)
	}
}

// Two tasks sharing an ID is the ambiguity every milestone-keyed reader refuses
// to guess at. A move must not create one.
func TestRepairRefusesAMoveThatWouldDuplicateATaskID(t *testing.T) {
	doc := "### M2: Surface\n- [ ] P1-M3-9: the export button\n\n### M3: Later\n- [x] P1-M3-9: the export button\n"
	findings := collectRepairFindings(doc)
	_, rejected := validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, TaskID: "P1-M3-9", Action: repairMoveMilestone, Milestone: "M3", Reason: "its ID names M3"}})
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "ambiguous") {
		t.Errorf("repair would have created a duplicate task ID: %+v", rejected)
	}
}

// A repeated `### M<n>:` heading makes every milestone-keyed lookup resolve
// arbitrarily. The scope guard and the evidence guard both decline on such a
// file; repair, which relocates lines by milestone, must decline too.
func TestRepairRefusesAmbiguousMilestones(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [?] P1-M1-1: a\n\n## Session History\n\n### M1: retry notes\n- log\n")
	err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run"})
	if err == nil {
		t.Fatal("repair ran against a PROGRESS.md with two `### M1:` headings")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Withdrawal
// ----------------------------------------------------------------------------

// Withdrawal is `[-]` PLUS the reason in `## Decisions Log`. A marker cannot
// carry why, and a withdrawal with no recorded reason is what makes the next
// reader either re-open the task or delete the line.
func TestRepairWithdrawalRecordsItsReason(t *testing.T) {
	doc := "### M1: Work\n- [ ] P1-M1-1: the export button\n\n## Decisions Log\n\n(none yet)\n"
	findings := collectRepairFindings("### M1: Work\n- [?] P1-M1-1: the export button\n\n## Decisions Log\n\n(none yet)\n")
	doc = "### M1: Work\n- [?] P1-M1-1: the export button\n\n## Decisions Log\n\n(none yet)\n"

	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M1-1", Action: repairWithdraw, Reason: "the export button was cut in the design review"},
	})
	if len(rejected) != 0 {
		t.Fatalf("unexpected refusal: %+v", rejected)
	}
	got, applied, _ := applyRepairPlans(doc, plans, "2026-08-09")

	if !strings.Contains(got, "- [-] P1-M1-1: the export button") {
		t.Errorf("the task was not marked withdrawn:\n%s", got)
	}
	if strings.Contains(got, "(none yet)") {
		t.Errorf("the placeholder survived above a real decision:\n%s", got)
	}
	if !strings.Contains(got, "2026-08-09 — P1-M1-1 withdrawn: the export button was cut in the design review") {
		t.Errorf("the reason did not reach ## Decisions Log:\n%s", got)
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v, want one summary line", applied)
	}
	// The reason must be reachable by the reader that exists for it.
	if d := parseDecisions(got, 10); len(d) != 1 {
		t.Errorf("belmont status will not show the withdrawal reason: %v", d)
	}
}

// Withdrawal is never a deletion, and the two must not be confusable. This is
// the property TestWithdrawalSurvivesTheMergeThatDeletionDoesNot pins for the
// merge; here it is pinned for the writer.
func TestRepairWithdrawalKeepsTheLine(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: dropped work\n- [ ] P1-M1-2: real work\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M1-1", Action: repairWithdraw, Reason: "superseded by the task below it"},
	})
	got, _, _ := applyRepairPlans(doc, plans, "2026-08-09")
	if !strings.Contains(got, "P1-M1-1") {
		t.Errorf("repair deleted a withdrawn task line — mergeProgressState carries deleted lines back in, so the task returns as outstanding work:\n%s", got)
	}
	if strings.Count(got, "P1-M1-2") != 1 {
		t.Errorf("the untouched task was disturbed:\n%s", got)
	}
}

// `[-]` via set_marker is refused, because that path has nowhere to put the
// reason.
func TestRepairRefusesWithdrawalWithoutAReason(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: a\n"
	findings := collectRepairFindings(doc)

	_, rejected := validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, Action: repairSetMarker, Marker: "-", Reason: "dropped"}})
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "Decisions Log") {
		t.Errorf("set_marker [-] was allowed to bypass the reason: %+v", rejected)
	}

	_, rejected = validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, Action: repairWithdraw, Reason: "   "}})
	if len(rejected) != 1 {
		t.Errorf("a withdrawal with a blank reason was accepted: %+v", rejected)
	}
}

// A file with no `## Decisions Log` still has to record the reason somewhere.
func TestRepairCreatesTheDecisionsLogWhenAbsent(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: a\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings,
		[]repairAction{{Line: 2, Action: repairWithdraw, Reason: "descoped"}})
	got, _, _ := applyRepairPlans(doc, plans, "2026-08-09")
	if !strings.Contains(got, "## Decisions Log") || !strings.Contains(got, "descoped") {
		t.Errorf("the reason was dropped because the section did not exist:\n%s", got)
	}
	// It must land AFTER the milestones region, or it becomes a section break
	// in the middle of the milestones and truncates them.
	if strings.Index(got, "## Decisions Log") < strings.Index(got, "P1-M1-1") {
		t.Errorf("the new section was inserted above the milestones:\n%s", got)
	}
}

// ----------------------------------------------------------------------------
// Moving a task
// ----------------------------------------------------------------------------

func TestRepairMovesATaskUnderTheMilestoneItsIDNames(t *testing.T) {
	doc := "### M2: Surface\n- [ ] P1-M2-1: keep\n- [ ] P1-M3-9: the export button\n\n" +
		"### M3: Later\n- [ ] P1-M3-1: other\n\n## Session History\n"
	findings := collectRepairFindings(doc)
	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 3, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "its ID names M3 and the work belongs there"},
	})
	if len(rejected) != 0 {
		t.Fatalf("unexpected refusal: %+v", rejected)
	}
	got, applied, warnings := applyRepairPlans(doc, plans, "2026-08-09")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v", applied)
	}

	ms := parseMilestones(got)
	byID := map[string][]string{}
	for _, m := range ms {
		for _, task := range m.Tasks {
			byID[m.ID] = append(byID[m.ID], task.ID)
		}
	}
	if strings.Join(byID["M2"], ",") != "P1-M2-1" {
		t.Errorf("M2 still holds the moved task: %v\n%s", byID["M2"], got)
	}
	if strings.Join(byID["M3"], ",") != "P1-M3-1,P1-M3-9" {
		t.Errorf("M3 = %v, want the moved task appended after its last task line\n%s", byID["M3"], got)
	}
	if !strings.Contains(got, "## Session History") {
		t.Errorf("the move ate content past the milestones region:\n%s", got)
	}
	if v := detectViolations("demo", ms); len(v) != 0 {
		t.Errorf("the move left a violation behind: %+v", v)
	}
}

// A milestone that exists but holds no tasks yet still has to be able to
// receive one — anchoring only on "the last task line" drops it.
func TestRepairMovesIntoAnEmptyMilestone(t *testing.T) {
	doc := "### M2: Surface\n- [ ] P1-M3-9: the export button\n\n### M3: Later\n\n## Session History\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"},
	})
	got, _, warnings := applyRepairPlans(doc, plans, "2026-08-09")
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	ms := parseMilestones(got)
	for _, m := range ms {
		if m.ID == "M3" {
			if len(m.Tasks) != 1 || m.Tasks[0].ID != "P1-M3-9" {
				t.Errorf("the task did not land in the empty milestone: %+v\n%s", m.Tasks, got)
			}
			return
		}
	}
	t.Errorf("M3 is gone:\n%s", got)
}

// ----------------------------------------------------------------------------
// End to end, through the command
// ----------------------------------------------------------------------------

func TestRepairMechanicalOnlyAppliesCommitEvidenceAndStopsThere(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: shipped\n- [?] P1-M1-2: never touched\n",
		"P1-M1-1: implement the shipped thing")

	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--mechanical-only"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	got := progressOf(t, root)
	if !strings.Contains(got, "- [x] P1-M1-1: shipped") {
		t.Errorf("the evidenced task was not repaired:\n%s", got)
	}
	if !strings.Contains(got, "- [?] P1-M1-2: never touched") {
		t.Errorf("repair guessed a state for a task with no evidence:\n%s", got)
	}
}

func TestRepairDryRunWritesNothing(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: shipped\n",
		"P1-M1-1: implement the shipped thing")
	before := progressOf(t, root)

	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if after := progressOf(t, root); after != before {
		t.Errorf("--dry-run modified PROGRESS.md:\n%s", after)
	}
}

// The whole point: a file `belmont validate` rejects becomes one it accepts,
// without anyone being asked to remember what a marker meant.
func TestRepairMakesACorruptedFileValidate(t *testing.T) {
	root := gitFixture(t,
		"# Progress: Demo\n\n## Milestones\n\n### M1: Work\n- [?] P1-M1-1: shipped\n- [?] P1-M1-2: also shipped\n",
		"P1-M1-1: implement one", "P1-M1-2: implement two")

	if err := runValidateCmd([]string{"--root", root}); err == nil {
		t.Fatal("fixture is wrong — validate must reject it before repair runs")
	}
	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--mechanical-only"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if err := runValidateCmd([]string{"--root", root}); err != nil {
		t.Errorf("validate still rejects the file after repair: %v\n%s", err, progressOf(t, root))
	}
}

// Nothing to repair must be a clean, silent success — a healer that reports
// findings on a healthy file trains people to ignore it.
func TestRepairIsSilentOnAHealthyFile(t *testing.T) {
	doc := "### M1: Work\n- [x] P1-M1-1: a\n- [-] P1-M1-2: dropped\n- [V] P1-M1-3: verified\n\n## Session History\n\n| Date | Action |\n"
	if f := collectRepairFindings(doc); len(f) != 0 {
		t.Errorf("repair found problems in a legal file: %+v", f)
	}
}

// A proposal is validated identically whoever wrote it, so a hand-written or
// stale proposal file cannot get past the bounds either.
func TestRepairAppliesAProposalFileThroughTheSameGate(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [?] P1-M1-1: the export button\n")
	proposal := filepath.Join(root, "proposal.json")
	if err := os.WriteFile(proposal, []byte(`{"repairs":[
		{"line":2,"task_id":"P1-M1-1","action":"withdraw","reason":"cut in the design review"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--apply-proposal", proposal, "--yes"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	got := progressOf(t, root)
	if !strings.Contains(got, "- [-] P1-M1-1") {
		t.Errorf("the proposal was not applied:\n%s", got)
	}
	if !strings.Contains(got, "cut in the design review") {
		t.Errorf("the reason was not recorded:\n%s", got)
	}
	if !fileExists(proposal) {
		t.Error("repair deleted a proposal file the user supplied")
	}
}

// ----------------------------------------------------------------------------
// The shared primitive
// ----------------------------------------------------------------------------

// taskHasCommit's POSITIVE branch had no test anywhere: `if false &&
// pattern.MatchString(msg)` left the whole suite green, and that mutation means
// every legitimate `[v]` flip is reverted by runEvidenceCheck and verification
// silently undoes itself.
func TestEvidenceGuardAcceptsAFlipWithACommit(t *testing.T) {
	// The guard scopes to mergeBase..HEAD, so its evidence must be on a branch
	// — which is where the auto loop always is when it runs.
	root := gitFixture(t, "### M1: Work\n- [x] P1-M1-1: a\n")
	gitRun(t, root, "checkout", "-q", "-b", "belmont/m1")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "P1-M1-1: implement a")

	pre := parseProgressSnapshot("P", "### M1: Work\n- [ ] P1-M1-1: a\n")
	post := parseProgressSnapshot("P", "### M1: Work\n- [v] P1-M1-1: a\n")
	if missing := findEvidenceMissingFlips(root, pre, post, "M1"); len(missing) != 0 {
		t.Errorf("a [v] flip WITH a commit naming the task was reverted: %+v", missing)
	}

	// Control: the same repository, a task no commit names.
	pre2 := parseProgressSnapshot("P", "### M1: Work\n- [ ] P1-M1-2: b\n")
	post2 := parseProgressSnapshot("P", "### M1: Work\n- [v] P1-M1-2: b\n")
	if missing := findEvidenceMissingFlips(root, pre2, post2, "M1"); len(missing) != 1 {
		t.Errorf("an un-evidenced flip was accepted: %+v", missing)
	}
}

// The word boundary is what stops P1-1 being credited by a commit for P1-12.
func TestCommitEvidenceRespectsWordBoundaries(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [ ] P1-1: a\n", "P1-12: implement something else")

	if ev := lookupCommitEvidence(root, "P1-1", ""); ev.Found {
		t.Errorf("P1-1 was credited by a commit for P1-12: %+v", ev)
	}
	if ev := lookupCommitEvidence(root, "P1-12", ""); !ev.Found {
		t.Error("P1-12 was not credited by its own commit — the boundary pattern is too strict")
	}
}

// The CLI-dispatched half of "both invocation paths". Belmont assembles this
// prompt itself and the tool's own skill discovery is bypassed, so the brief has
// to survive adaptPromptForTool for the two tools whose `/` is their own command
// palette — there a literal `/belmont:repair` lands as plain text.
func TestRepairBriefAdaptsForPromptRewritingTools(t *testing.T) {
	brief := buildRepairBrief("auth", "/tmp/p.json", []repairFinding{{
		Rule: ruleUnrecognisedMarker, TaskID: "P1-M1-1", Milestone: "M1",
		Marker: "?", Text: "a", Line: 2,
	}}, true)
	if !strings.HasPrefix(brief, "/belmont:repair --feature auth") {
		t.Fatalf("brief does not open with the skill invocation:\n%s", brief[:80])
	}
	for _, tool := range []string{"pi", "opencode"} {
		got := adaptPromptForTool(brief, tool)
		if strings.HasPrefix(got, "/belmont:repair") {
			t.Errorf("%s: the literal slash form survived — it lands as plain text there", tool)
		}
		if !strings.Contains(got, "repair") || !strings.Contains(got, "REPAIR BRIEF") {
			t.Errorf("%s: rewriting lost the brief:\n%s", tool, got[:200])
		}
		if !strings.Contains(got, "/tmp/p.json") {
			t.Errorf("%s: the proposal path was lost", tool)
		}
	}
	if got := adaptPromptForTool(brief, "claude"); got != brief {
		t.Error("claude's prompt was rewritten; it accepts the slash form verbatim")
	}
}

// A preview that reports different work from the real run is worse than no
// preview. --dry-run used to list every finding as needing a code read,
// including the ones the commit log already answered, because it skipped the
// mechanical tier's own output entirely.
func TestRepairDryRunReportsTheSameWorkAsTheRealRun(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: shipped\n- [?] P1-M1-2: never touched\n"
	root := gitFixture(t, doc, "P1-M1-1: implement the shipped thing")

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(doc))
	settled := mechanicalRepairs(findings)
	if len(settled) != 1 {
		t.Fatalf("fixture: mechanical tier settled %d, want 1", len(settled))
	}
	// What the preview says is left…
	previewLeft := needsReview(findings, settled)
	// …must equal what the real run leaves after writing.
	applied, _, _ := applyRepairPlans(doc, settled, "2026-08-09")
	realLeft := collectRepairFindings(applied)

	if len(previewLeft) != len(realLeft) {
		t.Fatalf("preview says %d finding(s) remain, the real run leaves %d", len(previewLeft), len(realLeft))
	}
	for i := range previewLeft {
		if previewLeft[i].TaskID != realLeft[i].TaskID {
			t.Errorf("finding %d: preview %q, real run %q", i, previewLeft[i].TaskID, realLeft[i].TaskID)
		}
	}
}

// Applying a proposal rebuilds the WHOLE file and writes it back, so a stale
// snapshot silently reverts anything edited in the meantime — and on the agent
// path "the meantime" is a subprocess that runs for minutes.
//
// This asserts the outcome end to end: the hand edit survives and the untouched
// finding is still repaired. The mechanism has two halves — runRepairCmd
// re-reads PROGRESS.md after the review tier returns, and applyRepairPlans
// refuses any plan whose line no longer matches Finding.Raw. The second half is
// pinned directly by TestRepairSkipsALineThatChangedSinceTheScan; without the
// first, it would be comparing a snapshot against itself and could never fire.
func TestRepairDoesNotOverwriteAConcurrentEdit(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [?] P1-M1-1: as scanned\n- [?] P1-M1-2: untouched\n")
	progressPath := filepath.Join(root, ".belmont", "features", "demo", "PROGRESS.md")

	proposal := filepath.Join(root, "proposal.json")
	if err := os.WriteFile(proposal, []byte(`{"repairs":[
		{"line":2,"task_id":"P1-M1-1","action":"withdraw","reason":"cut"},
		{"line":3,"task_id":"P1-M1-2","action":"withdraw","reason":"also cut"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Somebody edits line 2 after the scan the proposal was written against.
	if err := os.WriteFile(progressPath,
		[]byte("### M1: Work\n- [x] P1-M1-1: a human fixed this by hand\n- [?] P1-M1-2: untouched\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--apply-proposal", proposal, "--yes"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	got := progressOf(t, root)
	if !strings.Contains(got, "- [x] P1-M1-1: a human fixed this by hand") {
		t.Errorf("repair overwrote an edit made while the agent was running:\n%s", got)
	}
	if !strings.Contains(got, "- [-] P1-M1-2") {
		t.Errorf("the untouched line's repair was not applied:\n%s", got)
	}
}

// A commit message mentioning a task ID proves something happened, not that the
// work stands. `Revert "P1-M1-1: add X"` is git saying so in its own generated
// words — the one case the log settles in the other direction.
func TestRepairDoesNotTreatARevertAsEvidence(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: the admin route\n",
		"P1-M1-1: add the admin route",
		`Revert "P1-M1-1: add the admin route"`)

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if !findings[0].Evidence.Found {
		t.Fatal("the revert commit was not even found")
	}
	if !findings[0].Evidence.Reverting {
		t.Errorf("the newest commit naming the task is a revert and was not recognised as one: %+v", findings[0].Evidence)
	}
	if plans := mechanicalRepairs(findings); len(plans) != 0 {
		t.Errorf("repair marked reverted work as done: %+v", plans)
	}

	// The control: without the revert on top, the same commit does settle it.
	plain := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: the admin route\n",
		"P1-M1-1: add the admin route")
	pf, _ := attachCommitEvidence(plain, "demo", collectRepairFindings(progressOf(t, plain)))
	if len(mechanicalRepairs(pf)) != 1 {
		t.Error("the revert check swallowed a legitimate piece of evidence")
	}
}

// validateRepairPlans builds its "which IDs does this milestone already hold"
// map from the pre-apply document. Without claiming IDs as it accepts actions,
// two moves of the same ID into the same milestone both pass — neither is there
// yet — and the result is the duplicate the check exists to prevent.
func TestRepairRefusesASecondMoveThatWouldDuplicateAnID(t *testing.T) {
	doc := "### M1: A\n- [?] P1-M3-9: first copy\n- [?] P1-M3-9: second copy\n\n### M3: Dest\n- [ ] P1-M3-1: other\n"
	findings := collectRepairFindings(doc)
	if len(findings) != 2 {
		t.Fatalf("fixture: %d findings, want 2", len(findings))
	}
	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"},
		{Line: 3, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"},
	})
	if len(plans) != 1 || len(rejected) != 1 {
		t.Fatalf("accepted %d move(s) of the same ID into one milestone, rejected %d", len(plans), len(rejected))
	}
	if !strings.Contains(rejected[0].Reason, "ambiguous") {
		t.Errorf("refused for the wrong reason: %s", rejected[0].Reason)
	}
}

// An action that names a task ID must be refused when the line it targets has
// none — otherwise an agent off by one line has `withdraw` applied to whatever
// ID-less finding sits there, marker and Decisions Log entry included.
func TestRepairRefusesAnIDdActionAimedAtAnIDlessLine(t *testing.T) {
	doc := "### M1: Work\n- [ ] P1-M1-1: real\n\n## Session History\n\n- [?] a bullet with no task ID\n"
	findings := collectRepairFindings(doc)
	if len(findings) != 1 || findings[0].TaskID != "" {
		t.Fatalf("fixture: want one ID-less finding, got %+v", findings)
	}
	_, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: findings[0].Line, TaskID: "P1-M1-1", Action: repairWithdraw, Reason: "off by one"},
	})
	if len(rejected) != 1 {
		t.Fatal("an action naming P1-M1-1 was applied to a line that holds no task ID")
	}
}

// `## Decisions Log` is not always the last section. A milestone header is
// deliberately not a section break, so scanning for one alone ran past
// `### M1:` and appended the entry at EOF — inside the last milestone.
func TestRepairWritesTheDecisionAboveTheMilestones(t *testing.T) {
	// NO `## Milestones` between the log and the milestone header — with one
	// there, the scan stops at that column-zero heading and the milestone check
	// is never needed, so the fixture would pass with the fix removed.
	doc := "# Progress\n\n## Decisions Log\n\n(none yet)\n\n### M1: Work\n- [?] P1-M1-1: dropped\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings,
		[]repairAction{{Line: findings[0].Line, TaskID: "P1-M1-1", Action: repairWithdraw, Reason: "descoped"}})
	got, _, _ := applyRepairPlans(doc, plans, "2026-08-09")

	logIdx := strings.Index(got, "2026-08-09 — P1-M1-1 withdrawn: descoped")
	if logIdx < 0 {
		t.Fatalf("the reason was not recorded:\n%s", got)
	}
	if logIdx > strings.Index(got, "### M1: Work") {
		t.Errorf("the decision landed below the milestone header, inside the milestone block:\n%s", got)
	}
	// …and the milestones still parse afterwards.
	ms := parseMilestones(got)
	if len(ms) != 1 || len(ms[0].Tasks) != 1 {
		t.Errorf("the write disturbed the milestones region: %+v\n%s", ms, got)
	}
	// …and the reader that exists for the log can see the entry.
	if d := parseDecisions(got, 10); len(d) != 1 {
		t.Errorf("belmont status will not show the reason: %v", d)
	}
}

// The report and the JSON must describe the same changes. A plan that was
// skipped, or that writes nothing by definition, is not an applied change.
func TestRepairReportsOnlyWhatItWrote(t *testing.T) {
	doc := "### M1: Work\n- [?] P1-M1-1: a\n- [?] P1-M1-2: b\n"
	findings := collectRepairFindings(doc)
	plans, _ := validateRepairPlans(doc, findings, []repairAction{
		{Line: 2, TaskID: "P1-M1-1", Action: repairSetMarker, Marker: "x", Reason: "shipped"},
		{Line: 3, TaskID: "P1-M1-2", Action: repairEscalate, Reason: "cannot tell"},
	})
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(plans))
	}
	_, applied, _ := applyRepairPlans(doc, plans, "2026-08-09")
	if len(applied) != 1 {
		t.Fatalf("applied = %d, want 1 — an escalation writes nothing: %+v", len(applied), applied)
	}
	if applied[0].Action.Action != repairSetMarker {
		t.Errorf("the wrong plan was reported as applied: %+v", applied[0])
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

// The dry-run preview must exclude what the mechanical tier settles, through
// the COMMAND — not just through the helper. TestRepairDryRunReportsTheSame
// WorkAsTheRealRun pins needsReview directly; this pins the call site, which is
// where the `nil` that broke it lived.
func TestRepairDryRunJSONExcludesSettledFindings(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [?] P1-M1-1: shipped\n- [?] P1-M1-2: never touched\n",
		"P1-M1-1: implement the shipped thing")

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair: %v", runErr)
	}
	var got struct {
		WouldApply  []map[string]any `json:"would_apply"`
		NeedsReview []struct {
			TaskID string `json:"task_id"`
		} `json:"needs_review"`
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	if got.Changed {
		t.Error("--dry-run reported the file as changed")
	}
	if len(got.WouldApply) != 1 {
		t.Errorf("would_apply = %d, want 1 — the preview must say what the commit log settles", len(got.WouldApply))
	}
	if len(got.NeedsReview) != 1 || got.NeedsReview[0].TaskID != "P1-M1-2" {
		t.Errorf("needs_review = %+v, want only P1-M1-2 — a finding the commit log settles does not need a code read", got.NeedsReview)
	}
}

// Task IDs are feature-local; the commit log is not. `P1-M1-1` exists in
// essentially every feature, so an unscoped search can match a commit that
// belongs to entirely different work — and the mechanical tier writes without
// asking.
func TestRepairRefusesEvidenceAnotherFeatureCouldOwn(t *testing.T) {
	root := gitFixture(t, "### M1: Invoices\n- [?] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the login form")
	// A sibling feature that also holds P1-M1-1 — the commit could be its.
	writeFeature(t, root, "auth", "### M1: Login\n- [x] P1-M1-1: build the login form\n")

	findings, _ := attachCommitEvidence(root, "demo", collectRepairFindings(progressOf(t, root)))
	if !findings[0].Evidence.Found {
		t.Fatal("fixture: the commit must be found, or there is no ambiguity to refuse")
	}
	if !findings[0].Evidence.Ambiguous {
		t.Error("a task ID two features both claim was treated as unambiguous evidence")
	}
	if plans := mechanicalRepairs(findings); len(plans) != 0 {
		t.Errorf("repair marked a never-built task done on another feature's commit: %+v", plans)
	}

	// The control: with no sibling claiming the ID, the same commit still
	// settles it. Scoping this away entirely would make the tier inert — which
	// is what a pathspec on `.belmont/features/<slug>` would do, since
	// `.belmont/` is assume-unchanged in worktrees and work commits never
	// touch it.
	solo := gitFixture(t, "### M1: Login\n- [?] P1-M1-1: build the login form\n",
		"P1-M1-1: build the login form")
	sf, _ := attachCommitEvidence(solo, "demo", collectRepairFindings(progressOf(t, solo)))
	if len(mechanicalRepairs(sf)) != 1 {
		t.Error("the ambiguity check swallowed evidence in a single-feature project")
	}
}

// moveTaskLines has to place a line whose anchor is itself being moved out of
// the way. Without the fallback the insertion point is never emitted, so BOTH
// lines vanish — and the caller reports them applied, because the move was
// planned rather than dropped.
func TestRepairMovesTwoTasksWhoseAnchorIsAlsoMoving(t *testing.T) {
	// The two milestones swap a task, so EACH one's last task line is the line
	// leaving it. Both anchors are therefore "moving", and each has to fall
	// back to its milestone header. A destination that is merely empty does
	// not reach this branch — its lastTaskIdx never existed.
	doc := "### M2: Surface\n- [ ] P1-M2-1: stays\n- [ ] P1-M3-9: beta\n\n" +
		"### M3: Later\n- [ ] P1-M3-1: stays\n- [ ] P1-M2-4: gamma\n\n## Session History\n"
	findings := collectRepairFindings(doc)
	if len(findings) != 2 {
		t.Fatalf("fixture: %d findings, want 2: %+v", len(findings), findings)
	}
	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 3, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"},
		{Line: 7, TaskID: "P1-M2-4", Action: repairMoveMilestone, Reason: "belongs to M2"},
	})
	if len(rejected) != 0 || len(plans) != 2 {
		t.Fatalf("plans=%d rejected=%+v", len(plans), rejected)
	}
	got, applied, warnings := applyRepairPlans(doc, plans, "2026-08-09")

	for _, id := range []string{"P1-M3-9", "P1-M2-4", "P1-M2-1", "P1-M3-1"} {
		if !strings.Contains(got, id) {
			t.Errorf("%s was DELETED by the move:\n%s", id, got)
		}
	}
	if len(applied) != 2 {
		t.Errorf("applied = %d, want 2 — the report must match the file", len(applied))
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	byID := map[string][]string{}
	for _, m := range parseMilestones(got) {
		for _, task := range m.Tasks {
			byID[m.ID] = append(byID[m.ID], task.ID)
		}
	}
	// Order is not the property under test — placement is. Both anchors fell
	// back to their milestone header, so the moved line lands first.
	for ms, want := range map[string][]string{"M2": {"P1-M2-1", "P1-M2-4"}, "M3": {"P1-M3-1", "P1-M3-9"}} {
		have := append([]string{}, byID[ms]...)
		sort.Strings(have)
		if strings.Join(have, ",") != strings.Join(want, ",") {
			t.Errorf("%s = %v, want %v:\n%s", ms, byID[ms], want, got)
		}
	}
	if v := detectViolations("demo", parseMilestones(got)); len(v) != 0 {
		t.Errorf("the swap left a violation behind: %+v", v)
	}
	if !strings.Contains(got, "## Session History") {
		t.Errorf("the move ate the region boundary:\n%s", got)
	}
}

// ----------------------------------------------------------------------------
// The verified-without-evidence audit
// ----------------------------------------------------------------------------

// The mirror of runEvidenceCheck, for the half it cannot see. That guard
// compares a phase's before and after, so a `[v]` already on disk when the run
// started is never a flip and is never audited — by anything, ever.
func TestRepairAuditsVerifiedTasksNoCommitNames(t *testing.T) {
	root := gitFixture(t,
		"### M1: Work\n- [v] P1-M1-1: committed\n- [V] P1-M1-2: never committed\n- [x] P1-M1-3: only done\n",
		"P1-M1-1: implement the committed thing")

	got := auditVerifiedWithoutEvidence(root, progressOf(t, root))
	if len(got) != 1 {
		t.Fatalf("audit = %+v, want only P1-M1-2", got)
	}
	if got[0].TaskID != "P1-M1-2" {
		t.Errorf("audited %s, want P1-M1-2", got[0].TaskID)
	}
	if got[0].Rule != ruleVerifiedWithoutEvidence {
		t.Errorf("rule = %q", got[0].Rule)
	}
	// Case-insensitivity is the whole reason this gap became visible: `[V]` used
	// to be an unrecognised marker that blocked the loop.
	if got[0].Marker != "V" {
		t.Errorf("marker = %q — the audit must see the capital form", got[0].Marker)
	}
}

// It is an audit, not a parse finding, and the difference is load-bearing:
// these lines parse, `belmont validate` is happy with them, and folding them
// into collectRepairFindings would mean a clean file never reads clean.
func TestVerifiedAuditIsNotAParseFinding(t *testing.T) {
	doc := "### M1: Work\n- [v] P1-M1-1: never committed\n"
	if f := collectRepairFindings(doc); len(f) != 0 {
		t.Errorf("the audit leaked into the parse findings: %+v", f)
	}
	root := writeValidateFixture(t, "# Progress\n\n"+doc)
	if err := runValidateCmd([]string{"--root", root}); err != nil {
		t.Errorf("validate now fails on a file that parses perfectly: %v", err)
	}
}

// NEVER MECHANICAL. No commit means no commit; it does not mean the work is
// absent. verify-evidence.md records commit-less tasks as a known rough edge —
// a docs- or config-only task can be genuinely verified with nothing naming it.
func TestRepairNeverActsOnTheVerifiedAuditByItself(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [v] P1-M1-1: never committed\n")

	unearned := auditVerifiedWithoutEvidence(root, progressOf(t, root))
	if len(unearned) != 1 {
		t.Fatalf("fixture: audit = %d, want 1", len(unearned))
	}
	if plans := mechanicalRepairs(unearned); len(plans) != 0 {
		t.Errorf("repair demoted a verified task with no review: %+v", plans)
	}
	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--mechanical-only"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if got := progressOf(t, root); !strings.Contains(got, "- [v] P1-M1-1") {
		t.Errorf("the mechanical tier changed a verified task:\n%s", got)
	}
}

// "Could not look" must not print as "no evidence" — the same rule the
// mechanical tier follows.
func TestVerifiedAuditIsSilentWithoutAGitRepo(t *testing.T) {
	root := t.TempDir()
	writeFeature(t, root, "demo", "### M1: Work\n- [v] P1-M1-1: verified\n")
	if got := auditVerifiedWithoutEvidence(root, progressOf(t, root)); len(got) != 0 {
		t.Errorf("the audit accused a task in a directory with no git history: %+v", got)
	}
}

// The audit reaches the review tier even though it is not in `remaining` — the
// gate refuses to act on a line it was not shown.
func TestVerifiedAuditCanBeSettledByAProposal(t *testing.T) {
	root := gitFixture(t, "### M1: Work\n- [v] P1-M1-1: never committed\n- [v] P1-M1-2: also never committed\n")
	proposal := filepath.Join(root, "p.json")
	if err := os.WriteFile(proposal, []byte(`{"repairs":[
		{"line":2,"task_id":"P1-M1-1","action":"set_marker","marker":"x","reason":"the handler does not exist"},
		{"line":3,"task_id":"P1-M1-2","action":"leave","reason":"docs-only; the page is there"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--apply-proposal", proposal, "--yes"}); err != nil {
		t.Fatalf("repair: %v", err)
	}
	got := progressOf(t, root)
	if !strings.Contains(got, "- [x] P1-M1-1") {
		t.Errorf("the demote was not applied:\n%s", got)
	}
	if !strings.Contains(got, "- [v] P1-M1-2") {
		t.Errorf("a `leave` verdict changed the line:\n%s", got)
	}

	// …and repair still refuses to write a [v], including back onto a task it
	// just demoted. `belmont reverify` is the only thing that may.
	back := filepath.Join(root, "back.json")
	if err := os.WriteFile(back, []byte(`{"repairs":[
		{"line":2,"task_id":"P1-M1-1","action":"set_marker","marker":"v","reason":"putting it back"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_ = runRepairCmd([]string{"--root", root, "--feature", "demo", "--apply-proposal", back, "--yes"})
	if got := progressOf(t, root); strings.Contains(got, "- [v] P1-M1-1") {
		t.Errorf("repair minted a verified marker:\n%s", got)
	}
}

// One line can produce two findings: a `[v]` filed under the wrong milestone
// that no commit names is both a cross-milestone parse finding and a
// verified-without-evidence audit finding. The gate keys findings by line, so
// the second silently replaced the first — and then refused a legitimate move
// because the audit entry it kept carried no destination.
func TestRepairHandlesALineThatIsBothFindings(t *testing.T) {
	root := gitFixture(t, "### M2: Surface\n- [v] P1-M3-9: filed wrong, never committed\n\n### M3: Later\n- [ ] P1-M3-1: other\n")
	doc := progressOf(t, root)

	parse, _ := attachCommitEvidence(root, "demo", collectRepairFindings(doc))
	audit := auditVerifiedWithoutEvidence(root, doc)
	if len(parse) != 1 || len(audit) != 1 || parse[0].Line != audit[0].Line {
		t.Fatalf("fixture: want one of each on the same line, got parse=%+v audit=%+v", parse, audit)
	}

	reviewable := append(append([]repairFinding{}, parse...), audit...)
	plans, rejected := validateRepairPlans(doc, reviewable,
		[]repairAction{{Line: parse[0].Line, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"}})
	if len(plans) != 1 {
		t.Fatalf("a legitimate move was refused: %+v", rejected)
	}
	got, applied, warnings := applyRepairPlans(doc, plans, "2026-08-09")
	if len(applied) != 1 || len(warnings) != 0 {
		t.Fatalf("applied=%d warnings=%v", len(applied), warnings)
	}
	for _, m := range parseMilestones(got) {
		if m.ID == "M3" && len(m.Tasks) != 2 {
			t.Errorf("M3 = %+v, want its own task plus the moved one:\n%s", m.Tasks, got)
		}
		if m.ID == "M2" && len(m.Tasks) != 0 {
			t.Errorf("M2 still holds the moved task: %+v\n%s", m.Tasks, got)
		}
	}
	// The audit finding alone must still carry enough to validate a move, in
	// case it is the one the gate keeps.
	if audit[0].NamedMilestone != "M3" {
		t.Errorf("audit finding NamedMilestone = %q, want M3", audit[0].NamedMilestone)
	}
}
