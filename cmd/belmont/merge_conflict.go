package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// classifyChanges runs git diff between preSHA and HEAD and classifies the work type.
func classifyChanges(root, preSHA string) (workType, int) {
	if preSHA == "" {
		return workUnknown, 0
	}
	cmd := exec.Command("git", "diff", "--name-only", preSHA+"..HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return workUnknown, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return workMinimal, 0
	}
	if len(files) < 3 {
		return workMinimal, len(files)
	}

	frontendExts := map[string]bool{
		".tsx": true, ".jsx": true, ".css": true, ".scss": true,
		".html": true, ".vue": true, ".svelte": true, ".less": true,
	}
	backendExts := map[string]bool{
		".go": true, ".py": true, ".rs": true, ".java": true,
		".rb": true, ".php": true, ".cs": true, ".kt": true,
		".scala": true, ".ex": true, ".exs": true,
	}
	configExts := map[string]bool{
		".yml": true, ".yaml": true, ".json": true, ".toml": true,
		".ini": true, ".env": true,
	}
	docExts := map[string]bool{
		".md": true, ".txt": true, ".rst": true,
	}

	var feCount, beCount, cfgCount, docCount int
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		switch {
		case frontendExts[ext]:
			feCount++
		case backendExts[ext]:
			beCount++
		case configExts[ext]:
			cfgCount++
		case docExts[ext]:
			docCount++
		}
	}

	total := len(files)
	if docCount == total {
		return workDocs, total
	}
	if cfgCount == total {
		return workConfig, total
	}
	if feCount*2 > total {
		return workFrontend, total
	}
	if beCount*2 > total {
		return workBackend, total
	}
	if feCount > 0 && beCount > 0 {
		return workMixed, total
	}
	if feCount > 0 {
		return workFrontend, total
	}
	if beCount > 0 {
		return workBackend, total
	}
	return workMixed, total
}

// activityHeadingRe matches the heading that opens PROGRESS.md's activity log.
//
// Anchored at column zero and required to be a level-2 ATX heading, for the same
// reason isSectionBreak is: an ordinary task line that merely MENTIONS
// "## Activity" is not a heading, and `###` is not a level-2 heading. Testing
// this with strings.Contains let a task open the section, and testing the end
// with HasPrefix(line, "##") let a milestone header close it.
var activityHeadingRe = regexp.MustCompile(`^##\s+(?:Recent Activity|Activity|Session History)\b`)

// resolveProgressConflict merges a conflicted PROGRESS.md by taking the most-advanced
// task state from both sides. State ordering: [v] > [x] > [>] > [ ], [!] preserved.
// Returns true if successfully resolved.
func resolveProgressConflict(root, relPath, filePath string) bool {
	// Get "ours" version
	oursCmd := exec.Command("git", "show", ":2:"+relPath)
	oursCmd.Dir = root
	oursOut, err := oursCmd.Output()
	if err != nil {
		return false
	}

	// Get "theirs" version
	theirsCmd := exec.Command("git", "show", ":3:"+relPath)
	theirsCmd.Dir = root
	theirsOut, err := theirsCmd.Output()
	if err != nil {
		return false
	}

	// State priority comes from markerRank (state.go), keyed by canonical
	// taskStatus rather than raw marker — a raw-marker map silently ranked
	// every spelling it lacked a key for at Go's zero value (0, tied with todo
	// and BELOW in-progress), so `[X]` (done) lost to `[>]` and an
	// unrecognised marker was overwritten and committed.
	rank := markerRank

	// Parse task states from "theirs".
	//
	// Scoped to the milestones region, because this map is last-occurrence-wins
	// and a task-shaped line under `## Session History` is not a task. Walking
	// every line let a history entry shadow the real one in both directions: a
	// `[v]` quoted in a session log MINTED a verified flip on the in-region task
	// — with no commit evidence, and `runEvidenceCheck` never sees a post-merge
	// file — while a stale `[ ]` quote hid theirs' genuine `[x]` and the
	// completion was dropped under a green "conflicts auto-resolved". Same
	// boundary every other reader uses; see isSectionBreak and issue #31.
	// The ID is `taskIDShape`, the same definition parseTaskID and
	// commitNamedTaskIDs use. It was `P\d+-…` only, so a hand-written ID was
	// matched on neither side and silently took whichever side git left —
	// see issue #38. A `P<n>-` ID behaves exactly as before; that alternative
	// is first in the alternation.
	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Identity comes from mergeTaskLine — the one definition both merge readers
	// share. See its comment for why the `:` delimiter is required of the
	// hand-written form only.

	theirsLines := strings.Split(string(theirsOut), "\n")
	oursLines := strings.Split(string(oursOut), "\n")

	// Refuse the whole file when a milestone ID heads more than one block, or a
	// task ID appears more than once inside the region, on either side.
	//
	// Both make every lookup below ambiguous: theirsStates is
	// last-occurrence-wins and the carry anchor maps are last-writer-wins. The input that
	// produces them is ordinary — a header-shaped session note. `### M2: verify
	// round notes` under `## Session History` re-opens the milestones region for
	// this scanner exactly as it does for parseMilestones, so a task line quoted
	// beneath it is read as a real task. A quoted `[v]` then mints a verified
	// flip on the real task with no commit evidence — and the comment at the top
	// of this function is why runEvidenceCheck never sees a post-merge file. The
	// note heading can also anchor the splice itself, landing a carried task
	// inside the history section.
	//
	// requireUnambiguousMilestones catches a duplicate that predates the run,
	// but a note written mid-run gets past it, and when it duplicates a real ID
	// the scope guard declines rather than reverting — so a file can reach merge
	// time in this shape. mergeProgressState already refuses this exact input
	// via dupMS/dupIDs; this side had no check at all on the walks, only in the
	// carry branch. Same policy this function applies to an unrecognised marker:
	// escalate, never guess.
	countMSHeads := func(lines []string) map[string]int {
		n := map[string]int{}
		for _, line := range lines {
			if m := msHeaderRe.FindStringSubmatch(line); m != nil {
				n["M"+m[1]]++
			}
		}
		return n
	}
	// Region-scoped, for the same reason countIDs is in mergeProgressState: a
	// task-shaped line under `## Session History` belongs to no milestone and is
	// never merged, so counting it would make the real in-region task look
	// ambiguous and refuse a file that is fine.
	countTaskIDs := func(lines []string) map[string]int {
		n := map[string]int{}
		inRegion := false
		for _, line := range lines {
			if msHeaderRe.MatchString(line) {
				inRegion = true
				continue
			}
			if isSectionBreak(line) {
				inRegion = false
				continue
			}
			if !inRegion {
				continue
			}
			if _, id, ok := mergeTaskLine(line); ok {
				n[id]++
			}
		}
		return n
	}
	// Sorted, so the reported ID does not vary between runs on identical input.
	firstDup := func(counts map[string]int) (string, int) {
		ids := make([]string, 0, len(counts))
		for id, n := range counts {
			if n > 1 {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return "", 0
		}
		sort.Strings(ids)
		return ids[0], counts[ids[0]]
	}
	for _, side := range []struct {
		name  string
		lines []string
	}{{"ours", oursLines}, {"theirs", theirsLines}} {
		if id, n := firstDup(countMSHeads(side.lines)); id != "" {
			fmt.Fprintf(os.Stderr,
				"  \033[33m⚠ %s: %s heads %d separate blocks on the %s side — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
				relPath, id, n, side.name)
			return false
		}
		if id, n := firstDup(countTaskIDs(side.lines)); id != "" {
			fmt.Fprintf(os.Stderr,
				"  \033[33m⚠ %s: task %s appears %d times inside the milestones region on the %s side — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
				relPath, id, n, side.name)
			return false
		}
	}

	theirsStates := make(map[string]string) // task ID → checkbox marker

	// Theirs' task lines in document order, with the milestone each one sits
	// in and the task it is nested under. Needed because a task present only in
	// "theirs" has to be carried into the merged file rather than dropped (issue
	// #43), and the block it belongs in is the one it was written in.
	//
	// EVERY task bullet is recorded, nested ones included. Recording only
	// top-level bullets made a nested task travel solely inside its parent's
	// block, so a nested bullet only "theirs" had was dropped outright whenever
	// the parent already existed here — the parent was not carried, and nothing
	// else was looking at the child. A nested task is a real task everywhere
	// else in Belmont: `parseMilestones` counts it, `taskBodyEnd` bounds it,
	// `moveTaskLines` moves it. See issue #47.
	type theirsTask struct {
		id        string
		block     []string
		milestone string
		parent    string // enclosing task's ID; "" at top level
	}
	var theirsOrder []theirsTask
	inRegion := false
	theirsMS := ""
	// Enclosing task bullets, innermost last, each held open until the line its
	// body ends on.
	//
	// The bound is `taskBodyEnd`'s and NOT an indent comparison, because the skip
	// rule in the carry below says a nested task "travels inside its parent's
	// block" — and the block is exactly `taskBodyEnd`'s range. An indent-only
	// stack disagreed with it: a body ends at the first non-blank line at the
	// task's own indent or shallower, so a column-zero note closes it, while an
	// indent stack kept the task open across that note. A deeper bullet written
	// after the note was then recorded as that task's child, skipped as travelling
	// inside a block that does not contain it, and dropped from the merged file —
	// silent state loss in the function whose purpose is to prevent it, and a
	// regression against the top-level-only carry it replaced.
	type openTask struct {
		end int
		id  string
	}
	var open []openTask
	for i, line := range theirsLines {
		if m := msHeaderRe.FindStringSubmatch(line); m != nil {
			inRegion = true
			theirsMS = "M" + m[1]
			open = open[:0]
			continue
		}
		if isSectionBreak(line) {
			inRegion = false
			theirsMS = ""
			open = open[:0]
			continue
		}
		if !inRegion {
			continue
		}
		if marker, id, ok := mergeTaskLine(line); ok {
			theirsStates[id] = marker
			for len(open) > 0 && i > open[len(open)-1].end {
				open = open[:len(open)-1]
			}
			parent := ""
			if len(open) > 0 {
				parent = open[len(open)-1].id
			}
			// The task is the bullet PLUS its indented body — carrying the
			// bullet alone strands its `**Verification**` / `**Evidence**`
			// lines on the side they came from, which is the #33 failure in
			// a different function. A nested bullet inside that body is
			// recorded in its own right too, and the carry below decides
			// which of the two travels so neither arrives twice.
			end := taskBodyEnd(theirsLines, i)
			theirsOrder = append(theirsOrder, theirsTask{
				id:        id,
				block:     append([]string{}, theirsLines[i:end+1]...),
				milestone: theirsMS,
				parent:    parent,
			})
			open = append(open, openTask{end: end, id: id})
		}
	}

	// Refuse to auto-resolve a file containing a marker we cannot read.
	//
	// This resolver rewrites checkbox lines in place. Applied to an
	// unrecognised marker it would normalise it away and commit the result
	// under a green "conflicts auto-resolved" — destroying the exact signal
	// that `[?]` rendering, the status warning and the `unrecognised_task_marker`
	// violation exist to raise, before any of them can fire. Bail out instead
	// so the conflict escalates to the reconciliation agent. Deliberately
	// broader than mergeTaskLine: it matches ID-less task lines too, which
	// mergeTaskLine skips. See issue #27.
	anyTaskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s`)
	for _, side := range []struct {
		name  string
		lines []string
	}{{"ours", oursLines}, {"theirs", theirsLines}} {
		for _, line := range side.lines {
			m := anyTaskRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if _, ok := canonicalMarker(m[1]); !ok {
				fmt.Fprintf(os.Stderr,
					"  \033[33m⚠ %s: unrecognised task marker [%s] on the %s side — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
					relPath, m[1], side.name)
				return false
			}
		}
	}

	// Collect unique activity entries from "theirs", in document order.
	//
	// Three things this used to get wrong, all from testing the heading with an
	// unanchored Contains and ending the section on any `##` prefix:
	//
	//   - `strings.HasPrefix(line, "##")` is also true of `###`, so a milestone
	//     header ended the section — and, on the write side below, triggered the
	//     flush INSIDE the milestones region.
	//   - `strings.Contains(line, "## Activity")` is true of an ordinary task line
	//     that merely mentions the string, so a task could open the section.
	//   - the rows were held in a map, so the merged file's row order varied
	//     between runs on identical input.
	//
	// The heading is a level-2 ATX heading at column zero, and the section ends
	// where every other reader says a section ends: isSectionBreak.
	theirsActivityLines := make(map[string]bool)
	var theirsActivityOrder []string
	inActivity := false
	for _, line := range theirsLines {
		if activityHeadingRe.MatchString(line) {
			inActivity = true
			continue
		}
		if inActivity && isSectionBreak(line) {
			inActivity = false
		}
		if inActivity && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			row := strings.TrimSpace(line)
			if !theirsActivityLines[row] {
				theirsActivityLines[row] = true
				theirsActivityOrder = append(theirsActivityOrder, row)
			}
		}
	}

	// Merge: start from "ours", upgrade task states from "theirs"
	var merged []string
	inActivitySection := false
	activityInserted := make(map[string]bool)

	// A decision — `[-]` withdrawn or `[!]` blocked — beats progress from either
	// side, and whichever side's decision wins, the state it displaced is
	// reported. `mergeProgressState` warns in all four of those directions; this
	// resolver warned in none of them, and it is the path that runs when git
	// registered a conflict — the one occasion the user is already being told
	// went fine ("conflicts auto-resolved"). `.belmont/` is `--assume-unchanged`
	// inside a worktree, so a state dropped here is in no commit and
	// unrecoverable. See issue #46.
	warnDisplaced := func(taskID, decision, keptMarker, lostMarker string, keptIsOurs bool) {
		if keptIsOurs {
			fmt.Fprintf(os.Stderr,
				"  \033[33m⚠ %s: task %s is [%s] here but [%s] in the incoming version — the %s wins, so that state was not merged\033[0m\n",
				relPath, taskID, keptMarker, lostMarker, decision)
			return
		}
		fmt.Fprintf(os.Stderr,
			"  \033[33m⚠ %s: task %s is [%s] in the incoming version but [%s] here — the %s wins, so this side's state was not merged\033[0m\n",
			relPath, taskID, keptMarker, lostMarker, decision)
	}

	// Same region gate on the write side: a `[x]` quoted in ours' session log
	// must not be rewritten to theirs' state either.
	oursInRegion := false
	oursMS := ""
	oursSeen := make(map[string]bool)
	// Where a task carried over from "theirs" gets spliced in. These two maps
	// are the raw material; `milestoneCarryAnchor` turns them into the index,
	// and it is shared with `mergeProgressState` so the two merge paths cannot
	// answer "the end of this milestone's task list" differently again.
	//
	// This used to be a single `msInsertAfter` holding the index of the LAST
	// NON-BLANK line of the milestone's block. That agrees with the anchor for
	// every milestone ending on its last task's body — which is most of them —
	// and disagrees the moment one ends with prose after its tasks, putting a
	// carried task below the closing note while the copy path put it above.
	// Issue #60.
	oursLastTaskIdx := make(map[string]int)
	oursHeaderIdx := make(map[string]int)
	// Where each of ours' task lines landed in `merged`, and which milestone it
	// sits in. A carried task that was nested under a parent this side already
	// has is spliced after that parent's body rather than at the end of the
	// milestone: a nested bullet's meaning is positional, and re-parenting it to
	// whatever task happens to sit last is #33's mis-attribution in a new place.
	oursTaskIdx := make(map[string]int)
	oursTaskMS := make(map[string]string)
	for _, line := range oursLines {
		oursLineID := ""
		if m := msHeaderRe.FindStringSubmatch(line); m != nil {
			oursInRegion = true
			oursMS = "M" + m[1]
		} else if isSectionBreak(line) {
			oursInRegion = false
			oursMS = ""
		}
		// Upgrade task checkboxes to the more-advanced state
		if marker, id, ok := mergeTaskLine(line); oursInRegion && ok {
			oursMarker := marker
			taskID := id
			oursSeen[taskID] = true
			oursTaskMS[taskID] = oursMS
			oursLineID = taskID
			if theirsMarker, ok := theirsStates[taskID]; ok {
				oursRank, oursKnown := rank(oursMarker)
				theirsRank, theirsKnown := rank(theirsMarker)
				oursState, _ := canonicalMarker(oursMarker)
				theirsState, _ := canonicalMarker(theirsMarker)
				// Take the more-advanced state, except for the two states that
				// are decisions rather than progress.
				//
				// Withdrawal is checked FIRST and wins from either side, exactly
				// as it does in mergeProgressState — the two merge paths must
				// agree about a withdrawal, since which one runs depends only on
				// whether git happened to register a conflict. Rank alone cannot
				// express it: taskWithdrawn sorts at -2, below everything, so
				// without this branch anything at all outranked a withdrawal and
				// revived it as work to do. Before `[-]` was a recognised marker
				// the bail-out above caught that case and escalated the whole
				// file; making the marker legal removed the escalation and left
				// nothing in its place, so cancelled work came back as todo —
				// or as an un-evidenced `[v]` — under a green "conflicts
				// auto-resolved".
				//
				// Both sides are known here — the scan above already bailed on
				// anything unrecognised — but stay defensive rather than let a
				// future edit reintroduce a zero-value rank.
				switch {
				case oursState == taskWithdrawn:
					// Already withdrawn on ours; leave the line as written.
					if theirsState != taskTodo && theirsState != taskWithdrawn {
						warnDisplaced(taskID, "withdrawal", oursMarker, theirsMarker, true)
					}
				case theirsState == taskWithdrawn:
					if oursState != taskTodo {
						warnDisplaced(taskID, "withdrawal", theirsMarker, oursMarker, false)
					}
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				case oursState == taskBlocked:
					// Already blocked on ours; leave the line as written.
					if theirsState != taskTodo && theirsState != taskBlocked {
						warnDisplaced(taskID, "blocker", oursMarker, theirsMarker, true)
					}
				case theirsState == taskBlocked:
					// The half this function used to get wrong. `[!]` wins from EITHER
					// side, exactly as mergeProgressState has it. Keeping ours meant a
					// blocker the incoming side raised was discarded whenever this side
					// held a more advanced state — and which of the two functions runs
					// depends only on whether git happened to register a conflict, so the
					// same two documents merged either way gave different answers. Either
					// rule could be defended in isolation; having both, selected by that,
					// cannot. See issue #46.
					if oursState != taskTodo {
						warnDisplaced(taskID, "blocker", theirsMarker, oursMarker, false)
					}
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				case oursKnown && theirsKnown && theirsRank > oursRank:
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				}
			}
		}

		// Track activity section for merging entries. Same heading test and same
		// section boundary as the theirs-side scan above — when these two
		// disagreed, the flush fired on a `### M<n>:` header before ours' own
		// rows had been seen, so theirs' rows were inserted inside the milestones
		// region AND ours' identical rows were then appended again below.
		if activityHeadingRe.MatchString(line) {
			inActivitySection = true
		} else if inActivitySection && isSectionBreak(line) {
			for _, theirsLine := range theirsActivityOrder {
				if !activityInserted[theirsLine] {
					merged = append(merged, theirsLine)
					activityInserted[theirsLine] = true
				}
			}
			inActivitySection = false
		}

		if inActivitySection && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			activityInserted[strings.TrimSpace(line)] = true
		}

		merged = append(merged, line)

		// Recorded AFTER the append, against the index the line actually landed
		// at. Recording it before was a live bug: the activity-merge block above
		// appends theirs' unseen rows during this same iteration, and its trigger
		// `strings.HasPrefix(line, "##")` also matches a `###` milestone header —
		// so the header landed K rows later than the index taken at the top. A
		// milestone whose block here is a bare header has no later line to correct
		// it, and the carried task was spliced before that header, into the
		// PREVIOUS milestone. The result parses as a cross-milestone task ID,
		// which `belmont validate` rates severityError.
		// Both recorded AFTER the append, against the index the line actually
		// landed at, for the reason the comment above gives.
		if oursInRegion && oursMS != "" {
			if msHeaderRe.MatchString(line) {
				oursHeaderIdx[oursMS] = len(merged) - 1
			}
			if _, _, isTask := mergeTaskLine(line); isTask {
				oursLastTaskIdx[oursMS] = len(merged) - 1
			}
		}
		// Recorded AFTER the append for the same reason the anchor maps are: the
		// activity-merge block above can append rows during this same iteration,
		// so an index taken at the top of the body is stale by exactly that many.
		if oursLineID != "" {
			oursTaskIdx[oursLineID] = len(merged) - 1
		}
	}

	// Flush at end of document. The loop only flushes when a following section
	// heading arrives, so an activity section that is the file's LAST section —
	// which is where PROGRESS.md actually puts it — never received theirs' new
	// rows at all. They were silently dropped.
	// Inserted BEFORE any trailing blank lines, not after them. A file ending in
	// a newline splits into a final "" element, which the walk appends like any
	// other line — so a plain append put the incoming rows below the blank that
	// ends the table. They then render as a paragraph outside the log they
	// belong to, and the file loses its trailing newline. Master's template puts
	// `## Recent Activity` last, which is exactly where this flush is the one
	// that runs.
	if inActivitySection {
		var rows []string
		for _, theirsLine := range theirsActivityOrder {
			if !activityInserted[theirsLine] {
				rows = append(rows, theirsLine)
				activityInserted[theirsLine] = true
			}
		}
		if len(rows) > 0 {
			at := len(merged)
			for at > 0 && strings.TrimSpace(merged[at-1]) == "" {
				at--
			}
			tail := append([]string{}, merged[at:]...)
			merged = append(merged[:at], rows...)
			merged = append(merged, tail...)
		}
	}

	// Carry over every task that exists only in "theirs".
	//
	// The merge above walks "ours" and upgrades markers from a map built off
	// "theirs", so a task line that "theirs" has and "ours" does not was never
	// visited and never written — it was dropped, and the result committed under
	// a green "conflicts auto-resolved". `.belmont/` is `--assume-unchanged`
	// inside a worktree, so a dropped line is in no commit and unrecoverable;
	// losing a `[!]` this way looks exactly like the question was answered.
	// mergeProgressState already carries missing lines the other way for the
	// same reason. See issue #43.
	missing := map[string]bool{}
	for _, tt := range theirsOrder {
		if !oursSeen[tt.id] {
			missing[tt.id] = true
		}
	}
	// A task whose PARENT is itself being carried travels inside that parent's
	// block, exactly as `moveTaskLines` relocates a nested bullet with the block
	// enclosing it. Carrying both would splice the child twice, and a duplicated
	// ID is durable damage: countIDs in mergeProgressState refuses to reconcile
	// that task on every later merge and tells the user to de-duplicate by hand.
	// The IMMEDIATE parent is the whole test, and deliberately so: an ancestor
	// further up that is missing here while the immediate parent is not means
	// that ancestor's block carries a bullet this side already has, which the
	// nested-ID check below refuses outright. So there is no case where walking
	// the chain would skip a task this test keeps.
	var carried []theirsTask
	for _, tt := range theirsOrder {
		if !missing[tt.id] || missing[tt.parent] {
			continue
		}
		carried = append(carried, tt)
	}
	if len(carried) > 0 {
		// A carried task whose milestone "ours" does not have cannot be placed.
		// Escalate rather than append it somewhere plausible: the two sides
		// disagree about structure, which is a reconciliation-agent question,
		// and guessing here would be the silent-normalisation failure of #27
		// wearing a different hat.
		//
		// Each carried block is filed against the index in `merged` it goes
		// after — the end of its milestone's block, or the end of its parent's
		// body when the parent exists here — so both kinds of anchor splice
		// through one descending pass.
		//
		// Two anchors can be the SAME index, and then their order inside the
		// bucket decides parentage rather than merely tidiness:
		//
		//   - a parent that is the last task in its milestone here has
		//     `taskBodyEnd(parent) == milestoneCarryAnchor(ms)`, so a carried child and
		//     a brand-new top-level task collide. Emitted in incoming document
		//     order, a new top-level task written first takes the child with it —
		//     exactly the re-parenting the parent anchor exists to prevent.
		//   - a task whose body ends on the same line its own parent's body ends
		//     on collides with its parent, so two children with different parents
		//     share a bucket; whichever is emitted second nests under the first.
		//
		// Both are settled by emitting DEEPER anchors first: a child of the inner
		// task, then a child of the outer one, then anything anchored to the
		// milestone as a whole (depth -1). Ties keep incoming document order.
		type carriedBlock struct {
			depth int // indent of the anchoring task line here; -1 for a milestone anchor
			lines []string
		}
		pending := make(map[int][]carriedBlock)
		for _, tt := range carried {
			anchor, anchorOK := milestoneCarryAnchor(merged, oursLastTaskIdx, oursHeaderIdx, tt.milestone)
			if !anchorOK {
				fmt.Fprintf(os.Stderr,
					"  \033[33m⚠ %s: incoming task %s belongs to %s, which this side does not have — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
					relPath, tt.id, tt.milestone)
				return false
			}
			// The block is the bullet plus its body, and the body can hold
			// nested task bullets of its own. Those were never checked against
			// oursSeen — only the block's own ID was — so a nested bullet whose
			// ID already exists here as a top-level task got spliced in a second
			// time. That is durable damage rather than cosmetic: countIDs in
			// mergeProgressState sees the duplicate on the NEXT merge and refuses
			// to reconcile that task from then on, telling the user to
			// de-duplicate by hand. Escalate instead, matching the policy this
			// branch already applies to a structure disagreement.
			for _, bl := range tt.block[1:] {
				_, nestedID, ok := mergeTaskLine(bl)
				if !ok || !oursSeen[nestedID] {
					continue
				}
				fmt.Fprintf(os.Stderr,
					"  \033[33m⚠ %s: incoming task %s carries a nested bullet for %s, which already exists on this side — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
					relPath, tt.id, nestedID)
				return false
			}
			at, depth := anchor, -1
			if tt.parent != "" && oursSeen[tt.parent] {
				// Nested under a task this side already has: land it inside that
				// parent's body, past the parent's own `**Verification**` /
				// `**Evidence**` lines. Appending it at the end of the milestone
				// instead would re-parent it to whichever task sits last.
				if oursTaskMS[tt.parent] != tt.milestone {
					// The two sides file the parent under different milestones,
					// so placing the child under it would move the child too —
					// and a task whose ID names another milestone is
					// `cross_milestone_task_id`, severityError. Same escalation
					// as any other structure disagreement.
					fmt.Fprintf(os.Stderr,
						"  \033[33m⚠ %s: incoming task %s is nested under %s, which this side files under %s rather than %s — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
						relPath, tt.id, tt.parent, nonEmpty(oursTaskMS[tt.parent], "no milestone"), tt.milestone)
					return false
				}
				at = taskBodyEnd(merged, oursTaskIdx[tt.parent])
				depth = lineIndentWidth(merged[oursTaskIdx[tt.parent]])
			}
			pending[at] = append(pending[at], carriedBlock{depth: depth, lines: tt.block})
		}

		// Splice highest index first so the earlier insertion points stay valid.
		anchors := make([]int, 0, len(pending))
		for at := range pending {
			anchors = append(anchors, at)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(anchors)))
		for _, anchor := range anchors {
			blocks := pending[anchor]
			// Stable, so equal-depth blocks keep the order the incoming document
			// wrote them in.
			sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].depth > blocks[j].depth })
			var lines []string
			for _, b := range blocks {
				lines = append(lines, b.lines...)
			}
			at := anchor + 1
			tail := append([]string{}, merged[at:]...)
			merged = append(merged[:at], lines...)
			merged = append(merged, tail...)
		}

		fmt.Fprintf(os.Stderr,
			"  \033[36mⓘ %s: carried %d task line(s) present only in the incoming version\033[0m\n",
			relPath, len(carried))
	}

	result := strings.Join(merged, "\n")
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return false
	}

	addCmd := exec.Command("git", "add", relPath)
	addCmd.Dir = root
	addCmd.Run()
	return true
}

// autoResolveLockFiles detects conflicted lock files and regenerates them.
// Only handles lock files whose corresponding manifest is NOT conflicted
// (if the manifest is also conflicted, the AI agent needs to handle both together).
func autoResolveLockFiles(root string) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}

	// Map lock file → (package manager install command, manifest file)
	lockFileMap := map[string]struct {
		installCmd string
		manifest   string
	}{
		"package-lock.json": {"npm install", "package.json"},
		"pnpm-lock.yaml":    {"pnpm install", "package.json"},
		"yarn.lock":         {"yarn install", "package.json"},
		"bun.lockb":         {"bun install", "package.json"},
		"Cargo.lock":        {"cargo generate-lockfile", "Cargo.toml"},
		"go.sum":            {"go mod tidy", "go.mod"},
		"Gemfile.lock":      {"bundle install", "Gemfile"},
		"poetry.lock":       {"poetry lock --no-update", "pyproject.toml"},
	}

	conflicted := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			conflicted[line] = true
		}
	}

	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		// Only handle lock files in the repo root (basename match)
		baseName := filepath.Base(file)
		info, isLock := lockFileMap[baseName]
		if !isLock {
			continue
		}

		// Check if the corresponding manifest is also conflicted
		manifestPath := filepath.Join(filepath.Dir(file), info.manifest)
		if conflicted[manifestPath] {
			// Both conflicted — leave for the AI agent to handle together
			continue
		}

		fmt.Fprintf(os.Stderr, "  \033[2mAuto-resolving %s via %s\033[0m\n", file, info.installCmd)

		// Delete the conflicted lock file
		os.Remove(filepath.Join(root, file))

		// Run the package manager to regenerate
		parts := strings.Fields(info.installCmd)
		installCmd := exec.Command(parts[0], parts[1:]...)
		installCmd.Dir = root
		if installOut, err := installCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to regenerate %s: %s\033[0m\n", file, strings.TrimSpace(string(installOut)))
			// Restore the conflicted version so git knows it's still unresolved
			checkoutCmd := exec.Command("git", "checkout", "--merge", "--", file)
			checkoutCmd.Dir = root
			checkoutCmd.Run()
			continue
		}

		// Stage the regenerated lock file
		addCmd := exec.Command("git", "add", file)
		addCmd.Dir = root
		addCmd.Run()
	}
}
