package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// `belmont blockers` is the decision queue.
//
// A `[!]` task is the one marker no agent can clear. It means the work needs a
// human: an approval, a ruling, a credential, an operator action, access nobody
// automated has. Belmont already refused to overwrite it — `mergeProgressState`
// never ranks over `[!]` in either direction — but refusing to lose a signal is
// not the same as surfacing it.
//
// Until this command, the only place those tasks appeared was the tail of
// `belmont status`, one truncated line each, interleaved with progress counts.
// A long-running loop can bank up dozens across a dozen milestones, and the
// person who has to answer them reads them one line at a time in the middle of
// a report about something else. The observed case: 19 blocked follow-ups on a
// single feature, every one of them a question for the same person, none of
// them visible together.
//
// So this prints them together, grouped by feature and milestone, with the
// indented body each task carries — because the body is where the actual
// question lives, and truncating it to 55 characters is what made the queue
// unreadable in the first place.
//
// It reports; it never writes. Answering a blocker means editing PROGRESS.md
// (or letting `/belmont:next` do it once the human input exists), and a command
// that both surfaces a question and resolves it would be free to guess.

// blockerSummaryNameCap bounds the checkbox-line text in --summary mode only.
// Full mode never truncates: the whole point of the command is that the
// question survives intact.
const blockerSummaryNameCap = 110

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

type blockerEntry struct {
	Feature       string   `json:"feature"`
	FeatureName   string   `json:"feature_name,omitempty"`
	Milestone     string   `json:"milestone"`
	MilestoneName string   `json:"milestone_name,omitempty"`
	TaskID        string   `json:"task_id,omitempty"`
	Task          string   `json:"task"`
	Line          int      `json:"line"`
	Detail        []string `json:"detail,omitempty"`
}

type blockersReport struct {
	Count    int            `json:"count"`
	Features int            `json:"features"`
	Blockers []blockerEntry `json:"blockers"`
}

func runBlockersCmd(args []string) error {
	fsFlags := flag.NewFlagSet("blockers", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root, format, feature, colorMode string
	var summary bool
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.StringVar(&feature, "feature", "", "feature slug (default: every feature)")
	fsFlags.StringVar(&colorMode, "color", "auto", "auto, always, or never")
	fsFlags.BoolVar(&summary, "summary", false, "one line per blocker; omit the task body")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("blockers: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	report, err := buildBlockers(absRoot, feature)
	if err != nil {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text":
		useColor, err := shouldColor(colorMode, os.Stdout)
		if err != nil {
			return fmt.Errorf("blockers: %w", err)
		}
		fmt.Print(renderBlockers(report, useColor, summary))
		return nil
	default:
		return fmt.Errorf("blockers: unknown format %q", format)
	}
}

func buildBlockers(root, feature string) (blockersReport, error) {
	featuresDir := filepath.Join(root, ".belmont", "features")

	var slugs []string
	if feature != "" {
		if !dirExists(filepath.Join(featuresDir, feature)) {
			return blockersReport{}, fmt.Errorf("blockers: feature %q not found in %s", feature, featuresDir)
		}
		slugs = []string{feature}
	} else {
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			// No features directory is not an error — it is an empty queue.
			return blockersReport{Blockers: []blockerEntry{}}, nil
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				slugs = append(slugs, e.Name())
			}
		}
		sort.Strings(slugs)
	}

	// Read live worktree state where a run is active, exactly as `status` does:
	// a blocker raised inside a worktree is not in the master copy yet, and it
	// is the one a person needs to see soonest.
	overrides := loadAutoWorktrees(root)

	report := blockersReport{Blockers: []blockerEntry{}}
	seen := map[string]bool{}
	for _, slug := range slugs {
		featurePath := filepath.Join(featuresDir, slug)
		if override, ok := overrides[slug]; ok && dirExists(override) {
			featurePath = override
		}
		content, err := os.ReadFile(filepath.Join(featurePath, "PROGRESS.md"))
		if err != nil {
			continue
		}
		name := slug
		if prd, err := os.ReadFile(filepath.Join(featurePath, "PRD.md")); err == nil {
			if extracted := extractFeatureName(string(prd)); extracted != "Unknown" {
				name = extracted
			}
		}

		lines := strings.Split(string(content), "\n")
		for _, m := range parseMilestones(string(content)) {
			for _, t := range m.Tasks {
				if t.Status != taskBlocked {
					continue
				}
				report.Blockers = append(report.Blockers, blockerEntry{
					Feature:       slug,
					FeatureName:   name,
					Milestone:     m.ID,
					MilestoneName: m.Name,
					TaskID:        t.ID,
					Task:          t.Name,
					Line:          t.Line,
					Detail:        taskDetail(lines, t.Line),
				})
				seen[slug] = true
			}
		}
	}
	report.Count = len(report.Blockers)
	report.Features = len(seen)
	return report, nil
}

// taskDetail returns the indented continuation lines belonging to the task on
// 1-based line `line`, with their common indent stripped. Blank interior lines
// are dropped rather than preserved: this is a queue to read, not a document to
// round-trip, and `taskBodyEnd` already stops at the first blank line.
func taskDetail(lines []string, line int) []string {
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return nil
	}
	end := taskBodyEnd(lines, idx)
	if end == idx {
		return nil
	}
	var out []string
	for _, l := range lines[idx+1 : end+1] {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func renderBlockers(report blockersReport, color, summary bool) string {
	var sb strings.Builder
	bold := func(s string) string {
		if color {
			return "\033[1m" + s + "\033[0m"
		}
		return s
	}
	yellow := func(s string) string {
		if color {
			return "\033[33m" + s + "\033[0m"
		}
		return s
	}
	dim := func(s string) string {
		if color {
			return "\033[2m" + s + "\033[0m"
		}
		return s
	}

	sb.WriteString(bold("Belmont Blockers") + "\n")
	sb.WriteString("================\n\n")

	if report.Count == 0 {
		sb.WriteString("No blocked tasks. Nothing is waiting on you.\n")
		return sb.String()
	}

	plural := "s"
	if report.Count == 1 {
		plural = ""
	}
	featurePlural := "s"
	if report.Features == 1 {
		featurePlural = ""
	}
	sb.WriteString(fmt.Sprintf("%d blocked task%s across %d feature%s — each one needs a person, not an agent.\n\n",
		report.Count, plural, report.Features, featurePlural))

	lastFeature, lastMilestone := "", ""
	for _, b := range report.Blockers {
		if b.Feature != lastFeature {
			if lastFeature != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(bold(b.FeatureName) + dim(" ("+b.Feature+")") + "\n")
			lastFeature, lastMilestone = b.Feature, ""
		}
		key := b.Milestone
		if key != lastMilestone {
			sb.WriteString("  " + b.Milestone + ": " + b.MilestoneName + "\n")
			lastMilestone = key
		}
		label := b.Task
		if summary {
			// Some tasks carry their whole explanation on the checkbox line
			// rather than in an indented body. In full mode that is what you
			// came to read; in summary mode it is a 900-character line that
			// wraps over half a screen, so the queue stops being scannable.
			label = truncateRunes(label, blockerSummaryNameCap)
		}
		if b.TaskID != "" {
			label = b.TaskID + ": " + label
		}
		sb.WriteString("    " + yellow("[!]") + " " + label + "\n")
		if !summary {
			for _, d := range b.Detail {
				sb.WriteString("        " + d + "\n")
			}
		}
		sb.WriteString(dim(fmt.Sprintf("        PROGRESS.md:%d", b.Line)) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dim("A blocked task never clears itself. Answer it, then edit the marker in\n"))
	sb.WriteString(dim("PROGRESS.md (or run /belmont:next once the input exists) to release it.\n"))
	return sb.String()
}
