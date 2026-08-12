package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func runLoop(cfg loopConfig) error {
	startTime := time.Now()
	var history []historyEntry
	var lastOutput string

	// Write auto.json for status visibility when running standalone (not from parallel mode).
	// In parallel mode, the worktreeTracker manages auto.json separately.
	var autoCleanup func()
	if cfg.Port == 0 {
		autoPath := filepath.Join(cfg.Root, ".belmont", "auto.json")
		writeLoopAutoJSON(autoPath, cfg)
		autoCleanup = func() { os.Remove(autoPath) }
	}
	if autoCleanup != nil {
		defer autoCleanup()
	}

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s | Max iterations: %d\033[0m\n", cfg.Tool, cfg.Policy, cfg.MaxIterations)
	if cfg.From != "" || cfg.To != "" {
		fromStr := cfg.From
		if fromStr == "" {
			fromStr = "start"
		}
		toStr := cfg.To
		if toStr == "" {
			toStr = "end"
		}
		fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", fromStr, toStr)
	}
	fmt.Fprintln(os.Stderr)

	for i := 1; i <= cfg.MaxIterations; i++ {
		// 1. Read current state
		report, err := buildStatus(cfg.Root, 55, cfg.Feature)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mFailed to read state: %s\033[0m\n", err)
			return fmt.Errorf("auto: state read failed: %w", err)
		}

		// 2. Derive extra signals
		hasFwlup := detectFwlupTasks(cfg.Root, cfg.Feature, report)
		currentMsID := lastMilestoneID(history)
		hasMsFwlup := currentMsID != "" && detectFwlupTasksForMilestone(cfg.Root, cfg.Feature, report, currentMsID)
		msStates := buildMilestoneLoopStates(history, report.Milestones)

		// Range-scoped signals: only consider tasks/FWLUPs under milestones within --from/--to
		pendingInRange := pendingTasksInRange(cfg.Root, cfg.Feature, cfg.From, cfg.To)
		fwlupInRange := fwlupTasksInRange(cfg.Root, cfg.Feature, report, cfg.From, cfg.To)

		// Print state summary
		printLoopState(report, hasFwlup)

		// 3. Check hard guardrails first
		action := checkHardGuardrails(report, history, cfg)

		// 4. If no guardrail triggered, try smart rules first
		if action == nil {
			action = decideLoopActionSmart(report, history, cfg, hasFwlup, hasMsFwlup, pendingInRange, fwlupInRange, msStates)
		}

		// 4b. If smart rules returned nil, check stuck detection before AI
		if action == nil && isLoopStuck(history) {
			action = &loopAction{Type: actionPause, Reason: fmt.Sprintf("Loop appears stuck — no state change after 2 iterations (last action: %s)", history[len(history)-1].Action.Type)}
		}

		// 5. If smart rules returned nil, use AI decisions (with rules fallback)
		if action == nil {
			aiAction, err := decideLoopActionAI(report, history, cfg, hasFwlup, lastOutput, msStates)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m  AI decision failed: %s — falling back to rules\033[0m\n", err)
				decided := decideLoopAction(report, history, cfg, fwlupInRange, pendingInRange)
				action = &decided
			} else {
				action = aiAction
			}
		}

		label := describeMilestone(action, report)
		actionLabel := shortActionLabel(action.Type)
		if label != "" {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━ %s › %s ━━\033[0m\n", i, actionLabel, cfg.Feature, label)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━ %s ━━\033[0m\n", i, actionLabel, cfg.Feature)
		}
		fmt.Fprintf(os.Stderr, "\033[2m  %s\033[0m\n\n", action.Reason)

		// 5. Terminal actions
		if action.Type == actionComplete {
			fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — %s (%.1fs total)\n", action.Reason, time.Since(startTime).Seconds())
			return nil
		}
		if action.Type == actionError {
			fmt.Fprintf(os.Stderr, "\n\033[31m✗ Error\033[0m — %s\n", action.Reason)
			return fmt.Errorf("auto: %s", action.Reason)
		}
		if action.Type == actionPause {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Paused\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return errFeaturePaused
		}

		// 6. Handle SKIP_MILESTONE (state mutation, no tool call)
		if action.Type == actionSkipMilestone {
			skipErr := skipMilestoneInProgress(cfg.Root, cfg.Feature, action.MilestoneID)
			if skipErr != nil {
				fmt.Fprintf(os.Stderr, "\033[31m  ✗ Failed to skip milestone: %s\033[0m\n\n", skipErr)
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m  ✓ Skipped milestone %s\033[0m\n\n", action.MilestoneID)
			}
			entry := historyEntry{
				Action:       *action,
				Result:       &executionResult{Success: skipErr == nil, DurationMs: 0},
				TasksDone:    report.TaskCounts["done"] + report.TaskCounts["verified"],
				TasksTotal:   report.TaskCounts["total"],
				MsDone:       countDoneMilestones(report.Milestones),
				MsTotal:      len(report.Milestones),
				BlockerCount: blockedTaskCount(report.Milestones),
				HasFwlup:     hasFwlup,
				Iteration:    i,
			}
			history = append(history, entry)
			continue
		}

		// 7. Checkpoint policy check
		if shouldLoopCheckpoint(*action, cfg.Policy, lastActionType(history)) {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Checkpoint\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return nil
		}

		// 8. Capture pre-action SHA
		preSHA := captureGitSHA(cfg.Root)

		// 9. Execute action
		result := executeLoopAction(*action, cfg)
		lastOutput = truncateTail(result.Output, 1500)

		// 9b. Parse triage decision from output
		if action.Type == actionTriage && result.Success {
			if td := parseTriageDecision(result.Output); td != nil {
				action.TriageDecision = td.Decision
				action.ReverifyScope = td.ReverifyScope
				fmt.Fprintf(os.Stderr, "\033[2m  Triage: %s (%s)\033[0m\n", td.Decision, td.Reason)
			} else {
				fmt.Fprintf(os.Stderr, "\033[33m  Warning: could not parse triage decision from output — deferring\033[0m\n")
				// Default to defer_and_proceed if parsing fails — avoids expensive fix-all + re-verify loops
				action.TriageDecision = "defer_and_proceed"
				action.ReverifyScope = ""
			}
		}

		// 9c. Propagate triage decision to FIX_ALL action for downstream rules
		if action.Type == actionFixAll && action.TriageDecision == "" {
			// Look back in history for the most recent triage decision
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Action.Type == actionTriage {
					action.TriageDecision = history[i].Action.TriageDecision
					action.ReverifyScope = history[i].Action.ReverifyScope
					break
				}
			}
		}

		// 10. Post-action classification
		postSHA := captureGitSHA(cfg.Root)
		wt, fc := classifyChanges(cfg.Root, preSHA)

		// 11. Record in history
		entry := historyEntry{
			Action:       *action,
			Result:       &result,
			TasksDone:    report.TaskCounts["done"] + report.TaskCounts["verified"],
			TasksTotal:   report.TaskCounts["total"],
			MsDone:       countDoneMilestones(report.Milestones),
			MsTotal:      len(report.Milestones),
			BlockerCount: blockedTaskCount(report.Milestones),
			HasFwlup:     hasFwlup,
			Iteration:    i,
			WorkType:     wt,
			FilesChanged: fc,
			GitSHA:       preSHA,
			PostGitSHA:   postSHA,
		}
		history = append(history, entry)

		// 12. Print result
		if result.Success {
			fmt.Fprintf(os.Stderr, "\n\033[32m  ✓ %.1fs\033[0m\n", float64(result.DurationMs)/1000)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[31m  ✗ %s (%.1fs)\033[0m\n", result.Error, float64(result.DurationMs)/1000)
		}
	}

	fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Max iterations reached (%d)\033[0m\n", cfg.MaxIterations)
	return nil
}

func executeLoopAction(action loopAction, cfg loopConfig) executionResult {
	prompt := adaptPromptForTool(buildLoopPrompt(action, cfg.Feature), cfg.Tool)

	// Snapshot PROGRESS.md before the agent runs — the post-phase scope guard
	// uses this baseline to revert out-of-scope milestone structure changes.
	preSnap := snapshotProgress(cfg.Root, cfg.Feature)

	// Consume any pending user steering for this milestone. Runs before any
	// shell-out so a single injection maps to one agent run — the auto loop
	// re-enters the same milestone across phases and we don't want the same
	// instruction to fire repeatedly.
	steeringBlock, steeringCount := consumePendingSteering(cfg.Root, cfg.Feature, action.MilestoneID, string(action.Type))
	if steeringCount > 0 {
		logSteeringInjection(cfg.Feature, action.MilestoneID, steeringCount, steeringBlock)
	}

	// Triage uses its own prompt template
	if action.Type == actionTriage {
		result := executeTriageAction(cfg, steeringBlock)
		runScopeGuard(cfg, action, preSnap)
		runEvidenceCheck(cfg, action, preSnap)
		return result
	}

	if steeringCount > 0 {
		prompt = steeringBlock + prompt
	}

	modelFlags := resolveModelFlags(cfg.Tool, tierForAction(action.Type, cfg.ModelTiers), cfg.Root)

	args := toolHeadlessArgs(cfg.Tool, prompt, cfg.Root, modelFlags, true)
	if args == nil {
		return executionResult{Success: false, Error: fmt.Sprintf("unsupported tool: %s", cfg.Tool)}
	}
	cmd := exec.Command(toolBinary(cfg.Tool), args...)
	cmd.Dir = cfg.Root

	// Worktree isolation: inject env vars and set process group
	if cfg.Port != 0 {
		cmd.Env = buildWorktreeEnv(cfg.Port, cfg.WorktreeEnv, cfg.Workspaces, cfg.PrimaryWorkspace, cfg.MonorepoType)
		setSysProcAttr(cmd)
	}

	var prefix string
	if cfg.Feature != "" {
		if action.MilestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", cfg.Feature, action.MilestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", cfg.Feature)
		}
	}

	var tw *tailWriter
	if cfg.Tool == "claude" {
		tw = newTailWriter(os.Stderr, 1500, "")
		cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: prefix}
		cmd.Stderr = tw
	} else {
		tw = newTailWriter(os.Stderr, 1500, prefix)
		cmd.Stdout = tw
		cmd.Stderr = tw
	}

	var stopTimer chan struct{}
	if cfg.Tool != "claude" {
		stopTimer = make(chan struct{})
		go func() {
			start := time.Now()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m  ⏱ %s\033[0m", time.Since(start).Truncate(time.Second))
				case <-stopTimer:
					fmt.Fprintf(os.Stderr, "\r\033[K")
					return
				}
			}
		}()
	}

	start := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		if stopTimer != nil {
			close(stopTimer)
		}
		return executionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to start: %s", startErr),
		}
	}

	// Track the PGID so Ctrl-C cleanup works via worktreeTracker
	pid := cmd.Process.Pid
	if cfg.Tracker != nil && cfg.TrackerID != "" {
		cfg.Tracker.setPgid(cfg.TrackerID, pid)
	}

	err := cmd.Wait()
	if stopTimer != nil {
		close(stopTimer)
	}

	// Kill the entire process group to clean up orphaned child processes
	// (dev servers, test runners, etc.) that survive the AI tool exiting.
	if cfg.Port != 0 {
		killProcessGroup(pid)
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		runScopeGuard(cfg, action, preSnap)
		runEvidenceCheck(cfg, action, preSnap)
		return executionResult{
			Success:    false,
			Output:     tw.String(),
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	runScopeGuard(cfg, action, preSnap)
	runEvidenceCheck(cfg, action, preSnap)
	return executionResult{
		Success:    true,
		Output:     tw.String(),
		DurationMs: durationMs,
	}
}

// executeTriageAction runs the triage prompt template as a full tool-equipped invocation.
// steeringPrefix, if non-empty, is prepended to the prompt before the template body.
func executeTriageAction(cfg loopConfig, steeringPrefix string) executionResult {
	featureBase := filepath.Join(".belmont", "features", cfg.Feature)

	// Determine fix round from milestone states
	fixRound := 0
	// We'll pass 0 and let the template handle it; the prompt template itself
	// contains circuit breaker logic based on this value

	// Load the triage prompt template
	tmpl, tmplErr := loadPromptTemplate("post-verify-triage")
	var prompt string
	if tmplErr != nil {
		// Fallback inline prompt
		prompt = fmt.Sprintf(`You are a triage agent. Read %s/PROGRESS.md to find incomplete tasks (marked [ ] or [>]).
Classify each as blocking (real bug) or deferrable (polish).
If all are deferrable, move them to %s/NOTES.md under ## Polish and remove from PROGRESS.md.
Output JSON: {"decision":"defer_and_proceed|fix_and_proceed|fix_and_reverify","blocking_tasks":[],"deferred_tasks":[],"reason":"...","reverify_scope":"focused"}`, featureBase, featureBase)
	} else {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{
			"Feature":      cfg.Feature,
			"FeatureBase":  featureBase,
			"FixRound":     fmt.Sprintf("%d", fixRound),
			"VerifyOutput": "", // verify output is available from lastOutput in the loop
		}); err != nil {
			return executionResult{Success: false, Error: fmt.Sprintf("execute triage template: %s", err)}
		}
		prompt = buf.String()
	}

	if steeringPrefix != "" {
		prompt = steeringPrefix + prompt
	}

	triageFlags := resolveModelFlags(cfg.Tool, tierForAction(actionTriage, cfg.ModelTiers), cfg.Root)
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, triageFlags...)

	// Worktree isolation: inject env vars and set process group
	if cfg.Port != 0 {
		cmd.Env = buildWorktreeEnv(cfg.Port, cfg.WorktreeEnv, cfg.Workspaces, cfg.PrimaryWorkspace, cfg.MonorepoType)
		setSysProcAttr(cmd)
	}

	var triagePrefix string
	if cfg.Port != 0 && cfg.Feature != "" {
		triagePrefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", cfg.Feature)
	}

	var tw *tailWriter
	if cfg.Tool == "claude" {
		tw = newTailWriter(os.Stderr, 1500, "")
		cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: triagePrefix}
		cmd.Stderr = tw
	} else {
		tw = newTailWriter(os.Stderr, 1500, triagePrefix)
		cmd.Stdout = tw
		cmd.Stderr = tw
	}

	var stopTimer chan struct{}
	if cfg.Tool != "claude" {
		stopTimer = make(chan struct{})
		go func() {
			start := time.Now()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m  ⏱ %s\033[0m", time.Since(start).Truncate(time.Second))
				case <-stopTimer:
					fmt.Fprintf(os.Stderr, "\r\033[K")
					return
				}
			}
		}()
	}

	start := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		if stopTimer != nil {
			close(stopTimer)
		}
		return executionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to start: %s", startErr),
		}
	}

	pid := cmd.Process.Pid
	if cfg.Tracker != nil && cfg.TrackerID != "" {
		cfg.Tracker.setPgid(cfg.TrackerID, pid)
	}

	err := cmd.Wait()
	if stopTimer != nil {
		close(stopTimer)
	}

	// Kill orphaned child processes (dev servers, etc.)
	if cfg.Port != 0 {
		killProcessGroup(pid)
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return executionResult{
			Success:    false,
			Output:     tw.String(),
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	return executionResult{
		Success:    true,
		Output:     tw.String(),
		DurationMs: durationMs,
	}
}

func buildLoopPrompt(action loopAction, feature string) string {
	switch action.Type {
	case actionImplementMilestone:
		prompt := fmt.Sprintf("/belmont:implement --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED IMPLEMENTATION: Only implement tasks in milestone %s. Do NOT touch tasks in other milestones — they are either already complete, in progress elsewhere, or intentionally queued for later. Do NOT flip task checkboxes, add/remove tasks, or edit notes for any milestone other than %s.\n\nCRITICAL: In PROGRESS.md only the heading and tasks for %s may change. Other milestones may be intentionally incomplete (parallel worktree, queued re-verify, blocked). Treat their state as read-only context.", action.MilestoneID, action.MilestoneID, action.MilestoneID)
		}
		return prompt
	case actionImplementNext:
		prompt := fmt.Sprintf("/belmont:next --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nSCOPE: Only work on tasks within milestone %s. Do NOT implement tasks from other milestones.", action.MilestoneID)
		}
		return prompt
	case actionVerify:
		prompt := fmt.Sprintf("/belmont:verify --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED VERIFICATION: Only verify tasks in milestone %s. Do NOT verify tasks from other milestones — those were verified previously. Focus on: (1) the tasks in %s meet their acceptance criteria, (2) build passes, (3) tests pass.\n\nCRITICAL: Do NOT modify the status of ANY other milestone in PROGRESS.md. Only update the heading for %s. Other milestones may be intentionally incomplete (queued for re-verification) — do NOT change their task states.", action.MilestoneID, action.MilestoneID, action.MilestoneID)
		}
		if action.ReverifyScope == "focused" {
			prompt += "\n\nFOCUSED RE-VERIFICATION: This is a re-verify after follow-up fixes. Only verify: (1) the specific FWLUP tasks that were just fixed, (2) build/test pass, (3) any previously-failing acceptance criteria. Do NOT re-run Lighthouse. Do NOT re-check visual specs unless a FWLUP specifically addressed UI. Do NOT create new Polish-level issues."
		}
		return prompt
	case actionReplan:
		return fmt.Sprintf("/belmont:tech-plan --feature %s", feature)
	case actionDebug:
		return fmt.Sprintf("/belmont:debug-auto --feature %s", feature)
	case actionFixAll:
		milestoneClause := "the current milestone"
		if action.MilestoneID != "" {
			milestoneClause = action.MilestoneID
		}
		return fmt.Sprintf("/belmont:next --feature %s\n\nBATCH MODE: Implement ALL pending FWLUP tasks in %s sequentially. For each task: find it, create MILESTONE file, dispatch to implementation agent, process results, archive MILESTONE, then loop to the next pending FWLUP. Stop when no FWLUP tasks remain in %s.\n\nARCHIVE CAVEAT: your own Step 5 says to OVERWRITE the archived MILESTONE done-file when one of that name already exists. That rule assumes one task per invocation; in BATCH MODE it would destroy every implementation log but the last. APPEND each task's log to that file instead.\n\nIMPORTANT: Only work on FWLUP tasks (tasks with \"FWLUP\" in their ID) that belong to %s. If there are NO pending FWLUP tasks in %s, stop immediately and report \"No FWLUP tasks to fix.\" Do NOT implement regular tasks — those require the full implementation pipeline.", feature, milestoneClause, milestoneClause, milestoneClause, milestoneClause)
	case actionTriage:
		// Triage uses its own prompt template — handled in executeLoopAction
		return ""
	default:
		return ""
	}
}

func shouldLoopCheckpoint(action loopAction, policy checkpointPolicy, last loopActionType) bool {
	// Terminal actions handled elsewhere
	if action.Type == actionPause || action.Type == actionError || action.Type == actionComplete {
		return false
	}

	switch policy {
	case policyAutonomous:
		return false
	case policyMilestone:
		if action.Type == actionImplementMilestone {
			return true
		}
		// Significant changes warrant a checkpoint
		if action.Type == actionReplan || action.Type == actionSkipMilestone {
			return true
		}
		// Auto-verify after implement
		if action.Type == actionVerify && last == actionImplementMilestone {
			return false
		}
		// Pause after verify results
		if last == actionVerify {
			return true
		}
		return false
	case policyEveryAction:
		return true
	}
	return false
}
