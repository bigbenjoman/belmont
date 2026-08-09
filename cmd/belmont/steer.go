package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// parseSteeringEntries walks STEERING.md and returns every block it finds.
// Unrecognised preamble or garbage between entries is ignored.
func parseSteeringEntries(data string) []steeringEntry {
	lines := strings.Split(data, "\n")
	var entries []steeringEntry
	var current *steeringEntry
	var body []string
	flush := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimRight(strings.Join(body, "\n"), "\n")
		entries = append(entries, *current)
		current = nil
		body = nil
	}
	for _, line := range lines {
		if m := steeringHeaderRe.FindStringSubmatch(line); m != nil {
			flush()
			current = &steeringEntry{
				Timestamp: m[1],
				Milestone: m[2],
				State:     m[3],
			}
			continue
		}
		if current != nil {
			body = append(body, line)
		}
	}
	flush()
	return entries
}

// renderSteeringEntries serialises entries back to STEERING.md format.
// Preserves order.
func renderSteeringEntries(entries []steeringEntry) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## ")
		b.WriteString(e.Timestamp)
		if e.Milestone != "" {
			b.WriteString(" [")
			b.WriteString(e.Milestone)
			b.WriteString("]")
		}
		b.WriteString(" (")
		b.WriteString(e.State)
		b.WriteString(")\n")
		if trimmed := strings.TrimSpace(e.Body); trimmed != "" {
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// steeringHeader returns the prompt-side framing for injected instructions.
// The wording is deliberately forceful — the point of steering is to override
// the normal prompt flow when the user has new information.
func steeringHeader() string {
	return `## URGENT — User steering (higher priority than NOTES.md)

The user has injected the following instruction(s) into this feature loop. They override or amend the surrounding task. Read them carefully, apply them to your current action, and acknowledge them at the start of your reply.

`
}

// consumePendingSteering reads STEERING.md for the given feature, takes any
// pending entry that matches the current milestone (or has no milestone tag),
// and returns the formatted user-steering block (prefixed with steeringHeader)
// plus a count. Returns ("", 0) when there is nothing pending.
//
// Invariant: STEERING.md contains only (pending) entries. Consumed entries
// are dropped from disk entirely (the audit lives in the auto run's stderr
// stream — the `[STEERING] injected …` line and its timestamp). STEERING.md
// is deleted when no pending entries remain so agents exploring the feature
// dir don't see a stale file and burn input tokens re-reading text that's
// already in the prompt.
//
// Legacy (consumed) entries written by older versions of this code are
// silently dropped on first encounter, same migration path.
//
// All filesystem errors are non-fatal.
func consumePendingSteering(root, feature, milestoneID, phase string) (string, int) {
	if root == "" || feature == "" {
		return "", 0
	}
	path := filepath.Join(root, ".belmont", "features", feature, "STEERING.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0
	}
	entries := parseSteeringEntries(string(data))
	now := time.Now().UTC().Format(time.RFC3339)

	var remainingPending []steeringEntry
	var newlyConsumed []steeringEntry
	for _, e := range entries {
		if e.State != "pending" {
			// Legacy consumed entries from older code — drop silently.
			continue
		}
		if e.Milestone == "" || e.Milestone == milestoneID {
			e.State = fmt.Sprintf("consumed %s by %s", now, phase)
			newlyConsumed = append(newlyConsumed, e)
		} else {
			remainingPending = append(remainingPending, e)
		}
	}

	// Rewrite STEERING.md with only pending entries; delete when empty so
	// the live file acts purely as the agent-facing inbox.
	if len(remainingPending) == 0 {
		_ = os.Remove(path)
	} else {
		_ = os.WriteFile(path, []byte(renderSteeringEntries(remainingPending)), 0644)
	}

	if len(newlyConsumed) == 0 {
		return "", 0
	}

	var b strings.Builder
	b.WriteString(steeringHeader())
	for i, e := range newlyConsumed {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		if e.Milestone != "" {
			fmt.Fprintf(&b, "Scope: milestone %s\n\n", e.Milestone)
		}
		b.WriteString(strings.TrimSpace(e.Body))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String(), len(newlyConsumed)
}

// logSteeringInjection prints a one-line notice to stderr matching the
// `[feature][milestone]: ...` prefix used by the auto loop. The preview
// is the first ~100 chars of the first consumed entry's body, truncated
// with `…` so the stream stays one line per injection.
func logSteeringInjection(feature, milestoneID string, count int, block string) {
	preview := steeringPreview(block)
	noun := "instruction"
	if count != 1 {
		noun = "instructions"
	}
	var prefix string
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	fmt.Fprintf(os.Stderr, "%s\033[35m[STEERING]\033[0m injected %d %s — \"%s\"\n", prefix, count, noun, preview)
}

// steeringPreview extracts the first non-header line of the injected block
// and truncates to ~100 chars.
func steeringPreview(block string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if strings.HasPrefix(trimmed, "The user has injected") {
			continue
		}
		if strings.HasPrefix(trimmed, "Scope:") {
			continue
		}
		if len(trimmed) > 100 {
			return trimmed[:99] + "…"
		}
		return trimmed
	}
	return ""
}

// runSteerCmd implements `belmont steer`.
func runSteerCmd(args []string) error {
	fs := flag.NewFlagSet("steer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, milestone, message, file string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug (auto-detected if only one active)")
	fs.StringVar(&milestone, "milestone", "", "narrow steering to a single milestone (e.g. M5)")
	fs.StringVar(&message, "message", "", "steering text (inline)")
	fs.StringVar(&file, "file", "", "read steering text from file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("steer: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("steer: resolve root: %w", err)
	}

	// Resolve the active auto run and its feature.
	aj, err := readActiveAutoJSON(absRoot)
	if err != nil {
		return err
	}
	resolvedFeature, err := resolveSteerFeature(aj, feature)
	if err != nil {
		return err
	}

	// Read the steering text from exactly one source.
	text, err := readSteeringInput(fs.Args(), message, file, resolvedFeature, milestone)
	if err != nil {
		return err
	}

	// Figure out targets: every active worktree for the feature, optionally
	// narrowed to one milestone. In serial mode the only target is the master
	// feature directory.
	targets, err := resolveSteeringTargets(absRoot, aj, resolvedFeature, milestone)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("steer: no active worktree matched feature=%q milestone=%q", resolvedFeature, milestone)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		entryMilestone := milestone
		if entryMilestone == "" && t.MilestoneID != "" && len(targets) > 1 {
			// Broadcast into a parallel run: tag each entry with the target
			// milestone so it only fires when that milestone runs.
			entryMilestone = t.MilestoneID
		}
		path := filepath.Join(t.Root, ".belmont", "features", resolvedFeature, "STEERING.md")
		if err := appendSteeringEntry(path, timestamp, entryMilestone, text); err != nil {
			return fmt.Errorf("steer: write %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m injected → %s", path)
		if entryMilestone != "" {
			fmt.Fprintf(os.Stderr, " \033[2m[%s]\033[0m", entryMilestone)
		}
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// resolveSteerFeature picks the feature slug from --feature or auto.json.
func resolveSteerFeature(aj autoJSON, requested string) (string, error) {
	if requested != "" {
		if aj.Feature != "" && aj.Feature != requested {
			return "", fmt.Errorf("steer: feature %q is not the active auto run (active: %q)", requested, aj.Feature)
		}
		return requested, nil
	}
	if aj.Feature != "" {
		return aj.Feature, nil
	}
	// Multi-feature runs don't set aj.Feature; require explicit selection.
	return "", fmt.Errorf("steer: --feature required (auto.json does not record a single active feature)")
}

// resolveSteeringTargets returns the writable targets for a steer request.
// When auto.json has per-milestone worktree entries, each entry becomes a
// target; otherwise the single target is the master feature directory.
func resolveSteeringTargets(root string, aj autoJSON, feature, milestone string) ([]steeringTarget, error) {
	// Parallel: one target per active worktree entry for the feature.
	if len(aj.Worktrees) > 0 {
		var targets []steeringTarget
		for id, entry := range aj.Worktrees {
			if milestone != "" && id != milestone {
				continue
			}
			// Verify the worktree still exists — stale entries shouldn't
			// silently accept writes.
			if !dirExists(entry.Path) {
				continue
			}
			targets = append(targets, steeringTarget{
				MilestoneID: id,
				Root:        entry.Path,
				Label:       id,
			})
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].MilestoneID < targets[j].MilestoneID })
		return targets, nil
	}
	// Serial: single target is the master feature directory under root.
	featureDir := filepath.Join(root, ".belmont", "features", feature)
	if !dirExists(featureDir) {
		return nil, fmt.Errorf("steer: feature directory not found: %s", featureDir)
	}
	return []steeringTarget{{Root: root, Label: "serial"}}, nil
}

// readSteeringInput reads steering text from exactly one source (errors if
// zero or two+ sources are provided). Sources are, in order: --message,
// --file, "-" positional for stdin, or $EDITOR fallback when stdin is a
// TTY and $EDITOR is set.
func readSteeringInput(positional []string, message, file, feature, milestone string) (string, error) {
	wantStdin := false
	for _, a := range positional {
		if a == "-" {
			wantStdin = true
		}
	}
	sourceCount := 0
	if message != "" {
		sourceCount++
	}
	if file != "" {
		sourceCount++
	}
	if wantStdin {
		sourceCount++
	}
	if sourceCount > 1 {
		return "", fmt.Errorf("steer: provide exactly one of --message, --file, or `-` (stdin)")
	}

	var text string
	switch {
	case message != "":
		text = message
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("steer: read --file: %w", err)
		}
		text = string(data)
	case wantStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("steer: read stdin: %w", err)
		}
		text = string(data)
	default:
		// $EDITOR fallback — only when attached to a TTY and EDITOR is set.
		if !isTerminal(os.Stdin) {
			return "", fmt.Errorf("steer: no input provided — pass --message \"text\", --file PATH, or `-` (stdin)")
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			return "", fmt.Errorf("steer: no input provided and $EDITOR is unset — pass --message \"text\", --file PATH, or `-` (stdin)")
		}
		edited, err := runSteerEditor(editor, feature, milestone)
		if err != nil {
			return "", err
		}
		text = edited
	}

	// Strip lines beginning with `#` when the text came through $EDITOR;
	// harmless for other sources (users rarely write literal `#` lines at
	// the column 0 position).
	text = stripSteerComments(text)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("steer: empty steering text — aborting")
	}
	return trimmed, nil
}

// stripSteerComments drops lines beginning with `#` and any trailing blank
// lines left behind. Used for $EDITOR input so the seeded template comments
// don't end up in STEERING.md.
func stripSteerComments(text string) string {
	var keep []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// runSteerEditor opens $EDITOR on a seeded temp file and returns the saved
// contents. An empty edit (after comment stripping) returns an error.
func runSteerEditor(editor, feature, milestone string) (string, error) {
	tmp, err := os.CreateTemp("", "belmont-steer-*.md")
	if err != nil {
		return "", fmt.Errorf("steer: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	seed := "# Belmont steer — lines starting with `#` are ignored.\n"
	seed += fmt.Sprintf("# Feature: %s\n", feature)
	if milestone != "" {
		seed += fmt.Sprintf("# Milestone: %s\n", milestone)
	}
	seed += "# Write the instructions for the agent below this comment and save.\n\n"
	if _, err := tmp.WriteString(seed); err != nil {
		tmp.Close()
		return "", fmt.Errorf("steer: seed temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("sh", "-c", fmt.Sprintf("%s %q", editor, tmpPath))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("steer: $EDITOR exited non-zero: %w", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("steer: re-read temp file: %w", err)
	}
	return string(data), nil
}

// appendSteeringEntry appends a pending entry to STEERING.md, creating the
// file (and parent dir) if needed.
func appendSteeringEntry(path, timestamp, milestone, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var header strings.Builder
	header.WriteString("## ")
	header.WriteString(timestamp)
	if milestone != "" {
		header.WriteString(" [")
		header.WriteString(milestone)
		header.WriteString("]")
	}
	header.WriteString(" (pending)\n")
	header.WriteString(strings.TrimSpace(body))
	header.WriteString("\n\n")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Ensure there's a blank line before the new entry when the file was
	// non-empty and didn't end with one.
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		last := make([]byte, 2)
		// Best-effort peek at the last two bytes; ignore errors.
		_, _ = f.Seek(-2, io.SeekEnd)
		_, _ = f.Read(last)
		_, _ = f.Seek(0, io.SeekEnd)
		if !(last[0] == '\n' && last[1] == '\n') {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	_, err = f.WriteString(header.String())
	return err
}

// injectScopeGuardSteering writes a STEERING.md entry so the next phase's
// agent prompt starts with an explicit correction. Reuses the steering
// infrastructure; entries are tagged with the target milestone so they
// only fire for the offending worktree.
func injectScopeGuardSteering(cfg loopConfig, action loopAction, violations []scopeViolation) {
	if cfg.Root == "" || cfg.Feature == "" {
		return
	}
	path := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature, "STEERING.md")
	var body strings.Builder
	body.WriteString("belmont's scope guard reverted edits your previous phase made to PROGRESS.md because they violated the milestone-immutability rule. The milestone structure is preserved by the Go CLI after each phase — you cannot bypass this by committing with `--no-verify`, and future phases will apply the same check. To avoid re-triggering the guard:\n\n")
	for _, v := range violations {
		switch v.Kind {
		case "new_milestone":
			// No file reference here: `skills/belmont/_partials/` is a
			// build-time source path that `belmont install` never writes, so in
			// a consuming project it is a dangling pointer — and this text goes
			// straight into the next phase's prompt as a correction the agent is
			// told to follow. State the rule instead of citing a file.
			body.WriteString(fmt.Sprintf("- Do NOT create new milestones. The milestone %s %q was removed. Follow-ups belong inside the source milestone as new `[ ]` tasks. Only `/belmont:tech-plan` may add, rename or remove a milestone.\n", v.Milestone, v.MilestoneName))
		case "out_of_scope_flip":
			body.WriteString(fmt.Sprintf("- Do NOT modify tasks outside your target milestone. Your edit to %s in %s (%s → %s) was reverted. Only touch tasks inside %s.\n", v.TaskID, v.Milestone, v.FromState, v.ToState, action.MilestoneID))
		}
	}
	body.WriteString("\nIf you genuinely believe a cross-milestone edit is needed, STOP and report it as a blocker in your summary instead of editing PROGRESS.md.")
	_ = appendSteeringEntry(path, time.Now().UTC().Format(time.RFC3339), action.MilestoneID, body.String())
}

// injectEvidenceSteering tells the next phase explicitly which tasks lost
// their [v] and why, so it doesn't try to re-flip them without doing work.
func injectEvidenceSteering(cfg loopConfig, action loopAction, missing []evidenceMissing) {
	if cfg.Root == "" || cfg.Feature == "" {
		return
	}
	path := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature, "STEERING.md")
	var body strings.Builder
	body.WriteString("belmont's verify-evidence guard reverted [v] flips on the following task(s) because no commit in this worktree's history mentions their task IDs. A task cannot be marked verified without a commit that implements it and names the task ID in the commit message (the existing convention: `[P1-1]:` or `P1-1:`). The guard runs in the Go CLI after each phase and cannot be bypassed.\n\nReverted flips:\n")
	for _, m := range missing {
		body.WriteString(fmt.Sprintf("- %s (milestone %s) — had no commit referencing it; reverted [v] → [%s].\n", m.TaskID, m.Milestone, nonEmpty(m.FromState, " ")))
	}
	body.WriteString("\nTo verify these tasks: either (a) show the existing commit by its hash if the task was genuinely implemented (perhaps under a different task ID — then update PROGRESS.md's task ID to match), or (b) implement the task now, commit with the task ID in the message, then re-verify.")
	_ = appendSteeringEntry(path, time.Now().UTC().Format(time.RFC3339), action.MilestoneID, body.String())
}
