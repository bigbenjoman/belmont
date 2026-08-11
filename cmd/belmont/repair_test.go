package main

import (
	"encoding/json"
	"os"
	"os/exec"
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

	got := auditVerifiedWithoutEvidence(root, "demo", progressOf(t, root))
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

	unearned := auditVerifiedWithoutEvidence(root, "demo", progressOf(t, root))
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
	if got := auditVerifiedWithoutEvidence(root, "demo", progressOf(t, root)); len(got) != 0 {
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
	audit := auditVerifiedWithoutEvidence(root, "demo", doc)
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

// `evidence_available` is a property of the REPOSITORY, not of the findings.
//
// attachCommitEvidence used to raise the flag only inside its per-finding loop,
// so "no finding carried a task ID" was indistinguishable from "the commit log
// could not be read". Both of the cases below reach the report with an empty or
// ID-less finding list, and both used to claim the log was unreadable: the text
// output told the user to run repair from inside the git working tree they were
// already inside, and buildRepairBrief told the review tier that NONE of the
// findings had been checked against a log it had just read to build them.
//
// Driven through the command in both directions, because pinning only the true
// case leaves the condition free — `return true` would satisfy it.
func TestRepairReportsWhetherTheCommitLogWasActuallyReadable(t *testing.T) {
	// Parses perfectly, so there are no parse findings at all — only the audit,
	// which exists solely because the log WAS read.
	root := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: no commit ever named this\n",
		"unrelated work")
	var runErr error
	out := captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair: %v", runErr)
	}
	var got struct {
		EvidenceAvailable bool `json:"evidence_available"`
		Unearned          []struct {
			TaskID string `json:"task_id"`
		} `json:"verified_without_evidence"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	if len(got.Unearned) != 1 {
		t.Fatalf("verified_without_evidence = %d, want 1 — the fixture must reach the audit", len(got.Unearned))
	}
	if !got.EvidenceAvailable {
		t.Error("evidence_available = false in a real git repository whose log was read to produce the audit above")
	}

	// The other direction: no git at all. The flag must still go down, or the
	// fix is just `return true`.
	bare := t.TempDir()
	writeFeature(t, bare, "demo", "### M1: Login\n- [?] P1-M1-1: unreadable\n")
	out = captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", bare, "--feature", "demo", "--dry-run", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair (no git): %v", runErr)
	}
	got.EvidenceAvailable = true
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	if got.EvidenceAvailable {
		t.Error("evidence_available = true outside a git working tree")
	}

	// And the case only commitLogReadable can answer: a work tree with no
	// commits yet. isGitWorkTree says true here, so the check above never
	// reaches this branch — without this fixture `return true` passes.
	fresh := t.TempDir()
	writeFeature(t, fresh, "demo", "### M1: Login\n- [?] P1-M1-1: unreadable\n")
	gitRun(t, fresh, "init", "-q", "-b", "main")
	gitRun(t, fresh, "config", "user.email", "t@t.t")
	gitRun(t, fresh, "config", "user.name", "t")
	if !isGitWorkTree(fresh) {
		t.Fatal("fixture is not a work tree, so it does not reach the branch this case exists for")
	}
	out = captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", fresh, "--feature", "demo", "--dry-run", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair (no commits): %v", runErr)
	}
	got.EvidenceAvailable = true
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	if got.EvidenceAvailable {
		t.Error("evidence_available = true in a work tree with no commits — `git log` fails there")
	}
}

// The brief is the only thing the CLI-dispatched review tier reads, so a false
// statement in it is a false statement to the agent. Asserted on the bytes
// buildRepairBrief produces rather than on the flag, because the flag is one
// call away from the sentence and the sentence is what does the damage.
func TestRepairBriefDoesNotClaimAnUnreadLogWhenItWasRead(t *testing.T) {
	root := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: no commit ever named this\n",
		"unrelated work")
	progress := progressOf(t, root)
	findings := collectRepairFindings(progress)
	findings, available := attachCommitEvidence(root, "demo", findings)
	if len(findings) != 0 {
		t.Fatalf("fixture produced %d parse findings, want 0 — it must exercise the ID-less path", len(findings))
	}
	unearned := auditVerifiedWithoutEvidence(root, "demo", progress)
	if len(unearned) != 1 {
		t.Fatalf("audit produced %d findings, want 1", len(unearned))
	}
	brief := buildRepairBrief("demo", filepath.Join(root, "p.json"), unearned, available)
	if strings.Contains(brief, "could not be read here") {
		t.Errorf("the brief tells the review tier the commit log could not be read, but the audit it carries was built from that log:\n%s", brief)
	}
}

// Repair must report the findings it CREATES, not only the ones it was shown.
//
// `collectRepairFindings` reports one finding per line — an unreadable marker
// `continue`s before the cross-milestone check, because a line whose state
// cannot be read has a state problem before it has a location problem. So a
// `[?] P1-M2-7` filed under `### M1:` is only ever the marker problem. The
// mechanical tier settles the marker, and the SAME LINE is now a cross-milestone
// finding — which `needsReview` then subtracted again, because it filters by
// line number and that line was in `plans`.
//
// The result was a report that said "Nothing left needs a code read" and pointed
// the user at `belmont validate`, which exited 1 on the line repair had just
// written.
func TestRepairSurfacesAFindingItCreated(t *testing.T) {
	root := gitFixture(t,
		"### M1: Login\n- [?] P1-M2-7: add the logout route\n\n### M2: Logout\n- [ ] P1-M2-1: something else\n",
		"P1-M2-7: add the logout route")

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", root, "--feature", "demo", "--mechanical-only", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair: %v", runErr)
	}
	var got struct {
		Applied []struct {
			Action struct {
				Marker string `json:"marker"`
			} `json:"action"`
		} `json:"applied"`
		NeedsReview []struct {
			Rule   string `json:"rule"`
			TaskID string `json:"task_id"`
		} `json:"needs_review"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	if len(got.Applied) != 1 || got.Applied[0].Action.Marker != "x" {
		t.Fatalf("applied = %+v, want the mechanical [?] -> [x] — without it the fixture never creates the second finding", got.Applied)
	}
	if len(got.NeedsReview) != 1 ||
		got.NeedsReview[0].Rule != ruleCrossMilestoneTaskID ||
		got.NeedsReview[0].TaskID != "P1-M2-7" {
		t.Errorf("needs_review = %+v, want the cross-milestone finding repair's own write created", got.NeedsReview)
	}

	// And the file really is in that state — the JSON is not the only claim.
	if v := detectViolations("demo", parseMilestones(progressOf(t, root))); len(v) == 0 {
		t.Error("fixture no longer produces a violation, so this test cannot fail for the reason it names")
	}
}

// All three of repairJSON's lists declared without `omitempty` must marshal as
// `[]`, never `null` — the interactive skill iterates them straight out of the
// JSON. `applied` was the one missed: it is null on every dry run, which is the
// first command the skill runs.
func TestRepairJSONNeverEmitsNullForItsLists(t *testing.T) {
	// A file with NO parse findings and one audit finding: every one of the
	// three lists is empty at once, which is the only shape that distinguishes
	// `[]` from `null` for all three. A fixture with findings leaves two of them
	// non-nil for free.
	root := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: nothing ever named this\n", "unrelated work")
	var runErr error
	out := captureStdout(t, func() {
		runErr = runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run", "--format", "json"})
	})
	if runErr != nil {
		t.Fatalf("repair: %v", runErr)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"findings", "needs_review", "applied"} {
		v, ok := raw[key]
		if !ok {
			t.Errorf("%q missing from the JSON entirely", key)
			continue
		}
		if string(v) == "null" {
			t.Errorf("%q marshalled as null, not []", key)
		}
	}
}

// The audit prints the marker AS WRITTEN, like every other Belmont diagnostic.
// `[V]` is the spelling that made this gap visible — it used to be an
// unrecognised marker that blocked the loop and now reads as the terminal state
// — and a report that renders it `[v]` hides the string the reader has to go
// and find in the file.
func TestVerifiedAuditPrintsTheMarkerAsWritten(t *testing.T) {
	root := gitFixture(t, "### M1: Login\n- [V] P1-M1-1: nothing ever named this\n", "unrelated")
	out := captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	if !strings.Contains(out, "[V] PROGRESS.md") {
		t.Errorf("the audit rendered the marker as something other than the [V] on disk:\n%s", out)
	}
}

// commitNamedTaskIDs and lookupCommitEvidence must agree about what counts as a
// mention, which is what the comment on commitTaskIDRe claims. Without the left
// word boundary the audit read `P1-M1-1` out of the middle of `feat-P1-M1-1`
// and fell silent on a `[v]` the per-ID primitive calls unproven.
//
// Both directions, because pinning only the silence would be satisfied by an
// audit that never fires.
func TestVerifiedAuditUsesTheSameWordBoundariesAsTheEvidenceGuard(t *testing.T) {
	embedded := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: the login route\n",
		"feat-P1-M1-1: something else entirely")
	if ev := lookupCommitEvidence(embedded, "P1-M1-1", ""); ev.Found {
		t.Fatalf("lookupCommitEvidence matched inside a longer token — the fixture no longer separates the two")
	}
	if got := auditVerifiedWithoutEvidence(embedded, "demo", progressOf(t, embedded)); len(got) != 1 {
		t.Errorf("audit reported %d findings, want 1 — `feat-P1-M1-1` is not a mention of P1-M1-1", len(got))
	}

	named := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: the login route\n",
		"P1-M1-1: build the login route")
	if got := auditVerifiedWithoutEvidence(named, "demo", progressOf(t, named)); len(got) != 0 {
		t.Errorf("audit reported %d findings for a task a commit plainly names, want 0", len(got))
	}
}

// "Unresolved" means "you still have to act on this", and a `leave` verdict on
// an orphan is precisely the case where you do not: `task_outside_milestone` is
// a warning, `belmont validate` exits 0 on it, and the review tier has just
// ruled the line is not a task.
//
// Counting the re-scan alone ended the run with "1 finding(s) unresolved — edit
// those lines by hand" printed directly under "left as written (not a task)"
// for the same line. Reasoning from the pre-review list instead is the other
// failure: it counts findings repair has since fixed. The fixture holds one of
// each so both mutations die on it.
func TestUnresolvedCountMatchesTheVerdictsItJustPrinted(t *testing.T) {
	// Four findings, one of each verdict, so no two measures agree by accident:
	//   line 2  [?] -> set_marker " "  the tier FIXES it        (gone from the file)
	//   line 3  [?] escalate           unsettled                (still counts)
	//   line 4  [?] leave              still unreadable         (still counts)
	//   line 8  [x] orphan, leave      ruled not a task         (does NOT count)
	// Correct answer: 2. Re-scan alone says 3. len(remaining) says 4.
	// Subtracting every `leave` rather than only the orphans says 1.
	root := gitFixture(t,
		"### M1: Login\n- [?] P1-M1-1: settled by the tier\n- [?] P1-M1-2: nobody can settle this\n- [?] P1-M1-3: illustrative, but still unreadable\n\n## Session History\n\n- [x] P1-M1-9: quoted from a sibling's log\n",
		"unrelated work")
	proposal := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(proposal, []byte(`{"repairs":[
	  {"line":2,"task_id":"P1-M1-1","action":"set_marker","marker":" ","reason":"nothing in the code does this yet"},
	  {"line":3,"task_id":"P1-M1-2","action":"escalate","reason":"the code does not settle it"},
	  {"line":4,"task_id":"P1-M1-3","action":"leave","reason":"an illustration"},
	  {"line":8,"task_id":"P1-M1-9","action":"leave","reason":"a quoted log line, not a task"}
	]}`), 0644)

	out := captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--apply-proposal", proposal, "--yes"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	if !strings.Contains(out, "2 finding(s) unresolved") {
		t.Errorf("want exactly 2 unresolved — the two markers still unreadable, not the orphan the tier ruled is not a task and not the one it fixed:\n%s", out)
	}
	for _, wrong := range []string{"1 finding(s) unresolved", "3 finding(s) unresolved", "4 finding(s) unresolved"} {
		if strings.Contains(out, wrong) {
			t.Errorf("unresolved count is %q:\n%s", wrong, out)
		}
	}
}

// The audit applies the same cross-feature rule the mechanical tier does.
//
// Task IDs are feature-local, the commit log is not. A sibling's commit for its
// own P1-M1-1 is not evidence about this feature's P1-M1-1, and clearing the
// audit on it silences the finding for every feature that shares the ID — which,
// under the shipped template, is every feature. That is `taskIDsClaimedElsewhere`
// pointed the other way: there it wrote a state nobody could justify, here it
// withheld a question nobody else asks.
//
// All three directions, because pinning only the ambiguous case would be
// satisfied by an audit that reports everything, and pinning only the silence by
// one that reports nothing.
func TestVerifiedAuditRefusesEvidenceAnotherFeatureCouldOwn(t *testing.T) {
	root := gitFixture(t, "### M1: Invoices\n- [v] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the login form")
	// A sibling that also holds P1-M1-1 — the commit could be its.
	writeFeature(t, root, "auth", "### M1: Login\n- [x] P1-M1-1: build the login form\n")

	got := auditVerifiedWithoutEvidence(root, "demo", progressOf(t, root))
	if len(got) != 1 {
		t.Fatalf("audit reported %d findings, want 1 — a sibling's commit is not evidence about this feature", len(got))
	}
	if !got[0].Evidence.Ambiguous || !got[0].Evidence.Found {
		t.Errorf("evidence = %+v, want found+ambiguous so the report can name the commit and say why it does not settle", got[0].Evidence)
	}
	if got[0].Evidence.SHA == "" {
		t.Error("no SHA on the ambiguous finding — the report prints it, and 'some commit somewhere' is not actionable")
	}

	// Same file, no sibling: the commit settles it and the audit is silent.
	alone := gitFixture(t, "### M1: Invoices\n- [v] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the invoice table")
	if got := auditVerifiedWithoutEvidence(alone, "demo", progressOf(t, alone)); len(got) != 0 {
		t.Errorf("audit reported %d findings on an unambiguous ID a commit names, want 0", len(got))
	}

	// And a sibling that does NOT hold the ID leaves the tier working.
	unrelated := gitFixture(t, "### M1: Invoices\n- [v] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the invoice table")
	writeFeature(t, unrelated, "auth", "### M1: Login\n- [x] P2-M4-9: build the login form\n")
	if got := auditVerifiedWithoutEvidence(unrelated, "demo", progressOf(t, unrelated)); len(got) != 0 {
		t.Errorf("audit reported %d findings, want 0 — no other feature claims this ID", len(got))
	}
}

// One line, one finding, one action.
//
// A `[v]` filed under a milestone its ID does not name that no commit names is
// both a cross-milestone parse finding and an audit finding. Handing the review
// tier two entries for that line asked it for two actions on a line the gate
// accepts exactly one action for: an agent obeying "one entry per finding"
// earned "line N already has an action", and whichever entry `byLine` happened
// to keep decided whether a `leave` printed "left verified" or "left as written
// (not a task)".
func TestOneFindingPerLineReachesTheReviewTier(t *testing.T) {
	root := gitFixture(t,
		"### M1: Login\n- [ ] P1-M1-1: real work\n\n### M2: Logout\n- [v] P1-M1-9: filed under M2, ID names M1\n",
		"unrelated work")
	progress := progressOf(t, root)
	parse := collectRepairFindings(progress)
	parse, _ = attachCommitEvidence(root, "demo", parse)
	unearned := auditVerifiedWithoutEvidence(root, "demo", progress)
	if len(parse) != 1 || len(unearned) != 1 || parse[0].Line != unearned[0].Line {
		t.Fatalf("fixture must put both findings on ONE line: parse=%+v unearned=%+v", parse, unearned)
	}

	out := captureStdout(t, func() {
		if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run", "--format", "json"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	var got struct {
		NeedsReview []struct {
			Line           int  `json:"line"`
			AlsoUnverified bool `json:"also_verified_without_evidence"`
		} `json:"needs_review"`
		Unearned []struct {
			Line int `json:"line"`
		} `json:"verified_without_evidence"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse repair JSON: %v\n%s", err, out)
	}
	// The audit still reports the line in its own block — that block is the
	// standing question, and folding it away would hide it.
	if len(got.Unearned) != 1 {
		t.Errorf("verified_without_evidence = %d, want 1 — the audit block still names the line", len(got.Unearned))
	}
	if len(got.NeedsReview) != 1 {
		t.Fatalf("needs_review = %d, want 1 — the review tier must see one entry per line", len(got.NeedsReview))
	}
	if !got.NeedsReview[0].AlsoUnverified {
		t.Error("the surviving finding does not carry also_verified_without_evidence, so nothing downstream knows the line is two problems")
	}

	// The merge itself, directly: `reviewable` never reaches the JSON, so the
	// command-level assertion above cannot see a duplicate entry for the line.
	both := []repairFinding{
		{Rule: ruleCrossMilestoneTaskID, TaskID: "P1-M1-9", Line: 5},
		{Rule: ruleTaskOutsideMilestone, TaskID: "P1-M1-4", Line: 9},
	}
	audit := []repairFinding{
		{Rule: ruleVerifiedWithoutEvidence, TaskID: "P1-M1-9", Line: 5},
		{Rule: ruleVerifiedWithoutEvidence, TaskID: "P1-M1-2", Line: 12},
	}
	markAlsoUnverified(both, audit)
	merged := reviewableFindings(both, audit)
	if len(merged) != 3 {
		t.Errorf("reviewable = %d entries, want 3 — line 5 is two findings and must arrive once", len(merged))
	}
	seenLines := map[int]int{}
	for _, f := range merged {
		seenLines[f.Line]++
	}
	for line, n := range seenLines {
		if n > 1 {
			t.Errorf("line %d appears %d times in reviewable; the gate accepts one action per line", line, n)
		}
	}
	if !both[0].AlsoUnverified || both[1].AlsoUnverified {
		t.Errorf("AlsoUnverified = %v/%v, want true on the colliding line only", both[0].AlsoUnverified, both[1].AlsoUnverified)
	}

	// The wording follows the flag, not whichever finding the gate kept.
	plan := repairPlan{
		Finding: repairFinding{TaskID: "P1-M1-9", Rule: ruleCrossMilestoneTaskID, AlsoUnverified: true},
		Action:  repairAction{Action: repairLeave, Reason: "the migration is in the repo and its test passes"},
	}
	if desc := describeRepairPlan(plan); !strings.Contains(desc, "left verified") {
		t.Errorf("describeRepairPlan = %q, want 'left verified' — 'not a task' reads as the tool misunderstanding its own finding", desc)
	}
}

// Both invocation paths carry the cross-feature rule, or the fix only landed in
// Go. The mechanical tier refuses evidence a sibling feature could own; the
// review tier is the thing that rule DELEGATES to, and the interactive skill's
// no-CLI fallback is a hand-rolled version of the same query.
func TestBothPathsExplainAmbiguousEvidence(t *testing.T) {
	if !strings.Contains(repairAgentRules, "ambiguous") {
		t.Error("the CLI brief never explains evidence.ambiguous, so the tier the refusal delegates to cannot use it")
	}
	if !strings.Contains(repairAgentRules, "also_verified_without_evidence") {
		t.Error("the CLI brief never explains also_verified_without_evidence")
	}
	skill, err := os.ReadFile(filepath.Join("..", "..", "skills", "belmont", "_src", "repair.md"))
	if err != nil {
		t.Skipf("skill source not readable from the test tree: %v", err)
	}
	body := string(skill)
	if !strings.Contains(body, "ambiguous") {
		t.Error("skills/belmont/_src/repair.md never mentions ambiguous evidence — Mode B has no way to know")
	}
	if !strings.Contains(body, ".belmont/features/*/PROGRESS.md") {
		t.Error("Mode B's no-CLI fallback does not tell the agent to check whether a sibling feature claims the ID, so it reproduces the bug bb8eca7 fixed")
	}
}

// validateRepairPlans keys findings by line and keeps the FIRST. Nothing above
// it should now hand it two findings for one line — `reviewable` is built one
// entry per line — but the gate is the single place every action passes through
// and it must not depend on a caller getting that right. Pinned directly,
// because the callers no longer construct the collision.
func TestValidateKeepsTheFirstFindingForALine(t *testing.T) {
	doc := "### M1: Login\n- [ ] P1-M1-1: a\n\n### M2: Logout\n- [v] P1-M1-9: filed under M2, ID names M1\n"
	structural := repairFinding{
		Rule: ruleCrossMilestoneTaskID, TaskID: "P1-M1-9", Milestone: "M2",
		NamedMilestone: "M1", Marker: "v", Line: 5, Raw: "- [v] P1-M1-9: filed under M2, ID names M1",
	}
	audit := structural
	audit.Rule = ruleVerifiedWithoutEvidence
	audit.NamedMilestone = "" // the shape dcdda03 fixed; kept here as the hostile input

	plans, rejected := validateRepairPlans(doc, []repairFinding{structural, audit},
		[]repairAction{{TaskID: "P1-M1-9", Line: 5, Action: repairMoveMilestone, Reason: "its ID names M1"}})
	if len(rejected) != 0 {
		t.Fatalf("a legitimate move was refused: %+v", rejected)
	}
	if len(plans) != 1 || plans[0].Finding.Rule != ruleCrossMilestoneTaskID {
		t.Fatalf("plans = %+v, want the structural finding — it is the one carrying the problem being acted on", plans)
	}
	if plans[0].Action.Milestone != "M1" {
		t.Errorf("destination = %q, want M1 resolved from the finding the gate kept", plans[0].Action.Milestone)
	}
}

// `git log` walks newest-first, and `lookupCommitEvidence` returns on its first
// match — so the newest commit naming an ID is the one that speaks for it. When
// commitNamedTaskIDs started carrying the SHA and subject, it had to keep the
// same rule, or the audit would name a commit the per-ID primitive disagrees
// with. It matters for reverts most of all: the newest word on a task can be
// "this was undone".
func TestCommitIndexKeepsTheNewestCommitNamingAnID(t *testing.T) {
	root := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: the login route\n",
		"P1-M1-1: build the login route",
		`Revert "P1-M1-1: build the login route"`)

	idx, ok := commitNamedTaskIDs(root)
	if !ok {
		t.Fatal("commit log unreadable in a real repository")
	}
	ev, found := idx["P1-M1-1"]
	if !found {
		t.Fatal("P1-M1-1 missing from the commit index")
	}
	if !strings.HasPrefix(ev.Subject, "Revert") {
		t.Errorf("subject = %q, want the NEWEST commit — the index disagrees with lookupCommitEvidence", ev.Subject)
	}
	if !ev.Reverting {
		t.Error("Reverting not set, so the newest word on this task being a revert is invisible to the audit")
	}
	// And the two primitives agree, which is the whole point of sharing
	// commitTaskIDRe.
	per := lookupCommitEvidence(root, "P1-M1-1", "")
	if per.SHA != ev.SHA || per.Subject != ev.Subject || per.Reverting != ev.Reverting {
		t.Errorf("commitNamedTaskIDs = %+v but lookupCommitEvidence = %+v", ev, per)
	}
}

// The ambiguity check fails open, and repair says so rather than letting it be
// discovered.
//
// `taskIDsClaimedElsewhere` sees only the IDs in sibling PROGRESS.md files that
// still exist. `/belmont:cleanup`'s archive option replaces a completed
// feature's files with a slim ARCHIVE.md — so it stops claiming its IDs while
// its commits stay in the log for ever, and the cross-feature crediting bb8eca7
// refuses becomes invisible again.
//
// Both directions: the note must fire when the tier wrote something with an
// archived sibling present, and must NOT fire when every sibling is readable —
// or it is wallpaper on every run.
func TestRepairSaysWhenTheAmbiguityCheckWasPartial(t *testing.T) {
	root := gitFixture(t, "### M1: Invoices\n- [?] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the login form")
	// A sibling archived by /belmont:cleanup: no PROGRESS.md, commits intact.
	archived := filepath.Join(root, ".belmont", "features", "auth")
	if err := os.MkdirAll(archived, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archived, "ARCHIVE.md"),
		[]byte("# Auth — archived\n\nCompleted. Login form, session cookie.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, blind := taskIDsClaimedElsewhere(root, "demo"); len(blind) != 1 || blind[0] != "auth" {
		t.Fatalf("unreadable siblings = %v, want [auth] — the fixture must reach the fail-open branch", blind)
	}

	out := captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--mechanical-only"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	if !strings.Contains(out, "auth has no PROGRESS.md") {
		t.Errorf("repair wrote [x] on a commit it could not check for cross-feature collisions and said nothing:\n%s", out)
	}

	// Every sibling readable: silence.
	clean := gitFixture(t, "### M1: Invoices\n- [?] P1-M1-1: generate PDF invoices\n",
		"P1-M1-1: build the invoice table")
	writeFeature(t, clean, "auth", "### M1: Login\n- [x] P2-M1-1: build the login form\n")
	out = captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", clean, "--feature", "demo", "--mechanical-only"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	if strings.Contains(out, "has no PROGRESS.md") {
		t.Errorf("the partial-check note fired with every sibling readable:\n%s", out)
	}
}

// A shallow clone makes `git log` succeed on a fifth of the history, so the
// audit's "no commit names it" becomes an accusation against work committed
// before the cut. `taskHasCommit` already fails open for the same reason; the
// audit cannot fail open (its whole job is to raise the question), so it says
// what it could not see instead.
func TestVerifiedAuditSaysWhenTheHistoryIsTruncated(t *testing.T) {
	root := gitFixture(t, "### M1: Login\n- [v] P1-M1-1: nothing here names this\n", "unrelated work")
	if isShallowClone(root) {
		t.Fatal("fixture is already shallow, so the negative case below proves nothing")
	}
	full := captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", root, "--feature", "demo", "--dry-run"}); err != nil {
			t.Fatalf("repair: %v", err)
		}
	})
	if !strings.Contains(full, "marked [v] that the commit log does not settle") {
		t.Fatalf("fixture did not reach the audit block:\n%s", full)
	}
	if strings.Contains(full, "clone is shallow") {
		t.Errorf("the shallow note fired on a complete history:\n%s", full)
	}

	// Now a genuinely shallow clone of the same repository.
	dst := filepath.Join(t.TempDir(), "shallow")
	c := exec.Command("git", "clone", "-q", "--depth", "1", "file://"+root, dst)
	if out, err := c.CombinedOutput(); err != nil {
		t.Skipf("shallow clone unavailable in this environment: %v %s", err, out)
	}
	// Ask GIT whether the clone is shallow, not the function under test — with
	// `isShallowClone` mutated to `return false` the obvious form skips, and a
	// skip is not a pass. `.git/shallow` is git's own marker for a truncated
	// history.
	if _, err := os.Stat(filepath.Join(dst, ".git", "shallow")); err != nil {
		t.Skipf("clone --depth 1 did not produce a shallow repository here: %v", err)
	}
	if !isShallowClone(dst) {
		t.Fatal("isShallowClone says no, but git wrote .git/shallow — the audit will accuse work committed before the cut")
	}
	short := captureStderr(t, func() {
		if err := runRepairCmd([]string{"--root", dst, "--feature", "demo", "--dry-run"}); err != nil {
			t.Fatalf("repair (shallow): %v", err)
		}
	})
	if !strings.Contains(short, "clone is shallow") {
		t.Errorf("the audit accused a task of having no commit without saying it had only read part of the history:\n%s", short)
	}
}

// The mirror of the `[v]` cap. Repair may not write a verified marker, and it
// may not remove one in a single step either: nothing in Belmont can put a `[v]`
// back — `belmont reverify` only ever promotes `[x]` — so `[v]` → `[-]` is
// irreversible by tooling, and the evidence that reaches the gate for a verified
// task is explicitly a question rather than a verdict.
func TestRepairRefusesToWithdrawAVerifiedTaskInOneStep(t *testing.T) {
	doc := "### M1: Login\n- [v] P1-M1-9: filed under M1\n- [?] P1-M1-2: unreadable\n"
	verified := repairFinding{
		Rule: ruleVerifiedWithoutEvidence, TaskID: "P1-M1-9", Milestone: "M1",
		Marker: "v", Line: 2, Raw: "- [v] P1-M1-9: filed under M1",
	}
	unreadable := repairFinding{
		Rule: ruleUnrecognisedMarker, TaskID: "P1-M1-2", Milestone: "M1",
		Marker: "?", Line: 3, Raw: "- [?] P1-M1-2: unreadable",
	}
	findings := []repairFinding{verified, unreadable}

	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{TaskID: "P1-M1-9", Line: 2, Action: repairWithdraw, Reason: "superseded by the new auth flow"},
	})
	if len(plans) != 0 {
		t.Errorf("a [v] task was withdrawn to [-] in one step, and nothing can write [v] back: %+v", plans)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, `set_marker "x"`) {
		t.Errorf("rejection = %+v, want one naming the demote-first route", rejected)
	}

	// The refusal is about the marker, not about withdrawal: an ordinary
	// finding still withdraws.
	plans, rejected = validateRepairPlans(doc, findings, []repairAction{
		{TaskID: "P1-M1-2", Line: 3, Action: repairWithdraw, Reason: "the endpoint it names was removed in M3"},
	})
	if len(plans) != 1 || len(rejected) != 0 {
		t.Errorf("withdrawal of a non-verified finding was refused: plans=%+v rejected=%+v", plans, rejected)
	}
}

// A moved line must land after the destination's last task AND its indented
// body — not between the task and its own continuation lines, which would
// re-attach the body to the moved task.
func TestRepairMoveLandsAfterAnchorTaskBody(t *testing.T) {
	doc := "### M2: Surface\n- [ ] P1-M2-1: stays\n- [ ] P1-M3-9: beta\n\n" +
		"### M3: Later\n- [ ] P1-M3-1: stays\n  - Done when: baz holds\n\n## Session History\n"
	findings := collectRepairFindings(doc)
	if len(findings) != 1 {
		t.Fatalf("fixture: %d findings, want 1: %+v", len(findings), findings)
	}
	plans, rejected := validateRepairPlans(doc, findings, []repairAction{
		{Line: 3, TaskID: "P1-M3-9", Action: repairMoveMilestone, Reason: "belongs to M3"},
	})
	if len(rejected) != 0 || len(plans) != 1 {
		t.Fatalf("plans=%d rejected=%+v", len(plans), rejected)
	}
	got, applied, warnings := applyRepairPlans(doc, plans, "2026-08-09")
	if len(applied) != 1 || len(warnings) != 0 {
		t.Fatalf("applied=%d warnings=%v", len(applied), warnings)
	}
	body := strings.Index(got, "Done when: baz holds")
	moved := strings.Index(got, "P1-M3-9")
	if moved < body {
		t.Errorf("the moved line was spliced between P1-M3-1 and its body:\n%s", got)
	}
	byID := map[string][]string{}
	for _, m := range parseMilestones(got) {
		for _, task := range m.Tasks {
			byID[m.ID] = append(byID[m.ID], task.ID)
		}
	}
	want := []string{"P1-M3-1", "P1-M3-9"}
	if strings.Join(byID["M3"], ",") != strings.Join(want, ",") {
		t.Errorf("M3 tasks = %v, want %v:\n%s", byID["M3"], want, got)
	}
}
