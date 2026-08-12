package main

import (
	"strings"
	"testing"
)

// `parseTaskID` is the single definition of what a task ID looks like. It exists
// because two definitions disagreed: the runtime guards happily treated
// `FWLUP-SWEEP-1` as a task, while every reader that needed the task's IDENTITY
// required a `P<n>-` prefix — so `belmont repair` printed "(no task ID)" for a
// line the guards were tracking, and never asked the commit log about it. On the
// reporting project that was 40 tasks from one cross-cutting sweep. See issue
// #34.
//
// The widening has to stop short of prose, which is the whole difficulty: a task
// line is free text and most of it contains colons.
func TestParseTaskIDAcceptsHandWrittenIDsWithoutEatingProse(t *testing.T) {
	cases := []struct {
		text   string
		wantID string
		name   string
	}{
		// The shape Belmont's own templates emit — unchanged, and first in the
		// alternation precisely so it cannot regress.
		{"P0-M1-1: build the thing", "P0-M1-1", "build the thing"},
		{"P1-FWLUP-M2-3: the follow-up", "P1-FWLUP-M2-3", "the follow-up"},
		{"P1-auth: no trailing number", "P1-auth", "no trailing number"},
		// Hand-written IDs. These are the issue.
		{"FWLUP-SWEEP-1: a cross-cutting follow-up", "FWLUP-SWEEP-1", "a cross-cutting follow-up"},
		{"E2E-4: flaky spec", "E2E-4", "flaky spec"},
		// Prose. Every one of these would become a bogus task ID under the
		// guards' wider `(\S+?):`, and a bogus ID gets looked up in the commit
		// log like a real one.
		{"Note: something worth remembering", "", "Note: something worth remembering"},
		{"Fix the login: it breaks on Safari", "", "Fix the login: it breaks on Safari"},
		{"TODO: chase this", "", "TODO: chase this"},
		{"re-run: the whole suite", "", "re-run: the whole suite"},
		{"a plain task with no colon at all", "", "a plain task with no colon at all"},
	}
	for _, c := range cases {
		id, name, ok := parseTaskID(c.text)
		if id != c.wantID {
			t.Errorf("parseTaskID(%q) id = %q, want %q", c.text, id, c.wantID)
		}
		if name != c.name {
			t.Errorf("parseTaskID(%q) name = %q, want %q", c.text, name, c.name)
		}
		if ok != (c.wantID != "") {
			t.Errorf("parseTaskID(%q) ok = %v, want %v", c.text, ok, c.wantID != "")
		}
	}
}

// Both identity readers must route through the parser, or the disagreement
// simply moves rather than closing. An orphan is the case from the issue; an
// in-milestone task is the one that feeds commit evidence.
func TestBothTaskReadersResolveAHandWrittenID(t *testing.T) {
	doc := "## Milestones\n\n### M1: Work\n- [ ] FWLUP-SWEEP-9: inside a milestone\n\n" +
		"## Session History\n\n- [ ] FWLUP-SWEEP-1: outside every milestone\n"

	ms := parseMilestones(doc)
	if len(ms) != 1 || len(ms[0].Tasks) != 1 {
		t.Fatalf("parseMilestones = %+v", ms)
	}
	if ms[0].Tasks[0].ID != "FWLUP-SWEEP-9" || ms[0].Tasks[0].Name != "inside a milestone" {
		t.Errorf("in-milestone task = %+v, want the ID split off", ms[0].Tasks[0])
	}

	orphans := orphanedTaskLines(doc)
	if len(orphans) != 1 {
		t.Fatalf("orphanedTaskLines = %+v", orphans)
	}
	if orphans[0].ID != "FWLUP-SWEEP-1" || orphans[0].Name != "outside every milestone" {
		t.Errorf("orphan = %+v, want the ID split off", orphans[0])
	}
}

// A finding with an ID gets a commit-evidence lookup; one without gets told the
// commit log cannot speak to it. That is the practical cost of the mismatch, so
// pin it at the level the user sees.
func TestRepairAsksTheCommitLogAboutAHandWrittenID(t *testing.T) {
	root := gitFixture(t,
		"## Milestones\n\n### M1: Work\n- [ ] P1-M1-1: a\n\n## Session History\n\n- [ ] FWLUP-SWEEP-1: the sweep's follow-up\n",
		"FWLUP-SWEEP-1: land the sweep follow-up")

	findings := collectRepairFindings(progressOf(t, root))
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	if findings[0].TaskID != "FWLUP-SWEEP-1" {
		t.Fatalf("finding has no task ID: %+v", findings[0])
	}
	withEvidence, _ := attachCommitEvidence(root, "demo", findings)
	if !withEvidence[0].Evidence.Checked || !withEvidence[0].Evidence.Found {
		t.Errorf("the commit naming the task was not found: %+v", withEvidence[0].Evidence)
	}
}

// The advice on an orphan is conditional, and that is the fix for the guidance
// loop in issue #34. "Move it under its `### M<n>:` heading" is only actionable
// when there is such a heading; when there is not, the reader escalates to
// /belmont:tech-plan, which forbids creating a milestone for follow-ups, and
// lands back on a task outside every milestone.
func TestOrphanAdviceNamesTheMilestoneWhenTheIDDoes(t *testing.T) {
	doc := "## Milestones\n\n### M1: Work\n- [x] P1-M1-1: a\n\n### M2: More\n- [x] P1-M2-1: b\n\n" +
		"## Session History\n\n- [ ] P1-M2-7: strayed out of M2\n"

	v := detectOrphanViolations("demo", doc)
	if len(v) != 1 {
		t.Fatalf("detectOrphanViolations = %+v", v)
	}
	msg := v[0].Message
	if !strings.Contains(msg, "Its ID names M2, which this file already has") {
		t.Errorf("advice does not name the destination it can see:\n%s", msg)
	}
	if !strings.Contains(msg, "belmont repair --feature demo") {
		t.Errorf("advice does not name the command that can move it:\n%s", msg)
	}
	if strings.Contains(msg, "highest-numbered") {
		t.Errorf("an attributable task was sent through the judgement-call rule:\n%s", msg)
	}
}

// The other half: an ID naming no milestone gets the destination rule rather
// than a dangling escalation. A task whose ID names a milestone the file does
// NOT have belongs here too — repair cannot create it, so pointing at it would
// be the same dead end.
func TestOrphanAdviceRulesOnTheDestinationWhenTheIDNamesNone(t *testing.T) {
	doc := "## Milestones\n\n### M1: Work\n- [x] P1-M1-1: a\n\n" +
		"## Session History\n\n- [ ] FWLUP-SWEEP-1: found by a sweep\n- [ ] P1-M9-1: names a milestone that is not here\n"

	v := detectOrphanViolations("demo", doc)
	if len(v) != 2 {
		t.Fatalf("detectOrphanViolations = %+v", v)
	}
	for _, got := range v {
		if !strings.Contains(got.Message, "highest-numbered existing milestone whose work it touches") {
			t.Errorf("no destination rule for an unattributable task:\n%s", got.Message)
		}
		if !strings.Contains(got.Message, "Never a new milestone") {
			t.Errorf("the rule does not close off the forbidden option:\n%s", got.Message)
		}
	}
	// The warning must stay a warning: a `- [ ]` bullet in a retro is not work,
	// and this rule is on `belmont auto`'s startup path.
	if v[0].Severity != severityWarning {
		t.Errorf("severity = %q, want a warning", v[0].Severity)
	}
}
