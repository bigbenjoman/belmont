package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Upgrading Belmont must not refuse a run that worked the day before. #27's
// unrecognised marker genuinely stops the loop — a milestone holding one can
// never read complete — so it blocks. An orphaned task line does not stop
// anything; it is information that must not be dropped. Conflating the two
// meant a `- [ ] follow up next week` bullet in a retro aborted `belmont auto`.

const severityDoc = `# Progress: Demo

## Milestones

### M1: Work
- [v] P1-M1-1: first

## Session History

- [ ] follow up with design next week
`

func TestOrphanedTaskLinesAreWarningsNotErrors(t *testing.T) {
	v := detectOrphanViolations("demo", severityDoc)
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].Severity != severityWarning {
		t.Errorf("orphan severity = %q, want %q — a bullet below the region is not a reason to refuse a run",
			v[0].Severity, severityWarning)
	}
	blocking, advisory := splitBySeverity(v)
	if len(blocking) != 0 || len(advisory) != 1 {
		t.Errorf("split = %d blocking / %d advisory, want 0 / 1", len(blocking), len(advisory))
	}
}

func TestUnrecognisedMarkerStaysBlocking(t *testing.T) {
	ms := parseMilestones("### M1: Work\n- [-] P1-M1-1: withdrawn\n")
	v := detectViolations("demo", ms)
	if len(v) == 0 {
		t.Fatal("no violation for an unrecognised marker")
	}
	for _, x := range v {
		if x.Rule != "unrecognised_task_marker" {
			continue
		}
		if x.Severity != severityError {
			t.Errorf("severity = %q, want %q — this is the #27 bug and it must stop the loop", x.Severity, severityError)
		}
	}
	blocking, _ := splitBySeverity(v)
	if len(blocking) == 0 {
		t.Error("an unrecognised marker must be blocking")
	}
}

// Every rule must set a severity explicitly. splitBySeverity treats an unset
// one as blocking, so a forgotten field fails loudly rather than silently
// downgrading a real error to advice — but it should never get that far.
func TestEveryViolationCarriesASeverity(t *testing.T) {
	doc := "### M1: Polish and cleanup\n- [-] P1-M1-1: withdrawn\n- [ ] P2-M2-1: wrong milestone\n\n" +
		"### M1: duplicate heading\n- [ ] P1-M1-9: x\n\n## Session History\n- [ ] a stray bullet\n"
	v := append(detectViolations("demo", parseMilestones(doc)), detectOrphanViolations("demo", doc)...)
	if len(v) < 4 {
		t.Fatalf("expected the fixture to trip at least 4 rules, got %d: %+v", len(v), v)
	}
	seen := map[string]bool{}
	for _, x := range v {
		seen[x.Rule] = true
		if x.Severity != severityError && x.Severity != severityWarning {
			t.Errorf("rule %q has severity %q, want %q or %q", x.Rule, x.Severity, severityError, severityWarning)
		}
	}
	for _, rule := range []string{
		"polish_milestone_name", "unrecognised_task_marker", "cross_milestone_task_id",
		"duplicate_milestone_id", "task_outside_milestone",
	} {
		if !seen[rule] {
			t.Errorf("fixture did not trip %q, so its severity is unpinned", rule)
		}
	}
}

// splitBySeverity's fallback: anything without an explicit severity is
// blocking. A new rule that forgets the field must fail loudly, never be
// silently downgraded to advice.
func TestSplitBySeverityTreatsUnsetAsBlocking(t *testing.T) {
	blocking, advisory := splitBySeverity([]validationViolation{
		{Rule: "forgot_to_set_one"},
		{Rule: "warn", Severity: severityWarning},
		{Rule: "err", Severity: severityError},
	})
	if len(blocking) != 2 {
		t.Errorf("blocking = %d, want 2 — an unset severity must not read as advisory", len(blocking))
	}
	if len(advisory) != 1 {
		t.Errorf("advisory = %d, want 1", len(advisory))
	}
}

// The lint gate itself, driven through runAutoCmd. Testing detectViolations in
// isolation cannot catch the gate blocking on warnings, nor the preflight call
// sites being deleted. --from/--to skips the interactive milestone picker,
// which would otherwise fire because `go test`'s stdin is a character device.
func TestAutoGateBySeverity(t *testing.T) {
	warnOnly := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [ ] P1-M1-1: pending\n\n"+
		"## Session History\n- [ ] follow up next week\n")
	if err := runAutoCmd([]string{"--root", warnOnly, "--feature", "demo", "--from", "M1", "--to", "M1", "--dry-run"}); err != nil {
		t.Errorf("auto refused a run over a warning-only file: %v — this is the upgrade break", err)
	}

	blocked := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [-] P1-M1-1: withdrawn\n")
	if err := runAutoCmd([]string{"--root", blocked, "--feature", "demo", "--from", "M1", "--to", "M1", "--dry-run"}); err == nil {
		t.Error("auto started against an unrecognised marker; that is issue #27's gate")
	}
}

func TestAutoPreflightRefusesDuplicateMilestoneOnBothPaths(t *testing.T) {
	root := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [ ] P1-M1-1: pending\n\n"+
		"## Session History\n\n### M1: retry notes\n- attempt log\n")

	for _, args := range [][]string{
		{"--root", root, "--feature", "demo", "--from", "M1", "--to", "M1", "--dry-run"},
		{"--root", root, "--features", "demo", "--dry-run"},
	} {
		err := runAutoCmd(args)
		if err == nil {
			t.Errorf("%v: auto started with both runtime guards switched off", args)
			continue
		}
		if !strings.Contains(err.Error(), "guard") {
			t.Errorf("%v: refused for the wrong reason: %v", args, err)
		}
	}
}

// Every violation must say where its answer comes from, and the report must
// show the shape rather than cite a file. `skills/belmont/_partials/` is a
// build-time source path `belmont install` never writes, so the line that used
// to promise "the canonical rule" was dangling in every consuming project.
func TestReportTeachesConformance(t *testing.T) {
	doc := "### M1: Polish and cleanup\n- [-] P1-M1-1: withdrawn\n- [ ] P2-M2-1: wrong milestone\n\n" +
		"### M1: duplicate heading\n- [ ] P1-M1-9: x\n\n## Session History\n- [ ] a stray bullet\n"
	v := append(detectViolations("demo", parseMilestones(doc)), detectOrphanViolations("demo", doc)...)

	for _, x := range v {
		if x.Remedy != remedyNeedsEvidence && x.Remedy != remedyTechPlan {
			t.Errorf("rule %q has remedy %q — an agent cannot tell whether to investigate or route it",
				x.Rule, x.Remedy)
		}
	}

	// Assert against the help block itself. Checking the whole report lets a
	// violation's own message satisfy the assertion — the marker set also
	// appears in the unrecognised-marker text, so dropping it from the help
	// block passed until this was split out.
	for _, want := range []string{
		"### M<n>: Name",         // the milestone header shape
		"[ ] [>] [x] [v] [!]",    // the legal markers
		"ENDS the",               // where the region stops
		"Indentation is load-be", // why an indented heading is not a break
		"git log",                // how to establish the answer from the repo
		"/belmont:tech-plan",     // where structural fixes go
	} {
		if !strings.Contains(progressStructureHelp, want) {
			t.Errorf("the structure block never shows %q, so an agent cannot conform the file from it", want)
		}
	}

	var buf strings.Builder
	renderValidationReport(&buf, v)
	out := buf.String()

	if strings.Contains(out, "_partials/") {
		t.Error("report cites a build-time source path that no installed project contains")
	}
	if !strings.Contains(out, progressStructureHelp) {
		t.Error("the structure block is not printed with the report")
	}
	// The remedy has to reach the reader, not just the struct.
	for _, want := range []string{"[" + remedyNeedsEvidence + "]", "[" + remedyTechPlan + "]"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered report never shows %s, so the routing is invisible in text output", want)
		}
	}
}

// The scope guard's correction is written into STEERING.md and prepended to the
// next phase's prompt. A file path in there is worse than one in a report: the
// agent is told to follow it, and `skills/belmont/_partials/` does not exist in
// any project Belmont installs into.
func TestSteeringCorrectionsCiteNoBuildTimePaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := loopConfig{Root: root, Feature: "demo"}
	action := loopAction{MilestoneID: "M1"}
	injectScopeGuardSteering(cfg, action, []scopeViolation{
		{Kind: "new_milestone", Milestone: "M9", MilestoneName: "Polish"},
		{Kind: "out_of_scope_flip", Milestone: "M2", TaskID: "P1-M2-1", FromState: " ", ToState: "x"},
	})

	data, err := os.ReadFile(filepath.Join(dir, "STEERING.md"))
	if err != nil {
		t.Fatalf("no steering entry written: %v", err)
	}
	if strings.Contains(string(data), "_partials/") {
		t.Errorf("steering tells the agent to read a path that does not exist in a project:\n%s", data)
	}
	if !strings.Contains(string(data), "/belmont:tech-plan") {
		t.Error("steering does not name the skill that may change milestone structure")
	}
}

func writeValidateFixture(t *testing.T, doc string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".belmont", "features", "demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# PRD: Demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestValidateExitCodesBySeverity(t *testing.T) {
	root := writeValidateFixture(t, severityDoc)

	if err := runValidateCmd([]string{"--root", root}); err != nil {
		t.Errorf("validate failed on a warning-only file: %v — upgrading would break a working project", err)
	}
	if err := runValidateCmd([]string{"--root", root, "--strict"}); err == nil {
		t.Error("validate --strict returned nil on a warning-only file; CI has no way to fail on warnings")
	}

	blockingRoot := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [-] P1-M1-1: withdrawn\n")
	if err := runValidateCmd([]string{"--root", blockingRoot}); err == nil {
		t.Error("validate returned nil on an unrecognised marker; that is issue #27's gate")
	}

	// Colliding milestone IDs must block here too. `belmont auto` refuses on its
	// own preflight, so validate is the only surface that tells someone editing
	// the file by hand — and it is what the auto error tells them to run.
	dupRoot := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [ ] P1-M1-1: pending\n\n"+
		"## Session History\n\n### M1: retry notes\n- attempt log\n")
	if err := runValidateCmd([]string{"--root", dupRoot}); err == nil {
		t.Error("validate returned nil on colliding milestone IDs, which switch both runtime guards off")
	}
}

// The guards cannot place a milestone whose ID appears twice, so they decline —
// and the multi-feature path never reaches the structural lint. Without a hard
// preflight a stray `### M<n>:` session note runs a whole feature with both
// runtime guards silently absent.
func TestAutoRefusesDuplicateMilestoneIDs(t *testing.T) {
	root := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [ ] P1-M1-1: pending\n\n"+
		"## Session History\n\n### M1: retry notes\n- attempt log\n")

	err := requireUnambiguousMilestones(root, []string{"demo"})
	if err == nil {
		t.Fatal("auto would start with both runtime guards switched off")
	}
	for _, want := range []string{"demo", "M1", "guard"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not mention %q: %s", want, err)
		}
	}

	clean := writeValidateFixture(t, "# Progress\n\n### M1: Work\n- [ ] P1-M1-1: pending\n\n## Session History\n- a note\n")
	if err := requireUnambiguousMilestones(clean, []string{"demo"}); err != nil {
		t.Errorf("refused an unambiguous file: %v", err)
	}
	// A feature with no PROGRESS.md is reported elsewhere, not here.
	if err := requireUnambiguousMilestones(clean, []string{"nonexistent"}); err != nil {
		t.Errorf("preflight should ignore a missing PROGRESS.md, got: %v", err)
	}
}
