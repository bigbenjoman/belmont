//go:build eval

// Belmont eval harness.
//
// Run:
//
//	go test -tags eval ./cmd/belmont              # Tier 1 only (offline, free)
//	BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont
//
// Two tiers, because one cannot do both jobs.
//
//	Tier 1 (offline) drives pure Go — state parsing, the deterministic decision
//	prefix, wave computation, the scope guard. It asserts the machinery, and it
//	is free. It says nothing about whether a change to *prose* is safe: a stub
//	agent never reads a SKILL.md.
//
//	Tier 2 (live) drives a real tool through executeLoopAction and asserts the
//	final PROGRESS state across N runs. It is the only tier that can license a
//	prose change, and it costs real tokens.
//
// Honest scope: much of Tier 1 overlaps cmd/belmont/scope_guard_test.go, which
// already covers the guards directly. Tier 1's value here is regression-locking
// a whole fixture through the real entry points, not novel coverage.
//
// Assertions are state transitions only — never prose. An eval that asserts on
// agent wording fails the first time a model is swapped, teaching everyone to
// ignore it.
//
// See knowledge/meta/evals.md.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// evalFixture names a directory under testdata/eval/ and the state the harness
// expects Belmont to compute from it.
//
// Fixtures are stored as inert content plus a commits.txt script, never as
// nested git repos: `git add -A` on a repo inside testdata/ stages a gitlink
// (mode 160000) and a fresh clone yields empty directories. materialiseFixture
// builds a real repo in t.TempDir() instead.
type evalFixture struct {
	Name string

	// Expected parse of PROGRESS.md.
	Milestones int
	TaskStates map[string]taskStatus // task ID -> expected state

	// Expected deterministic decision. DecidesTo is the action the rules
	// resolve to before any AI call; empty means the deterministic prefix
	// returns nil and the loop would fall through to decideLoopActionAI.
	DecidesTo   loopActionType
	DecidesOnMS string

	// Expected wave shape: one entry per wave, each listing milestone IDs.
	Waves [][]string

	// Live indicates the fixture is exercised by Tier 2. Only fixtures whose
	// transitions actually depend on agent output are worth the tokens.
	Live bool

	// LiveExpect maps a task ID to every state Tier 2 will accept after the
	// live phase. A list rather than one value, because some transitions have
	// more than one legitimate outcome and an over-tight assertion becomes a
	// flaky test everyone learns to re-run. Where only one state is acceptable
	// — failing-acceptance — the list has exactly one entry, deliberately.
	LiveExpect map[string][]taskStatus
}

func evalFixtures() []evalFixture {
	return []evalFixture{
		{
			Name:       "single-milestone-clean",
			Milestones: 1,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskTodo, "P1-M1-2": taskTodo, "P1-M1-3": taskTodo,
			},
			DecidesTo:   actionImplementMilestone,
			DecidesOnMS: "M1",
			Waves:       [][]string{{"M1"}},
			Live:        true,
			// Implement must leave every task done. [v] is also acceptable if
			// the agent verified inline; [ ] is not.
			LiveExpect: map[string][]taskStatus{
				"P1-M1-1": {taskDone, taskVerified},
				"P1-M1-2": {taskDone, taskVerified},
				"P1-M1-3": {taskDone, taskVerified},
			},
		},
		{
			Name:       "mid-milestone",
			Milestones: 1,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskDone, "P1-M1-2": taskTodo, "P1-M1-3": taskTodo,
			},
			DecidesTo:   actionImplementMilestone,
			DecidesOnMS: "M1",
			Waves:       [][]string{{"M1"}},
			Live:        true,
			LiveExpect: map[string][]taskStatus{
				"P1-M1-1": {taskDone, taskVerified},
				"P1-M1-2": {taskDone, taskVerified},
				"P1-M1-3": {taskDone, taskVerified},
			},
		},
		{
			// The pause path. A blocked task is a hard guardrail: it must
			// short-circuit before any rule or AI call, so no tokens are spent
			// on a feature that cannot proceed.
			Name:       "blocked-task",
			Milestones: 1,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskDone, "P1-M1-2": taskBlocked,
			},
			DecidesTo: actionPause,
			Waves:     [][]string{{"M1"}},
		},
		{
			// Wave ordering. M2 and M3 both depend on M1 and nothing else, so
			// wave 1 is [M1] alone and wave 2 is [M2, M3] concurrently.
			//
			// M1 is deliberately NOT pre-verified. An earlier version of this
			// fixture had it [v], which made the assertion untestable: with the
			// edge already satisfied, deleting dependency handling from
			// computeWaves entirely still produced the expected single wave.
			// Caught by running that exact control.
			Name:       "multi-milestone-deps",
			Milestones: 3,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskTodo, "P1-M2-1": taskTodo, "P1-M3-1": taskTodo,
			},
			DecidesTo:   actionImplementMilestone,
			DecidesOnMS: "M1",
			Waves:       [][]string{{"M1"}, {"M2", "M3"}},
		},
		{
			// The negative fixture. Without one the suite cannot fail
			// meaningfully: on a clean-pass milestone a rigorous verify and a
			// lazy one produce identical PROGRESS bytes.
			//
			// divide.js satisfies two of three acceptance criteria — divide(1,0)
			// returns Infinity where the PRD requires a thrown Error. The task
			// is [x]. A rigorous verify must refuse to flip it to [v].
			//
			// Tier 1 here pins a genuinely surprising fact rather than the
			// verify transition: milestoneAllDone treats [x] as done, so a
			// *fresh* loop over an all-[x] milestone resolves to COMPLETE and
			// computeWaves yields no waves — the milestone is never verified.
			// That is intended (`belmont reverify` is the documented path for
			// [x] -> [v]), and pinning it means any change to that contract
			// shows up here. Tier 2 therefore drives actionVerify directly
			// rather than routing through the decision engine.
			Name:       "failing-acceptance",
			Milestones: 1,
			TaskStates: map[string]taskStatus{"P1-M1-1": taskDone},
			DecidesTo:  actionComplete,
			Waves:      nil,
			Live:       true,
			// THE assertion that gives this suite teeth. divide(1,0) returns
			// Infinity where the PRD demands a thrown Error, so a rigorous
			// verify must leave P1-M1-1 at [x] and add a follow-up. A lazy
			// verify flips it to [v] and fails here.
			// Exactly one acceptable state. [v] here means the agent verified a
			// module that does not meet its own PRD.
			LiveExpect: map[string][]taskStatus{"P1-M1-1": {taskDone}},
		},
		{
			// The design-quality negative fixture, and the only one covering the
			// third design state: visible UI with no Figma anywhere. The other
			// five are either design-free or unconcerned with design.
			//
			// Its TECH_PLAN carries a Design Contract with
			// **Mode**: derived — UI, no Figma. badge.css satisfies every
			// acceptance criterion in PRD.md while violating that contract
			// (2.6:1 error text against a 4.5:1 floor; a semantic declaring a
			// background but no border or text colour). The separation is
			// deliberate: acceptance criteria alone cannot catch it, so only a
			// verify that actually runs the contract checks can fail this
			// milestone. That is what makes it a gate rather than a checkbox.
			//
			// Tier 1 pins the same surprising fact as failing-acceptance:
			// milestoneAllDone treats [x] as done, so a fresh loop over an
			// all-[x] milestone resolves to COMPLETE and yields no waves.
			// Tier 2 drives actionVerify directly.
			Name:       "ui-no-figma",
			Milestones: 1,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskDone, "P1-M1-2": taskDone,
			},
			DecidesTo: actionComplete,
			Waves:     nil,
			Live:      true,
			// Exactly one acceptable state per task. [v] here means the agent
			// verified a UI that violates the design contract its own feature
			// plan declares — the regression this fixture exists to catch.
			LiveExpect: map[string][]taskStatus{
				"P1-M1-1": {taskDone},
				"P1-M1-2": {taskDone},
			},
		},
		{
			// Two independent milestones. Used by the manual /belmont:loop
			// before/after comparison, because the loop must advance from one
			// milestone to the next rather than running a single phase.
			// Deliberately design-free — no Figma, no UI — so it exercises the
			// design-agent skip.
			Name:       "loop-two-milestones",
			Milestones: 2,
			TaskStates: map[string]taskStatus{
				"P1-M1-1": taskTodo, "P1-M2-1": taskTodo,
			},
			DecidesTo:   actionImplementMilestone,
			DecidesOnMS: "M1",
			Waves:       [][]string{{"M1", "M2"}},
		},
	}
}

// ---------------------------------------------------------------------------
// Materialisation
// ---------------------------------------------------------------------------

// materialiseFixture builds a real git repo in t.TempDir() from a fixture's
// inert content, and returns (root, slug).
func materialiseFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	src := filepath.Join("testdata", "eval", name)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}

	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "eval@belmont.test")
	runGit(t, root, "config", "user.name", "Belmont Eval")
	runGit(t, root, "config", "commit.gpgsign", "false")

	replayCommits(t, root, filepath.Join(src, "commits.txt"))

	// Feature state lands after the code history, so task-ID commits are
	// already reachable when runEvidenceCheck looks for them.
	base := filepath.Join(root, ".belmont", "features", name)
	for _, f := range []string{"PRD.md", "PROGRESS.md", "TECH_PLAN.md"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatalf("fixture %s: read %s: %v", name, f, err)
		}
		mustWrite(t, filepath.Join(base, f), string(data))
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-qm", "belmont: feature state for "+name)

	return root, name
}

// replayCommits executes a fixture's commits.txt.
//
// Format — directives are `== <verb>: <arg>` at the start of a line; every
// other line is content appended to the file most recently opened. `#` lines
// are comments only when no file is open, so file bodies keep their hashes.
//
//	# a comment
//	== file: divide.js
//	function divide(a, b) { return a / b }
//	== commit: P1-M1-1: add divide.js
func replayCommits(t *testing.T, root, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read commits.txt: %v", err)
	}

	var openFile string
	var body []string
	flush := func() {
		if openFile == "" {
			return
		}
		mustWrite(t, filepath.Join(root, openFile), strings.Join(body, "\n")+"\n")
		openFile, body = "", nil
	}

	for i, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "== file: "):
			flush()
			openFile = strings.TrimSpace(strings.TrimPrefix(line, "== file: "))
			if openFile == "" {
				t.Fatalf("%s:%d: empty file path", path, i+1)
			}
		case strings.HasPrefix(line, "== commit: "):
			flush()
			msg := strings.TrimSpace(strings.TrimPrefix(line, "== commit: "))
			if msg == "" {
				t.Fatalf("%s:%d: empty commit message", path, i+1)
			}
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-qm", msg)
		case strings.HasPrefix(line, "== "):
			t.Fatalf("%s:%d: unknown directive %q", path, i+1, line)
		case openFile == "" && (strings.HasPrefix(line, "#") || strings.TrimSpace(line) == ""):
			// comment or blank between blocks
		default:
			body = append(body, line)
		}
	}
	if openFile != "" {
		t.Fatalf("%s: file %q was never committed", path, openFile)
	}
}

// decideDeterministic mirrors runLoop's decision prefix (auto_loop.go) exactly:
// hard guardrails, then the smart rules. It deliberately stops before
// decideLoopActionAI — that path shells out to a model and is not assertable
// offline. Returning nil means "this fixture would cost an AI call".
func decideDeterministic(t *testing.T, root, slug string) *loopAction {
	t.Helper()
	report, err := buildStatus(root, 55, slug)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}
	cfg := loopConfig{Feature: slug, Root: root, MaxFailures: 3, MaxIterations: 50}

	var history []historyEntry
	hasFwlup := detectFwlupTasks(root, slug, report)
	hasMsFwlup := false
	pendingInRange := pendingTasksInRange(root, slug, cfg.From, cfg.To)
	fwlupInRange := fwlupTasksInRange(root, slug, report, cfg.From, cfg.To)
	msStates := buildMilestoneLoopStates(history, report.Milestones)

	if a := checkHardGuardrails(report, history, cfg); a != nil {
		return a
	}
	return decideLoopActionSmart(report, history, cfg, hasFwlup, hasMsFwlup, pendingInRange, fwlupInRange, msStates)
}

// ---------------------------------------------------------------------------
// Tier 1 — offline
// ---------------------------------------------------------------------------

// TestEvalTier1 runs every fixture through the real entry points and asserts
// the state Belmont computes. Free, deterministic, no agent involved.
func TestEvalTier1(t *testing.T) {
	for _, fx := range evalFixtures() {
		t.Run(fx.Name, func(t *testing.T) {
			root, slug := materialiseFixture(t, fx.Name)

			// Every fixture must satisfy `belmont validate`, because
			// `belmont auto` lints at startup — a fixture that fails it could
			// never run in practice.
			t.Run("validate", func(t *testing.T) {
				violations, err := validateFeature(root, slug)
				if err != nil {
					t.Fatalf("validateFeature: %v", err)
				}
				if len(violations) > 0 {
					for _, v := range violations {
						t.Errorf("validation violation [%s] %s: %s", v.Rule, v.Milestone, v.Message)
					}
				}
			})

			t.Run("parse", func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md"))
				if err != nil {
					t.Fatal(err)
				}
				ms := parseMilestones(string(data))
				if len(ms) != fx.Milestones {
					t.Errorf("milestones = %d, want %d", len(ms), fx.Milestones)
				}
				got := map[string]taskStatus{}
				for _, m := range ms {
					for _, task := range m.Tasks {
						got[task.ID] = task.Status
					}
				}
				if len(got) != len(fx.TaskStates) {
					t.Errorf("parsed %d tasks, want %d", len(got), len(fx.TaskStates))
				}
				for id, want := range fx.TaskStates {
					if got[id] != want {
						t.Errorf("task %s state = %v, want %v", id, got[id], want)
					}
				}
			})

			t.Run("decision", func(t *testing.T) {
				action := decideDeterministic(t, root, slug)
				if fx.DecidesTo == "" {
					if action != nil {
						t.Errorf("expected fall-through to AI, got deterministic %s", action.Type)
					}
					return
				}
				if action == nil {
					t.Fatalf("deterministic rules returned nil — this fixture now costs an AI call, "+
						"where it previously resolved to %s", fx.DecidesTo)
				}
				if action.Type != fx.DecidesTo {
					t.Errorf("action = %s (%s), want %s", action.Type, action.Reason, fx.DecidesTo)
				}
				if fx.DecidesOnMS != "" && action.MilestoneID != fx.DecidesOnMS {
					t.Errorf("action milestone = %q, want %q", action.MilestoneID, fx.DecidesOnMS)
				}
			})

			t.Run("waves", func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md"))
				if err != nil {
					t.Fatal(err)
				}
				waves, err := computeWaves(parseMilestones(string(data)))
				if err != nil {
					t.Fatalf("computeWaves: %v", err)
				}
				if len(waves) != len(fx.Waves) {
					t.Fatalf("waves = %d, want %d (%v)", len(waves), len(fx.Waves), describeWaves(waves))
				}
				for i, want := range fx.Waves {
					var got []string
					for _, m := range waves[i].Milestones {
						got = append(got, m.ID)
					}
					if strings.Join(got, ",") != strings.Join(want, ",") {
						t.Errorf("wave %d = %v, want %v", i+1, got, want)
					}
				}
			})
		})
	}
}

// TestEvalScopeGuardOnFixtures regression-locks the scope guard against a real
// fixture rather than a hand-built snapshot: an agent working M2 flips a task
// under M3, and the guard must put it back.
func TestEvalScopeGuardOnFixtures(t *testing.T) {
	root, slug := materialiseFixture(t, "multi-milestone-deps")
	progressPath := filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md")

	pre := snapshotProgress(root, slug)
	if pre == nil {
		t.Fatal("snapshotProgress returned nil")
	}

	// Simulate an M2 agent reaching outside its milestone.
	before, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	leaked := strings.Replace(string(before), "- [ ] P1-M3-1:", "- [v] P1-M3-1:", 1)
	if leaked == string(before) {
		t.Fatal("fixture no longer contains the M3 task this test leaks")
	}
	mustWrite(t, progressPath, leaked)

	cfg := loopConfig{Feature: slug, Root: root, MaxFailures: 3}
	runScopeGuard(cfg, loopAction{Type: actionImplementMilestone, MilestoneID: "M2"}, pre)

	after, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "- [v] P1-M3-1:") {
		t.Errorf("scope guard did not revert an out-of-scope flip:\n%s", after)
	}
}

// ---------------------------------------------------------------------------
// Tier 2 — live
// ---------------------------------------------------------------------------

// TestEvalLive drives a real AI tool through executeLoopAction and asserts the
// resulting PROGRESS state. This is the ONLY tier that can license a change to
// skill prose: Tier 1 never reads a SKILL.md.
//
// Gated twice — build tag plus BELMONT_EVAL_LIVE=1 — because it spends real
// tokens. Run it as:
//
//	BELMONT_EVAL_LIVE=1 go test -tags eval -timeout 0 -run TestEvalLive ./cmd/belmont
//
// -timeout 0 is REQUIRED. Go's default is a 10-minute budget for the whole test
// binary, and executeLoopAction has no watchdog of its own. On a timeout panic
// the deferred killProcessGroup never runs, so the live `claude -p` child is
// orphaned and keeps consuming budget after the test is gone.
//
// Cost, measured on the fixtures below: roughly 2–10 minutes and one full
// implement-or-verify phase per run, times evalLiveRuns, times the number of
// live fixtures. Budget for it before starting.
func TestEvalLive(t *testing.T) {
	if os.Getenv("BELMONT_EVAL_LIVE") != "1" {
		t.Skip("live evals are opt-in: set BELMONT_EVAL_LIVE=1 (and pass -timeout 0)")
	}
	tool := os.Getenv("BELMONT_EVAL_TOOL")
	if tool == "" {
		tool = "claude"
	}

	for _, fx := range evalFixtures() {
		if !fx.Live {
			continue
		}
		t.Run(fx.Name, func(t *testing.T) {
			// N >= 3. A single run cannot distinguish a real regression from
			// ordinary model variance.
			results := make([]map[string]taskStatus, 0, evalLiveRuns)
			for run := 0; run < evalLiveRuns; run++ {
				root, slug := materialiseLiveFixture(t, fx.Name)

				action := liveActionFor(fx)
				res := executeLoopAction(action, loopConfig{
					Feature:       slug,
					Root:          root,
					Tool:          tool,
					MaxFailures:   3,
					MaxIterations: 50,
					Policy:        policyAutonomous,
				})
				if !res.Success {
					t.Fatalf("run %d: %s failed: %s", run+1, action.Type, res.Error)
				}
				results = append(results, taskStatesOf(t, root, slug))
			}

			// Assert state transitions only — never prose.
			for i, got := range results {
				for id, accepted := range fx.LiveExpect {
					if !containsState(accepted, got[id]) {
						t.Errorf("run %d: task %s = %v, want one of %v", i+1, id, got[id], accepted)
					}
				}
			}
			// Flag nondeterminism explicitly rather than letting run 1 speak
			// for all of them.
			for i := 1; i < len(results); i++ {
				for id, v := range results[0] {
					if results[i][id] != v {
						t.Errorf("nondeterministic across runs: task %s was %v in run 1, %v in run %d",
							id, v, results[i][id], i+1)
					}
				}
			}
		})
	}
}

// evalLiveRuns is the N in "N >= 3 runs".
const evalLiveRuns = 3

// liveActionFor picks the phase each live fixture exercises.
//
// failing-acceptance and ui-no-figma are driven straight to VERIFY rather than
// through the decision engine: their milestones are all-[x], which
// milestoneAllDone treats as done, so the engine would return COMPLETE and no
// agent would ever run. See the fixture comments.
func liveActionFor(fx evalFixture) loopAction {
	switch fx.Name {
	case "failing-acceptance":
		return loopAction{Type: actionVerify, MilestoneID: "M1", Reason: "eval: verify a milestone with a known defect"}
	case "ui-no-figma":
		return loopAction{Type: actionVerify, MilestoneID: "M1", Reason: "eval: verify a milestone that violates its design contract"}
	}
	return loopAction{Type: actionImplementMilestone, MilestoneID: "M1", Reason: "eval: implement"}
}

// materialiseLiveFixture builds the fixture AND installs Belmont's skill
// surface into it. Without the install a bare t.TempDir() repo has no
// .agents/skills/, so a live agent could never read the prose this tier exists
// to evaluate — the test would pass while measuring nothing.
func materialiseLiveFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	root, slug := materialiseFixture(t, name)

	src, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve belmont source: %v", err)
	}
	if err := runInstall([]string{"--source", src, "--project", root, "--no-prompt"}); err != nil {
		t.Fatalf("install belmont into fixture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "belmont")); err != nil {
		t.Fatalf("fixture has no skill surface after install — a live agent would read no prose: %v", err)
	}

	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-qm", "belmont: install skill surface")
	return root, slug
}

func containsState(accepted []taskStatus, got taskStatus) bool {
	for _, a := range accepted {
		if a == got {
			return true
		}
	}
	return false
}

func taskStatesOf(t *testing.T, root, slug string) map[string]taskStatus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md"))
	if err != nil {
		t.Fatalf("read PROGRESS.md: %v", err)
	}
	out := map[string]taskStatus{}
	for _, m := range parseMilestones(string(data)) {
		for _, task := range m.Tasks {
			out[task.ID] = task.Status
		}
	}
	return out
}

func describeWaves(waves []wave) []string {
	var out []string
	for _, w := range waves {
		var ids []string
		for _, m := range w.Milestones {
			ids = append(ids, m.ID)
		}
		out = append(out, strings.Join(ids, "+"))
	}
	return out
}
