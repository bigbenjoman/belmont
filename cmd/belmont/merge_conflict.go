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
	// Delimiter included deliberately — see the matching comment in
	// mergeProgressState. `parseTaskID` keys on `<ID>:`, and taking the shape
	// without the `:` made a bullet beginning `OAuth-2 …` carry that token as its
	// identity here while the parser said it had none.
	taskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s+(` + taskIDShape + `):`)
	theirsStates := make(map[string]string) // task ID → checkbox marker

	// Theirs' task lines in document order, with the milestone each one sits
	// in. Needed because a task present only in "theirs" has to be carried into
	// the merged file rather than dropped (issue #43), and the block it belongs
	// in is the one it was written in.
	type theirsTask struct {
		id        string
		block     []string
		milestone string
	}
	var theirsOrder []theirsTask

	theirsLines := strings.Split(string(theirsOut), "\n")
	inRegion := false
	theirsMS := ""
	// A bullet nested inside a carried task's body travels with the block that
	// encloses it, exactly as `moveTaskLines` treats one. Without this, the
	// nested line would also be carried on its own and appear twice.
	carriedThrough := -1
	for i, line := range theirsLines {
		if m := msHeaderRe.FindStringSubmatch(line); m != nil {
			inRegion = true
			theirsMS = "M" + m[1]
			continue
		}
		if isSectionBreak(line) {
			inRegion = false
			theirsMS = ""
			continue
		}
		if !inRegion {
			continue
		}
		if m := taskRe.FindStringSubmatch(line); m != nil {
			theirsStates[m[2]] = m[1]
			if i > carriedThrough {
				// The task is the bullet PLUS its indented body — carrying the
				// bullet alone strands its `**Verification**` / `**Evidence**`
				// lines on the side they came from, which is the #33 failure in
				// a different function.
				end := taskBodyEnd(theirsLines, i)
				theirsOrder = append(theirsOrder, theirsTask{
					id:        m[2],
					block:     append([]string{}, theirsLines[i:end+1]...),
					milestone: theirsMS,
				})
				carriedThrough = end
			}
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
	// broader than taskRe: it matches ID-less task lines too, which taskRe
	// skips. See issue #27.
	anyTaskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s`)
	for _, side := range []struct {
		name  string
		lines []string
	}{{"ours", strings.Split(string(oursOut), "\n")}, {"theirs", theirsLines}} {
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
	oursLines := strings.Split(string(oursOut), "\n")
	var merged []string
	inActivitySection := false
	activityInserted := make(map[string]bool)

	// Same region gate on the write side: a `[x]` quoted in ours' session log
	// must not be rewritten to theirs' state either.
	oursInRegion := false
	oursMS := ""
	oursSeen := make(map[string]bool)
	// Where a task carried over from "theirs" gets spliced in: the index in
	// `merged` of the LAST NON-BLANK line of that milestone's block. Tracking
	// the last task bullet alone would splice a carried task between a task and
	// its own indented `**Verification**` / `**Evidence**` body — the same class
	// of stranding as issue #33 — and inserting at the block boundary instead
	// would land it past the blank line that ends the list.
	msInsertAfter := make(map[string]int)
	// A milestone ID that heads more than one block cannot be an anchor: which
	// block does a carried task belong to? Refused rather than guessed, the same
	// policy mergeProgressState applies via dupMS and requireUnambiguousMilestones
	// applies at startup.
	oursMSHeadCount := make(map[string]int)
	for _, line := range oursLines {
		if m := msHeaderRe.FindStringSubmatch(line); m != nil {
			oursInRegion = true
			oursMS = "M" + m[1]
			oursMSHeadCount[oursMS]++
		} else if isSectionBreak(line) {
			oursInRegion = false
			oursMS = ""
		}
		// Upgrade task checkboxes to the more-advanced state
		if m := taskRe.FindStringSubmatch(line); oursInRegion && m != nil {
			oursMarker := m[1]
			taskID := m[2]
			oursSeen[taskID] = true
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
				case theirsState == taskWithdrawn:
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				case oursState == taskBlocked || theirsState == taskBlocked:
					// Keep ours. Note this is NOT the rule mergeProgressState
					// applies — there `[!]` wins from either side, here a blocker
					// on theirs loses to a more advanced ours. Pre-existing;
					// left alone deliberately rather than changed as a side
					// effect of the withdrawn fix.
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
		if oursInRegion && oursMS != "" && strings.TrimSpace(line) != "" {
			msInsertAfter[oursMS] = len(merged) - 1
		}
	}

	// Flush at end of document. The loop only flushes when a following section
	// heading arrives, so an activity section that is the file's LAST section —
	// which is where PROGRESS.md actually puts it — never received theirs' new
	// rows at all. They were silently dropped.
	if inActivitySection {
		for _, theirsLine := range theirsActivityOrder {
			if !activityInserted[theirsLine] {
				merged = append(merged, theirsLine)
				activityInserted[theirsLine] = true
			}
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
	var carried []theirsTask
	for _, tt := range theirsOrder {
		if !oursSeen[tt.id] {
			carried = append(carried, tt)
		}
	}
	if len(carried) > 0 {
		// A carried task whose milestone "ours" does not have cannot be placed.
		// Escalate rather than append it somewhere plausible: the two sides
		// disagree about structure, which is a reconciliation-agent question,
		// and guessing here would be the silent-normalisation failure of #27
		// wearing a different hat.
		byMS := make(map[string][]string)
		for _, tt := range carried {
			if _, ok := msInsertAfter[tt.milestone]; !ok {
				fmt.Fprintf(os.Stderr,
					"  \033[33m⚠ %s: incoming task %s belongs to %s, which this side does not have — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
					relPath, tt.id, tt.milestone)
				return false
			}
			if oursMSHeadCount[tt.milestone] > 1 {
				fmt.Fprintf(os.Stderr,
					"  \033[33m⚠ %s: incoming task %s belongs to %s, which heads %d separate blocks on this side — not auto-resolving; escalating to the reconciliation agent\033[0m\n",
					relPath, tt.id, tt.milestone, oursMSHeadCount[tt.milestone])
				return false
			}
			byMS[tt.milestone] = append(byMS[tt.milestone], tt.block...)
		}

		// Splice highest index first so the earlier insertion points stay valid.
		msIDs := make([]string, 0, len(byMS))
		for ms := range byMS {
			msIDs = append(msIDs, ms)
		}
		sort.Slice(msIDs, func(i, j int) bool { return msInsertAfter[msIDs[i]] > msInsertAfter[msIDs[j]] })
		for _, ms := range msIDs {
			at := msInsertAfter[ms] + 1
			tail := append([]string{}, merged[at:]...)
			merged = append(merged[:at], byMS[ms]...)
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
