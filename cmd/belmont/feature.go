package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// listFeaturesWithOverrides is like listFeatures but allows worktree path overrides.
// The overrides map slug → worktree feature path for features with active worktrees.
func listFeaturesWithOverrides(featuresDir string, maxName int, worktreeOverrides map[string]string) []featureSummary {
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil
	}
	var features []featureSummary
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		slug := entry.Name()
		featurePath := filepath.Join(featuresDir, slug)
		// If there's an active worktree for this feature, read state from there instead
		if override, ok := worktreeOverrides[slug]; ok {
			featurePath = override
		}
		prdPath := filepath.Join(featurePath, "PRD.md")
		archivePath := filepath.Join(featurePath, "ARCHIVE.md")

		name := slug
		if prdContent, err := os.ReadFile(prdPath); err == nil {
			extracted := extractFeatureName(string(prdContent))
			if extracted != "Unknown" {
				name = extracted
			}
		} else if archiveContent, err := os.ReadFile(archivePath); err == nil {
			// Archived feature: PRD.md is gone; recover the name from the
			// "# Archive: <name>" header written by /belmont:cleanup.
			if extracted := extractArchiveName(string(archiveContent)); extracted != "" {
				name = extracted
			}
		}

		// Read all state from PROGRESS.md
		var milestones []milestone
		var orphaned int
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		if progressContent, err := os.ReadFile(progressPath); err == nil {
			milestones = parseMilestones(string(progressContent))
			orphaned = len(orphanedTaskLines(string(progressContent)))
		}

		tasks := flattenTasks(milestones, maxName)
		tasksTotal := len(tasks)
		tasksDone := 0
		tasksVerified := 0
		tasksInProgress := 0
		tasksBlocked := 0
		for _, t := range tasks {
			switch t.Status {
			case taskDone:
				tasksDone++
			case taskVerified:
				tasksVerified++
			case taskInProgress:
				tasksInProgress++
			case taskBlocked:
				tasksBlocked++
			}
		}

		milestonesDone := 0
		for _, m := range milestones {
			if milestoneAllDone(m) {
				milestonesDone++
			}
		}

		featureNextMilestone := nextMilestone(milestones)
		featureNextTask := nextTask(tasks)

		status := computeOverallStatus(tasks)

		// Features archived via /belmont:cleanup have their planning files
		// replaced with a single ARCHIVE.md summary. Without this check the
		// missing PROGRESS.md would make them look like "Not Started" and they
		// would leak into `belmont auto --all`.
		if fileExists(filepath.Join(featurePath, "ARCHIVE.md")) {
			status = "Archived"
		}

		features = append(features, featureSummary{
			Slug:            slug,
			Name:            name,
			TasksDone:       tasksDone + tasksVerified,
			TasksVerified:   tasksVerified,
			TasksInProgress: tasksInProgress,
			TasksBlocked:    tasksBlocked,
			TasksTotal:      tasksTotal,
			MilestonesDone:  milestonesDone,
			TasksOrphaned:   orphaned,
			MilestonesTotal: len(milestones),
			Milestones:      milestones,
			NextMilestone:   featureNextMilestone,
			NextTask:        featureNextTask,
			Status:          status,
		})
	}
	return features
}

// syncMasterFeatureStatuses updates the ## Features table in master .belmont/PROGRESS.md
// to match computed feature-level statuses. This prevents stale master data from causing
// auto mode to skip features that still have pending work.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func syncMasterFeatureStatuses(root string, features []featureSummary) {
	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	// Build lookup from computed features
	type computed struct {
		Status     string
		MsDone     int
		MsTotal    int
		TasksDone  int
		TasksTotal int
	}
	lookup := make(map[string]computed)
	for _, f := range features {
		lookup[f.Slug] = computed{
			Status:     f.Status,
			MsDone:     f.MilestonesDone,
			MsTotal:    f.MilestonesTotal,
			TasksDone:  f.TasksDone,
			TasksTotal: f.TasksTotal,
		}
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	changed := false
	for i, line := range lines {
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
		statusCol := colIdx["Status"]
		msCol := colIdx["Milestones"]
		tasksCol := colIdx["Tasks"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		c, ok := lookup[slug]
		if !ok {
			continue
		}

		newStatus := c.Status
		newMs := fmt.Sprintf("%d/%d", c.MsDone, c.MsTotal)
		newTasks := fmt.Sprintf("%d/%d", c.TasksDone, c.TasksTotal)

		cellsChanged := false
		if statusCol >= 0 && statusCol < len(cells) && cells[statusCol] != newStatus {
			cells[statusCol] = newStatus
			cellsChanged = true
		}
		if msCol >= 0 && msCol < len(cells) && cells[msCol] != newMs {
			cells[msCol] = newMs
			cellsChanged = true
		}
		if tasksCol >= 0 && tasksCol < len(cells) && cells[tasksCol] != newTasks {
			cells[tasksCol] = newTasks
			cellsChanged = true
		}

		if cellsChanged {
			var parts []string
			for _, c := range cells {
				parts = append(parts, " "+c+" ")
			}
			lines[i] = "|" + strings.Join(parts, "|") + "|"
			changed = true
		}
	}

	if changed {
		os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
	}
}

// handleStaleWorktree checks for a stale branch/worktree from a previous interrupted run.
// Returns resumed=true if the existing worktree should be reused (skip creation).
// Returns resumed=false if stale state was cleaned up (proceed with fresh creation).
func handleStaleWorktree(root, id, branch, wtPath string) (resumed bool, err error) {
	// Check if branch already exists
	checkCmd := exec.Command("git", "branch", "--list", branch)
	checkCmd.Dir = root
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false, nil // no stale branch, proceed normally
	}

	// Stale branch exists — determine what to do
	_, wtDirErr := os.Stat(wtPath)
	wtExists := wtDirErr == nil

	if isTerminal(os.Stdin) {
		// Interactive: prompt the user
		status := "branch exists"
		if wtExists {
			status = "branch + worktree exist"
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Branch '%s' exists from a previous run (%s).\033[0m\n", branch, status)
		fmt.Fprintf(os.Stderr, "  [r] Resume from where it left off\n")
		fmt.Fprintf(os.Stderr, "  [s] Start fresh (delete branch and restart)\n")
		fmt.Fprintf(os.Stderr, "  [q] Quit\n")
		fmt.Fprintf(os.Stderr, "> ")

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "r", "resume":
			if wtExists {
				// Worktree still exists — reuse it directly
				fmt.Fprintf(os.Stderr, "  Resuming with existing worktree at %s\n", wtPath)
			} else {
				// Branch exists but worktree is gone — reattach
				fmt.Fprintf(os.Stderr, "  Reattaching worktree to existing branch %s\n", branch)
				wtDir := filepath.Dir(wtPath)
				if err := os.MkdirAll(wtDir, 0755); err != nil {
					return false, fmt.Errorf("create worktree dir: %w", err)
				}
				addCmd := exec.Command("git", "worktree", "add", wtPath, branch)
				addCmd.Dir = root
				if out, err := addCmd.CombinedOutput(); err != nil {
					return false, fmt.Errorf("git worktree add (resume): %w (%s)", err, strings.TrimSpace(string(out)))
				}
			}
			// Rebase the worktree onto current main so sibling features that
			// merged while this one was paused are picked up. Conflicts abort
			// and warn — never auto-resolve. See
			// knowledge/auto-mode/resume-rebase.md for the design.
			n, rebaseErr := rebaseWorktreeOnMain(root, wtPath)
			announceWorktreeRebase(id, n, rebaseErr)
			// Leave .belmont/ as-is in the worktree — it has committed state from the
			// previous run. Don't overwrite with stale copy from main repo.
			// If it's an old symlink from a previous version, replace with a fresh copy.
			dstBelmont := filepath.Join(wtPath, ".belmont")
			if fi, err := os.Lstat(dstBelmont); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				// Old-style symlink — replace with copy-based approach
				os.RemoveAll(dstBelmont)
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			} else if err != nil {
				// .belmont/ missing entirely — copy it in
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			}
			return true, nil

		case "q", "quit":
			return false, fmt.Errorf("user chose to quit")

		default: // "s", "start", or anything else → start fresh
			fmt.Fprintf(os.Stderr, "  Cleaning up stale state for %s...\n", id)
		}
	} else {
		// Non-interactive: auto-restart
		fmt.Fprintf(os.Stderr, "  Cleaning up stale branch '%s' from previous run...\n", branch)
	}

	// Clean up stale state (restart path)
	if wtExists {
		removeWorktree(root, wtPath, id)
	}
	// Prune any orphaned worktree references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = root
	pruneCmd.Run()
	// Delete the stale branch
	delCmd := exec.Command("git", "branch", "-D", branch)
	delCmd.Dir = root
	delCmd.Run()

	return false, nil
}

// buildMilestoneLoopStates derives per-milestone state from the loop history.
func buildMilestoneLoopStates(history []historyEntry, milestones []milestone) map[string]*milestoneLoopState {
	states := make(map[string]*milestoneLoopState)
	for _, m := range milestones {
		states[m.ID] = &milestoneLoopState{
			ID:   m.ID,
			Name: m.Name,
			Done: milestoneAllDone(m),
		}
	}

	var lastImplementedMS string
	for _, h := range history {
		switch h.Action.Type {
		case actionImplementMilestone:
			msID := h.Action.MilestoneID
			if msID != "" {
				if s, ok := states[msID]; ok && h.Result != nil && h.Result.Success {
					s.Implemented = true
					s.WorkType = h.WorkType
					s.FilesChanged = h.FilesChanged
					lastImplementedMS = msID
				}
			}
		case actionVerify:
			// Attribute verification to the most recently implemented milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					if h.Result != nil {
						if h.Result.Success {
							s.Verified = true
							s.VerifySucceeded++
						} else {
							s.VerifyFailed++
						}
					}
				}
			}
		case actionTriage:
			// Track triage rounds per milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					s.FwlupFixRounds++
				}
			}
		}
	}
	return states
}

// interactiveMilestoneSelect shows milestones and lets user pick a range.
func interactiveMilestoneSelect(milestones []milestone) (from, to string, err error) {
	if len(milestones) == 0 {
		return "", "", nil
	}

	fmt.Fprintf(os.Stderr, "\033[1mMilestones:\033[0m\n")
	firstUndone := ""
	for _, m := range milestones {
		marker := "[ ]"
		if milestoneAllDone(m) {
			marker = "[x]"
		}
		if !milestoneAllDone(m) && firstUndone == "" {
			firstUndone = m.ID
		}
		depStr := ""
		if len(m.Deps) > 0 {
			depStr = fmt.Sprintf(" \033[2m(depends: %s)\033[0m", strings.Join(m.Deps, ", "))
		}
		fmt.Fprintf(os.Stderr, "  %s %s: %s%s\n", marker, m.ID, m.Name, depStr)
	}

	lastID := milestones[len(milestones)-1].ID
	defaultRange := ""
	if firstUndone != "" {
		defaultRange = fmt.Sprintf("%s → %s", firstUndone, lastID)
	}

	fmt.Fprintf(os.Stderr, "\n\033[2mDefault range: %s\033[0m\n", defaultRange)
	fmt.Fprintf(os.Stderr, "Press Enter to accept, 'q' to quit, or enter range (e.g. M2 M5): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", "", fmt.Errorf("auto: no input")
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "q" || input == "quit" || input == "exit" {
		return "", "", fmt.Errorf("auto: cancelled by user")
	}

	if input == "" {
		// Accept defaults
		return "", "", nil
	}

	// Parse custom range
	parts := strings.Fields(input)
	if len(parts) == 1 {
		return parts[0], parts[0], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("auto: invalid range %q — use 'M2 M5' format", input)
}

// computeWaves groups milestones into waves using Kahn's algorithm for topological sort.
// Milestones in the same wave have all deps satisfied by prior waves.
// Already-done milestones satisfy deps but don't execute.
func computeWaves(milestones []milestone) ([]wave, error) {
	if len(milestones) == 0 {
		return nil, nil
	}

	// Build ID -> milestone map
	byID := make(map[string]milestone)
	for _, m := range milestones {
		byID[m.ID] = m
	}

	// Compute in-degree for each undone milestone
	inDegree := make(map[string]int)
	for _, m := range milestones {
		if milestoneAllDone(m) {
			continue
		}
		count := 0
		for _, dep := range m.Deps {
			if dm, ok := byID[dep]; ok && !milestoneAllDone(dm) {
				count++
			}
		}
		inDegree[m.ID] = count
	}

	var waves []wave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all milestones with zero in-degree
		var ready []milestone
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, byID[id])
			}
		}

		if len(ready) == 0 {
			// Cycle detected
			var cycleIDs []string
			for id := range inDegree {
				cycleIDs = append(cycleIDs, id)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among milestones: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready milestones by ID for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return parseMilestoneNum(ready[i].ID) < parseMilestoneNum(ready[j].ID)
		})

		waves = append(waves, wave{Index: waveIdx, Milestones: ready})
		waveIdx++

		// Remove completed milestones and update in-degrees
		for _, m := range ready {
			delete(inDegree, m.ID)
			remaining--
		}
		for id, deg := range inDegree {
			m := byID[id]
			newDeg := deg
			for _, dep := range m.Deps {
				for _, completed := range ready {
					if dep == completed.ID {
						newDeg--
					}
				}
			}
			inDegree[id] = newDeg
		}
	}

	return waves, nil
}
