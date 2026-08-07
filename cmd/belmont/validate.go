package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug (default: scan every feature)")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
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

	if len(violations) > 0 {
		return fmt.Errorf("validate: found %d milestone-structure violation(s)", len(violations))
	}
	return nil
}

// validateFeature reads a feature's PROGRESS.md and returns any violations.
func validateFeature(root, slug string) ([]validationViolation, error) {
	featuresDir := filepath.Join(root, ".belmont", "features", slug)
	if !dirExists(featuresDir) {
		return nil, fmt.Errorf("feature %q not found at %s", slug, featuresDir)
	}
	// Prefer live worktree state if one exists for this feature, same as
	// `belmont status` does.
	progressPath := filepath.Join(featuresDir, "PROGRESS.md")
	if override := loadAutoWorktrees(root); override != nil {
		if wtFeature, ok := override[slug]; ok {
			if p := filepath.Join(wtFeature, "PROGRESS.md"); fileExists(p) {
				progressPath = p
			}
		}
	}
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", progressPath, err)
	}
	milestones := parseMilestones(string(data))
	return detectViolations(slug, milestones), nil
}
