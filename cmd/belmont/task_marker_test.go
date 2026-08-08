package main

import "testing"

// Issue #27: an unrecognised checkbox marker was silently mapped to taskTodo.
// The entry was counted in the totals, rendered as `[ ]`, and — the part that
// actually cost real work — handed to an agent as "Next Individual Task".
//
// These tests pin all four halves of the fix: capital [X]/[V] are recognised,
// anything else becomes taskUnknown, unknown is never scheduled, and
// `belmont validate` treats it as a hard violation so `belmont auto` refuses
// to start against a file it cannot read.

const markerProgress = `# Progress: Demo

## Milestones

### M1: Mixed markers
- [ ] P1-M1-1: todo
- [>] P1-M1-2: in progress
- [x] P1-M1-3: done lower
- [X] P1-M1-4: done capital
- [v] P1-M1-5: verified lower
- [V] P1-M1-6: capital V, not a state
- [!] P1-M1-7: blocked
- [?] P1-M1-8: withdrawn
- [-] P1-M1-9: cancelled
- [~] P1-M1-10: partial
`

func taskByID(ms []milestone, id string) *task {
	for _, m := range ms {
		for i := range m.Tasks {
			if m.Tasks[i].ID == id {
				return &m.Tasks[i]
			}
		}
	}
	return nil
}

func TestParseMilestonesMarkerStates(t *testing.T) {
	ms := parseMilestones(markerProgress)

	want := map[string]taskStatus{
		"P1-M1-1":  taskTodo,
		"P1-M1-2":  taskInProgress,
		"P1-M1-3":  taskDone,
		"P1-M1-4":  taskDone, // capital X — GitHub renders it checked
		"P1-M1-5":  taskVerified,
		"P1-M1-6":  taskUnknown, // capital V — deliberately NOT verified, see canonicalMarker
		"P1-M1-7":  taskBlocked,
		"P1-M1-8":  taskUnknown,
		"P1-M1-9":  taskUnknown,
		"P1-M1-10": taskUnknown,
	}
	for id, wantStatus := range want {
		got := taskByID(ms, id)
		if got == nil {
			t.Fatalf("task %s not parsed", id)
		}
		if got.Status != wantStatus {
			t.Errorf("task %s: status = %v, want %v (marker %q)", id, got.Status, wantStatus, got.Marker)
		}
	}
}

// The specific regression from the issue: a capital [X] must not resurrect
// completed work as outstanding.
func TestCapitalXIsNotTodo(t *testing.T) {
	ms := parseMilestones("# P\n\n## Milestones\n\n### M1: Done\n- [x] P1-M1-1: a\n- [X] P1-M1-2: b\n")
	if !milestoneAllDone(ms[0]) {
		t.Fatalf("milestone with [x] and [X] should read as all done, got %+v", ms[0].Tasks)
	}
	if nt := nextTask(flattenTasks(ms, 0)); nt != nil {
		t.Errorf("next task = %q, want none — every task is done", nt.ID)
	}
}

// The dangerous half: an entry Belmont could not parse must never be handed to
// an agent as the next thing to build.
func TestUnknownMarkerIsNeverScheduled(t *testing.T) {
	ms := parseMilestones("# P\n\n## Milestones\n\n### M1: M\n- [x] P1-M1-1: done\n- [?] P1-M1-2: withdrawn\n")
	if nt := nextTask(flattenTasks(ms, 0)); nt != nil {
		t.Errorf("next task = %q (status %v), want none — the only incomplete entry is unparseable",
			nt.ID, nt.Status)
	}
}

func TestUnknownMarkerNotCountedAsTodo(t *testing.T) {
	ms := parseMilestones(markerProgress)
	counts := map[taskStatus]int{}
	for _, tk := range flattenTasks(ms, 0) {
		counts[tk.Status]++
	}
	if counts[taskTodo] != 1 {
		t.Errorf("todo count = %d, want 1 — unknown markers must not inflate it", counts[taskTodo])
	}
	if counts[taskUnknown] != 4 {
		t.Errorf("unknown count = %d, want 4 ([V] [?] [-] [~])", counts[taskUnknown])
	}
}

// A milestone holding an unparseable entry is not complete. It stalls visibly
// rather than completing on a state nobody declared.
func TestUnknownMarkerBlocksMilestoneCompletion(t *testing.T) {
	ms := parseMilestones("# P\n\n## Milestones\n\n### M1: M\n- [x] P1-M1-1: done\n- [?] P1-M1-2: withdrawn\n")
	if milestoneAllDone(ms[0]) {
		t.Error("milestone with an unrecognised marker must not read as all done")
	}
	if milestoneAllVerified(ms[0]) {
		t.Error("milestone with an unrecognised marker must not read as all verified")
	}
}

func TestUnknownMarkerRecordsMarkerAndLine(t *testing.T) {
	ms := parseMilestones(markerProgress)
	tk := taskByID(ms, "P1-M1-8")
	if tk.Marker != "?" {
		t.Errorf("marker = %q, want %q", tk.Marker, "?")
	}
	// Line 13 of markerProgress (1-based): the `[?]` entry.
	if tk.Line != 13 {
		t.Errorf("line = %d, want 13", tk.Line)
	}
}

// validate must hard-fail, because `belmont auto` lints at startup — that is
// what stops the loop running against a PROGRESS.md it cannot read.
func TestValidateFlagsUnrecognisedMarkers(t *testing.T) {
	ms := parseMilestones(markerProgress)
	violations := detectViolations("demo", ms)

	var found []string
	for _, v := range violations {
		if v.Rule == "unrecognised_task_marker" {
			found = append(found, v.TaskID)
		}
	}
	if len(found) != 4 {
		t.Fatalf("unrecognised_task_marker violations = %d (%v), want 4", len(found), found)
	}
	for _, v := range violations {
		if v.Rule != "unrecognised_task_marker" {
			continue
		}
		if v.Message == "" {
			t.Error("violation message is empty")
		}
	}
}

// A clean file must produce no marker violations — otherwise the rule would
// fire on every project and get ignored.
func TestValidateSilentOnRecognisedMarkers(t *testing.T) {
	clean := "# P\n\n## Milestones\n\n### M1: M\n- [ ] P1-M1-1: a\n- [>] P1-M1-2: b\n- [x] P1-M1-3: c\n- [X] P1-M1-4: d\n- [v] P1-M1-5: e\n- [!] P1-M1-6: f\n"
	for _, v := range detectViolations("demo", parseMilestones(clean)) {
		if v.Rule == "unrecognised_task_marker" {
			t.Errorf("false positive on a recognised marker: %s", v.Message)
		}
	}
}

func TestUnknownMarkerTasksHelper(t *testing.T) {
	got := unknownMarkerTasks(parseMilestones(markerProgress))
	if len(got) != 4 {
		t.Fatalf("unknownMarkerTasks returned %d, want 4", len(got))
	}
	// Document order, so diagnostics read top-to-bottom like the file.
	wantOrder := []string{"P1-M1-6", "P1-M1-8", "P1-M1-9", "P1-M1-10"}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("position %d = %s, want %s", i, got[i].ID, w)
		}
	}
}

func TestUnknownMarkerRendersDistinctly(t *testing.T) {
	if icon := taskStatusIcon(taskUnknown, false); icon != "[?]" {
		t.Errorf("icon = %q, want %q — rendering it as [ ] is how the bug stayed invisible", icon, "[?]")
	}
}
