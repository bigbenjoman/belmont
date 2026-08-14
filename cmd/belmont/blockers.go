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
// A `[!]` task is the one marker no *skill* can clear. It usually means the
// work needs a human: an approval, a ruling, a credential, an operator action,
// access nobody automated has. (Two writers mint a `[!]` an agent can legally
// reopen — the milestone-immutability partial's later-milestone dependency, and
// the reconciliation agent when the other side of a merge is `[x]`/`[v]`. Both
// name their reason, which is how you tell them apart.)
//
// Until this command, the only place those tasks appeared was `belmont status`,
// which lists each blocker's checkbox line but never the indented body under it
// — and the body is where the question actually lives. Nor did anything group
// the queue across features. A long-running loop banks up dozens across a dozen
// milestones, and the person who has to answer them read them one at a time in
// the middle of a report about something else. The observed case: 19 blocked
// follow-ups on a single feature, every one a question for the same person,
// none of them visible together.
//
// So this prints them together, grouped by feature and milestone, with the body
// intact.
//
// It reports; it never writes. Answering a blocker means editing PROGRESS.md,
// and a command that both surfaces a question and resolves it would be free to
// guess.

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
	Feature       string `json:"feature"`
	FeatureName   string `json:"feature_name,omitempty"`
	Milestone     string `json:"milestone,omitempty"`
	MilestoneName string `json:"milestone_name,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	Task          string `json:"task"`
	// Path is where the line actually lives, relative to the project root. It
	// is not always `.belmont/features/<slug>/PROGRESS.md`: during an active
	// run the live copy is inside a worktree, and its line numbers differ from
	// master's. Printing a bare "PROGRESS.md:12" sent the reader to a line that
	// does not exist in the file they can see.
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	LiveFrom string   `json:"live_from,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Marker   string   `json:"marker,omitempty"`
	Detail   []string `json:"detail,omitempty"`
}

type blockersReport struct {
	Count    int            `json:"count"`
	Features int            `json:"features"`
	Blockers []blockerEntry `json:"blockers"`
	// Unplaced holds `[!]` lines sitting outside every milestone. They are
	// invisible to `parseMilestones` and therefore to every count in Belmont;
	// reporting them separately is the same contract `orphanedTaskLines` has
	// with `status`, `validate` and `repair` — anything unplaceable is
	// surfaced, never dropped. Without this, a queue of one orphaned blocker
	// made this command print an affirmative "nothing is waiting on you".
	Unplaced []blockerEntry `json:"unplaced,omitempty"`
	// Unknown holds task lines whose marker Belmont cannot read. A `[!]` that
	// got corrupted to `[?]` is still a question waiting on a person, but it
	// parses as `taskUnknown` and matches no filter here. Everywhere else in
	// Belmont an unreadable marker is loud — `status` counts it, `validate`
	// exits 1, `repair` flags it — and this reader must not be the one place
	// it goes quiet, for the same reason Unplaced exists.
	Unknown []blockerEntry `json:"unknown,omitempty"`
	// Skipped names features whose PROGRESS.md could not be read at all, so
	// their absence from the queue is stated rather than silent.
	Skipped []string `json:"skipped,omitempty"`
	// LiveGaps names milestones whose live worktree state could not be read
	// during a parallel run, so their blockers — if any — were read from
	// master's fork-point copy. Same contract as Skipped, one level down: a
	// question raised inside that worktree is not in this queue and nothing else
	// would say so. See issue #48.
	LiveGaps []blockersLiveGap `json:"live_gaps,omitempty"`
}

// blockersLiveGap is a liveOverlayGap plus the feature it belongs to — this
// report spans every feature, so the milestone ID alone does not locate it.
type blockersLiveGap struct {
	Feature string `json:"feature"`
	liveOverlayGap
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

// progressSource is one PROGRESS.md and where it came from, so a blocker can
// carry the line number of the file it is actually in.
type progressSource struct {
	lines    []string
	path     string // display path, relative to root where possible
	liveFrom string // worktree feature dir, when this came from an active run
}

func newProgressSource(root, featureDir, liveFrom string) (progressSource, string, error) {
	full := filepath.Join(featureDir, "PROGRESS.md")
	content, err := os.ReadFile(full)
	if err != nil {
		return progressSource{}, "", err
	}
	display := full
	if rel, relErr := filepath.Rel(root, full); relErr == nil && !strings.HasPrefix(rel, "..") {
		display = rel
	}
	return progressSource{
		lines:    strings.Split(string(content), "\n"),
		path:     display,
		liveFrom: liveFrom,
	}, string(content), nil
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

	// Read live state from an active run, the same two ways `buildStatus` does.
	//
	// `loadAutoWorktrees` alone is wrong for single-feature-parallel mode: it
	// collapses a per-MILESTONE worktree map to one representative directory,
	// so a `[!]` raised in any other milestone's worktree was invisible here
	// while `belmont status --feature` showed it and then pointed the reader at
	// this command.
	liveFeature, perMilestoneLive := loadAutoWorktreeStateByMilestone(root)
	overrides := loadAutoWorktrees(root)

	report := blockersReport{Blockers: []blockerEntry{}}
	seen := map[string]bool{}
	for _, slug := range slugs {
		masterDir := filepath.Join(featuresDir, slug)
		baseDir := masterDir
		var baseLive string
		if perMilestoneLive == nil {
			if override, ok := overrides[slug]; ok && dirExists(override) {
				baseDir, baseLive = override, override
			}
		}

		src, content, err := newProgressSource(root, baseDir, baseLive)
		if err != nil && baseDir != masterDir {
			// A half-cleaned worktree must not take master's blockers down
			// with it. Fall back rather than drop the whole feature — and move
			// baseDir with it, or the PRD.md read below still points at the
			// dead worktree and the feature renders as its bare slug.
			src, content, err = newProgressSource(root, masterDir, "")
			baseDir, baseLive = masterDir, ""
		}
		if err != nil {
			report.Skipped = append(report.Skipped, slug)
			continue
		}

		// An archived feature (PRD.md removed by /belmont:cleanup, ARCHIVE.md
		// left behind) keeps whatever `[!]` it held. Those are not live
		// questions, but dropping them silently is the same failure as
		// dropping an orphan, so they are labelled rather than hidden.
		archived := fileExists(filepath.Join(baseDir, "ARCHIVE.md")) &&
			!fileExists(filepath.Join(baseDir, "PRD.md"))
		name := slug
		if prd, prdErr := os.ReadFile(filepath.Join(baseDir, "PRD.md")); prdErr == nil {
			if extracted := extractFeatureName(string(prd)); extracted != "Unknown" {
				name = extracted
			}
		} else if arch, archErr := os.ReadFile(filepath.Join(baseDir, "ARCHIVE.md")); archErr == nil {
			if extracted := extractArchiveName(string(arch)); extracted != "" {
				name = extracted
			}
		}

		// Per-milestone live overlay: each milestone's tasks, line numbers and
		// bodies come from the worktree that owns it.
		liveSrc := map[string]progressSource{}
		milestones := parseMilestones(content)
		if perMilestoneLive != nil && liveFeature == slug {
			for msID, wtFeatureDir := range perMilestoneLive {
				s, c, wErr := newProgressSource(root, wtFeatureDir, wtFeatureDir)
				if wErr != nil {
					continue // worktree lost its PROGRESS.md — master's copy stands
				}
				for _, wm := range parseMilestones(c) {
					if wm.ID == msID {
						liveSrc[msID] = s
						break
					}
				}
			}
			var gaps []liveOverlayGap
			milestones, gaps = overlayLiveMilestones(milestones, perMilestoneLive)
			// A milestone answered from master's fork-point copy is a queue this
			// command did not fully see. `Skipped` already states the same thing
			// one level up — a feature whose PROGRESS.md could not be read at all
			// — and this is that scenario arriving at a milestone. See issue #48.
			for _, g := range gaps {
				report.LiveGaps = append(report.LiveGaps, blockersLiveGap{Feature: slug, liveOverlayGap: g})
			}
		}

		for _, m := range milestones {
			use := src
			if s, ok := liveSrc[m.ID]; ok {
				use = s
			}
			for _, t := range m.Tasks {
				if t.Status != taskBlocked && t.Status != taskUnknown {
					continue
				}
				e := blockerEntry{
					Feature:       slug,
					FeatureName:   name,
					Milestone:     m.ID,
					MilestoneName: m.Name,
					TaskID:        t.ID,
					Task:          t.Name,
					Path:          use.path,
					Line:          t.Line,
					LiveFrom:      use.liveFrom,
					Archived:      archived,
					Detail:        taskDetail(use.lines, t.Line),
				}
				if t.Status == taskUnknown {
					e.Marker = t.Marker
					report.Unknown = append(report.Unknown, e)
				} else {
					report.Blockers = append(report.Blockers, e)
				}
				seen[slug] = true
			}
		}

		// Orphans come from the base file only: a task line outside every
		// milestone belongs to no worktree's milestone block.
		for _, t := range orphanedTaskLines(content) {
			status, _ := canonicalMarker(t.Marker)
			if status != taskBlocked && status != taskUnknown {
				continue
			}
			e := blockerEntry{
				Feature:     slug,
				FeatureName: name,
				TaskID:      t.ID,
				Task:        t.Name,
				Path:        src.path,
				Line:        t.Line,
				LiveFrom:    src.liveFrom,
				Archived:    archived,
				Detail:      taskDetail(src.lines, t.Line),
			}
			if status == taskUnknown {
				e.Marker = t.Marker
				report.Unknown = append(report.Unknown, e)
			} else {
				report.Unplaced = append(report.Unplaced, e)
			}
			seen[slug] = true
		}
	}
	report.Count = len(report.Blockers) + len(report.Unplaced) + len(report.Unknown)
	report.Features = len(seen)
	return report, nil
}

// taskDetail returns the continuation lines belonging to the task on 1-based
// line `line`, with their indent stripped.
//
// The extent is `taskBodyEnd` and nothing else. That helper bounds a body by the
// task's OWN indent, so a sibling checkbox at the same indent already ends the
// block — an earlier version of this function carried its own sibling-checkbox
// guard on top, which is now both redundant and wrong: a bullet nested INSIDE a
// task's body travels with the block enclosing it (see `moveTaskLines`), and the
// guard would have truncated the question at it.
//
// Note a blank line does not end a body: a loose-list task keeps its
// `**Evidence**` behind one. That is what this command wants — the whole
// question, not the first paragraph of it. Blank lines are dropped from the
// output because this is a queue to read, not a document to round-trip.
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
		writeBlockerSkipped(&sb, report, dim)
		writeBlockerLiveGaps(&sb, report, dim)
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
	sb.WriteString(fmt.Sprintf("%d blocked task%s across %d feature%s.\n\n",
		report.Count, plural, report.Features, featurePlural))

	writeBlocker := func(b blockerEntry, indent string) {
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
		loc := fmt.Sprintf("%s:%d", b.Path, b.Line)
		if summary {
			// One line means one line. The location rides along rather than
			// doubling the queue's length.
			sb.WriteString(indent + yellow("[!]") + " " + label + " " + dim("("+loc+")") + "\n")
			return
		}
		sb.WriteString(indent + yellow("[!]") + " " + label + "\n")
		for _, d := range b.Detail {
			sb.WriteString(indent + "    " + d + "\n")
		}
		sb.WriteString(dim(indent+"    "+loc) + "\n")
	}

	lastFeature, lastMilestone := "", ""
	for _, b := range report.Blockers {
		if b.Feature != lastFeature {
			if lastFeature != "" {
				sb.WriteString("\n")
			}
			suffix := " (" + b.Feature + ")"
			if b.Archived {
				suffix += " — archived"
			}
			sb.WriteString(bold(b.FeatureName) + dim(suffix) + "\n")
			lastFeature, lastMilestone = b.Feature, ""
		}
		if b.Milestone != lastMilestone {
			sb.WriteString("  " + b.Milestone + ": " + b.MilestoneName + "\n")
			lastMilestone = b.Milestone
		}
		writeBlocker(b, "    ")
	}

	if len(report.Unplaced) > 0 {
		if lastFeature != "" {
			sb.WriteString("\n")
		}
		sb.WriteString(bold("Outside any milestone") + "\n")
		sb.WriteString(dim("  Invisible to every count in Belmont — `belmont validate` reports these\n"))
		sb.WriteString(dim("  as task_outside_milestone, and `belmont repair` can file them.\n"))
		for _, b := range report.Unplaced {
			sb.WriteString("  " + b.FeatureName + dim(" ("+b.Feature+")") + "\n")
			writeBlocker(b, "    ")
		}
	}

	if len(report.Unknown) > 0 {
		sb.WriteString("\n")
		sb.WriteString(bold("Unreadable markers") + "\n")
		sb.WriteString(dim("  Belmont cannot tell what these are. A corrupted [!] is still a question\n"))
		sb.WriteString(dim("  waiting on someone. Run `belmont validate` — it exits 1 on these — and\n"))
		sb.WriteString(dim("  `belmont repair` can settle them against the commit log.\n"))
		for _, b := range report.Unknown {
			sb.WriteString("  " + b.FeatureName + dim(" ("+b.Feature+")") + "\n")
			writeBlocker(b, "    ")
		}
	}

	// The worktree warning is per-path, not per-report. In a
	// single-feature-parallel run one milestone's blocker can live in a
	// worktree while another's is still in master, and the advice inverts
	// between them: editing master is wrong for the first (overwritten on
	// merge) and exactly right for the second (there is no worktree file, and
	// the next wave reads master). One blanket sentence sends half the readers
	// to a file that does not exist.
	var anyLive, anyMaster bool
	all := append(append(append([]blockerEntry{}, report.Blockers...), report.Unplaced...), report.Unknown...)
	for _, b := range all {
		if b.LiveFrom != "" {
			anyLive = true
		} else {
			anyMaster = true
		}
	}

	sb.WriteString("\n")
	sb.WriteString(dim("A blocked task never clears itself. Answer the question, then flip the\n"))
	sb.WriteString(dim("marker to [ ] in the file named above — /belmont:next only ever picks up\n"))
	sb.WriteString(dim("a [ ] task, so it cannot act on one while it is still [!].\n"))
	if anyLive {
		sb.WriteString(dim("\nAn auto run is active. For the paths above that are inside a worktree,\n"))
		sb.WriteString(dim("edit there or use `belmont steer` — an edit to master is overwritten when\n"))
		sb.WriteString(dim("that worktree merges.\n"))
		if anyMaster {
			sb.WriteString(dim("The paths under .belmont/features/ have no worktree yet: edit those\n"))
			sb.WriteString(dim("directly, and the next wave picks the answer up.\n"))
		}
	}
	writeBlockerSkipped(&sb, report, dim)
	writeBlockerLiveGaps(&sb, report, dim)
	return sb.String()
}

func writeBlockerSkipped(sb *strings.Builder, report blockersReport, dim func(string) string) {
	if len(report.Skipped) == 0 {
		return
	}
	sb.WriteString(dim(fmt.Sprintf("\nCould not read PROGRESS.md for: %s — those features are not in this queue.\n",
		strings.Join(report.Skipped, ", "))))
}

// writeBlockerLiveGaps states which milestones were answered from master
// because their worktree could not be read. Called on both exit paths — most of
// all the "nothing is waiting on you" one, which is the affirmative this report
// has no right to make about a milestone it did not read.
func writeBlockerLiveGaps(sb *strings.Builder, report blockersReport, dim func(string) string) {
	if len(report.LiveGaps) == 0 {
		return
	}
	sb.WriteString(dim("\nDuring an active run, these milestones could not be read from their worktrees, so master's copy answered for them:\n"))
	for _, g := range report.LiveGaps {
		sb.WriteString(dim(fmt.Sprintf("  %s %s\n", g.Feature, g.describe())))
	}
	sb.WriteString(dim("  A blocker raised inside one of those worktrees is not in this queue: belmont recover --list\n"))
}
