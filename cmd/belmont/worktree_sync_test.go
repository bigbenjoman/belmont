package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #24: syncFeatureStateAfterMerge was a destructive whole-directory
// replace, called once per sibling merge in a wave. Sibling worktrees each hold
// a full copy of the same feature dir from fork time, so the last merge reverted
// every earlier one's task state. The flips live in no commit (`.belmont/` is
// assume-unchanged in worktrees), so they were unrecoverable.
//
// Issue #29: copyBelmontStateToWorktree sat outside `if !resumed` in
// runMilestoneInWorktree, so resuming a paused worktree overwrote its live
// PROGRESS.md with master's fork-time snapshot.

func writeFeature(t *testing.T, root, slug, progress string) string {
	t.Helper()
	dir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "PROGRESS.md")
	if err := os.WriteFile(p, []byte(progress), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const waveProgress = `# Progress: Demo

## Milestones

### M1: Base
- [v] P1-M1-1: base

### M2: Divide (depends: M1)
- [%s] P1-M2-1: divide
- [%s] P1-M2-2: divide tests

### M3: Modulo (depends: M1)
- [%s] P1-M3-1: modulo
- [%s] P1-M3-2: modulo tests
`

func waveDoc(m2a, m2b, m3a, m3b string) string {
	return strings.NewReplacer("[%s]", "").Replace("") + // keep vet quiet about the const
		strings.Replace(
			strings.Replace(
				strings.Replace(
					strings.Replace(waveProgress, "%s", m2a, 1), "%s", m2b, 1),
				"%s", m3a, 1), "%s", m3b, 1)
}

// THE issue #24 regression: two sibling worktrees merging in sequence.
func TestSyncPreservesSiblingCompletions(t *testing.T) {
	master := t.TempDir()
	wtM2 := t.TempDir()
	wtM3 := t.TempDir()

	// All three forked with nothing done in M2/M3.
	masterProgress := writeFeature(t, master, "demo", waveDoc(" ", " ", " ", " "))
	// M2's agent completed its own milestone; M3's copy is still fork-time.
	writeFeature(t, wtM2, "demo", waveDoc("x", "x", " ", " "))
	writeFeature(t, wtM3, "demo", waveDoc(" ", " ", "x", "x"))

	// The wave's merge loop: M2 merges and syncs, then M3.
	syncFeatureStateAfterMerge(master, wtM2, "demo")
	afterM2 := readFile(t, masterProgress)
	if !strings.Contains(afterM2, "- [x] P1-M2-1") {
		t.Fatalf("after M2's sync, master should record M2 done:\n%s", afterM2)
	}

	syncFeatureStateAfterMerge(master, wtM3, "demo")
	got := readFile(t, masterProgress)

	for _, want := range []string{
		"- [x] P1-M2-1", "- [x] P1-M2-2", // M2's work must survive M3's sync
		"- [x] P1-M3-1", "- [x] P1-M3-2", // M3's work must arrive
		"- [v] P1-M1-1", // untouched milestone preserved
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q after the second sibling merge — a completion was lost:\n%s", want, got)
		}
	}
}

// A stale worktree must never drag a task backwards.
func TestSyncNeverRegressesAState(t *testing.T) {
	master := t.TempDir()
	wt := t.TempDir()
	p := writeFeature(t, master, "demo", waveDoc("v", "v", " ", " "))
	writeFeature(t, wt, "demo", waveDoc(" ", "x", "x", " "))

	syncFeatureStateAfterMerge(master, wt, "demo")
	got := readFile(t, p)

	if !strings.Contains(got, "- [v] P1-M2-1") || !strings.Contains(got, "- [v] P1-M2-2") {
		t.Errorf("master's verified tasks were regressed by a staler worktree:\n%s", got)
	}
	if !strings.Contains(got, "- [x] P1-M3-1") {
		t.Errorf("the worktree's own advance did not arrive:\n%s", got)
	}
}

// [!] is a human signal and must survive from EITHER side. Rank alone would
// clear it — taskBlocked sorts below todo, so anything outranks it.
func TestSyncPreservesBlockedMarkers(t *testing.T) {
	t.Run("blocked on main survives a worktree that marked it done", func(t *testing.T) {
		master, wt := t.TempDir(), t.TempDir()
		p := writeFeature(t, master, "demo", waveDoc("!", " ", " ", " "))
		writeFeature(t, wt, "demo", waveDoc("x", " ", "x", " "))

		syncFeatureStateAfterMerge(master, wt, "demo")
		got := readFile(t, p)
		if !strings.Contains(got, "- [!] P1-M2-1") {
			t.Errorf("a [!] blocker on main was silently cleared by the worktree:\n%s", got)
		}
		if !strings.Contains(got, "- [x] P1-M3-1") {
			t.Errorf("the worktree's unrelated advance was lost:\n%s", got)
		}
	})

	t.Run("blocked in the worktree survives a main that marked it done", func(t *testing.T) {
		master, wt := t.TempDir(), t.TempDir()
		p := writeFeature(t, master, "demo", waveDoc("x", " ", " ", " "))
		writeFeature(t, wt, "demo", waveDoc("!", " ", " ", " "))

		syncFeatureStateAfterMerge(master, wt, "demo")
		if got := readFile(t, p); !strings.Contains(got, "- [!] P1-M2-1") {
			t.Errorf("a blocker raised in the worktree was dropped on sync:\n%s", got)
		}
	})
}

// An unrecognised marker must be left exactly as written, never ranked. (#27)
func TestSyncLeavesUnrecognisedMarkersAlone(t *testing.T) {
	got, warnings := mergeProgressState(
		"### M2: M\n- [?] P1-M2-1: withdrawn\n",
		"### M2: M\n- [x] P1-M2-1: withdrawn\n",
	)
	if !strings.Contains(got, "- [x] P1-M2-1") {
		t.Errorf("worktree's own line should stand when master's marker is unreadable:\n%s", got)
	}
	if len(warnings) == 0 {
		t.Error("an unrecognised marker on one side should be reported, not merged silently")
	}
}

// A follow-up a sibling added after this worktree forked must not be dropped.
func TestSyncCarriesOverMasterOnlyTasks(t *testing.T) {
	got, _ := mergeProgressState(
		"### M2: M\n- [x] P1-M2-1: a\n- [ ] P1-M2-FIX-1: added by a sibling\n\n### M3: M\n- [ ] P1-M3-1: c\n",
		"### M2: M\n- [ ] P1-M2-1: a\n\n### M3: M\n- [x] P1-M3-1: c\n",
	)
	if !strings.Contains(got, "P1-M2-FIX-1") {
		t.Errorf("a task present only on main was dropped:\n%s", got)
	}
	if !strings.Contains(got, "- [x] P1-M3-1") {
		t.Errorf("the worktree's own advance was lost:\n%s", got)
	}
	// The carried-over line must land under its own milestone, not at the end.
	m2 := strings.Index(got, "### M2")
	m3 := strings.Index(got, "### M3")
	fix := strings.Index(got, "P1-M2-FIX-1")
	if !(m2 < fix && fix < m3) {
		t.Errorf("carried-over task landed outside M2's block:\n%s", got)
	}
}

// Issue #29: createWorktreeIfNeeded must be a no-op on resume. Both
// runFeatureInWorktree and runMilestoneInWorktree route through it, so this
// pins the guard for both — they used to inline it and drifted apart.
func TestCreateWorktreeIfNeededResumeIsNoOp(t *testing.T) {
	root, wtPath, wtProgress := resumeFixture(t)

	// The agent completed M2 in the worktree, then the run paused.
	os.WriteFile(wtProgress, []byte(waveDoc("x", "x", " ", " ")), 0644)

	if err := createWorktreeIfNeeded(root, wtPath, "belmont/m2", "demo", true); err != nil {
		t.Fatalf("resume path returned an error: %v", err)
	}

	got := readFile(t, wtProgress)
	if !strings.Contains(got, "- [x] P1-M2-1") || !strings.Contains(got, "- [x] P1-M2-2") {
		t.Errorf("resuming wiped the worktree's completed tasks with master's fork-time snapshot:\n%s", got)
	}
}

// The control: the hazard is real, so the guard is load-bearing. Seeding a
// worktree does overwrite PROGRESS.md — which is correct on a FRESH worktree
// and catastrophic on a resumed one.
func TestCreateWorktreeIfNeededSeedsFreshWorktree(t *testing.T) {
	root, wtPath, wtProgress := resumeFixture(t)
	os.WriteFile(wtProgress, []byte(waveDoc("x", "x", " ", " ")), 0644)

	// resumed=false on an existing dir would fail at `git worktree add`, so
	// call the seeding step the same way the fresh path reaches it.
	if err := copyBelmontStateToWorktree(root, wtPath, "demo"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	// Asserted, not skipped. A t.Skip here made the test unfailable: correct
	// behaviour fell off the end having checked only "no error", and the
	// behaviour it was meant to detect took the Skip — both green. That left
	// TestCreateWorktreeIfNeededResumeIsNoOp with no counterweight, so a
	// createWorktreeIfNeeded that no-opped in BOTH directions passed both tests.
	if got := readFile(t, wtProgress); strings.Contains(got, "- [x] P1-M2-1") {
		t.Fatalf("seeding no longer replaces PROGRESS.md, so the resume guard in "+
			"createWorktreeIfNeeded is guarding nothing — re-derive #29 before trusting either test:\n%s", got)
	}
}

// resumeFixture builds a real repo with a real git worktree and returns
// (root, worktree path, worktree PROGRESS.md path).
func resumeFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	git := func(a ...string) {
		c := exec.Command("git", a...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t.t")
	git("config", "user.name", "t")
	writeFeature(t, root, "demo", waveDoc(" ", " ", " ", " "))
	git("add", "-A")
	git("commit", "-q", "-m", "base")

	wtPath := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "-q", "-b", "belmont/m2", wtPath, "HEAD")
	return root, wtPath, filepath.Join(wtPath, ".belmont", "features", "demo", "PROGRESS.md")
}

// Red-team follow-ups on the #24/#29 fixes.

// A duplicated task ID makes the ID->line match ambiguous. Collapsing them
// silently would merge the wrong line, so the merge must decline and say so.
func TestSyncRefusesToMergeDuplicateIDs(t *testing.T) {
	master := "### M2: M\n- [x] P1-M2-1: first\n- [x] P1-M2-1: duplicate id\n"
	wt := "### M2: M\n- [ ] P1-M2-1: first\n- [ ] P1-M2-1: duplicate id\n"
	got, warnings := mergeProgressState(master, wt)
	if strings.Contains(got, "- [x]") {
		t.Errorf("merged an ambiguous duplicate ID:\n%s", got)
	}
	if len(warnings) == 0 {
		t.Error("a duplicate task ID must be reported, not silently collapsed")
	}
}

// An unrecognised marker must survive even when the other side is [!]. The
// blocked rule used to run first and overwrite it.
func TestSyncBlockedRuleNeverOverwritesUnrecognised(t *testing.T) {
	got, warnings := mergeProgressState(
		"### M2: M\n- [!] P1-M2-1: blocked on main\n",
		"### M2: M\n- [?] P1-M2-1: withdrawn here\n",
	)
	if !strings.Contains(got, "- [?] P1-M2-1") {
		t.Errorf("an unrecognised marker was rewritten to [!]:\n%s", got)
	}
	if len(warnings) == 0 {
		t.Error("expected a warning about the unrecognised marker")
	}
}

// Resume must still re-arm --assume-unchanged: handleStaleWorktree can
// reattach a worktree with `git worktree add`, which builds a fresh index.
func TestResumeReArmsUntracking(t *testing.T) {
	root, wtPath, wtProgress := resumeFixture(t)

	// Simulate the reattach path: a fresh index with the bits cleared.
	clear := exec.Command("git", "update-index", "--no-assume-unchanged",
		filepath.Join(".belmont", "features", "demo", "PROGRESS.md"))
	clear.Dir = wtPath
	clear.Run()

	if err := createWorktreeIfNeeded(root, wtPath, "belmont/m2", "demo", true); err != nil {
		t.Fatalf("resume: %v", err)
	}

	ls := exec.Command("git", "ls-files", "-v", filepath.Join(".belmont", "features", "demo", "PROGRESS.md"))
	ls.Dir = wtPath
	out, err := ls.Output()
	if err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	if !strings.HasPrefix(string(out), "h ") {
		t.Errorf("assume-unchanged not re-armed on resume (ls-files -v = %q) — the worktree's "+
			".belmont edits are committable and would clobber sibling state at merge", strings.TrimSpace(string(out)))
	}
	// And the resume guarantee still holds: state was not overwritten.
	if _, err := os.Stat(wtProgress); err != nil {
		t.Fatalf("worktree PROGRESS.md missing: %v", err)
	}
}

// mergeProgressState was the one reader that did not honour isSectionBreak
// after #31, so it attributed a task past a `## ` break to the last milestone
// header it had seen and spliced master-only orphans into that milestone.
func TestSyncRespectsSectionBreaks(t *testing.T) {
	master := `### M1: Work
- [x] P1-M1-1: a

## Session History

- [ ] P1-M9-1: orphan on main, belongs to no milestone
`
	wt := `### M1: Work
- [ ] P1-M1-1: a

## Session History

- [ ] P1-M9-2: orphan here
`
	got, _ := mergeProgressState(master, wt)

	// The in-milestone task still merges normally.
	if !strings.Contains(got, "- [x] P1-M1-1") {
		t.Errorf("in-milestone state was not merged:\n%s", got)
	}
	// Master's orphan must NOT be adopted into M1.
	if strings.Contains(got, "P1-M9-1") {
		t.Errorf("an orphan past a section break was spliced into a milestone:\n%s", got)
	}
	// The worktree's own orphan is left exactly as written.
	if !strings.Contains(got, "- [ ] P1-M9-2: orphan here") {
		t.Errorf("the worktree's orphan was altered:\n%s", got)
	}
	// And the merged document must still agree with the parser about structure.
	ms := parseMilestones("# P\n\n## Milestones\n\n" + got)
	if len(ms) != 1 || len(ms[0].Tasks) != 1 {
		t.Errorf("merged output no longer parses to 1 milestone with 1 task: %+v", ms)
	}
}

// An indented heading inside a task body must not split the milestone here
// either — the merge has to see the same region the parser sees.
func TestSyncIndentedHeadingDoesNotSplitMilestone(t *testing.T) {
	master := "### M1: Work\n- [x] P1-M1-1: a\n\n  ## quoted in the body\n\n- [x] P1-M1-2: b\n"
	wt := "### M1: Work\n- [ ] P1-M1-1: a\n\n  ## quoted in the body\n\n- [ ] P1-M1-2: b\n"
	got, _ := mergeProgressState(master, wt)
	for _, want := range []string{"- [x] P1-M1-1", "- [x] P1-M1-2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q — a task after an indented heading was not merged:\n%s", want, got)
		}
	}
}

// A carried master-only task must land after the anchor task's indented body,
// not between the task and its own continuation lines — otherwise the body is
// re-attached to the inserted task.
func TestSyncCarryLandsAfterTaskBody(t *testing.T) {
	master := "### M1: Core\n- [ ] P1-M1-1: foo\n  - Done when: bar renders\n- [ ] P1-M1-2: added by a sibling\n"
	wt := "### M1: Core\n- [ ] P1-M1-1: foo\n  - Done when: bar renders\n"
	got, _ := mergeProgressState(master, wt)
	if !strings.Contains(got, "P1-M1-2") {
		t.Fatalf("the master-only task was dropped:\n%s", got)
	}
	body := strings.Index(got, "Done when: bar renders")
	carried := strings.Index(got, "P1-M1-2")
	if carried < body {
		t.Errorf("the carried task was spliced between P1-M1-1 and its body:\n%s", got)
	}
}

// A duplicated milestone heading — a genuine duplicate, or a session note
// written in header shape after the startup lint already ran — makes every
// anchor for that milestone ambiguous. The merge must decline it the way the
// runtime guards and repair do, not guess.
func TestSyncDeclinesDuplicateMilestoneHeading(t *testing.T) {
	master := "### M1: Core\n- [x] P1-M1-1: real\n- [ ] P1-M1-2: added by a sibling\n"
	wt := "### M1: Core\n\n## Session History\n\n### M1: retro notes\nprose about the retro\n"
	got, warnings := mergeProgressState(master, wt)
	// The carry must not land under the header-shaped note in Session History.
	if strings.Contains(got, "P1-M1-2") {
		t.Errorf("a task was carried into a milestone with a duplicated heading:\n%s", got)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "M1") && strings.Contains(w, "more than one heading") {
			found = true
		}
	}
	if !found {
		t.Errorf("a duplicated milestone heading must be warned about, got: %v", warnings)
	}
	// Declining must not corrupt the document.
	if got != wt {
		t.Errorf("declining the merge still altered the worktree document:\ngot:\n%s\nwant:\n%s", got, wt)
	}
}

// The decline covers the in-place rewrite too: a task-shaped line quoted under
// a header-shaped note must never be rewritten with master's state. The quoted
// ID here is unique on each side, so neither the dupIDs refusal nor the master
// walk's own dupMS skip is what protects it — only the worktree walk's decline.
func TestSyncDuplicateHeadingNeverRewritesQuotedLine(t *testing.T) {
	master := "### M2: Other\n- [x] P1-M2-1: real task\n"
	wt := "### M1: Core\n- [ ] P1-M1-1: core\n\n## Session History\n\n### M1: retro notes\n- [ ] P1-M2-1: quoted into the log\n"
	got, _ := mergeProgressState(master, wt)
	if !strings.Contains(got, "- [ ] P1-M2-1: quoted into the log") {
		t.Errorf("a line quoted under a header-shaped note was rewritten with master's state:\n%s", got)
	}
}
