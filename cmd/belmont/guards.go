package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// snapshotProgress reads PROGRESS.md for the feature and builds a snapshot.
// Returns nil on any read/parse error — the caller should treat nil as
// "no baseline, skip guard."
func snapshotProgress(root, feature string) *progressSnapshot {
	if root == "" || feature == "" {
		return nil
	}
	path := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseProgressSnapshot(path, string(data))
}

// runScopeGuard performs the post-phase check + revert + commit amend +
// steering correction. Runs for every action except tech-plan (which is
// allowed to restructure milestones). Silent no-op on nil snapshot.
func runScopeGuard(cfg loopConfig, action loopAction, pre *progressSnapshot) {
	if pre == nil {
		return
	}
	// Tech-plan phase may legitimately create/edit milestone structure.
	if action.Type == actionReplan {
		return
	}
	postData, err := os.ReadFile(pre.Path)
	if err != nil {
		return
	}
	post := parseProgressSnapshot(pre.Path, string(postData))
	if post == nil {
		return
	}
	violations := diffScopeViolations(pre, post, action.MilestoneID)
	if len(violations) == 0 {
		return
	}
	rebuilt, err := rebuildAfterScopeGuard(pre, post, action.MilestoneID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ scope guard rebuild failed: %s\033[0m\n", err)
		return
	}
	if rebuilt == post.Raw {
		// Nothing to rewrite (violations were structural but revert produced
		// identical bytes — should be impossible, but bail safely).
		return
	}
	if err := os.WriteFile(pre.Path, []byte(rebuilt), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ scope guard write failed: %s\033[0m\n", err)
		return
	}
	// Amend the agent's last commit to include our revert (best-effort).
	// If there's no agent commit, this no-ops harmlessly.
	amend := exec.Command("git", "commit", "-a", "--amend", "--no-edit")
	amend.Dir = cfg.Root
	_ = amend.Run()

	// Stream log + steering correction.
	logScopeGuardRevert(cfg.Feature, action.MilestoneID, violations)
	injectScopeGuardSteering(cfg, action, violations)
}

// diffScopeViolations walks pre vs post and returns the list of violations.
// When targetMS is empty the out-of-scope-flip rule is relaxed (allow any
// milestone's checkboxes to change) but the no-new-milestone rule still
// applies.
func diffScopeViolations(pre, post *progressSnapshot, targetMS string) []scopeViolation {
	var out []scopeViolation
	// Rule A: new milestones in post that didn't exist in pre.
	for _, pb := range post.Blocks {
		if _, ok := pre.ByID[pb.ID]; !ok {
			out = append(out, scopeViolation{
				Kind:          "new_milestone",
				Milestone:     pb.ID,
				MilestoneName: pb.Name,
			})
		}
	}
	// Rule B: checkbox changes in milestones other than the target (and that
	// existed pre — newly added ones are handled by rule A).
	if targetMS != "" {
		for _, pb := range post.Blocks {
			if pb.ID == targetMS {
				continue
			}
			preIdx, ok := pre.ByID[pb.ID]
			if !ok {
				continue
			}
			preBlock := pre.Blocks[preIdx]
			for taskID, postState := range pb.TaskStates {
				preState, existed := preBlock.TaskStates[taskID]
				if !existed {
					// New task added to a non-target milestone: treat as a flip
					// violation with From=∅. This catches cases like FWLUPs
					// being inserted under the wrong milestone.
					out = append(out, scopeViolation{
						Kind:          "out_of_scope_flip",
						Milestone:     pb.ID,
						MilestoneName: pb.Name,
						TaskID:        taskID,
						FromState:     "∅",
						ToState:       postState,
					})
					continue
				}
				if preState != postState {
					out = append(out, scopeViolation{
						Kind:          "out_of_scope_flip",
						Milestone:     pb.ID,
						MilestoneName: pb.Name,
						TaskID:        taskID,
						FromState:     preState,
						ToState:       postState,
					})
				}
			}
		}
	}
	return out
}

// rebuildAfterScopeGuard produces the corrected PROGRESS.md content:
//
//   - For milestone blocks in post that existed in pre and are out of scope:
//     replace their text with the pre version.
//   - For milestone blocks newly added in post: remove them entirely.
//   - For the target milestone (and non-milestone content): keep post.
func rebuildAfterScopeGuard(pre, post *progressSnapshot, targetMS string) (string, error) {
	// Strategy: walk post's raw lines, identifying milestone block boundaries
	// on the fly. For each block, emit either the pre version or the post
	// version (or skip entirely for new milestones). Non-milestone lines are
	// emitted verbatim.
	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)

	var out strings.Builder
	lines := strings.Split(post.Raw, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
			msID := "M" + m[1]
			// Find block boundary: next line matching block-start or level-2 header.
			start := i
			j := i + 1
			for j < len(lines) {
				nxt := lines[j]
				if msHeaderRe.MatchString(nxt) {
					break
				}
				trim := strings.TrimSpace(nxt)
				if strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ") {
					break
				}
				j++
			}
			// start:j is the post block (inclusive:exclusive).
			preIdx, existedPre := pre.ByID[msID]
			switch {
			case !existedPre:
				// Newly added milestone — skip entirely (emit nothing).
			case msID == targetMS || targetMS == "":
				// In scope (or unscoped action): keep post bytes as-is.
				for k := start; k < j; k++ {
					out.WriteString(lines[k])
					if k < j-1 || j < len(lines) {
						out.WriteString("\n")
					}
				}
			default:
				// Out of scope: replace with pre block verbatim.
				preLines := pre.Blocks[preIdx].RawLines
				for k, pl := range preLines {
					out.WriteString(pl)
					if k < len(preLines)-1 || j < len(lines) {
						out.WriteString("\n")
					}
				}
			}
			i = j
			continue
		}
		// Non-milestone line: emit verbatim.
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
		i++
	}
	return out.String(), nil
}

// runEvidenceCheck is the public entry point.
func runEvidenceCheck(cfg loopConfig, action loopAction, pre *progressSnapshot) {
	if pre == nil {
		return
	}
	if action.Type != actionVerify {
		return
	}
	postData, err := os.ReadFile(pre.Path)
	if err != nil {
		return
	}
	post := parseProgressSnapshot(pre.Path, string(postData))
	if post == nil {
		return
	}
	missing := findEvidenceMissingFlips(cfg.Root, pre, post, action.MilestoneID)
	if len(missing) == 0 {
		return
	}
	rebuilt := revertEvidenceMissing(post, pre, missing)
	if rebuilt == post.Raw {
		return
	}
	if err := os.WriteFile(pre.Path, []byte(rebuilt), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ evidence check write failed: %s\033[0m\n", err)
		return
	}
	amend := exec.Command("git", "commit", "-a", "--amend", "--no-edit")
	amend.Dir = cfg.Root
	_ = amend.Run()
	logEvidenceRevert(cfg.Feature, action.MilestoneID, missing)
	injectEvidenceSteering(cfg, action, missing)
}

// detectViolations is the pure rule engine — no IO. Takes parsed milestones,
// returns a list of findings. Safe for test coverage.
func detectViolations(slug string, milestones []milestone) []validationViolation {
	var out []validationViolation
	for _, m := range milestones {
		// Rule 1: milestone name matches a polish/follow-up pattern.
		if polishMilestoneNameRe.MatchString(m.Name) {
			out = append(out, validationViolation{
				Feature:       slug,
				Milestone:     m.ID,
				MilestoneName: m.Name,
				Rule:          "polish_milestone_name",
				Message: fmt.Sprintf(
					"milestone %s %q looks like a polish/follow-up catch-all. Follow-ups belong in their source milestone (the one that discovered them) as new `[ ]` tasks, not a dedicated milestone. Run `/belmont:tech-plan` to restructure.",
					m.ID, m.Name),
			})
		}
		// Rule 2: task IDs reference a milestone other than the one they live in.
		currentNum := milestoneNumber(m.ID)
		for _, t := range m.Tasks {
			match := taskIDMilestoneRefRe.FindStringSubmatch(t.ID)
			if len(match) < 2 {
				continue
			}
			refNum, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			if refNum != currentNum {
				out = append(out, validationViolation{
					Feature:       slug,
					Milestone:     m.ID,
					MilestoneName: m.Name,
					TaskID:        t.ID,
					Rule:          "cross_milestone_task_id",
					Message: fmt.Sprintf(
						"task %q in milestone %s names milestone M%d in its ID. It belongs under M%d. Move it there — keeping it here is the dependency-graph lie that causes parallel merge conflicts.",
						t.ID, m.ID, refNum, refNum),
				})
			}
		}
	}
	return out
}

// parseProgressSnapshot splits the PROGRESS.md content into milestone blocks.
// A block begins at a line matching the milestone header regex; it ends when
// the next block begins, or when a `## ` level-2 heading is encountered, or
// at EOF. Non-milestone content is not stored as a separate block; instead
// the Raw field is kept so the rebuilder can do line-boundary-preserving
// replacement via string operations.
func parseProgressSnapshot(path, content string) *progressSnapshot {
	snap := &progressSnapshot{Path: path, Raw: content, ByID: map[string]int{}}
	msHeaderRe := regexp.MustCompile(`(?m)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	taskRe := regexp.MustCompile(`(?m)^\s*-\s+\[(.)\]\s+(\S+?):`)

	lines := strings.Split(content, "\n")
	var current *milestoneBlockText
	flush := func() {
		if current != nil {
			snap.ByID[current.ID] = len(snap.Blocks)
			snap.Blocks = append(snap.Blocks, *current)
			current = nil
		}
	}
	for _, line := range lines {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
			flush()
			id := "M" + m[1]
			name := strings.TrimSpace(m[2])
			name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
			current = &milestoneBlockText{ID: id, Name: name, TaskStates: map[string]string{}}
			current.RawLines = append(current.RawLines, line)
			continue
		}
		// A non-milestone level-2 heading closes the current block.
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ") {
			flush()
			continue
		}
		if current != nil {
			current.RawLines = append(current.RawLines, line)
			if tm := taskRe.FindStringSubmatch(line); len(tm) >= 3 {
				current.TaskStates[tm[2]] = tm[1]
			}
		}
	}
	flush()
	return snap
}

// revertEvidenceMissing rebuilds PROGRESS.md so that each entry in `missing`
// reverts its task line from "[v]" back to the prior state captured in pre.
// Uses a line-level replacement scoped to each task's milestone block.
func revertEvidenceMissing(post, pre *progressSnapshot, missing []evidenceMissing) string {
	// Build a map: milestone -> taskID -> fromState, for O(1) lookup.
	byMS := map[string]map[string]string{}
	for _, m := range missing {
		if byMS[m.Milestone] == nil {
			byMS[m.Milestone] = map[string]string{}
		}
		fromState := m.FromState
		if fromState == "" {
			fromState = " "
		}
		byMS[m.Milestone][m.TaskID] = fromState
	}

	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)
	taskRe := regexp.MustCompile(`^(\s*-\s+\[)(.)\](\s+)(\S+?)(:.*)$`)

	var out strings.Builder
	var currentMS string
	lines := strings.Split(post.Raw, "\n")
	for i, line := range lines {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
			currentMS = "M" + m[1]
		} else {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ") {
				currentMS = ""
			}
		}
		if currentMS != "" {
			if overrides, ok := byMS[currentMS]; ok {
				if tm := taskRe.FindStringSubmatch(line); len(tm) >= 6 {
					taskID := tm[4]
					if from, hit := overrides[taskID]; hit && tm[2] == "v" {
						line = tm[1] + from + "]" + tm[3] + tm[4] + tm[5]
					}
				}
			}
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

// validationViolation is one finding from `belmont validate`.
type validationViolation struct {
	Feature       string `json:"feature"`
	Milestone     string `json:"milestone"`
	MilestoneName string `json:"milestone_name"`
	TaskID        string `json:"task_id,omitempty"`
	Rule          string `json:"rule"`
	Message       string `json:"message"`
}

// Matches milestone names that look like polish / follow-up catch-alls.
// Conservative — only fires on unambiguously bad patterns so legitimate
// cross-cutting milestones ("Accessibility audit across routes") aren't
// flagged. Case-insensitive via (?i).
var polishMilestoneNameRe = regexp.MustCompile(`(?i)(\bpolish\b|\bfollow[- ]?ups?\b|\bcleanup\b|\bverification fixes?\b|\bdesign fidelity fixes?\b|\bdeviations from\s+m\d+|from\s+m\d+\s+implementation\b|\bfwlup[s]?\b)`)

// Matches task IDs that embed a milestone number, e.g. P3-FWLUP-M2-1 or
// P1-M4-FIX-2. Capture group 1 is the milestone number referenced by the ID.
var taskIDMilestoneRefRe = regexp.MustCompile(`^P\d+-(?:FWLUP-)?M(\d+)(?:-|$)`)

// milestoneNumber extracts the integer from a milestone ID like "M5".
func milestoneNumber(id string) int {
	trimmed := strings.TrimPrefix(id, "M")
	if n, err := strconv.Atoi(trimmed); err == nil {
		return n
	}
	return -1
}

// renderValidationReport writes a human-readable summary to w.
func renderValidationReport(w io.Writer, violations []validationViolation) {
	if len(violations) == 0 {
		fmt.Fprintln(w, "\033[32m✓ No milestone-structure violations found.\033[0m")
		return
	}
	fmt.Fprintf(w, "\033[31m✗ %d violation(s) found:\033[0m\n\n", len(violations))
	// Group by feature → milestone for readability.
	byFeat := map[string][]validationViolation{}
	var featOrder []string
	for _, v := range violations {
		if _, seen := byFeat[v.Feature]; !seen {
			featOrder = append(featOrder, v.Feature)
		}
		byFeat[v.Feature] = append(byFeat[v.Feature], v)
	}
	for _, feat := range featOrder {
		fmt.Fprintf(w, "  \033[1m%s\033[0m\n", feat)
		for _, v := range byFeat[feat] {
			if v.TaskID != "" {
				fmt.Fprintf(w, "    • [%s/%s] %s — %s\n", v.Milestone, v.TaskID, v.Rule, v.Message)
			} else {
				fmt.Fprintf(w, "    • [%s] %s — %s\n", v.Milestone, v.Rule, v.Message)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\033[2mRerun after fixing with `belmont validate`. See skills/belmont/_partials/milestone-immutability.md for the canonical rule.\033[0m")
}

// milestoneBlockText captures a single milestone block's exact bytes along
// with its task state map, so we can both diff state and rewrite verbatim.
type milestoneBlockText struct {
	ID         string
	Name       string
	RawLines   []string          // the header line plus every line up to (exclusive) the next block boundary
	TaskStates map[string]string // taskID -> marker rune as string ([ ], [x], [v], [>], [!])
}

// progressSnapshot preserves enough of PROGRESS.md to rebuild it after
// reverting out-of-scope edits. Non-milestone lines (preamble, activity log,
// decisions section) are stored too so they can be preserved verbatim.
type progressSnapshot struct {
	Path   string
	Raw    string
	Blocks []milestoneBlockText
	ByID   map[string]int // milestone ID -> index in Blocks
}

// scopeViolation is one finding from the post-phase guard.
type scopeViolation struct {
	Kind          string // "new_milestone" | "out_of_scope_flip"
	Milestone     string // milestone ID involved
	MilestoneName string
	TaskID        string // for out_of_scope_flip
	FromState     string // for out_of_scope_flip
	ToState       string // for out_of_scope_flip
}

// logScopeGuardRevert prints a one-line summary of each violation to stderr.
func logScopeGuardRevert(feature, milestoneID string, violations []scopeViolation) {
	prefix := ""
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	summary := summarizeScopeViolations(violations)
	fmt.Fprintf(os.Stderr, "%s\033[33m[SCOPE-GUARD]\033[0m reverted %d violation(s) — %s\n", prefix, len(violations), summary)
}

// summarizeScopeViolations produces a terse one-line summary suitable for
// the stream (matches steering's preview style).
func summarizeScopeViolations(violations []scopeViolation) string {
	counts := map[string]int{}
	var sample string
	for _, v := range violations {
		counts[v.Kind]++
		if sample == "" {
			switch v.Kind {
			case "new_milestone":
				sample = fmt.Sprintf("new milestone %s %q", v.Milestone, v.MilestoneName)
			case "out_of_scope_flip":
				sample = fmt.Sprintf("%s in %s (%s→%s)", v.TaskID, v.Milestone, v.FromState, v.ToState)
			}
		}
	}
	var parts []string
	if n := counts["new_milestone"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d new milestone", n))
	}
	if n := counts["out_of_scope_flip"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d out-of-scope flip", n))
	}
	return fmt.Sprintf("%s — first: %s", strings.Join(parts, ", "), sample)
}

// evidenceMissing records one [v] flip that lacks a commit referencing the task.
type evidenceMissing struct {
	Milestone string
	TaskID    string
	FromState string // prior state (pre)
}

// findEvidenceMissingFlips walks post, identifies tasks that flipped TO "v"
// this phase (vs pre), and returns any without a matching git commit. When
// targetMS is non-empty only tasks under that milestone are evaluated.
func findEvidenceMissingFlips(root string, pre, post *progressSnapshot, targetMS string) []evidenceMissing {
	mergeBase := findMergeBaseRef(root)
	var missing []evidenceMissing
	for _, pb := range post.Blocks {
		if targetMS != "" && pb.ID != targetMS {
			continue
		}
		preIdx, existedPre := pre.ByID[pb.ID]
		for taskID, postState := range pb.TaskStates {
			if postState != "v" {
				continue
			}
			var preState string
			if existedPre {
				preState = pre.Blocks[preIdx].TaskStates[taskID]
			}
			if preState == "v" {
				continue // already verified, not a fresh flip this phase
			}
			if taskHasCommit(root, taskID, mergeBase) {
				continue
			}
			missing = append(missing, evidenceMissing{
				Milestone: pb.ID,
				TaskID:    taskID,
				FromState: preState,
			})
		}
	}
	return missing
}

// findMergeBaseRef returns the best-guess fork point of the current branch.
// Empty string means "no scoping" — fall back to the full log.
func findMergeBaseRef(root string) string {
	for _, candidate := range []string{"main", "master", "origin/main", "origin/master"} {
		cmd := exec.Command("git", "merge-base", "HEAD", candidate)
		cmd.Dir = root
		if out, err := cmd.Output(); err == nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// taskHasCommit reports whether any commit reachable from HEAD names the
// given task ID. When sinceRef is non-empty the search is limited to
// sinceRef..HEAD so older features' commits don't produce false positives.
func taskHasCommit(root, taskID, sinceRef string) bool {
	if taskID == "" {
		return true // nothing to check
	}
	pattern := regexp.MustCompile(`(^|[^A-Za-z0-9-])` + regexp.QuoteMeta(taskID) + `([^A-Za-z0-9-]|$)`)
	args := []string{"log", "--format=%B%x1e"}
	if sinceRef != "" {
		args = append(args, sinceRef+"..HEAD")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// If the git query fails (e.g., shallow clone, bad ref), treat as
		// "evidence present" to avoid false negatives blocking real work.
		return true
	}
	for _, msg := range strings.Split(string(out), "\x1e") {
		if pattern.MatchString(msg) {
			return true
		}
	}
	return false
}

// logEvidenceRevert prints a one-line summary per verify-guard revert batch.
func logEvidenceRevert(feature, milestoneID string, missing []evidenceMissing) {
	prefix := ""
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	var ids []string
	for _, m := range missing {
		ids = append(ids, m.TaskID)
	}
	sort.Strings(ids)
	preview := strings.Join(ids, ", ")
	if len(preview) > 100 {
		preview = preview[:99] + "…"
	}
	fmt.Fprintf(os.Stderr, "%s\033[33m[VERIFY-GUARD]\033[0m reverted %d [v] flip(s) lacking commit evidence — %s\n", prefix, len(missing), preview)
}

// nonEmpty returns fallback when s is empty.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// branchTouchedFiles returns the list of files whose content differs on
// `branch` compared to the merge base with HEAD. Empty slice on error.
func branchTouchedFiles(root, branch string) []string {
	base := "HEAD"
	if mb := findMergeBaseOfBranch(root, branch); mb != "" {
		base = mb
	}
	cmd := exec.Command("git", "diff", "--name-only", base+".."+branch)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return files
}

// findMergeBaseOfBranch returns the merge-base between HEAD and branch.
// Empty string if git fails or no common ancestor.
func findMergeBaseOfBranch(root, branch string) string {
	cmd := exec.Command("git", "merge-base", "HEAD", branch)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// reportMergeOverlap prints a one-block warning listing files that this
// branch touches and that were also touched by siblings merged earlier.
// mergedFiles is the cumulative map from prior iterations.
func reportMergeOverlap(root, branch, msID string, mergedFiles map[string][]string) {
	if len(mergedFiles) == 0 {
		return
	}
	touched := branchTouchedFiles(root, branch)
	if len(touched) == 0 {
		return
	}
	type entry struct {
		File    string
		Sources []string
	}
	var overlaps []entry
	for _, f := range touched {
		if sources, ok := mergedFiles[f]; ok {
			overlaps = append(overlaps, entry{File: f, Sources: sources})
		}
	}
	if len(overlaps) == 0 {
		return
	}
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].File < overlaps[j].File })
	fmt.Fprintf(os.Stderr, "\n  \033[33m⚠ Merge overlap for %s:\033[0m %d file(s) also modified by earlier sibling(s)\n", msID, len(overlaps))
	maxShow := 8
	for i, o := range overlaps {
		if i == maxShow {
			fmt.Fprintf(os.Stderr, "      \033[2m… and %d more\033[0m\n", len(overlaps)-maxShow)
			break
		}
		fmt.Fprintf(os.Stderr, "      %s \033[2m(also in: %s)\033[0m\n", o.File, strings.Join(o.Sources, ", "))
	}
	fmt.Fprintf(os.Stderr, "  \033[2m  Proceeding with default merge strategy — review the resulting commit before pushing.\033[0m\n\n")
}
