package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func milestoneAllDone(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskDone && t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneAllVerified(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneHasBlockers(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status == taskBlocked {
			return true
		}
	}
	return false
}

func milestoneNotStarted(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status != taskTodo {
			return false
		}
	}
	return true
}

// parseMasterDeps reads the master PROGRESS.md and extracts feature slug → dependency slugs mapping
// from the ## Features table. Handles "None", empty, and comma-separated slugs.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func parseMasterDeps(root string) (deps map[string][]string, priorities map[string]string) {
	deps = make(map[string][]string)
	priorities = make(map[string]string)

	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}

		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		prioCol := colIdx["Priority"]
		depCol := colIdx["Dependencies"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		if prioCol >= 0 && prioCol < len(cells) {
			priorities[slug] = strings.TrimSpace(cells[prioCol])
		}

		if depCol < 0 || depCol >= len(cells) {
			continue
		}
		depStr := strings.TrimSpace(cells[depCol])
		if depStr == "" || strings.EqualFold(depStr, "None") || depStr == "-" {
			continue
		}

		var depSlugs []string
		for _, d := range strings.Split(depStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depSlugs = append(depSlugs, d)
			}
		}
		if len(depSlugs) > 0 {
			deps[slug] = depSlugs
		}
	}
	return
}

// flattenTasks extracts all tasks from parsed milestones, sorted by task ID.
func flattenTasks(milestones []milestone, maxName int) []task {
	var tasks []task
	for _, m := range milestones {
		for _, t := range m.Tasks {
			name := t.Name
			if maxName > 0 && len([]rune(name)) > maxName {
				name = string([]rune(name)[:maxName-1]) + "…"
			}
			tasks = append(tasks, task{ID: t.ID, Name: name, Status: t.Status, MilestoneID: t.MilestoneID, Marker: t.Marker, Line: t.Line})
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		pi, ni := parseTaskOrder(tasks[i].ID)
		pj, nj := parseTaskOrder(tasks[j].ID)
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})

	return tasks
}

// canonicalMarker maps a raw checkbox marker to its task state. It is the
// SINGLE source of truth for what a marker means — every reader that needs to
// interpret one must route through it rather than comparing raw bytes.
//
// The second return value reports whether the marker was recognised. An
// unrecognised marker yields (taskUnknown, false): Belmont does not guess a
// state for a checkbox it cannot read. See issue #27.
//
// Case handling is deliberately asymmetric:
//
//   - `[X]` is accepted as done. A capital X is a universal Markdown
//     convention and is what GitHub itself renders as checked, so reading it
//     as anything else silently resurrects finished work.
//   - `[V]` is NOT accepted as verified. Unlike X it has no external
//     convention behind it — `[v]` is Belmont's own invention, nothing else
//     emits it, and no skill, agent, prompt or template in this repo writes a
//     capital one. Accepting it bought nothing and cost enforcement: the
//     commit-evidence guard compares the raw marker, so a `[V]` flip counted
//     as verified everywhere while being invisible to `runEvidenceCheck` —
//     a silent bypass of the invariant in knowledge/auto-mode/verify-evidence.md.
//     A capital V now parses to taskUnknown and fails loudly instead.
//
// If you add a state here, every caller of canonicalMarker picks it up. That
// is the point: the previous design spread marker literals across four files
// that disagreed with each other.
func canonicalMarker(marker string) (taskStatus, bool) {
	switch marker {
	case " ":
		return taskTodo, true
	case ">":
		return taskInProgress, true
	case "x", "X":
		return taskDone, true
	case "v":
		return taskVerified, true
	case "!":
		return taskBlocked, true
	default:
		return taskUnknown, false
	}
}

// taskStatePriority orders task states by how advanced they are. Used wherever
// two versions of the same task must be reconciled — merge-conflict resolution
// and the post-merge worktree state sync.
//
// Blocked is deliberately below todo: `[!]` is a human signal that something
// needs attention, so it must never be silently overwritten by a "more
// advanced" state from the other side. Callers special-case it.
var taskStatePriority = map[taskStatus]int{
	taskTodo:       0,
	taskInProgress: 1,
	taskDone:       2,
	taskVerified:   3,
	taskBlocked:    -1,
}

// markerRank returns how advanced a raw marker is, and whether it was
// recognised. Unrecognised markers report (0, false) — callers must treat that
// as "cannot compare", NOT as "equal to todo". Ranking an unreadable marker
// alongside real states is what let `[X]` lose to `[>]` and let unrecognised
// markers be silently overwritten at merge time. See issue #27.
func markerRank(marker string) (int, bool) {
	st, ok := canonicalMarker(marker)
	if !ok {
		return 0, false
	}
	p, known := taskStatePriority[st]
	return p, known
}

// markerIsVerified reports whether a raw marker means "verified". Used by the
// evidence guard, which works on raw markers read out of PROGRESS.md rather
// than on parsed tasks.
func markerIsVerified(marker string) bool {
	st, ok := canonicalMarker(marker)
	return ok && st == taskVerified
}

func parseMilestones(progress string) []milestone {
	// Match milestone headers: ### M1: Name
	msRe := regexp.MustCompile(`(?m)^###\s+M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	// Match task checkboxes: - [ ] P0-1: Task Name, - [x] ..., - [>] ..., - [v] ..., - [!] ...
	taskRe := regexp.MustCompile(`(?m)^\s*-\s+\[(.)\]\s+(.+)$`)

	lines := strings.Split(progress, "\n")
	var milestones []milestone
	var currentMS *milestone

	for lineIdx, line := range lines {
		// Check for milestone header
		if msMatch := msRe.FindStringSubmatch(line); len(msMatch) >= 3 {
			// Save previous milestone
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
			}

			id := "M" + strings.TrimSpace(msMatch[1])
			name := strings.TrimSpace(msMatch[2])

			// Extract dependency annotations from name
			var deps []string
			if depsMatch := depsRe.FindStringSubmatch(name); len(depsMatch) >= 2 {
				name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
				for _, d := range strings.Split(depsMatch[1], ",") {
					deps = append(deps, strings.TrimSpace(d))
				}
			}

			currentMS = &milestone{ID: id, Name: name, Deps: deps}
			continue
		}

		// Check for next section (## header) — stops current milestone
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
				currentMS = nil
			}
			continue
		}

		// Parse task checkboxes under current milestone
		if currentMS != nil {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				marker := taskMatch[1]
				taskText := strings.TrimSpace(taskMatch[2])

				status, _ := canonicalMarker(marker)

				// Extract task ID if present (e.g., "P0-1: Task Name")
				taskID := ""
				taskName := taskText
				idRe := regexp.MustCompile(`^(P\d+-[\w][\w-]*):\s*(.+)$`)
				if idMatch := idRe.FindStringSubmatch(taskText); len(idMatch) >= 3 {
					taskID = idMatch[1]
					taskName = strings.TrimSpace(idMatch[2])
				}

				currentMS.Tasks = append(currentMS.Tasks, task{
					ID:          taskID,
					Name:        taskName,
					Status:      status,
					MilestoneID: currentMS.ID,
					Marker:      marker,
					Line:        lineIdx + 1,
				})
			}
		}
	}

	// Don't forget the last milestone
	if currentMS != nil {
		milestones = append(milestones, *currentMS)
	}

	return milestones
}

// overlayLiveMilestones returns `base` with each milestone whose ID matches an
// entry in `perMilestoneLive` replaced by that worktree's current view of the
// milestone. Milestones with no active worktree are returned unchanged.
// Overlaid milestones carry a LiveFrom pointer so renderers can annotate them.
func overlayLiveMilestones(base []milestone, perMilestoneLive map[string]string) []milestone {
	out := make([]milestone, 0, len(base))
	for _, m := range base {
		live, ok := perMilestoneLive[m.ID]
		if !ok {
			out = append(out, m)
			continue
		}
		wtProgressPath := filepath.Join(live, "PROGRESS.md")
		data, err := os.ReadFile(wtProgressPath)
		if err != nil {
			out = append(out, m) // worktree lost its PROGRESS.md — fall back to master
			continue
		}
		wtMilestones := parseMilestones(string(data))
		var replaced bool
		for _, wm := range wtMilestones {
			if wm.ID == m.ID {
				wm.LiveFrom = live
				out = append(out, wm)
				replaced = true
				break
			}
		}
		if !replaced {
			// Worktree doesn't have this milestone (shouldn't happen) — keep master.
			out = append(out, m)
		}
	}
	return out
}

func parseTaskOrder(id string) (int, int) {
	re := regexp.MustCompile(`^P(\d+)-(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) != 3 {
		return 99, 99
	}
	return atoiDefault(match[1], 99), atoiDefault(match[2], 99)
}

func lastCompletedTask(tasks []task) *task {
	var last *task
	for i := range tasks {
		if tasks[i].Status == taskDone || tasks[i].Status == taskVerified {
			t := tasks[i]
			last = &t
		}
	}
	return last
}

func nextMilestone(milestones []milestone) *milestone {
	for _, m := range milestones {
		if !milestoneAllDone(m) {
			mm := m
			return &mm
		}
	}
	return nil
}

// nextTask returns the first task available to work on.
//
// The condition is a POSITIVE match on the two workable states, and that is
// load-bearing: it is what keeps taskUnknown out. Issue #27 was an entry whose
// marker Belmont could not read being handed to an agent as the next thing to
// build — withdrawn work, and one feature cancelled by a product decision.
//
// Do not rewrite this as a negative ("anything not done or verified"). That
// reads as equivalent and silently re-admits taskUnknown, plus any state added
// later. TestUnknownMarkerIsNeverScheduled fails if you do.
func nextTask(tasks []task) *task {
	for _, t := range tasks {
		if t.Status == taskInProgress || t.Status == taskTodo {
			tt := t
			return &tt
		}
	}
	return nil
}

// unknownMarkerTasks returns every task whose checkbox marker Belmont could not
// recognise, across all milestones, in document order.
func unknownMarkerTasks(milestones []milestone) []task {
	var out []task
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskUnknown {
				out = append(out, t)
			}
		}
	}
	return out
}

// blockedTaskCount returns the number of tasks with [!] status across all milestones.
func blockedTaskCount(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				count++
			}
		}
	}
	return count
}

// blockedTaskNames returns descriptions of blocked tasks for display.
func blockedTaskNames(milestones []milestone) []string {
	var names []string
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				label := t.Name
				if t.ID != "" {
					label = t.ID + ": " + t.Name
				}
				names = append(names, label)
			}
		}
	}
	return names
}

func parseDecisions(progress string, limit int) []string {
	lines := parseSectionLines(progress, "## Decisions Log")
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func parseSectionLines(doc, header string) []string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(header) + `\s*$`)
	loc := re.FindStringIndex(doc)
	if loc == nil {
		return nil
	}
	rest := doc[loc[1]:]
	lines := strings.Split(rest, "\n")
	var results []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if trimmed == "" {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "none") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "-")
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func techPlanReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) != ""
}

// fileHasRealContent checks if a file exists and has content beyond template/placeholder text.
func fileHasRealContent(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return false
	}
	// Check for known template/placeholder texts
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "run /belmont:") || strings.HasPrefix(lower, "run the /belmont:") {
		return false
	}
	return true
}

func computeOverallStatus(tasks []task) string {
	if len(tasks) == 0 {
		return "Not Started"
	}

	allVerified := true
	allDone := true
	anyProgress := false
	allBlocked := true

	for _, t := range tasks {
		if t.Status != taskVerified {
			allVerified = false
		}
		if t.Status != taskDone && t.Status != taskVerified {
			allDone = false
		}
		if t.Status == taskDone || t.Status == taskVerified || t.Status == taskInProgress {
			anyProgress = true
		}
		if t.Status != taskBlocked {
			allBlocked = false
		}
	}

	if allVerified {
		return "Verified"
	}
	if allDone {
		return "Complete"
	}
	if allBlocked {
		return "BLOCKED"
	}
	if anyProgress {
		return "In Progress"
	}
	return "Not Started"
}

// isFeatureTerminal reports whether a feature is done — either all tasks
// implemented ("Complete"), all tasks verified ("Verified"), or archived via
// /belmont:cleanup ("Archived"). Terminal features are skipped by `auto --all`
// and treated as dep-satisfying (but non-executing) in wave planning.
func isFeatureTerminal(status string) bool {
	switch status {
	case "Complete", "Verified", "Archived":
		return true
	}
	return false
}

func milestonesInRange(milestones []milestone, from, to string) []milestone {
	if from == "" && to == "" {
		return milestones
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	var result []milestone
	for _, m := range milestones {
		num := parseMilestoneNum(m.ID)
		if num < 0 {
			continue
		}
		if fromNum >= 0 && num < fromNum {
			continue
		}
		if toNum >= 0 && num > toNum {
			continue
		}
		result = append(result, m)
	}
	return result
}

func parseMilestoneNum(id string) int {
	if id == "" {
		return -1
	}
	re := regexp.MustCompile(`(?i)M(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) < 2 {
		return -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return n
}

// skipMilestoneInProgress marks all incomplete tasks in a milestone as done in PROGRESS.md.
func skipMilestoneInProgress(root, feature, milestoneID string) error {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("read PROGRESS.md: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	taskRe := regexp.MustCompile(`^(\s*-\s+)\[[ >!]\](\s+.*)$`)

	inTarget := false
	changed := false
	for i, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			inTarget = ("M" + m[1]) == milestoneID
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inTarget = false
			continue
		}
		if inTarget {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				lines[i] = taskMatch[1] + "[x]" + taskMatch[2]
				changed = true
			}
		}
	}

	if !changed {
		return fmt.Errorf("milestone %s not found or already done", milestoneID)
	}

	return os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
}

func detectFwlupTasks(root, feature string, report statusReport) bool {
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	// Check if any todo/in_progress tasks have FWLUP in their ID or name
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				return true
			}
		}
	}
	return false
}

// extractMilestoneFromTaskID extracts the milestone ID from a task ID like "P5-M5-FWLUP-1" → "M5".
func extractMilestoneFromTaskID(taskID string) string {
	re := regexp.MustCompile(`P\d+-M(\d+)`)
	m := re.FindStringSubmatch(taskID)
	if len(m) >= 2 {
		return "M" + m[1]
	}
	return ""
}

// detectFwlupTasksForMilestone checks for pending FWLUP tasks scoped to a specific milestone.
func detectFwlupTasksForMilestone(root, feature string, report statusReport, milestoneID string) bool {
	if milestoneID == "" {
		return false
	}
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				// Tasks now carry their milestone ID from PROGRESS.md
				if t.MilestoneID == milestoneID || extractMilestoneFromTaskID(t.ID) == milestoneID {
					return true
				}
			}
		}
	}
	return false
}

// pendingTasksInRange checks for incomplete tasks under milestones
// that fall within the from/to range in the feature's PROGRESS.md.
// When from and to are both empty, falls back to checking all milestones.
func pendingTasksInRange(root, feature, from, to string) bool {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task: [ ], [>], [!]
	taskRe := regexp.MustCompile(`^\s*-\s+\[[ >!]\]`)

	inRange := fromNum < 0 && toNum < 0 // if no range, all milestones are in range
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && taskRe.MatchString(line) {
			return true
		}
	}
	return false
}

// fwlupTasksInRange checks for unchecked FWLUP tasks under milestones within the from/to range.
// When from and to are both empty, falls back to the global detectFwlupTasks.
func fwlupTasksInRange(root, feature string, report statusReport, from, to string) bool {
	if from == "" && to == "" {
		return detectFwlupTasks(root, feature, report)
	}

	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task with FWLUP in the text
	fwlupTaskRe := regexp.MustCompile(`(?i)^\s*-\s+\[[ >!]\].*FWLUP`)

	inRange := false
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && fwlupTaskRe.MatchString(line) {
			return true
		}
	}
	return false
}
