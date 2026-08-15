package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Belmont has two functions that merge two copies of PROGRESS.md.
// `resolveProgressConflict` runs when git registers a conflict;
// `mergeProgressState` runs on every worktree merge whether or not one occurred.
// #46 made them agree about MARKERS. These tests are about PLACEMENT, where they
// still disagreed: `resolveProgressConflict` was taught to carry a task's body
// and to keep its nesting in PR #51, and `mergeProgressState` was not.
//
// Both defects are silent by construction. The file still parses, the task count
// is unchanged, and `belmont validate` reports clean — which is why the existing
// carry tests, which assert the marker survives, caught neither. And the loss is
// unrecoverable: `.belmont/` is `--assume-unchanged` inside a worktree, so this
// copy is the only transport home and anything it drops is in no commit.
// See issue #53, and #33 for the same body-stranding failure in `repair`.

// TestMergeCarryBringsTheTaskBodyWithItsBullet is defect one. A master-only task
// carried as its bullet alone strands its `**Verification**` / `**Evidence**`
// lines, which then re-attach to whatever task now precedes them.
func TestMergeCarryBringsTheTaskBodyWithItsBullet(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [v] P0-M1-1: Parse the header
- [x] P0-M1-2: Parse the body
  **Verification**: ran the parser against every fixture
  **Evidence**: commit deadbee
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	got, _ := mergeProgressState(master, worktree)

	if !strings.Contains(got, "P0-M1-2") {
		t.Fatalf("the carried task is missing entirely:\n%s", got)
	}
	for _, want := range []string{
		"**Verification**: ran the parser against every fixture",
		"**Evidence**: commit deadbee",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("carried task lost its body line %q — it exists in no commit anywhere.\nmerged:\n%s", want, got)
		}
	}

	// Stranding is not only about absence: a body that arrives detached from its
	// bullet re-attaches to the task above it, which is worse than losing it
	// because it then reads as evidence for work it does not describe.
	lines := strings.Split(got, "\n")
	bulletAt := -1
	for i, l := range lines {
		if strings.Contains(l, "P0-M1-2") {
			bulletAt = i
			break
		}
	}
	if bulletAt == -1 {
		t.Fatalf("no bullet line for the carried task:\n%s", got)
	}
	body := strings.Join(lines[bulletAt+1:], "\n")
	if !strings.Contains(body, "**Evidence**: commit deadbee") {
		t.Errorf("the carried task's evidence did not follow its own bullet — it landed under another task.\nmerged:\n%s", got)
	}
}

// TestMergeCarriedNestedTaskLandsUnderItsParent is defect two. Anchoring every
// carried task after the milestone's last task re-parents a nested one: its
// meaning is positional, so a sub-task of P0-M1-1 that arrives at the end of the
// milestone has silently become a sub-task of whatever now sits last.
func TestMergeCarriedNestedTaskLandsUnderItsParent(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
  - [ ] P0-M1-1a: Handle the BOM
- [x] P0-M1-2: Parse the body
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
- [x] P0-M1-2: Parse the body
`
	got, _ := mergeProgressState(master, worktree)

	lines := strings.Split(got, "\n")
	idx := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	child, parent, sibling := idx("P0-M1-1a"), idx("P0-M1-1:"), idx("P0-M1-2")
	if child == -1 {
		t.Fatalf("the nested task was not carried at all:\n%s", got)
	}
	if !(parent < child && child < sibling) {
		t.Errorf("carried nested task was re-parented: it must sit between its parent P0-M1-1 (line %d) "+
			"and the next top-level task P0-M1-2 (line %d), but landed at line %d.\nmerged:\n%s",
			parent, sibling, child, got)
	}
	if lineIndentWidth(lines[child]) <= lineIndentWidth(lines[parent]) {
		t.Errorf("carried nested task lost its indent, so it is no longer nested at all.\nmerged:\n%s", got)
	}
}

// TestMergeCarriedChildStaysWithItsParentWhenAnchorsCollide pins the collision
// PR #51 had to fix next door. When the parent is the LAST task in its milestone
// here, `taskBodyEnd(parent)` and the milestone anchor are the same index, so a
// carried child and a brand-new top-level task land in the same bucket. Emitted
// in document order the top-level task goes first and takes the child with it —
// exactly the re-parenting the parent anchor exists to prevent.
func TestMergeCarriedChildStaysWithItsParentWhenAnchorsCollide(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
  - [ ] P0-M1-1a: Handle the BOM
- [ ] P0-M1-9: A brand new top-level task
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	got, _ := mergeProgressState(master, worktree)

	lines := strings.Split(got, "\n")
	idx := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	child, parent, newTop := idx("P0-M1-1a"), idx("P0-M1-1:"), idx("P0-M1-9")
	if child == -1 || newTop == -1 {
		t.Fatalf("a carried task is missing:\n%s", got)
	}
	if !(parent < child && child < newTop) {
		t.Errorf("the carried child must stay with its parent, before the new top-level task.\n"+
			"parent=%d child=%d newTop=%d\nmerged:\n%s", parent, child, newTop, got)
	}
}

// TestMergeDoesNotCarryANestedTaskTwice pins that a child travelling inside its
// carried parent's block is not ALSO spliced in its own right. A duplicated ID is
// durable damage rather than cosmetic: countIDs refuses to reconcile that task on
// every later merge and tells the user to de-duplicate by hand.
func TestMergeDoesNotCarryANestedTaskTwice(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
- [x] P0-M1-2: Parse the body
  - [ ] P0-M1-2a: Handle continuations
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	got, _ := mergeProgressState(master, worktree)

	if n := strings.Count(got, "P0-M1-2a"); n != 1 {
		t.Errorf("nested task appears %d times, want 1 — it travelled inside its parent's block AND was spliced separately.\nmerged:\n%s", n, got)
	}
	if n := strings.Count(got, "P0-M1-2:"); n != 1 {
		t.Errorf("parent task appears %d times, want 1.\nmerged:\n%s", n, got)
	}
}

// TestMergeCarryExcisesANestedTaskThisSideAlreadyHas covers the other way a
// carried block can duplicate an ID: the block's own ID is missing here, but a
// bullet nested INSIDE it already exists on this side in its own right. Only the
// block's own ID was ever checked, so the nested one rode in a second time.
//
// The remedy is to excise it rather than to skip the whole carry. The nested
// task's state is already reconciled in place by the marker walk, so removing it
// from the block loses nothing — whereas dropping the parent would lose work
// that exists in no commit anywhere.
func TestMergeCarryExcisesANestedTaskThisSideAlreadyHas(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
- [x] P0-M1-3: Parse the trailer
  **Evidence**: commit ccc333
  - [ ] P0-M1-2: Parse the body
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
- [>] P0-M1-2: Parse the body
`
	got, warnings := mergeProgressState(master, worktree)

	if n := strings.Count(got, "P0-M1-2"); n != 1 {
		t.Errorf("P0-M1-2 appears %d times, want 1 — the nested copy rode in inside its parent's block.\nmerged:\n%s", n, got)
	}
	if !strings.Contains(got, "P0-M1-3") {
		t.Errorf("the carried parent was dropped entirely — that is state loss, not de-duplication.\nmerged:\n%s", got)
	}
	if !strings.Contains(got, "**Evidence**: commit ccc333") {
		t.Errorf("the carried parent lost its own body while its nested bullet was excised.\nmerged:\n%s", got)
	}
	// Excising a line is a decision, and this function warns about every other
	// state it declines to merge.
	var mentioned bool
	for _, w := range warnings {
		if strings.Contains(w, "P0-M1-2") {
			mentioned = true
		}
	}
	if !mentioned {
		t.Errorf("nothing warned that P0-M1-2 was excised from the carried block; warnings were %v", warnings)
	}
}

// TestMergeCarryStillPlacesATopLevelTaskAtTheMilestoneEnd is the guard against
// over-correcting. A task with no parent must still land at the end of its
// milestone's block, past the last task's own body — not immediately after the
// milestone header, and not inside the preceding task.
func TestMergeCarryStillPlacesATopLevelTaskAtTheMilestoneEnd(t *testing.T) {
	master := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
- [ ] P0-M1-2: Parse the body

## Decisions Log
`
	worktree := `# Progress: demo

## Milestones

### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111

## Decisions Log
`
	got, _ := mergeProgressState(master, worktree)

	lines := strings.Split(got, "\n")
	idx := func(needle string) int {
		for i, l := range lines {
			if strings.Contains(l, needle) {
				return i
			}
		}
		return -1
	}
	carried, evidence, decisions := idx("P0-M1-2"), idx("**Evidence**: commit aaa111"), idx("## Decisions Log")
	if carried == -1 {
		t.Fatalf("top-level task was not carried:\n%s", got)
	}
	if carried < evidence {
		t.Errorf("carried task landed between P0-M1-1 and its own evidence body.\nmerged:\n%s", got)
	}
	if carried > decisions {
		t.Errorf("carried task landed outside the milestones region, past `## Decisions Log`.\nmerged:\n%s", got)
	}
	if lineIndentWidth(lines[carried]) != 0 {
		t.Errorf("carried top-level task was indented, making it a child of P0-M1-1.\nmerged:\n%s", got)
	}
}

// TestBothMergePathsPlaceACarriedTaskTheSameWay is the placement counterpart of
// TestBothMergePathsAgreeOnEveryMarkerPair. #46 made the two functions agree
// about markers and that test pins it; they still disagreed about WHERE a
// carried task lands, and nothing pinned that — which is issue #53.
//
// Which function runs depends only on whether git happened to register a
// conflict, so a disagreement here means the same wave produces a different
// document for reasons no user can see or predict.
func TestBothMergePathsPlaceACarriedTaskTheSameWay(t *testing.T) {
	base := `### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	// "ours" is the receiving side in both functions: the worktree copy for
	// mergeProgressState, the current branch for resolveProgressConflict.
	ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
- [x] P0-M1-2: Parse the body
`
	// "theirs" is the side carried FROM: master for mergeProgressState, the
	// incoming branch for resolveProgressConflict. It holds a nested task and a
	// top-level one that the receiving side has never seen.
	theirs := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
  - [ ] P0-M1-1a: Handle the BOM
    **Verification**: byte-order mark stripped before parse
- [x] P0-M1-2: Parse the body
- [ ] P0-M1-9: Emit a summary
`
	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("the conflict resolver declined a file of legal markers")
	}
	conflictOut := readFile(t, filepath.Join(root, rel))
	syncOut, _ := mergeProgressState(theirs, ours)

	// Compared as the sequence of task IDs in document order plus each one's
	// indent, which is exactly what "placement" means here and ignores the
	// incidental differences between the two paths (conflict markers stripped,
	// surrounding prose).
	shape := func(doc string) []string {
		var out []string
		for _, l := range strings.Split(doc, "\n") {
			if _, id, ok := mergeTaskLine(l); ok {
				out = append(out, id+"@"+itoa(lineIndentWidth(l)))
			}
		}
		return out
	}
	got, want := shape(syncOut), shape(conflictOut)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the two merge paths placed the carried tasks differently.\n"+
			"mergeProgressState:      %v\nresolveProgressConflict: %v\n\nmergeProgressState output:\n%s\nresolveProgressConflict output:\n%s",
			got, want, syncOut, conflictOut)
	}

	// And both must actually have nested it — two paths agreeing on the WRONG
	// placement would satisfy the comparison above while reproducing the bug.
	for name, doc := range map[string]string{"mergeProgressState": syncOut, "resolveProgressConflict": conflictOut} {
		lines := strings.Split(doc, "\n")
		child, parent := -1, -1
		for i, l := range lines {
			if strings.Contains(l, "P0-M1-1a") {
				child = i
			}
			if strings.Contains(l, "P0-M1-1:") && parent == -1 {
				parent = i
			}
		}
		if child == -1 {
			t.Errorf("%s: the nested task was not carried at all:\n%s", name, doc)
			continue
		}
		if lineIndentWidth(lines[child]) <= lineIndentWidth(lines[parent]) {
			t.Errorf("%s: the carried task is no longer nested under its parent:\n%s", name, doc)
		}
		if !strings.Contains(doc, "byte-order mark stripped before parse") {
			t.Errorf("%s: the carried task lost its own body:\n%s", name, doc)
		}
	}
}
