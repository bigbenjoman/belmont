package main

import (
	"encoding/json"
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

// censusReport is the whole run, with its denominators stated.
type censusReport struct {
	Roots            []string        `json:"roots"`
	FeatureDirs      int             `json:"feature_dirs"`
	LiveRegisters    int             `json:"live_registers"`
	ArchivedDirs     int             `json:"archived_dirs"`
	NoRegisterDirs   int             `json:"dirs_without_register"`
	ThresholdBytes   int             `json:"threshold_bytes"`
	TotalBytes       int             `json:"total_bytes"`
	TotalIndexBytes  int             `json:"total_index_bytes"`
	TotalDetailBytes int             `json:"total_detail_bytes"`
	OverBefore       []string        `json:"over_threshold_before"`
	OverAfter        []string        `json:"over_threshold_after"`
	Features         []featureCensus `json:"features"`
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
func runCensus(roots []string, only string) (censusReport, error) {
	rep := censusReport{Roots: roots, ThresholdBytes: extractionThresholdBytes}

	for _, root := range roots {
		featuresDir := filepath.Join(root, ".belmont", "features")
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return rep, fmt.Errorf("extract: read %s: %w", featuresDir, err)
		}
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

func renderCensus(rep censusReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Extraction census (dry run — nothing written)\n")
	fmt.Fprintf(&b, "Roots: %s\n\n", strings.Join(rep.Roots, ", "))

	// State the denominator before any statistic derived from it.
	fmt.Fprintf(&b, "Denominator: %d feature directories, of which %d carry a live PROGRESS.md.\n",
		rep.FeatureDirs, rep.LiveRegisters)
	fmt.Fprintf(&b, "  %d have no register (%d of those archived to ARCHIVE.md).\n",
		rep.NoRegisterDirs, rep.ArchivedDirs)
	fmt.Fprintf(&b, "  Every figure below is over the %d live registers only.\n\n", rep.LiveRegisters)

	if rep.LiveRegisters == 0 {
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

	rep, err := runCensus(roots, strings.TrimSpace(*feature))
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
