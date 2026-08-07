package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// loadAutoWorktrees reads .belmont/auto.json and returns a map of feature slug → worktree feature path.
// If auto.json doesn't exist or isn't active, returns nil. In multi-feature
// mode the map key is the feature slug (one worktree per feature). In single-
// feature parallel mode there may be multiple worktrees for one feature; this
// function picks a representative (alphabetically first milestone's worktree)
// so legacy callers that look up "is there a worktree for this feature" still
// get a sensible answer. New callers that need per-milestone live state
// should use loadAutoWorktreeStateByMilestone.
func loadAutoWorktrees(root string) map[string]string {
	aj := readActiveAutoJSONOrNil(root)
	if aj == nil {
		return nil
	}
	result := make(map[string]string)
	if aj.Mode == "single-feature-parallel" && aj.Feature != "" {
		// All entries are milestones under this one feature. Pick the
		// alphabetically first milestone as the representative path.
		var keys []string
		for id := range aj.Worktrees {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			wtFeaturePath := filepath.Join(aj.Worktrees[id].Path, ".belmont", "features", aj.Feature)
			if dirExists(wtFeaturePath) {
				result[aj.Feature] = wtFeaturePath
				break
			}
		}
		return result
	}
	// Multi-feature (or legacy) mode: keys are feature slugs.
	for slug, entry := range aj.Worktrees {
		wtFeaturePath := filepath.Join(entry.Path, ".belmont", "features", slug)
		if dirExists(wtFeaturePath) {
			result[slug] = wtFeaturePath
		}
	}
	return result
}

func runAutoCmd(args []string) error {
	fs := flag.NewFlagSet("auto", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg loopConfig
	var policyStr string
	var featuresFlag string
	var allFlag bool
	var allowDirty bool
	fs.StringVar(&cfg.Feature, "feature", "", "feature slug (required)")
	fs.StringVar(&featuresFlag, "features", "", "comma-separated feature slugs for parallel execution")
	fs.BoolVar(&allFlag, "all", false, "run all pending features in parallel")
	fs.StringVar(&cfg.From, "from", "", "start milestone (e.g. M1)")
	fs.StringVar(&cfg.To, "to", "", "end milestone (e.g. M5)")
	fs.StringVar(&cfg.Tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor|pi|opencode)")
	fs.StringVar(&policyStr, "policy", "autonomous", "checkpoint policy (autonomous|milestone|every_action)")
	fs.IntVar(&cfg.MaxIterations, "max-iterations", 50, "maximum loop iterations")
	fs.IntVar(&cfg.MaxFailures, "max-failures", 3, "consecutive failures before stopping")
	fs.IntVar(&cfg.MaxParallel, "max-parallel", 5, "max concurrent goroutines for parallel execution")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "show execution plan without running")
	fs.BoolVar(&allowDirty, "allow-dirty", false, "skip the clean-working-tree check (not recommended — risks merge failures)")
	fs.StringVar(&cfg.Root, "root", ".", "project root")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Validate mutual exclusivity
	multiFeature := featuresFlag != "" || allFlag
	if multiFeature && cfg.Feature != "" {
		return fmt.Errorf("auto: --feature cannot be combined with --features or --all")
	}
	if featuresFlag != "" && allFlag {
		return fmt.Errorf("auto: --features and --all are mutually exclusive")
	}
	if multiFeature && (cfg.From != "" || cfg.To != "") {
		return fmt.Errorf("auto: --from/--to cannot be used with --features or --all")
	}
	if !multiFeature && cfg.Feature == "" {
		return fmt.Errorf("auto: --feature is required (or use --features/--all for multi-feature mode)")
	}

	switch checkpointPolicy(policyStr) {
	case policyAutonomous, policyMilestone, policyEveryAction:
		cfg.Policy = checkpointPolicy(policyStr)
	default:
		return fmt.Errorf("auto: invalid --policy %q (use autonomous, milestone, or every_action)", policyStr)
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = absRoot

	// Auto-detect tool if not specified
	if cfg.Tool == "" {
		detected := detectTool()
		if detected == "" {
			return fmt.Errorf("auto: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor, pi, opencode\nInstall one or use --tool to specify")
		}
		cfg.Tool = detected
	} else {
		// Validate tool name
		switch cfg.Tool {
		case "claude", "codex", "gemini", "copilot", "cursor", "pi", "opencode":
			// ok
		default:
			return fmt.Errorf("auto: unsupported tool %q (use claude, codex, gemini, copilot, cursor, pi, or opencode)", cfg.Tool)
		}
	}

	// Refuse to start against a dirty working tree — uncommitted changes risk
	// blocking later worktree merges. Skipped on --dry-run (no merges happen)
	// and --allow-dirty (explicit opt-out).
	if !allowDirty && !cfg.DryRun {
		if err := requireCleanWorkingTree(absRoot); err != nil {
			return err
		}
	}

	// Multi-feature mode: --features or --all
	if multiFeature {
		slugs, err := resolveFeatureSlugs(absRoot, featuresFlag, allFlag)
		if err != nil {
			return err
		}
		return runAutoMultiFeature(cfg, slugs)
	}

	// Single-feature mode
	// Verify feature directory exists
	featureDir := filepath.Join(absRoot, ".belmont", "features", cfg.Feature)
	if !dirExists(featureDir) {
		return fmt.Errorf("auto: feature %q not found at %s", cfg.Feature, featureDir)
	}

	// Load per-feature model tiers (if models.yaml exists)
	tiers, tierErr := parseModelTiers(filepath.Join(featureDir, "models.yaml"))
	if tierErr != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ failed to parse models.yaml: %s — falling back to defaults\033[0m\n", tierErr)
	}
	cfg.ModelTiers = tiers

	// Read milestones and check for dependency syntax
	progressPath := filepath.Join(absRoot, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("auto: failed to read PROGRESS.md: %w", err)
	}
	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, cfg.From, cfg.To)

	// Structural lint: warn on polish/follow-up milestone patterns before
	// starting the loop. These are the exact shape that causes parallel merge
	// conflicts (per skills/belmont/_partials/milestone-immutability.md).
	// Prompt the user to continue or abort; non-interactive runs abort on
	// violations to avoid silent damage.
	if violations := detectViolations(cfg.Feature, milestones); len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "\033[31m✗ Milestone-structure violation(s) detected:\033[0m\n\n")
		renderValidationReport(os.Stderr, violations)
		if !isTerminal(os.Stdin) {
			return fmt.Errorf("auto: %d milestone-structure violation(s); restructure via `/belmont:tech-plan` before rerunning, or run with a TTY to override interactively", len(violations))
		}
		fmt.Fprintf(os.Stderr, "Proceed anyway? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			return fmt.Errorf("auto: aborted — restructure milestones via `/belmont:tech-plan` then rerun")
		}
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Proceeding despite violations — you are on your own for merge fallout\033[0m\n")
	} else {
		fmt.Fprintf(os.Stderr, "\033[32m✓\033[0m milestone structure valid (%d milestone(s) scanned, no polish/follow-up patterns)\n", len(milestones))
	}

	// Interactive milestone selection when stdin is a terminal and no --from/--to
	if cfg.From == "" && cfg.To == "" && isTerminal(os.Stdin) {
		selectedFrom, selectedTo, err := interactiveMilestoneSelect(inRange)
		if err != nil {
			return err
		}
		cfg.From = selectedFrom
		cfg.To = selectedTo
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (single-feature) — %s\033[0m\n", cfg.Feature)
		fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s\033[0m\n", cfg.Tool, cfg.Policy)
		if cfg.From != "" || cfg.To != "" {
			fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", cfg.From, cfg.To)
		}
		fmt.Fprintf(os.Stderr, "\n\033[1mMilestones:\033[0m\n")
		for _, m := range inRange {
			status := "pending"
			if milestoneAllVerified(m) {
				status = "verified"
			} else if milestoneAllDone(m) {
				status = "done"
			} else if !milestoneNotStarted(m) {
				status = "in progress"
			}
			fmt.Fprintf(os.Stderr, "  • %s — %s [%s]\n", m.ID, m.Name, status)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// Check if any milestones have explicit dependencies
	hasExplicitDeps := false
	for _, m := range inRange {
		if len(m.Deps) > 0 {
			hasExplicitDeps = true
			break
		}
	}

	if hasExplicitDeps {
		return runAutoParallel(cfg, inRange)
	}

	return runLoop(cfg)
}

// requireCleanWorkingTree returns an error if `git status --porcelain` reports
// any uncommitted, unstaged, or untracked changes in root. The error message
// is shaped for direct printing — coloured headers, a truncated path list,
// and resolution hints. If root isn't a git repo, returns nil (no block).
func requireCleanWorkingTree(root string) error {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=normal")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	// Trim only trailing newline — porcelain lines start with a status code in
	// columns 0-1 that may include a leading space (e.g. " M path"), so a full
	// TrimSpace would corrupt the first line's path offset.
	output := strings.TrimRight(string(out), "\n")
	if output == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	belmontHits := 0
	for _, ln := range lines {
		if pathIsBelmontManaged(porcelainPath(ln)) {
			belmontHits++
		}
	}

	const maxList = 20
	listed := lines
	truncated := 0
	if len(listed) > maxList {
		truncated = len(listed) - maxList
		listed = listed[:maxList]
	}

	var sb strings.Builder
	sb.WriteString(ansiRed + "working tree is not clean — refusing to start auto" + ansiReset + "\n\n")
	sb.WriteString("The following files have uncommitted, unstaged, or untracked changes:\n")
	for _, ln := range listed {
		sb.WriteString("  ")
		sb.WriteString(ln)
		sb.WriteString("\n")
	}
	if truncated > 0 {
		sb.WriteString(fmt.Sprintf("  ... and %d more\n", truncated))
	}
	sb.WriteString("\n")
	if belmontHits > 0 {
		sb.WriteString(ansiYellow + "Looks like a recent `belmont update` left files uncommitted." + ansiReset + "\n")
		sb.WriteString("If a worktree merges back into this branch later, those uncommitted files will block the merge.\n\n")
	} else {
		sb.WriteString(ansiYellow + "If a worktree merges back into this branch later, uncommitted files may block the merge." + ansiReset + "\n\n")
	}
	sb.WriteString("Resolve with one of:\n")
	sb.WriteString("  git stash -u                  # stash changes (incl. untracked) and start clean\n")
	sb.WriteString("  git commit -am \"...\"          # commit your changes\n")
	sb.WriteString("  belmont auto --allow-dirty    # skip this check (not recommended)")
	return errors.New(sb.String())
}

// autoResolveBelmontConflicts attempts to auto-resolve merge conflicts on .belmont/ files.
// For PROGRESS.md files, it takes the union of milestone completions (each milestone marks
// only its own [x] status, so union is safe). Returns true if any conflicts were resolved.
func autoResolveBelmontConflicts(root string) bool {
	// Get list of conflicted files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	resolved := false
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if !strings.HasPrefix(file, ".belmont/") {
			continue
		}

		filePath := filepath.Join(root, file)

		if strings.HasSuffix(file, "PROGRESS.md") {
			// For PROGRESS.md: get both sides and merge milestone completions
			if resolveProgressConflict(root, file, filePath) {
				resolved = true
			}
		} else {
			// For other .belmont/ files (e.g., MILESTONE.md): take "theirs" (the branch being merged)
			checkoutCmd := exec.Command("git", "checkout", "--theirs", "--", file)
			checkoutCmd.Dir = root
			if _, err := checkoutCmd.CombinedOutput(); err == nil {
				addCmd := exec.Command("git", "add", file)
				addCmd.Dir = root
				addCmd.Run()
				resolved = true
			}
		}
	}
	return resolved
}
