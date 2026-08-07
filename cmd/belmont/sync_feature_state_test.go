package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover issue #24: in a parallel wave, sibling milestones merge one
// after another into the same main repo, and a whole-directory state sync made
// that last-writer-wins — the final milestone's fork-time PROGRESS.md overwrote
// every mark its siblings had earned.

const waveForkProgress = `# Progress: calc-ops

## Milestones

### M1: Baseline
- [v] P1-M1-1: Baseline calculator exists

### M2: Divide (depends: M1)
- [ ] P1-M2-1: Create divide.js

### M3: Modulo (depends: M1)
- [ ] P1-M3-1: Create modulo.js

## Notes

Nothing yet.
`

// seedWaveFixture writes the fork-time PROGRESS.md into a main repo and into two
// worktrees, then flips each worktree's own milestone task to the given marker.
// Returns (mainRoot, m2Worktree, m3Worktree).
func seedWaveFixture(t *testing.T, slug string) (string, string, string) {
	t.Helper()
	mainRoot := t.TempDir()
	writeFeatureProgress(t, mainRoot, slug, waveForkProgress)

	wt2 := t.TempDir()
	writeFeatureProgress(t, wt2, slug, strings.Replace(waveForkProgress,
		"- [ ] P1-M2-1:", "- [v] P1-M2-1:", 1))

	wt3 := t.TempDir()
	writeFeatureProgress(t, wt3, slug, strings.Replace(waveForkProgress,
		"- [ ] P1-M3-1:", "- [v] P1-M3-1:", 1))

	return mainRoot, wt2, wt3
}

func writeFeatureProgress(t *testing.T, root, slug, content string) {
	t.Helper()
	dir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	mustWrite(t, filepath.Join(dir, "PROGRESS.md"), content)
}

func readFeatureProgress(t *testing.T, root, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md"))
	if err != nil {
		t.Fatalf("read main PROGRESS.md: %v", err)
	}
	return string(data)
}

// TestWaveMergeKeepsEarlierSiblingMarks is the direct regression test for #24.
// M2 merges, then M3 merges. M2's [v] must survive M3's sync.
func TestWaveMergeKeepsEarlierSiblingMarks(t *testing.T) {
	mainRoot, wt2, wt3 := seedWaveFixture(t, "calc-ops")

	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "M2")
	syncFeatureStateAfterMerge(mainRoot, wt3, "calc-ops", "M3")

	got := readFeatureProgress(t, mainRoot, "calc-ops")
	for _, want := range []string{
		"- [v] P1-M1-1:",
		"- [v] P1-M2-1:",
		"- [v] P1-M3-1:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lost a mark: expected %q in merged PROGRESS.md\n---\n%s", want, got)
		}
	}
}

// Merge order must not matter: M3 first, then M2.
func TestWaveMergeKeepsEarlierSiblingMarksReversedOrder(t *testing.T) {
	mainRoot, wt2, wt3 := seedWaveFixture(t, "calc-ops")

	syncFeatureStateAfterMerge(mainRoot, wt3, "calc-ops", "M3")
	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "M2")

	got := readFeatureProgress(t, mainRoot, "calc-ops")
	if !strings.Contains(got, "- [v] P1-M2-1:") || !strings.Contains(got, "- [v] P1-M3-1:") {
		t.Errorf("marks lost with reversed merge order:\n---\n%s", got)
	}
}

// Everything outside the merged milestone's block must survive byte-for-byte:
// preamble, the other milestones, and trailing sections.
func TestWaveMergePreservesSurroundingContent(t *testing.T) {
	mainRoot, wt2, _ := seedWaveFixture(t, "calc-ops")

	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "M2")

	got := readFeatureProgress(t, mainRoot, "calc-ops")
	for _, want := range []string{
		"# Progress: calc-ops",
		"### M1: Baseline",
		"### M3: Modulo (depends: M1)",
		"## Notes",
		"Nothing yet.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("surrounding content lost: %q missing\n---\n%s", want, got)
		}
	}
}

// A worktree's verify phase may add follow-up tasks to its own milestone. Those
// must arrive in the main repo, not be dropped by the narrowed sync.
func TestWaveMergeCarriesFollowUpTasks(t *testing.T) {
	mainRoot, wt2, wt3 := seedWaveFixture(t, "calc-ops")

	// M3's verify phase adds a follow-up under M3.
	writeFeatureProgress(t, wt3, "calc-ops", strings.Replace(
		strings.Replace(waveForkProgress, "- [ ] P1-M3-1:", "- [v] P1-M3-1:", 1),
		"- [v] P1-M3-1: Create modulo.js",
		"- [v] P1-M3-1: Create modulo.js\n- [ ] P1-M3-2: Handle divide-by-zero", 1))

	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "M2")
	syncFeatureStateAfterMerge(mainRoot, wt3, "calc-ops", "M3")

	got := readFeatureProgress(t, mainRoot, "calc-ops")
	if !strings.Contains(got, "- [ ] P1-M3-2: Handle divide-by-zero") {
		t.Errorf("follow-up task added in the worktree did not reach the main repo:\n---\n%s", got)
	}
	if !strings.Contains(got, "- [v] P1-M2-1:") {
		t.Errorf("sibling mark lost while carrying follow-ups:\n---\n%s", got)
	}
}

// A task removed from its own milestone in the worktree (triage moves deferrable
// follow-ups to NOTES.md) must be removed in the main repo too.
func TestWaveMergeHonoursTaskRemovalInOwnMilestone(t *testing.T) {
	mainRoot, wt2, _ := seedWaveFixture(t, "calc-ops")

	writeFeatureProgress(t, wt2, "calc-ops", strings.Replace(waveForkProgress,
		"- [ ] P1-M2-1: Create divide.js\n", "", 1))

	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "M2")

	got := readFeatureProgress(t, mainRoot, "calc-ops")
	if strings.Contains(got, "P1-M2-1") {
		t.Errorf("task deleted in the worktree's own milestone survived the sync:\n---\n%s", got)
	}
}

// Files a sibling merge left in the main repo must not be deleted by a later
// milestone's sync. This is the other half of dropping the RemoveAll.
func TestWaveMergeDoesNotDeleteSiblingFiles(t *testing.T) {
	mainRoot, _, wt3 := seedWaveFixture(t, "calc-ops")

	sibling := filepath.Join(mainRoot, ".belmont", "features", "calc-ops", "M2-REPORT.md")
	mustWrite(t, sibling, "M2 verification report\n")

	syncFeatureStateAfterMerge(mainRoot, wt3, "calc-ops", "M3")

	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("milestone sync deleted a file left by a sibling merge: %v", err)
	}
}

// The whole-feature caller (multi-feature mode) is unchanged: its worktree
// covered every milestone, so it still replaces the directory wholesale,
// including deleting stale files.
func TestWholeFeatureSyncStillReplacesWholesale(t *testing.T) {
	mainRoot, wt2, _ := seedWaveFixture(t, "calc-ops")

	stale := filepath.Join(mainRoot, ".belmont", "features", "calc-ops", "STALE.md")
	mustWrite(t, stale, "left over\n")

	syncFeatureStateAfterMerge(mainRoot, wt2, "calc-ops", "")

	if _, err := os.Stat(stale); err == nil {
		t.Errorf("whole-feature sync no longer removes stale files — behaviour changed")
	}
	got := readFeatureProgress(t, mainRoot, "calc-ops")
	if got != strings.Replace(waveForkProgress, "- [ ] P1-M2-1:", "- [v] P1-M2-1:", 1) {
		t.Errorf("whole-feature sync did not copy the worktree file verbatim:\n---\n%s", got)
	}
}

// Milestone names and (depends: …) annotations are structural and immutable
// outside /belmont:tech-plan, so the main repo's header line wins.
func TestResolveMilestoneProgressKeepsMasterHeader(t *testing.T) {
	worktree := strings.Replace(
		strings.Replace(waveForkProgress, "- [ ] P1-M2-1:", "- [v] P1-M2-1:", 1),
		"### M2: Divide (depends: M1)", "### M2: Divide and polish", 1)

	got := resolveMilestoneProgress(waveForkProgress, worktree, "M2")

	if !strings.Contains(got, "### M2: Divide (depends: M1)") {
		t.Errorf("worktree header edit overwrote the main repo's milestone header:\n---\n%s", got)
	}
	if strings.Contains(got, "Divide and polish") {
		t.Errorf("renamed header leaked through the merge:\n---\n%s", got)
	}
	if !strings.Contains(got, "- [v] P1-M2-1:") {
		t.Errorf("task mark did not come across:\n---\n%s", got)
	}
}

// If the worktree somehow has no block for its own milestone, the main repo's
// accumulated state must not be erased.
func TestResolveMilestoneProgressWorktreeMissingBlock(t *testing.T) {
	master := strings.Replace(waveForkProgress, "- [ ] P1-M2-1:", "- [v] P1-M2-1:", 1)
	worktree := "# Progress: calc-ops\n\n## Milestones\n"

	if got := resolveMilestoneProgress(master, worktree, "M2"); got != master {
		t.Errorf("main repo state was replaced by a worktree that lost the milestone:\n---\n%s", got)
	}
}

// If the main repo has no block for the milestone there is nothing to preserve,
// so the worktree's file is taken whole.
func TestResolveMilestoneProgressMasterMissingBlock(t *testing.T) {
	master := "# Progress: calc-ops\n\n## Milestones\n"
	worktree := strings.Replace(waveForkProgress, "- [ ] P1-M2-1:", "- [v] P1-M2-1:", 1)

	if got := resolveMilestoneProgress(master, worktree, "M2"); got != worktree {
		t.Errorf("expected fallback to the worktree file when the main repo has no such block:\n---\n%s", got)
	}
}

// The last milestone's block runs to EOF — splicing it must not truncate or
// duplicate trailing content.
func TestResolveMilestoneProgressLastBlockRunsToEOF(t *testing.T) {
	master := "## Milestones\n\n### M1: One\n- [v] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n"
	worktree := "## Milestones\n\n### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [v] P1-M2-1: b\n"

	got := resolveMilestoneProgress(master, worktree, "M2")
	want := "## Milestones\n\n### M1: One\n- [v] P1-M1-1: a\n\n### M2: Two\n- [v] P1-M2-1: b\n"
	if got != want {
		t.Errorf("EOF-terminated block spliced incorrectly:\ngot:\n%q\nwant:\n%q", got, want)
	}
}
