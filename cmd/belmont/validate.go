package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// validateFeatureDeps checks for dangling dependency references and cycles.
func validateFeatureDeps(features []featureSummary, allKnown []featureSummary) error {
	// Build slug set from ALL known features so completed deps are recognized
	slugSet := make(map[string]bool)
	for _, f := range allKnown {
		slugSet[f.Slug] = true
	}

	// Check for dangling references — collect all errors
	var errs []string
	for _, f := range features {
		for _, dep := range f.Deps {
			if !slugSet[dep] {
				errs = append(errs, fmt.Sprintf("feature %q depends on %q which does not exist", f.Slug, dep))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	// Check for cycles by attempting wave computation
	_, err := computeFeatureWaves(features)
	return err
}

// validateRepoState checks that the repo is in a clean state suitable for auto mode.
// Returns an error if there's an in-progress merge, rebase, or unmerged files.
func validateRepoState(root string) error {
	// Resolve .git dir (could be a file in a worktree)
	gitDir := filepath.Join(root, ".git")

	// Check no in-progress merge
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return fmt.Errorf("repository has an in-progress merge — resolve with 'git merge --abort' or 'git merge --continue' first")
	}
	// Check no in-progress rebase
	if dirExists(filepath.Join(gitDir, "rebase-merge")) || dirExists(filepath.Join(gitDir, "rebase-apply")) {
		return fmt.Errorf("repository has an in-progress rebase — resolve first")
	}
	// Check no unmerged files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("repository has unmerged files — resolve conflicts first:\n%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func runSyncCmd(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	fs.StringVar(&root, "root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	root, _ = filepath.Abs(root)

	// If running in a worktree context (detected via .worktree marker), no-op.
	// Worktrees have isolated state; sync only makes sense on the main repo.
	if fileExists(filepath.Join(root, ".belmont", ".worktree")) {
		return nil
	}

	featuresDir := filepath.Join(root, ".belmont", "features")
	features := listFeatures(featuresDir, 50)
	if len(features) == 0 {
		return nil
	}

	syncMasterFeatureStatuses(root, features)

	// Commit if anything changed (best-effort)
	commitBelmontState(root)
	return nil
}

// runValidateCmd implements `belmont validate`.
func runValidateCmd(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, format string
	var strict bool
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug (default: scan every feature)")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
	fs.BoolVar(&strict, "strict", false, "exit non-zero on warnings as well as errors")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("validate: resolve root: %w", err)
	}

	var violations []validationViolation
	if feature != "" {
		v, err := validateFeature(absRoot, feature)
		if err != nil {
			return err
		}
		violations = v
	} else {
		featuresDir := filepath.Join(absRoot, ".belmont", "features")
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			return fmt.Errorf("validate: read features dir %s: %w", featuresDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			v, err := validateFeature(absRoot, entry.Name())
			if err != nil {
				// Missing PROGRESS.md etc. is not fatal across features.
				fmt.Fprintf(os.Stderr, "\033[33m⚠ %s: %s\033[0m\n", entry.Name(), err)
				continue
			}
			violations = append(violations, v...)
		}
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(violations); err != nil {
			return err
		}
	default:
		renderValidationReport(os.Stdout, violations)
	}

	// Only errors fail the command. A warning is information Belmont refuses to
	// drop, not a reason to stop — upgrading must not turn a working project
	// into a failing one over a `- [ ]` bullet in a retro. `--strict` is for CI,
	// where "tell me about everything" is the right default.
	blocking, advisory := splitBySeverity(violations)
	if len(blocking) > 0 {
		return fmt.Errorf("validate: found %d milestone-structure violation(s)", len(blocking))
	}
	if strict && len(advisory) > 0 {
		return fmt.Errorf("validate: found %d warning(s) and --strict was set", len(advisory))
	}
	return nil
}

// validateFeature reads a feature's PROGRESS.md and returns any violations.
func validateFeature(root, slug string) ([]validationViolation, error) {
	featuresDir := filepath.Join(root, ".belmont", "features", slug)
	if !dirExists(featuresDir) {
		return nil, fmt.Errorf("feature %q not found at %s", slug, featuresDir)
	}
	// Prefer live worktree state if one exists, in the same two shapes
	// `belmont status` reads it: a whole-feature override for serial and
	// multi-feature runs, a per-milestone overlay for single-feature-parallel.
	//
	// Reading only the collapsed representative worktree made `belmont validate`
	// blind to a violation raised in any other milestone's worktree — issue #42's
	// defect in a different reader.
	//
	// This is now also what `belmont auto` lints with at startup on the
	// single-feature path. That gate used to hand-roll the same checks against
	// master's PROGRESS.md alone, so the two disagreed about a worktree-raised
	// violation and a resumed run could start on a file `belmont validate` exits
	// 1 on. Note what routing it here does and does not buy: the gate runs once,
	// before the loop, so it sees worktrees only when a run is resumed. Drift
	// during a run is runScopeGuard and runEvidenceCheck's job, not this one.
	progressPath := filepath.Join(featuresDir, "PROGRESS.md")
	liveFeature, perMilestoneLive := loadAutoWorktreeStateByMilestone(root)
	parallelLive := perMilestoneLive != nil && slug == liveFeature
	if !parallelLive {
		if override := loadAutoWorktrees(root); override != nil {
			if wtFeature, ok := override[slug]; ok {
				if p := filepath.Join(wtFeature, "PROGRESS.md"); fileExists(p) {
					progressPath = p
				}
			}
		}
	}
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", progressPath, err)
	}

	milestones := parseMilestones(string(data))
	// Orphan detection is document-level, so the milestone overlay cannot carry
	// it: a task line outside every milestone exists in one specific file. Scan
	// master plus each live worktree and union. An orphan is additive by nature,
	// and one raised inside a worktree is invisible to every other view.
	docs := []string{string(data)}
	if parallelLive {
		milestones = overlayLiveMilestones(milestones, perMilestoneLive)
		msIDs := make([]string, 0, len(perMilestoneLive))
		for id := range perMilestoneLive {
			msIDs = append(msIDs, id)
		}
		sort.Strings(msIDs) // map order would vary the report between runs
		for _, id := range msIDs {
			if b, err := os.ReadFile(filepath.Join(perMilestoneLive[id], "PROGRESS.md")); err == nil {
				docs = append(docs, string(b))
			}
		}
	}

	v := detectViolations(slug, milestones)
	// validationViolation is all-string, comparable, and carries no line number,
	// so the same orphan seen in master and in a worktree's copy dedupes cleanly.
	seen := make(map[validationViolation]bool, len(v))
	for _, x := range v {
		seen[x] = true
	}
	for _, doc := range docs {
		for _, o := range detectOrphanViolations(slug, doc) {
			if seen[o] {
				continue
			}
			seen[o] = true
			v = append(v, o)
		}
	}
	return v, nil
}
