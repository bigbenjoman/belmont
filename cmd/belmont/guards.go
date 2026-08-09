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
	// A repeated `### M<n>:` heading — a duplicate milestone, or a session note
	// written in milestone-header shape — makes every ByID lookup ambiguous.
	// Guarding anyway rewrote the real block with the shadow block's bytes,
	// deleting live task lines from a document the agent had not even changed.
	// Refuse and say why, rather than guess which one was meant.
	if dup := append(append([]string{}, pre.DupIDs...), post.DupIDs...); len(dup) > 0 {
		fmt.Fprintf(os.Stderr,
			"\033[33m⚠ scope guard skipped: PROGRESS.md has more than one `### %s:` heading, so milestone blocks are ambiguous. "+
				"De-duplicate the heading (a session note must not be written in milestone-header shape) and re-run.\033[0m\n",
			dup[0])
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
				if markersDiffer(preState, postState) {
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
				// Must be the SAME boundary parseProgressSnapshot used to build
				// pre.Blocks — this loop bounds the post block that gets replaced
				// by (or skipped in favour of) that pre block. When the two
				// disagree, the replacement is misaligned: the pre block is
				// emitted in full and post's leftover tail is then re-emitted
				// verbatim, so the tail is duplicated and the very flip this
				// guard detected survives on disk. See isSectionBreak / issue #31.
				if isSectionBreak(nxt) {
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
	// Same ambiguity as the scope guard: with a repeated `### M<n>:` heading,
	// pre.ByID resolves to one block arbitrarily, so an already-verified task
	// can read as a fresh un-evidenced flip and get reverted. Refuse instead.
	if len(pre.DupIDs) > 0 || len(post.DupIDs) > 0 {
		fmt.Fprintf(os.Stderr,
			"\033[33m⚠ evidence check skipped: PROGRESS.md has a repeated `### M<n>:` heading, so milestone blocks are ambiguous.\033[0m\n")
		return
	}
	missing := findEvidenceMissingFlips(cfg.Root, pre, post, action.MilestoneID)
	if len(missing) == 0 {
		return
	}
	rebuilt := revertEvidenceMissing(post, missing)
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

// detectOrphanViolations flags task-shaped lines that sit outside any
// milestone, so they cannot be lost in silence.
//
// A `## ` heading at column zero legitimately ends the milestones region, and a
// normal PROGRESS.md has `## Session History` and `## Decisions Log` after it —
// so a task line down there is not automatically wrong. But it is invisible to
// every count, never rendered, and never scheduled, and issue #31 is what that
// costs: 85 of 541 tasks vanished on a real project, including outstanding
// `[ ]` and `[!]` work, with `belmont validate` exiting 0.
//
// Takes raw content rather than parsed milestones for the obvious reason: by
// the time parseMilestones has run, these lines are already gone.
func detectOrphanViolations(slug, progress string) []validationViolation {
	orphans := orphanedTaskLines(progress)
	if len(orphans) == 0 {
		return nil
	}
	var out []validationViolation
	for _, t := range orphans {
		label := t.ID
		if label == "" {
			label = t.Name
		}
		out = append(out, validationViolation{
			Feature:  slug,
			TaskID:   t.ID,
			Rule:     ruleTaskOutsideMilestone,
			Severity: severityWarning,
			Remedy:   remedyNeedsEvidence,
			Message: fmt.Sprintf(
				"task line at PROGRESS.md:%d (%s) sits outside any milestone, so it is counted by nothing and never scheduled. A `## ` heading at column zero ends the milestones region — if this task is real, move it under its `### M<n>:` heading, or indent the heading above it so it reads as part of the preceding task's body.",
				t.Line, label),
		})
	}
	return out
}

// detectViolations is the pure rule engine — no IO. Takes parsed milestones,
// returns a list of findings. Safe for test coverage.
func detectViolations(slug string, milestones []milestone) []validationViolation {
	var out []validationViolation

	// Rule 0: two milestones sharing an ID. Every structure that keys milestones
	// by ID — the scope guard's ByID, the evidence guard, the merge's
	// lastTaskIdx — can only name one of them, so a collision silently resolves
	// to whichever the reader saw last. Both guards now refuse to run on such a
	// file, which makes this the only thing that gets it fixed.
	seenMS := map[string]bool{}
	reportedMS := map[string]bool{}
	for _, m := range milestones {
		if seenMS[m.ID] && !reportedMS[m.ID] {
			reportedMS[m.ID] = true
			out = append(out, validationViolation{
				Feature:       slug,
				Milestone:     m.ID,
				MilestoneName: m.Name,
				Rule:          ruleDuplicateMilestoneID,
				Severity:      severityError,
				Remedy:        remedyNeedsEvidence,
				Message: fmt.Sprintf(
					"milestone %s appears more than once. Milestones are keyed by ID throughout, so a duplicate makes every lookup ambiguous — the scope guard and the evidence guard both refuse to run against it. Renumber one of them, or, if this is a session note, stop writing it in `### M<n>:` heading shape.",
					m.ID),
			})
		}
		seenMS[m.ID] = true
	}

	for _, m := range milestones {
		// Rule 1: milestone name matches a polish/follow-up pattern.
		if polishMilestoneNameRe.MatchString(m.Name) {
			out = append(out, validationViolation{
				Feature:       slug,
				Milestone:     m.ID,
				MilestoneName: m.Name,
				Rule:          rulePolishMilestoneName,
				Severity:      severityError,
				Remedy:        remedyTechPlan,
				Message: fmt.Sprintf(
					"milestone %s %q looks like a polish/follow-up catch-all. Follow-ups belong in their source milestone (the one that discovered them) as new `[ ]` tasks, not a dedicated milestone. Run `/belmont:tech-plan` to restructure.",
					m.ID, m.Name),
			})
		}
		// Rule 2: an unrecognised checkbox marker. Hard violation, because
		// `belmont auto` lints at startup — this is what stops the loop running
		// against a PROGRESS.md it cannot read. See issue #27.
		for _, t := range m.Tasks {
			if t.Status != taskUnknown {
				continue
			}
			label := t.ID
			if label == "" {
				label = t.Name
			}
			out = append(out, validationViolation{
				Feature:       slug,
				Milestone:     m.ID,
				MilestoneName: m.Name,
				TaskID:        t.ID,
				Rule:          ruleUnrecognisedMarker,
				Severity:      severityError,
				Remedy:        remedyNeedsEvidence,
				Message: fmt.Sprintf(
					"unrecognised task marker %q at PROGRESS.md:%d (%s) — expected one of [ ] [>] [x] [v] [!] [-] (letters are case-insensitive). Belmont will not guess a state for it: it is excluded from the counts and never offered as the next task. If the work was deliberately dropped use [-] withdrawn and record why in ## Decisions Log; otherwise set the state the evidence supports.",
					"["+t.Marker+"]", t.Line, label),
			})
		}

		// Rule 3: task IDs reference a milestone other than the one they live in.
		currentNum := milestoneNumber(m.ID)
		for _, t := range m.Tasks {
			_, refNum := taskIDNamedMilestone(t.ID)
			if refNum < 0 {
				continue
			}
			if refNum != currentNum {
				out = append(out, validationViolation{
					Feature:       slug,
					Milestone:     m.ID,
					MilestoneName: m.Name,
					TaskID:        t.ID,
					Rule:          ruleCrossMilestoneTaskID,
					Severity:      severityError,
					Remedy:        remedyTechPlan,
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
			if _, seen := snap.ByID[current.ID]; seen {
				snap.DupIDs = append(snap.DupIDs, current.ID)
			} else {
				snap.ByID[current.ID] = len(snap.Blocks)
			}
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
		if isSectionBreak(line) {
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
// reverts its task line from "[v]" back to its prior state. Uses a line-level
// replacement scoped to each task's milestone block.
//
// The prior state travels on evidenceMissing.FromState, which is why there is
// no `pre` snapshot parameter. There used to be one and it was never read —
// harmless in itself, but it made two tests read as though they exercised a
// pre/post relationship when both were passing the same snapshot twice.
func revertEvidenceMissing(post *progressSnapshot, missing []evidenceMissing) string {
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
			if isSectionBreak(line) {
				currentMS = ""
			}
		}
		if currentMS != "" {
			if overrides, ok := byMS[currentMS]; ok {
				if tm := taskRe.FindStringSubmatch(line); len(tm) >= 6 {
					taskID := tm[4]
					if from, hit := overrides[taskID]; hit && markerIsVerified(tm[2]) {
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

// Finding rules. These are the SAME strings `belmont validate` reports, so the
// two commands name a problem identically: `validate` tells you the file is
// broken, `repair` fixes the thing it named.
const (
	ruleUnrecognisedMarker   = "unrecognised_task_marker"
	ruleTaskOutsideMilestone = "task_outside_milestone"
	ruleCrossMilestoneTaskID = "cross_milestone_task_id"
	rulePolishMilestoneName  = "polish_milestone_name"
	ruleDuplicateMilestoneID = "duplicate_milestone_id"
)

// validationViolation is one finding from `belmont validate`.
type validationViolation struct {
	Feature       string `json:"feature"`
	Milestone     string `json:"milestone"`
	MilestoneName string `json:"milestone_name"`
	TaskID        string `json:"task_id,omitempty"`
	Rule          string `json:"rule"`
	Severity      string `json:"severity"`
	Remedy        string `json:"remedy"`
	Message       string `json:"message"`
}

// Remedies say where the answer comes from, which is not always "ask someone".
//
//   - remedyNeedsEvidence — the correct state is a fact about the repository:
//     whether a commit carries the task ID, whether the code it describes
//     exists, whether a test covers it. NOT a question for a human's memory. A
//     project can carry dozens of these at once, and nobody recalls what a
//     marker meant six weeks ago; asking produces "I don't know" and then a
//     guess, which is issue #27 all over again. `taskHasCommit` is the
//     primitive — it is what runEvidenceCheck already uses to demote a `[v]`
//     nothing committed.
//   - remedyTechPlan — the fix changes milestone structure, which is immutable
//     outside `/belmont:tech-plan` and enforced at runtime: an agent that moves
//     a task between milestones has the edit reverted by runScopeGuard and a
//     correction injected. Route it, do not attempt it.
//
// Deliberately only two values. A "safe to just fix it" class was considered
// and dropped: every remaining case turns on what a marker or a stray line
// MEANT, and Belmont does not guess meanings.
const (
	remedyNeedsEvidence = "needs_evidence"
	remedyTechPlan      = "tech_plan"
)

// progressStructureHelp is printed once beneath any violation report. The
// report used to end by naming `skills/belmont/_partials/milestone-immutability.md`,
// which is a build-time source path that `belmont install` never writes — so in
// every consuming project the one line promising "the canonical rule" was a
// dangling reference. Showing the shape beats pointing at it: this needs no
// file to exist, works identically in both invocation paths, and is what an
// agent needs in order to conform the file.
const progressStructureHelp = `Expected structure:
  ### M<n>: Name           milestone header — level 3, column zero
  - [ ] P<n>-<id>: Task    task line — marker is one of [ ] [>] [x] [v] [!] [-]
                           (todo, in progress, done, verified, blocked,
                            withdrawn); letters are case-insensitive
  ## Anything              a level-2 heading at column zero ENDS the
                           milestones region; task lines below it belong
                           to no milestone and are counted by nothing

Indentation is load-bearing: "  ## Foo" indented under a task is that task's
body, not a heading.

needs_evidence — do not guess and do not rely on memory. Check the repository:
  belmont repair --dry-run     does a commit carry each task, and what else is wrong
  then read the code each task describes, and its tests.
Prefer that over a hand-rolled "git log --grep": --grep is an unanchored
substring match, so P1-1 is credited by a commit for P1-12, and --all searches
every ref including dead branches.
tech_plan — milestone structure is immutable outside /belmont:tech-plan, and
  runScopeGuard reverts an agent that edits it anyway. Run that skill.`

// Violation severities.
//
// The split exists so upgrading Belmont cannot stop a run that worked
// yesterday over something Belmont merely wants to tell you about. Only a file
// Belmont genuinely cannot act on blocks:
//
//   - severityError — the loop cannot proceed correctly. An unrecognised
//     marker makes its milestone permanently un-completable; colliding
//     milestone IDs make every milestone-keyed lookup ambiguous, which turns
//     the runtime guards off. Blocks `belmont auto`, exits 1.
//   - severityWarning — real information that must not be lost, but nothing
//     downstream is wrong. An orphaned task line is counted by nothing, which
//     is worth saying loudly and is not worth refusing to run over: a `- [ ]`
//     bullet in a retro is not work. Printed by `belmont status` and
//     `belmont validate`, exits 0, `belmont auto` continues.
//
// `belmont validate --strict` exits 1 on warnings too, for CI.
const (
	severityError   = "error"
	severityWarning = "warning"
)

// splitBySeverity partitions violations into blocking and advisory. Anything
// without an explicit severity counts as blocking — a new rule that forgets to
// set one fails loudly rather than being silently advisory.
func splitBySeverity(violations []validationViolation) (blocking, advisory []validationViolation) {
	for _, v := range violations {
		if v.Severity == severityWarning {
			advisory = append(advisory, v)
			continue
		}
		blocking = append(blocking, v)
	}
	return blocking, advisory
}

// Matches milestone names that look like polish / follow-up catch-alls.
// Conservative — only fires on unambiguously bad patterns so legitimate
// cross-cutting milestones ("Accessibility audit across routes") aren't
// flagged. Case-insensitive via (?i).
var polishMilestoneNameRe = regexp.MustCompile(`(?i)(\bpolish\b|\bfollow[- ]?ups?\b|\bcleanup\b|\bverification fixes?\b|\bdesign fidelity fixes?\b|\bdeviations from\s+m\d+|from\s+m\d+\s+implementation\b|\bfwlup[s]?\b)`)

// Matches task IDs that embed a milestone number, e.g. P3-FWLUP-M2-1 or
// P1-M4-FIX-2. Capture group 1 is the milestone number referenced by the ID.
var taskIDMilestoneRefRe = regexp.MustCompile(`^P\d+-(?:FWLUP-)?M(\d+)(?:-|$)`)

// taskIDNamedMilestone returns the milestone a task ID names — ("M2", 2) for
// P3-M2-1 — or ("", -1) when it names none.
//
// One definition, because two consumers must agree on it: `belmont validate`
// reports a task filed under the wrong milestone, and `belmont repair` offers
// to move it to the one its ID names. A healer that disagreed with the
// detector about which milestone that is would move tasks somewhere the lint
// then flags again.
func taskIDNamedMilestone(taskID string) (string, int) {
	match := taskIDMilestoneRefRe.FindStringSubmatch(taskID)
	if len(match) < 2 {
		return "", -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return "", -1
	}
	return "M" + match[1], n
}

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
	// Errors and warnings are reported separately, because only one of them
	// stops anything. Printing them in one undifferentiated list is what makes
	// an advisory finding look like a failed run.
	blocking, advisory := splitBySeverity(violations)
	if len(blocking) > 0 {
		fmt.Fprintf(w, "\033[31m✗ %d violation(s) found:\033[0m\n\n", len(blocking))
		renderViolationGroup(w, blocking)
	}
	if len(advisory) > 0 {
		fmt.Fprintf(w, "\033[33m⚠ %d warning(s) — reported, not blocking:\033[0m\n\n", len(advisory))
		renderViolationGroup(w, advisory)
	}
	if len(blocking) == 0 {
		fmt.Fprintln(w, "\033[32m✓ No blocking violations.\033[0m")
	}
	fmt.Fprintln(w, progressStructureHelp)
	fmt.Fprintln(w, "\033[2mRerun after fixing with `belmont validate`.\033[0m")
}

func renderViolationGroup(w io.Writer, violations []validationViolation) {
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
			// A violation can have neither a milestone nor a task ID — an
			// orphaned task line belongs to no milestone by definition. Emit no
			// bracket at all rather than an empty one.
			rule := v.Rule
			if v.Remedy != "" {
				rule = fmt.Sprintf("%s [%s]", v.Rule, v.Remedy)
			}
			switch {
			case v.Milestone != "" && v.TaskID != "":
				fmt.Fprintf(w, "    • [%s/%s] %s — %s\n", v.Milestone, v.TaskID, rule, v.Message)
			case v.Milestone != "":
				fmt.Fprintf(w, "    • [%s] %s — %s\n", v.Milestone, rule, v.Message)
			case v.TaskID != "":
				fmt.Fprintf(w, "    • [%s] %s — %s\n", v.TaskID, rule, v.Message)
			default:
				fmt.Fprintf(w, "    • %s — %s\n", rule, v.Message)
			}
		}
		fmt.Fprintln(w)
	}
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
	// DupIDs lists milestone IDs that appear more than once. ByID can only
	// name one block per ID, so with a collision every lookup resolves to the
	// wrong half of the document and the guard rewrites the wrong lines. The
	// guard refuses to run rather than guess which block was meant.
	DupIDs []string
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
			// Route through canonicalMarker rather than comparing the raw
			// byte. A literal `postState != "v"` here meant any other spelling
			// of verified was invisible to this guard while still counting as
			// verified everywhere else — a silent bypass of the whole
			// evidence contract.
			if !markerIsVerified(postState) {
				continue
			}
			var preState string
			if existedPre {
				preState = pre.Blocks[preIdx].TaskStates[taskID]
			}
			// Routed, not compared. A raw `preState == "v"` here missed a
			// capital V, so an already-verified task read as a fresh flip and
			// this guard reverted it for lacking a commit it never needed.
			if markerIsVerified(preState) {
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
//
// The query itself lives in `lookupCommitEvidence` (repair.go) so the auto
// loop's evidence guard and `belmont repair` cannot drift apart about what
// counts as a commit naming a task. This wrapper adds only the fail-open
// policy, which is the guard's and NOT repair's — see commitEvidence.
//
// FAILS OPEN. A git query that errors (shallow clone, bad ref, no repository)
// returns true, so the guard never blocks real work over plumbing. Any caller
// that must distinguish "no evidence" from "could not look" has to call
// lookupCommitEvidence directly and read Checked — a test that runs this
// against a bare t.TempDir() is asserting nothing at all.
func taskHasCommit(root, taskID, sinceRef string) bool {
	if taskID == "" {
		return true // nothing to check
	}
	ev := lookupCommitEvidence(root, taskID, sinceRef)
	if !ev.Checked {
		return true
	}
	return ev.Found
}

// commitEvidence is the mechanical tier's answer for one task ID.
//
// Checked is not decoration. `taskHasCommit` deliberately FAILS OPEN — a git
// query that errors returns true so a shallow clone cannot block real work in
// the auto loop. Repair must not inherit that: fail-open here would mark every
// unreadable task `[x]` in a directory that is not a git repository at all.
// So repair asks the two-value question and treats "could not check" as "no
// mechanical answer", which sends the finding to the review tier.
type commitEvidence struct {
	Checked bool   `json:"checked"`
	Found   bool   `json:"found"`
	SHA     string `json:"sha,omitempty"`
	Subject string `json:"subject,omitempty"`
	// Reverting is set when the newest commit naming the task is a revert of
	// one. A commit message mentioning a task ID is evidence that SOMETHING
	// happened, not that the work stands — and `Revert "P1-M1-1: add X"` is git
	// saying in its own generated words that it does not. That is the one case
	// where the mechanical tier can tell, so it declines and sends the finding
	// to the review tier instead of writing [x] over work that was taken out.
	//
	// A hand-written "Descope P1-M1-2" is the same hazard and cannot be
	// detected this way. That is why every applied change prints the commit it
	// relied on and nothing is committed for you.
	Reverting bool `json:"reverting,omitempty"`
	// Ambiguous is set when another feature's PROGRESS.md claims the same task
	// ID. Task IDs are feature-local and the commit log is not, so the match
	// may be about entirely different work. See taskIDsClaimedElsewhere.
	Ambiguous bool `json:"ambiguous,omitempty"`
}

// isGitWorkTree reports whether root is inside a git working tree. Repair asks
// before trusting the commit log, because `taskHasCommit` fails open and would
// otherwise report evidence for every task in a directory with no git at all.
func isGitWorkTree(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// lookupCommitEvidence finds the first commit naming taskID, scoped to
// sinceRef..HEAD when sinceRef is non-empty.
//
// This is the one implementation of "does a commit name this task ID?" —
// `taskHasCommit` (the auto loop's evidence guard) routes through it too, so
// the guard and the healer can never disagree about what counts as evidence.
// The word-boundary pattern is what stops P1-1 being credited by a commit for
// P1-12.
func lookupCommitEvidence(root, taskID, sinceRef string) commitEvidence {
	if taskID == "" {
		return commitEvidence{}
	}
	pattern := regexp.MustCompile(`(^|[^A-Za-z0-9-])` + regexp.QuoteMeta(taskID) + `([^A-Za-z0-9-]|$)`)
	args := []string{"log", "--format=%H%x1f%s%x1f%B%x1e"}
	if sinceRef != "" {
		args = append(args, sinceRef+"..HEAD")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return commitEvidence{Checked: false}
	}
	ev := commitEvidence{Checked: true}
	for _, record := range strings.Split(string(out), "\x1e") {
		fields := strings.SplitN(record, "\x1f", 3)
		if len(fields) < 3 {
			continue
		}
		if !pattern.MatchString(fields[2]) {
			continue
		}
		ev.Found = true
		ev.SHA = strings.TrimSpace(strings.TrimLeft(fields[0], "\n"))
		ev.Subject = strings.TrimSpace(fields[1])
		// `git log` walks newest-first, so this is the most recent thing said
		// about the task. If that is a revert, the task's state is not settled.
		ev.Reverting = revertSubjectRe.MatchString(ev.Subject)
		return ev
	}
	return ev
}

// revertSubjectRe matches the subject git generates for `git revert`, in both
// the quoted form and the bare one people write by hand.
var revertSubjectRe = regexp.MustCompile(`^[Rr]evert\b`)

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
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
