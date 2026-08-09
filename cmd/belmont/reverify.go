package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// resetVerifiedTasks rewrites verified task markers back to `[x]` within the
// milestones named in resetIDs, so the verification agent picks them up again.
// Returns the new content and whether anything changed.
//
// Extracted from runReverifyCmd so it can be tested directly. The marker is
// captured and classified via canonicalMarker rather than matched as a literal
// `\[v\]`: resetIDs is built from PARSED task states, so a rewriter that
// recognises fewer spellings than the parser silently no-ops — the milestone
// is selected for reset and then nothing is reset, and `belmont reverify`
// reports "nothing to re-verify" on a file full of verified tasks.
func resetVerifiedTasks(content string, resetIDs map[string]bool) (string, bool) {
	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*`)
	taskRe := regexp.MustCompile(`^(\s*-\s+)\[(.)\](\s+.*)$`)

	lines := strings.Split(content, "\n")
	currentMSID := ""
	changed := false
	for i, line := range lines {
		if hm := msHeaderRe.FindStringSubmatch(line); len(hm) >= 2 {
			currentMSID = "M" + hm[1]
		} else if isSectionBreak(line) {
			// This is a WRITER that attributes lines to a milestone, so it needs
			// the same region boundary every reader uses. Without it currentMSID
			// survived past a column-zero `## `, and `belmont reverify` rewrote
			// `[v]`-shaped lines under `## Session History` — historical log
			// entries silently mutated. See isSectionBreak and issue #31.
			currentMSID = ""
		}
		if !resetIDs[currentMSID] {
			continue
		}
		if vm := taskRe.FindStringSubmatch(line); len(vm) >= 4 && markerIsVerified(vm[2]) {
			lines[i] = vm[1] + "[x]" + vm[3]
			changed = true
		}
	}
	return strings.Join(lines, "\n"), changed
}

// runReverifyCmd handles the "belmont reverify" command.
// Walks through completed milestones and runs verification on each sequentially.
// Reports which milestones passed and which had follow-up tasks created.
func runReverifyCmd(args []string) error {
	fs := flag.NewFlagSet("reverify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, from, to, format, tool string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug")
	fs.StringVar(&from, "from", "", "start milestone (e.g. M3)")
	fs.StringVar(&to, "to", "", "end milestone (e.g. M10)")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
	fs.StringVar(&tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor|pi|opencode)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("reverify: %w", err)
	}
	root, _ = filepath.Abs(root)

	// Auto-detect tool if not specified
	if tool == "" {
		tool = detectTool()
		if tool == "" {
			return fmt.Errorf("reverify: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor, pi, opencode\nInstall one or use --tool to specify")
		}
	}

	// Resolve feature — auto-detect if only one exists
	featuresDir := filepath.Join(root, ".belmont", "features")
	feature, err := resolveSingleFeature(root, feature, "reverify")
	if err != nil {
		return err
	}

	progressPath := filepath.Join(featuresDir, feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("reverify: cannot read %s: %w", progressPath, err)
	}

	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, from, to)

	// Reset [v] (verified) tasks to [x] (done) in targeted milestones so the
	// verification agent will pick them up again. Only milestones in range that
	// have [v] or [x] tasks are candidates for re-verification.
	//
	// Build a set of milestone IDs that contain [v] tasks and need resetting.
	resetIDs := map[string]bool{}
	for _, m := range inRange {
		for _, t := range m.Tasks {
			if t.Status == taskVerified {
				resetIDs[m.ID] = true
				break
			}
		}
	}

	if len(resetIDs) > 0 {
		newContent, changed := resetVerifiedTasks(string(progressContent), resetIDs)
		if changed {
			if err := os.WriteFile(progressPath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("reverify: failed to reset verified tasks: %w", err)
			}
			// Re-parse after rewrite so downstream filtering sees [x] tasks.
			milestones = parseMilestones(newContent)
			inRange = milestonesInRange(milestones, from, to)
		}
	}

	// Find milestones that have [x] (done but not verified) tasks
	var targets []milestone
	for _, m := range inRange {
		hasDoneTasks := false
		for _, t := range m.Tasks {
			if t.Status == taskDone {
				hasDoneTasks = true
				break
			}
		}
		if hasDoneTasks {
			targets = append(targets, m)
		}
	}

	if len(targets) == 0 {
		if format == "json" {
			fmt.Println(`{"verified":0,"results":[]}`)
		} else {
			fmt.Fprintln(os.Stderr, "No milestones with unverified tasks to re-verify in the specified range.")
		}
		return nil
	}

	// Print header
	ids := make([]string, len(targets))
	for i, m := range targets {
		ids[i] = m.ID
	}
	msWord := "milestones"
	if len(targets) == 1 {
		msWord = "milestone"
	}
	fmt.Fprintf(os.Stderr, "\033[1mBelmont Reverify\033[0m — %s (%d %s)\n", feature, len(targets), msWord)
	fmt.Fprintf(os.Stderr, "Tool: %s | Milestones: %s\n\n", tool, strings.Join(ids, ", "))

	// Walk milestones sequentially, running verification on each
	type msResult struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Passed   bool     `json:"passed"`
		Fwlups   []string `json:"fwlups,omitempty"`
		Duration float64  `json:"duration_s"`
		Error    string   `json:"error,omitempty"`
	}
	results := make([]msResult, 0, len(targets))

	// Load per-feature model tiers once; reverify maps to the verification agent tier.
	tiers, _ := parseModelTiers(filepath.Join(featuresDir, feature, "models.yaml"))
	verifyModelFlags := resolveModelFlags(tool, tiers.Tiers["verification"], root)

	for i, m := range targets {
		fmt.Fprintf(os.Stderr, "━━ [%d/%d] VERIFY ━━ %s › %s: %s ━━\n", i+1, len(targets), feature, m.ID, m.Name)

		// Build milestone-scoped verify prompt
		prompt := fmt.Sprintf("/belmont:verify --feature %s", feature)
		prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED VERIFICATION: Only verify tasks marked [x] (done) in milestone %s. Do NOT verify tasks from other milestones. Focus on: (1) the tasks in %s meet their acceptance criteria, (2) build passes, (3) tests pass.\n\nOn success: mark verified tasks as [v] in PROGRESS.md.\nOn failure: add new [ ] follow-up tasks to milestone %s and leave originals as [x].\n\nCRITICAL: Do NOT modify tasks in any other milestone.", m.ID, m.ID, m.ID)
		prompt = adaptPromptForTool(prompt, tool)

		// Build and run the tool command
		args := toolHeadlessArgs(tool, prompt, root, verifyModelFlags, true)
		if args == nil {
			return fmt.Errorf("reverify: unsupported tool: %s", tool)
		}
		cmd := exec.Command(toolBinary(tool), args...)
		cmd.Dir = root

		prefix := fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, m.ID)
		var tw *tailWriter
		if tool == "claude" {
			tw = newTailWriter(os.Stderr, 1500, "")
			cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: prefix}
			cmd.Stderr = tw
		} else {
			tw = newTailWriter(os.Stderr, 1500, prefix)
			cmd.Stdout = tw
			cmd.Stderr = tw
		}

		start := time.Now()
		runErr := cmd.Run()
		duration := time.Since(start)

		res := msResult{
			ID:       m.ID,
			Name:     m.Name,
			Duration: duration.Seconds(),
		}

		if runErr != nil {
			res.Passed = false
			res.Error = runErr.Error()
			fmt.Fprintf(os.Stderr, "\n\033[31m  ✗ %s failed (%.1fs): %s\033[0m\n\n", m.ID, res.Duration, runErr)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[32m  ✓ %s (%.1fs)\033[0m\n", m.ID, res.Duration)

			// Re-read status to detect verification results
			report, statusErr := buildStatus(root, 55, feature)
			if statusErr == nil {
				// Check for new incomplete tasks (follow-ups) in this milestone
				var followups []string
				for _, t := range report.Tasks {
					if (t.Status == taskTodo || t.Status == taskInProgress) &&
						t.MilestoneID == m.ID {
						label := t.ID
						if label == "" {
							label = t.Name
						}
						followups = append(followups, label)
					}
				}
				// Also check if any [x] tasks remain (verification didn't pass them)
				hasUnverified := false
				for _, t := range report.Tasks {
					if t.Status == taskDone && t.MilestoneID == m.ID {
						hasUnverified = true
						break
					}
				}
				res.Fwlups = followups
				res.Passed = len(followups) == 0 && !hasUnverified
			} else {
				res.Passed = true // assume passed if we can't check
			}

			if len(res.Fwlups) > 0 {
				fmt.Fprintf(os.Stderr, "  \033[33m  %d follow-up(s): %s\033[0m\n\n", len(res.Fwlups), strings.Join(res.Fwlups, ", "))
			} else {
				fmt.Fprintln(os.Stderr)
			}
		}

		results = append(results, res)
	}

	// Print summary
	passed := 0
	var allFwlups []string
	for _, r := range results {
		if r.Passed {
			passed++
		}
		allFwlups = append(allFwlups, r.Fwlups...)
	}

	if format == "json" {
		// Build JSON manually to avoid importing encoding/json just for this
		fmt.Printf(`{"feature":%q,"verified":%d,"passed":%d,"total_fwlups":%d,"results":[`, feature, len(results), passed, len(allFwlups))
		for i, r := range results {
			if i > 0 {
				fmt.Print(",")
			}
			fwlupsJSON := "[]"
			if len(r.Fwlups) > 0 {
				fwlupsJSON = `["` + strings.Join(r.Fwlups, `","`) + `"]`
			}
			errJSON := "null"
			if r.Error != "" {
				errJSON = fmt.Sprintf("%q", r.Error)
			}
			fmt.Printf(`{"id":%q,"name":%q,"passed":%t,"fwlups":%s,"duration_s":%.1f,"error":%s}`,
				r.ID, r.Name, r.Passed, fwlupsJSON, r.Duration, errJSON)
		}
		fmt.Println("]}")
	} else {
		fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Fprintf(os.Stderr, "\033[1mReverify Summary\033[0m — %s\n\n", feature)
		for _, r := range results {
			if r.Error != "" {
				fmt.Fprintf(os.Stderr, "  \033[31m✗\033[0m %s: %s — error: %s\n", r.ID, r.Name, r.Error)
			} else if !r.Passed {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠\033[0m %s: %s — %d follow-up(s): %s\n", r.ID, r.Name, len(r.Fwlups), strings.Join(r.Fwlups, ", "))
			} else {
				fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m %s: %s\n", r.ID, r.Name)
			}
		}
		fmt.Fprintf(os.Stderr, "\n  %d/%d passed", passed, len(results))
		if len(allFwlups) > 0 {
			fmt.Fprintf(os.Stderr, ", %d follow-up task(s) created", len(allFwlups))
		}
		fmt.Fprintln(os.Stderr)

		if len(allFwlups) > 0 {
			fmt.Fprintf(os.Stderr, "\nTo fix follow-ups: belmont auto --feature %s\n", feature)
		}
	}

	return nil
}
