package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
