package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Metrics: what a milestone actually cost.
//
// Nothing in Belmont measured its own spend before this. The feature's success
// bar is tokens and wall-clock per *verified* milestone, and the framework's own
// history contains an optimisation that measurement later disproved — so an
// unmeasured claim is the one thing this must not produce.
//
// The governing rule is that a figure is either reported by the tool that spent
// it or it is null. **Nothing here estimates a token count**, not from character
// counts, not from a ratio, not from a previous run. A null with a stated reason
// is a usable measurement ("this host cannot tell us"); a plausible invention is
// not, and it would silently contaminate the M1 baseline that every later
// milestone is judged against.
//
// Records live at .belmont/metrics/<feature>.jsonl, are gitignored, and are
// append-only: one line per phase, so two runs of the same feature are compared
// by reading the file rather than by keeping any state.

// toolUsage is a token count a tool reported about itself. Every field came out
// of the tool's own JSON; none is derived.
type toolUsage struct {
	Input         int64
	Output        int64
	CacheCreation int64
	CacheRead     int64
}

// metricsRecord is one phase of one milestone: the unit the success bar is
// expressed in. The token fields are pointers so that "not reported" and "zero"
// stay distinguishable — encoding/json writes a nil pointer as null, and a zero
// int is a real measurement some hosts genuinely return.
type metricsRecord struct {
	Run           string `json:"run"`
	Feature       string `json:"feature"`
	Milestone     string `json:"milestone,omitempty"`
	Phase         string `json:"phase"`
	Tool          string `json:"tool"`
	WallMs        int64  `json:"wall_ms"`
	Input         *int64 `json:"input"`
	Output        *int64 `json:"output"`
	CacheCreation *int64 `json:"cache_creation"`
	CacheRead     *int64 `json:"cache_read"`
	CriticalPath  bool   `json:"critical_path"`
	Note          string `json:"note,omitempty"`
}

// usageUnavailableNote explains, per tool, why no token count is recorded.
//
// The distinction matters and is load-bearing rather than pedantic:
//
//   - copilot, pi and opencode CANNOT report usage. copilot exposes no
//     output-format flag; pi's `-p` emits plain text by documented design; and
//     opencode's `--format json` is deliberately not passed, because its escaped
//     event stream defeats extractDecisionJSON. opencode is the Oct-2026
//     local-LLM target, so this degradation is permanent and the plan must not
//     pretend otherwise.
//   - gemini and cursor DO emit a usage block, but Belmont has never verified
//     its shape against a live run. Writing a parser against a guessed schema is
//     worse than recording null: it fails silently, either always-zero or
//     always-absent, and a baseline built on it cannot be trusted. These become
//     real measurements the moment someone runs the tool and reads the JSON.
var usageUnavailableNote = map[string]string{
	"copilot":  "tool reports no usage (no output-format flag)",
	"pi":       "tool reports no usage (-p emits plain text)",
	"opencode": "tool reports no usage (--format json deliberately not passed; see toolexec.go)",
	"gemini":   "usage schema not yet verified against a live run — not estimated",
	"cursor":   "usage schema not yet verified against a live run — not estimated",
}

// toolReportsUsage says whether Belmont can extract a verified token count from
// this tool. Verified empirically 2026-08-18 for claude (result event) and codex
// (turn.completed event); see usageUnavailableNote for why the rest are false.
func toolReportsUsage(tool string) bool {
	switch tool {
	case "claude", "codex":
		return true
	}
	return false
}

// codexUsageFromLine extracts usage from a codex `turn.completed` NDJSON event.
//
// Verified against codex 2026-08-18. The event carries:
//
//	{"type":"turn.completed","usage":{"input_tokens":17304,"cached_input_tokens":3456,
//	 "cache_write_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":13}}
//
// Note the field names differ from Claude's: cached_input_tokens is the read,
// cache_write_input_tokens is the creation. Mapping them the other way round
// would produce a plausible number that is wrong, which is exactly the failure
// this file exists to avoid.
func codexUsageFromLine(line []byte) *toolUsage {
	var ev struct {
		Type  string `json:"type"`
		Usage *struct {
			InputTokens           int64 `json:"input_tokens"`
			CachedInputTokens     int64 `json:"cached_input_tokens"`
			CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
			OutputTokens          int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}
	if ev.Type != "turn.completed" || ev.Usage == nil {
		return nil
	}
	return &toolUsage{
		Input:         ev.Usage.InputTokens,
		Output:        ev.Usage.OutputTokens,
		CacheCreation: ev.Usage.CacheWriteInputTokens,
		CacheRead:     ev.Usage.CachedInputTokens,
	}
}

// buildMetricsRecord assembles one record. usage may be nil, in which case the
// token fields are null and a note says why — the note is never omitted when
// usage is absent, because a bare null is indistinguishable from a bug.
func buildMetricsRecord(runID, feature, milestone, phase, tool string, wallMs int64, usage *toolUsage, criticalPath bool) metricsRecord {
	rec := metricsRecord{
		Run:          runID,
		Feature:      feature,
		Milestone:    milestone,
		Phase:        phase,
		Tool:         tool,
		WallMs:       wallMs,
		CriticalPath: criticalPath,
	}
	if usage != nil {
		in, out := usage.Input, usage.Output
		cc, cr := usage.CacheCreation, usage.CacheRead
		rec.Input, rec.Output, rec.CacheCreation, rec.CacheRead = &in, &out, &cc, &cr
		return rec
	}
	if note, ok := usageUnavailableNote[tool]; ok {
		rec.Note = note
	} else {
		rec.Note = "tool reported no usage"
	}
	return rec
}

// metricsPath is the append-only record for one feature. Under .belmont/metrics/,
// which ensureStateFiles gitignores: bookkeeping already accounts for 41-46% of
// commit volume in these repos, and a metrics file per run would worsen that for
// no analytical gain.
func metricsPath(root, feature string) string {
	slug := feature
	if slug == "" {
		slug = "_unassigned"
	}
	return filepath.Join(root, ".belmont", "metrics", slug+".jsonl")
}

// metricsRoot is the root .belmont/metrics/ resolves under: MetricsRoot when
// set, Root otherwise. The fallback is what keeps the serial path unchanged —
// only the two worktree sites in auto_parallel.go need to know the difference.
func (cfg loopConfig) metricsRoot() string {
	if cfg.MetricsRoot != "" {
		return cfg.MetricsRoot
	}
	return cfg.Root
}

// appendMetricsRecord appends one JSONL line. Deliberately best-effort at the
// call sites: a metrics failure must never fail a run that otherwise succeeded.
//
// This is an append of a single complete line, not a rewrite, so it does not go
// through writeStateFile — converting an append into a read-modify-rewrite would
// widen the window rather than close it (see knowledge/cross-cutting/state-atomicity.md).
//
// Concurrency: every milestone in a parallel wave appends to this one file under
// the main root, so the path is shared — by goroutines within a wave, and by any
// other belmont process running against the same checkout. It is safe, and the
// reason is narrow enough to be worth writing down rather than trusting:
//
//   - The file is opened O_APPEND, so the kernel seeks to end-of-file and writes
//     as one indivisible operation. Two appenders cannot land at the same offset
//     and overwrite each other. (Go maps O_APPEND to FILE_APPEND_DATA on Windows,
//     which carries the same guarantee.)
//   - The whole record INCLUDING its terminating newline goes out in a single
//     f.Write. That is the load-bearing detail: writing the payload and the '\n'
//     as two calls would let another appender interleave between them and splice
//     two records into one unparseable line. Do not split this write, and do not
//     wrap the file in a bufio.Writer — a buffer flushes on its own boundaries,
//     not on record boundaries.
//   - Records are a few hundred bytes, far below any size at which a local
//     filesystem would split one write() into several.
//
// So the hazard here is a garbled *line*, not a torn *file* — categorically
// unlike the truncate-then-write window writeStateFile exists to close — and the
// single-write rule removes it. TestAppendMetricsRecordIsConcurrencySafe pins it.
// What this does NOT survive is a network filesystem: O_APPEND is not atomic over
// NFS. Belmont's metrics live beside the checkout, so that is out of scope — but
// if .belmont/ ever legitimately lands on NFS, this is the call site to lock.
func appendMetricsRecord(root string, rec metricsRecord) error {
	path := metricsPath(root, rec.Feature)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// readMetricsRecords reads a feature's records. A malformed line is skipped
// rather than fatal: the file is append-only and local, so a truncated final
// line from an interrupted run must not make the whole history unreadable.
func readMetricsRecords(root, feature string) ([]metricsRecord, error) {
	path := metricsPath(root, feature)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []metricsRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec metricsRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

// criticalPathMilestones returns the milestones lying on a longest chain through
// the dependency DAG.
//
// This is NOT the wave index. computeWaves is a Kahn topological levelling: a
// milestone sits in wave 2 whenever any dependency sits in wave 1, whether or
// not it lies on the longest chain. Latency correlates with critical-path tokens
// at r = 0.901 and with total tokens far less, so attributing spend to the chain
// rather than merely summing it is what makes the final comparison mean anything.
//
// Depth is measured in milestones, every edge weighted 1 — Belmont has no
// per-milestone cost estimate before a run, and inventing one would be the same
// error as estimating a token count.
func criticalPathMilestones(milestones []milestone) map[string]bool {
	onPath := make(map[string]bool)
	if len(milestones) == 0 {
		return onPath
	}

	byID := make(map[string]milestone, len(milestones))
	for _, m := range milestones {
		byID[m.ID] = m
	}

	// depth(m) = 1 + max(depth(dep)). Memoised, with a visiting set so a cycle
	// (which computeWaves reports as an error) degrades to depth 0 here rather
	// than recursing forever — this function must never be the thing that hangs
	// a run.
	depth := make(map[string]int, len(milestones))
	visiting := make(map[string]bool, len(milestones))
	var compute func(id string) int
	compute = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true
		best := 0
		for _, dep := range byID[id].Deps {
			if _, ok := byID[dep]; !ok {
				// A dangling dependency contributes nothing. It is a real
				// defect, but it belongs to P2-4, which makes it an error;
				// silently absorbing it here is not this function's business.
				continue
			}
			if d := compute(dep); d > best {
				best = d
			}
		}
		visiting[id] = false
		depth[id] = best + 1
		return depth[id]
	}

	maxDepth := 0
	for _, m := range milestones {
		if d := compute(m.ID); d > maxDepth {
			maxDepth = d
		}
	}

	// Walk back from every deepest milestone through the dependencies that
	// realise its depth. A milestone can lie on several longest chains, and all
	// of them are on the critical path.
	var mark func(id string)
	mark = func(id string) {
		if onPath[id] {
			return
		}
		onPath[id] = true
		d := depth[id]
		for _, dep := range byID[id].Deps {
			if _, ok := byID[dep]; !ok {
				continue
			}
			if depth[dep] == d-1 {
				mark(dep)
			}
		}
	}
	for _, m := range milestones {
		if depth[m.ID] == maxDepth {
			mark(m.ID)
		}
	}
	return onPath
}

// metricsSummary aggregates one feature's records for `belmont metrics`.
type metricsSummary struct {
	Feature       string   `json:"feature"`
	Runs          int      `json:"runs"`
	Phases        int      `json:"phases"`
	WallMs        int64    `json:"wall_ms"`
	Input         int64    `json:"input"`
	Output        int64    `json:"output"`
	CacheCreation int64    `json:"cache_creation"`
	CacheRead     int64    `json:"cache_read"`
	CriticalWall  int64    `json:"critical_path_wall_ms"`
	CriticalInput int64    `json:"critical_path_input"`
	Unreported    int      `json:"phases_without_usage"`
	ByRun         []runAgg `json:"runs_detail"`
}

type runAgg struct {
	Run           string `json:"run"`
	Phases        int    `json:"phases"`
	WallMs        int64  `json:"wall_ms"`
	Input         int64  `json:"input"`
	Output        int64  `json:"output"`
	CacheCreation int64  `json:"cache_creation"`
	CacheRead     int64  `json:"cache_read"`
	Unreported    int    `json:"phases_without_usage"`
}

func summariseMetrics(feature string, recs []metricsRecord) metricsSummary {
	s := metricsSummary{Feature: feature, Phases: len(recs)}
	perRun := map[string]*runAgg{}
	var order []string

	for _, r := range recs {
		s.WallMs += r.WallMs
		if r.CriticalPath {
			s.CriticalWall += r.WallMs
		}
		agg, ok := perRun[r.Run]
		if !ok {
			agg = &runAgg{Run: r.Run}
			perRun[r.Run] = agg
			order = append(order, r.Run)
		}
		agg.Phases++
		agg.WallMs += r.WallMs

		if r.Input == nil && r.Output == nil {
			s.Unreported++
			agg.Unreported++
			continue
		}
		if r.Input != nil {
			s.Input += *r.Input
			agg.Input += *r.Input
			if r.CriticalPath {
				s.CriticalInput += *r.Input
			}
		}
		if r.Output != nil {
			s.Output += *r.Output
			agg.Output += *r.Output
		}
		if r.CacheCreation != nil {
			s.CacheCreation += *r.CacheCreation
			agg.CacheCreation += *r.CacheCreation
		}
		if r.CacheRead != nil {
			s.CacheRead += *r.CacheRead
			agg.CacheRead += *r.CacheRead
		}
	}

	sort.Strings(order)
	for _, run := range order {
		s.ByRun = append(s.ByRun, *perRun[run])
	}
	s.Runs = len(order)
	return s
}

func renderMetricsSummary(s metricsSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Metrics: %s\n", s.Feature)
	if s.Phases == 0 {
		b.WriteString("  No records yet. They are written by `belmont auto` / `belmont loop`.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  %d run(s), %d phase(s), %s wall-clock\n", s.Runs, s.Phases, formatDurationMs(s.WallMs))
	fmt.Fprintf(&b, "  tokens: %d in / %d out (cache %d created, %d read)\n",
		s.Input, s.Output, s.CacheCreation, s.CacheRead)
	fmt.Fprintf(&b, "  critical path: %s wall-clock, %d input tokens\n",
		formatDurationMs(s.CriticalWall), s.CriticalInput)
	if s.Unreported > 0 {
		fmt.Fprintf(&b, "  %d phase(s) reported no usage — wall-clock only, never estimated\n", s.Unreported)
	}
	b.WriteString("\n  Run                       Phases   Wall        Input    Output\n")
	for _, r := range s.ByRun {
		fmt.Fprintf(&b, "  %-24s  %6d   %-10s  %7d  %7d\n",
			r.Run, r.Phases, formatDurationMs(r.WallMs), r.Input, r.Output)
	}
	return b.String()
}

func formatDurationMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := ms / 1000
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh%02dm", sec/3600, (sec%3600)/60)
}

// runMetricsCmd prints the recorded cost of a feature's runs.
//
// The success bar this feature is judged against is tokens AND wall-clock per
// *verified* milestone, measured against an M1 baseline. This is the command
// that reads it back — P0-3 captures a baseline with it, and P3-3 compares
// against that baseline with the same command, so the two halves of the
// comparison are produced by one code path.
func runMetricsCmd(args []string) error {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	root := fs.String("root", ".", "project root")
	feature := fs.String("feature", "", "feature slug (required)")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*feature) == "" {
		return fmt.Errorf("metrics: --feature SLUG is required")
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("metrics: resolve root: %w", err)
	}

	recs, err := readMetricsRecords(absRoot, *feature)
	if err != nil {
		return fmt.Errorf("metrics: read records: %w", err)
	}
	summary := summariseMetrics(*feature, recs)

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	fmt.Print(renderMetricsSummary(summary))
	return nil
}
