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
  Needs an operator — the credentials live in a vault no agent can read.
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
	if !strings.Contains(first.Detail[0], "vault") {
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
	// Without this the test passes on an empty report, which is exactly the
	// bug it is meant to catch: a filter that drops everything leaks nothing.
	if report.Count != 2 {
		t.Fatalf("count = %d, want 2 — the fixture's two [!] tasks", report.Count)
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
		"vault",
		"PROGRESS.md:8",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full render missing %q:\n%s", want, full)
		}
	}

	sum := renderBlockers(report, false, true)
	if strings.Contains(sum, "vault") {
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

func TestTaskDetailSpansABlankLineAndStopsAtTheOwnIndent(t *testing.T) {
	// A blank does not end a body — a loose-list task keeps its **Evidence**
	// behind one, and a blocker's whole question is the point of this command.
	lines := []string{
		"- [!] P0-1: Blocked",
		"  body one",
		"  body two",
		"",
		"  still mine — a blank does not end the body",
	}
	got := taskDetail(lines, 1)
	if len(got) != 3 || got[2] != "still mine — a blank does not end the body" {
		t.Fatalf("taskDetail = %#v, want all three body lines", got)
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

// --- Live-run state -------------------------------------------------------
//
// These cover the branch that carried two shipped bugs and had zero tests: the
// queue read during an active `belmont auto` run.

func writeAutoJSON(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".belmont"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".belmont", "auto.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// In single-feature-parallel mode each MILESTONE has its own worktree.
// `loadAutoWorktrees` collapses that map to one representative directory, so
// reading through it hid every blocker except the representative's — while
// `belmont status --feature` showed them and pointed the reader here.
func TestBuildBlockersReadsEveryMilestoneWorktree(t *testing.T) {
	root := t.TempDir()
	master := "### M1: One\n\n- [ ] P0-1: A\n\n### M2: Two\n\n- [ ] P0-2: B\n"
	writeBlockerFeature(t, root, "auth", "# PRD: Auth\n", master)

	wt1 := filepath.Join(root, "..", "wt-m1")
	wt2 := filepath.Join(root, "..", "wt-m2")
	for _, w := range []struct{ path, progress string }{
		{wt1, "### M1: One\n\n- [ ] P0-1: A\n\n### M2: Two\n\n- [ ] P0-2: B\n"},
		// M2's worktree raised the blocker, with a body, at a different line
		// number from master's copy.
		{wt2, "# notes\n# notes\n\n### M1: One\n\n- [ ] P0-1: A\n\n### M2: Two\n\n- [!] P0-2: B\n  Needs an operator to approve the production apply.\n"},
	} {
		dir := filepath.Join(w.path, ".belmont", "features", "auth")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(w.progress), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeAutoJSON(t, root, `{"active":true,"mode":"single-feature-parallel","feature":"auth",
		"worktrees":{"M1":{"path":"`+wt1+`"},"M2":{"path":"`+wt2+`"}}}`)

	report, err := buildBlockers(root, "auth")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 1 {
		t.Fatalf("expected M2's worktree blocker, got %d: %+v", report.Count, report.Blockers)
	}
	b := report.Blockers[0]
	if b.TaskID != "P0-2" || b.Milestone != "M2" {
		t.Errorf("wrong task: %+v", b)
	}
	if b.Line != 10 {
		t.Errorf("line = %d, want 10 (the worktree's line, not master's 7)", b.Line)
	}
	if b.LiveFrom == "" {
		t.Error("LiveFrom should name the worktree the line lives in")
	}
	if !strings.Contains(strings.Join(b.Detail, " "), "approve the production apply") {
		t.Errorf("detail should come from the worktree copy: %#v", b.Detail)
	}
	if !strings.Contains(b.Path, "wt-m2") {
		t.Errorf("path = %q, should point at the worktree file", b.Path)
	}
}

// A half-cleaned worktree must not take master's blockers down with it.
func TestBuildBlockersFallsBackToMasterWhenOverrideUnreadable(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "auth", "# PRD: Auth\n", "### M1: One\n\n- [!] P0-1: Rotate the credentials\n")

	empty := filepath.Join(root, "..", "wt-empty")
	if err := os.MkdirAll(filepath.Join(empty, ".belmont", "features", "auth"), 0755); err != nil {
		t.Fatal(err)
	}
	writeAutoJSON(t, root, `{"active":true,"mode":"multi-feature","worktrees":{"auth":{"path":"`+empty+`"}}}`)

	report, err := buildBlockers(root, "auth")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 1 || report.Blockers[0].TaskID != "P0-1" {
		t.Fatalf("master's blocker should survive an unreadable override: %+v", report)
	}
	if len(report.Skipped) != 0 {
		t.Errorf("feature was recoverable, should not be reported skipped: %v", report.Skipped)
	}
}

func TestBuildBlockersReportsUnreadableFeature(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "broken")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	} // no PROGRESS.md at all

	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "broken" {
		t.Fatalf("skipped = %v, want [broken]", report.Skipped)
	}
	out := renderBlockers(report, false, false)
	if !strings.Contains(out, "Could not read PROGRESS.md for: broken") {
		t.Errorf("an unreadable feature must be stated, not silent:\n%s", out)
	}
}

// --- Orphans --------------------------------------------------------------

// A `[!]` outside every milestone is invisible to parseMilestones. Dropping it
// made this command print an affirmative all-clear about a file it could not
// fully read — the one thing a decision queue must never do.
func TestBuildBlockersSurfacesOrphanedBlockers(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "orphan", "# PRD: Orphan\n",
		"### M1: One\n\n- [v] P0-1: Done\n\n## Session History\n\n- [!] P0-9: Rotate the production credentials\n  Needs an operator with console access.\n")

	report, err := buildBlockers(root, "orphan")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if len(report.Unplaced) != 1 {
		t.Fatalf("unplaced = %+v, want the orphaned blocker", report.Unplaced)
	}
	if report.Count != 1 {
		t.Errorf("count = %d, orphans must be counted", report.Count)
	}
	if report.Unplaced[0].TaskID != "P0-9" {
		t.Errorf("wrong orphan: %+v", report.Unplaced[0])
	}
	if len(report.Unplaced[0].Detail) != 1 {
		t.Errorf("orphan should carry its body: %#v", report.Unplaced[0].Detail)
	}

	out := renderBlockers(report, false, false)
	if strings.Contains(out, "Nothing is waiting on you") {
		t.Errorf("must not print an all-clear when an orphaned blocker exists:\n%s", out)
	}
	if !strings.Contains(out, "Outside any milestone") {
		t.Errorf("orphans need their own block:\n%s", out)
	}
}

// --- Rendering ------------------------------------------------------------

// --summary is documented as one line per blocker, and the loop skill calls it
// so a 19-item queue is a scannable handover. It must not emit two.
func TestRenderBlockersSummaryIsOneLinePerBlocker(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", blockerProgress)
	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatal(err)
	}
	sum := renderBlockers(report, false, true)
	n := 0
	for _, line := range strings.Split(sum, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[!] ") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected 2 blocker lines, got %d:\n%s", n, sum)
	}
	for _, line := range strings.Split(sum, "\n") {
		if strings.Contains(line, "PROGRESS.md:") && !strings.Contains(line, "[!]") {
			t.Errorf("summary emitted a second line for a blocker: %q", line)
		}
	}
	if !strings.Contains(sum, "PROGRESS.md:8)") {
		t.Errorf("summary should carry the location inline:\n%s", sum)
	}
}

func TestRenderBlockersPrintsTheRealPath(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", blockerProgress)
	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".belmont", "features", "alpha", "PROGRESS.md") + ":8"
	out := renderBlockers(report, false, false)
	if !strings.Contains(out, want) {
		t.Errorf("render should name the file the line is in (%s):\n%s", want, out)
	}
}

func TestRenderBlockersSeparatesFeatures(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "zulu", "# PRD: Zulu\n", "### M1: One\n\n- [!] P0-1: Zulu blocker\n")
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n", "### M1: One\n\n- [!] P0-1: Alpha blocker\n")
	report, err := buildBlockers(root, "")
	if err != nil {
		t.Fatal(err)
	}
	out := renderBlockers(report, false, false)
	if !strings.Contains(out, "Alpha (alpha)") || !strings.Contains(out, "Zulu (zulu)") {
		t.Errorf("both feature headers should render:\n%s", out)
	}
	if strings.Index(out, "Alpha (alpha)") > strings.Index(out, "Zulu (zulu)") {
		t.Errorf("features should render in slug order:\n%s", out)
	}
}

// --- taskDetail edge cases ------------------------------------------------

// Under a nested list, taskBodyEnd alone does not stop at a sibling task —
// every following task would be swallowed into the first one's body.
func TestTaskDetailStopsAtASiblingTaskViaOwnIndent(t *testing.T) {
	lines := []string{
		"  - [!] P0-1: Blocked",
		"    needs a person",
		"  - [ ] P0-2: Next task",
		"    its own body",
	}
	got := taskDetail(lines, 1)
	if len(got) != 1 || got[0] != "needs a person" {
		t.Fatalf("taskDetail = %#v, want just the blocker's own body", got)
	}
}

// The detail view's pointer line is the route to this command; nothing asserted
// it existed.
func TestStatusDetailViewNamesTheBlockersCommand(t *testing.T) {
	report := statusReport{
		FeatureSlug: "alpha",
		TaskCounts:  map[string]int{},
		Milestones: []milestone{{ID: "M1", Name: "One", Tasks: []task{
			{ID: "P0-1", Name: "Rotate the credentials", Status: taskBlocked},
		}}},
	}
	out := renderStatus(report, false, false)
	if !strings.Contains(out, "belmont blockers --feature alpha") {
		t.Errorf("detail view should name the command with the slug:\n%s", out)
	}
}

// --- Review round: unknown markers, archived features, per-path footer ----

// An unreadable marker is loud everywhere else in Belmont — status counts it,
// validate exits 1, repair flags it. This reader must not be the one place a
// corrupted [!] goes quiet behind an affirmative all-clear.
func TestBuildBlockersSurfacesUnknownMarkers(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "alpha", "# PRD: Alpha\n",
		"### M1: One\n\n- [v] P0-1: Done\n- [?] P0-2: Was a blocker before the marker got mangled\n  Needs a ruling.\n")

	report, err := buildBlockers(root, "alpha")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if len(report.Unknown) != 1 {
		t.Fatalf("unknown = %+v, want the mangled task", report.Unknown)
	}
	if report.Unknown[0].Marker != "?" {
		t.Errorf("marker = %q, want the raw character quoted back", report.Unknown[0].Marker)
	}
	if report.Count != 1 {
		t.Errorf("count = %d, unknown markers must be counted", report.Count)
	}
	out := renderBlockers(report, false, false)
	if strings.Contains(out, "Nothing is waiting on you") {
		t.Errorf("must not print an all-clear over an unreadable marker:\n%s", out)
	}
	if !strings.Contains(out, "Unreadable markers") || !strings.Contains(out, "belmont validate") {
		t.Errorf("unknown markers need their own block naming validate:\n%s", out)
	}
}

// An archived feature's stale [!] is not a live question. Dropping it silently
// is the same failure as dropping an orphan, so it is labelled instead.
func TestBuildBlockersLabelsArchivedFeatures(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "old")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// /belmont:cleanup removes PRD.md and leaves ARCHIVE.md behind.
	if err := os.WriteFile(filepath.Join(dir, "ARCHIVE.md"), []byte("# Archive: Legacy Feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"),
		[]byte("### M1: One\n\n- [!] P0-1: Never answered\n"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := buildBlockers(root, "old")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	if report.Count != 1 {
		t.Fatalf("archived blockers must still be reported, got %d", report.Count)
	}
	if !report.Blockers[0].Archived {
		t.Error("entry should be flagged Archived")
	}
	if report.Blockers[0].FeatureName != "Legacy Feature" {
		t.Errorf("name = %q, want the ARCHIVE.md header (PRD.md is gone)", report.Blockers[0].FeatureName)
	}
	if out := renderBlockers(report, false, false); !strings.Contains(out, "archived") {
		t.Errorf("render should mark the feature archived:\n%s", out)
	}
}

// The fallback must move baseDir with the content, or the PRD.md read still
// points at the dead worktree and the feature renders as its bare slug.
func TestBuildBlockersFallbackAlsoRecoversTheName(t *testing.T) {
	root := t.TempDir()
	writeBlockerFeature(t, root, "auth", "# PRD: Auth Service\n", "### M1: One\n\n- [!] P0-1: Blocked\n")
	empty := filepath.Join(root, "..", "wt-dead")
	if err := os.MkdirAll(filepath.Join(empty, ".belmont", "features", "auth"), 0755); err != nil {
		t.Fatal(err)
	}
	writeAutoJSON(t, root, `{"active":true,"mode":"multi-feature","worktrees":{"auth":{"path":"`+empty+`"}}}`)

	report, err := buildBlockers(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 1 {
		t.Fatalf("count = %d, want 1", report.Count)
	}
	if report.Blockers[0].FeatureName != "Auth Service" {
		t.Errorf("name = %q, want Auth Service — the fallback must read PRD.md from master too",
			report.Blockers[0].FeatureName)
	}
}

// In a parallel run one blocker can be in a worktree and another in master.
// The advice inverts between them, so one blanket sentence sends half the
// readers to a file that does not exist.
func TestRenderBlockersFooterScopesWorktreeAdvice(t *testing.T) {
	mixed := blockersReport{
		Count: 2, Features: 1,
		Blockers: []blockerEntry{
			{Feature: "a", FeatureName: "A", Milestone: "M1", TaskID: "P0-1", Task: "in worktree",
				Path: "wt/PROGRESS.md", Line: 3, LiveFrom: "/wt/a"},
			{Feature: "a", FeatureName: "A", Milestone: "M3", TaskID: "P0-2", Task: "in master",
				Path: ".belmont/features/a/PROGRESS.md", Line: 9},
		},
	}
	out := renderBlockers(mixed, false, false)
	if !strings.Contains(out, "belmont steer") {
		t.Errorf("worktree entries need the steer advice:\n%s", out)
	}
	if !strings.Contains(out, "have no worktree yet") {
		t.Errorf("master entries need their own advice — editing them directly is correct:\n%s", out)
	}

	allMaster := blockersReport{Count: 1, Features: 1, Blockers: []blockerEntry{mixed.Blockers[1]}}
	if out := renderBlockers(allMaster, false, false); strings.Contains(out, "auto run is active") {
		t.Errorf("no worktree entries — must not claim a run is active:\n%s", out)
	}
}
