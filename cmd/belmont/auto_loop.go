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

	// Identify this run so two runs of the same feature can be compared in
	// .belmont/metrics/<feature>.jsonl (P0-2). Set here rather than by the
	// caller so every entry point — auto, loop, parallel waves — is measured on
	// the same basis; a comparison whose halves were gathered differently proves
	// nothing, which is the same reason P0-12 was sequenced ahead of the baseline.
	if cfg.RunID == "" {
		cfg.RunID = startTime.UTC().Format(time.RFC3339)
	}
	if cfg.CriticalPath == nil {
		if report, err := buildStatus(cfg.Root, 55, cfg.Feature); err == nil {
			cfg.CriticalPath = criticalPathMilestones(report.Milestones)
		}
	}

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
			blockersLeft, skipErr := skipMilestoneInProgress(cfg.Root, cfg.Feature, action.MilestoneID)
			if skipErr != nil {
				fmt.Fprintf(os.Stderr, "\033[31m  ✗ Failed to skip milestone: %s\033[0m\n\n", skipErr)
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m  ✓ Skipped milestone %s\033[0m\n", action.MilestoneID)
				// Say what was left rather than letting the milestone read as
				// finished. A skipped milestone holding a `[!]` is not done —
				// the marker is a question for the user, and nothing here can
				// answer it.
				if blockersLeft > 0 {
					fmt.Fprintf(os.Stderr, "\033[33m    %d blocked task(s) left as [!] — %s is not finished; see belmont blockers --feature %s\033[0m\n",
						blockersLeft, action.MilestoneID, cfg.Feature)
				}
				fmt.Fprintln(os.Stderr)
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
	} else {
		tw = newTailWriter(os.Stderr, 1500, prefix)
	}
	usageOf := attachUsageCapture(cmd, cfg.Tool, tw, prefix)

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

	// Record the phase whether it succeeded or failed. A failed milestone's
	// tokens count against the same denominator — "[x] -> [v] is the unit", and
	// work that had to be redone is exactly the spend this feature is trying to
	// remove.
	recordPhaseMetrics(cfg, action.MilestoneID, string(action.Type), durationMs, usageOf())

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
		// Fallback inline prompt. Must stay in step with
		// prompts/belmont/post-verify-triage.md — it stands in for that
		// template, so a divergence here is a second, quieter triage policy.
		prompt = fmt.Sprintf(`You are a triage agent. Read %s/PROGRESS.md to find incomplete tasks (marked [ ] or [>]).
Classify each into exactly one of three classes, checking human-gated FIRST:
- Human-gated: no agent can close it, because the missing thing is a PERSON — an approval, a product or architecture ruling, a credential or console action nobody automated. Mark it [!] in PROGRESS.md, leave its body and its PRD section where they are, record the ask in ## Decisions Log, and do NOT list it in blocking_tasks or deferred_tasks. Never attempt it: an attempt that cannot succeed is not a fix round.
- Blocking: a real bug, failing test, security issue, or unmet acceptance criterion an agent can actually fix.
- Deferrable: polish.
If every remaining follow-up is deferrable, mark each one [-] withdrawn in PROGRESS.md — do NOT delete the line; a deleted task is carried back in by mergeProgressState from either direction and records no reason — add one ## Decisions Log line each, and move the detail to %s/NOTES.md under ## Polish.
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
	} else {
		tw = newTailWriter(os.Stderr, 1500, triagePrefix)
	}
	usageOf := attachUsageCapture(cmd, cfg.Tool, tw, triagePrefix)

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

	// Triage carries no milestone of its own — it decides what to do about one.
	// Recorded against the feature with an empty milestone so its spend is
	// counted but never attributed to a milestone it did not run.
	recordPhaseMetrics(cfg, "", string(actionTriage), durationMs, usageOf())

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
		return fmt.Sprintf("/belmont:next --feature %s\n\nBATCH MODE: Implement ALL pending FWLUP tasks in %s sequentially. For each task: find it, create MILESTONE file, dispatch to implementation agent, process results, archive MILESTONE, then loop to the next pending FWLUP. Stop when no FWLUP tasks remain in %s.\n\nARCHIVE: Step 5 appends to the milestone's done-file rather than overwriting it, which is what keeps every task's implementation log in a batch. Follow it as written.\n\nIMPORTANT: Only work on FWLUP tasks (tasks with \"FWLUP\" in their ID) that belong to %s. If there are NO pending FWLUP tasks in %s, stop immediately and report \"No FWLUP tasks to fix.\" Do NOT implement regular tasks — those require the full implementation pipeline.", feature, milestoneClause, milestoneClause, milestoneClause, milestoneClause)
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

// attachUsageCapture wires the writers for a tool invocation and returns a
// closure yielding whatever token count the tool reported about itself.
//
// Claude is handled by claudeStreamWriter, which already parses every NDJSON
// line and now keeps the `result` event's usage block. Every other tool writes
// into a tailWriter that keeps only the last 1500 bytes, so its usage event is
// recoverable only by luck; usageCapture retains the usage-bearing line by
// content instead of by position. Tools Belmont cannot read a verified schema
// for get no capture at all and record null with a stated reason — never a guess.
func attachUsageCapture(cmd *exec.Cmd, tool string, tw *tailWriter, prefix string) func() *toolUsage {
	if tool == "claude" {
		csw := &claudeStreamWriter{tw: tw, prefix: prefix}
		cmd.Stdout = csw
		cmd.Stderr = tw
		return csw.Usage
	}
	if tool == "codex" {
		uc := newUsageCapture(tw, codexUsageFromLine)
		cmd.Stdout = uc
		cmd.Stderr = tw
		return uc.Usage
	}
	cmd.Stdout = tw
	cmd.Stderr = tw
	return func() *toolUsage { return nil }
}

// recordPhaseMetrics appends one metrics record for a completed phase.
//
// Best-effort by design: a metrics write must never fail a run that otherwise
// succeeded, and it must never be the reason a milestone is retried. An empty
// RunID disables recording, which is what keeps unit tests and dry runs from
// leaving records behind.
//
// Writes to cfg.metricsRoot(), NOT cfg.Root. Under runAutoParallel, Root is the
// worktree and the worktree is deleted on merge, so records written there are
// destroyed before anything can read them; MetricsRoot is the originating root
// that outlives the wave. See loopConfig.MetricsRoot.
func recordPhaseMetrics(cfg loopConfig, milestoneID, phase string, durationMs int64, usage *toolUsage) {
	if cfg.RunID == "" {
		return
	}
	rec := buildMetricsRecord(cfg.RunID, cfg.Feature, milestoneID, phase, cfg.Tool,
		durationMs, usage, cfg.CriticalPath[milestoneID])
	if err := appendMetricsRecord(cfg.metricsRoot(), rec); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ metrics write failed: %s\033[0m\n", err)
	}
}
