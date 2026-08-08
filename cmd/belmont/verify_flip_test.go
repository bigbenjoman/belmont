package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #30: the [x] -> [v] flip after verification is written by the agent as
// prose, and every stop condition in the product treats [x] as finished --
// computeOverallStatus returns "Complete" for an all-[x] feature, so
// loop.md's "no pending milestones", decideLoopActionSmart's actionComplete
// rules and computeWaves all finish on it. Nothing repairs it: nextTask
// positively matches only todo/in-progress and the guards are subtractive.
//
// These pin the one mechanical defence: `belmont status` must say so, and must
// name `belmont reverify`, which appears in no skill or partial.

func buildFeature(t *testing.T, progress string) (statusReport, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# PRD: Demo\n"), 0644)
	os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(progress), 0644)
	report, err := buildStatus(root, 0, "demo")
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}
	return report, root
}

const allDoneProgress = `# Progress: Demo

## Milestones

### M1: Verified properly
- [v] P1-M1-1: verified

### M2: Flip never written
- [x] P1-M2-1: built it
- [x] P1-M2-2: built the other bit
`

func TestStatusWarnsOnDoneNotVerified(t *testing.T) {
	report, _ := buildFeature(t, allDoneProgress)

	// The precondition that makes this invisible: it reads as success.
	if report.OverallStatus != "Complete" {
		t.Fatalf("OverallStatus = %q, want %q — fixture no longer reproduces the state", report.OverallStatus, "Complete")
	}
	if nextMilestone(report.Milestones) != nil {
		t.Error("fixture should offer no next milestone — that is why the loop stops")
	}

	out := renderStatus(report, false, false)
	if !strings.Contains(out, "never verified") {
		t.Errorf("status is silent about 2 done-but-unverified tasks:\n%s", out)
	}
	if !strings.Contains(out, "belmont reverify --feature demo") {
		t.Errorf("status must name the recovery command with the SLUG (the only recovery, and it appears in no skill):\n%s", out)
	}
	for _, id := range []string{"P1-M2-1", "P1-M2-2"} {
		if !strings.Contains(out, id) {
			t.Errorf("status should name the unverified task %s:\n%s", id, out)
		}
	}
	if strings.Contains(out, "P1-M1-1") && strings.Contains(out, "never verified\n  [x] P1-M1-1") {
		t.Error("an already-verified task was listed as unverified")
	}
}

// The control: a genuinely verified feature must stay silent, or the warning
// fires on every project and gets ignored.
func TestStatusSilentWhenAllVerified(t *testing.T) {
	report, _ := buildFeature(t, strings.ReplaceAll(allDoneProgress, "- [x]", "- [v]"))
	// All-[v] reads "Verified", which is precisely the distinction the warning
	// exists to surface: "Complete" is the weaker state that looks identical
	// at every stop condition.
	if report.OverallStatus != "Verified" {
		t.Fatalf("OverallStatus = %q, want %q", report.OverallStatus, "Verified")
	}
	if out := renderStatus(report, false, false); strings.Contains(out, "never verified") {
		t.Errorf("false positive on a fully verified feature:\n%s", out)
	}
}

// Mid-run is the self-correcting case: a later milestone is still pending and
// verify rescans for [x] next iteration. Warning there would be pure noise.
func TestStatusSilentMidRun(t *testing.T) {
	report, _ := buildFeature(t, allDoneProgress+"\n### M3: Not started\n- [ ] P1-M3-1: todo\n")
	if report.OverallStatus == "Complete" {
		t.Fatal("fixture should not read Complete")
	}
	if out := renderStatus(report, false, false); strings.Contains(out, "never verified") {
		t.Errorf("warned mid-run, where the drop self-corrects:\n%s", out)
	}
}

// isFeatureTerminal switches on the literal "Complete" and auto --all wave
// planning depends on it. The warning must be rendered text only.
func TestDoneNotVerifiedDoesNotChangeOverallStatus(t *testing.T) {
	report, _ := buildFeature(t, allDoneProgress)
	if report.OverallStatus != "Complete" {
		t.Errorf("OverallStatus = %q, want %q — changing it silently breaks isFeatureTerminal and wave planning",
			report.OverallStatus, "Complete")
	}
	if !isFeatureTerminal(report.OverallStatus) {
		t.Error("isFeatureTerminal no longer accepts the status; auto --all scheduling would change")
	}
}

func TestDoneNotVerifiedTasksHelper(t *testing.T) {
	ms := parseMilestones(allDoneProgress)
	got := doneNotVerifiedTasks(ms)
	if len(got) != 2 {
		t.Fatalf("doneNotVerifiedTasks returned %d, want 2", len(got))
	}
	if got[0].ID != "P1-M2-1" || got[1].ID != "P1-M2-2" {
		t.Errorf("expected document order, got %s then %s", got[0].ID, got[1].ID)
	}
	// An unknown-marker task is not "done" and must not be swept in here.
	withUnknown := parseMilestones(allDoneProgress + "\n### M3: M\n- [?] P1-M3-1: withdrawn\n")
	if len(doneNotVerifiedTasks(withUnknown)) != 2 {
		t.Error("an unrecognised marker was counted as done-but-unverified")
	}
}

// The listing view is what status.md's fast path runs (no --feature), so it is
// the half that reaches an interactive agent deciding whether to stop.
func TestListingWarnsOnDoneNotVerified(t *testing.T) {
	report := statusReport{
		Feature:       "Demo Product",
		OverallStatus: "Complete",
		TaskCounts:    map[string]int{},
		Features: []featureSummary{{
			Name: "Demo", Slug: "demo", Status: "Complete",
			TasksDone: 3, TasksTotal: 3, TasksVerified: 1,
			MilestonesDone: 2, MilestonesTotal: 2,
		}},
	}
	out := renderStatus(report, false, false)
	if !strings.Contains(out, "never verified") || !strings.Contains(out, "belmont reverify --feature demo") {
		t.Errorf("listing mode is silent about an unverified feature:\n%s", out)
	}

	report.Features[0].TasksVerified = 3
	if out := renderStatus(report, false, false); strings.Contains(out, "never verified") {
		t.Errorf("listing mode false-positives on a fully verified feature:\n%s", out)
	}
}

// Listing mode is what status.md's fast path runs (no --feature), so every
// "this file is not what it looks like" signal must appear there too. The
// unknown-marker and orphan warnings originally shipped detail-view only.
func TestListingSurfacesAllDiagnostics(t *testing.T) {
	ms := parseMilestones("# P\n\n## Milestones\n\n### M1: M\n- [x] P1-M1-1: a\n- [?] P1-M1-2: b\n")
	report := statusReport{
		Feature: "Demo Product", OverallStatus: "In Progress", TaskCounts: map[string]int{},
		Features: []featureSummary{{
			Name: "Demo", Slug: "demo", Status: "In Progress",
			TasksDone: 1, TasksTotal: 2, Milestones: ms, TasksOrphaned: 3,
		}},
	}
	out := renderStatus(report, false, false)
	if !strings.Contains(out, "unrecognised task marker") {
		t.Errorf("listing mode hides unrecognised markers (#27):\n%s", out)
	}
	if !strings.Contains(out, "outside any milestone") {
		t.Errorf("listing mode hides orphaned task lines (#31):\n%s", out)
	}
	if !strings.Contains(out, "--feature demo") {
		t.Errorf("listing warnings must point at the detail view:\n%s", out)
	}

	// Control: a clean feature stays silent.
	clean := parseMilestones("# P\n\n## Milestones\n\n### M1: M\n- [x] P1-M1-1: a\n")
	report.Features[0].Milestones = clean
	report.Features[0].TasksOrphaned = 0
	if out := renderStatus(report, false, false); strings.Contains(out, "⚠") {
		t.Errorf("false positive on a clean feature:\n%s", out)
	}
}
