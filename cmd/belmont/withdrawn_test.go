package main

import (
	"os/exec"
	"strings"
	"testing"
)

// `[-]` withdrawn exists because its absence caused issue #27. Belmont's marker
// set had no way to say "we decided not to do this", so a user invented `[-]`,
// every unrecognised marker parsed as todo, and cancelled work was offered to an
// agent as the next thing to build. Repair tooling does not fix that on its own
// — without a legal way to express withdrawal the next person invents a marker
// too.

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

	// The control: deletion is NOT stable, which is why we do not use it.
	if got, _ := mergeProgressState(deleted, full); !strings.Contains(got, "P1-M3-2") {
		t.Log("deletion happened to survive here; the marker approach does not depend on it")
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
