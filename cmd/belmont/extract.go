package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The extraction census (P0-4).
//
// The tiered design in M3 assumes that once indented narrative is lifted out of
// a register, what remains is small. That was measured on two files. This
// answers it for every feature directory on disk, and it answers one specific
// open question the wave structure depends on: **does any feature's index
// ALONE still exceed 25,000 tokens after extraction?** If one does, M4's read
// path stops being an optimisation and becomes a prerequisite of M3.
//
// This is a census ONLY. It measures what extraction would yield and writes
// nothing — the move itself is M3/P1-1, and the round-trip proof that must
// precede any real write belongs there too.
//
// Two measurement rules, both learned the hard way and both load-bearing:
//
//   - **Walk <root>/.belmont/features/ directly. Never glob for PROGRESS.md
//     across the tree.** Worktrees under .claude/worktrees/ hold copies of the
//     same register — repo-4 carries a 1,293,057-byte copy of a file already
//     counted — and a glob double-counts them into every statistic.
//   - **State the denominator.** Feature directories and *live* registers are
//     different numbers: many directories have been archived to ARCHIVE.md by
//     /belmont:cleanup and hold no PROGRESS.md at all. A census that does not
//     say which it counted cannot be checked.

// extractionThresholdBytes is the size an extracted index must stay under.
// 100 KB is 25,000 tokens at the 4-bytes-per-token conversion every measurement
// in this feature uses, so the ceiling and the success criterion are one number
// rather than two that can drift apart.
const extractionThresholdBytes = 100_000

// bytesPerToken is the stated conversion. It is a documented approximation for
// FILE SIZE, and deliberately not the same kind of claim as a tool-reported
// token count — see metrics.go, where nothing is ever estimated.
const bytesPerToken = 4

// featureCensus is what extraction would yield for one feature's register.
type featureCensus struct {
	Repo        string `json:"repo"`
	Slug        string `json:"slug"`
	TotalBytes  int    `json:"total_bytes"`
	IndexBytes  int    `json:"index_bytes"`
	DetailBytes int    `json:"detail_bytes"`
	Milestones  int    `json:"milestones"`
	Tasks       int    `json:"tasks"`
	TasksWith   int    `json:"tasks_with_detail"`
	// OverThreshold is the answer to the open question: would this feature's
	// index alone still exceed the ceiling once its detail is removed?
	OverThreshold bool `json:"index_over_threshold"`
}

func (f featureCensus) reductionPct() float64 {
	if f.TotalBytes == 0 {
		return 0
	}
	return 100 * float64(f.DetailBytes) / float64(f.TotalBytes)
}

// unreadableRoot is a census subject that yielded nothing: the root does not
// exist, holds no .belmont/features, or could not be listed. It is carried in
// the report rather than logged, because the report is what gets quoted.
type unreadableRoot struct {
	Root   string `json:"root"`
	Reason string `json:"reason"`
}

// censusReport is the whole run, with its denominators stated.
//
// Roots is what was asked for; RootsWalked is what was actually measured. They
// are separate fields on purpose — every figure below them is a denominator or
// derived from one, and a denominator is only checkable if the scope that
// produced it is written down beside it.
type censusReport struct {
	Roots            []string         `json:"roots"`
	RootsWalked      []string         `json:"roots_walked"`
	UnreadableRoots  []unreadableRoot `json:"unreadable_roots"`
	CoverageComplete bool             `json:"coverage_complete"`
	FeatureDirs      int              `json:"feature_dirs"`
	LiveRegisters    int              `json:"live_registers"`
	ArchivedDirs     int              `json:"archived_dirs"`
	NoRegisterDirs   int              `json:"dirs_without_register"`
	ThresholdBytes   int              `json:"threshold_bytes"`
	TotalBytes       int              `json:"total_bytes"`
	TotalIndexBytes  int              `json:"total_index_bytes"`
	TotalDetailBytes int              `json:"total_detail_bytes"`
	OverBefore       []string         `json:"over_threshold_before"`
	OverAfter        []string         `json:"over_threshold_after"`
	Features         []featureCensus  `json:"features"`
}

// censusFeature measures one register.
//
// Detail is defined exactly as extraction defines it: the indented body beneath
// a task line, bounded by taskBodyEnd — the same separator blockers.go already
// uses, rather than a third line-scanner with its own idea of where a task ends.
// Everything else — milestone headings, task head lines, the Session History and
// Decisions Log sections below the milestones region — is index and stays put.
func censusFeature(repo, slug, content string) featureCensus {
	lines := strings.Split(content, "\n")
	fc := featureCensus{
		Repo:       repo,
		Slug:       slug,
		TotalBytes: len(content),
	}

	milestones := parseMilestones(content)
	fc.Milestones = len(milestones)

	// A line index is claimed by at most one task's body. Recording which lines
	// move — rather than summing per-task lengths — is what keeps a nested task
	// inside its parent's block from being counted twice.
	moved := make([]bool, len(lines))
	for _, ms := range milestones {
		for _, t := range ms.Tasks {
			fc.Tasks++
			idx := t.Line - 1 // task.Line is 1-based (state.go)
			if idx < 0 || idx >= len(lines) {
				continue
			}
			end := taskBodyEnd(lines, idx)
			if end <= idx {
				continue
			}
			fc.TasksWith++
			for j := idx + 1; j <= end && j < len(lines); j++ {
				moved[j] = true
			}
		}
	}

	for i, l := range lines {
		// +1 for the newline that separated it; the final line has none, but the
		// error is one byte on a file measured in hundreds of thousands.
		if moved[i] {
			fc.DetailBytes += len(l) + 1
		}
	}
	fc.IndexBytes = fc.TotalBytes - fc.DetailBytes
	fc.OverThreshold = fc.IndexBytes > extractionThresholdBytes
	return fc
}

// runCensus walks each root's .belmont/features/ and measures every live
// register. Roots are walked directly — see the file comment on why globbing is
// wrong.
//
// **A root that cannot be read is a reported failure, never a skip.** This used
// to be `os.IsNotExist(err) -> continue`, and that one line is how this census
// published a wrong denominator: it walked four of the PRD's five repos,
// reported 138 dirs / 65 live instead of 168 / 82, and then used the smaller
// number to declare the PRD's own figure wrong. Nothing objected, because an
// incomplete walk and a complete one produced output that looked identical.
// Two reviewers then hit the same failure from the other direction — the
// documented command's unexpanded tildes resolved four roots under the CWD, and
// it answered 43 registers instead of 65, silently.
//
// **Error, not warning — chosen deliberately.** A census's entire value is that
// its coverage is knowable, and a warning printed above a table of plausible
// numbers is read once and copied never: the numbers travel into other
// documents and the caveat does not. That is not a hypothetical here, it is the
// documented history of this exact figure. So the number must not exist at all
// unless its scope is whole.
//
// The cost of that strictness — a five-repo census being useless while one repo
// sits on an unmounted volume — is paid by allowUnreadableRoots, an opt-in the
// operator has to type. It is not a quieter version of the same failure: the
// missed roots stay in the report (RootsWalked, UnreadableRoots,
// CoverageComplete) and are stated in both the text and JSON output, so a
// reader of the output can tell what was covered without re-running anything.
// Dropping the root from the command line instead would lose exactly that.
func runCensus(roots []string, only string, allowUnreadableRoots bool) (censusReport, error) {
	// Both slices start empty rather than nil so the JSON always carries the
	// coverage fields as arrays, and a consumer can read them without a
	// null check.
	rep := censusReport{
		Roots:           roots,
		RootsWalked:     []string{},
		UnreadableRoots: []unreadableRoot{},
		ThresholdBytes:  extractionThresholdBytes,
	}

	for _, root := range roots {
		featuresDir := filepath.Join(root, ".belmont", "features")
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			// Collect every unreadable root before failing, so one run names all
			// of them rather than making the operator rediscover them one at a
			// time.
			reason := err.Error()
			if os.IsNotExist(err) {
				reason = "no " + filepath.Join(".belmont", "features") + " directory under this root"
			}
			rep.UnreadableRoots = append(rep.UnreadableRoots, unreadableRoot{Root: root, Reason: reason})
			continue
		}
		rep.RootsWalked = append(rep.RootsWalked, root)
		repoName := filepath.Base(root)
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			slug := e.Name()
			if only != "" && slug != only {
				continue
			}
			rep.FeatureDirs++
			dir := filepath.Join(featuresDir, slug)
			progressPath := filepath.Join(dir, "PROGRESS.md")
			data, err := os.ReadFile(progressPath)
			if err != nil {
				rep.NoRegisterDirs++
				if fileExists(filepath.Join(dir, "ARCHIVE.md")) {
					rep.ArchivedDirs++
				}
				continue
			}
			rep.LiveRegisters++
			fc := censusFeature(repoName, slug, string(data))
			rep.Features = append(rep.Features, fc)
			rep.TotalBytes += fc.TotalBytes
			rep.TotalIndexBytes += fc.IndexBytes
			rep.TotalDetailBytes += fc.DetailBytes
			label := repoName + "/" + slug
			if fc.TotalBytes > extractionThresholdBytes {
				rep.OverBefore = append(rep.OverBefore, label)
			}
			if fc.OverThreshold {
				rep.OverAfter = append(rep.OverAfter, label)
			}
		}
	}

	sort.Slice(rep.Features, func(i, j int) bool {
		return rep.Features[i].TotalBytes > rep.Features[j].TotalBytes
	})

	rep.CoverageComplete = len(rep.UnreadableRoots) == 0
	if !rep.CoverageComplete && !allowUnreadableRoots {
		var b strings.Builder
		fmt.Fprintf(&b, "extract: %d of %d roots could not be read, so this census would cover only part of what was asked for:",
			len(rep.UnreadableRoots), len(roots))
		for _, u := range rep.UnreadableRoots {
			fmt.Fprintf(&b, "\n  %s: %s", u.Root, u.Reason)
		}
		b.WriteString("\nRefusing rather than reporting a smaller denominator: an incomplete census reads exactly like a complete one once its numbers are quoted elsewhere.")
		b.WriteString("\nFix the paths (absolute, no ~ — only the first is a shell word), or pass --allow-unreadable-roots to census the readable roots anyway; the report then states which roots it missed.")
		return rep, errors.New(b.String())
	}
	return rep, nil
}

func percentile(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)))
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// renderCoverage states the scope of the run before any figure derived from it.
// Which roots were walked, and which were asked for and missed, are the first
// thing a reader needs and the first thing a copied-out number loses.
func renderCoverage(b *strings.Builder, rep censusReport) {
	if rep.CoverageComplete {
		fmt.Fprintf(b, "Coverage: COMPLETE — all %d requested roots were read.\n", len(rep.Roots))
	} else {
		fmt.Fprintf(b, "Coverage: INCOMPLETE — %d of %d requested roots could not be read.\n",
			len(rep.UnreadableRoots), len(rep.Roots))
	}
	for _, r := range rep.RootsWalked {
		fmt.Fprintf(b, "  walked  %s\n", r)
	}
	for _, u := range rep.UnreadableRoots {
		fmt.Fprintf(b, "  MISSED  %s (%s)\n", u.Root, u.Reason)
	}
	if !rep.CoverageComplete {
		b.WriteString("  Every figure below counts only the walked roots and is a LOWER BOUND on the estate.\n")
	}
}

// renderCoverageFooter repeats an incomplete-coverage verdict at the end of the
// report. A caveat at the top of a long table is the one a reader scrolls past.
func renderCoverageFooter(b *strings.Builder, rep censusReport) {
	if rep.CoverageComplete {
		return
	}
	missed := make([]string, 0, len(rep.UnreadableRoots))
	for _, u := range rep.UnreadableRoots {
		missed = append(missed, u.Root)
	}
	fmt.Fprintf(b, "\n!! COVERAGE INCOMPLETE — %d of %d roots were never read (%s).\n",
		len(rep.UnreadableRoots), len(rep.Roots), strings.Join(missed, ", "))
	b.WriteString("!! Do not quote these totals as the estate: they are lower bounds over a partial walk.\n")
}

func renderCensus(rep censusReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Extraction census (dry run — nothing written)\n")
	renderCoverage(&b, rep)
	b.WriteString("\n")

	// State the denominator before any statistic derived from it.
	fmt.Fprintf(&b, "Denominator: %d feature directories, of which %d carry a live PROGRESS.md.\n",
		rep.FeatureDirs, rep.LiveRegisters)
	fmt.Fprintf(&b, "  %d have no register (%d of those archived to ARCHIVE.md).\n",
		rep.NoRegisterDirs, rep.ArchivedDirs)
	fmt.Fprintf(&b, "  Every figure below is over the %d live registers only.\n\n", rep.LiveRegisters)

	if rep.LiveRegisters == 0 {
		renderCoverageFooter(&b, rep)
		return b.String()
	}

	totals := make([]int, 0, len(rep.Features))
	indexes := make([]int, 0, len(rep.Features))
	for _, f := range rep.Features {
		totals = append(totals, f.TotalBytes)
		indexes = append(indexes, f.IndexBytes)
	}
	sort.Ints(totals)
	sort.Ints(indexes)

	reduction := 0.0
	if rep.TotalBytes > 0 {
		reduction = 100 * float64(rep.TotalDetailBytes) / float64(rep.TotalBytes)
	}
	fmt.Fprintf(&b, "Today          total %12d B   median %9d   p90 %9d   max %9d\n",
		rep.TotalBytes, percentile(totals, 0.5), percentile(totals, 0.9), totals[len(totals)-1])
	fmt.Fprintf(&b, "After extract  total %12d B   median %9d   p90 %9d   max %9d\n",
		rep.TotalIndexBytes, percentile(indexes, 0.5), percentile(indexes, 0.9), indexes[len(indexes)-1])
	fmt.Fprintf(&b, "Detail moved   %12d B  (%.1f%% of all register bytes)\n\n",
		rep.TotalDetailBytes, reduction)

	fmt.Fprintf(&b, "Threshold: %d B (= %d tokens at %d bytes/token)\n",
		rep.ThresholdBytes, rep.ThresholdBytes/bytesPerToken, bytesPerToken)
	fmt.Fprintf(&b, "  Over it today:            %d\n", len(rep.OverBefore))
	fmt.Fprintf(&b, "  Still over it after:      %d\n", len(rep.OverAfter))
	if len(rep.OverAfter) == 0 {
		b.WriteString("  -> No feature's index alone exceeds the ceiling. M4 does NOT become a prerequisite of M3.\n\n")
	} else {
		b.WriteString("  -> These features' indexes alone STILL exceed the ceiling after extraction:\n")
		for _, s := range rep.OverAfter {
			b.WriteString("       " + s + "\n")
		}
		b.WriteString("  -> Extraction alone does not bring every register under the ceiling.\n")
		b.WriteString("     Per the PRD's open question, M4's read path becomes a PREREQUISITE of M3\n")
		b.WriteString("     rather than its successor, and the wave structure changes. That is a\n")
		b.WriteString("     tech-planning decision — only tech-planning may restructure milestones.\n\n")
	}

	fmt.Fprintf(&b, "%-40s %11s %11s %11s %7s\n", "FEATURE", "TODAY", "INDEX", "DETAIL", "SAVED")
	shown := rep.Features
	if len(shown) > 20 {
		shown = shown[:20]
	}
	for _, f := range shown {
		fmt.Fprintf(&b, "%-40s %11d %11d %11d %6.1f%%\n",
			truncateTail(f.Repo+"/"+f.Slug, 40), f.TotalBytes, f.IndexBytes, f.DetailBytes, f.reductionPct())
	}
	if len(rep.Features) > len(shown) {
		fmt.Fprintf(&b, "... and %d more (use --format json for all)\n", len(rep.Features)-len(shown))
	}
	renderCoverageFooter(&b, rep)
	return b.String()
}

// runExtractCmd is the census entry point.
//
// P0-4 ships the measuring half only. --dry-run is therefore mandatory rather
// than optional: the write path, and the byte-for-byte round-trip proof that
// must precede it, are M3/P1-1. Refusing loudly is better than a flag that
// looks available and silently does nothing.
func runExtractCmd(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	root := fs.String("root", ".", "project root")
	feature := fs.String("feature", "", "single feature slug (default: every feature in --root)")
	all := fs.Bool("all", false, "census every feature directory (default when --feature is absent)")
	dryRun := fs.Bool("dry-run", false, "report only; required in this release")
	extraRoots := fs.String("roots", "", "comma-separated additional project roots to include in the census")
	allowUnreadable := fs.Bool("allow-unreadable-roots", false,
		"census the readable roots even if one cannot be read; the report states which roots were missed")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = all // accepted for forward compatibility; census-over-all is the default

	if !*dryRun {
		return fmt.Errorf("extract: --dry-run is required — this release ships the census only; " +
			"the write path and its round-trip proof land with the detail tier (M3/P1-1)")
	}

	var roots []string
	abs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("extract: resolve root: %w", err)
	}
	roots = append(roots, abs)
	for _, r := range strings.Split(*extraRoots, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		a, err := filepath.Abs(r)
		if err != nil {
			return fmt.Errorf("extract: resolve root %s: %w", r, err)
		}
		roots = append(roots, a)
	}

	rep, err := runCensus(roots, strings.TrimSpace(*feature), *allowUnreadable)
	if err != nil {
		return err
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Print(renderCensus(rep))
	return nil
}
