package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
