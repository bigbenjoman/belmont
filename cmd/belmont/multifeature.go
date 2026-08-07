package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// resolveFeatureSlugs resolves the list of feature slugs for multi-feature mode.
func resolveFeatureSlugs(root, featuresFlag string, allFlag bool) ([]string, error) {
	featuresDir := filepath.Join(root, ".belmont", "features")

	if allFlag {
		// Get all features, filter to pending ones
		features := listFeatures(featuresDir, 50)
		if len(features) == 0 {
			return nil, fmt.Errorf("auto: no features found in %s", featuresDir)
		}
		// Sync computed feature statuses back to master PROGRESS.md to fix drift
		syncMasterFeatureStatuses(root, features)
		var slugs []string
		for _, f := range features {
			// Skip features that are already done (Complete/Verified) or
			// archived via /belmont:cleanup. Only active/pending work runs.
			if isFeatureTerminal(f.Status) {
				continue
			}
			slugs = append(slugs, f.Slug)
		}
		if len(slugs) == 0 {
			return nil, fmt.Errorf("auto: no pending features — all are complete, verified, or archived")
		}
		// `--all` has no caller-supplied order to preserve; use a stable
		// alphabetical contract so scheduler output is deterministic.
		// (`computeFeatureWaves` honours input order, so we sort here.)
		sort.Strings(slugs)
		return slugs, nil
	}

	// Parse comma-separated slugs
	parts := strings.Split(featuresFlag, ",")
	var slugs []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		// Verify feature directory exists
		featureDir := filepath.Join(featuresDir, s)
		if !dirExists(featureDir) {
			return nil, fmt.Errorf("auto: feature %q not found at %s", s, featureDir)
		}
		slugs = append(slugs, s)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("auto: no valid feature slugs provided")
	}
	return slugs, nil
}

// scanReadiness reports requested features whose declared deps are not yet
// terminal (Complete/Verified/Archived). Uses the same `isFeatureTerminal`
// predicate as the scheduler so messaging cannot drift. Pure: no I/O.
func scanReadiness(features []featureSummary) []readinessWarning {
	bySlug := make(map[string]featureSummary, len(features))
	for _, f := range features {
		bySlug[f.Slug] = f
	}
	var warnings []readinessWarning
	for _, f := range features {
		for _, dep := range f.Deps {
			df, ok := bySlug[dep]
			if !ok || isFeatureTerminal(df.Status) {
				continue
			}
			warnings = append(warnings, readinessWarning{
				Slug:      f.Slug,
				DepSlug:   dep,
				DepStatus: df.Status,
				Blocked:   df.TasksBlocked,
			})
		}
	}
	return warnings
}

// filterWaveByBlocked partitions a wave into runnable features and skipped
// features based on which dep sets are populated. A feature is skipped on
// the first dep it has that appears in `failed` or `paused` (deps scanned in
// declaration order; failed wins over paused so the skip message reflects
// the harder failure when both apply). Pure: no I/O.
func filterWaveByBlocked(wave []featureSummary, failed, paused map[string]bool) (runnable []featureSummary, skipped []skipResult) {
	for _, f := range wave {
		var blockedBy *skipResult
		for _, dep := range f.Deps {
			if failed[dep] {
				blockedBy = &skipResult{Slug: f.Slug, DepSlug: dep, Reason: "failed"}
				break
			}
			if paused[dep] && blockedBy == nil {
				blockedBy = &skipResult{Slug: f.Slug, DepSlug: dep, Reason: "paused"}
				// Don't break — keep scanning in case a later dep is failed,
				// which should win for messaging purposes.
			}
		}
		if blockedBy != nil {
			skipped = append(skipped, *blockedBy)
		} else {
			runnable = append(runnable, f)
		}
	}
	return runnable, skipped
}

// computeFeatureWaves groups features into waves using Kahn's algorithm for topological sort.
// Features in the same wave have all deps satisfied by prior waves.
// Already-complete features satisfy deps but don't execute.
//
// Ordering contract: caller-supplied input order wins. Sibling features in the
// same wave appear in the order they appear in `features`. Callers that want
// alphabetical ordering (e.g. `auto --all`) must sort `features` themselves.
func computeFeatureWaves(features []featureSummary) ([]featureWave, error) {
	if len(features) == 0 {
		return nil, nil
	}

	// Build slug -> feature map
	bySlug := make(map[string]featureSummary)
	for _, f := range features {
		bySlug[f.Slug] = f
	}

	// Compute in-degree for each non-terminal feature. Complete/Verified/
	// Archived features don't execute but still satisfy downstream deps.
	inDegree := make(map[string]int)
	for _, f := range features {
		if isFeatureTerminal(f.Status) {
			continue
		}
		count := 0
		for _, dep := range f.Deps {
			if df, ok := bySlug[dep]; ok && !isFeatureTerminal(df.Status) {
				count++
			}
		}
		inDegree[f.Slug] = count
	}

	var waves []featureWave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all features with zero in-degree, scanning the input slice so
		// caller-supplied order is preserved within the wave.
		var ready []featureSummary
		for _, f := range features {
			deg, tracked := inDegree[f.Slug]
			if !tracked || deg != 0 {
				continue
			}
			ready = append(ready, bySlug[f.Slug])
		}

		if len(ready) == 0 {
			var cycleIDs []string
			for slug := range inDegree {
				cycleIDs = append(cycleIDs, slug)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among features: %s", strings.Join(cycleIDs, ", "))
		}

		waves = append(waves, featureWave{Index: waveIdx, Features: ready})
		waveIdx++

		// Remove completed features and update in-degrees
		for _, f := range ready {
			delete(inDegree, f.Slug)
			remaining--
		}
		for slug, deg := range inDegree {
			f := bySlug[slug]
			newDeg := deg
			for _, dep := range f.Deps {
				for _, completed := range ready {
					if dep == completed.Slug {
						newDeg--
					}
				}
			}
			inDegree[slug] = newDeg
		}
	}

	return waves, nil
}

// runAutoMultiFeature orchestrates wave-based execution of multiple features.
// Features with dependencies execute after their dependencies complete.
func runAutoMultiFeature(cfg loopConfig, slugs []string) error {
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
	os.MkdirAll(worktreeBasePath(cfg.Root), 0755)

	// Build feature summaries with dependency info
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	allFeatures := listFeatures(featuresDir, 50)
	populateFeatureDeps(allFeatures, cfg.Root)

	// Filter to requested slugs, preserving caller-supplied (CLI) order.
	// `allFeatures` is in directory-listing order (effectively alphabetical),
	// so we look up by slug to keep `--features=A,B,C` order intact for the
	// scheduler's sibling tie-break.
	bySlug := make(map[string]featureSummary, len(allFeatures))
	for _, f := range allFeatures {
		bySlug[f.Slug] = f
	}
	var features []featureSummary
	for _, slug := range slugs {
		if f, ok := bySlug[slug]; ok {
			features = append(features, f)
		}
	}

	// Validate dependencies
	if err := validateFeatureDeps(features, allFeatures); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Pre-flight readiness scan: warn (don't abort) on any requested feature
	// whose dep is not yet terminal. Operator can Ctrl-C before launch.
	if warns := scanReadiness(features); len(warns) > 0 {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Readiness check:\033[0m\n")
		for _, w := range warns {
			suffix := ""
			if w.Blocked > 0 {
				if w.Blocked == 1 {
					suffix = ", 1 blocked task"
				} else {
					suffix = fmt.Sprintf(", %d blocked tasks", w.Blocked)
				}
			}
			fmt.Fprintf(os.Stderr, "  • %s depends on %s (status: %s%s)\n", w.Slug, w.DepSlug, w.DepStatus, suffix)
		}
	}

	// Check if any features have deps — if not, use flat parallel (original behavior)
	hasAnyDeps := false
	for _, f := range features {
		if len(f.Deps) > 0 {
			hasAnyDeps = true
			break
		}
	}

	// Compute waves
	waves, err := computeFeatureWaves(features)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if !hasAnyDeps {
		// No dependencies — single wave with all features (original behavior)
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features\033[0m\n", len(slugs))
	} else {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features in %d waves\033[0m\n", len(slugs), len(waves))
	}
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	// Print wave execution plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var names []string
		for _, f := range w.Features {
			names = append(names, f.Slug)
		}
		if len(waves) == 1 {
			for _, n := range names {
				fmt.Fprintf(os.Stderr, "  • %s\n", n)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  Wave %d: [%s]\n", w.Index+1, strings.Join(names, ", "))
		}
	}
	fmt.Fprintln(os.Stderr)

	if cfg.DryRun {
		return nil
	}

	// Set up worktree tracker and signal handler
	activeWorktrees := &worktreeTracker{
		root:    cfg.Root,
		mode:    "multi-feature",
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

	type featureResult struct {
		Slug         string
		Branch       string
		WorktreePath string
		Err          error
	}

	var allFailures []featureResult
	failedSlugs := make(map[string]bool)
	pausedSlugs := make(map[string]bool)
	totalMerged := 0

	// Execute wave by wave
	for _, w := range waves {
		// Partition the wave into runnable vs skipped using the failed/paused
		// dep sets. A skipped feature inherits its blocker's reason and is
		// added to the matching set so its own dependents cascade-skip in
		// later waves.
		waveFeatures, skipped := filterWaveByBlocked(w.Features, failedSlugs, pausedSlugs)
		for _, s := range skipped {
			if s.Reason == "failed" {
				fmt.Fprintf(os.Stderr, "\033[31m⊘ %s skipped\033[0m — dependency %s failed\n", s.Slug, s.DepSlug)
				failedSlugs[s.Slug] = true
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: fmt.Errorf("dependency %s failed", s.DepSlug)})
			} else {
				fmt.Fprintf(os.Stderr, "\033[33m⊘ %s skipped\033[0m — dependency %s paused\n", s.Slug, s.DepSlug)
				pausedSlugs[s.Slug] = true
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: fmt.Errorf("dependency %s paused", s.DepSlug)})
			}
		}

		if len(waveFeatures) == 0 {
			continue
		}

		if len(waves) > 1 {
			fmt.Fprintf(os.Stderr, "\n\033[1m── Wave %d ──\033[0m\n", w.Index+1)
		}

		// Serial path: when MaxParallel == 1, run each feature and merge it
		// inline before moving on. The next feature's worktree forks from a
		// main that includes the prior feature's merge, so implicit cross-
		// feature task deps (e.g. F2 calling into F1's new route) resolve at
		// the fork point instead of producing `[!]` blockers. Stale-worktree
		// resolution is deferred to just-in-time so each feature's rebase-on-
		// resume targets the post-prior-merge main rather than the pre-wave
		// main. See knowledge/auto-mode/multi-feature-scheduling.md.
		if cfg.MaxParallel <= 1 {
			for _, f := range waveFeatures {
				slug := f.Slug
				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(worktreeBasePath(cfg.Root), slug)

				resumed, err := handleStaleWorktree(cfg.Root, slug, branch, wtPath)
				if err != nil {
					return err
				}

				activeWorktrees.add(slug, wtPath, branch)
				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				runErr := runFeatureInWorktree(cfg, slug, branch, wtPath, activeWorktrees, resumed)
				if runErr != nil {
					if errors.Is(runErr, errFeaturePaused) {
						fmt.Fprintf(os.Stderr, "\033[33m⏸ %s paused\033[0m — has unresolved blockers\n", slug)
						pausedSlugs[slug] = true
						allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: runErr})
					} else {
						fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", slug, runErr)
						failedSlugs[slug] = true
						allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: runErr})
					}
					continue
				}

				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", slug)
				if err := ensureCleanMergeState(cfg.Root); err != nil {
					fmt.Fprintf(os.Stderr, "\033[33m⚠ %s — skipping merge\033[0m\n", err)
					allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: fmt.Errorf("skipped: unclean merge state")})
					failedSlugs[slug] = true
					continue
				}
				if err := mergeFeatureBranch(cfg, slug, branch, wtPath, activeWorktrees); err != nil {
					fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", slug, err)
					fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", wtPath)
					fmt.Fprintf(os.Stderr, "  Branch: %s\n", branch)
					allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: err})
					failedSlugs[slug] = true
					continue
				}
				totalMerged++
			}
			continue
		}

		// Resolve stale worktrees sequentially (before parallel launch) to avoid stdin races
		preResolved := make(map[string]bool) // slug -> resumed
		for _, f := range waveFeatures {
			branch := fmt.Sprintf("belmont/auto/%s", f.Slug)
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), f.Slug)
			resumed, err := handleStaleWorktree(cfg.Root, f.Slug, branch, wtPath)
			if err != nil {
				return err
			}
			preResolved[f.Slug] = resumed
		}

		// Run this wave's features in parallel
		semaphore := make(chan struct{}, cfg.MaxParallel)
		var wg sync.WaitGroup
		results := make(chan featureResult, len(waveFeatures))

		for _, f := range waveFeatures {
			wg.Add(1)
			go func(slug string, resumed bool) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(worktreeBasePath(cfg.Root), slug)

				activeWorktrees.add(slug, wtPath, branch)

				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				err := runFeatureInWorktree(cfg, slug, branch, wtPath, activeWorktrees, resumed)
				results <- featureResult{
					Slug:         slug,
					Branch:       branch,
					WorktreePath: wtPath,
					Err:          err,
				}
			}(f.Slug, preResolved[f.Slug])
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect wave results
		var waveSuccesses []featureResult
		for r := range results {
			if r.Err != nil {
				if errors.Is(r.Err, errFeaturePaused) {
					fmt.Fprintf(os.Stderr, "\033[33m⏸ %s paused\033[0m — has unresolved blockers\n", r.Slug)
					// Track in pausedSlugs so dependents in later waves skip
					// with reason=paused (vs reason=failed). The feature is
					// not added to failedSlugs or waveSuccesses — its
					// worktree is preserved for the user to fix and resume.
					pausedSlugs[r.Slug] = true
					allFailures = append(allFailures, r)
				} else {
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", r.Slug, r.Err)
					allFailures = append(allFailures, r)
					failedSlugs[r.Slug] = true
				}
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", r.Slug)
				waveSuccesses = append(waveSuccesses, r)
			}
		}

		// Merge this wave's successes before proceeding to next wave
		// State is NOT committed before merge — only after successful merge.
		// This prevents "phantom completion" where state says complete but code never merged.
		for i, s := range waveSuccesses {
			// Ensure repo is in a clean merge state before each merge
			if err := ensureCleanMergeState(cfg.Root); err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ %s — skipping remaining %d merge(s)\033[0m\n", err, len(waveSuccesses)-i)
				for _, remaining := range waveSuccesses[i:] {
					allFailures = append(allFailures, featureResult{Slug: remaining.Slug, Err: fmt.Errorf("skipped: unclean merge state")})
					failedSlugs[remaining.Slug] = true
				}
				break
			}
			if err := mergeFeatureBranch(cfg, s.Slug, s.Branch, s.WorktreePath, activeWorktrees); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", s.Slug, err)
				fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", s.WorktreePath)
				fmt.Fprintf(os.Stderr, "  Branch: %s\n", s.Branch)
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: err})
				failedSlugs[s.Slug] = true
			} else {
				totalMerged++
			}
		}
	}

	// Sync master PROGRESS.md with actual feature states after all merges
	featuresDir = filepath.Join(cfg.Root, ".belmont", "features")
	syncMasterFeatureStatuses(cfg.Root, listFeatures(featuresDir, 50))

	// Commit any remaining .belmont/ state changes after all merges
	if err := commitBelmontState(cfg.Root); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to commit final .belmont/ state: %s\033[0m\n", err)
	}

	// Report
	if totalMerged == 0 && len(pausedSlugs) > 0 {
		// Halt summary: nothing merged and at least one feature paused.
		// Group paused features and dependents-skipped-due-to-paused so the
		// user sees the root cause instead of a flat "N failed" list.
		var paused, skipped []featureResult
		for _, f := range allFailures {
			if pausedSlugs[f.Slug] {
				// Distinguish "actually paused" (originated the pause) from
				// "skipped because dep paused" using the error message
				// shape — skip-due-to-pause uses "dependency X paused".
				if f.Err != nil && strings.HasPrefix(f.Err.Error(), "dependency ") {
					skipped = append(skipped, f)
				} else {
					paused = append(paused, f)
				}
			}
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⏸ %d feature(s) paused (no merges made):\033[0m\n", len(paused))
		for _, f := range paused {
			fmt.Fprintf(os.Stderr, "  %s\n", f.Slug)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "\033[33m⊘ %d feature(s) skipped:\033[0m\n", len(skipped))
			for _, f := range skipped {
				fmt.Fprintf(os.Stderr, "  %s — %s\n", f.Slug, f.Err)
			}
		}
		fmt.Fprintf(os.Stderr, "Fix the blocker(s) and rerun.\n")
	} else if len(allFailures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d feature(s) failed:\033[0m\n", len(allFailures))
		for _, f := range allFailures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Slug, f.Err)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
	}

	// Clean up auto.json now that all features are processed
	activeWorktrees.removeAutoJSON()

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ %d/%d features complete\033[0m (%.1fs total)\n", totalMerged, len(slugs), time.Since(startTime).Seconds())

	if len(allFailures) > 0 {
		return fmt.Errorf("auto: %d feature(s) failed", len(allFailures))
	}
	return nil
}
