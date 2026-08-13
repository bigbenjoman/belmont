package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBlockerFeature(t *testing.T, root, slug, prd, progress string) {
	t.Helper()
	dir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if prd != "" {
		if err := os.WriteFile(filepath.Join(dir, "PRD.md"), []byte(prd), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(progress), 0644); err != nil {
		t.Fatal(err)
	}
}

const blockerProgress = `# Progress

## Milestones

### M1: Foundation

- [v] P0-1: Done thing
- [!] P0-2: Rotate the database passwords
  Needs Ben — the credentials live in 1Password and no agent has access.
  Second detail line.
- [ ] P0-3: Not started

### M2: Second (depends: M1)

- [!] P0-4: Approve the full-backlog apply
- [-] P0-5: Dropped thing
`

func TestBuildBlockersCollectsOnlyBlockedTasks(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha Feature\n", blockerProgress)

	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 2 {
		t.Fatalf("expected 2 blockers, got %d: %+v", report.Count, report.Blockers)
	}
	if report.Features != 1 {
		t.Fatalf("expected 1 feature, got %d", report.Features)
	}

	first := report.Blockers[0]
	if first.TaskID != "P0-2" {
		t.Errorf("task id = %q, want P0-2", first.TaskID)
	}
	if first.Milestone != "M1" || first.MilestoneName != "Foundation" {
		t.Errorf("milestone = %q/%q, want M1/Foundation", first.Milestone, first.MilestoneName)
	}
	if first.FeatureName != "Alpha Feature" {
		t.Errorf("feature name = %q, want Alpha Feature", first.FeatureName)
	}
	if len(first.Detail) != 2 {
		t.Fatalf("detail = %#v, want 2 lines", first.Detail)
	}
	if !strings.Contains(first.Detail[0], "1Password") {
		t.Errorf("detail[0] = %q, want the indented body", first.Detail[0])
	}

	// M2's blocker carries no indented body — it must still be reported, with
	// no detail rather than borrowing the next task's lines.
	second := report.Blockers[1]
	if second.TaskID != "P0-4" {
		t.Errorf("second task id = %q, want P0-4", second.TaskID)
	}
	if len(second.Detail) != 0 {
		t.Errorf("second detail = %#v, want none", second.Detail)
	}
}

// A `[-]` withdrawn task is deliberately dropped work, not a question for a
// human. It must never appear in the decision queue — the marker set added it
// precisely so dropped work stops being offered as outstanding.
func TestBuildBlockersIgnoresWithdrawnAndOtherMarkers(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", blockerProgress)

	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	for _, b := range report.Blockers {
		if strings.Contains(b.Task, "Dropped") || strings.Contains(b.Task, "Not started") ||
			strings.Contains(b.Task, "Done thing") {
			t.Errorf("non-blocked task leaked into the queue: %+v", b)
		}
	}
}

func TestBuildBlockersSpansFeaturesInSlugOrder(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "zulu", "# PRD: Zulu\n", "### M1: One\n\n- [!] P0-1: Zulu blocker\n")
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", "### M1: One\n\n- [!] P0-1: Alpha blocker\n")

	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 2 || report.Features != 2 {
		t.Fatalf("count=%d features=%d, want 2/2", report.Count, report.Features)
	}
	if report.Blockers[0].Feature != "alpha" || report.Blockers[1].Feature != "zulu" {
		t.Errorf("features out of order: %q then %q", report.Blockers[0].Feature, report.Blockers[1].Feature)
	}
}

func TestBuildBlockersFeatureFilter(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "zulu", "# PRD: Zulu\n", "### M1: One\n\n- [!] P0-1: Zulu blocker\n")
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", "### M1: One\n\n- [!] P0-1: Alpha blocker\n")

	report, err := buildBlockers(root, "zulu")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 1 || report.Blockers[0].Feature != "zulu" {
		t.Fatalf("filter failed: %+v", report.Blockers)
	}

	if _, err := buildBlockers(root, "nope"); err == nil {
		t.Error("expected an error for an unknown feature slug")
	}
}

func TestBuildBlockersEmptyProjectIsNotAnError(t *testing.T) {
	root := t.TempDir()
	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatalf("buildBlockers on an empty project: %v", err)
	}
	if report.Count != 0 {
		t.Errorf("count = %d, want 0", report.Count)
	}
}

func TestRenderBlockersEmptyAndFull(t *testing.T) {
	empty := renderBlockers(blockersReport{}, false, false)
	if !strings.Contains(empty, "Nothing is waiting on you") {
		t.Errorf("empty render = %q", empty)
	}

	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha Feature\n", blockerProgress)
	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatal(err)
	}

	full := renderBlockers(report, false, false)
	for _, want := range []string{
		"2 blocked tasks across 1 feature",
		"Alpha Feature (alpha)",
		"M1: Foundation",
		"P0-2: Rotate the database passwords",
		"1Password",
		"PROGRESS.md:8",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full render missing %q:\n%s", want, full)
		}
	}

	sum := renderBlockers(report, false, true)
	if strings.Contains(sum, "1Password") {
		t.Errorf("summary render should omit the body:\n%s", sum)
	}
	if !strings.Contains(sum, "P0-2: Rotate the database passwords") {
		t.Errorf("summary render should keep the task line:\n%s", sum)
	}
}

// A task whose whole explanation sits on the checkbox line must not blow up
// summary mode, and must not be truncated in full mode.
func TestRenderBlockersSummaryTruncatesLongNamesOnly(t *testing.T) {
	long := strings.Repeat("word ", 80)
	report := blockersReport{
		Count: 1, Features: 1,
		Blockers: []blockerEntry{{
			Feature: "alpha", FeatureName: "Alpha", Milestone: "M1", MilestoneName: "One",
			TaskID: "P0-1", Task: long, Line: 3,
		}},
	}
	sum := renderBlockers(report, false, true)
	if !strings.Contains(sum, "…") {
		t.Errorf("summary should truncate a very long task line:\n%s", sum)
	}
	for _, line := range strings.Split(sum, "\n") {
		if len([]rune(line)) > 200 {
			t.Errorf("summary line too long (%d runes): %q", len([]rune(line)), line)
		}
	}
	full := renderBlockers(report, false, false)
	if strings.Contains(full, "…") {
		t.Errorf("full mode must not truncate:\n%s", full)
	}
}

func TestTaskDetailStopsAtBlankAndColumnZero(t *testing.T) {
	lines := []string{
		"- [!] P0-1: Blocked",
		"  body one",
		"  body two",
		"",
		"  not mine — after a blank line",
	}
	got := taskDetail(lines, 1)
	if len(got) != 2 || got[0] != "body one" || got[1] != "body two" {
		t.Fatalf("taskDetail = %#v", got)
	}

	lines = []string{
		"- [!] P0-1: Blocked",
		"- [ ] P0-2: Next task",
	}
	if got := taskDetail(lines, 1); got != nil {
		t.Fatalf("taskDetail across a sibling task = %#v, want nil", got)
	}

	if got := taskDetail(lines, 99); got != nil {
		t.Fatalf("taskDetail out of range = %#v, want nil", got)
	}
}

// The listing view caps how many blockers it prints per feature, but must say
// how many it withheld and name the command that shows them.
func TestFeatureListingCapsBlockersAndPointsAtTheCommand(t *testing.T) {
	var ms []milestone
	var tasks []task
	for i := 0; i < blockedListingCap+4; i++ {
		tasks = append(tasks, task{ID: "P0-" + string(rune('1'+i)), Name: "blocker", Status: taskBlocked})
	}
	ms = append(ms, milestone{ID: "M1", Name: "One", Tasks: tasks})

	report := statusReport{
		TaskCounts: map[string]int{},
		Features: []featureSummary{{
			Slug: "alpha", Name: "Alpha", Status: "In Progress",
			TasksBlocked: len(tasks), TasksTotal: len(tasks), Milestones: ms,
		}},
	}
	out := renderFeatureListing(report, false, false)
	if strings.Count(out, "    - ") != blockedListingCap {
		t.Errorf("expected %d blocker lines, got %d:\n%s", blockedListingCap, strings.Count(out, "    - "), out)
	}
	if !strings.Contains(out, "…and 4 more — belmont blockers --feature alpha") {
		t.Errorf("listing should name the overflow and the command:\n%s", out)
	}
}
