package main

import (
	"strings"
	"testing"
)

// Issue #31: a `## ` heading anywhere in the milestones region silently ended
// task collection for the rest of the file — including one indented inside a
// task's own continuation body, because every reader trimmed the line before
// testing the prefix. On the reporting project that hid 85 of 541 tasks,
// including outstanding [ ] and [!] work, with `belmont validate` exiting 0.

// The issue's exact reproduction.
const quotedHeadingProgress = `# Progress: Demo

## Milestones

### M1: Everything here is real work
- [x] T-1: done, and visible
- [x] T-2: done, and its write-up quotes a heading

  ## Ruling 2026-08-02 — a heading quoted inside the task body

  The two spaces above make this part of T-2's body, not a new section.

- [ ] T-3: TODO — never rendered, never counted, never offered
- [!] T-4: BLOCKED — also invisible
`

func TestIndentedHeadingDoesNotEndMilestone(t *testing.T) {
	ms := parseMilestones(quotedHeadingProgress)
	if len(ms) != 1 {
		t.Fatalf("milestones = %d, want 1", len(ms))
	}
	if got := len(ms[0].Tasks); got != 4 {
		t.Fatalf("tasks = %d, want 4 — a heading indented inside a task body ended collection:\n%+v", got, ms[0].Tasks)
	}
	tasks := flattenTasks(ms, 0)
	if s := computeOverallStatus(tasks); s == "Complete" {
		t.Error(`status reads "Complete" while a [ ] and a [!] sit in the file`)
	}
	if !milestoneHasBlockers(ms[0]) {
		t.Error("the [!] blocker is invisible")
	}
	if nextTask(tasks) == nil {
		t.Error("the [ ] task is never offered as next work")
	}
}

// isSectionBreak is the single definition. Five readers used to inline a
// trimmed check and all five were wrong the same way.
func TestIsSectionBreak(t *testing.T) {
	breaks := []string{"## Milestones", "## Session History", "##", "##\tTabbed"}
	for _, s := range breaks {
		if !isSectionBreak(s) {
			t.Errorf("isSectionBreak(%q) = false, want true", s)
		}
	}
	notBreaks := []string{
		"  ## Indented under a list item", // the #31 case
		"\t## Tab-indented",
		"### M1: Milestone header",
		"#### Deeper heading",
		"###",
		"#Heading",
		"# Title",
		"##Nospace",
		"- [x] T-1: a task",
		"",
		"Some prose mentioning ## in the middle",
	}
	for _, s := range notBreaks {
		if isSectionBreak(s) {
			t.Errorf("isSectionBreak(%q) = true, want false", s)
		}
	}
}

// A column-zero `## ` legitimately ends the region — that part must not change,
// or `## Session History` would be swallowed into the last milestone.
func TestColumnZeroHeadingStillEndsMilestone(t *testing.T) {
	body := `# Progress: Demo

## Milestones

### M1: Work
- [x] P1-M1-1: a

## Session History

| Session | Date |
|---------|------|
`
	ms := parseMilestones(body)
	if len(ms) != 1 || len(ms[0].Tasks) != 1 {
		t.Fatalf("want 1 milestone with 1 task, got %d milestones", len(ms))
	}
}

// Tasks stranded past a real section break are invisible to every count, so
// they must be reported rather than silently dropped.
func TestOrphanedTasksAreReported(t *testing.T) {
	body := `# Progress: Demo

## Milestones

### M1: Work
- [x] P1-M1-1: a

## Session History

- [ ] P1-M9-1: stranded outside every milestone
- [!] P1-M9-2: also stranded
`
	orphans := orphanedTaskLines(body)
	if len(orphans) != 2 {
		t.Fatalf("orphanedTaskLines = %d, want 2: %+v", len(orphans), orphans)
	}
	if orphans[0].ID != "P1-M9-1" || orphans[0].Line != 10 {
		t.Errorf("first orphan = %s at line %d, want P1-M9-1 at 11", orphans[0].ID, orphans[0].Line)
	}
	if orphans[0].Status != taskTodo || orphans[1].Status != taskBlocked {
		t.Errorf("orphan statuses = %v, %v — want todo, blocked", orphans[0].Status, orphans[1].Status)
	}

	v := detectOrphanViolations("demo", body)
	if len(v) != 2 {
		t.Fatalf("detectOrphanViolations = %d, want 2", len(v))
	}
	if v[0].Rule != "task_outside_milestone" || !strings.Contains(v[0].Message, "PROGRESS.md:10") {
		t.Errorf("violation should name the rule and the line: %+v", v[0])
	}

	// The rendered report must not print an empty [] bracket for a violation
	// that legitimately has neither milestone nor task ID.
	var sb strings.Builder
	renderValidationReport(&sb, []validationViolation{{Feature: "demo", Rule: "task_outside_milestone", Message: "m"}})
	if strings.Contains(sb.String(), "[]") {
		t.Errorf("empty bracket rendered:\n%s", sb.String())
	}
}

// A clean file must produce no orphan violations, or the rule fires everywhere
// and gets ignored.
func TestNoOrphansOnCleanFile(t *testing.T) {
	clean := `# Progress: Demo

## Milestones

### M1: Work
- [x] P1-M1-1: a
- [v] P1-M1-2: b

## Session History

| Session | Date |
|---------|------|
| 1       | now  |

## Decisions Log

1. Chose X over Y.
`
	if got := orphanedTaskLines(clean); len(got) != 0 {
		t.Errorf("false positives on a clean file: %+v", got)
	}
}

// The scope-guard snapshot reads the region independently. If it disagrees with
// parseMilestones about where a milestone ends, the guard reverts the wrong
// lines.
func TestSnapshotAgreesWithParserOnIndentedHeading(t *testing.T) {
	snap := parseProgressSnapshot("P", `### M1: Work
- [x] P1-M1-1: a

  ## A heading quoted in the body

- [ ] P1-M1-2: b
`)
	if len(snap.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(snap.Blocks))
	}
	if len(snap.Blocks[0].TaskStates) != 2 {
		t.Errorf("snapshot sees %d tasks, parser sees 2 — the guard would revert the wrong lines: %v",
			len(snap.Blocks[0].TaskStates), snap.Blocks[0].TaskStates)
	}
}
