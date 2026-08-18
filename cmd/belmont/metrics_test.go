package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// These tests pin P0-2's instrumentation.
//
// The fixtures below are REAL tool output, captured on 2026-08-18 from claude
// 2.1.234 and codex, not invented JSON. That matters more than usual here: a
// usage parser written against a guessed schema fails silently — it returns
// always-zero or always-nil rather than erroring — and the M1 baseline every
// later milestone is judged against would be quietly wrong. If either tool
// changes its event shape, these fixtures are what fails.

// realClaudeResultEvent is the terminal `result` event of a live claude
// stream-json run, trimmed to the fields Belmont reads plus enough of the
// surrounding block to prove the parser ignores what it should.
const realClaudeResultEvent = `{"type":"result","subtype":"success","usage":{"input_tokens":2,"cache_creation_input_tokens":20998,"cache_read_input_tokens":15900,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":20998,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":15900,"cache_creation_input_tokens":20998,"type":"message"}],"speed":"standard"}}`

// realCodexTurnCompleted is a live codex `exec --json` terminal event.
const realCodexTurnCompleted = `{"type":"turn.completed","usage":{"input_tokens":17304,"cached_input_tokens":3456,"cache_write_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":13}}`

// TestClaudeStreamWriterCapturesResultUsage pins the defect P0-2 fixes on the
// Claude path: claudeStreamWriter discarded every line whose type was not
// "assistant", which is exactly where the only token count Claude emits lives.
func TestClaudeStreamWriterCapturesResultUsage(t *testing.T) {
	tw := newTailWriter(io.Discard, 1500, "")
	csw := &claudeStreamWriter{tw: tw}

	stream := `{"type":"system","subtype":"init","session_id":"x"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"text","text":"OK"}]}}` + "\n" +
		realClaudeResultEvent + "\n"

	if _, err := csw.Write([]byte(stream)); err != nil {
		t.Fatalf("write: %v", err)
	}

	usage := csw.Usage()
	if usage == nil {
		t.Fatal("no usage captured from the result event — the regression P0-2 fixed")
	}
	if usage.Input != 2 || usage.Output != 4 {
		t.Errorf("input/output: got %d/%d, want 2/4", usage.Input, usage.Output)
	}
	if usage.CacheCreation != 20998 {
		t.Errorf("cache_creation: got %d, want 20998", usage.CacheCreation)
	}
	if usage.CacheRead != 15900 {
		t.Errorf("cache_read: got %d, want 15900", usage.CacheRead)
	}
}

// TestClaudeStreamWriterReportsNoUsageWhenStreamHasNoResult confirms an aborted
// run yields nil rather than zeros. Zeros would be recorded as a real
// measurement of "this cost nothing", which is a lie; nil is recorded as null.
func TestClaudeStreamWriterReportsNoUsageWhenStreamHasNoResult(t *testing.T) {
	tw := newTailWriter(io.Discard, 1500, "")
	csw := &claudeStreamWriter{tw: tw}
	csw.Write([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}` + "\n"))
	if csw.Usage() != nil {
		t.Error("expected nil usage for a stream with no result event")
	}
}

// TestCodexUsageFromLine pins the field mapping, which differs from Claude's.
// codex calls the read `cached_input_tokens` and the creation
// `cache_write_input_tokens`; swapping them yields a plausible number that is
// wrong, which is the failure mode this whole file exists to prevent.
func TestCodexUsageFromLine(t *testing.T) {
	usage := codexUsageFromLine([]byte(realCodexTurnCompleted))
	if usage == nil {
		t.Fatal("no usage parsed from a real codex turn.completed event")
	}
	if usage.Input != 17304 || usage.Output != 20 {
		t.Errorf("input/output: got %d/%d, want 17304/20", usage.Input, usage.Output)
	}
	if usage.CacheRead != 3456 {
		t.Errorf("cache_read: got %d, want 3456 (cached_input_tokens)", usage.CacheRead)
	}
	if usage.CacheCreation != 0 {
		t.Errorf("cache_creation: got %d, want 0 (cache_write_input_tokens)", usage.CacheCreation)
	}

	for _, line := range []string{
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"text":"usage"}}`,
		`not json at all`,
	} {
		if got := codexUsageFromLine([]byte(line)); got != nil {
			t.Errorf("expected nil for %q, got %+v", line, got)
		}
	}
}

// TestUsageCaptureSurvivesMoreThanTheTailWindow is the point of usageCapture.
//
// Non-Claude tools write into a tailWriter that keeps only the last 1500 bytes,
// so whether a tool's usage event lands inside that window is luck: codex emits
// turn.completed last, but the item.completed events before it grow with the
// agent's output. Here the usage line is followed by far more than 1500 bytes of
// trailing output, so a tail-based reader would miss it entirely.
func TestUsageCaptureSurvivesMoreThanTheTailWindow(t *testing.T) {
	tw := newTailWriter(io.Discard, 1500, "")
	uc := newUsageCapture(tw, codexUsageFromLine)

	uc.Write([]byte(strings.Repeat(`{"type":"item.completed","item":{"text":"noise"}}`+"\n", 40)))
	uc.Write([]byte(realCodexTurnCompleted + "\n"))
	// Everything after the usage line pushes it out of any fixed-size tail.
	uc.Write([]byte(strings.Repeat(`{"type":"item.completed","item":{"text":"trailing"}}`+"\n", 200)))

	usage := uc.Usage()
	if usage == nil {
		t.Fatal("usage lost behind trailing output — this is the tailWriter blocker P0-2 had to settle")
	}
	if usage.Input != 17304 {
		t.Errorf("input: got %d, want 17304", usage.Input)
	}

	// The tail is unchanged in purpose: it still holds the end of the stream for
	// error reporting, and is still capped.
	if len(tw.String()) > 1500 {
		t.Errorf("tail grew beyond its cap: %d bytes", len(tw.String()))
	}
}

// TestUsageCaptureWritesEverythingThrough confirms the capture is a tee, not a
// filter — it must not swallow output the human is watching.
func TestUsageCaptureWritesEverythingThrough(t *testing.T) {
	var sink strings.Builder
	uc := newUsageCapture(&sink, codexUsageFromLine)
	payload := "line one\nline two\nline three\n"
	if n, err := uc.Write([]byte(payload)); err != nil || n != len(payload) {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	if sink.String() != payload {
		t.Errorf("passthrough: got %q, want %q", sink.String(), payload)
	}
}

// TestBuildMetricsRecordNeverEstimates pins the governing rule: a tool that
// cannot report usage produces null fields and a stated reason, never a number.
func TestBuildMetricsRecordNeverEstimates(t *testing.T) {
	for _, tool := range []string{"copilot", "pi", "opencode", "gemini", "cursor"} {
		rec := buildMetricsRecord("run1", "feat", "M1", "VERIFY", tool, 1234, nil, false)
		if rec.Input != nil || rec.Output != nil || rec.CacheCreation != nil || rec.CacheRead != nil {
			t.Errorf("%s: token fields must be nil when the tool reports nothing", tool)
		}
		if rec.Note == "" {
			t.Errorf("%s: a null with no stated reason is indistinguishable from a bug", tool)
		}
		if rec.WallMs != 1234 {
			t.Errorf("%s: wall-clock must still be recorded, got %d", tool, rec.WallMs)
		}

		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(data), `"input":null`) {
			t.Errorf("%s: expected an explicit null in the JSON, got %s", tool, data)
		}
	}
}

// TestBuildMetricsRecordZeroIsNotNull pins the pointer choice. Some hosts
// genuinely return zero, and a real zero must not be reported as "unavailable".
func TestBuildMetricsRecordZeroIsNotNull(t *testing.T) {
	rec := buildMetricsRecord("run1", "feat", "M1", "IMPLEMENT_MILESTONE", "claude", 10,
		&toolUsage{Input: 0, Output: 0, CacheCreation: 0, CacheRead: 0}, true)
	if rec.Input == nil || *rec.Input != 0 {
		t.Error("a reported zero must survive as 0, not become null")
	}
	if rec.Note != "" {
		t.Errorf("a reported measurement needs no note, got %q", rec.Note)
	}
	data, _ := json.Marshal(rec)
	if strings.Contains(string(data), `"input":null`) {
		t.Errorf("reported zero serialised as null: %s", data)
	}
}

// TestCriticalPathIsNotTheWaveIndex is the correctness point of the
// critical-path attribution.
//
// computeWaves is a Kahn topological levelling: M4 below sits in the same wave
// as M2 because both depend only on M1. But the longest chain is M1→M2→M3, and
// M4 lies on no longest chain at all. Latency correlates with critical-path
// tokens at r = 0.901, so marking M4 as critical-path spend — which reusing the
// wave index would do — would misattribute it.
func TestCriticalPathIsNotTheWaveIndex(t *testing.T) {
	milestones := []milestone{
		{ID: "M1"},
		{ID: "M2", Deps: []string{"M1"}},
		{ID: "M3", Deps: []string{"M2"}},
		{ID: "M4", Deps: []string{"M1"}},
	}
	onPath := criticalPathMilestones(milestones)

	for _, id := range []string{"M1", "M2", "M3"} {
		if !onPath[id] {
			t.Errorf("%s should be on the critical path", id)
		}
	}
	if onPath["M4"] {
		t.Error("M4 is in wave 2 but on no longest chain — marking it critical is the wave-index confusion")
	}

	// Sanity: the wave computation genuinely does put M2 and M4 together, which
	// is what makes this test worth having.
	waves, err := computeWaves(milestones)
	if err != nil {
		t.Fatalf("computeWaves: %v", err)
	}
	var m2Wave, m4Wave = -1, -1
	for _, w := range waves {
		for _, m := range w.Milestones {
			if m.ID == "M2" {
				m2Wave = w.Index
			}
			if m.ID == "M4" {
				m4Wave = w.Index
			}
		}
	}
	if m2Wave != m4Wave {
		t.Fatalf("fixture no longer demonstrates the confusion: M2 wave %d, M4 wave %d", m2Wave, m4Wave)
	}
}

// TestCriticalPathMarksEveryLongestChain — a milestone can lie on more than one
// longest chain, and all of them are critical.
func TestCriticalPathMarksEveryLongestChain(t *testing.T) {
	milestones := []milestone{
		{ID: "M1"},
		{ID: "M2", Deps: []string{"M1"}},
		{ID: "M3", Deps: []string{"M1"}},
		{ID: "M4", Deps: []string{"M2", "M3"}},
	}
	onPath := criticalPathMilestones(milestones)
	for _, id := range []string{"M1", "M2", "M3", "M4"} {
		if !onPath[id] {
			t.Errorf("%s lies on a longest chain and should be marked", id)
		}
	}
}

// TestCriticalPathToleratesCyclesAndDanglingDeps — this function must never be
// the thing that hangs or crashes a run. A cycle is computeWaves's error to
// report, and a dangling dependency is P2-4's.
func TestCriticalPathToleratesCyclesAndDanglingDeps(t *testing.T) {
	cyclic := []milestone{
		{ID: "M1", Deps: []string{"M2"}},
		{ID: "M2", Deps: []string{"M1"}},
	}
	if got := criticalPathMilestones(cyclic); len(got) == 0 {
		t.Error("expected a best-effort answer rather than an empty one")
	}

	dangling := []milestone{
		{ID: "M1", Deps: []string{"M99"}},
		{ID: "M2", Deps: []string{"M1"}},
	}
	onPath := criticalPathMilestones(dangling)
	if !onPath["M2"] || !onPath["M1"] {
		t.Error("a dangling dep should contribute nothing, not break attribution")
	}
}

// TestMetricsRoundTripAcrossRuns is P0-2's acceptance criterion: two consecutive
// runs on the same feature produce comparable records.
func TestMetricsRoundTripAcrossRuns(t *testing.T) {
	root := t.TempDir()

	write := func(run, milestone, phase string, wall int64, usage *toolUsage) {
		t.Helper()
		rec := buildMetricsRecord(run, "throughput", milestone, phase, "claude", wall, usage, milestone == "M1")
		if err := appendMetricsRecord(root, rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	write("2026-08-18T09:00:00Z", "M1", "IMPLEMENT_MILESTONE", 60000, &toolUsage{Input: 100, Output: 50, CacheCreation: 10, CacheRead: 5})
	write("2026-08-18T09:00:00Z", "M1", "VERIFY", 30000, &toolUsage{Input: 40, Output: 10})
	write("2026-08-18T10:00:00Z", "M1", "IMPLEMENT_MILESTONE", 45000, &toolUsage{Input: 70, Output: 30})
	write("2026-08-18T10:00:00Z", "M2", "VERIFY", 15000, nil)

	recs, err := readMetricsRecords(root, "throughput")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(recs) != 4 {
		t.Fatalf("records: got %d, want 4", len(recs))
	}

	s := summariseMetrics("throughput", recs)
	if s.Runs != 2 {
		t.Errorf("runs: got %d, want 2", s.Runs)
	}
	if s.Input != 210 {
		t.Errorf("input: got %d, want 210", s.Input)
	}
	if s.WallMs != 150000 {
		t.Errorf("wall: got %d, want 150000", s.WallMs)
	}
	if s.Unreported != 1 {
		t.Errorf("unreported phases: got %d, want 1", s.Unreported)
	}
	// M2 is not critical-path in this fixture, so its wall-clock is excluded.
	if s.CriticalWall != 135000 {
		t.Errorf("critical wall: got %d, want 135000", s.CriticalWall)
	}
	if len(s.ByRun) != 2 || s.ByRun[0].Phases != 2 || s.ByRun[1].Phases != 2 {
		t.Errorf("per-run split wrong: %+v", s.ByRun)
	}

	// The two runs must be comparable — same feature, same phases, different totals.
	if s.ByRun[0].Input == s.ByRun[1].Input {
		t.Error("fixture should produce differing per-run totals")
	}
}

// TestReadMetricsRecordsSkipsTruncatedLine — the file is append-only and local,
// so a run interrupted mid-write must not make the whole history unreadable.
func TestReadMetricsRecordsSkipsTruncatedLine(t *testing.T) {
	root := t.TempDir()
	path := metricsPath(root, "feat")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	good := `{"run":"r1","feature":"feat","phase":"VERIFY","tool":"claude","wall_ms":10,"input":null,"output":null,"cache_creation":null,"cache_read":null,"critical_path":false}`
	if err := os.WriteFile(path, []byte(good+"\n"+`{"run":"r2","feat`), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := readMetricsRecords(root, "feat")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected the good line to survive the truncated one, got %d", len(recs))
	}
}

// TestMetricsPathIsUnderGitignoredDirectory pins the storage decision: metrics
// are local-only, alongside auto.json, because bookkeeping is already 41-46% of
// commit volume in these repos.
func TestMetricsPathIsUnderGitignoredDirectory(t *testing.T) {
	got := metricsPath("/proj", "throughput")
	want := filepath.Join("/proj", ".belmont", "metrics", "throughput.jsonl")
	if got != want {
		t.Errorf("path: got %s, want %s", got, want)
	}
}

// TestToolReportsUsageMatchesTheNoteTable — every tool Belmont supports either
// has a verified extractor or an explicit reason it does not. A tool in neither
// set would silently record a bare null.
func TestToolReportsUsageMatchesTheNoteTable(t *testing.T) {
	for _, tool := range []string{"claude", "codex", "gemini", "copilot", "cursor", "pi", "opencode"} {
		_, hasNote := usageUnavailableNote[tool]
		if toolReportsUsage(tool) == hasNote {
			t.Errorf("%s: must either report usage or carry a stated reason it cannot — never both or neither", tool)
		}
	}
}

func TestFormatDurationMs(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{500, "500ms"},
		{1500, "1s"},
		{65000, "1m05s"},
		{3725000, "1h02m"},
	}
	for _, c := range cases {
		if got := formatDurationMs(c.ms); got != c.want {
			t.Errorf("formatDurationMs(%d): got %s, want %s", c.ms, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-M1-FIX-1 — metrics must outlive the worktree
// ---------------------------------------------------------------------------
//
// The defect: recordPhaseMetrics wrote to cfg.Root, and both worktree sites in
// auto_parallel.go set Root to the worktree path before calling runLoop. Since
// .belmont/metrics/ is gitignored, syncFeatureStateAfterMerge copies back only
// PROGRESS.md, and removeWorktree then deletes the tree, every record produced
// in parallel-wave mode was destroyed unread — and autocmd.go routes to
// runAutoParallel whenever any milestone declares `(depends: …)`, which is the
// mode this feature actually runs in. Nothing errored; the file simply was not
// there afterwards.
//
// These tests exercise the worktree path specifically, which is the prevention
// rule NOTES.md §Root Cause Patterns records for exactly this class of bug.

// TestWorktreeLoopConfigKeepsMetricsOnTheOriginatingRoot pins the derivation the
// two auto_parallel.go sites use: Root moves to the worktree, MetricsRoot and
// RunID do not.
func TestWorktreeLoopConfigKeepsMetricsOnTheOriginatingRoot(t *testing.T) {
	mainRoot := t.TempDir()
	wtPath := filepath.Join(t.TempDir(), "belmont-worktrees", "M3")

	parent := loopConfig{Root: mainRoot, Feature: "throughput", Tool: "claude", RunID: "2026-08-18T09:00:00Z"}
	child := worktreeLoopConfig(parent, wtPath)

	if child.Root != wtPath {
		t.Fatalf("Root: got %s, want the worktree %s", child.Root, wtPath)
	}
	if child.MetricsRoot != mainRoot {
		t.Errorf("MetricsRoot: got %s, want the originating root %s", child.MetricsRoot, mainRoot)
	}
	if child.metricsRoot() == child.Root {
		t.Error("metrics still resolve under the worktree — this is the defect P0-M1-FIX-1 fixes")
	}
	if child.RunID != parent.RunID {
		t.Errorf("RunID: got %q, want the parent's %q — a wave must record as one run", child.RunID, parent.RunID)
	}
}

// TestRecordPhaseMetricsWritesUnderMainRootNotWorktree is the end-of-fix
// assertion: with the worktree config, the record lands under the main root and
// the worktree gets no .belmont/metrics/ at all — so deleting the worktree loses
// nothing.
func TestRecordPhaseMetricsWritesUnderMainRootNotWorktree(t *testing.T) {
	mainRoot := t.TempDir()
	wtPath := t.TempDir()

	parent := loopConfig{
		Root:         mainRoot,
		Feature:      "throughput",
		Tool:         "claude",
		RunID:        "2026-08-18T09:00:00Z",
		CriticalPath: map[string]bool{"M3": true},
	}
	cfg := worktreeLoopConfig(parent, wtPath)

	recordPhaseMetrics(cfg, "M3", "IMPLEMENT_MILESTONE", 60000,
		&toolUsage{Input: 100, Output: 50})

	recs, err := readMetricsRecords(mainRoot, "throughput")
	if err != nil {
		t.Fatalf("read from main root: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("main root records: got %d, want 1 — the wave's measurement is the deliverable", len(recs))
	}
	if recs[0].Run != parent.RunID || recs[0].Milestone != "M3" || !recs[0].CriticalPath {
		t.Errorf("record content wrong: %+v", recs[0])
	}

	// The worktree must hold nothing. If it does, that copy dies with the tree.
	if _, err := os.Stat(filepath.Join(wtPath, ".belmont", "metrics")); !os.IsNotExist(err) {
		t.Errorf("worktree has a metrics dir (err=%v) — it would be deleted with the worktree", err)
	}
}

// TestWaveOfMilestonesRecordsAsOneRun pins RunID propagation. runLoop mints an
// ID from its own start time when it finds none, which is right for a serial run
// and wrong for a wave: `belmont metrics` aggregates per run, so N worktrees each
// minting their own would report one invocation as N runs and make the P3-3
// comparison against the M1 baseline meaningless.
func TestWaveOfMilestonesRecordsAsOneRun(t *testing.T) {
	mainRoot := t.TempDir()
	parent := loopConfig{Root: mainRoot, Feature: "throughput", Tool: "claude", RunID: "2026-08-18T09:00:00Z"}

	for _, ms := range []string{"M2", "M3", "M4"} {
		cfg := worktreeLoopConfig(parent, filepath.Join(t.TempDir(), ms))
		cfg.From, cfg.To = ms, ms
		recordPhaseMetrics(cfg, ms, "IMPLEMENT_MILESTONE", 60000, &toolUsage{Input: 100, Output: 50})
	}

	recs, err := readMetricsRecords(mainRoot, "throughput")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := summariseMetrics("throughput", recs)
	if s.Phases != 3 {
		t.Fatalf("phases: got %d, want 3", s.Phases)
	}
	if s.Runs != 1 {
		t.Errorf("runs: got %d, want 1 — one wave is one run, not one run per worktree", s.Runs)
	}
	if s.Input != 300 {
		t.Errorf("input: got %d, want 300", s.Input)
	}
}

// TestRecordPhaseMetricsFallsBackToRoot keeps the serial path honest: an unset
// MetricsRoot must behave exactly as before this change, so no construction site
// has to remember the new field.
func TestRecordPhaseMetricsFallsBackToRoot(t *testing.T) {
	root := t.TempDir()
	cfg := loopConfig{Root: root, Feature: "throughput", Tool: "claude", RunID: "2026-08-18T09:00:00Z"}

	if cfg.metricsRoot() != root {
		t.Fatalf("metricsRoot fallback: got %s, want %s", cfg.metricsRoot(), root)
	}
	recordPhaseMetrics(cfg, "M1", "VERIFY", 1000, nil)

	recs, err := readMetricsRecords(root, "throughput")
	if err != nil || len(recs) != 1 {
		t.Fatalf("serial path regressed: %d record(s), err=%v", len(recs), err)
	}
}

// TestAppendMetricsRecordIsConcurrencySafe covers what the fix newly exposes:
// several worktrees in one wave now append to ONE file under the main root, so
// the append path is shared across goroutines (and across any other belmont
// process on the same checkout).
//
// It is safe because O_APPEND makes the seek-to-EOF and the write one operation,
// AND because appendMetricsRecord emits the record and its newline in a single
// f.Write. The second half is the fragile one, so the test carries its own
// negative control: writing the payload and the '\n' as two calls, which is the
// most plausible way for a later edit to break this. Measured 2026-08-18 on
// macOS/APFS, 16 goroutines × 400 records: the single-write path produced 6400
// intact lines with zero unparseable, the split-write control lost ~47% of its
// lines and left ~1850 unparseable, on every run. A concurrency test with no
// failing control proves nothing.
func TestAppendMetricsRecordIsConcurrencySafe(t *testing.T) {
	const goroutines, perGoroutine = 16, 400

	countLines := func(t *testing.T, path string) (total, unparseable int) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			total++
			var rec metricsRecord
			if json.Unmarshal([]byte(line), &rec) != nil {
				unparseable++
			}
		}
		return total, unparseable
	}

	hammer := func(t *testing.T, root string, write func(root string, rec metricsRecord) error) (total, unparseable int) {
		t.Helper()
		var wg sync.WaitGroup
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < perGoroutine; i++ {
					rec := buildMetricsRecord("2026-08-18T09:00:00Z", "throughput",
						fmt.Sprintf("M%d", g), fmt.Sprintf("PHASE_%d", i), "claude",
						int64(i), &toolUsage{Input: 100, Output: 50}, g == 0)
					// A note long enough that a spliced line is unmistakable,
					// while staying far below any size a local filesystem would
					// split one write() at.
					rec.Note = strings.Repeat("x", 200)
					if err := write(root, rec); err != nil {
						t.Errorf("append: %v", err)
						return
					}
				}
			}(g)
		}
		wg.Wait()
		return countLines(t, metricsPath(root, "throughput"))
	}

	root := t.TempDir()
	total, unparseable := hammer(t, root, appendMetricsRecord)
	if want := goroutines * perGoroutine; total != want {
		t.Errorf("lines: got %d, want %d — concurrent appends lost or spliced records", total, want)
	}
	if unparseable != 0 {
		t.Errorf("unparseable lines: got %d, want 0 — the shared append path is garbling records", unparseable)
	}

	// Negative control. Skipped when the scheduler cannot interleave the two
	// writes, because then it would pass for the wrong reason.
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("negative control needs GOMAXPROCS >= 2 to interleave")
	}
	ncRoot := t.TempDir()
	_, ncUnparseable := hammer(t, ncRoot, appendSplitWriteForTest)
	if ncUnparseable == 0 {
		t.Error("negative control produced no damage — this test cannot detect a split write, so it proves nothing about the real one")
	}
}

// appendSplitWriteForTest is appendMetricsRecord with the one property under
// test removed: the record and its newline go out as two writes. Test-only, and
// it exists solely to be the control that fails.
func appendSplitWriteForTest(root string, rec metricsRecord) error {
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
	if _, err := f.Write(data); err != nil {
		return err
	}
	_, err = f.Write([]byte("\n"))
	return err
}
