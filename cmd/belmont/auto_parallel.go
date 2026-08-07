package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// runFeatureInWorktree creates a worktree for a feature, installs belmont, and runs the full loop.
func runFeatureInWorktree(cfg loopConfig, slug, branch, wtPath string, tracker *worktreeTracker, resumed bool) error {
	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}

		// Copy .belmont state into worktree (isolated copy, not symlink)
		if err := copyBelmontStateToWorktree(cfg.Root, wtPath, slug); err != nil {
			return fmt.Errorf("copy .belmont state to worktree: %w", err)
		}

		// Commit the initial feature state so the AI agent starts from a clean git state
		commitWorktreeFeatureState(wtPath, slug)
	}

	// Resolve monorepo workspaces (auto-detect, or honor worktree.json overrides)
	hooks := loadWorktreeHooks(cfg.Root)
	workspaces, primary, mType := resolveWorkspaces(cfg.Root, hooks)
	overrides := map[string]workspaceOverride{}
	if hooks != nil {
		overrides = hooks.Workspaces
	}

	// Copy .env files (gitignored, so not present in fresh worktrees).
	// In monorepo mode, also seed env files into qualifying workspace dirs.
	copyEnvFiles(cfg.Root, wtPath, workspaces, overrides)

	// Run belmont install in the worktree
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33mInstall warning for %s: %s\033[0m\n", slug, strings.TrimSpace(string(out)))
	}

	// Allocate a port for this worktree
	port, err := allocatePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to allocate port for %s: %s\033[0m\n", slug, err)
	} else {
		fmt.Fprintf(os.Stderr, "  Port %d assigned to %s\n", port, slug)
	}
	if tracker != nil {
		tracker.setPort(slug, port)
	}

	if mType != monorepoNone {
		fmt.Fprintf(os.Stderr, "  Detected %s monorepo (%d workspaces, primary=%s)\n", mType, len(workspaces), primary)
	}

	// Run worktree setup hooks
	if hooks != nil && len(hooks.Setup) > 0 {
		fmt.Fprintf(os.Stderr, "  Running worktree setup hooks for %s...\n", slug)
		if err := runWorktreeHookCommands(hooks.Setup, wtPath, port, hooks.Env, workspaces, primary, mType); err != nil {
			return fmt.Errorf("worktree setup for %s: %w", slug, err)
		}
	} else if hooks == nil {
		// No worktree.json — auto-detect dependency install from lock files
		if cmds := detectAutoInstallCommands(cfg.Root); len(cmds) > 0 {
			fmt.Fprintf(os.Stderr, "  Auto-installing dependencies for %s (%s)...\n", slug, strings.Join(cmds, ", "))
			if err := runWorktreeHookCommands(cmds, wtPath, port, nil, workspaces, primary, mType); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠ Auto-install failed for %s: %s (continuing)\033[0m\n", slug, err)
			}
		}
	}

	// Run loop for this feature (all milestones)
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.Feature = slug
	mCfg.Port = port
	mCfg.Workspaces = workspaces
	mCfg.PrimaryWorkspace = primary
	mCfg.MonorepoType = mType
	if hooks != nil {
		mCfg.WorktreeEnv = hooks.Env
	}

	// Load per-feature model tiers from the worktree's copy of models.yaml.
	if t, err := parseModelTiers(filepath.Join(wtPath, ".belmont", "features", slug, "models.yaml")); err == nil {
		mCfg.ModelTiers = t
	}

	return runLoop(mCfg)
}

// mergeFeatureBranch merges a feature branch back to main and cleans up.
func mergeFeatureBranch(cfg loopConfig, slug, branch, wtPath string, tracker *worktreeTracker) error {
	// Commit any uncommitted CODE changes in the worktree before merging.
	// .belmont/ is assume-unchanged so it won't be included in this commit.
	if err := commitWorktreeChanges(wtPath, slug); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to commit worktree changes for %s: %s\033[0m\n", slug, err)
	}

	commitMsg := fmt.Sprintf("belmont: merge feature %s", slug)

	if err := attemptMerge(cfg, commitMsg, branch, slug); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for feature %s\033[0m\n", slug)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", slug)
		return err
	}

	// Copy the feature's updated state from the worktree back to the main repo.
	// .belmont/ was excluded from the merge (assume-unchanged), so we sync it
	// separately. This preserves other features' state on the main branch.
	syncFeatureStateAfterMerge(cfg.Root, wtPath, slug)

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Run teardown hooks and clean up worktree
	tracker.teardownEntry(slug)
	removeWorktree(cfg.Root, wtPath, slug)
	tracker.remove(slug)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Feature %s merged successfully\033[0m\n", slug)
	return nil
}

// runAutoParallel executes milestones with dependency-aware parallel waves using worktrees.
func runAutoParallel(cfg loopConfig, milestones []milestone) error {
	startTime := time.Now()

	// Pre-flight: ensure repo is in a clean state before starting
	if err := validateRepoState(cfg.Root); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Record original branch and restore on exit if changed
	origBranch := getCurrentBranch(cfg.Root)
	defer func() {
		if origBranch != "" && origBranch != "HEAD" {
			if cur := getCurrentBranch(cfg.Root); cur != origBranch {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ Branch changed from %s to %s — restoring...\033[0m\n", origBranch, cur)
				restoreCmd := exec.Command("git", "checkout", origBranch)
				restoreCmd.Dir = cfg.Root
				restoreCmd.Run()
			}
		}
	}()

	// Ensure the sibling worktree base directory exists
	if err := os.MkdirAll(worktreeBasePath(cfg.Root), 0755); err != nil {
		return fmt.Errorf("create worktree base dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (parallel) — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	waves, err := computeWaves(milestones)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if len(waves) == 0 {
		fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — all milestones already done\n")
		return nil
	}

	// Print wave plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var ids []string
		for _, m := range w.Milestones {
			ids = append(ids, m.ID)
		}
		parallel := ""
		if len(w.Milestones) > 1 {
			parallel = " (parallel)"
		}
		fmt.Fprintf(os.Stderr, "  Wave %d: %s%s\n", w.Index+1, strings.Join(ids, ", "), parallel)
	}
	fmt.Fprintln(os.Stderr)

	// Set up signal handler for cleanup
	activeWorktrees := &worktreeTracker{
		root:    cfg.Root,
		feature: cfg.Feature,
		mode:    "single-feature-parallel",
		entries: make(map[string]worktreeEntry),
		hooks:   loadWorktreeHooks(cfg.Root),
	}
	sigCh := make(chan os.Signal, 1)
	notifySignals(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — preserving worktrees for resume...\033[0m\n")
		activeWorktrees.gracefulShutdown(cfg.Root)
		os.Exit(1)
	}()

	for _, w := range waves {
		fmt.Fprintf(os.Stderr, "\033[1m━━ Wave %d ━━\033[0m\n", w.Index+1)

		// Every wave — including single-milestone waves — runs through the
		// worktree path. The tiny startup overhead is worth the uniformity:
		// scope-guard amends don't rewrite the user's working branch, rollback
		// is a worktree remove rather than a reset, and state visibility via
		// `belmont status` behaves the same regardless of wave size. See
		// knowledge/auto-mode/parallel-wave-orchestration.md.
		if err := runWaveParallel(cfg, w, activeWorktrees); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "\033[32m  ✓ Wave %d complete\033[0m\n\n", w.Index+1)
	}

	// Sync master PROGRESS.md with actual feature states
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	syncMasterFeatureStatuses(cfg.Root, listFeatures(featuresDir, 50))

	// Commit any remaining .belmont/ state changes
	if err := commitBelmontState(cfg.Root); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to commit final .belmont/ state: %s\033[0m\n", err)
	}

	// Clean up auto.json
	activeWorktrees.removeAutoJSON()

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ All waves complete\033[0m (%.1fs total)\n", time.Since(startTime).Seconds())
	return nil
}

// runWaveParallel runs every milestone in a wave in its own worktree, even
// single-milestone waves. Uniform behavior is worth the small startup cost;
// see knowledge/auto-mode/parallel-wave-orchestration.md on the removed
// master-tree shortcut.
func runWaveParallel(cfg loopConfig, w wave, tracker *worktreeTracker) error {
	type result struct {
		MilestoneID  string
		Branch       string
		WorktreePath string
		Err          error
	}

	// Serial path: when MaxParallel == 1, run each milestone and merge it
	// inline before moving on. The next milestone's worktree forks from a
	// feature branch that already includes the prior milestone's merge.
	// Stale-worktree resolution is deferred to just-in-time so each
	// milestone's rebase-on-resume targets the post-prior-merge tip.
	// See knowledge/auto-mode/multi-feature-scheduling.md (semantic is
	// shared across multi-feature and single-feature-parallel modes).
	if cfg.MaxParallel <= 1 {
		mergedFiles := map[string][]string{}
		var failures []result
		for _, m := range w.Milestones {
			branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(m.ID))
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(m.ID)))

			resumed, err := handleStaleWorktree(cfg.Root, m.ID, branch, wtPath)
			if err != nil {
				return err
			}

			tracker.add(m.ID, wtPath, branch)
			fmt.Fprintf(os.Stderr, "  \033[36m▶ %s: %s\033[0m (worktree)\n", m.ID, m.Name)

			if err := runMilestoneInWorktree(cfg, m, branch, wtPath, tracker, resumed); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[31m✗ %s failed: %s\033[0m\n", m.ID, err)
				failures = append(failures, result{MilestoneID: m.ID, Branch: branch, WorktreePath: wtPath, Err: err})
				continue
			}
			fmt.Fprintf(os.Stderr, "  \033[32m✓ %s complete\033[0m\n", m.ID)

			if err := ensureCleanMergeState(cfg.Root); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠ %s — skipping merge for %s\033[0m\n", err, m.ID)
				failures = append(failures, result{MilestoneID: m.ID, Branch: branch, WorktreePath: wtPath, Err: fmt.Errorf("skipped: unclean merge state")})
				continue
			}
			reportMergeOverlap(cfg.Root, branch, m.ID, mergedFiles)
			if err := mergeWorktreeBranch(cfg, m.ID, branch, wtPath, tracker); err != nil {
				return fmt.Errorf("auto: merge failed for %s: %w", m.ID, err)
			}
			for _, f := range branchTouchedFiles(cfg.Root, branch) {
				mergedFiles[f] = append(mergedFiles[f], m.ID)
			}
		}

		if len(failures) > 0 {
			fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d milestone(s) failed in wave %d:\033[0m\n", len(failures), w.Index+1)
			for _, f := range failures {
				fmt.Fprintf(os.Stderr, "  %s: worktree preserved at %s\n", f.MilestoneID, f.WorktreePath)
				fmt.Fprintf(os.Stderr, "    Resume: cd %s && belmont auto --feature %s --from %s --to %s\n", f.WorktreePath, cfg.Feature, f.MilestoneID, f.MilestoneID)
			}
			return fmt.Errorf("auto: wave %d had %d failure(s)", w.Index+1, len(failures))
		}
		return nil
	}

	semaphore := make(chan struct{}, cfg.MaxParallel)
	var wg sync.WaitGroup

	results := make(chan result, len(w.Milestones))

	// Resolve stale worktrees sequentially (before parallel launch) to avoid stdin races
	msPreResolved := make(map[string]bool) // milestone ID -> resumed
	for _, m := range w.Milestones {
		branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(m.ID))
		wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(m.ID)))
		resumed, err := handleStaleWorktree(cfg.Root, m.ID, branch, wtPath)
		if err != nil {
			return err
		}
		msPreResolved[m.ID] = resumed
	}

	for _, m := range w.Milestones {
		wg.Add(1)
		go func(ms milestone, resumed bool) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire
			defer func() { <-semaphore }() // release

			branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(ms.ID))
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(ms.ID)))

			tracker.add(ms.ID, wtPath, branch)

			fmt.Fprintf(os.Stderr, "  \033[36m▶ %s: %s\033[0m (worktree)\n", ms.ID, ms.Name)

			err := runMilestoneInWorktree(cfg, ms, branch, wtPath, tracker, resumed)
			results <- result{
				MilestoneID:  ms.ID,
				Branch:       branch,
				WorktreePath: wtPath,
				Err:          err,
			}
		}(m, msPreResolved[m.ID])
	}

	// Wait for all goroutines then close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var successes []result
	var failures []result
	for r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  \033[31m✗ %s failed: %s\033[0m\n", r.MilestoneID, r.Err)
			failures = append(failures, r)
		} else {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ %s complete\033[0m\n", r.MilestoneID)
			successes = append(successes, r)
		}
	}

	// Merge successful branches in milestone ID order
	sort.Slice(successes, func(i, j int) bool {
		return parseMilestoneNum(successes[i].MilestoneID) < parseMilestoneNum(successes[j].MilestoneID)
	})

	// Track files already merged from earlier siblings in this wave so we
	// can warn when a later merge overlaps with them — the signature of
	// cross-milestone scope leak that survived Layer 1 guards.
	mergedFiles := map[string][]string{} // file -> []milestoneIDs that touched it

	for i, s := range successes {
		// Ensure repo is in a clean merge state before each merge
		if err := ensureCleanMergeState(cfg.Root); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ %s — skipping remaining %d merge(s)\033[0m\n", err, len(successes)-i)
			for _, remaining := range successes[i:] {
				failures = append(failures, result{MilestoneID: remaining.MilestoneID, WorktreePath: remaining.WorktreePath, Err: fmt.Errorf("skipped: unclean merge state")})
			}
			break
		}

		// Pre-merge overlap report: list files this branch touches that are
		// also touched by siblings we've already merged. Does not block.
		reportMergeOverlap(cfg.Root, s.Branch, s.MilestoneID, mergedFiles)

		if err := mergeWorktreeBranch(cfg, s.MilestoneID, s.Branch, s.WorktreePath, tracker); err != nil {
			return fmt.Errorf("auto: merge failed for %s: %w", s.MilestoneID, err)
		}

		// Record this branch's touched files so the next iteration can detect
		// overlap.
		for _, f := range branchTouchedFiles(cfg.Root, s.Branch) {
			mergedFiles[f] = append(mergedFiles[f], s.MilestoneID)
		}
	}

	// Clean up failed worktrees (preserve for manual intervention)
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d milestone(s) failed in wave %d:\033[0m\n", len(failures), w.Index+1)
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  %s: worktree preserved at %s\n", f.MilestoneID, f.WorktreePath)
			fmt.Fprintf(os.Stderr, "    Resume: cd %s && belmont auto --feature %s --from %s --to %s\n", f.WorktreePath, cfg.Feature, f.MilestoneID, f.MilestoneID)
		}
		return fmt.Errorf("auto: wave %d had %d failure(s)", w.Index+1, len(failures))
	}

	return nil
}

// runMilestoneInWorktree creates a worktree, installs belmont, copies state, and runs the loop.
func runMilestoneInWorktree(cfg loopConfig, ms milestone, branch, wtPath string, tracker *worktreeTracker, resumed bool) error {
	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	// Copy .belmont state into worktree (isolated copy, not symlink)
	if err := copyBelmontStateToWorktree(cfg.Root, wtPath, cfg.Feature); err != nil {
		return fmt.Errorf("copy .belmont state to worktree: %w", err)
	}

	// Commit the initial feature state so the AI agent starts from a clean git state
	commitWorktreeFeatureState(wtPath, cfg.Feature)

	// Resolve monorepo workspaces (auto-detect, or honor worktree.json overrides)
	hooks := loadWorktreeHooks(cfg.Root)
	workspaces, primary, mType := resolveWorkspaces(cfg.Root, hooks)
	overrides := map[string]workspaceOverride{}
	if hooks != nil {
		overrides = hooks.Workspaces
	}

	// Copy .env files (gitignored, so not present in fresh worktrees).
	// In monorepo mode, also seed env files into qualifying workspace dirs.
	copyEnvFiles(cfg.Root, wtPath, workspaces, overrides)

	// Run belmont install in the worktree (shell out to self)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    \033[33mInstall warning for %s: %s\033[0m\n", ms.ID, strings.TrimSpace(string(out)))
	}

	// Allocate a port for this worktree
	port, err := allocatePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "    \033[33m⚠ Failed to allocate port for %s: %s\033[0m\n", ms.ID, err)
	} else {
		fmt.Fprintf(os.Stderr, "    Port %d assigned to %s\n", port, ms.ID)
	}
	if tracker != nil {
		tracker.setPort(ms.ID, port)
	}

	if mType != monorepoNone {
		fmt.Fprintf(os.Stderr, "    Detected %s monorepo (%d workspaces, primary=%s)\n", mType, len(workspaces), primary)
	}

	// Run worktree setup hooks
	if hooks != nil && len(hooks.Setup) > 0 {
		fmt.Fprintf(os.Stderr, "    Running worktree setup hooks for %s...\n", ms.ID)
		if err := runWorktreeHookCommands(hooks.Setup, wtPath, port, hooks.Env, workspaces, primary, mType); err != nil {
			return fmt.Errorf("worktree setup for %s: %w", ms.ID, err)
		}
	} else if hooks == nil {
		// No worktree.json — auto-detect dependency install from lock files
		if cmds := detectAutoInstallCommands(cfg.Root); len(cmds) > 0 {
			fmt.Fprintf(os.Stderr, "    Auto-installing dependencies for %s (%s)...\n", ms.ID, strings.Join(cmds, ", "))
			if err := runWorktreeHookCommands(cmds, wtPath, port, nil, workspaces, primary, mType); err != nil {
				fmt.Fprintf(os.Stderr, "    \033[33m⚠ Auto-install failed for %s: %s (continuing)\033[0m\n", ms.ID, err)
			}
		}
	}

	// Run loop for this single milestone
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.From = ms.ID
	mCfg.To = ms.ID
	mCfg.Port = port
	mCfg.Tracker = tracker
	mCfg.TrackerID = ms.ID
	mCfg.Workspaces = workspaces
	mCfg.PrimaryWorkspace = primary
	mCfg.MonorepoType = mType
	if hooks != nil {
		mCfg.WorktreeEnv = hooks.Env
	}

	// Load per-feature model tiers from the worktree's copy of models.yaml.
	if t, err := parseModelTiers(filepath.Join(wtPath, ".belmont", "features", cfg.Feature, "models.yaml")); err == nil {
		mCfg.ModelTiers = t
	}

	return runLoop(mCfg)
}

// mergeWorktreeBranch merges a milestone branch back and cleans up the worktree.
func mergeWorktreeBranch(cfg loopConfig, milestoneID, branch, wtPath string, tracker *worktreeTracker) error {
	// Find the milestone name for the commit message
	var msName string
	progressPath := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	if data, err := os.ReadFile(progressPath); err == nil {
		for _, m := range parseMilestones(string(data)) {
			if m.ID == milestoneID {
				msName = m.Name
				break
			}
		}
	}

	// Commit any uncommitted changes in the worktree before merging
	if err := commitWorktreeChanges(wtPath, milestoneID); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to commit worktree changes for %s: %s\033[0m\n", milestoneID, err)
	}

	commitMsg := fmt.Sprintf("belmont: merge %s (%s)", milestoneID, msName)

	if err := attemptMerge(cfg, commitMsg, branch, milestoneID); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for %s\033[0m\n", milestoneID)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", filepath.Base(wtPath))
		return err
	}

	// Copy the feature's updated state from the worktree back to the main repo
	syncFeatureStateAfterMerge(cfg.Root, wtPath, cfg.Feature)

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Run teardown hooks and clean up worktree
	tracker.teardownEntry(milestoneID)
	removeWorktree(cfg.Root, wtPath, milestoneID)
	tracker.remove(milestoneID)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	return nil
}

// attemptMerge tries to merge a branch, handling various failure modes automatically.
// It handles untracked file overwrites, dirty worktrees, and merge conflicts.
func attemptMerge(cfg loopConfig, commitMsg, branch, id string) error {
	// Try the merge
	cmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
	cmd.Dir = cfg.Root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil // merge succeeded
	}

	output := string(out)
	kind := classifyMergeError(output)

	switch kind {
	case mergeUntrackedOverwrite:
		// Temporarily stash untracked files that conflict
		files := parseOverwrittenFiles(output)
		if len(files) == 0 {
			return fmt.Errorf("merge failed (untracked overwrite) but could not parse file list: %s", output)
		}

		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Untracked files would be overwritten for %s — auto-stashing %d files...\033[0m\n", id, len(files))

		stashDir := filepath.Join(cfg.Root, ".belmont", "merge-stash")
		if err := os.MkdirAll(stashDir, 0755); err != nil {
			return fmt.Errorf("create merge-stash dir: %w", err)
		}

		// Move conflicting files to stash
		for _, f := range files {
			src := filepath.Join(cfg.Root, f)
			dst := filepath.Join(stashDir, f)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				continue
			}
			os.Rename(src, dst)
		}

		// Retry the merge
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()

		if retryErr == nil {
			// Merge succeeded — clean up stash
			os.RemoveAll(stashDir)
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing untracked files for %s\033[0m\n", id)
			return nil
		}

		// Retry failed — restore stashed files and fall through
		for _, f := range files {
			src := filepath.Join(stashDir, f)
			dst := filepath.Join(cfg.Root, f)
			os.MkdirAll(filepath.Dir(dst), 0755)
			os.Rename(src, dst)
		}
		os.RemoveAll(stashDir)

		// Classify the retry failure
		retryOutput := string(retryOut)
		retryKind := classifyMergeError(retryOutput)
		if retryKind == mergeConflict {
			// Fall through to conflict handling below
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing untracked files: %s", id, retryOutput)

	case mergeDirtyWorktree:
		// Stash local changes and retry merge
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Local changes would be overwritten for %s — stashing...\033[0m\n", id)

		stashCmd := exec.Command("git", "stash", "push", "--include-untracked", "-m", "belmont: pre-merge stash")
		stashCmd.Dir = cfg.Root
		if _, stashErr := stashCmd.CombinedOutput(); stashErr != nil {
			return fmt.Errorf("git stash failed for %s: %w", id, stashErr)
		}

		retryCmd2 := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd2.Dir = cfg.Root
		retryOut2, retryErr2 := retryCmd2.CombinedOutput()

		// Pop the stash — handle failure to avoid orphaned stash entries
		popCmd := exec.Command("git", "stash", "pop")
		popCmd.Dir = cfg.Root
		if popOut, popErr := popCmd.CombinedOutput(); popErr != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ stash pop had conflicts for %s — your local changes are preserved in 'git stash list', resolve with 'git stash pop': %s\033[0m\n", id, strings.TrimSpace(string(popOut)))
		}

		if retryErr2 == nil {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing local changes for %s\033[0m\n", id)
			return nil
		}

		retryOutput2 := string(retryOut2)
		retryKind2 := classifyMergeError(retryOutput2)
		if retryKind2 == mergeConflict {
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing local changes: %s", id, retryOutput2)

	case mergeConflict:
		goto handleConflict

	case mergeUnmergedFiles:
		// Stale merge state from a previous operation — abort and retry once
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Stale unmerged files for %s — aborting previous merge and retrying...\033[0m\n", id)
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = cfg.Root
		abortCmd.Run()
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()
		if retryErr == nil {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after aborting stale merge for %s\033[0m\n", id)
			return nil
		}
		retryKind := classifyMergeError(string(retryOut))
		if retryKind == mergeConflict {
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after aborting stale merge: %s", id, string(retryOut))

	default:
		return fmt.Errorf("merge failed for %s: %s", id, output)
	}

handleConflict:
	// Try auto-resolving .belmont/ conflicts first (common with parallel milestones)
	autoResolveBelmontConflicts(cfg.Root)

	// Try auto-resolving lock files (delete + regenerate via package manager)
	autoResolveLockFiles(cfg.Root)

	// Check if all conflicts are now resolved
	{
		checkCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
		checkCmd.Dir = cfg.Root
		if checkOut, err := checkCmd.Output(); err == nil && strings.TrimSpace(string(checkOut)) == "" {
			// All conflicts resolved — commit the merge
			commitCmd2 := exec.Command("git", "commit", "--no-edit")
			commitCmd2.Dir = cfg.Root
			if _, err := commitCmd2.CombinedOutput(); err == nil {
				fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge conflicts auto-resolved for %s\033[0m\n", id)
				return nil
			}
		}
	}

	fmt.Fprintf(os.Stderr, "  \033[33m⚠ Merge conflict for %s — invoking reconciliation agent...\033[0m\n", id)

	reconcileErr := runReconciliationAgent(cfg, id, branch)
	if reconcileErr != nil {
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = cfg.Root
		abortCmd.Run()
		return fmt.Errorf("merge conflict resolution failed for %s: %w", id, reconcileErr)
	}

	// Reconciliation succeeded — commit the merge
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = cfg.Root
	if _, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
		// Abort the merge to prevent stale MERGE_HEAD from poisoning subsequent operations
		abortCmd2 := exec.Command("git", "merge", "--abort")
		abortCmd2.Dir = cfg.Root
		abortCmd2.Run()
		return fmt.Errorf("commit after reconciliation for %s: %w", id, commitErr)
	}

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Reconciliation resolved merge conflict for %s\033[0m\n", id)
	return nil
}
