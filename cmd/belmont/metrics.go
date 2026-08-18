package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
//
// The same rule has a second edge, which is easy to miss because nothing is
// estimated on it: **an arithmetic result that has no definition is also not a
// measurement**. `input_tokens` does not mean the same thing to every tool, so
// adding it across tools yields a number that is neither the uncached total nor
// the whole prompt. Every record therefore carries what its Input counts
// (`input_semantics`, written at ingest — see inputSemantics), and
// summariseMetrics reports a combined Input only when every contributing record
// shares one definition. Where they differ it reports null with a stated reason
// and the per-tool figures, exactly as it does for a host that cannot report.

// toolUsage is a token count a tool reported about itself. Every field came out
// of the tool's own JSON; none is derived.
type toolUsage struct {
	Input         int64
	Output        int64
	CacheCreation int64
	CacheRead     int64
}

// inputSemantics records what a tool's reported `input_tokens` actually counts.
//
// This is the aggregation boundary's problem, not the parsers'. Both per-tool
// parsers are correct and the field names are not transposed — but the two
// vendors give the same-named field two different definitions:
//
//   - claude: `input_tokens` is the uncached remainder only. The whole prompt is
//     input_tokens + cache_creation_input_tokens + cache_read_input_tokens.
//   - codex (OpenAI lineage): `input_tokens` is the whole prompt, cache reads
//     included, so it overlaps CacheRead.
//
// codex's half was verified against a live run rather than assumed (codex-cli
// 0.147.0, 2026-08-18). Three turns of one session reported input/cached of
// 17308/4480 -> 34647/21248 -> 52003/38016. Those figures are session-cumulative,
// so the per-turn values are 17308/4480, 17339/16768 and 17356/16768: each turn
// re-sends the same ~17.3k context and the cache read is a subset of it. Under
// the excluding reading the context would have had to grow from 21,788 to 34,107
// tokens across a 26-token exchange, which it plainly did not.
//
// What that run did NOT settle is whether cache *writes* also sit inside codex's
// input_tokens — cache_write_input_tokens was 0 on every turn, so the question
// never arose. It does not need settling here: the rule below keys off the
// definitions differing at all, not off which part differs.
//
// Belmont **refuses to aggregate** across differing definitions rather than
// normalising to a tool-independent quantity. Both were open. Normalising (store,
// say, whole-prompt tokens for every tool) is only correct while every tool's
// semantics are known and stay known; a new tool, or a vendor redefining the
// field under us, would then silently produce a wrong-but-plausible baseline —
// which is the failure being fixed, re-armed. Refusing is correct under every
// interpretation, including ones nobody has verified yet, and it costs no
// information: the per-tool figures are reported beside the null.
//
// The semantics travel with the record as data (metricsRecord.InputSemantics),
// never as a comment. A comment at the parse site is precisely what failed here:
// it does not reach summariseMetrics, and it does not reach a record already on
// disk.
type inputSemantics string

const (
	// inputExcludesCacheRead: Input counts only tokens processed at full price;
	// cache reads and writes are reported separately and do not overlap it.
	inputExcludesCacheRead inputSemantics = "excludes_cache_read"
	// inputIncludesCacheRead: Input counts the whole prompt, cache reads
	// included, so it overlaps CacheRead.
	inputIncludesCacheRead inputSemantics = "includes_cache_read"
	// inputSemanticsUnknown: a count Belmont cannot define. Recorded rather than
	// guessed, and never summed with any other tool's.
	inputSemanticsUnknown inputSemantics = "unknown"
)

// toolInputSemantics is the table, per tool, verified as described above. Every
// tool toolReportsUsage says yes to must appear here — a reported count with no
// recorded definition is the defect this table exists to prevent, and
// TestInputSemanticsCoversEveryToolThatReportsUsage pins it.
var toolInputSemantics = map[string]inputSemantics{
	"claude": inputExcludesCacheRead,
	"codex":  inputIncludesCacheRead,
}

// inputSemanticsFor is the ingest-side lookup. An unlisted tool records
// "unknown", which is a usable fact; guessing a definition is not.
func inputSemanticsFor(tool string) inputSemantics {
	if s, ok := toolInputSemantics[tool]; ok {
		return s
	}
	return inputSemanticsUnknown
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
	// InputSemantics is what Input counts, according to the tool that reported
	// it. Written at ingest so the distinction reaches summariseMetrics and any
	// later reader of the file, rather than living in a comment beside a parser.
	// Absent on a record written before this field existed; such a record
	// aggregates as unknown, which is fail-closed by design.
	InputSemantics inputSemantics `json:"input_semantics,omitempty"`
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
		rec.InputSemantics = inputSemanticsFor(tool)
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
//
// Input and CriticalInput are pointers for the same reason the record's token
// fields are: a figure that cannot be defined is null, never a plausible number.
// Here the cause is not a host that cannot report but records spanning tools
// that define input_tokens differently (see inputSemantics). InputNote always
// says which, and ByTool carries every figure the combined total would have
// held, beside the definition it was measured under.
type metricsSummary struct {
	Feature       string    `json:"feature"`
	Runs          int       `json:"runs"`
	Phases        int       `json:"phases"`
	WallMs        int64     `json:"wall_ms"`
	Input         *int64    `json:"input"`
	InputNote     string    `json:"input_note,omitempty"`
	Output        int64     `json:"output"`
	CacheCreation int64     `json:"cache_creation"`
	CacheRead     int64     `json:"cache_read"`
	CriticalWall  int64     `json:"critical_path_wall_ms"`
	CriticalInput *int64    `json:"critical_path_input"`
	Unreported    int       `json:"phases_without_usage"`
	ByTool        []toolAgg `json:"tools_detail"`
	ByRun         []runAgg  `json:"runs_detail"`
}

// toolAgg is the per-tool breakdown. It exists so that refusing to combine Input
// costs no information: within one tool the definition is constant, so these
// sums are always meaningful, whatever the mix above them.
type toolAgg struct {
	Tool           string         `json:"tool"`
	InputSemantics inputSemantics `json:"input_semantics"`
	Phases         int            `json:"phases"`
	WallMs         int64          `json:"wall_ms"`
	Input          int64          `json:"input"`
	CriticalInput  int64          `json:"critical_path_input"`
	Output         int64          `json:"output"`
	CacheCreation  int64          `json:"cache_creation"`
	CacheRead      int64          `json:"cache_read"`
	Unreported     int            `json:"phases_without_usage"`
}

type runAgg struct {
	Run           string `json:"run"`
	Phases        int    `json:"phases"`
	WallMs        int64  `json:"wall_ms"`
	Input         *int64 `json:"input"`
	InputNote     string `json:"input_note,omitempty"`
	Output        int64  `json:"output"`
	CacheCreation int64  `json:"cache_creation"`
	CacheRead     int64  `json:"cache_read"`
	Unreported    int    `json:"phases_without_usage"`
}

// inputAggKey decides whether two records' Input may be added together.
//
// A record whose definition Belmont knows is summable with any record sharing
// that definition, whatever tool produced it. A record whose definition it does
// not know is summable only with the same tool's: two undeclared tools may well
// disagree, and assuming they agree is the assumption this whole mechanism
// exists to remove.
func inputAggKey(rec metricsRecord) string {
	switch rec.InputSemantics {
	case inputExcludesCacheRead, inputIncludesCacheRead:
		return string(rec.InputSemantics)
	default:
		return "unknown(" + rec.Tool + ")"
	}
}

// recordInputSemantics is the label for a record, treating a missing field (a
// record written before the field existed) as unknown rather than as agreement.
func recordInputSemantics(rec metricsRecord) inputSemantics {
	if rec.InputSemantics == "" {
		return inputSemanticsUnknown
	}
	return rec.InputSemantics
}

// inputAcc sums Input while tracking how many distinct definitions contributed.
// One definition is a total; two are not a bigger total, they are no total.
type inputAcc struct {
	total    int64
	critical int64
	keys     map[string]bool
}

func (a *inputAcc) add(key string, input int64, criticalPath bool) {
	if a.keys == nil {
		a.keys = map[string]bool{}
	}
	a.keys[key] = true
	a.total += input
	if criticalPath {
		a.critical += input
	}
}

// resolve returns the aggregate only when every contributing record shared one
// definition of Input, and a stated reason when it did not. A null with a reason
// is a usable measurement; a sum across two definitions is not.
func (a *inputAcc) resolve() (total, critical *int64, note string) {
	switch len(a.keys) {
	case 0:
		return nil, nil, "not aggregated: no phase reported an input token count"
	case 1:
		t, c := a.total, a.critical
		return &t, &c, ""
	default:
		keys := make([]string, 0, len(a.keys))
		for k := range a.keys {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return nil, nil, "not aggregated: phases span more than one definition of input_tokens (" +
			strings.Join(keys, ", ") + ") — see tools_detail for the per-tool figures"
	}
}

func summariseMetrics(feature string, recs []metricsRecord) metricsSummary {
	s := metricsSummary{Feature: feature, Phases: len(recs)}
	perRun := map[string]*runAgg{}
	perRunInput := map[string]*inputAcc{}
	perTool := map[string]*toolAgg{}
	var runOrder, toolOrder []string
	var totalInput inputAcc

	for _, r := range recs {
		s.WallMs += r.WallMs
		if r.CriticalPath {
			s.CriticalWall += r.WallMs
		}
		agg, ok := perRun[r.Run]
		if !ok {
			agg = &runAgg{Run: r.Run}
			perRun[r.Run] = agg
			perRunInput[r.Run] = &inputAcc{}
			runOrder = append(runOrder, r.Run)
		}
		agg.Phases++
		agg.WallMs += r.WallMs

		tagg, ok := perTool[r.Tool]
		if !ok {
			tagg = &toolAgg{Tool: r.Tool}
			perTool[r.Tool] = tagg
			toolOrder = append(toolOrder, r.Tool)
		}
		tagg.Phases++
		tagg.WallMs += r.WallMs

		if r.Input == nil && r.Output == nil {
			s.Unreported++
			agg.Unreported++
			tagg.Unreported++
			continue
		}
		if r.Input != nil {
			key := inputAggKey(r)
			totalInput.add(key, *r.Input, r.CriticalPath)
			perRunInput[r.Run].add(key, *r.Input, r.CriticalPath)
			tagg.Input += *r.Input
			if r.CriticalPath {
				tagg.CriticalInput += *r.Input
			}
			// Within a tool the definition is constant in practice; if a file
			// ever mixes them under one tool name, say so rather than pick one.
			if sem := recordInputSemantics(r); tagg.InputSemantics == "" {
				tagg.InputSemantics = sem
			} else if tagg.InputSemantics != sem {
				tagg.InputSemantics = inputSemanticsUnknown
			}
		}
		if r.Output != nil {
			s.Output += *r.Output
			agg.Output += *r.Output
			tagg.Output += *r.Output
		}
		if r.CacheCreation != nil {
			s.CacheCreation += *r.CacheCreation
			agg.CacheCreation += *r.CacheCreation
			tagg.CacheCreation += *r.CacheCreation
		}
		if r.CacheRead != nil {
			s.CacheRead += *r.CacheRead
			agg.CacheRead += *r.CacheRead
			tagg.CacheRead += *r.CacheRead
		}
	}

	s.Input, s.CriticalInput, s.InputNote = totalInput.resolve()

	sort.Strings(runOrder)
	for _, run := range runOrder {
		agg := perRun[run]
		agg.Input, _, agg.InputNote = perRunInput[run].resolve()
		s.ByRun = append(s.ByRun, *agg)
	}
	sort.Strings(toolOrder)
	for _, tool := range toolOrder {
		s.ByTool = append(s.ByTool, *perTool[tool])
	}
	s.Runs = len(runOrder)
	return s
}

// formatOptionalTokens prints a token count, or "n/a" where one could not be
// defined. It never prints 0 for an absent figure — zero is a real measurement
// in this file, and the two must stay distinguishable in the text output as
// well as in the JSON.
func formatOptionalTokens(v *int64) string {
	if v == nil {
		return "n/a"
	}
	return strconv.FormatInt(*v, 10)
}

func (s inputSemantics) label() string {
	if s == "" {
		return string(inputSemanticsUnknown)
	}
	return string(s)
}

func renderMetricsSummary(s metricsSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Metrics: %s\n", s.Feature)
	if s.Phases == 0 {
		b.WriteString("  No records yet. They are written by `belmont auto` / `belmont loop`.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  %d run(s), %d phase(s), %s wall-clock\n", s.Runs, s.Phases, formatDurationMs(s.WallMs))
	fmt.Fprintf(&b, "  tokens: %s in / %d out (cache %d created, %d read)\n",
		formatOptionalTokens(s.Input), s.Output, s.CacheCreation, s.CacheRead)
	if s.Input == nil && s.InputNote != "" {
		fmt.Fprintf(&b, "  input %s\n", s.InputNote)
	}
	fmt.Fprintf(&b, "  critical path: %s wall-clock, %s input tokens\n",
		formatDurationMs(s.CriticalWall), formatOptionalTokens(s.CriticalInput))
	if s.Unreported > 0 {
		fmt.Fprintf(&b, "  %d phase(s) reported no usage — wall-clock only, never estimated\n", s.Unreported)
	}
	if len(s.ByTool) > 1 {
		b.WriteString("\n  Tool      Input counts          Phases     Input   Output\n")
		for _, t := range s.ByTool {
			fmt.Fprintf(&b, "  %-8s  %-20s  %6d  %8d  %7d\n",
				t.Tool, t.InputSemantics.label(), t.Phases, t.Input, t.Output)
		}
	}
	b.WriteString("\n  Run                       Phases   Wall        Input    Output\n")
	for _, r := range s.ByRun {
		fmt.Fprintf(&b, "  %-24s  %6d   %-10s  %7s  %7d\n",
			r.Run, r.Phases, formatDurationMs(r.WallMs), formatOptionalTokens(r.Input), r.Output)
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
