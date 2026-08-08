package main

import "fmt"

type taskStatus string

const (
	taskTodo       taskStatus = "todo"
	taskInProgress taskStatus = "in_progress"
	taskDone       taskStatus = "done"
	taskVerified   taskStatus = "verified"
	taskBlocked    taskStatus = "blocked"

	// taskUnknown is a checkbox whose marker Belmont does not recognise.
	//
	// It exists because the alternative — silently defaulting to taskTodo — is
	// the one outcome that loses information without telling anyone. An
	// unparsed entry was being counted in the totals, rendered as `[ ]`, and
	// handed to an agent as "Next Individual Task": work the tool could not
	// read was being scheduled for implementation. See issue #27.
	//
	// An unknown task is never offered as next work, never counted as todo,
	// and never lets a milestone read as complete. `belmont validate` exits 1
	// on it, and `resolveProgressConflict` refuses to auto-resolve a file
	// containing one so the conflict escalates rather than being normalised.
	//
	// The validate gate is narrower than it looks and must not be described as
	// an unconditional stop: `belmont auto` lints only on the single-feature
	// path, and only aborts when stdin is not a TTY — interactively it offers
	// "Proceed anyway? [y/N]". `--features`/`--all` skip the lint entirely.
	taskUnknown taskStatus = "unknown"
)

type task struct {
	ID          string
	Name        string
	Status      taskStatus
	MilestoneID string // which milestone this task belongs to (from PROGRESS.md)

	// Marker is the raw character between the brackets and Line is its 1-based
	// line number, kept so diagnostics can quote what was actually written and
	// point at it. Both are excluded from JSON: they are populated for every
	// task, and `status --format json` is read by agents on every loop
	// iteration, so serialising them would add per-task noise to a payload
	// PR #26 exists to shrink. The `unknown` entry in TaskCounts is the JSON
	// signal; the detail lives in the text output and in `belmont validate`.
	Marker string `json:"-"`
	Line   int    `json:"-"`
}

type milestone struct {
	ID       string
	Name     string
	Tasks    []task   // tasks in this milestone (from PROGRESS.md)
	Deps     []string // e.g. ["M1", "M3"] — nil for no explicit deps
	LiveFrom string   `json:"live_from,omitempty"` // worktree path when state was read from an active worktree (buildStatus only)
}

type featureSummary struct {
	Slug            string      `json:"slug"`
	Name            string      `json:"name"`
	TasksDone       int         `json:"tasks_done"`
	TasksVerified   int         `json:"tasks_verified"`
	TasksInProgress int         `json:"tasks_in_progress"`
	TasksBlocked    int         `json:"tasks_blocked"`
	TasksTotal      int         `json:"tasks_total"`
	TasksOrphaned   int         `json:"tasks_orphaned,omitempty"`
	MilestonesDone  int         `json:"milestones_done"`
	MilestonesTotal int         `json:"milestones_total"`
	Milestones      []milestone `json:"milestones"`
	NextMilestone   *milestone  `json:"next_milestone,omitempty"`
	NextTask        *task       `json:"next_task,omitempty"`
	Status          string      `json:"status"`
	Deps            []string    `json:"deps,omitempty"`
	Priority        string      `json:"priority,omitempty"`
}

type statusReport struct {
	Feature string
	// FeatureSlug is the on-disk directory name, as opposed to Feature which
	// is the human-readable name from PRD.md. Kept so diagnostics can print a
	// command the user can actually paste — `belmont reverify --feature` takes
	// the slug. Empty in listing mode, where featureSummary carries its own.
	FeatureSlug string `json:",omitempty"`
	// Orphans are task-shaped lines outside any milestone — invisible to every
	// count. Carried on the report because renderStatus has no raw content and
	// by the time parseMilestones has run they are already gone. See issue #31.
	Orphans          []task `json:",omitempty"`
	TechPlanReady    bool
	PRFAQReady       bool
	OverallStatus    string
	TaskCounts       map[string]int
	Tasks            []task
	Milestones       []milestone
	NextMilestone    *milestone
	NextTask         *task
	LastCompleted    *task
	RecentDecisions  []string
	Features         []featureSummary
	ArchivedFeatures []featureSummary `json:",omitempty"`
	Monorepo         *monorepoReport  `json:",omitempty"`
}

// monorepoReport summarizes detected monorepo workspaces for status output.
// Nil when the project is single-package.
type monorepoReport struct {
	Type       string              `json:"type"`
	Primary    string              `json:"primary,omitempty"`
	Workspaces []monorepoWorkspace `json:"workspaces"`
}

type monorepoWorkspace struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type config struct {
	Source string `json:"source"`
}

// Loop types
type loopActionType string

const (
	actionImplementMilestone loopActionType = "IMPLEMENT_MILESTONE"
	actionImplementNext      loopActionType = "IMPLEMENT_NEXT"
	actionVerify             loopActionType = "VERIFY"
	actionPause              loopActionType = "PAUSE"
	actionComplete           loopActionType = "COMPLETE"
	actionError              loopActionType = "ERROR"
	actionReplan             loopActionType = "REPLAN"
	actionSkipMilestone      loopActionType = "SKIP_MILESTONE"
	actionDebug              loopActionType = "DEBUG"
	actionTriage             loopActionType = "TRIAGE"
	actionFixAll             loopActionType = "FIX_ALL"
)

var errFeaturePaused = fmt.Errorf("feature paused")

// errWorktreeDirty signals that a rebase-on-resume was skipped because the
// worktree has uncommitted user/agent changes. Callers should warn but proceed.
var errWorktreeDirty = fmt.Errorf("worktree has uncommitted changes")

type loopAction struct {
	Type           loopActionType
	Reason         string
	MilestoneID    string
	TriageDecision string // "fix_and_reverify", "fix_and_proceed", "defer_and_proceed" — set after triage
	ReverifyScope  string // "full" or "focused" — set by triage
}

type executionResult struct {
	Success    bool
	Output     string
	Error      string
	DurationMs int64
}

type workType string

const (
	workFrontend workType = "frontend" // .tsx, .jsx, .css, .scss, .html, .vue, .svelte
	workBackend  workType = "backend"  // .go, .py, .rs, .java, etc.
	workConfig   workType = "config"   // .yml, .yaml, .json, .toml, CI files
	workDocs     workType = "docs"     // .md, .txt
	workMixed    workType = "mixed"
	workMinimal  workType = "minimal" // < 3 files changed
	workUnknown  workType = "unknown"
)

type historyEntry struct {
	Action       loopAction
	Result       *executionResult
	TasksDone    int
	TasksTotal   int
	MsDone       int
	MsTotal      int
	BlockerCount int
	HasFwlup     bool
	Iteration    int
	WorkType     workType
	FilesChanged int
	GitSHA       string
	PostGitSHA   string
}

type milestoneLoopState struct {
	ID              string
	Name            string
	Done            bool
	Implemented     bool
	Verified        bool
	VerifyFailed    int
	VerifySucceeded int // how many times verification passed for this milestone
	WorkType        workType
	FilesChanged    int
	FwlupFixRounds  int // how many triage+fix cycles have run for this milestone
}

type checkpointPolicy string

const (
	policyAutonomous  checkpointPolicy = "autonomous"
	policyMilestone   checkpointPolicy = "milestone"
	policyEveryAction checkpointPolicy = "every_action"
)

type loopConfig struct {
	Feature          string
	Root             string
	Tool             string
	From             string
	To               string
	Policy           checkpointPolicy
	MaxIterations    int
	MaxFailures      int
	MaxParallel      int
	DryRun           bool
	Port             int               // assigned port for worktree isolation (0 = not in worktree)
	WorktreeEnv      map[string]string // extra env vars from worktree.json
	Tracker          *worktreeTracker  // process group tracker for cleanup on interrupt
	TrackerID        string            // worktree ID for tracker operations
	ModelTiers       modelTierConfig   // per-feature model tiers (from .belmont/features/<slug>/models.yaml)
	Workspaces       []workspaceInfo   // monorepo workspaces (nil for single-package projects)
	PrimaryWorkspace string            // primary workspace ID (empty for single-package)
	MonorepoType     monorepoType      // detected/declared monorepo type (empty for single-package)
}

type aiDecision struct {
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	MilestoneID string `json:"milestone_id,omitempty"`
}

type reconciliationFile struct {
	Strategy        string `json:"strategy"`
	PostResolveCmd  string `json:"post_resolve_command"`
	File            string `json:"file"`
	Confidence      string `json:"confidence"`
	Reason          string `json:"reason"`
	ConflictSummary string `json:"conflict_summary"`
	ResolvedContent string `json:"resolved_content"`
}

type reconciliationReport struct {
	Files []reconciliationFile `json:"files"`
}

// featureWave represents a group of features that can execute in parallel.
type featureWave struct {
	Index    int
	Features []featureSummary
}

// readinessWarning describes a requested feature whose declared dep is not
// yet terminal at start-of-run. Caller emits these as yellow warning lines
// so the operator can Ctrl-C before the scheduler launches dependents that
// will likely cascade-skip if the dep pauses again.
type readinessWarning struct {
	Slug      string
	DepSlug   string
	DepStatus string
	Blocked   int // count of [!] tasks in the dep's PROGRESS.md
}

// skipResult records a feature skipped from a wave because one of its
// declared dependencies is in a non-runnable state. `Reason` is "paused" or
// "failed"; `DepSlug` is the first matching dep in declaration order.
type skipResult struct {
	Slug    string
	DepSlug string
	Reason  string // "paused" | "failed"
}

type triageDecision struct {
	Decision      string   `json:"decision"`
	BlockingTasks []string `json:"blocking_tasks"`
	DeferredTasks []string `json:"deferred_tasks"`
	Reason        string   `json:"reason"`
	ReverifyScope string   `json:"reverify_scope"`
}
