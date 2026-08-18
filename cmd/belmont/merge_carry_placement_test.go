package main

import (
	"path/filepath"
	"regexp"
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

// namesTask reports whether any warning mentions the task ID as a whole token.
//
// Bare `strings.Contains` matches a PREFIX: `namesTask(warnings, "P0-M2-1")` is
// satisfied by a warning that only ever mentions `P0-M2-1a` or `P0-M2-10`. Both
// fixtures here use hand-written sequential IDs, so a tenth task or an
// `a`-suffixed child would quietly make an assertion unfalsifiable — the same
// substring class as the `--all` / `--allow-dirty` hole this branch already fixed
// twice.
func namesTask(warnings []string, id string) bool {
	delimited := regexp.MustCompile(regexp.QuoteMeta(id) + `([^A-Za-z0-9_-]|$)`)
	for _, w := range warnings {
		if delimited.MatchString(w) {
			return true
		}
	}
	return false
}

// When a carry fails because the milestone is absent from this worktree's copy,
// EVERY task lost with it has to be named. The children used to be swallowed by
// the "travels inside its parent's block" skip, which ran before the anchor
// lookup and so `continue`d past the warning: main named all three, this
// function named one. The tasks are lost either way — that is the milestone
// being absent — but `.belmont/` is `--assume-unchanged` inside a worktree, so
// they exist in no commit and the warning is the only surviving record.
func TestFailedCarryNamesEveryTaskItLoses(t *testing.T) {
	master := `### M1: Parsing
- [x] P0-M1-1: Parse the header

### M2: Emitting
- [ ] P0-M2-1: Parent
  - [ ] P0-M2-2: Child
    - [ ] P0-M2-3: Grandchild
`
	worktree := `### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	_, warnings := mergeProgressState(master, worktree)
	for _, id := range []string{"P0-M2-1", "P0-M2-2", "P0-M2-3"} {
		if !namesTask(warnings, id) {
			t.Errorf("%s was dropped without any warning naming it — it is in no commit, so nothing else records it.\nwarnings: %v", id, warnings)
		}
	}
}

// A block written nested on main, landing on the MILESTONE anchor because its
// parent could not be resolved here, must be flattened to top level. Carried
// with main's indentation intact it becomes a child of whichever task sits last
// in that milestone — the re-parenting #53 is about, reached from the other
// side. Two inputs get here; both are covered.
func TestFallbackToMilestoneAnchorFlattensTheBlock(t *testing.T) {
	// (a) The parent is filed under a different milestone on each side.
	t.Run("parent under a different milestone here", func(t *testing.T) {
		master := `### M1: Parsing
- [x] P0-M1-1: Parent
  **Evidence**: p
  - [ ] P0-M1-1a: NewChild
`
		worktree := `### M1: Parsing
- [x] P0-M1-7: Something else
  **Evidence**: s

### M2: Emitting
- [x] P0-M1-1: Parent
  **Evidence**: p
`
		out, warnings := mergeProgressState(master, worktree)
		assertCarriedAtTopLevel(t, out, "P0-M1-1a", "P0-M1-7")
		if !namesTask(warnings, "P0-M1-1a") {
			t.Errorf("the fallback was silent about placement: %v", warnings)
		}
		// The wording itself, not just the ID. The pre-fix message said "carried
		// to the end of M1 instead of under its parent", which reads as top level
		// and was not what came out — the whole point of the fix. Nothing pinned
		// that, so reverting the wording alone left the suite green.
		if !warningSaying(warnings, "at top level") {
			t.Errorf("the warning does not say where the task actually went: %v", warnings)
		}
	})

	// (b) The parent's ID is duplicated on main, so it never enters masterOrder:
	// neither missingHere nor wtInRegionID is true of it, and the only warning
	// raised is the unrelated ambiguous-ID one.
	t.Run("parent ID duplicated on main", func(t *testing.T) {
		master := `### M1: Parsing
- [x] P0-M1-1: Parent
  - [ ] P0-M1-1a: NewChild
- [x] P0-M1-1: Parent again
`
		worktree := `### M1: Parsing
- [x] P0-M1-7: Something else
  **Evidence**: s
`
		out, warnings := mergeProgressState(master, worktree)
		assertCarriedAtTopLevel(t, out, "P0-M1-1a", "P0-M1-7")
		if !namesTask(warnings, "P0-M1-1a") {
			t.Errorf("the carried task was re-placed with no warning naming it: %v", warnings)
		}
		if !warningSaying(warnings, "at top level") {
			t.Errorf("the warning does not say where the task actually went: %v", warnings)
		}
	})
}

// assertCarriedAtTopLevel checks that `carried` was emitted at the same indent
// as `sibling` — i.e. as its peer, not as its child — AND under the same
// milestone heading.
//
// The milestone half is not decoration. Asserting indent alone passed a mutation
// that re-anchored the carry into the milestone THIS side files the parent
// under: both lines sit at indent 0, in different milestones, so the helper saw
// nothing. That outcome is a task whose ID names M1 filed under M2, which
// `belmont validate` rates `cross_milestone_task_id`, severityError — and the
// code comment at the fallback says in as many words that it must not relocate
// work across milestones on a guess. A helper that certifies that as correct is
// worse than no helper.
func assertCarriedAtTopLevel(t *testing.T, doc, carried, sibling string) {
	t.Helper()
	lines := strings.Split(doc, "\n")
	ci, si := -1, -1
	for i, l := range lines {
		if strings.Contains(l, carried) {
			ci = i
		}
		if strings.Contains(l, sibling) && si == -1 {
			si = i
		}
	}
	if ci == -1 {
		t.Fatalf("%s was not carried at all:\n%s", carried, doc)
	}
	if si == -1 {
		t.Fatalf("%s is missing from the output:\n%s", sibling, doc)
	}
	if lineIndentWidth(lines[ci]) > lineIndentWidth(lines[si]) {
		t.Errorf("%s landed nested under %s, a task it has nothing to do with:\n%s", carried, sibling, doc)
	}
	if cm, sm := enclosingMilestone(lines, ci), enclosingMilestone(lines, si); cm != sm {
		t.Errorf("%s was relocated into %s, but %s sits under %s — a task filed under a milestone its ID does not name is cross_milestone_task_id, severityError:\n%s",
			carried, nonEmpty(cm, "no milestone"), sibling, nonEmpty(sm, "no milestone"), doc)
	}
}

// enclosingMilestone returns the ID of the `### M<n>:` heading `idx` sits under,
// or "" when it sits outside every milestone.
func enclosingMilestone(lines []string, idx int) string {
	for i := idx; i >= 0; i-- {
		if m := msHeaderRe.FindStringSubmatch(lines[i]); len(m) >= 2 {
			return "M" + m[1]
		}
		if i != idx && isSectionBreak(lines[i]) {
			return ""
		}
	}
	return ""
}

// Issue #60. `TestBothMergePathsPlaceACarriedTaskTheSameWay` compares `id@indent`
// per task, so it is blind to WHERE a carried task sits relative to the non-task
// lines around it — and that is exactly where the two paths used to disagree.
// The copy path anchored on `taskBodyEnd` of the last task; the conflict path
// anchored on the last non-blank line of the milestone's block. Identical for
// every milestone that ends on its last task's body, and different the moment
// one ends with prose: the carried task landed above the closing note on one
// path and below it on the other, decided only by whether git registered a
// conflict.
//
// Both now call `milestoneCarryAnchor`.
func TestBothMergePathsAnchorPastAMilestonesClosingProse(t *testing.T) {
	base := `### M1: Parsing
- [x] P0-M1-1: Parse the header

Closing note for M1, at column zero.
`
	ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111

Closing note for M1, at column zero.
`
	theirs := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
- [ ] P0-M1-9: Emit a summary

Closing note for M1, at column zero.
`
	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("the conflict resolver declined a file of legal markers")
	}
	conflictOut := readFile(t, filepath.Join(root, rel))
	syncOut, _ := mergeProgressState(theirs, ours)

	// Placement is measured against the closing note, not against the other
	// tasks — comparing task IDs alone is what let this through.
	place := func(doc string) string {
		lines := strings.Split(doc, "\n")
		task, note := -1, -1
		for i, l := range lines {
			if strings.Contains(l, "P0-M1-9") {
				task = i
			}
			if strings.HasPrefix(l, "Closing note") && note == -1 {
				note = i
			}
		}
		switch {
		case task == -1:
			return "the carried task is absent"
		case note == -1:
			return "the closing note is absent"
		case task < note:
			return "carried task above the closing note"
		default:
			return "carried task below the closing note"
		}
	}
	got, want := place(syncOut), place(conflictOut)
	if got != want {
		t.Errorf("the two merge paths disagree about placement, and only git decides which one runs.\n"+
			"mergeProgressState:      %s\nresolveProgressConflict: %s\n\ncopy path:\n%s\nconflict path:\n%s",
			got, want, syncOut, conflictOut)
	}
	// And both must keep it with the tasks rather than stranding it below the
	// prose — agreeing on the wrong answer would satisfy the comparison above.
	if got != "carried task above the closing note" {
		t.Errorf("both paths stranded the carried task below the milestone's closing prose: %s", got)
	}
}

// Found by red-teaming the #60 fix. `milestoneCarryAnchor` first took
// `taskBodyEnd` of the milestone's LAST task bullet. When that bullet is a
// nested child, `taskBodyEnd` bounds the body by the child's own indent and so
// returns the child's line — the splice lands INSIDE the parent's body, above
// the parent's `**Evidence**`, which then reads as the carried task's evidence.
// That is #33's stranding produced by the function written to prevent it.
//
// The anchor is the furthest `taskBodyEnd` over every bullet in the milestone,
// so the enclosing parent's body end wins.
func TestCarryLandsBelowAParentsEvidenceWhenTheLastBulletIsNested(t *testing.T) {
	ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  - [x] P0-M1-2: Handle the BOM
  **Evidence**: commit aaa111 proves P0-M1-1
`
	theirs := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  - [x] P0-M1-2: Handle the BOM
  **Evidence**: commit aaa111 proves P0-M1-1
- [ ] P0-M1-9: Emit a summary
`
	base := `### M1: Parsing
- [x] P0-M1-1: Parse the header
`
	assertEvidenceStaysWithItsParent := func(t *testing.T, name, doc string) {
		t.Helper()
		lines := strings.Split(doc, "\n")
		carried, evidence := -1, -1
		for i, l := range lines {
			if strings.Contains(l, "P0-M1-9") {
				carried = i
			}
			if strings.Contains(l, "**Evidence**") && evidence == -1 {
				evidence = i
			}
		}
		if carried == -1 {
			t.Fatalf("%s: the carried task is missing:\n%s", name, doc)
		}
		if evidence == -1 {
			t.Fatalf("%s: the evidence line is missing:\n%s", name, doc)
		}
		if carried < evidence {
			t.Errorf("%s: carried task spliced ABOVE P0-M1-1's own **Evidence**, which now reads as the carried task's:\n%s", name, doc)
		}
	}

	syncOut, _ := mergeProgressState(theirs, ours)
	assertEvidenceStaysWithItsParent(t, "mergeProgressState", syncOut)

	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("the conflict resolver declined a file of legal markers")
	}
	assertEvidenceStaysWithItsParent(t, "resolveProgressConflict", readFile(t, filepath.Join(root, rel)))
}

// Also from that round. The anchor recorded bullets via `mergeTaskLine`, which
// demands a parseable ID — so a milestone whose tasks are all hand-written and
// ID-less recorded none, fell through to the header fallback, and the carried
// task was spliced ABOVE every task already there. `anyTaskBullet` records the
// bullet regardless of whether anything can name it: an ID-less task still
// bounds where the milestone's task list ends.
func TestCarryGoesBelowIDLessTasksRatherThanAboveThem(t *testing.T) {
	ours := `### M1: Parsing
- [x] Parse the header
- [x] Parse the body
`
	theirs := `### M1: Parsing
- [x] Parse the header
- [x] Parse the body
- [ ] P0-M1-9: Emit a summary
`
	out, _ := mergeProgressState(theirs, ours)
	lines := strings.Split(out, "\n")
	carried, lastExisting := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "P0-M1-9") {
			carried = i
		}
		if strings.Contains(l, "Parse the body") {
			lastExisting = i
		}
	}
	if carried == -1 {
		t.Fatalf("the carried task is missing:\n%s", out)
	}
	if carried < lastExisting {
		t.Errorf("carried task landed above the milestone's existing tasks — the anchor fell through to the header:\n%s", out)
	}
}

// The dedent has to happen on BOTH paths. A bullet nested under a NON-task list
// item has `parent == ""` everywhere, so it reaches the milestone anchor on both
// — and flattening it on the copy path alone left the conflict path re-parenting
// it under whichever task sat last. Which function runs depends only on whether
// git registered a conflict, so a one-sided fix to a two-path invariant is just
// a new instance of the bug.
func TestBothPathsFlattenABulletNestedUnderANonTaskItem(t *testing.T) {
	base := `### M1: Parsing
- [ ] P0-M1-1: Parse the header
`
	ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
`
	theirs := `### M1: Parsing
- [ ] P0-M1-1: Parse the header
- Follow-ups raised in review:
  - [ ] P0-M1-9: Emit a summary
`
	syncOut, _ := mergeProgressState(theirs, ours)
	root, rel := conflictFixture(t, base, ours, theirs)
	if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
		t.Fatal("the conflict resolver declined a file of legal markers")
	}
	conflictOut := readFile(t, filepath.Join(root, rel))

	indentOf := func(doc, id string) int {
		for _, l := range strings.Split(doc, "\n") {
			if strings.Contains(l, id) {
				return lineIndentWidth(l)
			}
		}
		return -1
	}
	syncIndent, conflictIndent := indentOf(syncOut, "P0-M1-9"), indentOf(conflictOut, "P0-M1-9")
	if syncIndent != conflictIndent {
		t.Errorf("the two paths disagree about indentation, and only git decides which runs.\n"+
			"mergeProgressState: %d\nresolveProgressConflict: %d\n\ncopy path:\n%s\nconflict path:\n%s",
			syncIndent, conflictIndent, syncOut, conflictOut)
	}
	if syncIndent != 0 {
		t.Errorf("the carried task stayed nested, so it became a child of an unrelated task (indent %d):\n%s", syncIndent, syncOut)
	}
}

// warningSaying reports whether any warning contains the phrase.
func warningSaying(warnings []string, phrase string) bool {
	for _, w := range warnings {
		if strings.Contains(w, phrase) {
			return true
		}
	}
	return false
}

// `dedentBlock`'s documented contract is that internal structure survives: a
// line indented deeper than the block's own bullet stays deeper by the same
// amount, so a carried parent keeps its own children and its own body. Nothing
// tested that — the flatten subtests each carry a single childless, body-less
// bullet, so replacing the whole function with `strings.TrimLeft(l, " \t")`
// passed the entire suite while destroying exactly what the doc comment
// promises. The clamp was untested for the same reason.
func TestDedentBlockKeepsInternalStructure(t *testing.T) {
	block := []string{
		"  - [ ] P0-M1-1a: Parent",
		"    **Verification**: it works",
		"",
		"    - [ ] P0-M1-1b: Child",
		"      **Evidence**: commit bbb222",
	}
	got := dedentBlock(block, 2)
	want := []string{
		"- [ ] P0-M1-1a: Parent",
		"  **Verification**: it works",
		"",
		"  - [ ] P0-M1-1b: Child",
		"    **Evidence**: commit bbb222",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d\n got %q\nwant %q", i, got[i], want[i])
		}
	}
	// Relative nesting is the property, stated independently of the exact strings
	// so a reader sees what is actually being protected.
	if lineIndentWidth(got[3]) <= lineIndentWidth(got[0]) {
		t.Errorf("the carried parent lost its child: parent indent %d, child indent %d\n%v",
			lineIndentWidth(got[0]), lineIndentWidth(got[3]), got)
	}
	// The clamp: a blank line has no indent to give and must survive as a blank,
	// not as a fragment of anything else.
	if got[2] != "" {
		t.Errorf("the blank separator was mangled to %q", got[2])
	}
}

// The anchor must land the carry INSIDE the milestone's list, not past the blank
// line that ends it. `milestoneCarryAnchor` returning `taskBodyEnd(...) + 1`
// passed every other test here: both paths shift equally so the agreement
// assertions hold, and the task stays above a closing note so the prose
// assertion holds too. The placement it produces is the one the anchor's own
// history names as wrong.
func TestCarryLandsInsideTheListNotPastItsTrailingBlank(t *testing.T) {
	ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header

### M2: Emitting
- [x] P0-M2-1: Emit
`
	theirs := `### M1: Parsing
- [x] P0-M1-1: Parse the header
- [ ] P0-M1-9: Emit a summary

### M2: Emitting
- [x] P0-M2-1: Emit
`
	out, _ := mergeProgressState(theirs, ours)
	lines := strings.Split(out, "\n")
	carried := -1
	for i, l := range lines {
		if strings.Contains(l, "P0-M1-9") {
			carried = i
		}
	}
	if carried == -1 {
		t.Fatalf("the carried task is missing:\n%s", out)
	}
	if strings.TrimSpace(lines[carried-1]) == "" {
		t.Errorf("the carried task landed past the blank line that ends M1's list, detached from it:\n%s", out)
	}
	if ms := enclosingMilestone(lines, carried); ms != "M1" {
		t.Errorf("the carried task landed under %s, not M1:\n%s", nonEmpty(ms, "no milestone"), out)
	}
}

// A checkbox bullet is not always a task. Recording EVERY bullet as an anchor
// candidate fixed the ID-less-milestone bug and introduced two worse ones: a
// sample inside a fenced code block, and a reviewer checklist written under a
// milestone's closing prose, both bound the "task list" and dragged the carry
// past the real tasks. The first splices it INSIDE the fence.
//
// Fenced regions are skipped outright, and the anchor prefers ID-bearing bullets
// with the ID-less set as fallback — so the widening still covers the milestone
// it was added for, without letting prose speak for a milestone that has tasks.
func TestCarryIgnoresCheckboxesThatAreNotTasks(t *testing.T) {
	t.Run("sample inside a fenced code block", func(t *testing.T) {
		base := "## Milestones\n\n### M1: Parsing\n- [ ] P0-M1-1: Parse the header\n"
		ours := "## Milestones\n\n### M1: Parsing\n- [x] P0-M1-1: Parse the header\n\nFollow-ups are written as:\n\n```\n- [ ] describe the work here\n```\n"
		theirs := "## Milestones\n\n### M1: Parsing\n- [ ] P0-M1-1: Parse the header\n- [ ] P0-M1-9: Emit a summary\n\nFollow-ups are written as:\n\n```\n- [ ] describe the work here\n```\n"

		assertOutsideFence := func(t *testing.T, name, doc string) {
			t.Helper()
			lines := strings.Split(doc, "\n")
			open, close, carried := -1, -1, -1
			for i, l := range lines {
				if isFenceDelimiter(l) {
					if open == -1 {
						open = i
					} else if close == -1 {
						close = i
					}
				}
				if strings.Contains(l, "P0-M1-9") {
					carried = i
				}
			}
			if carried == -1 {
				t.Fatalf("%s: the carried task is missing:\n%s", name, doc)
			}
			if open != -1 && close != -1 && open < carried && carried < close {
				t.Errorf("%s: carried task spliced INSIDE the fenced code block:\n%s", name, doc)
			}
		}

		syncOut, _ := mergeProgressState(theirs, ours)
		assertOutsideFence(t, "mergeProgressState", syncOut)

		root, rel := conflictFixture(t, base, ours, theirs)
		if !resolveProgressConflict(root, rel, filepath.Join(root, rel)) {
			t.Fatal("the conflict resolver declined a file of legal markers")
		}
		assertOutsideFence(t, "resolveProgressConflict", readFile(t, filepath.Join(root, rel)))
	})

	t.Run("checklist in a milestone's closing prose", func(t *testing.T) {
		ours := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111

Reviewer sign-off checklist (not tasks):
- [ ] Ben has read the diff
- [ ] CI is green
`
		theirs := `### M1: Parsing
- [x] P0-M1-1: Parse the header
  **Evidence**: commit aaa111
- [ ] P0-M1-9: Emit a summary

Reviewer sign-off checklist (not tasks):
- [ ] Ben has read the diff
- [ ] CI is green
`
		out, _ := mergeProgressState(theirs, ours)
		lines := strings.Split(out, "\n")
		carried, lastProse := -1, -1
		for i, l := range lines {
			if strings.Contains(l, "P0-M1-9") {
				carried = i
			}
			if strings.Contains(l, "CI is green") {
				lastProse = i
			}
		}
		if carried == -1 {
			t.Fatalf("the carried task is missing:\n%s", out)
		}
		if carried > lastProse {
			t.Errorf("the carried task landed below the milestone's closing checklist, detached from the real tasks:\n%s", out)
		}
	})

	// Where fence-skipping is actually load-bearing. In the fixtures above the
	// milestone has an ID-bearing task, so the ID-preference alone keeps the
	// fence out of it and removing the fence skip changes nothing. Here the
	// milestone's own tasks are ID-less, so the fallback IS consulted — and
	// without the skip it picks the bullet inside the fence and splices the carry
	// after it, inside the code block.
	t.Run("fence is skipped even when the fallback is in play", func(t *testing.T) {
		ours := "### M1: Parsing\n- [x] Parse the header\n\nWrite follow-ups like:\n\n```\n- [ ] describe the work here\n```\n"
		theirs := "### M1: Parsing\n- [x] Parse the header\n- [ ] P0-M1-9: Emit a summary\n\nWrite follow-ups like:\n\n```\n- [ ] describe the work here\n```\n"
		out, _ := mergeProgressState(theirs, ours)
		lines := strings.Split(out, "\n")
		open, close, carried := -1, -1, -1
		for i, l := range lines {
			if isFenceDelimiter(l) {
				if open == -1 {
					open = i
				} else if close == -1 {
					close = i
				}
			}
			if strings.Contains(l, "P0-M1-9") {
				carried = i
			}
		}
		if carried == -1 {
			t.Fatalf("the carried task is missing:\n%s", out)
		}
		if open != -1 && carried > open {
			t.Errorf("carried task landed at or past the fence, anchored on a sample rather than on the milestone's tasks:\n%s", out)
		}
	})

	// The case the widening was added for still works: a milestone whose tasks
	// are all ID-less falls back to them rather than to the header.
	t.Run("ID-less milestone still anchors on its tasks", func(t *testing.T) {
		ours := "### M1: Parsing\n- [x] Parse the header\n- [x] Parse the body\n"
		theirs := "### M1: Parsing\n- [x] Parse the header\n- [x] Parse the body\n- [ ] P0-M1-9: Emit a summary\n"
		out, _ := mergeProgressState(theirs, ours)
		lines := strings.Split(out, "\n")
		carried, lastExisting := -1, -1
		for i, l := range lines {
			if strings.Contains(l, "P0-M1-9") {
				carried = i
			}
			if strings.Contains(l, "Parse the body") {
				lastExisting = i
			}
		}
		if carried < lastExisting {
			t.Errorf("the ID-less fallback regressed — carry landed above the existing tasks:\n%s", out)
		}
	})
}
