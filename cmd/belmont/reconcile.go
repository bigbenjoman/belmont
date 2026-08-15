package main

import (
	"bufio"
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

// runReconciliationAgent orchestrates two-pass merge conflict resolution.
// Pass 1: AI analyzes conflicts and writes a structured report.
// Pass 2: Go auto-applies high-confidence resolutions and prompts for low-confidence ones.
// Falls back to legacy full-resolve if the report is invalid.
func runReconciliationAgent(cfg loopConfig, milestoneID, branch string) error {
	// Get list of conflicted files
	conflictCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	conflictCmd.Dir = cfg.Root
	conflictOut, err := conflictCmd.Output()
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}

	conflictedFiles := strings.TrimSpace(string(conflictOut))
	if conflictedFiles == "" {
		return fmt.Errorf("no conflicted files found")
	}

	reportPath := filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json")

	// Pass 1: AI analysis — writes structured report to disk
	if err := runReconciliationAnalysis(cfg, milestoneID, branch, conflictedFiles, reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Analysis failed, falling back to legacy resolve...\033[0m\n")
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Read and parse the report
	report, err := parseReconciliationReport(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Invalid report (%v), falling back to legacy resolve...\033[0m\n", err)
		os.Remove(reportPath)
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Pass 2: Apply resolutions
	if err := applyReconciliationReport(cfg, report); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Verify no conflict markers remain
	var resolvedFiles []string
	for _, f := range report.Files {
		resolvedFiles = append(resolvedFiles, f.File)
	}
	if err := verifyNoConflictMarkers(cfg.Root, resolvedFiles); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Clean up report file
	os.Remove(reportPath)
	return nil
}

// runReconciliationAnalysis invokes the AI to analyze conflicts and write a JSON report.
func runReconciliationAnalysis(cfg loopConfig, milestoneID, branch, conflictedFiles, reportPath string) error {
	prompt := fmt.Sprintf(`You are a merge conflict analysis agent. Read the reconciliation-agent instructions first, then analyze all merge conflicts.

CRITICAL: Read the file .agents/belmont/reconciliation-agent.md (or agents/belmont/reconciliation-agent.md) for your full instructions and merge strategies by file type. Those instructions are authoritative.

CORE PRINCIPLE: Every merge MUST produce a strictly better state than either side alone. Both branches represent intentional, completed, tested work. You are COMBINING parallel features — never choosing between them. If a resolution would lose ANY code, functionality, dependencies, or tracking state from either side, mark it as "unresolvable" instead of attempting a lossy merge. A blocked merge is ALWAYS preferable to a destructive one.

Conflicted files:
%s

Milestone/Feature: %s
Branch: %s

TASK: For each conflicted file:
1. Read the file to see the conflict markers
2. Understand what each side intended (both sides are valid completed work)
3. Determine the merge strategy based on file type (see agent instructions)
4. Combine BOTH sides — verify nothing is lost
5. Classify your confidence

CONFIDENCE LEVELS:
- "high": Both sides combined with certainty nothing is lost (import unions, additive functions, config entries from different features)
- "low": Both sides combined but semantic interaction possible (same function modified, overlapping config). Operator will review.
- "unresolvable": Cannot combine without losing something. Leave resolved_content empty. Merge will abort.

Write a JSON file to: %s

The JSON must have this exact structure:
{
  "files": [
    {
      "file": "path/to/file",
      "confidence": "high",
      "strategy": "brief strategy label (e.g. import-union, package-manifest-union, additive-functions, lock-regen)",
      "reason": "Why this confidence level",
      "conflict_summary": "Brief: what Side A did vs what Side B did",
      "resolved_content": "The complete resolved file content (no conflict markers)",
      "post_resolve_command": "optional: shell command to run after (e.g. npm install, npx prisma generate)"
    }
  ]
}

RULES:
1. ALWAYS combine both sides — never choose one side over the other. This is non-negotiable.
2. Include all imports from both sides (remove exact duplicates only)
3. Never delete functionality from either side — all completed work must survive
4. For lock files (package-lock.json, yarn.lock, etc.): set resolved_content to empty string and post_resolve_command to the install command
5. For package manifests: take the union of all dependency additions from both sides
6. The resolved_content must be the COMPLETE file with conflicts resolved
7. Do NOT modify any files on disk — only write the JSON report
8. Do NOT run git add — only write the report
9. Include ALL conflicted files in the report
10. When in doubt, mark "unresolvable" — blocking is safer than losing work`, conflictedFiles, milestoneID, branch, reportPath)

	// Reconciliation needs strong reasoning — use configured tier (defaults to high).
	flags := resolveModelFlags(cfg.Tool, reconciliationTier(cfg.ModelTiers), cfg.Root)
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, flags...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// parseReconciliationReport reads and validates the JSON report file.
func parseReconciliationReport(reportPath string) (reconciliationReport, error) {
	var report reconciliationReport

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return report, fmt.Errorf("read report: %w", err)
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("parse report: %w", err)
	}

	if len(report.Files) == 0 {
		return report, fmt.Errorf("report contains no files")
	}

	// Validate each file entry
	for i, f := range report.Files {
		if f.File == "" {
			return report, fmt.Errorf("file entry %d missing file path", i)
		}
		if f.Confidence != "high" && f.Confidence != "low" && f.Confidence != "unresolvable" {
			return report, fmt.Errorf("file %q has invalid confidence %q", f.File, f.Confidence)
		}
		if f.ResolvedContent == "" && f.Confidence != "unresolvable" && f.PostResolveCmd == "" {
			return report, fmt.Errorf("file %q has empty resolved_content (mark as unresolvable if it cannot be resolved)", f.File)
		}
	}

	return report, nil
}

// applyReconciliationReport applies resolved content from the report.
// High-confidence files are auto-applied. Low-confidence files are shown
// to the user interactively (if terminal) or auto-applied (if non-interactive).
// If ANY file is unresolvable, the entire merge is aborted.
func applyReconciliationReport(cfg loopConfig, report reconciliationReport) error {
	interactive := isTerminal(os.Stdin)
	autoAll := false

	// Check for unresolvable files first — abort before applying anything
	var unresolvable []reconciliationFile
	var highCount, lowCount int
	for _, f := range report.Files {
		switch f.Confidence {
		case "unresolvable":
			unresolvable = append(unresolvable, f)
		case "high":
			highCount++
		default:
			lowCount++
		}
	}

	if len(unresolvable) > 0 {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ %d file(s) marked unresolvable — aborting merge:\033[0m\n", len(unresolvable))
		for _, f := range unresolvable {
			fmt.Fprintf(os.Stderr, "    %s: %s\n", f.File, f.Reason)
		}
		return fmt.Errorf("unresolvable conflicts in %d file(s)", len(unresolvable))
	}

	if highCount > 0 {
		fmt.Fprintf(os.Stderr, "  \033[32m✓ Auto-applying %d high-confidence resolution(s)\033[0m\n", highCount)
	}
	if lowCount > 0 && interactive {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ %d file(s) need review\033[0m\n", lowCount)
	}

	// Collect post-resolve commands (deduped, ordered)
	var postCmds []string
	seenCmds := make(map[string]bool)

	for _, f := range report.Files {
		filePath := filepath.Join(cfg.Root, f.File)

		// Track post-resolve commands
		if f.PostResolveCmd != "" && !seenCmds[f.PostResolveCmd] {
			postCmds = append(postCmds, f.PostResolveCmd)
			seenCmds[f.PostResolveCmd] = true
		}

		// Skip writing files that will be regenerated by post-resolve commands
		// (e.g., lock files with empty resolved_content)
		if f.ResolvedContent == "" && f.PostResolveCmd != "" {
			// Delete the conflicted file so the post-resolve command regenerates it
			os.Remove(filePath)
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			addCmd.Run()
			continue
		}

		if f.Confidence == "high" || !interactive || autoAll {
			// Auto-apply
			if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
			continue
		}

		// Low confidence + interactive: prompt user
		choice, err := reviewConflict(cfg.Root, f)
		if err != nil {
			return err
		}

		switch choice {
		case "auto":
			autoAll = true
			// Apply this file and all remaining
			if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		case "accept", "edited":
			// File already written by reviewConflict for "edited", write for "accept"
			if choice == "accept" {
				if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
					return fmt.Errorf("write %s: %w", f.File, err)
				}
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		}
	}

	// Run post-resolve commands (e.g., npm install to regen lock files)
	if len(postCmds) > 0 {
		for _, cmd := range postCmds {
			fmt.Fprintf(os.Stderr, "  \033[2mRunning post-resolve: %s\033[0m\n", cmd)
			parts := strings.Fields(cmd)
			postCmd := exec.Command(parts[0], parts[1:]...)
			postCmd.Dir = cfg.Root
			if out, err := postCmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠ Post-resolve command failed: %s\n%s\033[0m\n", cmd, strings.TrimSpace(string(out)))
				// Don't fail the merge — the operator can fix this
			}
		}
		// Stage any files generated by post-resolve commands
		addCmd := exec.Command("git", "add", "-A")
		addCmd.Dir = cfg.Root
		addCmd.Run()

		// `git add -A` sweeps up transient reconciliation artifacts too.
		// Unstage the report explicitly so the merge commit doesn't include it;
		// the caller deletes it from disk right after this returns.
		unstage := exec.Command("git", "rm", "--cached", "--ignore-unmatch", "--", ".belmont/reconciliation-report.json")
		unstage.Dir = cfg.Root
		unstage.Run()
	}

	return nil
}

// writeReconciliationResolution writes a resolved conflict file to disk, handling:
//   - Missing parent directories (git conflict handling can leave them unpopulated)
//   - Existing symlinks at the target path (os.WriteFile would follow the symlink
//     and fail if the symlink is broken or points at a directory)
//   - Resolved content that is itself a symlink target (short single-line path) —
//     recreates as a symlink to preserve the original file type
func writeReconciliationResolution(filePath, content string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	// If the existing path is a symlink, remove it so we can decide fresh
	// whether to write a regular file or recreate a symlink. Otherwise WriteFile
	// follows the symlink and fails when the target is a dir or a broken path.
	wasSymlink := false
	if info, err := os.Lstat(filePath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		wasSymlink = true
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}

	// If the original was a symlink and the resolved content looks like a
	// single-line path (no embedded newlines, reasonable length), recreate as
	// a symlink rather than writing it as a text file.
	trimmed := strings.TrimSpace(content)
	if wasSymlink && !strings.ContainsAny(trimmed, "\n\x00") && len(trimmed) > 0 && len(trimmed) < 1024 {
		return os.Symlink(trimmed, filePath)
	}

	return os.WriteFile(filePath, []byte(content), 0o644)
}

// reviewConflict prompts the user to review a low-confidence conflict resolution.
func reviewConflict(root string, f reconciliationFile) (string, error) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Fprintf(os.Stderr, "\n  \033[1;33m⚠ Uncertain conflict in %s\033[0m\n", f.File)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.Reason)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.ConflictSummary)
		fmt.Fprintf(os.Stderr, "    [a] Accept AI's resolution  [v] View proposed resolution\n")
		fmt.Fprintf(os.Stderr, "    [e] Edit in $EDITOR         [s] Auto-resolve all remaining  [q] Abort\n\n")
		fmt.Fprintf(os.Stderr, "    Choice [a]: ")

		if !scanner.Scan() {
			return "", fmt.Errorf("reconciliation: no input")
		}
		input := strings.TrimSpace(strings.ToLower(scanner.Text()))

		if input == "" || input == "a" {
			return "accept", nil
		}

		switch input {
		case "v":
			// Show resolved content with line numbers
			fmt.Fprintf(os.Stderr, "\n    \033[2m--- Proposed resolution for %s ---\033[0m\n", f.File)
			lines := strings.Split(f.ResolvedContent, "\n")
			for i, line := range lines {
				fmt.Fprintf(os.Stderr, "    \033[2m%4d\033[0m  %s\n", i+1, line)
			}
			fmt.Fprintf(os.Stderr, "    \033[2m--- End ---\033[0m\n")
			// Re-prompt
			continue

		case "e":
			// Write proposed resolution to file for editing
			filePath := filepath.Join(root, f.File)
			if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
				return "", fmt.Errorf("write for edit %s: %w", f.File, err)
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.Command(editor, filePath)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			if err := editorCmd.Run(); err != nil {
				return "", fmt.Errorf("editor for %s: %w", f.File, err)
			}
			return "edited", nil

		case "s":
			return "auto", nil

		case "q":
			return "", fmt.Errorf("reconciliation: aborted by user")

		default:
			fmt.Fprintf(os.Stderr, "    Invalid choice. Try again.\n")
		}
	}
}

// runLegacyReconciliation is the fallback: AI resolves everything directly on disk.
func runLegacyReconciliation(cfg loopConfig, milestoneID, branch, conflictedFiles string) error {
	prompt := fmt.Sprintf(`You are a merge conflict reconciliation agent. Read your instructions first, then resolve all merge conflicts.

CRITICAL: Read the file .agents/belmont/reconciliation-agent.md (or agents/belmont/reconciliation-agent.md) for full instructions and merge strategies by file type.

CORE PRINCIPLE: Every merge MUST produce a strictly better state than either side alone. Both branches are intentional, completed, tested work. You are COMBINING parallel features — never choosing between them. If resolving a file would lose ANY code, functionality, dependencies, or state from either side, leave it conflicted and report the failure. A blocked merge is ALWAYS preferable to a destructive one.

Conflicted files:
%s

Milestone/Feature: %s
Branch: %s

Rules:
1. ALWAYS combine both sides — never choose one side over the other. This is non-negotiable.
2. Include all imports from both sides (remove exact duplicates only)
3. Never delete functionality from either side — all completed work must survive
4. Only modify conflicted files
5. For lock files (package-lock.json, yarn.lock, etc.): delete the conflicted lock, resolve the manifest, then run the package manager to regenerate
6. For package manifests: take the union of ALL dependency additions from both sides
7. After resolving each file, run "git add <file>"
8. Do NOT commit — the caller handles the commit
9. If you cannot safely resolve a file without losing work, leave it conflicted and report which files could not be resolved

Read each conflicted file, resolve the conflict markers by combining both sides, write the resolved version, and git add it.`, conflictedFiles, milestoneID, branch)

	// Reconciliation needs strong reasoning — use configured tier (defaults to high).
	flags := resolveModelFlags(cfg.Tool, reconciliationTier(cfg.ModelTiers), cfg.Root)
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, flags...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// runRecover handles the "belmont recover" command.
func runRecover(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	var format string
	var list bool
	var merge string
	var clean string
	var cleanAll bool
	var tool string
	var force bool
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.BoolVar(&list, "list", false, "list preserved worktrees")
	fs.StringVar(&merge, "merge", "", "retry merge for slug")
	fs.StringVar(&clean, "clean", "", "delete worktree and branch for slug")
	fs.BoolVar(&cleanAll, "clean-all", false, "clean all preserved worktrees")
	fs.StringVar(&tool, "tool", "", "CLI tool for reconciliation (claude|codex|gemini|copilot|cursor|pi|opencode) — auto-detected if omitted")
	fs.BoolVar(&force, "force", false, "act on worktrees an active run owns (use only when auto.json is stale)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	worktrees := listPreservedWorktrees(root)

	// `recover` exists for worktrees PRESERVED FROM A FAILED MERGE, but it finds
	// them by scanning the same directory `runAutoParallel` creates the live
	// wave's worktrees in, with no notion of an active run. So mid-run it
	// reported the running wave's worktrees as preserved and would have deleted
	// them.
	//
	// The cost is not "some commits". `.belmont/` is `--assume-unchanged` inside
	// a worktree, so every task state the wave has completed but not yet merged
	// is in no commit anywhere; removing the directory takes it with it.
	//
	// The split between actions is deliberate. The MUTATING ones refuse. `--list`
	// does not, because it is read-only and what invited the destructive command
	// in the first place was a listing that called a live worktree "preserved" —
	// refusing to inform would leave the reader with less than they started with.
	// See issue #52.
	live, liveLabel := activeRunWorktrees(root)
	if force {
		live = nil
	}

	if merge != "" {
		if err := refuseIfRunOwns(live, liveLabel, worktrees, merge, "merge"); err != nil {
			return err
		}
		return recoverMerge(root, merge, tool, worktrees)
	}
	if clean != "" {
		if err := refuseIfRunOwns(live, liveLabel, worktrees, clean, "clean"); err != nil {
			return err
		}
		return recoverClean(root, clean, worktrees)
	}
	if cleanAll {
		// All-or-nothing: cleaning the non-live ones and skipping the rest would
		// report partial success for a command whose whole contract is "all".
		for _, wt := range worktrees {
			if live[absPathOrSelf(wt.Path)] {
				return fmt.Errorf("recover: %s is still in flight and owns %s — refusing --clean-all.\n"+
					"Deleting a live worktree loses every task state the run has completed but not merged; `.belmont/` is assume-unchanged inside a worktree, so it is in no commit.\n"+
					"Wait for the run to finish, or pass --force if you are certain .belmont/auto.json is stale (a run killed with SIGKILL leaves it behind).",
					liveLabel, filepath.Base(wt.Path))
			}
		}
		return recoverCleanAll(root, worktrees, format)
	}

	// Default: list (explicit or implicit)
	return recoverList(root, worktrees, format, live, liveLabel)
}

// activeRunWorktrees returns the absolute worktree paths `.belmont/auto.json`
// claims for a run that is still active, plus a label naming what is running.
//
// Empty when no run is active — `readActiveAutoJSONOrNil` already returns nil for
// a missing file, unparseable JSON, or `active: false`, so an absent guard and a
// finished run are the same thing here.
func activeRunWorktrees(root string) (map[string]bool, string) {
	aj := readActiveAutoJSONOrNil(root)
	if aj == nil {
		return nil, ""
	}
	paths := map[string]bool{}
	for _, e := range aj.Worktrees {
		paths[absPathOrSelf(e.Path)] = true
	}
	if aj.Feature != "" {
		return paths, aj.Feature
	}
	// Multi-feature mode names no single feature; the worktree keys are the
	// feature slugs, so the label is built from them rather than left blank —
	// "an active run owns this" is much less useful than saying which.
	var slugs []string
	for slug := range aj.Worktrees {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	if len(slugs) == 0 {
		return paths, "an active run"
	}
	return paths, strings.Join(slugs, ", ")
}

func absPathOrSelf(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// refuseIfRunOwns blocks a mutating recover action aimed at a worktree the active
// run still owns. `slug` is matched the way the action itself matches it, on the
// worktree directory's base name.
func refuseIfRunOwns(live map[string]bool, liveLabel string, worktrees []worktreeEntry, slug, action string) error {
	if len(live) == 0 {
		return nil
	}
	for _, wt := range worktrees {
		if filepath.Base(wt.Path) != slug || !live[absPathOrSelf(wt.Path)] {
			continue
		}
		return fmt.Errorf("recover: %s is still in flight and owns %s — refusing --%s.\n"+
			"Its uncommitted `.belmont/` state is in no commit, so acting on it now can lose completed work.\n"+
			"Wait for the run to finish, or pass --force if you are certain .belmont/auto.json is stale.",
			liveLabel, slug, action)
	}
	return nil
}

func recoverList(root string, worktrees []worktreeEntry, format string, live map[string]bool, liveLabel string) error {
	if format == "json" {
		type wtJSON struct {
			Slug string `json:"slug"`
			Path string `json:"path"`
			// Named rather than omitted: an agent reading this needs the same
			// distinction the text view now draws, and `false` is a fact worth
			// stating for a command whose other actions delete things.
			ActiveRun bool   `json:"active_run"`
			Branch    string `json:"branch"`
		}
		var items []wtJSON
		for _, wt := range worktrees {
			slug := filepath.Base(wt.Path)
			items = append(items, wtJSON{
				Slug:      slug,
				Path:      wt.Path,
				ActiveRun: live[absPathOrSelf(wt.Path)],
				Branch:    wt.Branch,
			})
		}
		if items == nil {
			items = []wtJSON{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	if len(worktrees) == 0 {
		fmt.Println("No preserved worktrees found.")
		return nil
	}

	liveCount := 0
	for _, wt := range worktrees {
		if live[absPathOrSelf(wt.Path)] {
			liveCount++
		}
	}

	fmt.Printf("Preserved worktrees (%d):\n\n", len(worktrees))
	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		if live[absPathOrSelf(wt.Path)] {
			// Not "preserved from a failed merge" at all. Saying so here is the
			// point: this listing is what a confused user reads immediately
			// before reaching for --clean-all.
			fmt.Printf("  %s  [IN FLIGHT — owned by the active run: %s]\n", slug, liveLabel)
		} else {
			fmt.Printf("  %s\n", slug)
		}
		fmt.Printf("    Path:   %s\n", wt.Path)
		fmt.Printf("    Branch: %s\n", wt.Branch)
		fmt.Println()
	}
	if liveCount > 0 {
		fmt.Printf("%d of these belong to a run still in flight (%s) and are NOT preserved from a failed merge.\n",
			liveCount, liveLabel)
		fmt.Println("Cleaning or merging one now loses task state the run has completed but not merged.")
		fmt.Println("--merge / --clean / --clean-all will refuse to touch them until the run finishes.")
		fmt.Println()
	}
	fmt.Println("Actions:")
	fmt.Println("  belmont recover --merge <slug>    Retry merge with improved logic (uses --tool for reconciliation)")
	fmt.Println("  belmont recover --clean <slug>    Delete worktree and branch")
	fmt.Println("  belmont recover --clean-all       Clean all preserved worktrees")
	return nil
}

func recoverMerge(root, slug, tool string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	// Reconciliation needs an AI tool to analyze conflicts. Honour the explicit
	// --tool flag if given, otherwise auto-detect (mirrors the `auto` command).
	if tool == "" {
		tool = detectTool()
		if tool == "" {
			return fmt.Errorf("recover: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor, pi, opencode\nInstall one or use --tool to specify")
		}
	} else {
		switch tool {
		case "claude", "codex", "gemini", "copilot", "cursor", "pi", "opencode":
			// ok
		default:
			return fmt.Errorf("recover: unsupported tool %q (use claude, codex, gemini, copilot, cursor, pi, or opencode)", tool)
		}
	}

	commitMsg := fmt.Sprintf("belmont: merge recovered %s", slug)
	cfg := loopConfig{Root: root, Tool: tool}

	if err := attemptMerge(cfg, commitMsg, wt.Branch, slug); err != nil {
		return fmt.Errorf("merge failed for %s: %w", slug, err)
	}

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(root, ".belmont", "reconciliation-report.json"))

	// Clean up worktree and branch
	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-d", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Recovered and merged %s\033[0m\n", slug)
	return nil
}

func recoverClean(root, slug string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-D", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	return nil
}

func recoverCleanAll(root string, worktrees []worktreeEntry, format string) error {
	if len(worktrees) == 0 {
		if format != "json" {
			fmt.Println("No preserved worktrees to clean.")
		}
		return nil
	}

	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		removeWorktree(root, wt.Path, slug)

		delCmd := exec.Command("git", "branch", "-D", wt.Branch)
		delCmd.Dir = root
		delCmd.Run()

		fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	}
	return nil
}
