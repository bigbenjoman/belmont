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

// gapReported reports whether the overlay already filed this milestone as one
// it could not read, so the same condition is not announced twice.
func gapReported(gaps []liveOverlayGap, milestoneID string) bool {
	for _, g := range gaps {
		if g.Milestone == milestoneID {
			return true
		}
	}
	return false
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
	var liveGaps []liveOverlayGap
	if parallelLive {
		milestones, liveGaps = overlayLiveMilestones(milestones, perMilestoneLive)
		msIDs := make([]string, 0, len(perMilestoneLive))
		for id := range perMilestoneLive {
			msIDs = append(msIDs, id)
		}
		sort.Strings(msIDs) // map order would vary the report between runs
		for _, id := range msIDs {
			wtPath := filepath.Join(perMilestoneLive[id], "PROGRESS.md")
			b, err := os.ReadFile(wtPath)
			if err != nil {
				// Say so rather than skip. This lint gates `belmont auto`, so a
				// worktree whose PROGRESS.md cannot be read means the green it
				// prints covers less than the user thinks.
				//
				// Printed only when the overlay did not already file this
				// milestone as a gap — since #48 that is the normal case, and the
				// violation it raises is both machine-readable and carried in the
				// report. This stderr line remains for the narrow race where the
				// overlay read the file and this second read did not.
				if !gapReported(liveGaps, id) {
					fmt.Fprintf(os.Stderr,
						"\033[33m⚠ %s: could not read %s (%v) — %s was linted against master's copy, not this worktree's\033[0m\n",
						slug, wtPath, err, id)
				}
				continue
			}
			docs = append(docs, string(b))
		}
	}

	v := detectViolations(slug, milestones)
	// A milestone the overlay could not source from its worktree was linted
	// against master's copy, so the result covers less than it appears to. That
	// is exactly the claim a gate must not make silently: since #42 this lint is
	// what `belmont validate` reports and since #44 it is what `belmont auto`
	// starts on. Warning severity, deliberately — a half-cleaned worktree is
	// information Belmont refuses to drop, not a reason to refuse a run that
	// worked yesterday. `--strict` is what makes CI fail on it. See issue #48.
	for _, g := range liveGaps {
		v = append(v, validationViolation{
			Feature:   slug,
			Milestone: g.Milestone,
			Rule:      ruleUnreadableLiveMilestone,
			// No remedy: both of them answer "what should this line say", and
			// this finding is not about a line. The fix is to the worktree, and
			// the command for it is in the message.
			Severity: severityWarning,
			Message: fmt.Sprintf(
				"%s was linted against master's copy: its live worktree state could not be read — %s. A violation raised inside that worktree does not appear in this report. Recover or clean the worktree up with `belmont recover --list`.",
				g.Milestone, g.Reason),
		})
	}
	// validationViolation is all-string, comparable, and carries no line number,
	// so the same violation seen in master and in a worktree's copy dedupes
	// cleanly.
	seen := make(map[validationViolation]bool, len(v))
	for _, x := range v {
		seen[x] = true
	}
	// Milestone-structure violations are unioned across the same documents, not
	// read off the overlay alone.
	//
	// The overlay cannot carry them: overlayLiveMilestones REPLACES a milestone
	// master already has and never appends one it does not, so a `### M<n>:`
	// block that exists only inside a worktree — a polish milestone an agent
	// invented mid-run, or a second heading for an ID that already exists — was
	// dropped before detectViolations ever saw it. Reading the representative
	// worktree's whole file used to catch that by accident; overlaying stopped.
	// Both rules are severityError, and a duplicate heading is precisely the
	// case where runScopeGuard and runEvidenceCheck both decline, so this lint
	// is the last check standing.
	for _, doc := range docs[1:] {
		for _, x := range detectViolations(slug, parseMilestones(doc)) {
			if seen[x] {
				continue
			}
			seen[x] = true
			v = append(v, x)
		}
	}
	// Orphan detection is document-level for the same reason it is unioned
	// rather than overlaid: a task line outside every milestone lives in one
	// specific file.
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
