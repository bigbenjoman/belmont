package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func decideLoopAction(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, pendingTasks bool) loopAction {
	last := lastActionType(history)

	// Rule 1: Blocked tasks → PAUSE
	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		return loopAction{Type: actionPause, Reason: fmt.Sprintf("Blocked tasks: %s", strings.Join(blocked, ", "))}
	}

	// Rule 2: Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	// Rule 3: Stuck detection
	if isLoopStuck(history) {
		return loopAction{Type: actionPause, Reason: "Loop appears stuck — no state change after 2 iterations"}
	}

	// Rule 4: FWLUP tasks after VERIFY → TRIAGE
	if hasFwlup && (last == actionVerify || last == actionFixAll) {
		return loopAction{Type: actionTriage, Reason: "Triaging follow-up tasks"}
	}

	// Rule 5: After IMPLEMENT_NEXT or FIX_ALL → VERIFY
	if last == actionImplementNext || last == actionFixAll {
		return loopAction{Type: actionVerify, Reason: "Re-verifying after follow-up fix"}
	}

	// Rule 6: After IMPLEMENT_MILESTONE → VERIFY
	if last == actionImplementMilestone {
		return loopAction{Type: actionVerify, Reason: "Verifying completed milestone"}
	}

	// Rule 7-10: Check milestones in range
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			allDone = false
			break
		}
	}

	// Rule 7: All done + no FWLUP → COMPLETE (but check pending tasks first)
	if allDone && !hasFwlup && !pendingTasks {
		return loopAction{Type: actionComplete, Reason: "All milestones in range completed"}
	}
	if allDone && !hasFwlup && pendingTasks {
		return loopAction{Type: actionImplementNext, Reason: "All milestones marked done but tasks still pending"}
	}

	// Rule 8: All done but FWLUP remaining → TRIAGE
	if allDone && hasFwlup {
		return loopAction{Type: actionTriage, Reason: "Triaging remaining follow-up tasks"}
	}

	// Rule 10: Next milestone in range → IMPLEMENT_MILESTONE
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			return loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Fallback
	if pendingTasks {
		return loopAction{Type: actionImplementNext, Reason: "Tasks still pending — implementing next"}
	}
	return loopAction{Type: actionComplete, Reason: "No actionable milestones found"}
}

// decideLoopActionSmart applies deterministic rules for ~80% of cases.
// Returns nil for ambiguous cases that should fall through to AI.
// pendingInRange and fwlupInRange are scoped to the --from/--to milestone range.
func decideLoopActionSmart(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, hasMsFwlup bool, pendingInRange bool, fwlupInRange bool, msStates map[string]*milestoneLoopState) *loopAction {
	if len(history) == 0 {
		// First iteration: implement first undone milestone in range
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("First iteration — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		// All milestones marked done — but verify against actual task counts (range-scoped)
		if pendingInRange {
			// State drift: milestones marked done but tasks still pending within range
			lastMS := inRange[len(inRange)-1]
			if fwlupInRange {
				return &loopAction{Type: actionFixAll, Reason: fmt.Sprintf("State drift: %s marked complete but FWLUP tasks pending — fixing", lastMS.ID), MilestoneID: lastMS.ID}
			}
			return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("State drift: %s marked complete but tasks still pending — reimplementing", lastMS.ID), MilestoneID: lastMS.ID}
		}
		if !fwlupInRange {
			return &loopAction{Type: actionComplete, Reason: "All milestones in range already complete"}
		}
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionImplementNext, Reason: "All milestones done but follow-up tasks remain in range", MilestoneID: msID}
	}

	last := history[len(history)-1]
	lastType := last.Action.Type
	lastSuccess := last.Result != nil && last.Result.Success

	// Rule 1: After IMPLEMENT_MILESTONE success → almost always VERIFY
	if lastType == actionImplementMilestone && lastSuccess {
		wt := last.WorkType
		fc := last.FilesChanged
		// Skip verify only for: 0 files changed, pure docs, or non-critical config ≤2 files
		if fc == 0 {
			msID := last.Action.MilestoneID
			return &loopAction{Type: actionImplementNext, Reason: "No files changed — skipping verification", MilestoneID: msID}
		}
		if wt == workDocs {
			// Docs-only: skip verification, move to next milestone
			inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
			for _, m := range inRange {
				if !milestoneAllDone(m) {
					return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Docs-only milestone — moving to %s", m.ID), MilestoneID: m.ID}
				}
			}
			if pendingInRange {
				msID := last.Action.MilestoneID
				return &loopAction{Type: actionImplementNext, Reason: "Docs-only done but tasks still pending in range", MilestoneID: msID}
			}
			if !fwlupInRange {
				return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (last was docs-only)"}
			}
			msID := last.Action.MilestoneID
			return &loopAction{Type: actionImplementNext, Reason: "Docs-only milestone done, fixing follow-ups in range", MilestoneID: msID}
		}
		// Everything else: verify
		return &loopAction{Type: actionVerify, Reason: "Verifying completed milestone", MilestoneID: last.Action.MilestoneID}
	}

	// Rule 2: After VERIFY success + no follow-ups in this milestone → next undone milestone or COMPLETE
	if lastType == actionVerify && lastSuccess && !hasMsFwlup {
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Verification passed — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		if pendingInRange {
			msID := lastMilestoneID(history)
			return &loopAction{Type: actionImplementNext, Reason: "All milestones marked done but tasks still pending in range after verification", MilestoneID: msID}
		}
		// All in-range milestones done and verified — complete
		return &loopAction{Type: actionComplete, Reason: "All milestones in range verified and complete"}
	}

	// Rule 3: After VERIFY success + follow-ups exist in this milestone → TRIAGE (AI reads the actual FWLUPs)
	if lastType == actionVerify && lastSuccess && hasMsFwlup {
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionTriage, Reason: "Triaging follow-up tasks after verification", MilestoneID: msID}
	}

	// Rule 3b: After TRIAGE → decide based on triage decision
	if lastType == actionTriage && lastSuccess {
		decision := last.Action.TriageDecision
		switch decision {
		case "defer_and_proceed":
			// Triage already moved FWLUPs to NOTES.md — proceed to next milestone
			inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
			for _, m := range inRange {
				if !milestoneAllDone(m) {
					return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Triage deferred polish items — implementing %s", m.ID), MilestoneID: m.ID}
				}
			}
			if pendingInRange {
				msID := lastMilestoneID(history)
				return &loopAction{Type: actionImplementNext, Reason: "Triage deferred polish but tasks still pending in range", MilestoneID: msID}
			}
			return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (remaining items deferred as polish)"}
		case "fix_and_proceed":
			return &loopAction{Type: actionFixAll, Reason: "Fixing all blocking follow-ups (will skip re-verification)", TriageDecision: "fix_and_proceed", MilestoneID: last.Action.MilestoneID}
		case "fix_and_reverify":
			return &loopAction{Type: actionFixAll, Reason: "Fixing all blocking follow-ups (will re-verify after)", TriageDecision: "fix_and_reverify", ReverifyScope: last.Action.ReverifyScope, MilestoneID: last.Action.MilestoneID}
		default:
			// Unknown triage decision — fall back to fix-all with re-verify
			return &loopAction{Type: actionFixAll, Reason: "Fixing follow-ups after triage", TriageDecision: "fix_and_reverify", ReverifyScope: "focused", MilestoneID: last.Action.MilestoneID}
		}
	}

	// Rule 3c: After FIX_ALL success → check triage decision for next step
	if lastType == actionFixAll && lastSuccess {
		decision := last.Action.TriageDecision
		if decision == "fix_and_reverify" {
			scope := last.Action.ReverifyScope
			if scope == "" {
				scope = "focused"
			}
			return &loopAction{Type: actionVerify, Reason: fmt.Sprintf("Re-verifying after follow-up fixes (scope: %s)", scope), ReverifyScope: scope, MilestoneID: last.Action.MilestoneID}
		}
		// fix_and_proceed: skip re-verification, move to next milestone
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Follow-ups fixed — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		if pendingInRange {
			msID := lastMilestoneID(history)
			return &loopAction{Type: actionImplementNext, Reason: "Follow-ups fixed but tasks still pending in range", MilestoneID: msID}
		}
		if !fwlupInRange {
			return &loopAction{Type: actionComplete, Reason: "All milestones in range complete after follow-up fixes"}
		}
		return &loopAction{Type: actionTriage, Reason: "Follow-ups remain in range after fix-all — re-triaging"}
	}

	// Rule 4: After IMPLEMENT_NEXT success → verify only if milestone hasn't been verified yet
	if lastType == actionImplementNext && lastSuccess {
		msID := lastMilestoneID(history)
		// If this milestone already had a clean verify pass, skip re-verification
		if msID != "" {
			if s, ok := msStates[msID]; ok && s.VerifySucceeded >= 1 {
				// Move to next undone milestone or complete
				inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
				for _, m := range inRange {
					if !milestoneAllDone(m) {
						return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Milestone %s already verified — moving to %s", msID, m.ID), MilestoneID: m.ID}
					}
				}
				if !pendingInRange && !fwlupInRange {
					return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (already verified)"}
				}
				// Still pending in range — continue fixing within scope
				return &loopAction{Type: actionImplementNext, Reason: "Fixing remaining in-range tasks", MilestoneID: msID}
			}
		}
		return &loopAction{Type: actionVerify, Reason: "Verifying after follow-up fix", MilestoneID: msID}
	}

	// Rule 5: After VERIFY failure — check verify failure count
	if lastType == actionVerify && !lastSuccess {
		// Find the milestone being verified
		var verifyFailCount int
		var targetMS string
		// Walk history backward to find which milestone we're verifying
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
				targetMS = history[i].Action.MilestoneID
				break
			}
		}
		if targetMS != "" {
			if s, ok := msStates[targetMS]; ok {
				verifyFailCount = s.VerifyFailed
			}
		}

		if verifyFailCount >= 2 {
			// Delegate to AI for REPLAN/DEBUG decision
			return nil
		}
		// First failure: try fixing (scoped to the target milestone)
		return &loopAction{Type: actionImplementNext, Reason: "Verification failed — fixing issues", MilestoneID: targetMS}
	}

	// Rule 6: After DEBUG success → VERIFY
	if lastType == actionDebug && lastSuccess {
		return &loopAction{Type: actionVerify, Reason: "Re-verifying after debug"}
	}

	// Rule 7: All milestones done + verified + no follow-ups in range → COMPLETE
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	allVerified := true
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			allDone = false
			break
		}
		if s, ok := msStates[m.ID]; ok {
			if s.VerifySucceeded == 0 {
				allVerified = false
			}
		}
	}
	if allDone && allVerified && !fwlupInRange && !pendingInRange {
		return &loopAction{Type: actionComplete, Reason: "All milestones in range implemented, verified, and no follow-ups"}
	}
	if allDone && allVerified && !fwlupInRange && pendingInRange {
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionImplementNext, Reason: "All milestones in range marked done and verified but tasks still pending", MilestoneID: msID}
	}

	// Rule 8: All done but not all verified → VERIFY (only milestones never successfully verified)
	if allDone && !allVerified && !fwlupInRange {
		for _, m := range inRange {
			if s, ok := msStates[m.ID]; ok && s.VerifySucceeded == 0 {
				return &loopAction{Type: actionVerify, Reason: fmt.Sprintf("Verifying %s (not yet verified)", m.ID), MilestoneID: m.ID}
			}
		}
		// All have been verified at least once — complete
		return &loopAction{Type: actionComplete, Reason: "All milestones in range verified at least once"}
	}

	// Rule 9: All done but follow-ups remain in range → TRIAGE
	if allDone && fwlupInRange {
		return &loopAction{Type: actionTriage, Reason: "Triaging remaining follow-up tasks in range"}
	}

	// Rule 10: Next undone milestone
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Ambiguous — let AI decide
	return nil
}

// checkHardGuardrails runs safety checks that always apply before AI decisions.
// Returns nil if no guardrail triggers, otherwise a loopAction to take.
func checkHardGuardrails(report statusReport, history []historyEntry, cfg loopConfig) *loopAction {
	// Blocked tasks → PAUSE
	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		return &loopAction{Type: actionPause, Reason: fmt.Sprintf("Blocked tasks: %s", strings.Join(blocked, ", "))}
	}

	// Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return &loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	return nil
}

// decideLoopActionAI shells out to the configured tool to make a strategic decision.
// Only called for ambiguous cases that decideLoopActionSmart couldn't handle.
func decideLoopActionAI(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, lastOutput string, msStates map[string]*milestoneLoopState) (*loopAction, error) {
	// Build rich milestone state JSON
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	type msStateJSON struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Done            bool   `json:"done"`
		Implemented     bool   `json:"implemented"`
		Verified        bool   `json:"verified"`
		VerifyFailures  int    `json:"verify_failures,omitempty"`
		VerifySuccesses int    `json:"verify_successes,omitempty"`
		WorkType        string `json:"work_type,omitempty"`
		FilesChanged    int    `json:"files_changed,omitempty"`
	}
	var milestones []msStateJSON
	for _, m := range inRange {
		ms := msStateJSON{ID: m.ID, Name: m.Name, Done: milestoneAllDone(m)}
		if s, ok := msStates[m.ID]; ok {
			ms.Implemented = s.Implemented
			ms.Verified = s.Verified
			ms.VerifyFailures = s.VerifyFailed
			ms.VerifySuccesses = s.VerifySucceeded
			ms.WorkType = string(s.WorkType)
			ms.FilesChanged = s.FilesChanged
		}
		milestones = append(milestones, ms)
	}

	// Build recent history (last 5)
	type histItem struct {
		Action    string `json:"action"`
		Milestone string `json:"milestone,omitempty"`
		Success   bool   `json:"success"`
		WorkType  string `json:"work_type,omitempty"`
		Output    string `json:"output,omitempty"`
	}
	var recentHistory []histItem
	start := len(history) - 5
	if start < 0 {
		start = 0
	}
	for _, h := range history[start:] {
		item := histItem{
			Action:    string(h.Action.Type),
			Milestone: h.Action.MilestoneID,
			WorkType:  string(h.WorkType),
		}
		if h.Result != nil {
			item.Success = h.Result.Success
			item.Output = truncateTail(h.Result.Output, 500)
		}
		recentHistory = append(recentHistory, item)
	}

	// Determine why we're asking the AI (ambiguity reason)
	ambiguityReason := "Smart rules could not determine the next action"
	if len(history) > 0 {
		last := history[len(history)-1]
		if last.Action.Type == actionVerify && last.Result != nil && !last.Result.Success {
			// Find which milestone
			var targetMS string
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
					targetMS = history[i].Action.MilestoneID
					break
				}
			}
			if targetMS != "" {
				if s, ok := msStates[targetMS]; ok && s.VerifyFailed >= 2 {
					ambiguityReason = fmt.Sprintf("%s failed verification %d times — consider REPLAN or DEBUG", targetMS, s.VerifyFailed)
				}
			}
		}
	}

	state := map[string]interface{}{
		"tasks_done":       report.TaskCounts["done"] + report.TaskCounts["verified"],
		"tasks_total":      report.TaskCounts["total"],
		"milestone_states": milestones,
		"has_followup":     hasFwlup,
		"blocker_count":    blockedTaskCount(report.Milestones),
		"last_5_actions":   recentHistory,
		"ambiguity_reason": ambiguityReason,
	}
	if cfg.From != "" {
		state["milestone_from"] = cfg.From
	}
	if cfg.To != "" {
		state["milestone_to"] = cfg.To
	}
	if lastOutput != "" {
		state["previous_output"] = truncateTail(lastOutput, 1500)
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	// Load prompt template
	tmpl, tmplErr := loadPromptTemplate("ai-decision")
	var prompt string
	if tmplErr != nil {
		// Fallback to inline prompt if template not found
		prompt = fmt.Sprintf(`You are a loop controller for an automated feature implementation system.
You are ONLY called for ambiguous cases — simple decisions are already handled by deterministic rules.

STATE:
%s

AVAILABLE ACTIONS:
- IMPLEMENT_MILESTONE: Implement next incomplete milestone (set milestone_id)
- IMPLEMENT_NEXT: Fix a single follow-up task
- VERIFY: Run verification on completed milestones
- TRIAGE: Run AI triage to classify follow-up tasks as blocking vs polish
- FIX_ALL: Fix all blocking follow-up tasks in batch before re-verification
- REPLAN: Re-run tech planning when current approach has systemic issues
- DEBUG: Run automated debugging when verification keeps failing on recurring issues
- SKIP_MILESTONE: Skip a blocked milestone (set milestone_id)
- COMPLETE: All work in scope is done and verified
- PAUSE: Stop for human intervention

HARD RULES:
1. You are ONLY called for ambiguous cases — simple decisions are already handled.
2. Never skip verification for frontend/UI milestones.
3. If verification failed 2+ times on the SAME issue, choose REPLAN or DEBUG.
4. If verification failed on DIFFERENT issues each time, one more VERIFY is reasonable.
5. If a milestone has recurring failures across multiple cycles, use DEBUG.
6. Use SKIP_MILESTONE only when a milestone truly cannot proceed due to external blockers.
7. If all milestones in range are done+verified with no follow-ups, COMPLETE.
8. Prefer TRIAGE over IMPLEMENT_NEXT when follow-ups exist — let triage classify issues before fixing.

Respond with ONLY valid JSON: {"action":"...","reason":"...","milestone_id":"..."}`, string(stateJSON))
	} else {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{"StateJSON": string(stateJSON)}); err != nil {
			return nil, fmt.Errorf("execute prompt template: %w", err)
		}
		prompt = buf.String()
	}

	// AI decision calls are short classification tasks — use the low tier.
	decisionFlags := resolveModelFlags(cfg.Tool, "low", cfg.Root)
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, decisionFlags...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tool execution: %w (output: %s)", err, truncateTail(string(output), 200))
	}

	decisionJSON, err := extractDecisionJSON(string(output), cfg.Tool)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var decision aiDecision
	if err := json.Unmarshal([]byte(decisionJSON), &decision); err != nil {
		return nil, fmt.Errorf("unmarshal decision: %w", err)
	}

	// Validate action type
	actionType := loopActionType(decision.Action)
	switch actionType {
	case actionImplementMilestone, actionImplementNext, actionVerify,
		actionReplan, actionSkipMilestone, actionComplete, actionPause, actionDebug,
		actionTriage, actionFixAll:
		// valid
	default:
		return nil, fmt.Errorf("unknown action %q from AI", decision.Action)
	}

	// Validate milestone_id required for certain actions
	if (actionType == actionImplementMilestone || actionType == actionSkipMilestone) && decision.MilestoneID == "" {
		return nil, fmt.Errorf("action %s requires milestone_id", actionType)
	}

	return &loopAction{
		Type:        actionType,
		Reason:      decision.Reason,
		MilestoneID: decision.MilestoneID,
	}, nil
}
