package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests pin the P0-4 census.
//
// The census answers a question the wave structure depends on — whether any
// feature's index alone still exceeds the ceiling after extraction — so the
// thing that must not break is the split between what moves and what stays.

func writeRegister(t *testing.T, root, slug, content string) {
	t.Helper()
	dir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCensusSplitsIndentedBodiesFromTheRegister pins the core measurement: the
// indented body beneath a task line moves, the task head line stays.
func TestCensusSplitsIndentedBodiesFromTheRegister(t *testing.T) {
	content := strings.Join([]string{
		"# Progress: Thing",
		"",
		"## Milestones",
		"",
		"### M1: First",
		"- [x] P0-1: Head line stays",
		"  detail line one moves",
		"  detail line two moves",
		"- [ ] P0-2: No body",
		"",
		"### M2: Second",
		"- [ ] P1-1: Another head",
		"  moves too",
		"",
	}, "\n")

	fc := censusFeature("repo", "thing", content)

	if fc.Milestones != 2 {
		t.Errorf("milestones: got %d, want 2", fc.Milestones)
	}
	if fc.Tasks != 3 {
		t.Errorf("tasks: got %d, want 3", fc.Tasks)
	}
	if fc.TasksWith != 2 {
		t.Errorf("tasks with detail: got %d, want 2", fc.TasksWith)
	}

	// Exactly the three indented lines move, each counted with its newline.
	wantDetail := len("  detail line one moves") + 1 +
		len("  detail line two moves") + 1 +
		len("  moves too") + 1
	if fc.DetailBytes != wantDetail {
		t.Errorf("detail bytes: got %d, want %d", fc.DetailBytes, wantDetail)
	}
	if fc.IndexBytes != fc.TotalBytes-fc.DetailBytes {
		t.Errorf("index must be the remainder: %d != %d-%d", fc.IndexBytes, fc.TotalBytes, fc.DetailBytes)
	}

	// Nothing is lost or invented: the two halves reconstruct the whole.
	if fc.IndexBytes+fc.DetailBytes != fc.TotalBytes {
		t.Errorf("index+detail must equal total: %d+%d != %d", fc.IndexBytes, fc.DetailBytes, fc.TotalBytes)
	}
}

// TestCensusCountsANestedTaskBodyOnce pins the double-count guard. A nested task
// bullet lies inside its parent's body as well as being a task in its own right;
// summing per-task body lengths would count those lines twice and overstate what
// extraction saves.
func TestCensusCountsANestedTaskBodyOnce(t *testing.T) {
	content := strings.Join([]string{
		"### M1: One",
		"- [ ] P0-1: Parent",
		"  - [ ] P0-2: Nested child",
		"    child body",
		"",
	}, "\n")

	fc := censusFeature("repo", "nested", content)
	if fc.IndexBytes+fc.DetailBytes != fc.TotalBytes {
		t.Errorf("double count: index %d + detail %d != total %d", fc.IndexBytes, fc.DetailBytes, fc.TotalBytes)
	}
	if fc.DetailBytes > fc.TotalBytes {
		t.Fatalf("detail %d exceeds total %d — lines counted more than once", fc.DetailBytes, fc.TotalBytes)
	}
}

// TestCensusReportsZeroDetailForAFlatRegister is the finding that changed P0-4's
// answer. Three of the five registers that stay over the ceiling contain NO
// indented lines at all — their bulk is in task head lines, so extraction moves
// nothing. A census that quietly assumed size implies narrative would have
// reported the opposite conclusion.
func TestCensusReportsZeroDetailForAFlatRegister(t *testing.T) {
	content := strings.Join([]string{
		"### M1: Flat",
		"- [x] P0-1: " + strings.Repeat("a very long single-line task title ", 200),
		"- [x] P0-2: " + strings.Repeat("another long one ", 200),
		"",
	}, "\n")

	fc := censusFeature("repo", "flat", content)
	if fc.DetailBytes != 0 {
		t.Errorf("a register with no indented lines has no detail to move, got %d B", fc.DetailBytes)
	}
	if fc.IndexBytes != fc.TotalBytes {
		t.Errorf("index should be the whole file: got %d of %d", fc.IndexBytes, fc.TotalBytes)
	}
}

// TestCensusThresholdAnswersTheOpenQuestion pins the flag the wave structure
// turns on.
func TestCensusThresholdAnswersTheOpenQuestion(t *testing.T) {
	small := "### M1: X\n- [ ] P0-1: tiny\n"
	if censusFeature("r", "small", small).OverThreshold {
		t.Error("a tiny register must not be flagged")
	}

	// Over the ceiling, and flat, so extraction cannot rescue it.
	big := "### M1: X\n- [ ] P0-1: " + strings.Repeat("x", extractionThresholdBytes+1000) + "\n"
	fc := censusFeature("r", "big", big)
	if !fc.OverThreshold {
		t.Errorf("index of %d B exceeds the %d B ceiling and must be flagged", fc.IndexBytes, extractionThresholdBytes)
	}
}

// TestCensusStatesItsDenominator pins the reporting rule: feature directories
// and live registers are different numbers, and archived directories must be
// counted separately rather than silently dropped.
func TestCensusStatesItsDenominator(t *testing.T) {
	root := t.TempDir()
	writeRegister(t, root, "alpha", "### M1: A\n- [ ] P0-1: one\n  body\n")
	writeRegister(t, root, "beta", "### M1: B\n- [ ] P0-1: two\n")

	// An archived feature: directory present, no PROGRESS.md, has ARCHIVE.md.
	arch := filepath.Join(root, ".belmont", "features", "gamma")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(arch, "ARCHIVE.md"), []byte("archived\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory with neither.
	if err := os.MkdirAll(filepath.Join(root, ".belmont", "features", "delta"), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := runCensus([]string{root}, "", false)
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if rep.FeatureDirs != 4 {
		t.Errorf("feature dirs: got %d, want 4", rep.FeatureDirs)
	}
	if rep.LiveRegisters != 2 {
		t.Errorf("live registers: got %d, want 2", rep.LiveRegisters)
	}
	if rep.ArchivedDirs != 1 {
		t.Errorf("archived dirs: got %d, want 1", rep.ArchivedDirs)
	}
	if rep.NoRegisterDirs != 2 {
		t.Errorf("dirs without a register: got %d, want 2", rep.NoRegisterDirs)
	}

	out := renderCensus(rep)
	if !strings.Contains(out, "Denominator:") {
		t.Error("the rendered census must state its denominator")
	}
	if !strings.Contains(out, "nothing written") {
		t.Error("the census must say it wrote nothing")
	}
}

// TestCensusIgnoresDotDirectories mirrors listFeaturesWithOverrides, so the
// census and the rest of Belmont agree on what a feature directory is.
func TestCensusIgnoresDotDirectories(t *testing.T) {
	root := t.TempDir()
	writeRegister(t, root, "real", "### M1: A\n- [ ] P0-1: one\n")
	writeRegister(t, root, ".hidden", "### M1: A\n- [ ] P0-1: one\n")

	rep, err := runCensus([]string{root}, "", false)
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if rep.LiveRegisters != 1 {
		t.Errorf("dot-directories must be skipped: got %d live registers", rep.LiveRegisters)
	}
}

// TestExtractRefusesToWrite pins the milestone boundary. P0-4 ships the census
// only; the move and its byte-for-byte round-trip proof are M3/P1-1. A flag that
// looks available and silently does nothing would be worse than a refusal.
func TestExtractRefusesToWrite(t *testing.T) {
	err := runExtractCmd([]string{"--root", t.TempDir()})
	if err == nil {
		t.Fatal("extract without --dry-run must refuse in this release")
	}
	if !strings.Contains(err.Error(), "--dry-run is required") {
		t.Errorf("refusal should name the missing flag, got: %v", err)
	}
}

// TestCensusWritesNothing — the census must leave the tree exactly as it found it.
func TestCensusWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeRegister(t, root, "alpha", "### M1: A\n- [ ] P0-1: one\n  body\n")

	before, err := os.ReadDir(filepath.Join(root, ".belmont", "features", "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runCensus([]string{root}, "", false); err != nil {
		t.Fatalf("census: %v", err)
	}
	after, err := os.ReadDir(filepath.Join(root, ".belmont", "features", "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Errorf("census created files: %d before, %d after", len(before), len(after))
	}
	for _, e := range after {
		if e.Name() == "details" {
			t.Error("census must not create a details/ directory — that is M3")
		}
	}
}

// TestCensusMissingRootIsAnError pins the fix for P0-M1-FIX-7, and replaces a
// test that asserted the defect ("a missing root should be skipped"). Skipping
// it is precisely what let this census walk four of the PRD's five repos and
// publish the smaller denominator as a finding.
func TestCensusMissingRootIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")

	rep, err := runCensus([]string{missing}, "", false)
	if err == nil {
		t.Fatal("a root that cannot be read must fail the census, not shrink its denominator")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error must name the root it could not read, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--allow-unreadable-roots") {
		t.Errorf("the error must name the deliberate way to proceed, got: %v", err)
	}
	// The report still carries the coverage facts even on the failing path.
	if rep.CoverageComplete {
		t.Error("coverage cannot be complete when a root was never read")
	}
	if len(rep.UnreadableRoots) != 1 || rep.UnreadableRoots[0].Root != missing {
		t.Errorf("the unread root must be recorded in the report, got %+v", rep.UnreadableRoots)
	}
}

// TestCensusPartialWalkStatesItsOwnCoverage pins the second half of the rule:
// the opt-in must not be a quieter version of the silent skip. A reader of the
// output alone — not of the terminal it scrolled past — must be able to tell
// which roots were walked and which were missed.
func TestCensusPartialWalkStatesItsOwnCoverage(t *testing.T) {
	good := t.TempDir()
	writeRegister(t, good, "alpha", "### M1: A\n- [ ] P0-1: one\n  body\n")
	missing := filepath.Join(t.TempDir(), "gone")

	rep, err := runCensus([]string{good, missing}, "", true)
	if err != nil {
		t.Fatalf("--allow-unreadable-roots must complete the run: %v", err)
	}

	// The measurement itself is unchanged — only its honesty about coverage.
	if rep.LiveRegisters != 1 {
		t.Errorf("the readable root must still be measured: got %d live registers", rep.LiveRegisters)
	}
	if rep.CoverageComplete {
		t.Error("coverage must be reported incomplete")
	}
	if len(rep.Roots) != 2 {
		t.Errorf("the report must keep the roots that were asked for: %v", rep.Roots)
	}
	if len(rep.RootsWalked) != 1 || rep.RootsWalked[0] != good {
		t.Errorf("roots walked: got %v, want [%s]", rep.RootsWalked, good)
	}
	if len(rep.UnreadableRoots) != 1 || rep.UnreadableRoots[0].Root != missing {
		t.Errorf("unreadable roots: got %+v, want [%s]", rep.UnreadableRoots, missing)
	}

	out := renderCensus(rep)
	if !strings.Contains(out, "Coverage: INCOMPLETE") {
		t.Error("the rendered census must state that its coverage is incomplete")
	}
	if !strings.Contains(out, "walked  "+good) {
		t.Error("the rendered census must name the roots it walked")
	}
	if !strings.Contains(out, "MISSED  "+missing) {
		t.Error("the rendered census must name the root it missed")
	}
	if !strings.Contains(out, "COVERAGE INCOMPLETE") {
		t.Error("the verdict must be repeated below the figures, not only above them")
	}
}

// TestCensusCompleteWalkSaysSo — the good case must be as legible as the bad
// one, or a reader cannot tell a checked census from an unchecked one.
func TestCensusCompleteWalkSaysSo(t *testing.T) {
	root := t.TempDir()
	writeRegister(t, root, "alpha", "### M1: A\n- [ ] P0-1: one\n  body\n")

	rep, err := runCensus([]string{root}, "", false)
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if !rep.CoverageComplete {
		t.Error("every requested root was read; coverage is complete")
	}
	out := renderCensus(rep)
	if !strings.Contains(out, "Coverage: COMPLETE") {
		t.Error("the rendered census must state that it covered everything it was asked to")
	}
	if !strings.Contains(out, "walked  "+root) {
		t.Error("the rendered census must name the root it walked")
	}
	if strings.Contains(out, "COVERAGE INCOMPLETE") {
		t.Error("a complete walk must not carry the incomplete-coverage footer")
	}
}

// TestCensusUnlistableRootIsReportedNotAborted covers the other read failure —
// a directory that exists but cannot be listed. It lands in the same coverage
// report as a missing one rather than aborting mid-walk, so the operator sees
// every unreadable root in one run.
func TestCensusUnlistableRootIsReportedNotAborted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := t.TempDir()
	features := filepath.Join(root, ".belmont", "features")
	if err := os.MkdirAll(features, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(features, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(features, 0o755) })

	rep, err := runCensus([]string{root}, "", true)
	if err != nil {
		t.Fatalf("--allow-unreadable-roots must complete the run: %v", err)
	}
	if rep.CoverageComplete || len(rep.UnreadableRoots) != 1 {
		t.Fatalf("an unlistable root must be reported: complete=%v unreadable=%+v", rep.CoverageComplete, rep.UnreadableRoots)
	}
	if rep.UnreadableRoots[0].Reason == "" {
		t.Error("the report must say why the root could not be read")
	}
}
