// Belmont CLI.
//
//go:generate bash scripts/generate-skills.sh

package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

// Model tier registry: maps (tool, tier) to the CLI --model identifier.
// Tiers (low/medium/high) are stable across releases; model IDs get bumped
// here as tools ship new versions. This is the single source of truth —
// skill bodies reference the same mapping via _partials/tier-registry.md.
var modelTiers = map[string]map[string]string{
	"claude": {
		"low":    "haiku",
		"medium": "sonnet",
		"high":   "opus",
	},
	"codex": {
		"low":    "gpt-5.4-mini",
		"medium": "gpt-5.4",
		"high":   "gpt-5.5",
	},
	"gemini": {
		"low":    "gemini-2.5-flash-lite",
		"medium": "gemini-2.5-flash",
		"high":   "gemini-2.5-pro",
	},
	"cursor": {
		"low":    "sonnet-4",
		"medium": "sonnet-4-thinking",
		"high":   "gpt-5",
	},
	"copilot": {
		"low":    "haiku-4.5",
		"medium": "claude-sonnet-4.5",
		"high":   "gpt-5.4",
	},
	// opencode model IDs are `provider/model` pairs. The defaults below assume
	// the Anthropic provider (opencode's most common frontier setup); users on
	// another provider (opencode zen, OpenAI, local models, …) override per
	// tier via `opencode.tiers.<tier>.model` in local-llms.json or
	// BELMONT_OPENCODE_MODEL_<TIER> env vars — see resolveOpencodeModelFlags.
	"opencode": {
		"low":    "anthropic/claude-haiku-4-5",
		"medium": "anthropic/claude-sonnet-4-6",
		"high":   "anthropic/claude-opus-4-8",
	},
}

// toolSupportsModel indicates whether the tool's CLI accepts --model at all.
//
// Pi is `true` so the model-flag dispatch fires, but Pi is deliberately absent
// from `modelTiers` — Pi runs against user-provided local models whose IDs
// Belmont cannot know in advance. resolveModelFlags special-cases Pi to read
// from `~/.belmont/local-llms.json` (with project + env-var overrides). When
// nothing in that chain produces a value Belmont passes no flags and Pi falls
// back to the default model in its own `~/.pi/agent/models.json`.
var toolSupportsModel = map[string]bool{
	"claude":   true,
	"codex":    true,
	"gemini":   true,
	"cursor":   true,
	"copilot":  true,
	"pi":       true,
	"opencode": true,
}

// planningTier is always used for product-plan and tech-plan invocations.
// Planning produces the spec downstream agents execute against, so it
// always runs at the highest-capability tier regardless of per-feature
// config. Editing this is a deliberate, global decision.
const planningTier = "high"

// reconciliationDefaultTier is used when no models.yaml is present.
const reconciliationDefaultTier = "high"

// modelTierConfig holds the parsed contents of .belmont/features/<slug>/models.yaml.
// Empty value is safe to pass everywhere — callers get nil tier strings and fall
// back to agent-frontmatter defaults.
type modelTierConfig struct {
	Profile  string
	Planning string
	Tiers    map[string]string // agent name (e.g. "implementation") -> "low"|"medium"|"high"
}

// splitYAMLKV splits "key: value" into trimmed parts with quotes stripped.
func splitYAMLKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	v = strings.Trim(v, `"'`)
	return k, v, true
}

// actionAgent maps a loop action type to the Belmont agent name that runs
// the heaviest work for that action. Used for tier lookup. Empty string
// means "no agent mapping" (tier falls through to empty → default model).
func actionAgent(t loopActionType) string {
	switch t {
	case actionImplementMilestone, actionImplementNext, actionFixAll:
		return "implementation"
	case actionVerify:
		return "verification"
	case actionTriage:
		return "verification" // triage reads verification output; share its tier
	case actionReplan:
		return "" // planning uses planningTier, handled separately
	default:
		return ""
	}
}

type taskStatus string

const (
	taskTodo       taskStatus = "todo"
	taskInProgress taskStatus = "in_progress"
	taskDone       taskStatus = "done"
	taskVerified   taskStatus = "verified"
	taskBlocked    taskStatus = "blocked"
)

type task struct {
	ID          string
	Name        string
	Status      taskStatus
	MilestoneID string // which milestone this task belongs to (from PROGRESS.md)
}

type milestone struct {
	ID       string
	Name     string
	Tasks    []task   // tasks in this milestone (from PROGRESS.md)
	Deps     []string // e.g. ["M1", "M3"] — nil for no explicit deps
	LiveFrom string   `json:"live_from,omitempty"` // worktree path when state was read from an active worktree (buildStatus only)
}

// Milestone computed state helpers

type featureSummary struct {
	Slug            string      `json:"slug"`
	Name            string      `json:"name"`
	TasksDone       int         `json:"tasks_done"`
	TasksVerified   int         `json:"tasks_verified"`
	TasksInProgress int         `json:"tasks_in_progress"`
	TasksBlocked    int         `json:"tasks_blocked"`
	TasksTotal      int         `json:"tasks_total"`
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
	Feature          string
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

// worktreeHooks defines lifecycle hooks for worktree isolation.
//
// PrimaryWorkspace and Workspaces are optional monorepo overrides. When
// Workspaces is non-empty it replaces auto-detection; when PrimaryWorkspace
// is empty the first workspace with a `dev` script (or first detected) wins.
// All four new fields are optional — existing single-package worktree.json
// files parse and behave identically.
type worktreeHooks struct {
	Setup            []string                     `json:"setup"`
	Teardown         []string                     `json:"teardown"`
	Env              map[string]string            `json:"env"`
	PrimaryWorkspace string                       `json:"primary_workspace,omitempty"`
	Workspaces       map[string]workspaceOverride `json:"workspaces,omitempty"`
}

// workspaceOverride is a user-supplied workspace declaration in worktree.json.
// Path is relative to the project root. EnvFiles lists additional env file
// paths (also relative to project root) to seed into the workspace dir on
// top of the root .env propagation.
type workspaceOverride struct {
	Path     string   `json:"path"`
	EnvFiles []string `json:"env_files,omitempty"`
}

// loadWorktreeHooks reads .belmont/worktree.json from the project root.
// Returns nil if the file does not exist.
func loadWorktreeHooks(root string) *worktreeHooks {
	data, err := os.ReadFile(filepath.Join(root, ".belmont", "worktree.json"))
	if err != nil {
		return nil
	}
	var hooks worktreeHooks
	if err := json.Unmarshal(data, &hooks); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to parse .belmont/worktree.json: %s\033[0m\n", err)
		return nil
	}
	return &hooks
}

// worktreeBasePath returns the directory for worktrees, stored in the user's home directory.
// This avoids nesting worktrees inside the project where tools like Turbopack detect
// multiple lockfiles and infer the wrong workspace root.
// For /home/user/code/myapp -> ~/.belmont/worktrees/myapp/
func worktreeBasePath(root string) string {
	absRoot, _ := filepath.Abs(root)
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback: use a sibling directory if home is unavailable
		parent := filepath.Dir(absRoot)
		name := filepath.Base(absRoot)
		return filepath.Join(parent, ".belmont-worktrees", name)
	}
	name := filepath.Base(absRoot)
	return filepath.Join(home, ".belmont", "worktrees", name)
}

// allocatePort asks the OS for a free TCP port by binding to :0.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// detectAutoInstallCommands checks the project root for known lock files and
// returns the appropriate dependency install command(s). Returns nil if no
// recognized lock file is found. First match wins.
func detectAutoInstallCommands(root string) []string {
	type entry struct {
		File     string
		Commands []string
	}
	lockfiles := []entry{
		{"pnpm-lock.yaml", []string{"pnpm install --prefer-offline"}},
		{"bun.lockb", []string{"bun install"}},
		{"bun.lock", []string{"bun install"}},
		{"yarn.lock", []string{"yarn install --prefer-offline"}},
		{"package-lock.json", []string{"npm install --prefer-offline"}},
		{"Gemfile.lock", []string{"bundle install"}},
		{"requirements.txt", []string{"pip install -r requirements.txt"}},
		{"Cargo.lock", []string{"cargo build"}},
	}
	for _, lf := range lockfiles {
		if _, err := os.Stat(filepath.Join(root, lf.File)); err == nil {
			return lf.Commands
		}
	}
	return nil
}

// warnIfNotGitignored prints a one-line warning if the seeded path is not
// matched by the worktree's .gitignore rules. Best-effort; failures are
// silent (the warning is a friendly diagnostic, not a blocker).
func warnIfNotGitignored(wtPath, relPath string) {
	cmd := exec.Command("git", "check-ignore", "-q", relPath)
	cmd.Dir = wtPath
	if err := cmd.Run(); err != nil {
		// Exit code 1 == not ignored. Other errors (no git, no .gitignore) are
		// treated the same way for the purpose of warning.
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Seeded %s but it is not gitignored — make sure your .gitignore covers nested .env files.\033[0m\n", relPath)
	}
}

// monorepoType identifies the dominant monorepo system in use.
type monorepoType string

const (
	monorepoNone      monorepoType = ""
	monorepoTurborepo monorepoType = "turborepo"
	monorepoNx        monorepoType = "nx"
	monorepoPnpm      monorepoType = "pnpm"
	monorepoNpm       monorepoType = "npm"
	monorepoYarn      monorepoType = "yarn"
	monorepoBun       monorepoType = "bun"
	monorepoLerna     monorepoType = "lerna"
	monorepoRush      monorepoType = "rush"
	monorepoCargo     monorepoType = "cargo"
	monorepoGo        monorepoType = "go"
	monorepoUv        monorepoType = "uv"
	monorepoPoetry    monorepoType = "poetry"
)

// envSignals indicates whether a workspace consumes env at install/build time.
type envSignals struct {
	Postinstall   bool // package.json has scripts.postinstall (any postinstall:* variant)
	PrismaDep     bool // deps include prisma / @prisma/client
	DotenvDep     bool // deps include dotenv / dotenv-cli / drizzle-kit / tsx / vite-node
	BuildRs       bool // Rust workspace has build.rs
	PythonScripts bool // pyproject has [project.scripts] or [tool.poetry.scripts]
}

func (s envSignals) consumesEnv() bool {
	return s.Postinstall || s.PrismaDep || s.DotenvDep || s.BuildRs || s.PythonScripts
}

// workspaceInfo describes a discovered workspace.
type workspaceInfo struct {
	ID       string // package name (or directory base if not parseable)
	Path     string // relative path from project root
	Manifest string // absolute path to manifest file (may be empty for synthetic entries)
	Signals  envSignals
	HasDev   bool // package.json has scripts.dev / Cargo bin target / etc. (used for primary selection)
}

// walkForManifests scans subdirectories of root/base looking for the named
// manifest file. depth=-1 means recurse without bound; depth=1 means only
// immediate children. node_modules and .git directories are skipped.
func walkForManifests(root, base, manifestName string, depth int) []string {
	startDir := filepath.Join(root, base)
	st, err := os.Stat(startDir)
	if err != nil || !st.IsDir() {
		return nil
	}
	var out []string
	var walk func(rel string, remaining int)
	walk = func(rel string, remaining int) {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".") {
				continue
			}
			child := filepath.Join(rel, name)
			if fileExists(filepath.Join(root, child, manifestName)) {
				out = append(out, child)
				continue
			}
			if remaining == 0 {
				continue
			}
			next := remaining
			if remaining > 0 {
				next = remaining - 1
			}
			walk(child, next)
		}
	}
	walk(base, depth)
	return out
}

// jsManifestName extracts the `name` field from a package.json file.
// Returns "" if the manifest is missing, malformed, or has no name.
func jsManifestName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}

// jsManifestSignals reads a package.json and returns env-consumption signals
// plus whether the manifest has a `dev` script (used for primary-workspace
// selection).
func jsManifestSignals(path string) (envSignals, bool) {
	var sig envSignals
	data, err := os.ReadFile(path)
	if err != nil {
		return sig, false
	}
	var pkg struct {
		Scripts          map[string]string `json:"scripts"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return sig, false
	}
	hasDev := false
	for k := range pkg.Scripts {
		if k == "dev" {
			hasDev = true
		}
		if k == "postinstall" || strings.HasPrefix(k, "postinstall:") {
			sig.Postinstall = true
		}
	}
	merged := map[string]struct{}{}
	for k := range pkg.Dependencies {
		merged[k] = struct{}{}
	}
	for k := range pkg.DevDependencies {
		merged[k] = struct{}{}
	}
	for k := range pkg.PeerDependencies {
		merged[k] = struct{}{}
	}
	for _, prismaDep := range []string{"prisma", "@prisma/client"} {
		if _, ok := merged[prismaDep]; ok {
			sig.PrismaDep = true
		}
	}
	for _, dotenvDep := range []string{"dotenv", "dotenv-cli", "drizzle-kit", "tsx", "vite-node"} {
		if _, ok := merged[dotenvDep]; ok {
			sig.DotenvDep = true
		}
	}
	return sig, hasDev
}

// cargoCrateName extracts `name` from a Cargo.toml [package] section.
func cargoCrateName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	pkgIdx := strings.Index(text, "[package]")
	if pkgIdx < 0 {
		return ""
	}
	rest := text[pkgIdx:]
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			return v
		}
		if strings.HasPrefix(line, "[") && line != "[package]" {
			break
		}
	}
	return ""
}

// sliceUntilNextHeader returns text up to the next "[section]" header (or
// end of string). Used to scope TOML section parsing.
func sliceUntilNextHeader(text string) string {
	for i := 1; i < len(text); i++ {
		if text[i] == '[' && text[i-1] == '\n' {
			return text[:i]
		}
	}
	return text
}

// extractTomlArray pulls a `key = ["a", "b"]` array out of a TOML section.
func extractTomlArray(section, key string) []string {
	idx := strings.Index(section, key)
	if idx < 0 {
		return nil
	}
	openB := strings.Index(section[idx:], "[")
	if openB < 0 {
		return nil
	}
	openB += idx
	closeB := strings.Index(section[openB:], "]")
	if closeB < 0 {
		return nil
	}
	closeB += openB
	body := section[openB+1 : closeB]
	var out []string
	for _, item := range strings.Split(body, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// pythonProjectName reads a pyproject.toml and extracts [project] name.
func pythonProjectName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	pIdx := strings.Index(text, "[project]")
	if pIdx < 0 {
		return ""
	}
	section := sliceUntilNextHeader(text[pIdx:])
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			return v
		}
	}
	return ""
}

// pythonManifestSignals flags whether the workspace declares scripts (which
// usually means it has a runnable entrypoint that may consume env).
func pythonManifestSignals(path string) envSignals {
	var sig envSignals
	data, err := os.ReadFile(path)
	if err != nil {
		return sig
	}
	text := string(data)
	if strings.Contains(text, "[project.scripts]") || strings.Contains(text, "[tool.poetry.scripts]") {
		sig.PythonScripts = true
	}
	return sig
}

// guessManifest returns the absolute path to the workspace's manifest file
// (package.json / Cargo.toml / pyproject.toml / go.mod), or "" if none found.
func guessManifest(absDir string) string {
	for _, name := range []string{"package.json", "Cargo.toml", "pyproject.toml", "go.mod"} {
		p := filepath.Join(absDir, name)
		if fileExists(p) {
			return p
		}
	}
	return ""
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

func main() {
	// Clean up old binary on Windows after self-update
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			old := exe + ".old"
			if _, err := os.Stat(old); err == nil {
				os.Remove(old)
			}
		}
	}

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "status":
		must(runStatus(os.Args[2:]))
	case "auto", "loop":
		must(runAutoCmd(os.Args[2:]))
	case "install":
		must(runInstall(os.Args[2:]))
	case "update":
		must(runUpdate(os.Args[2:]))
	case "recover":
		must(runRecover(os.Args[2:]))
	case "steer":
		must(runSteerCmd(os.Args[2:]))
	case "validate":
		must(runValidateCmd(os.Args[2:]))
	case "reverify":
		must(runReverifyCmd(os.Args[2:]))
	case "sync":
		must(runSyncCmd(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("belmont %s (%s, %s)\n", Version, CommitSHA, BuildDate)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Belmont Helper")
	fmt.Fprintln(w, "==============")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  belmont install [--source PATH] [--project PATH] [--tools all|none|claude,codex,...]")
	fmt.Fprintln(w, "  belmont update [--check] [--force] [--no-commit]")
	fmt.Fprintln(w, "  belmont status [--root PATH] [--feature SLUG] [--format text|json] [--color auto|always|never]")
	fmt.Fprintln(w, "  belmont auto --feature SLUG [--from M1] [--to M5] [--tool claude|codex|gemini|copilot|cursor|pi|opencode] [--policy autonomous|milestone|every_action] [--max-iterations N] [--max-parallel N] [--allow-dirty] [--root PATH]")
	fmt.Fprintln(w, "    (alias: belmont loop)")
	fmt.Fprintln(w, "  belmont reverify [--feature SLUG] [--from M1] [--to M5] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont sync [--root PATH]")
	fmt.Fprintln(w, "  belmont recover [--list] [--merge SLUG] [--clean SLUG] [--clean-all] [--tool claude|codex|gemini|copilot|cursor|pi|opencode] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont steer [--feature SLUG] [--milestone M5] [--message \"text\" | --file PATH | -] [--root PATH]")
	fmt.Fprintln(w, "  belmont validate [--feature SLUG] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont version")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStatus(args []string) error {
	fsFlags := flag.NewFlagSet("status", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var format string
	var maxName int
	var feature string
	var colorMode string
	var showArchived bool
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.IntVar(&maxName, "max-task-name", 55, "max task name length")
	fsFlags.StringVar(&feature, "feature", "", "feature slug")
	fsFlags.StringVar(&colorMode, "color", "auto", "auto, always, or never")
	fsFlags.BoolVar(&showArchived, "show-archived", false, "include archived features in the listing (text mode)")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("status: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	report, err := buildStatus(absRoot, maxName, feature)
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
			return fmt.Errorf("status: %w", err)
		}
		fmt.Print(renderStatus(report, useColor, showArchived))
		return nil
	default:
		return fmt.Errorf("status: unknown format %q", format)
	}
}

// shouldColor resolves whether ANSI colors should be emitted.
//
//	mode "never"  → false
//	mode "always" → true (regardless of stdout type or NO_COLOR)
//	mode "auto"   → honor NO_COLOR, then fall back to isTerminal(f)
//
// Follows the NO_COLOR convention (https://no-color.org): any non-empty value
// disables colors under "auto" mode only. "always" is the explicit opt-in for
// piped contexts (e.g., when Claude Code runs the CLI as a sub-process but
// renders ANSI in its Bash tool output block).
func shouldColor(mode string, f *os.File) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		if os.Getenv("NO_COLOR") != "" {
			return false, nil
		}
		return isTerminal(f), nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("invalid --color value %q (want auto, always, or never)", mode)
	}
}

func buildStatus(root string, maxName int, feature string) (statusReport, error) {
	var report statusReport
	report.TaskCounts = map[string]int{
		"todo":        0,
		"in_progress": 0,
		"done":        0,
		"verified":    0,
		"blocked":     0,
		"total":       0,
	}

	// Detect monorepo workspaces. Honor explicit overrides in worktree.json
	// over auto-detection. Returns nil for single-package projects.
	if ws, primary, mType := resolveWorkspaces(root, loadWorktreeHooks(root)); mType != monorepoNone && len(ws) > 0 {
		entries := make([]monorepoWorkspace, 0, len(ws))
		for _, w := range ws {
			entries = append(entries, monorepoWorkspace{ID: w.ID, Path: w.Path})
		}
		report.Monorepo = &monorepoReport{
			Type:       string(mType),
			Primary:    primary,
			Workspaces: entries,
		}
	}

	// Check for PR_FAQ
	prfaqPath := filepath.Join(root, ".belmont", "PR_FAQ.md")
	report.PRFAQReady = fileHasRealContent(prfaqPath)

	// Determine base path based on feature mode
	featuresDir := filepath.Join(root, ".belmont", "features")

	// Load worktree overrides so we can read live state from active worktrees
	worktreeOverrides := loadAutoWorktrees(root)

	if feature != "" {
		// Specific feature requested
		featurePath := filepath.Join(featuresDir, feature)
		// If there's an active worktree for this feature (serial mode or
		// multi-feature mode), read state from there instead of the master
		// copy. In single-feature parallel mode we fall through to the
		// per-milestone merge below — master is still the baseline and we
		// overlay each milestone's live state from its own worktree.
		liveFeature, perMilestoneLive := loadAutoWorktreeStateByMilestone(root)
		if perMilestoneLive == nil {
			if override, ok := worktreeOverrides[feature]; ok {
				featurePath = override
			}
		}
		if !dirExists(featurePath) {
			return report, fmt.Errorf("status: feature %q not found in %s", feature, featuresDir)
		}

		prdPath := filepath.Join(featurePath, "PRD.md")
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		techPlanPath := filepath.Join(featurePath, "TECH_PLAN.md")

		prdContent, err := os.ReadFile(prdPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", prdPath)
		}

		progressContent, err := os.ReadFile(progressPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", progressPath)
		}

		report.Feature = extractFeatureName(string(prdContent))
		report.Milestones = parseMilestones(string(progressContent))

		// Single-feature parallel mode: overlay each active worktree's view
		// of its own milestone on top of master's baseline. Only the
		// worktree's owning milestone is overlaid; other milestones stay at
		// master's (possibly stale) state. Each overlaid milestone carries a
		// LiveFrom pointer so renderers can annotate it.
		if perMilestoneLive != nil && liveFeature == feature {
			report.Milestones = overlayLiveMilestones(report.Milestones, perMilestoneLive)
		}

		report.Tasks = flattenTasks(report.Milestones, maxName)

		report.TaskCounts["total"] = len(report.Tasks)
		for _, t := range report.Tasks {
			switch t.Status {
			case taskDone:
				report.TaskCounts["done"]++
			case taskVerified:
				report.TaskCounts["verified"]++
			case taskBlocked:
				report.TaskCounts["blocked"]++
			case taskInProgress:
				report.TaskCounts["in_progress"]++
			case taskTodo:
				report.TaskCounts["todo"]++
			}
		}

		report.LastCompleted = lastCompletedTask(report.Tasks)
		report.RecentDecisions = parseDecisions(string(progressContent), 3)
		report.NextMilestone = nextMilestone(report.Milestones)
		report.NextTask = nextTask(report.Tasks)
		report.TechPlanReady = techPlanReady(techPlanPath)
		report.OverallStatus = computeOverallStatus(report.Tasks)

		return report, nil
	}

	// Feature listing mode (default)
	features := listFeaturesWithOverrides(featuresDir, maxName, worktreeOverrides)
	if features == nil {
		features = []featureSummary{}
	}
	populateFeatureDeps(features, root)

	// Split archived features into their own slice so consumers (JSON + text
	// renderer) can treat them separately without re-filtering. Overall status
	// is still computed across the full set — archiving a finished feature
	// shouldn't regress the project status.
	active := make([]featureSummary, 0, len(features))
	var archived []featureSummary
	for _, f := range features {
		if f.Status == "Archived" {
			archived = append(archived, f)
		} else {
			active = append(active, f)
		}
	}
	report.Features = active
	report.ArchivedFeatures = archived
	report.Feature = extractProductName(filepath.Join(root, ".belmont", "PRD.md"))
	report.TechPlanReady = techPlanReady(filepath.Join(root, ".belmont", "TECH_PLAN.md"))

	if len(features) > 0 {
		report.OverallStatus = computeFeatureListStatus(features)
	} else {
		report.OverallStatus = "Not Started"
	}

	return report, nil
}

func extractProductName(prdPath string) string {
	content, err := os.ReadFile(prdPath)
	if err != nil {
		return "Unnamed Product"
	}
	re := regexp.MustCompile(`(?m)^#\s*Product:\s*(.+)$`)
	match := re.FindStringSubmatch(string(content))
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return extractFeatureName(string(content))
}

// loadAutoWorktrees reads .belmont/auto.json and returns a map of feature slug → worktree feature path.
// If auto.json doesn't exist or isn't active, returns nil. In multi-feature
// mode the map key is the feature slug (one worktree per feature). In single-
// feature parallel mode there may be multiple worktrees for one feature; this
// function picks a representative (alphabetically first milestone's worktree)
// so legacy callers that look up "is there a worktree for this feature" still
// get a sensible answer. New callers that need per-milestone live state
// should use loadAutoWorktreeStateByMilestone.
func loadAutoWorktrees(root string) map[string]string {
	aj := readActiveAutoJSONOrNil(root)
	if aj == nil {
		return nil
	}
	result := make(map[string]string)
	if aj.Mode == "single-feature-parallel" && aj.Feature != "" {
		// All entries are milestones under this one feature. Pick the
		// alphabetically first milestone as the representative path.
		var keys []string
		for id := range aj.Worktrees {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			wtFeaturePath := filepath.Join(aj.Worktrees[id].Path, ".belmont", "features", aj.Feature)
			if dirExists(wtFeaturePath) {
				result[aj.Feature] = wtFeaturePath
				break
			}
		}
		return result
	}
	// Multi-feature (or legacy) mode: keys are feature slugs.
	for slug, entry := range aj.Worktrees {
		wtFeaturePath := filepath.Join(entry.Path, ".belmont", "features", slug)
		if dirExists(wtFeaturePath) {
			result[slug] = wtFeaturePath
		}
	}
	return result
}

// loadAutoWorktreeStateByMilestone returns the active feature slug and a map
// of milestone ID → worktree feature directory for single-feature parallel
// runs. Returns ("", nil) in serial or multi-feature modes (neither has
// per-milestone worktrees). Only entries whose worktree directory still
// exists on disk are included.
func loadAutoWorktreeStateByMilestone(root string) (string, map[string]string) {
	aj := readActiveAutoJSONOrNil(root)
	if aj == nil {
		return "", nil
	}
	if aj.Mode != "single-feature-parallel" || aj.Feature == "" {
		return "", nil
	}
	perMS := map[string]string{}
	for msID, entry := range aj.Worktrees {
		wtFeaturePath := filepath.Join(entry.Path, ".belmont", "features", aj.Feature)
		if dirExists(wtFeaturePath) {
			perMS[msID] = wtFeaturePath
		}
	}
	if len(perMS) == 0 {
		return "", nil
	}
	return aj.Feature, perMS
}

// overlayLiveMilestones returns `base` with each milestone whose ID matches an
// entry in `perMilestoneLive` replaced by that worktree's current view of the
// milestone. Milestones with no active worktree are returned unchanged.
// Overlaid milestones carry a LiveFrom pointer so renderers can annotate them.
func overlayLiveMilestones(base []milestone, perMilestoneLive map[string]string) []milestone {
	out := make([]milestone, 0, len(base))
	for _, m := range base {
		live, ok := perMilestoneLive[m.ID]
		if !ok {
			out = append(out, m)
			continue
		}
		wtProgressPath := filepath.Join(live, "PROGRESS.md")
		data, err := os.ReadFile(wtProgressPath)
		if err != nil {
			out = append(out, m) // worktree lost its PROGRESS.md — fall back to master
			continue
		}
		wtMilestones := parseMilestones(string(data))
		var replaced bool
		for _, wm := range wtMilestones {
			if wm.ID == m.ID {
				wm.LiveFrom = live
				out = append(out, wm)
				replaced = true
				break
			}
		}
		if !replaced {
			// Worktree doesn't have this milestone (shouldn't happen) — keep master.
			out = append(out, m)
		}
	}
	return out
}

// readActiveAutoJSONOrNil is a shared helper for the readers above.
func readActiveAutoJSONOrNil(root string) *autoJSON {
	autoPath := filepath.Join(root, ".belmont", "auto.json")
	data, err := os.ReadFile(autoPath)
	if err != nil {
		return nil
	}
	var aj autoJSON
	if err := json.Unmarshal(data, &aj); err != nil || !aj.Active {
		return nil
	}
	return &aj
}

func listFeatures(featuresDir string, maxName int) []featureSummary {
	return listFeaturesWithOverrides(featuresDir, maxName, nil)
}

// listFeaturesWithOverrides is like listFeatures but allows worktree path overrides.
// The overrides map slug → worktree feature path for features with active worktrees.
func listFeaturesWithOverrides(featuresDir string, maxName int, worktreeOverrides map[string]string) []featureSummary {
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil
	}
	var features []featureSummary
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		slug := entry.Name()
		featurePath := filepath.Join(featuresDir, slug)
		// If there's an active worktree for this feature, read state from there instead
		if override, ok := worktreeOverrides[slug]; ok {
			featurePath = override
		}
		prdPath := filepath.Join(featurePath, "PRD.md")
		archivePath := filepath.Join(featurePath, "ARCHIVE.md")

		name := slug
		if prdContent, err := os.ReadFile(prdPath); err == nil {
			extracted := extractFeatureName(string(prdContent))
			if extracted != "Unknown" {
				name = extracted
			}
		} else if archiveContent, err := os.ReadFile(archivePath); err == nil {
			// Archived feature: PRD.md is gone; recover the name from the
			// "# Archive: <name>" header written by /belmont:cleanup.
			if extracted := extractArchiveName(string(archiveContent)); extracted != "" {
				name = extracted
			}
		}

		// Read all state from PROGRESS.md
		var milestones []milestone
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		if progressContent, err := os.ReadFile(progressPath); err == nil {
			milestones = parseMilestones(string(progressContent))
		}

		tasks := flattenTasks(milestones, maxName)
		tasksTotal := len(tasks)
		tasksDone := 0
		tasksVerified := 0
		tasksInProgress := 0
		tasksBlocked := 0
		for _, t := range tasks {
			switch t.Status {
			case taskDone:
				tasksDone++
			case taskVerified:
				tasksVerified++
			case taskInProgress:
				tasksInProgress++
			case taskBlocked:
				tasksBlocked++
			}
		}

		milestonesDone := 0
		for _, m := range milestones {
			if milestoneAllDone(m) {
				milestonesDone++
			}
		}

		featureNextMilestone := nextMilestone(milestones)
		featureNextTask := nextTask(tasks)

		status := computeOverallStatus(tasks)

		// Features archived via /belmont:cleanup have their planning files
		// replaced with a single ARCHIVE.md summary. Without this check the
		// missing PROGRESS.md would make them look like "Not Started" and they
		// would leak into `belmont auto --all`.
		if fileExists(filepath.Join(featurePath, "ARCHIVE.md")) {
			status = "Archived"
		}

		features = append(features, featureSummary{
			Slug:            slug,
			Name:            name,
			TasksDone:       tasksDone + tasksVerified,
			TasksVerified:   tasksVerified,
			TasksInProgress: tasksInProgress,
			TasksBlocked:    tasksBlocked,
			TasksTotal:      tasksTotal,
			MilestonesDone:  milestonesDone,
			MilestonesTotal: len(milestones),
			Milestones:      milestones,
			NextMilestone:   featureNextMilestone,
			NextTask:        featureNextTask,
			Status:          status,
		})
	}
	return features
}

// parseMasterTableColumns finds column indices by header name in the master PROGRESS.md features table.
func parseMasterTableColumns(lines []string) map[string]int {
	result := map[string]int{
		"Feature": -1, "Slug": -1, "Priority": -1, "Dependencies": -1,
		"Status": -1, "Milestones": -1, "Tasks": -1,
	}
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "|") {
			cells := splitTableCells(trimmed)
			for i, c := range cells {
				c = strings.TrimSpace(c)
				if _, ok := result[c]; ok {
					result[c] = i
				}
			}
			return result
		}
	}
	// Fallback: old 6-column format or new 7-column format by position
	return result
}

// splitTableCells splits a markdown table row into cells (stripping leading/trailing pipes).
func splitTableCells(line string) []string {
	cols := strings.Split(line, "|")
	var cells []string
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c != "" {
			cells = append(cells, c)
		}
	}
	return cells
}

// syncMasterFeatureStatuses updates the ## Features table in master .belmont/PROGRESS.md
// to match computed feature-level statuses. This prevents stale master data from causing
// auto mode to skip features that still have pending work.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func syncMasterFeatureStatuses(root string, features []featureSummary) {
	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	// Build lookup from computed features
	type computed struct {
		Status     string
		MsDone     int
		MsTotal    int
		TasksDone  int
		TasksTotal int
	}
	lookup := make(map[string]computed)
	for _, f := range features {
		lookup[f.Slug] = computed{
			Status:     f.Status,
			MsDone:     f.MilestonesDone,
			MsTotal:    f.MilestonesTotal,
			TasksDone:  f.TasksDone,
			TasksTotal: f.TasksTotal,
		}
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		statusCol := colIdx["Status"]
		msCol := colIdx["Milestones"]
		tasksCol := colIdx["Tasks"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		c, ok := lookup[slug]
		if !ok {
			continue
		}

		newStatus := c.Status
		newMs := fmt.Sprintf("%d/%d", c.MsDone, c.MsTotal)
		newTasks := fmt.Sprintf("%d/%d", c.TasksDone, c.TasksTotal)

		cellsChanged := false
		if statusCol >= 0 && statusCol < len(cells) && cells[statusCol] != newStatus {
			cells[statusCol] = newStatus
			cellsChanged = true
		}
		if msCol >= 0 && msCol < len(cells) && cells[msCol] != newMs {
			cells[msCol] = newMs
			cellsChanged = true
		}
		if tasksCol >= 0 && tasksCol < len(cells) && cells[tasksCol] != newTasks {
			cells[tasksCol] = newTasks
			cellsChanged = true
		}

		if cellsChanged {
			var parts []string
			for _, c := range cells {
				parts = append(parts, " "+c+" ")
			}
			lines[i] = "|" + strings.Join(parts, "|") + "|"
			changed = true
		}
	}

	if changed {
		os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
	}
}

// populateFeatureDeps enriches feature summaries with dependency and priority info from master PROGRESS.md.
func populateFeatureDeps(features []featureSummary, root string) {
	deps, priorities := parseMasterDeps(root)
	for i := range features {
		if d, ok := deps[features[i].Slug]; ok {
			features[i].Deps = d
		}
		if p, ok := priorities[features[i].Slug]; ok {
			features[i].Priority = p
		}
	}
}

func computeFeatureListStatus(features []featureSummary) string {
	allVerified := true
	allComplete := true
	anyProgress := false
	for _, f := range features {
		// Archived features count as terminal for both "Verified" and
		// "Complete" aggregates — otherwise cleaning up a finished feature
		// would regress the project status.
		if f.Status != "Verified" && f.Status != "Archived" {
			allVerified = false
		}
		if !isFeatureTerminal(f.Status) {
			allComplete = false
		}
		if f.TasksDone > 0 || f.TasksInProgress > 0 {
			anyProgress = true
		}
	}
	if allVerified && len(features) > 0 {
		return "Verified"
	}
	if allComplete && len(features) > 0 {
		return "Complete"
	}
	if anyProgress {
		return "In Progress"
	}
	return "Not Started"
}

func extractFeatureName(prd string) string {
	re := regexp.MustCompile(`(?m)^#\s*PRD:\s*(.+)$`)
	match := re.FindStringSubmatch(prd)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return "Unknown"
}

// extractArchiveName pulls the display name out of an ARCHIVE.md's
// "# Archive: <name>" header. Returns "" if no header is found.
func extractArchiveName(archive string) string {
	re := regexp.MustCompile(`(?m)^#\s*Archive:\s*(.+)$`)
	match := re.FindStringSubmatch(archive)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func parseTaskOrder(id string) (int, int) {
	re := regexp.MustCompile(`^P(\d+)-(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) != 3 {
		return 99, 99
	}
	return atoiDefault(match[1], 99), atoiDefault(match[2], 99)
}

func lastCompletedTask(tasks []task) *task {
	var last *task
	for i := range tasks {
		if tasks[i].Status == taskDone || tasks[i].Status == taskVerified {
			t := tasks[i]
			last = &t
		}
	}
	return last
}

func nextMilestone(milestones []milestone) *milestone {
	for _, m := range milestones {
		if !milestoneAllDone(m) {
			mm := m
			return &mm
		}
	}
	return nil
}

func nextTask(tasks []task) *task {
	for _, t := range tasks {
		if t.Status == taskInProgress || t.Status == taskTodo {
			tt := t
			return &tt
		}
	}
	return nil
}

// blockedTaskCount returns the number of tasks with [!] status across all milestones.
func blockedTaskCount(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				count++
			}
		}
	}
	return count
}

// blockedTaskNames returns descriptions of blocked tasks for display.
func blockedTaskNames(milestones []milestone) []string {
	var names []string
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				label := t.Name
				if t.ID != "" {
					label = t.ID + ": " + t.Name
				}
				names = append(names, label)
			}
		}
	}
	return names
}

func parseDecisions(progress string, limit int) []string {
	lines := parseSectionLines(progress, "## Decisions Log")
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func parseSectionLines(doc, header string) []string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(header) + `\s*$`)
	loc := re.FindStringIndex(doc)
	if loc == nil {
		return nil
	}
	rest := doc[loc[1]:]
	lines := strings.Split(rest, "\n")
	var results []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if trimmed == "" {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "none") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "-")
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func techPlanReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) != ""
}

// fileHasRealContent checks if a file exists and has content beyond template/placeholder text.
func fileHasRealContent(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return false
	}
	// Check for known template/placeholder texts
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "run /belmont:") || strings.HasPrefix(lower, "run the /belmont:") {
		return false
	}
	return true
}

func computeOverallStatus(tasks []task) string {
	if len(tasks) == 0 {
		return "Not Started"
	}

	allVerified := true
	allDone := true
	anyProgress := false
	allBlocked := true

	for _, t := range tasks {
		if t.Status != taskVerified {
			allVerified = false
		}
		if t.Status != taskDone && t.Status != taskVerified {
			allDone = false
		}
		if t.Status == taskDone || t.Status == taskVerified || t.Status == taskInProgress {
			anyProgress = true
		}
		if t.Status != taskBlocked {
			allBlocked = false
		}
	}

	if allVerified {
		return "Verified"
	}
	if allDone {
		return "Complete"
	}
	if allBlocked {
		return "BLOCKED"
	}
	if anyProgress {
		return "In Progress"
	}
	return "Not Started"
}

// isFeatureTerminal reports whether a feature is done — either all tasks
// implemented ("Complete"), all tasks verified ("Verified"), or archived via
// /belmont:cleanup ("Archived"). Terminal features are skipped by `auto --all`
// and treated as dep-satisfying (but non-executing) in wave planning.
func isFeatureTerminal(status string) bool {
	switch status {
	case "Complete", "Verified", "Archived":
		return true
	}
	return false
}

func renderStatus(report statusReport, color bool, showArchived bool) string {
	// Feature listing mode (default when no --feature specified)
	if report.Features != nil {
		return renderFeatureListing(report, color, showArchived)
	}

	techPlan := "Not written (run /belmont:tech-plan to create)"
	if report.TechPlanReady {
		techPlan = "Ready"
	}

	taskLine := fmt.Sprintf("Tasks: %d verified, %d done, %d in progress, %d blocked, %d todo (of %d total)",
		report.TaskCounts["verified"],
		report.TaskCounts["done"],
		report.TaskCounts["in_progress"],
		report.TaskCounts["blocked"],
		report.TaskCounts["todo"],
		report.TaskCounts["total"],
	)

	bold := func(s string) string {
		if color {
			return ansiBold + s + ansiReset
		}
		return s
	}

	var sb strings.Builder
	sb.WriteString(bold("Belmont Status") + "\n")
	sb.WriteString("==============\n\n")
	if report.Monorepo != nil {
		primaryLabel := ""
		if report.Monorepo.Primary != "" {
			primaryLabel = fmt.Sprintf(", primary=%s", report.Monorepo.Primary)
		}
		sb.WriteString(fmt.Sprintf("Monorepo: %s (%d workspaces%s)\n\n", report.Monorepo.Type, len(report.Monorepo.Workspaces), primaryLabel))
	}
	sb.WriteString(fmt.Sprintf("Feature: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", colorStatus(report.OverallStatus, color)))
	sb.WriteString(taskLine)
	sb.WriteString("\n\n")

	if len(report.Tasks) > 0 {
		for _, t := range report.Tasks {
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", taskStatusIcon(t.Status, color), t.ID, t.Name))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Milestones:\n")
	if len(report.Milestones) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		anyLive := false
		for _, m := range report.Milestones {
			icon := milestoneStatusIcon(m, color)
			line := fmt.Sprintf("  %s %s: %s", icon, m.ID, m.Name)
			if m.LiveFrom != "" {
				anyLive = true
				if color {
					line += " \033[2m(live from worktree)\033[0m"
				} else {
					line += " (live from worktree)"
				}
			}
			sb.WriteString(line + "\n")
		}
		if anyLive {
			if color {
				sb.WriteString("  \033[2m⟳ live-tagged milestones reflect the worktree's in-flight state (not yet merged to master)\033[0m\n")
			} else {
				sb.WriteString("  live-tagged milestones reflect the worktree's in-flight state (not yet merged to master)\n")
			}
		}
	}
	sb.WriteString("\n")

	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		sb.WriteString("Blocked Tasks:\n")
		for _, b := range blocked {
			sb.WriteString(fmt.Sprintf("  - %s\n", b))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Next Milestone:\n")
	if report.NextMilestone == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextMilestone.ID, report.NextMilestone.Name))
	}
	sb.WriteString("Next Individual Task:\n")
	if report.NextTask == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextTask.ID, report.NextTask.Name))
	}
	sb.WriteString("\n")

	sb.WriteString("Recent Activity:\n")
	sb.WriteString("---\n")
	if report.LastCompleted == nil {
		sb.WriteString("Last completed: None\n")
	} else {
		sb.WriteString(fmt.Sprintf("Last completed: %s - %s\n", report.LastCompleted.ID, report.LastCompleted.Name))
	}
	sb.WriteString("Recent decisions:\n")
	if len(report.RecentDecisions) == 0 {
		sb.WriteString("  - None\n")
	} else {
		for _, d := range report.RecentDecisions {
			sb.WriteString(fmt.Sprintf("  - %s\n", d))
		}
	}
	sb.WriteString(statusLegend(color))
	return sb.String()
}

func milestoneStatusIcon(m milestone, color bool) string {
	if milestoneAllVerified(m) {
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	} else if milestoneAllDone(m) {
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	} else if milestoneHasBlockers(m) {
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	} else if !milestoneNotStarted(m) {
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	}
	if color {
		return ansiDim + "[ ]" + ansiReset
	}
	return "[ ]"
}

func colorStatus(status string, color bool) string {
	if !color {
		return status
	}
	switch status {
	case "Verified":
		return ansiGreen + status + ansiReset
	case "Complete":
		return ansiCyan + status + ansiReset
	case "In Progress":
		return ansiYellow + status + ansiReset
	case "BLOCKED":
		return ansiRed + status + ansiReset
	case "Not Started", "Archived":
		return ansiDim + status + ansiReset
	default:
		return status
	}
}

func statusLegend(color bool) string {
	if !color {
		return "\nLegend: [v] verified  [x] done  [>] in progress  [!] blocked  [ ] todo\n"
	}
	return fmt.Sprintf("\nLegend: %s[v]%s verified  %s[x]%s done  %s[>]%s in progress  %s[!]%s blocked  %s[ ]%s todo\n",
		ansiGreen, ansiReset, ansiCyan, ansiReset, ansiYellow, ansiReset, ansiRed, ansiReset, ansiDim, ansiReset)
}

func featureStatusIcon(status string, color bool) string {
	bracket := "[ ]"
	switch status {
	case "Verified":
		bracket = "[v]"
	case "Complete":
		bracket = "[x]"
	case "In Progress":
		bracket = "[>]"
	case "Archived":
		bracket = "[-]"
	}
	if !color {
		return bracket
	}
	switch status {
	case "Verified":
		return ansiGreen + bracket + ansiReset
	case "Complete":
		return ansiCyan + bracket + ansiReset
	case "In Progress":
		return ansiYellow + bracket + ansiReset
	default:
		return ansiDim + bracket + ansiReset
	}
}

func renderFeatureListing(report statusReport, color bool, showArchived bool) string {
	prfaq := "Not written (run /belmont:working-backwards)"
	if report.PRFAQReady {
		prfaq = "Written"
	}
	techPlan := "Not written"
	if report.TechPlanReady {
		techPlan = "Ready"
	}

	bold := func(s string) string {
		if color {
			return ansiBold + s + ansiReset
		}
		return s
	}

	var sb strings.Builder
	sb.WriteString(bold("Belmont Status") + "\n")
	sb.WriteString("==============\n\n")
	if report.Monorepo != nil {
		primaryLabel := ""
		if report.Monorepo.Primary != "" {
			primaryLabel = fmt.Sprintf(", primary=%s", report.Monorepo.Primary)
		}
		sb.WriteString(fmt.Sprintf("Monorepo: %s (%d workspaces%s)\n\n", report.Monorepo.Type, len(report.Monorepo.Workspaces), primaryLabel))
	}
	sb.WriteString(fmt.Sprintf("Product: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("PR/FAQ: %s\n", prfaq))
	sb.WriteString(fmt.Sprintf("Master Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", colorStatus(report.OverallStatus, color)))

	// report.Features is already archived-free (split in buildStatus).
	// Archived features are rendered separately as a compact block below the
	// active listing so their "0/0" noise doesn't clutter active work.
	listing := report.Features

	if len(listing) == 0 && len(report.ArchivedFeatures) == 0 {
		sb.WriteString("Features:\n")
		sb.WriteString("  (none — run /belmont:product-plan to create your first feature)\n")
	} else if len(listing) == 0 {
		sb.WriteString("Features:\n")
		sb.WriteString("  (no active features — all archived)\n\n")
	} else {
		for _, f := range listing {
			icon := featureStatusIcon(f.Status, color)
			sb.WriteString(fmt.Sprintf("%s %s (%s)\n", icon, f.Name, f.Slug))
			sb.WriteString(fmt.Sprintf("  Tasks: %d/%d done", f.TasksDone, f.TasksTotal))
			if f.TasksVerified > 0 {
				sb.WriteString(fmt.Sprintf(" (%d verified)", f.TasksVerified))
			}
			if f.MilestonesTotal > 0 {
				sb.WriteString(fmt.Sprintf("  |  Milestones: %d/%d done", f.MilestonesDone, f.MilestonesTotal))
			}
			sb.WriteString("\n")

			// Show milestone listing
			if len(f.Milestones) > 0 {
				for _, m := range f.Milestones {
					isNext := f.NextMilestone != nil && m.ID == f.NextMilestone.ID
					mIcon := milestoneStatusIcon(m, color)
					if milestoneNotStarted(m) && isNext {
						if color {
							mIcon = ansiYellow + "[>]" + ansiReset
						} else {
							mIcon = "[>]"
						}
					}
					sb.WriteString(fmt.Sprintf("    %s %s: %s\n", mIcon, m.ID, m.Name))
				}
			}

			// Show next task if feature is in progress
			if f.NextTask != nil && f.Status == "In Progress" {
				sb.WriteString(fmt.Sprintf("  Next: %s — %s\n", f.NextTask.ID, f.NextTask.Name))
			}

			// Show blocked tasks if any
			if f.TasksBlocked > 0 {
				blockedNames := blockedTaskNames(f.Milestones)
				sb.WriteString("  Blocked:\n")
				for _, b := range blockedNames {
					sb.WriteString(fmt.Sprintf("    - %s\n", b))
				}
			}

			sb.WriteString("\n")
		}
	}

	if showArchived && len(report.ArchivedFeatures) > 0 {
		var block strings.Builder
		block.WriteString(fmt.Sprintf("Archived (%d):\n", len(report.ArchivedFeatures)))
		for _, f := range report.ArchivedFeatures {
			if f.Name != "" && f.Name != f.Slug {
				block.WriteString(fmt.Sprintf("  - %s — %s\n", f.Slug, f.Name))
			} else {
				block.WriteString(fmt.Sprintf("  - %s\n", f.Slug))
			}
		}
		out := block.String()
		if color {
			out = ansiDim + out + ansiReset
		}
		sb.WriteString(out)
		sb.WriteString("\n")
	}

	sb.WriteString("Use --feature <slug> for detailed task-level status.\n")
	sb.WriteString(statusLegend(color))
	return sb.String()
}

// ANSI color codes for terminal output
const (
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
)

func taskStatusIcon(status taskStatus, color bool) string {
	switch status {
	case taskVerified:
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	case taskDone:
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	case taskInProgress:
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	case taskBlocked:
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	default:
		if color {
			return ansiDim + "[ ]" + ansiReset
		}
		return "[ ]"
	}
}

type toolConfig struct {
	Name  string
	Label string
}

var toolConfigs = []toolConfig{
	{Name: "claude", Label: "Claude Code (.claude/)"},
	{Name: "codex", Label: "Codex (.codex/)"},
	{Name: "cursor", Label: "Cursor (.cursor/)"},
	{Name: "windsurf", Label: "Windsurf (.windsurf/)"},
	{Name: "gemini", Label: "Gemini (.gemini/)"},
	{Name: "copilot", Label: "GitHub Copilot (.copilot/)"},
	{Name: "pi", Label: "Pi (.pi/)"},
	{Name: "opencode", Label: "opencode (.opencode/)"},
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ensureSkillsGenerated re-runs `scripts/generate-skills.sh` against the
// given Belmont source root if its generated output is missing or stale
// (any `_src/` or `_partials/` file newer than any `<skill>/SKILL.md`).
// This makes source-mode `belmont install --source` self-healing — no
// separate `make` step required, even though the generated layout isn't
// committed to git.
func ensureSkillsGenerated(sourceRoot string) error {
	srcDir := filepath.Join(sourceRoot, "skills", "belmont", "_src")
	partialsDir := filepath.Join(sourceRoot, "skills", "belmont", "_partials")
	skillsDir := filepath.Join(sourceRoot, "skills", "belmont")
	script := filepath.Join(sourceRoot, "scripts", "generate-skills.sh")

	if !dirExists(srcDir) {
		// Older Belmont source layouts didn't have _src/ — skip silently.
		return nil
	}
	if !fileExists(script) {
		return nil
	}

	stale := false
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil
	}
	for _, entry := range srcEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		srcInfo, err := os.Stat(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			continue
		}
		genPath := filepath.Join(skillsDir, name, "SKILL.md")
		genInfo, err := os.Stat(genPath)
		if err != nil {
			stale = true
			break
		}
		if srcInfo.ModTime().After(genInfo.ModTime()) {
			stale = true
			break
		}
	}

	// Also check partials — any partial change invalidates everything that
	// includes it, and including that into per-file dependency tracking is
	// not worth the complexity.
	if !stale && dirExists(partialsDir) {
		partialEntries, _ := os.ReadDir(partialsDir)
		var oldestGen time.Time
		first := true
		_ = filepath.WalkDir(skillsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(p) != "SKILL.md" {
				return nil
			}
			info, err := os.Stat(p)
			if err != nil {
				return nil
			}
			if first || info.ModTime().Before(oldestGen) {
				oldestGen = info.ModTime()
				first = false
			}
			return nil
		})
		if !first {
			for _, entry := range partialEntries {
				if entry.IsDir() {
					continue
				}
				info, err := os.Stat(filepath.Join(partialsDir, entry.Name()))
				if err != nil {
					continue
				}
				if info.ModTime().After(oldestGen) {
					stale = true
					break
				}
			}
		}
	}

	if !stale {
		return nil
	}

	fmt.Println("Regenerating skills from _src/ + _partials/ ...")
	cmd := exec.Command("bash", script)
	cmd.Dir = sourceRoot
	cmd.Stdout = io.Discard // already verbose; suppress unless errored
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install: failed to regenerate skills (run scripts/generate-skills.sh manually): %w", err)
	}
	return nil
}

func resolveSourceRoot(source string) (string, error) {
	if source != "" {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if env := strings.TrimSpace(os.Getenv("BELMONT_SOURCE")); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if cfgSource, ok := loadConfigSource(); ok {
		abs, err := filepath.Abs(cfgSource)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	for i := 0; i < 6; i++ {
		skills := filepath.Join(dir, "skills", "belmont")
		agents := filepath.Join(dir, "agents", "belmont")
		if dirExists(skills) && dirExists(agents) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("install: unable to locate belmont source; pass --source PATH")
}

func loadConfigSource() (string, bool) {
	paths := configPaths()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg config
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if strings.TrimSpace(cfg.Source) != "" {
			return cfg.Source, true
		}
	}
	return "", false
}

func configPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	paths := []string{
		filepath.Join(home, ".config", "belmont", "config.json"),
		filepath.Join(home, ".belmont", "config.json"),
	}
	return paths
}

func resolveTools(projectRoot, toolsFlag string, noPrompt bool) ([]string, error) {
	detected := detectTools(projectRoot)

	if toolsFlag != "" {
		switch strings.ToLower(toolsFlag) {
		case "all":
			if len(detected) > 0 {
				return detected, nil
			}
			return allToolNames(), nil
		case "none":
			return nil, nil
		default:
			parts := strings.Split(toolsFlag, ",")
			var selected []string
			for _, p := range parts {
				name := strings.TrimSpace(strings.ToLower(p))
				if name == "" {
					continue
				}
				if !isKnownTool(name) {
					return nil, fmt.Errorf("install: unknown tool %q", name)
				}
				selected = append(selected, name)
			}
			return selected, nil
		}
	}

	if noPrompt {
		if len(detected) > 0 {
			return detected, nil
		}
		return nil, nil
	}

	reader := bufio.NewReader(os.Stdin)

	if len(detected) > 0 {
		fmt.Println("Detected AI tools:")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s\n", i+1, toolLabel(tool))
		}
		fmt.Println("")
		fmt.Println("Install skills for:")
		fmt.Println("  [a] All detected tools")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s only\n", i+1, toolLabel(tool))
		}
		fmt.Println("  [s] Skip (install agents only)")
		fmt.Println("")
		fmt.Print("Choice [a]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		fmt.Println("")

		if choice == "" || strings.EqualFold(choice, "a") {
			return detected, nil
		}
		if strings.EqualFold(choice, "s") {
			return nil, nil
		}
		if idx, err := strconv.Atoi(choice); err == nil {
			if idx >= 1 && idx <= len(detected) {
				return []string{detected[idx-1]}, nil
			}
		}
		return detected, nil
	}

	fmt.Println("No AI tool directories detected.")
	fmt.Println("")
	fmt.Println("Which tool are you using?")
	for i, tool := range toolConfigs {
		fmt.Printf("  [%d] %s\n", i+1, tool.Label)
	}
	fmt.Println("  [s] Skip (install agents only - reference files manually)")
	fmt.Println("")
	fmt.Print("Choice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	fmt.Println("")

	if strings.EqualFold(choice, "s") {
		return nil, nil
	}
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(toolConfigs) {
			return []string{toolConfigs[idx-1].Name}, nil
		}
	}

	fmt.Println("Invalid choice. Installing agents only.")
	return nil, nil
}

// detectTools returns the AI tools Belmont should install for. A tool is
// included when ANY of these signals fires:
//   - its conventional project-level dir exists (`.claude/`, `.codex/`, etc.),
//     signaling the user has used the tool in this repo before;
//   - its CLI binary is present on PATH, signaling the user has the tool
//     installed system-wide (so they'll likely want Belmont wired up here);
//   - a Belmont skill-routing section exists in AGENTS.md (for Codex and
//     Copilot) or GEMINI.md (for Gemini), signaling Belmont was previously
//     installed for that tool here — needed because Phase 1+ installs no
//     longer create `.codex/`, `.gemini/`, or `.copilot/` marker dirs;
//   - a root `opencode.json` / `opencode.jsonc` exists (for opencode, whose
//     `.opencode/` directory is optional when config lives at the root).
func detectTools(projectRoot string) []string {
	tools := []string{"claude", "codex", "cursor", "windsurf", "gemini", "copilot", "pi", "opencode"}
	dirMarker := map[string]string{
		"claude":   ".claude",
		"codex":    ".codex",
		"cursor":   ".cursor",
		"windsurf": ".windsurf",
		"gemini":   ".gemini",
		"copilot":  ".copilot",
		"pi":       ".pi",
		"opencode": ".opencode",
	}
	hasAgentsBelmontSection := fileContainsMarker(filepath.Join(projectRoot, "AGENTS.md"), belmontAgentsSectionStart)
	hasGeminiBelmontSection := fileContainsMarker(filepath.Join(projectRoot, "GEMINI.md"), belmontGeminiSectionStart)

	var detected []string
	for _, tool := range tools {
		if dirExists(filepath.Join(projectRoot, dirMarker[tool])) {
			detected = append(detected, tool)
			continue
		}
		if _, err := exec.LookPath(toolBinary(tool)); err == nil {
			detected = append(detected, tool)
			continue
		}
		// File-based fallback for AGENTS.md / GEMINI.md-routed tools.
		switch tool {
		case "codex", "copilot":
			if hasAgentsBelmontSection {
				detected = append(detected, tool)
			}
		case "gemini":
			if hasGeminiBelmontSection {
				detected = append(detected, tool)
			}
		case "opencode":
			// opencode projects often carry only a root opencode.json(c) —
			// the .opencode/ marker directory is optional.
			if fileExists(filepath.Join(projectRoot, "opencode.json")) || fileExists(filepath.Join(projectRoot, "opencode.jsonc")) {
				detected = append(detected, tool)
			}
		}
	}
	return detected
}

// fileContainsMarker returns true if the file at path exists and contains the
// given marker substring. Used by detectTools to spot Belmont's skill-routing
// section as a "previously installed for this tool" signal.
func fileContainsMarker(path, marker string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), marker)
}

func syncMarkdownDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync the references/ subdir if present (progressive-disclosure detail
	// loaded on demand by skills via relative paths).
	refsSrc := filepath.Join(sourceDir, "references")
	refsDest := filepath.Join(targetDir, "references")
	if dirExists(refsSrc) {
		if err := syncReferencesDir(refsSrc, refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// claudeOnlySkills lists skills that must NOT be exposed through the shared
// `.agents/skills/belmont/` discovery surface (which every supported CLI scans)
// because they depend on Claude-Code-only mechanics. These skills are skipped
// by syncSkillsFolderDir / syncEmbeddedSkillsFolderDir so non-Claude tools
// never list or load them. Claude Code still gets them as real
// `.claude/commands/belmont/<skill>.md` slash-command files (written from the
// generated SKILL.md content, not symlinked into `.agents/skills/`, since they
// don't live there). See knowledge/cross-cutting/skill-format.md.
//
// `loop` delegates to Claude Code's built-in `/loop` skill (self-paced via
// ScheduleWakeup) — those mechanics don't exist on Codex/Cursor/Gemini/etc.,
// so exposing it there would only produce a broken interactive experience.
var claudeOnlySkills = map[string]bool{
	"loop": true,
}

// collectClaudeOnlyCommandsSource reads the generated SKILL.md content for each
// claude-only skill from a source skills dir (`<root>/skills/belmont`). The
// returned map (skill name -> SKILL.md content) is written verbatim as real
// Claude slash-command files by linkClaudeCommands. A claude-only skill whose
// generated SKILL.md is missing is skipped silently (generation runs first).
func collectClaudeOnlyCommandsSource(skillsSource string) map[string]string {
	out := map[string]string{}
	for name := range claudeOnlySkills {
		data, err := os.ReadFile(filepath.Join(skillsSource, name, "SKILL.md"))
		if err != nil {
			continue
		}
		out[name] = string(data)
	}
	return out
}

// collectClaudeOnlyCommandsEmbedded is the embed.FS counterpart of
// collectClaudeOnlyCommandsSource. `root` is the embedded skills root
// (`skills/belmont`).
func collectClaudeOnlyCommandsEmbedded(embedFS embed.FS, root string) map[string]string {
	out := map[string]string{}
	for name := range claudeOnlySkills {
		data, err := fs.ReadFile(embedFS, root+"/"+name+"/SKILL.md")
		if err != nil {
			continue
		}
		out[name] = string(data)
	}
	return out
}

// syncReferencesDir mirrors .md files in a references/ subdirectory.
func syncReferencesDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// linkClaudeCommands creates per-skill symlinks at
// `.claude/commands/belmont/<skill>.md` -> `.agents/skills/belmont/<skill>/SKILL.md`.
// Claude Code 2.1.x registers each one as a `/belmont:<skill>` slash command
// (subfolder under `.claude/commands/` becomes the namespace prefix). The
// agentskills.io frontmatter (`name:`, `description:`) on SKILL.md is also
// valid frontmatter for Claude Code slash commands, so no rewriting is needed.
// claudeOnlyCmds maps claude-only skill names to their SKILL.md content; these
// are written as real command files (not symlinks) because the skills are
// deliberately absent from `.agents/skills/belmont/`.
func linkClaudeCommands(projectRoot, skillsTarget string, claudeOnlyCmds map[string]string) error {
	return syncSkillCommands(projectRoot, skillsTarget, filepath.Join(".claude", "commands", "belmont"),
		func(cmdPath, skillFile, skill string) error {
			return ensureSymlink(cmdPath, skillFile, false)
		}, claudeOnlyCmds)
}

// linkOpencodeCommands creates per-skill wrapper command files at
// `.opencode/command/belmont/<skill>.md`. opencode registers each one as a
// `/belmont/<skill>` slash command (the command name is the file path
// relative to `command/`, minus `.md` — opencode namespaces with `/`, not
// Claude's `:`). Skills themselves remain discoverable via opencode's
// `skill` tool, but the TUI's `/` autocomplete only lists commands — these
// files are what make `/belmont…` show the skill list.
//
// Unlike Claude Code, these CANNOT be symlinks to SKILL.md: opencode's
// command loader builds the raw config as `{name, ...frontmatter, template}`,
// so a SKILL.md `name:` key (required by agentskills.io) OVERRIDES the
// path-derived `belmont/<skill>` name and the command registers under the
// bare skill name instead — colliding with (and shadowing) the skill itself
// and never appearing under `/belmont`. So each command is a small generated
// wrapper: `description:` copied from the skill frontmatter (feeds the TUI
// autocomplete) and a body that tells the model to read the canonical
// SKILL.md — the same delegation form adaptPromptForTool uses in auto mode,
// which also keeps the skill's relative `references/` paths resolving from
// the real skill directory.
func linkOpencodeCommands(projectRoot, skillsTarget string) error {
	// opencode gets no claude-only skills (loop relies on Claude's /loop), so
	// the extra-commands map is nil.
	return syncSkillCommands(projectRoot, skillsTarget, filepath.Join(".opencode", "command", "belmont"),
		writeOpencodeCommandFile, nil)
}

// writeOpencodeCommandFile generates the wrapper command file for one skill.
// Idempotent: an up-to-date file is left untouched. A pre-existing symlink at
// cmdPath (from an older Belmont install attempt) is removed first — writing
// through it would corrupt the canonical SKILL.md it points at.
func writeOpencodeCommandFile(cmdPath, skillFile, skill string) error {
	var b strings.Builder
	b.WriteString("---\n")
	if desc := skillDescription(skillFile); desc != "" {
		b.WriteString("description: " + strconv.Quote(desc) + "\n")
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b,
		"Run the belmont:%s skill. Read .agents/skills/belmont/%s/SKILL.md fully and follow the instructions in its body to completion. $ARGUMENTS\n",
		skill, skill)
	content := b.String()

	if st, err := os.Lstat(cmdPath); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			fmt.Printf("  ~ %s (replacing symlink with wrapper command)\n", cmdPath)
			if err := os.Remove(cmdPath); err != nil {
				return err
			}
		} else if existing, err := os.ReadFile(cmdPath); err == nil && string(existing) == content {
			fmt.Printf("  = %s (command ok)\n", cmdPath)
			return nil
		}
	}
	if err := os.WriteFile(cmdPath, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("  + %s\n", cmdPath)
	return nil
}

// writeCodexSkillInterfaces writes Codex UI metadata at
// `.agents/skills/belmont/<skill>/agents/openai.yaml` for every installed
// skill. Codex's skill loader reads `agents/openai.yaml` next to each
// SKILL.md (core-skills loader.rs: SKILLS_METADATA_DIR="agents",
// SKILLS_METADATA_FILENAME="openai.yaml") and its `$`-mention popup prefers
// `interface.display_name` over the frontmatter `name:`, fuzzy-matching the
// typed filter against it. The shared `belmont:` display-name prefix is what
// groups the skills: typing `$belmont` surfaces the full set with
// descriptions — Codex's `/` menu itself only lists built-ins and cannot be
// extended (the `~/.codex/prompts` custom-prompt feature was removed
// upstream).
//
// Per agentskills.io, the `agents/` subdir is product-specific config that
// other CLIs ignore, so this stays inside the canonical skill folders
// without affecting the seven other supported tools. Idempotent: up-to-date
// files are left untouched. No separate prune pass is needed — a removed
// skill's openai.yaml disappears with its folder when the skill sync prunes
// the whole directory.
func writeCodexSkillInterfaces(skillsTarget string) error {
	entries, err := os.ReadDir(skillsTarget)
	if err != nil {
		return fmt.Errorf("read skills dir %s: %w", skillsTarget, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip generation scaffolding dirs (shouldn't be at this level after
		// install but defensive).
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsTarget, name, "SKILL.md")); err != nil {
			continue
		}
		content := "# Generated by Belmont -- Codex UI metadata; other CLIs ignore this file.\n" +
			"# The shared display-name prefix groups Belmont's skills in Codex's $-mention\n" +
			"# popup: type \"$belmont\" to list them all.\n" +
			"interface:\n" +
			"  display_name: \"belmont:" + name + "\"\n"
		yamlPath := filepath.Join(skillsTarget, name, "agents", "openai.yaml")
		if existing, err := os.ReadFile(yamlPath); err == nil && string(existing) == content {
			fmt.Printf("  = %s/agents/openai.yaml (ok)\n", name)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Printf("  + %s/agents/openai.yaml (Codex $-mention display name)\n", name)
	}
	return nil
}

// skillDescription extracts the `description:` value from a SKILL.md
// frontmatter block. Returns "" when the file or the key is missing —
// Belmont-generated skills always carry single-line descriptions.
func skillDescription(skillFile string) string {
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "description:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return ""
}

func ensureSymlink(linkPath, target string, isDir bool) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	// Compute a relative target from the link's directory to the target.
	// Relative symlinks resolve identically in main and in git worktrees, so
	// the symlink content is byte-identical across trees — prevents merge
	// conflicts when the same install is re-run from different worktree roots.
	// If relative computation fails (e.g. different volumes on Windows), fall
	// back to the absolute target.
	symlinkTarget := target
	if rel, err := filepath.Rel(filepath.Dir(linkPath), target); err == nil {
		symlinkTarget = rel
	}

	if existing, err := os.Lstat(linkPath); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(linkPath)
			if err == nil && current == symlinkTarget {
				fmt.Printf("  = %s (symlink ok)\n", linkPath)
				return nil
			}
		}
		if existing.IsDir() {
			fmt.Printf("  ~ %s (replacing old directory with symlink)\n", linkPath)
			if err := os.RemoveAll(linkPath); err != nil {
				return err
			}
		} else {
			fmt.Printf("  ~ %s (replacing existing file with symlink)\n", linkPath)
			if err := os.Remove(linkPath); err != nil {
				return err
			}
		}
	}

	if err := os.Symlink(symlinkTarget, linkPath); err != nil {
		fmt.Printf("  ! symlink failed for %s (copying instead)\n", linkPath)
		if isDir {
			return copyDir(target, linkPath)
		}
		return copyFile(target, linkPath)
	}
	fmt.Printf("  + %s -> %s\n", linkPath, symlinkTarget)
	return nil
}

func ensureStateFiles(projectRoot string) error {
	stateDir := filepath.Join(projectRoot, ".belmont")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	// Ensure auto-mode artifacts are gitignored
	ensureGitignoreEntry(projectRoot, ".belmont/auto.json")
	ensureGitignoreEntry(projectRoot, ".belmont/worktrees/")

	// Create features directory
	featuresDir := filepath.Join(stateDir, "features")
	if !dirExists(featuresDir) {
		if err := os.MkdirAll(featuresDir, 0o755); err != nil {
			return err
		}
		fmt.Println("  + .belmont/features/")
	} else {
		fmt.Println("  Exists: .belmont/features/ (keeping)")
	}

	// Create PR_FAQ.md template
	prfaqPath := filepath.Join(stateDir, "PR_FAQ.md")
	if !fileExists(prfaqPath) {
		if err := os.WriteFile(prfaqPath, []byte("Run /belmont:working-backwards to create your PR/FAQ document.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PR_FAQ.md")
	} else {
		fmt.Println("  Exists: .belmont/PR_FAQ.md (keeping)")
	}

	prdPath := filepath.Join(stateDir, "PRD.md")
	if !fileExists(prdPath) {
		if err := os.WriteFile(prdPath, []byte("Run the /belmont:product-plan skill to create a plan for your feature.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PRD.md")
	} else {
		fmt.Println("  Exists: .belmont/PRD.md (keeping)")
	}

	return nil
}

// Marker constants for Belmont's skill-routing section. Phase 2 no longer
// writes these sections (every supported CLI auto-discovers
// `.agents/skills/<name>/SKILL.md`), but the markers are still recognized for:
//   - detectTools (so a Phase 1 project where Belmont is already wired
//     continues to be detected during a Phase 2 re-install);
//   - the legacy cleanup pass (so AGENTS.md / GEMINI.md sections written by
//     Phase 1 get stripped on next install).
const belmontAgentsSectionStart = "<!-- belmont:skill-routing:start -->"
const belmontAgentsSectionEnd = "<!-- belmont:skill-routing:end -->"
const belmontGeminiSectionStart = "<!-- belmont:skill-routing:start -->"
const belmontGeminiSectionEnd = "<!-- belmont:skill-routing:end -->"

// Even older marker pair from the pre-Phase-1 Codex-only routing section.
// Cleanup pass strips this too.
const codexAgentsGuidanceStart = "<!-- belmont:codex-skill-routing:start -->"
const codexAgentsGuidanceEnd = "<!-- belmont:codex-skill-routing:end -->"

// removeMarkedSection strips a marker-delimited block from content, returning
// the result and whether anything was removed. Used to migrate users off
// older marker pairs to the current ones.
func removeMarkedSection(content, startMarker, endMarker string) (string, bool) {
	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start < 0 || end <= start {
		return content, false
	}
	end += len(endMarker)
	// Eat one trailing newline if present, to avoid leaving a blank line gap.
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[:start] + content[end:], true
}

func filesEqual(a, b string) (bool, error) {
	ab, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return string(ab) == string(bb), nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(dest); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func ensureDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return os.MkdirAll(path, 0o755)
}

func copyDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// syncEmbeddedDir mirrors syncMarkdownDir but reads from an embed.FS.
func syncEmbeddedDir(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	hasReferences := false
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "references" {
				hasReferences = true
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	// Clean stale files
	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync references/ subdir from the embed FS.
	refsDest := filepath.Join(targetDir, "references")
	if hasReferences {
		if err := syncEmbeddedReferences(embedFS, root+"/references", refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// syncEmbeddedReferences mirrors the references/ subdir from an embed.FS.
func syncEmbeddedReferences(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- Update command ---

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatestRelease() (*githubRelease, error) {
	url := "https://api.github.com/repos/blake-simpson/belmont/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach GitHub (are you offline?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return nil, fmt.Errorf("GitHub API rate limited — set GITHUB_TOKEN env var to authenticate")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func checkWriteAccess(dir string) error {
	tmp := filepath.Join(dir, ".belmont-update-check")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

func parseSemver(v string) (int, int, int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func isNewer(remote, local string) bool {
	rMaj, rMin, rPat, ok1 := parseSemver(remote)
	lMaj, lMin, lPat, ok2 := parseSemver(local)
	if !ok1 || !ok2 {
		return true
	}
	if rMaj != lMaj {
		return rMaj > lMaj
	}
	if rMin != lMin {
		return rMin > lMin
	}
	return rPat > lPat
}

// ── Loop command ──

func runAutoCmd(args []string) error {
	fs := flag.NewFlagSet("auto", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg loopConfig
	var policyStr string
	var featuresFlag string
	var allFlag bool
	var allowDirty bool
	fs.StringVar(&cfg.Feature, "feature", "", "feature slug (required)")
	fs.StringVar(&featuresFlag, "features", "", "comma-separated feature slugs for parallel execution")
	fs.BoolVar(&allFlag, "all", false, "run all pending features in parallel")
	fs.StringVar(&cfg.From, "from", "", "start milestone (e.g. M1)")
	fs.StringVar(&cfg.To, "to", "", "end milestone (e.g. M5)")
	fs.StringVar(&cfg.Tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor|pi|opencode)")
	fs.StringVar(&policyStr, "policy", "autonomous", "checkpoint policy (autonomous|milestone|every_action)")
	fs.IntVar(&cfg.MaxIterations, "max-iterations", 50, "maximum loop iterations")
	fs.IntVar(&cfg.MaxFailures, "max-failures", 3, "consecutive failures before stopping")
	fs.IntVar(&cfg.MaxParallel, "max-parallel", 5, "max concurrent goroutines for parallel execution")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "show execution plan without running")
	fs.BoolVar(&allowDirty, "allow-dirty", false, "skip the clean-working-tree check (not recommended — risks merge failures)")
	fs.StringVar(&cfg.Root, "root", ".", "project root")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Validate mutual exclusivity
	multiFeature := featuresFlag != "" || allFlag
	if multiFeature && cfg.Feature != "" {
		return fmt.Errorf("auto: --feature cannot be combined with --features or --all")
	}
	if featuresFlag != "" && allFlag {
		return fmt.Errorf("auto: --features and --all are mutually exclusive")
	}
	if multiFeature && (cfg.From != "" || cfg.To != "") {
		return fmt.Errorf("auto: --from/--to cannot be used with --features or --all")
	}
	if !multiFeature && cfg.Feature == "" {
		return fmt.Errorf("auto: --feature is required (or use --features/--all for multi-feature mode)")
	}

	switch checkpointPolicy(policyStr) {
	case policyAutonomous, policyMilestone, policyEveryAction:
		cfg.Policy = checkpointPolicy(policyStr)
	default:
		return fmt.Errorf("auto: invalid --policy %q (use autonomous, milestone, or every_action)", policyStr)
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = absRoot

	// Auto-detect tool if not specified
	if cfg.Tool == "" {
		detected := detectTool()
		if detected == "" {
			return fmt.Errorf("auto: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor, pi, opencode\nInstall one or use --tool to specify")
		}
		cfg.Tool = detected
	} else {
		// Validate tool name
		switch cfg.Tool {
		case "claude", "codex", "gemini", "copilot", "cursor", "pi", "opencode":
			// ok
		default:
			return fmt.Errorf("auto: unsupported tool %q (use claude, codex, gemini, copilot, cursor, pi, or opencode)", cfg.Tool)
		}
	}

	// Refuse to start against a dirty working tree — uncommitted changes risk
	// blocking later worktree merges. Skipped on --dry-run (no merges happen)
	// and --allow-dirty (explicit opt-out).
	if !allowDirty && !cfg.DryRun {
		if err := requireCleanWorkingTree(absRoot); err != nil {
			return err
		}
	}

	// Multi-feature mode: --features or --all
	if multiFeature {
		slugs, err := resolveFeatureSlugs(absRoot, featuresFlag, allFlag)
		if err != nil {
			return err
		}
		return runAutoMultiFeature(cfg, slugs)
	}

	// Single-feature mode
	// Verify feature directory exists
	featureDir := filepath.Join(absRoot, ".belmont", "features", cfg.Feature)
	if !dirExists(featureDir) {
		return fmt.Errorf("auto: feature %q not found at %s", cfg.Feature, featureDir)
	}

	// Load per-feature model tiers (if models.yaml exists)
	tiers, tierErr := parseModelTiers(filepath.Join(featureDir, "models.yaml"))
	if tierErr != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ failed to parse models.yaml: %s — falling back to defaults\033[0m\n", tierErr)
	}
	cfg.ModelTiers = tiers

	// Read milestones and check for dependency syntax
	progressPath := filepath.Join(absRoot, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("auto: failed to read PROGRESS.md: %w", err)
	}
	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, cfg.From, cfg.To)

	// Structural lint: warn on polish/follow-up milestone patterns before
	// starting the loop. These are the exact shape that causes parallel merge
	// conflicts (per skills/belmont/_partials/milestone-immutability.md).
	// Prompt the user to continue or abort; non-interactive runs abort on
	// violations to avoid silent damage.
	if violations := detectViolations(cfg.Feature, milestones); len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "\033[31m✗ Milestone-structure violation(s) detected:\033[0m\n\n")
		renderValidationReport(os.Stderr, violations)
		if !isTerminal(os.Stdin) {
			return fmt.Errorf("auto: %d milestone-structure violation(s); restructure via `/belmont:tech-plan` before rerunning, or run with a TTY to override interactively", len(violations))
		}
		fmt.Fprintf(os.Stderr, "Proceed anyway? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			return fmt.Errorf("auto: aborted — restructure milestones via `/belmont:tech-plan` then rerun")
		}
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Proceeding despite violations — you are on your own for merge fallout\033[0m\n")
	} else {
		fmt.Fprintf(os.Stderr, "\033[32m✓\033[0m milestone structure valid (%d milestone(s) scanned, no polish/follow-up patterns)\n", len(milestones))
	}

	// Interactive milestone selection when stdin is a terminal and no --from/--to
	if cfg.From == "" && cfg.To == "" && isTerminal(os.Stdin) {
		selectedFrom, selectedTo, err := interactiveMilestoneSelect(inRange)
		if err != nil {
			return err
		}
		cfg.From = selectedFrom
		cfg.To = selectedTo
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (single-feature) — %s\033[0m\n", cfg.Feature)
		fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s\033[0m\n", cfg.Tool, cfg.Policy)
		if cfg.From != "" || cfg.To != "" {
			fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", cfg.From, cfg.To)
		}
		fmt.Fprintf(os.Stderr, "\n\033[1mMilestones:\033[0m\n")
		for _, m := range inRange {
			status := "pending"
			if milestoneAllVerified(m) {
				status = "verified"
			} else if milestoneAllDone(m) {
				status = "done"
			} else if !milestoneNotStarted(m) {
				status = "in progress"
			}
			fmt.Fprintf(os.Stderr, "  • %s — %s [%s]\n", m.ID, m.Name, status)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// Check if any milestones have explicit dependencies
	hasExplicitDeps := false
	for _, m := range inRange {
		if len(m.Deps) > 0 {
			hasExplicitDeps = true
			break
		}
	}

	if hasExplicitDeps {
		return runAutoParallel(cfg, inRange)
	}

	return runLoop(cfg)
}

// resolveFeatureSlugs resolves the list of feature slugs for multi-feature mode.
func resolveFeatureSlugs(root, featuresFlag string, allFlag bool) ([]string, error) {
	featuresDir := filepath.Join(root, ".belmont", "features")

	if allFlag {
		// Get all features, filter to pending ones
		features := listFeatures(featuresDir, 50)
		if len(features) == 0 {
			return nil, fmt.Errorf("auto: no features found in %s", featuresDir)
		}
		// Sync computed feature statuses back to master PROGRESS.md to fix drift
		syncMasterFeatureStatuses(root, features)
		var slugs []string
		for _, f := range features {
			// Skip features that are already done (Complete/Verified) or
			// archived via /belmont:cleanup. Only active/pending work runs.
			if isFeatureTerminal(f.Status) {
				continue
			}
			slugs = append(slugs, f.Slug)
		}
		if len(slugs) == 0 {
			return nil, fmt.Errorf("auto: no pending features — all are complete, verified, or archived")
		}
		// `--all` has no caller-supplied order to preserve; use a stable
		// alphabetical contract so scheduler output is deterministic.
		// (`computeFeatureWaves` honours input order, so we sort here.)
		sort.Strings(slugs)
		return slugs, nil
	}

	// Parse comma-separated slugs
	parts := strings.Split(featuresFlag, ",")
	var slugs []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		// Verify feature directory exists
		featureDir := filepath.Join(featuresDir, s)
		if !dirExists(featureDir) {
			return nil, fmt.Errorf("auto: feature %q not found at %s", s, featureDir)
		}
		slugs = append(slugs, s)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("auto: no valid feature slugs provided")
	}
	return slugs, nil
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

// scanReadiness reports requested features whose declared deps are not yet
// terminal (Complete/Verified/Archived). Uses the same `isFeatureTerminal`
// predicate as the scheduler so messaging cannot drift. Pure: no I/O.
func scanReadiness(features []featureSummary) []readinessWarning {
	bySlug := make(map[string]featureSummary, len(features))
	for _, f := range features {
		bySlug[f.Slug] = f
	}
	var warnings []readinessWarning
	for _, f := range features {
		for _, dep := range f.Deps {
			df, ok := bySlug[dep]
			if !ok || isFeatureTerminal(df.Status) {
				continue
			}
			warnings = append(warnings, readinessWarning{
				Slug:      f.Slug,
				DepSlug:   dep,
				DepStatus: df.Status,
				Blocked:   df.TasksBlocked,
			})
		}
	}
	return warnings
}

// skipResult records a feature skipped from a wave because one of its
// declared dependencies is in a non-runnable state. `Reason` is "paused" or
// "failed"; `DepSlug` is the first matching dep in declaration order.
type skipResult struct {
	Slug    string
	DepSlug string
	Reason  string // "paused" | "failed"
}

// filterWaveByBlocked partitions a wave into runnable features and skipped
// features based on which dep sets are populated. A feature is skipped on
// the first dep it has that appears in `failed` or `paused` (deps scanned in
// declaration order; failed wins over paused so the skip message reflects
// the harder failure when both apply). Pure: no I/O.
func filterWaveByBlocked(wave []featureSummary, failed, paused map[string]bool) (runnable []featureSummary, skipped []skipResult) {
	for _, f := range wave {
		var blockedBy *skipResult
		for _, dep := range f.Deps {
			if failed[dep] {
				blockedBy = &skipResult{Slug: f.Slug, DepSlug: dep, Reason: "failed"}
				break
			}
			if paused[dep] && blockedBy == nil {
				blockedBy = &skipResult{Slug: f.Slug, DepSlug: dep, Reason: "paused"}
				// Don't break — keep scanning in case a later dep is failed,
				// which should win for messaging purposes.
			}
		}
		if blockedBy != nil {
			skipped = append(skipped, *blockedBy)
		} else {
			runnable = append(runnable, f)
		}
	}
	return runnable, skipped
}

// computeFeatureWaves groups features into waves using Kahn's algorithm for topological sort.
// Features in the same wave have all deps satisfied by prior waves.
// Already-complete features satisfy deps but don't execute.
//
// Ordering contract: caller-supplied input order wins. Sibling features in the
// same wave appear in the order they appear in `features`. Callers that want
// alphabetical ordering (e.g. `auto --all`) must sort `features` themselves.
func computeFeatureWaves(features []featureSummary) ([]featureWave, error) {
	if len(features) == 0 {
		return nil, nil
	}

	// Build slug -> feature map
	bySlug := make(map[string]featureSummary)
	for _, f := range features {
		bySlug[f.Slug] = f
	}

	// Compute in-degree for each non-terminal feature. Complete/Verified/
	// Archived features don't execute but still satisfy downstream deps.
	inDegree := make(map[string]int)
	for _, f := range features {
		if isFeatureTerminal(f.Status) {
			continue
		}
		count := 0
		for _, dep := range f.Deps {
			if df, ok := bySlug[dep]; ok && !isFeatureTerminal(df.Status) {
				count++
			}
		}
		inDegree[f.Slug] = count
	}

	var waves []featureWave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all features with zero in-degree, scanning the input slice so
		// caller-supplied order is preserved within the wave.
		var ready []featureSummary
		for _, f := range features {
			deg, tracked := inDegree[f.Slug]
			if !tracked || deg != 0 {
				continue
			}
			ready = append(ready, bySlug[f.Slug])
		}

		if len(ready) == 0 {
			var cycleIDs []string
			for slug := range inDegree {
				cycleIDs = append(cycleIDs, slug)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among features: %s", strings.Join(cycleIDs, ", "))
		}

		waves = append(waves, featureWave{Index: waveIdx, Features: ready})
		waveIdx++

		// Remove completed features and update in-degrees
		for _, f := range ready {
			delete(inDegree, f.Slug)
			remaining--
		}
		for slug, deg := range inDegree {
			f := bySlug[slug]
			newDeg := deg
			for _, dep := range f.Deps {
				for _, completed := range ready {
					if dep == completed.Slug {
						newDeg--
					}
				}
			}
			inDegree[slug] = newDeg
		}
	}

	return waves, nil
}

// validateFeatureDeps checks for dangling dependency references and cycles.
func validateFeatureDeps(features []featureSummary, allKnown []featureSummary) error {
	// Build slug set from ALL known features so completed deps are recognized
	slugSet := make(map[string]bool)
	for _, f := range allKnown {
		slugSet[f.Slug] = true
	}

	// Check for dangling references — collect all errors
	var errs []string
	for _, f := range features {
		for _, dep := range f.Deps {
			if !slugSet[dep] {
				errs = append(errs, fmt.Sprintf("feature %q depends on %q which does not exist", f.Slug, dep))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	// Check for cycles by attempting wave computation
	_, err := computeFeatureWaves(features)
	return err
}

// runAutoMultiFeature orchestrates wave-based execution of multiple features.
// Features with dependencies execute after their dependencies complete.
func runAutoMultiFeature(cfg loopConfig, slugs []string) error {
	startTime := time.Now()

	// Pre-flight: ensure repo is in a clean state before starting
	if err := validateRepoState(cfg.Root); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Record original branch and restore on exit if changed
	origBranch := getCurrentBranch(cfg.Root)
	defer func() {
		if origBranch != "" && origBranch != "HEAD" {
			if cur := getCurrentBranch(cfg.Root); cur != origBranch {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ Branch changed from %s to %s — restoring...\033[0m\n", origBranch, cur)
				restoreCmd := exec.Command("git", "checkout", origBranch)
				restoreCmd.Dir = cfg.Root
				restoreCmd.Run()
			}
		}
	}()

	// Ensure the sibling worktree base directory exists
	os.MkdirAll(worktreeBasePath(cfg.Root), 0755)

	// Build feature summaries with dependency info
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	allFeatures := listFeatures(featuresDir, 50)
	populateFeatureDeps(allFeatures, cfg.Root)

	// Filter to requested slugs, preserving caller-supplied (CLI) order.
	// `allFeatures` is in directory-listing order (effectively alphabetical),
	// so we look up by slug to keep `--features=A,B,C` order intact for the
	// scheduler's sibling tie-break.
	bySlug := make(map[string]featureSummary, len(allFeatures))
	for _, f := range allFeatures {
		bySlug[f.Slug] = f
	}
	var features []featureSummary
	for _, slug := range slugs {
		if f, ok := bySlug[slug]; ok {
			features = append(features, f)
		}
	}

	// Validate dependencies
	if err := validateFeatureDeps(features, allFeatures); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Pre-flight readiness scan: warn (don't abort) on any requested feature
	// whose dep is not yet terminal. Operator can Ctrl-C before launch.
	if warns := scanReadiness(features); len(warns) > 0 {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Readiness check:\033[0m\n")
		for _, w := range warns {
			suffix := ""
			if w.Blocked > 0 {
				if w.Blocked == 1 {
					suffix = ", 1 blocked task"
				} else {
					suffix = fmt.Sprintf(", %d blocked tasks", w.Blocked)
				}
			}
			fmt.Fprintf(os.Stderr, "  • %s depends on %s (status: %s%s)\n", w.Slug, w.DepSlug, w.DepStatus, suffix)
		}
	}

	// Check if any features have deps — if not, use flat parallel (original behavior)
	hasAnyDeps := false
	for _, f := range features {
		if len(f.Deps) > 0 {
			hasAnyDeps = true
			break
		}
	}

	// Compute waves
	waves, err := computeFeatureWaves(features)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if !hasAnyDeps {
		// No dependencies — single wave with all features (original behavior)
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features\033[0m\n", len(slugs))
	} else {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features in %d waves\033[0m\n", len(slugs), len(waves))
	}
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	// Print wave execution plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var names []string
		for _, f := range w.Features {
			names = append(names, f.Slug)
		}
		if len(waves) == 1 {
			for _, n := range names {
				fmt.Fprintf(os.Stderr, "  • %s\n", n)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  Wave %d: [%s]\n", w.Index+1, strings.Join(names, ", "))
		}
	}
	fmt.Fprintln(os.Stderr)

	if cfg.DryRun {
		return nil
	}

	// Set up worktree tracker and signal handler
	activeWorktrees := &worktreeTracker{
		root:    cfg.Root,
		mode:    "multi-feature",
		entries: make(map[string]worktreeEntry),
		hooks:   loadWorktreeHooks(cfg.Root),
	}
	sigCh := make(chan os.Signal, 1)
	notifySignals(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — preserving worktrees for resume...\033[0m\n")
		activeWorktrees.gracefulShutdown(cfg.Root)
		os.Exit(1)
	}()

	type featureResult struct {
		Slug         string
		Branch       string
		WorktreePath string
		Err          error
	}

	var allFailures []featureResult
	failedSlugs := make(map[string]bool)
	pausedSlugs := make(map[string]bool)
	totalMerged := 0

	// Execute wave by wave
	for _, w := range waves {
		// Partition the wave into runnable vs skipped using the failed/paused
		// dep sets. A skipped feature inherits its blocker's reason and is
		// added to the matching set so its own dependents cascade-skip in
		// later waves.
		waveFeatures, skipped := filterWaveByBlocked(w.Features, failedSlugs, pausedSlugs)
		for _, s := range skipped {
			if s.Reason == "failed" {
				fmt.Fprintf(os.Stderr, "\033[31m⊘ %s skipped\033[0m — dependency %s failed\n", s.Slug, s.DepSlug)
				failedSlugs[s.Slug] = true
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: fmt.Errorf("dependency %s failed", s.DepSlug)})
			} else {
				fmt.Fprintf(os.Stderr, "\033[33m⊘ %s skipped\033[0m — dependency %s paused\n", s.Slug, s.DepSlug)
				pausedSlugs[s.Slug] = true
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: fmt.Errorf("dependency %s paused", s.DepSlug)})
			}
		}

		if len(waveFeatures) == 0 {
			continue
		}

		if len(waves) > 1 {
			fmt.Fprintf(os.Stderr, "\n\033[1m── Wave %d ──\033[0m\n", w.Index+1)
		}

		// Serial path: when MaxParallel == 1, run each feature and merge it
		// inline before moving on. The next feature's worktree forks from a
		// main that includes the prior feature's merge, so implicit cross-
		// feature task deps (e.g. F2 calling into F1's new route) resolve at
		// the fork point instead of producing `[!]` blockers. Stale-worktree
		// resolution is deferred to just-in-time so each feature's rebase-on-
		// resume targets the post-prior-merge main rather than the pre-wave
		// main. See knowledge/auto-mode/multi-feature-scheduling.md.
		if cfg.MaxParallel <= 1 {
			for _, f := range waveFeatures {
				slug := f.Slug
				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(worktreeBasePath(cfg.Root), slug)

				resumed, err := handleStaleWorktree(cfg.Root, slug, branch, wtPath)
				if err != nil {
					return err
				}

				activeWorktrees.add(slug, wtPath, branch)
				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				runErr := runFeatureInWorktree(cfg, slug, branch, wtPath, activeWorktrees, resumed)
				if runErr != nil {
					if errors.Is(runErr, errFeaturePaused) {
						fmt.Fprintf(os.Stderr, "\033[33m⏸ %s paused\033[0m — has unresolved blockers\n", slug)
						pausedSlugs[slug] = true
						allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: runErr})
					} else {
						fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", slug, runErr)
						failedSlugs[slug] = true
						allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: runErr})
					}
					continue
				}

				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", slug)
				if err := ensureCleanMergeState(cfg.Root); err != nil {
					fmt.Fprintf(os.Stderr, "\033[33m⚠ %s — skipping merge\033[0m\n", err)
					allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: fmt.Errorf("skipped: unclean merge state")})
					failedSlugs[slug] = true
					continue
				}
				if err := mergeFeatureBranch(cfg, slug, branch, wtPath, activeWorktrees); err != nil {
					fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", slug, err)
					fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", wtPath)
					fmt.Fprintf(os.Stderr, "  Branch: %s\n", branch)
					allFailures = append(allFailures, featureResult{Slug: slug, Branch: branch, WorktreePath: wtPath, Err: err})
					failedSlugs[slug] = true
					continue
				}
				totalMerged++
			}
			continue
		}

		// Resolve stale worktrees sequentially (before parallel launch) to avoid stdin races
		preResolved := make(map[string]bool) // slug -> resumed
		for _, f := range waveFeatures {
			branch := fmt.Sprintf("belmont/auto/%s", f.Slug)
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), f.Slug)
			resumed, err := handleStaleWorktree(cfg.Root, f.Slug, branch, wtPath)
			if err != nil {
				return err
			}
			preResolved[f.Slug] = resumed
		}

		// Run this wave's features in parallel
		semaphore := make(chan struct{}, cfg.MaxParallel)
		var wg sync.WaitGroup
		results := make(chan featureResult, len(waveFeatures))

		for _, f := range waveFeatures {
			wg.Add(1)
			go func(slug string, resumed bool) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(worktreeBasePath(cfg.Root), slug)

				activeWorktrees.add(slug, wtPath, branch)

				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				err := runFeatureInWorktree(cfg, slug, branch, wtPath, activeWorktrees, resumed)
				results <- featureResult{
					Slug:         slug,
					Branch:       branch,
					WorktreePath: wtPath,
					Err:          err,
				}
			}(f.Slug, preResolved[f.Slug])
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect wave results
		var waveSuccesses []featureResult
		for r := range results {
			if r.Err != nil {
				if errors.Is(r.Err, errFeaturePaused) {
					fmt.Fprintf(os.Stderr, "\033[33m⏸ %s paused\033[0m — has unresolved blockers\n", r.Slug)
					// Track in pausedSlugs so dependents in later waves skip
					// with reason=paused (vs reason=failed). The feature is
					// not added to failedSlugs or waveSuccesses — its
					// worktree is preserved for the user to fix and resume.
					pausedSlugs[r.Slug] = true
					allFailures = append(allFailures, r)
				} else {
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", r.Slug, r.Err)
					allFailures = append(allFailures, r)
					failedSlugs[r.Slug] = true
				}
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", r.Slug)
				waveSuccesses = append(waveSuccesses, r)
			}
		}

		// Merge this wave's successes before proceeding to next wave
		// State is NOT committed before merge — only after successful merge.
		// This prevents "phantom completion" where state says complete but code never merged.
		for i, s := range waveSuccesses {
			// Ensure repo is in a clean merge state before each merge
			if err := ensureCleanMergeState(cfg.Root); err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ %s — skipping remaining %d merge(s)\033[0m\n", err, len(waveSuccesses)-i)
				for _, remaining := range waveSuccesses[i:] {
					allFailures = append(allFailures, featureResult{Slug: remaining.Slug, Err: fmt.Errorf("skipped: unclean merge state")})
					failedSlugs[remaining.Slug] = true
				}
				break
			}
			if err := mergeFeatureBranch(cfg, s.Slug, s.Branch, s.WorktreePath, activeWorktrees); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", s.Slug, err)
				fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", s.WorktreePath)
				fmt.Fprintf(os.Stderr, "  Branch: %s\n", s.Branch)
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: err})
				failedSlugs[s.Slug] = true
			} else {
				totalMerged++
			}
		}
	}

	// Sync master PROGRESS.md with actual feature states after all merges
	featuresDir = filepath.Join(cfg.Root, ".belmont", "features")
	syncMasterFeatureStatuses(cfg.Root, listFeatures(featuresDir, 50))

	// Commit any remaining .belmont/ state changes after all merges
	if err := commitBelmontState(cfg.Root); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to commit final .belmont/ state: %s\033[0m\n", err)
	}

	// Report
	if totalMerged == 0 && len(pausedSlugs) > 0 {
		// Halt summary: nothing merged and at least one feature paused.
		// Group paused features and dependents-skipped-due-to-paused so the
		// user sees the root cause instead of a flat "N failed" list.
		var paused, skipped []featureResult
		for _, f := range allFailures {
			if pausedSlugs[f.Slug] {
				// Distinguish "actually paused" (originated the pause) from
				// "skipped because dep paused" using the error message
				// shape — skip-due-to-pause uses "dependency X paused".
				if f.Err != nil && strings.HasPrefix(f.Err.Error(), "dependency ") {
					skipped = append(skipped, f)
				} else {
					paused = append(paused, f)
				}
			}
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⏸ %d feature(s) paused (no merges made):\033[0m\n", len(paused))
		for _, f := range paused {
			fmt.Fprintf(os.Stderr, "  %s\n", f.Slug)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
		if len(skipped) > 0 {
			fmt.Fprintf(os.Stderr, "\033[33m⊘ %d feature(s) skipped:\033[0m\n", len(skipped))
			for _, f := range skipped {
				fmt.Fprintf(os.Stderr, "  %s — %s\n", f.Slug, f.Err)
			}
		}
		fmt.Fprintf(os.Stderr, "Fix the blocker(s) and rerun.\n")
	} else if len(allFailures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d feature(s) failed:\033[0m\n", len(allFailures))
		for _, f := range allFailures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Slug, f.Err)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
	}

	// Clean up auto.json now that all features are processed
	activeWorktrees.removeAutoJSON()

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ %d/%d features complete\033[0m (%.1fs total)\n", totalMerged, len(slugs), time.Since(startTime).Seconds())

	if len(allFailures) > 0 {
		return fmt.Errorf("auto: %d feature(s) failed", len(allFailures))
	}
	return nil
}

// handleStaleWorktree checks for a stale branch/worktree from a previous interrupted run.
// Returns resumed=true if the existing worktree should be reused (skip creation).
// Returns resumed=false if stale state was cleaned up (proceed with fresh creation).
func handleStaleWorktree(root, id, branch, wtPath string) (resumed bool, err error) {
	// Check if branch already exists
	checkCmd := exec.Command("git", "branch", "--list", branch)
	checkCmd.Dir = root
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false, nil // no stale branch, proceed normally
	}

	// Stale branch exists — determine what to do
	_, wtDirErr := os.Stat(wtPath)
	wtExists := wtDirErr == nil

	if isTerminal(os.Stdin) {
		// Interactive: prompt the user
		status := "branch exists"
		if wtExists {
			status = "branch + worktree exist"
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Branch '%s' exists from a previous run (%s).\033[0m\n", branch, status)
		fmt.Fprintf(os.Stderr, "  [r] Resume from where it left off\n")
		fmt.Fprintf(os.Stderr, "  [s] Start fresh (delete branch and restart)\n")
		fmt.Fprintf(os.Stderr, "  [q] Quit\n")
		fmt.Fprintf(os.Stderr, "> ")

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "r", "resume":
			if wtExists {
				// Worktree still exists — reuse it directly
				fmt.Fprintf(os.Stderr, "  Resuming with existing worktree at %s\n", wtPath)
			} else {
				// Branch exists but worktree is gone — reattach
				fmt.Fprintf(os.Stderr, "  Reattaching worktree to existing branch %s\n", branch)
				wtDir := filepath.Dir(wtPath)
				if err := os.MkdirAll(wtDir, 0755); err != nil {
					return false, fmt.Errorf("create worktree dir: %w", err)
				}
				addCmd := exec.Command("git", "worktree", "add", wtPath, branch)
				addCmd.Dir = root
				if out, err := addCmd.CombinedOutput(); err != nil {
					return false, fmt.Errorf("git worktree add (resume): %w (%s)", err, strings.TrimSpace(string(out)))
				}
			}
			// Rebase the worktree onto current main so sibling features that
			// merged while this one was paused are picked up. Conflicts abort
			// and warn — never auto-resolve. See
			// knowledge/auto-mode/resume-rebase.md for the design.
			n, rebaseErr := rebaseWorktreeOnMain(root, wtPath)
			announceWorktreeRebase(id, n, rebaseErr)
			// Leave .belmont/ as-is in the worktree — it has committed state from the
			// previous run. Don't overwrite with stale copy from main repo.
			// If it's an old symlink from a previous version, replace with a fresh copy.
			dstBelmont := filepath.Join(wtPath, ".belmont")
			if fi, err := os.Lstat(dstBelmont); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				// Old-style symlink — replace with copy-based approach
				os.RemoveAll(dstBelmont)
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			} else if err != nil {
				// .belmont/ missing entirely — copy it in
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			}
			return true, nil

		case "q", "quit":
			return false, fmt.Errorf("user chose to quit")

		default: // "s", "start", or anything else → start fresh
			fmt.Fprintf(os.Stderr, "  Cleaning up stale state for %s...\n", id)
		}
	} else {
		// Non-interactive: auto-restart
		fmt.Fprintf(os.Stderr, "  Cleaning up stale branch '%s' from previous run...\n", branch)
	}

	// Clean up stale state (restart path)
	if wtExists {
		removeWorktree(root, wtPath, id)
	}
	// Prune any orphaned worktree references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = root
	pruneCmd.Run()
	// Delete the stale branch
	delCmd := exec.Command("git", "branch", "-D", branch)
	delCmd.Dir = root
	delCmd.Run()

	return false, nil
}

func printLoopState(report statusReport, hasFwlup bool) {
	done := report.TaskCounts["done"] + report.TaskCounts["verified"]
	total := report.TaskCounts["total"]
	msDone := countDoneMilestones(report.Milestones)
	msTotal := len(report.Milestones)

	// Progress bar
	barWidth := 20
	filled := 0
	if total > 0 {
		filled = (done * barWidth) / total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Fprintf(os.Stderr, "  [%s] %d/%d tasks, %d/%d milestones", bar, done, total, msDone, msTotal)

	if hasFwlup {
		fmt.Fprintf(os.Stderr, " \033[33m(FWLUP)\033[0m")
	}
	blockedCount := blockedTaskCount(report.Milestones)
	if blockedCount > 0 {
		fmt.Fprintf(os.Stderr, " \033[31m(%d blocked)\033[0m", blockedCount)
	}
	fmt.Fprintln(os.Stderr)
}

func milestonesInRange(milestones []milestone, from, to string) []milestone {
	if from == "" && to == "" {
		return milestones
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	var result []milestone
	for _, m := range milestones {
		num := parseMilestoneNum(m.ID)
		if num < 0 {
			continue
		}
		if fromNum >= 0 && num < fromNum {
			continue
		}
		if toNum >= 0 && num > toNum {
			continue
		}
		result = append(result, m)
	}
	return result
}

func parseMilestoneNum(id string) int {
	if id == "" {
		return -1
	}
	re := regexp.MustCompile(`(?i)M(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) < 2 {
		return -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return n
}

// tailWriter writes all data to an underlying writer and keeps a rolling
// buffer of the last `size` bytes for later retrieval.
type tailWriter struct {
	out     io.Writer
	buf     []byte
	size    int
	prefix  string
	lineBuf []byte // partial line accumulator (used when prefix is set)
}

func newTailWriter(out io.Writer, size int, prefix string) *tailWriter {
	return &tailWriter{out: out, buf: make([]byte, 0, size), size: size, prefix: prefix}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	// Always store raw bytes in buf for error tail reporting
	tw.buf = append(tw.buf, p...)
	if len(tw.buf) > tw.size {
		tw.buf = tw.buf[len(tw.buf)-tw.size:]
	}

	// When no prefix, pass through directly
	if tw.prefix == "" {
		_, err := tw.out.Write(p)
		return len(p), err
	}

	// Line-buffer and prepend prefix to each complete line
	tw.lineBuf = append(tw.lineBuf, p...)
	for {
		idx := bytes.IndexByte(tw.lineBuf, '\n')
		if idx < 0 {
			break
		}
		line := tw.lineBuf[:idx]
		tw.lineBuf = tw.lineBuf[idx+1:]
		tw.out.Write([]byte(tw.prefix + string(line) + "\n"))
	}
	return len(p), nil
}

func (tw *tailWriter) String() string {
	return string(tw.buf)
}

// claudeStreamWriter wraps a tailWriter and parses Claude stream-json NDJSON,
// extracting only human-readable content (assistant text + tool use indicators).
type claudeStreamWriter struct {
	tw      *tailWriter
	partial []byte
	prefix  string // e.g. "\033[36m[slug]\033[0m: "
}

type streamLine struct {
	Type    string        `json:"type"`
	Message streamMessage `json:"message"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

func (c *claudeStreamWriter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		idx := bytes.IndexByte(c.partial, '\n')
		if idx < 0 {
			break
		}
		line := c.partial[:idx]
		c.partial = c.partial[idx+1:]
		if len(line) == 0 {
			continue
		}
		var sl streamLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue
		}
		if sl.Type != "assistant" {
			continue
		}
		for _, item := range sl.Message.Content {
			switch item.Type {
			case "text":
				if item.Text != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + item.Text + "\n"))
					} else {
						c.tw.Write([]byte("  " + item.Text + "\n"))
					}
				}
			case "tool_use":
				if item.Name != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + toolSummary(item.Name, item.Input) + "\n"))
					} else {
						c.tw.Write([]byte("  → " + toolSummary(item.Name, item.Input) + "\n"))
					}
				}
			}
		}
	}
	return len(p), nil
}

func shortActionLabel(t loopActionType) string {
	switch t {
	case actionImplementMilestone:
		return "IMPLEMENT"
	case actionImplementNext:
		return "FIX"
	case actionVerify:
		return "VERIFY"
	case actionReplan:
		return "REPLAN"
	case actionSkipMilestone:
		return "SKIP"
	case actionDebug:
		return "DEBUG"
	case actionTriage:
		return "TRIAGE"
	case actionFixAll:
		return "FIX-ALL"
	default:
		return string(t)
	}
}

func describeMilestone(action *loopAction, report statusReport) string {
	if action.MilestoneID != "" {
		for _, m := range report.Milestones {
			if m.ID == action.MilestoneID {
				return m.ID + ": " + m.Name
			}
		}
	}
	if action.Type == actionVerify || action.Type == actionImplementNext {
		if report.NextMilestone != nil {
			return report.NextMilestone.ID + ": " + report.NextMilestone.Name
		}
	}
	return ""
}

func lastActionType(history []historyEntry) loopActionType {
	if len(history) == 0 {
		return ""
	}
	return history[len(history)-1].Action.Type
}

func consecutiveFailures(history []historyEntry) int {
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Result != nil && !history[i].Result.Success {
			count++
		} else {
			break
		}
	}
	return count
}

func isLoopStuck(history []historyEntry) bool {
	if len(history) < 2 {
		return false
	}
	recent := history[len(history)-2:]
	// Both must have succeeded
	for _, e := range recent {
		if e.Result == nil || !e.Result.Success {
			return false
		}
	}
	// Compare state fingerprints
	fp0 := loopFingerprint(recent[0])
	fp1 := loopFingerprint(recent[1])
	return fp0 == fp1
}

func loopFingerprint(e historyEntry) string {
	return fmt.Sprintf("%d/%d|%d/%d|%d|%v|%s", e.TasksDone, e.TasksTotal, e.MsDone, e.MsTotal, e.BlockerCount, e.HasFwlup, e.PostGitSHA)
}

func countDoneMilestones(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		if milestoneAllDone(m) {
			count++
		}
	}
	return count
}

// captureGitSHA returns the current HEAD SHA, or "" on error.
func captureGitSHA(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// classifyChanges runs git diff between preSHA and HEAD and classifies the work type.
func classifyChanges(root, preSHA string) (workType, int) {
	if preSHA == "" {
		return workUnknown, 0
	}
	cmd := exec.Command("git", "diff", "--name-only", preSHA+"..HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return workUnknown, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return workMinimal, 0
	}
	if len(files) < 3 {
		return workMinimal, len(files)
	}

	frontendExts := map[string]bool{
		".tsx": true, ".jsx": true, ".css": true, ".scss": true,
		".html": true, ".vue": true, ".svelte": true, ".less": true,
	}
	backendExts := map[string]bool{
		".go": true, ".py": true, ".rs": true, ".java": true,
		".rb": true, ".php": true, ".cs": true, ".kt": true,
		".scala": true, ".ex": true, ".exs": true,
	}
	configExts := map[string]bool{
		".yml": true, ".yaml": true, ".json": true, ".toml": true,
		".ini": true, ".env": true,
	}
	docExts := map[string]bool{
		".md": true, ".txt": true, ".rst": true,
	}

	var feCount, beCount, cfgCount, docCount int
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		switch {
		case frontendExts[ext]:
			feCount++
		case backendExts[ext]:
			beCount++
		case configExts[ext]:
			cfgCount++
		case docExts[ext]:
			docCount++
		}
	}

	total := len(files)
	if docCount == total {
		return workDocs, total
	}
	if cfgCount == total {
		return workConfig, total
	}
	if feCount*2 > total {
		return workFrontend, total
	}
	if beCount*2 > total {
		return workBackend, total
	}
	if feCount > 0 && beCount > 0 {
		return workMixed, total
	}
	if feCount > 0 {
		return workFrontend, total
	}
	if beCount > 0 {
		return workBackend, total
	}
	return workMixed, total
}

// buildMilestoneLoopStates derives per-milestone state from the loop history.
func buildMilestoneLoopStates(history []historyEntry, milestones []milestone) map[string]*milestoneLoopState {
	states := make(map[string]*milestoneLoopState)
	for _, m := range milestones {
		states[m.ID] = &milestoneLoopState{
			ID:   m.ID,
			Name: m.Name,
			Done: milestoneAllDone(m),
		}
	}

	var lastImplementedMS string
	for _, h := range history {
		switch h.Action.Type {
		case actionImplementMilestone:
			msID := h.Action.MilestoneID
			if msID != "" {
				if s, ok := states[msID]; ok && h.Result != nil && h.Result.Success {
					s.Implemented = true
					s.WorkType = h.WorkType
					s.FilesChanged = h.FilesChanged
					lastImplementedMS = msID
				}
			}
		case actionVerify:
			// Attribute verification to the most recently implemented milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					if h.Result != nil {
						if h.Result.Success {
							s.Verified = true
							s.VerifySucceeded++
						} else {
							s.VerifyFailed++
						}
					}
				}
			}
		case actionTriage:
			// Track triage rounds per milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					s.FwlupFixRounds++
				}
			}
		}
	}
	return states
}

// lastMilestoneID walks backward through history and returns the most recent non-empty MilestoneID.
func lastMilestoneID(history []historyEntry) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Action.MilestoneID != "" {
			return history[i].Action.MilestoneID
		}
	}
	return ""
}

// fencedJSONBodies returns the contents of every fenced code block in s,
// in document order. Recognises ```json ... ```, ```JSON ... ``` and
// language-less ``` ... ``` fences. Anything else (e.g. ```bash) is skipped.
func fencedJSONBodies(s string) []string {
	var out []string
	rest := s
	for {
		open := strings.Index(rest, "```")
		if open == -1 {
			return out
		}
		after := rest[open+3:]
		// Optional language tag through to the next newline.
		nl := strings.IndexByte(after, '\n')
		if nl == -1 {
			return out
		}
		lang := strings.TrimSpace(after[:nl])
		body := after[nl+1:]
		close := strings.Index(body, "```")
		if close == -1 {
			return out
		}
		if lang == "" || strings.EqualFold(lang, "json") {
			out = append(out, body[:close])
		}
		rest = body[close+3:]
	}
}

// firstBalancedJSONObjectWithAction scans s for the first `{ ... }` whose
// braces balance and which contains an "action" key. Strings (including
// escapes) are tracked so braces inside string literals don't throw the
// depth count off.
func firstBalancedJSONObjectWithAction(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		end := matchBalancedBrace(s, i)
		if end == -1 {
			continue
		}
		candidate := s[i : end+1]
		if !strings.Contains(candidate, `"action"`) {
			continue
		}
		var probe map[string]any
		if err := json.Unmarshal([]byte(candidate), &probe); err != nil {
			continue
		}
		if _, ok := probe["action"]; ok {
			return candidate
		}
	}
	return ""
}

// matchBalancedBrace returns the index of the `}` that closes the `{` at
// start, or -1 if there is no balanced match. Tracks string state so braces
// inside JSON string literals don't affect depth.
func matchBalancedBrace(s string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

type triageDecision struct {
	Decision      string   `json:"decision"`
	BlockingTasks []string `json:"blocking_tasks"`
	DeferredTasks []string `json:"deferred_tasks"`
	Reason        string   `json:"reason"`
	ReverifyScope string   `json:"reverify_scope"`
}

// parseTriageDecision extracts the triage JSON decision from the tool output.
func parseTriageDecision(output string) *triageDecision {
	// Find JSON object with "decision" field
	re := regexp.MustCompile(`\{[^{}]*"decision"\s*:\s*"[^"]+?"[^{}]*\}`)
	match := re.FindString(output)
	if match == "" {
		// Try to find it in the last 2000 chars (triage outputs it at the end)
		tail := output
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		match = re.FindString(tail)
		if match == "" {
			return nil
		}
	}
	var td triageDecision
	if err := json.Unmarshal([]byte(match), &td); err != nil {
		return nil
	}
	if td.Decision == "" {
		return nil
	}
	return &td
}

// truncateTail returns the last maxLen characters of s.
func truncateTail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// skipMilestoneInProgress marks all incomplete tasks in a milestone as done in PROGRESS.md.
func skipMilestoneInProgress(root, feature, milestoneID string) error {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("read PROGRESS.md: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	taskRe := regexp.MustCompile(`^(\s*-\s+)\[[ >!]\](\s+.*)$`)

	inTarget := false
	changed := false
	for i, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			inTarget = ("M" + m[1]) == milestoneID
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inTarget = false
			continue
		}
		if inTarget {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				lines[i] = taskMatch[1] + "[x]" + taskMatch[2]
				changed = true
			}
		}
	}

	if !changed {
		return fmt.Errorf("milestone %s not found or already done", milestoneID)
	}

	return os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
}

func detectFwlupTasks(root, feature string, report statusReport) bool {
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	// Check if any todo/in_progress tasks have FWLUP in their ID or name
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				return true
			}
		}
	}
	return false
}

// extractMilestoneFromTaskID extracts the milestone ID from a task ID like "P5-M5-FWLUP-1" → "M5".
func extractMilestoneFromTaskID(taskID string) string {
	re := regexp.MustCompile(`P\d+-M(\d+)`)
	m := re.FindStringSubmatch(taskID)
	if len(m) >= 2 {
		return "M" + m[1]
	}
	return ""
}

// detectFwlupTasksForMilestone checks for pending FWLUP tasks scoped to a specific milestone.
func detectFwlupTasksForMilestone(root, feature string, report statusReport, milestoneID string) bool {
	if milestoneID == "" {
		return false
	}
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				// Tasks now carry their milestone ID from PROGRESS.md
				if t.MilestoneID == milestoneID || extractMilestoneFromTaskID(t.ID) == milestoneID {
					return true
				}
			}
		}
	}
	return false
}

// pendingTasksInRange checks for incomplete tasks under milestones
// that fall within the from/to range in the feature's PROGRESS.md.
// When from and to are both empty, falls back to checking all milestones.
func pendingTasksInRange(root, feature, from, to string) bool {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task: [ ], [>], [!]
	taskRe := regexp.MustCompile(`^\s*-\s+\[[ >!]\]`)

	inRange := fromNum < 0 && toNum < 0 // if no range, all milestones are in range
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && taskRe.MatchString(line) {
			return true
		}
	}
	return false
}

// fwlupTasksInRange checks for unchecked FWLUP tasks under milestones within the from/to range.
// When from and to are both empty, falls back to the global detectFwlupTasks.
func fwlupTasksInRange(root, feature string, report statusReport, from, to string) bool {
	if from == "" && to == "" {
		return detectFwlupTasks(root, feature, report)
	}

	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task with FWLUP in the text
	fwlupTaskRe := regexp.MustCompile(`(?i)^\s*-\s+\[[ >!]\].*FWLUP`)

	inRange := false
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && fwlupTaskRe.MatchString(line) {
			return true
		}
	}
	return false
}

// loadPromptTemplate loads a prompt template from embedded FS or source filesystem.
func loadPromptTemplate(name string) (*template.Template, error) {
	filename := name + ".md"

	// Try embedded first
	if hasEmbeddedFiles {
		data, err := fs.ReadFile(embeddedPrompts, filepath.Join("prompts", "belmont", filename))
		if err == nil {
			return template.New(name).Parse(string(data))
		}
	}

	// Try source resolution
	sourceRoot := resolveSourceForPrompts()
	if sourceRoot == "" {
		return nil, fmt.Errorf("prompt %q: no embedded files and no source directory found", name)
	}

	data, err := os.ReadFile(filepath.Join(sourceRoot, "prompts", "belmont", filename))
	if err != nil {
		return nil, fmt.Errorf("prompt %q: %w", name, err)
	}
	return template.New(name).Parse(string(data))
}

// resolveSourceForPrompts returns the belmont source directory path, or "" if not found.
func resolveSourceForPrompts() string {
	if src := os.Getenv("BELMONT_SOURCE"); src != "" {
		return src
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		configPath := filepath.Join(configDir, "belmont", "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var cfg config
			if json.Unmarshal(data, &cfg) == nil && cfg.Source != "" {
				return cfg.Source
			}
		}
	}

	// Walk up from binary location
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for {
		if _, err := os.Stat(filepath.Join(dir, "prompts", "belmont")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// isTerminal returns true if the given file is a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveMilestoneSelect shows milestones and lets user pick a range.
func interactiveMilestoneSelect(milestones []milestone) (from, to string, err error) {
	if len(milestones) == 0 {
		return "", "", nil
	}

	fmt.Fprintf(os.Stderr, "\033[1mMilestones:\033[0m\n")
	firstUndone := ""
	for _, m := range milestones {
		marker := "[ ]"
		if milestoneAllDone(m) {
			marker = "[x]"
		}
		if !milestoneAllDone(m) && firstUndone == "" {
			firstUndone = m.ID
		}
		depStr := ""
		if len(m.Deps) > 0 {
			depStr = fmt.Sprintf(" \033[2m(depends: %s)\033[0m", strings.Join(m.Deps, ", "))
		}
		fmt.Fprintf(os.Stderr, "  %s %s: %s%s\n", marker, m.ID, m.Name, depStr)
	}

	lastID := milestones[len(milestones)-1].ID
	defaultRange := ""
	if firstUndone != "" {
		defaultRange = fmt.Sprintf("%s → %s", firstUndone, lastID)
	}

	fmt.Fprintf(os.Stderr, "\n\033[2mDefault range: %s\033[0m\n", defaultRange)
	fmt.Fprintf(os.Stderr, "Press Enter to accept, 'q' to quit, or enter range (e.g. M2 M5): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", "", fmt.Errorf("auto: no input")
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "q" || input == "quit" || input == "exit" {
		return "", "", fmt.Errorf("auto: cancelled by user")
	}

	if input == "" {
		// Accept defaults
		return "", "", nil
	}

	// Parse custom range
	parts := strings.Fields(input)
	if len(parts) == 1 {
		return parts[0], parts[0], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("auto: invalid range %q — use 'M2 M5' format", input)
}

// wave represents a group of milestones that can execute in parallel.
type wave struct {
	Index      int
	Milestones []milestone
}

// computeWaves groups milestones into waves using Kahn's algorithm for topological sort.
// Milestones in the same wave have all deps satisfied by prior waves.
// Already-done milestones satisfy deps but don't execute.
func computeWaves(milestones []milestone) ([]wave, error) {
	if len(milestones) == 0 {
		return nil, nil
	}

	// Build ID -> milestone map
	byID := make(map[string]milestone)
	for _, m := range milestones {
		byID[m.ID] = m
	}

	// Compute in-degree for each undone milestone
	inDegree := make(map[string]int)
	for _, m := range milestones {
		if milestoneAllDone(m) {
			continue
		}
		count := 0
		for _, dep := range m.Deps {
			if dm, ok := byID[dep]; ok && !milestoneAllDone(dm) {
				count++
			}
		}
		inDegree[m.ID] = count
	}

	var waves []wave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all milestones with zero in-degree
		var ready []milestone
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, byID[id])
			}
		}

		if len(ready) == 0 {
			// Cycle detected
			var cycleIDs []string
			for id := range inDegree {
				cycleIDs = append(cycleIDs, id)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among milestones: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready milestones by ID for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return parseMilestoneNum(ready[i].ID) < parseMilestoneNum(ready[j].ID)
		})

		waves = append(waves, wave{Index: waveIdx, Milestones: ready})
		waveIdx++

		// Remove completed milestones and update in-degrees
		for _, m := range ready {
			delete(inDegree, m.ID)
			remaining--
		}
		for id, deg := range inDegree {
			m := byID[id]
			newDeg := deg
			for _, dep := range m.Deps {
				for _, completed := range ready {
					if dep == completed.ID {
						newDeg--
					}
				}
			}
			inDegree[id] = newDeg
		}
	}

	return waves, nil
}

// worktreeEntry stores both the path and branch name for a worktree.
type worktreeEntry struct {
	Path   string
	Branch string
	Port   int
	Pgid   int // process group ID for cleanup
}

// worktreeTracker keeps track of active worktrees for cleanup on interrupt.
type worktreeTracker struct {
	mu      sync.Mutex
	root    string                   // project root for persisting auto.json
	feature string                   // feature slug for single-feature parallel mode (empty in multi-feature mode)
	mode    string                   // "single-feature-parallel" | "multi-feature" (used by live-status readers)
	entries map[string]worktreeEntry // ID -> worktree entry (milestone IDs in single-feature, feature slugs in multi-feature)
	hooks   *worktreeHooks           // shared hooks config (nil if no worktree.json)
}

// autoJSON is the on-disk format for .belmont/auto.json, enabling belmont status
// to discover active worktrees and read live feature state from them.
type autoJSON struct {
	Active    bool                     `json:"active"`
	Started   string                   `json:"started"`
	Mode      string                   `json:"mode,omitempty"`    // "single-feature" or "parallel" or "multi-feature"
	Feature   string                   `json:"feature,omitempty"` // active feature slug (single-feature mode)
	From      string                   `json:"from,omitempty"`    // milestone range start
	To        string                   `json:"to,omitempty"`      // milestone range end
	Worktrees map[string]autoJSONEntry `json:"worktrees"`
}

type autoJSONEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

func (wt *worktreeTracker) add(id, path, branch string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.entries[id] = worktreeEntry{Path: path, Branch: branch}
	wt.persistAutoJSON()
}

func (wt *worktreeTracker) setPort(id string, port int) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if entry, ok := wt.entries[id]; ok {
		entry.Port = port
		wt.entries[id] = entry
	}
}

func (wt *worktreeTracker) setPgid(id string, pgid int) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if entry, ok := wt.entries[id]; ok {
		entry.Pgid = pgid
		wt.entries[id] = entry
	}
}

func (wt *worktreeTracker) remove(id string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	delete(wt.entries, id)
	wt.persistAutoJSON()
}

// persistAutoJSON writes .belmont/auto.json so belmont status can discover active worktrees.
// Must be called with wt.mu held.
func (wt *worktreeTracker) persistAutoJSON() {
	if wt.root == "" {
		return
	}
	aj := autoJSON{
		Active:    len(wt.entries) > 0,
		Started:   time.Now().UTC().Format(time.RFC3339),
		Mode:      wt.mode,
		Feature:   wt.feature,
		Worktrees: make(map[string]autoJSONEntry),
	}
	for id, entry := range wt.entries {
		aj.Worktrees[id] = autoJSONEntry{Path: entry.Path, Branch: entry.Branch}
	}
	data, err := json.MarshalIndent(aj, "", "  ")
	if err != nil {
		return
	}
	autoPath := filepath.Join(wt.root, ".belmont", "auto.json")
	os.WriteFile(autoPath, data, 0644) // best-effort
}

// writeLoopAutoJSON writes a minimal auto.json for single-feature runLoop mode,
// enabling belmont status to show what's being worked on.
func writeLoopAutoJSON(autoPath string, cfg loopConfig) {
	aj := autoJSON{
		Active:    true,
		Started:   time.Now().UTC().Format(time.RFC3339),
		Mode:      "single-feature",
		Feature:   cfg.Feature,
		From:      cfg.From,
		To:        cfg.To,
		Worktrees: make(map[string]autoJSONEntry),
	}
	data, err := json.MarshalIndent(aj, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(autoPath, data, 0644) // best-effort
}

// removeAutoJSON deletes .belmont/auto.json when auto is done.
func (wt *worktreeTracker) removeAutoJSON() {
	if wt.root == "" {
		return
	}
	os.Remove(filepath.Join(wt.root, ".belmont", "auto.json"))
}

// teardownEntry runs teardown hooks for a worktree entry (if configured).
func (wt *worktreeTracker) teardownEntry(id string) {
	wt.mu.Lock()
	entry, ok := wt.entries[id]
	hooks := wt.hooks
	wt.mu.Unlock()
	if !ok {
		return
	}
	if entry.Pgid != 0 {
		signalProcessGroup(entry.Pgid)
	}
	if hooks != nil && len(hooks.Teardown) > 0 {
		// Teardown runs without monorepo env vars; teardown commands are
		// typically simple (kill servers, remove volumes) and don't need
		// workspace context. Passing nil keeps the call site lean.
		_ = runWorktreeHookCommands(hooks.Teardown, entry.Path, entry.Port, hooks.Env, nil, "", monorepoNone)
	}
}

// gracefulShutdown stops processes and releases resources but preserves worktrees
// and branches so the user can resume on next run.
func (wt *worktreeTracker) gracefulShutdown(root string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	for id, entry := range wt.entries {
		// Kill process group if running
		if entry.Pgid != 0 {
			signalProcessGroup(entry.Pgid)
		}
		// Run teardown hooks (release ports, stop dev servers)
		if wt.hooks != nil && len(wt.hooks.Teardown) > 0 {
			_ = runWorktreeHookCommands(wt.hooks.Teardown, entry.Path, entry.Port, wt.hooks.Env, nil, "", monorepoNone)
		}
		// Preserve worktree and branch for resume
		recoverSlug := filepath.Base(entry.Path)
		fmt.Fprintf(os.Stderr, "  Worktree preserved for %s at %s\n", id, entry.Path)
		fmt.Fprintf(os.Stderr, "    Resume with: belmont auto (will prompt to resume)\n")
		fmt.Fprintf(os.Stderr, "    Or clean up: belmont recover --clean %s\n", recoverSlug)
	}
	wt.entries = make(map[string]worktreeEntry)
}

// verifyNoConflictMarkers scans resolved files for leftover conflict markers.
func verifyNoConflictMarkers(root string, files []string) error {
	markers := []string{"<<<<<<<", "=======", ">>>>>>>"}
	var badFiles []string

	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		content := string(data)
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				badFiles = append(badFiles, file)
				break
			}
		}
	}

	if len(badFiles) > 0 {
		return fmt.Errorf("conflict markers remain in: %s", strings.Join(badFiles, ", "))
	}
	return nil
}

// validateRepoState checks that the repo is in a clean state suitable for auto mode.
// Returns an error if there's an in-progress merge, rebase, or unmerged files.
func validateRepoState(root string) error {
	// Resolve .git dir (could be a file in a worktree)
	gitDir := filepath.Join(root, ".git")

	// Check no in-progress merge
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return fmt.Errorf("repository has an in-progress merge — resolve with 'git merge --abort' or 'git merge --continue' first")
	}
	// Check no in-progress rebase
	if dirExists(filepath.Join(gitDir, "rebase-merge")) || dirExists(filepath.Join(gitDir, "rebase-apply")) {
		return fmt.Errorf("repository has an in-progress rebase — resolve first")
	}
	// Check no unmerged files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("repository has unmerged files — resolve conflicts first:\n%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// getCurrentBranch returns the current branch name, or "HEAD" if detached.
func getCurrentBranch(root string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// belmontManagedPaths is the allow-list of subtrees Belmont's installer writes
// into (or has previously written into and now manages cleanup of). Used by
// the dirty-tree preflight (to give a Belmont-aware error) and by the update
// auto-commit (to scope `git add` so unrelated user work isn't swept up).
//
// Phase 2 actively writes: `.agents/belmont/`, `.agents/skills/belmont/`,
// `.claude/agents/belmont` (sub-agent symlink), `.claude/commands/belmont/`
// and `.opencode/command/belmont/` (per-skill slash-command symlinks →
// .agents/skills/belmont/<skill>/SKILL.md).
// Every other entry below is a legacy path kept in the list so deletions of
// stale dirs/files also get staged for the auto-commit when an older project
// upgrades through `belmont update` → `belmont install` → runLegacyCleanup.
var belmontManagedPaths = []string{
	".agents/belmont",
	".agents/skills/belmont",
	".claude/agents/belmont",
	".claude/commands/belmont",
	".opencode/command/belmont",
	// Legacy (may be staged for deletion):
	".claude/skills/belmont",  // Phase-2 nested symlink — never worked
	".claude/plugins/belmont", // Phase-2.5 project-local-plugin attempt — also never worked
	".codex/belmont",
	".cursor/rules/belmont",
	".windsurf/rules/belmont",
	".gemini/rules/belmont",
	".copilot/belmont",
	"AGENTS.md",
	"GEMINI.md",
}

func pathIsBelmontManaged(p string) bool {
	for _, prefix := range belmontManagedPaths {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// porcelainPath extracts the path from a `git status --porcelain` line
// ("XY path" or "XY old -> new" for renames).
func porcelainPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	p := strings.TrimSpace(line[3:])
	if idx := strings.Index(p, " -> "); idx >= 0 {
		p = p[idx+4:]
	}
	return p
}

// requireCleanWorkingTree returns an error if `git status --porcelain` reports
// any uncommitted, unstaged, or untracked changes in root. The error message
// is shaped for direct printing — coloured headers, a truncated path list,
// and resolution hints. If root isn't a git repo, returns nil (no block).
func requireCleanWorkingTree(root string) error {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=normal")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	// Trim only trailing newline — porcelain lines start with a status code in
	// columns 0-1 that may include a leading space (e.g. " M path"), so a full
	// TrimSpace would corrupt the first line's path offset.
	output := strings.TrimRight(string(out), "\n")
	if output == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	belmontHits := 0
	for _, ln := range lines {
		if pathIsBelmontManaged(porcelainPath(ln)) {
			belmontHits++
		}
	}

	const maxList = 20
	listed := lines
	truncated := 0
	if len(listed) > maxList {
		truncated = len(listed) - maxList
		listed = listed[:maxList]
	}

	var sb strings.Builder
	sb.WriteString(ansiRed + "working tree is not clean — refusing to start auto" + ansiReset + "\n\n")
	sb.WriteString("The following files have uncommitted, unstaged, or untracked changes:\n")
	for _, ln := range listed {
		sb.WriteString("  ")
		sb.WriteString(ln)
		sb.WriteString("\n")
	}
	if truncated > 0 {
		sb.WriteString(fmt.Sprintf("  ... and %d more\n", truncated))
	}
	sb.WriteString("\n")
	if belmontHits > 0 {
		sb.WriteString(ansiYellow + "Looks like a recent `belmont update` left files uncommitted." + ansiReset + "\n")
		sb.WriteString("If a worktree merges back into this branch later, those uncommitted files will block the merge.\n\n")
	} else {
		sb.WriteString(ansiYellow + "If a worktree merges back into this branch later, uncommitted files may block the merge." + ansiReset + "\n\n")
	}
	sb.WriteString("Resolve with one of:\n")
	sb.WriteString("  git stash -u                  # stash changes (incl. untracked) and start clean\n")
	sb.WriteString("  git commit -am \"...\"          # commit your changes\n")
	sb.WriteString("  belmont auto --allow-dirty    # skip this check (not recommended)")
	return errors.New(sb.String())
}

// ensureCleanMergeState aborts any in-progress merge and cleans up unmerged files.
// Called between sequential merges to prevent cascade failures.
func ensureCleanMergeState(root string) error {
	gitDir := filepath.Join(root, ".git")

	// Abort any in-progress merge
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = root
		abortCmd.Run()
	}

	// Check for remaining unmerged files
	diffCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	diffCmd.Dir = root
	out, _ := diffCmd.Output()
	if strings.TrimSpace(string(out)) == "" {
		return nil // clean
	}

	// Try harder: reset index and checkout
	resetCmd := exec.Command("git", "reset", "HEAD")
	resetCmd.Dir = root
	resetCmd.Run()
	checkoutCmd := exec.Command("git", "checkout", "--", ".")
	checkoutCmd.Dir = root
	checkoutCmd.Run()

	// Re-check
	recheckCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	recheckCmd.Dir = root
	out2, _ := recheckCmd.Output()
	if strings.TrimSpace(string(out2)) != "" {
		return fmt.Errorf("unable to clean merge state — unmerged files remain:\n%s", strings.TrimSpace(string(out2)))
	}
	return nil
}

// commitWorktreeFeatureState commits the initial .belmont/features/ state in a worktree
// so the AI agent starts from a clean git state.
func commitWorktreeFeatureState(wtPath, slug string) {
	// .belmont/ is marked assume-unchanged to prevent worktree merges from
	// deleting other features' state. No .belmont/ commit needed here —
	// the orchestrator copies feature state back after merge.
}

// ensureGitignoreEntry adds an entry to .gitignore if not already present.
func ensureGitignoreEntry(root, entry string) {
	gitignorePath := filepath.Join(root, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(content), entry) {
			return // already present
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

// commitBelmontState commits any uncommitted .belmont/ state files in the main repo.
// Used after belmont sync updates master PROGRESS.md.
func commitBelmontState(root string) error {
	// Don't try to commit if there's an in-progress merge — git commit would
	// either fail or finalize the merge unintentionally
	if fileExists(filepath.Join(root, ".git", "MERGE_HEAD")) {
		return fmt.Errorf("skipping: merge in progress")
	}

	statusCmd := exec.Command("git", "status", "--porcelain", ".belmont/")
	statusCmd.Dir = root
	out, err := statusCmd.Output()
	if err != nil {
		return nil // can't check, skip gracefully
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil // nothing to commit
	}

	addCmd := exec.Command("git", "add", ".belmont/")
	addCmd.Dir = root
	if _, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add .belmont/: %w", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "belmont: update state files")
	commitCmd.Dir = root
	if _, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit .belmont/: %w", err)
	}
	return nil
}

// mergeFailureKind classifies the type of git merge failure.
type mergeFailureKind int

const (
	mergeConflict           mergeFailureKind = iota // file-level conflicts
	mergeUntrackedOverwrite                         // untracked files would be overwritten
	mergeDirtyWorktree                              // local changes would be overwritten
	mergeUnmergedFiles                              // stale unmerged files from previous merge
	mergeOtherFailure                               // unknown merge failure
)

// classifyMergeError determines what kind of merge failure occurred from git output.
func classifyMergeError(output string) mergeFailureKind {
	if strings.Contains(output, "untracked working tree files would be overwritten") {
		return mergeUntrackedOverwrite
	}
	if strings.Contains(output, "Your local changes to the following files would be overwritten") {
		return mergeDirtyWorktree
	}
	if strings.Contains(output, "CONFLICT") || strings.Contains(output, "Automatic merge failed") {
		return mergeConflict
	}
	if strings.Contains(output, "unmerged files") {
		return mergeUnmergedFiles
	}
	return mergeOtherFailure
}

// parseOverwrittenFiles extracts file paths from git's "untracked working tree files would be overwritten" error.
func parseOverwrittenFiles(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")
	inFileList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "untracked working tree files would be overwritten") {
			inFileList = true
			continue
		}
		if inFileList {
			if trimmed == "" || strings.HasPrefix(trimmed, "Please move or remove") || strings.HasPrefix(trimmed, "Aborting") {
				break
			}
			if trimmed != "" {
				files = append(files, trimmed)
			}
		}
	}
	return files
}

// autoResolveBelmontConflicts attempts to auto-resolve merge conflicts on .belmont/ files.
// For PROGRESS.md files, it takes the union of milestone completions (each milestone marks
// only its own [x] status, so union is safe). Returns true if any conflicts were resolved.
func autoResolveBelmontConflicts(root string) bool {
	// Get list of conflicted files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	resolved := false
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if !strings.HasPrefix(file, ".belmont/") {
			continue
		}

		filePath := filepath.Join(root, file)

		if strings.HasSuffix(file, "PROGRESS.md") {
			// For PROGRESS.md: get both sides and merge milestone completions
			if resolveProgressConflict(root, file, filePath) {
				resolved = true
			}
		} else {
			// For other .belmont/ files (e.g., MILESTONE.md): take "theirs" (the branch being merged)
			checkoutCmd := exec.Command("git", "checkout", "--theirs", "--", file)
			checkoutCmd.Dir = root
			if _, err := checkoutCmd.CombinedOutput(); err == nil {
				addCmd := exec.Command("git", "add", file)
				addCmd.Dir = root
				addCmd.Run()
				resolved = true
			}
		}
	}
	return resolved
}

// resolveProgressConflict merges a conflicted PROGRESS.md by taking the most-advanced
// task state from both sides. State ordering: [v] > [x] > [>] > [ ], [!] preserved.
// Returns true if successfully resolved.
func resolveProgressConflict(root, relPath, filePath string) bool {
	// Get "ours" version
	oursCmd := exec.Command("git", "show", ":2:"+relPath)
	oursCmd.Dir = root
	oursOut, err := oursCmd.Output()
	if err != nil {
		return false
	}

	// Get "theirs" version
	theirsCmd := exec.Command("git", "show", ":3:"+relPath)
	theirsCmd.Dir = root
	theirsOut, err := theirsCmd.Output()
	if err != nil {
		return false
	}

	// State priority: higher = more advanced
	statePriority := map[string]int{" ": 0, ">": 1, "x": 2, "v": 3, "!": -1}

	// Parse task states from "theirs"
	theirsStates := make(map[string]string) // task ID → checkbox marker
	taskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s+(P\d+-[\w][\w-]*)`)
	theirsLines := strings.Split(string(theirsOut), "\n")
	for _, line := range theirsLines {
		if m := taskRe.FindStringSubmatch(line); m != nil {
			theirsStates[m[2]] = m[1]
		}
	}

	// Collect unique activity entries from "theirs"
	theirsActivityLines := make(map[string]bool)
	inActivity := false
	for _, line := range theirsLines {
		if strings.Contains(line, "## Recent Activity") || strings.Contains(line, "## Activity") || strings.Contains(line, "## Session History") {
			inActivity = true
			continue
		}
		if inActivity && strings.HasPrefix(line, "##") {
			inActivity = false
		}
		if inActivity && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			theirsActivityLines[strings.TrimSpace(line)] = true
		}
	}

	// Merge: start from "ours", upgrade task states from "theirs"
	oursLines := strings.Split(string(oursOut), "\n")
	var merged []string
	inActivitySection := false
	activityInserted := make(map[string]bool)

	for _, line := range oursLines {
		// Upgrade task checkboxes to the more-advanced state
		if m := taskRe.FindStringSubmatch(line); m != nil {
			oursMarker := m[1]
			taskID := m[2]
			if theirsMarker, ok := theirsStates[taskID]; ok {
				// Take the more-advanced state (but preserve [!] blocked)
				if oursMarker == "!" || theirsMarker == "!" {
					// Keep blocked as-is from ours
				} else if statePriority[theirsMarker] > statePriority[oursMarker] {
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				}
			}
		}

		// Track activity section for merging entries
		if strings.Contains(line, "## Recent Activity") || strings.Contains(line, "## Activity") || strings.Contains(line, "## Session History") {
			inActivitySection = true
		} else if inActivitySection && strings.HasPrefix(line, "##") {
			for theirsLine := range theirsActivityLines {
				if !activityInserted[theirsLine] {
					merged = append(merged, theirsLine)
					activityInserted[theirsLine] = true
				}
			}
			inActivitySection = false
		}

		if inActivitySection && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			activityInserted[strings.TrimSpace(line)] = true
		}

		merged = append(merged, line)
	}

	result := strings.Join(merged, "\n")
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return false
	}

	addCmd := exec.Command("git", "add", relPath)
	addCmd.Dir = root
	addCmd.Run()
	return true
}

// autoResolveLockFiles detects conflicted lock files and regenerates them.
// Only handles lock files whose corresponding manifest is NOT conflicted
// (if the manifest is also conflicted, the AI agent needs to handle both together).
func autoResolveLockFiles(root string) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}

	// Map lock file → (package manager install command, manifest file)
	lockFileMap := map[string]struct {
		installCmd string
		manifest   string
	}{
		"package-lock.json": {"npm install", "package.json"},
		"pnpm-lock.yaml":    {"pnpm install", "package.json"},
		"yarn.lock":         {"yarn install", "package.json"},
		"bun.lockb":         {"bun install", "package.json"},
		"Cargo.lock":        {"cargo generate-lockfile", "Cargo.toml"},
		"go.sum":            {"go mod tidy", "go.mod"},
		"Gemfile.lock":      {"bundle install", "Gemfile"},
		"poetry.lock":       {"poetry lock --no-update", "pyproject.toml"},
	}

	conflicted := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			conflicted[line] = true
		}
	}

	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		// Only handle lock files in the repo root (basename match)
		baseName := filepath.Base(file)
		info, isLock := lockFileMap[baseName]
		if !isLock {
			continue
		}

		// Check if the corresponding manifest is also conflicted
		manifestPath := filepath.Join(filepath.Dir(file), info.manifest)
		if conflicted[manifestPath] {
			// Both conflicted — leave for the AI agent to handle together
			continue
		}

		fmt.Fprintf(os.Stderr, "  \033[2mAuto-resolving %s via %s\033[0m\n", file, info.installCmd)

		// Delete the conflicted lock file
		os.Remove(filepath.Join(root, file))

		// Run the package manager to regenerate
		parts := strings.Fields(info.installCmd)
		installCmd := exec.Command(parts[0], parts[1:]...)
		installCmd.Dir = root
		if installOut, err := installCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to regenerate %s: %s\033[0m\n", file, strings.TrimSpace(string(installOut)))
			// Restore the conflicted version so git knows it's still unresolved
			checkoutCmd := exec.Command("git", "checkout", "--merge", "--", file)
			checkoutCmd.Dir = root
			checkoutCmd.Run()
			continue
		}

		// Stage the regenerated lock file
		addCmd := exec.Command("git", "add", file)
		addCmd.Dir = root
		addCmd.Run()
	}
}

// listPreservedWorktrees finds worktrees that still exist.
// Checks both the new sibling location and the legacy .belmont/worktrees/ path.
func listPreservedWorktrees(root string) []worktreeEntry {
	var result []worktreeEntry
	// New location: sibling directory
	result = append(result, scanWorktreeDir(worktreeBasePath(root))...)
	// Legacy location: inside project
	legacyDir := filepath.Join(root, ".belmont", "worktrees")
	result = append(result, scanWorktreeDir(legacyDir)...)
	return result
}

// scanWorktreeDir scans a directory for preserved git worktrees.
func scanWorktreeDir(wtDir string) []worktreeEntry {
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return nil
	}

	var result []worktreeEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(wtDir, e.Name())
		// Check if it's actually a git worktree by looking for .git file
		gitFile := filepath.Join(wtPath, ".git")
		if _, err := os.Stat(gitFile); err != nil {
			continue
		}
		// Detect the actual branch from the worktree's HEAD
		branch := detectWorktreeBranch(wtPath)
		if branch == "" {
			branch = "belmont/auto/" + e.Name() // fallback
		}
		result = append(result, worktreeEntry{Path: wtPath, Branch: branch})
	}
	return result
}

// detectWorktreeBranch reads the actual branch name from a git worktree.
func detectWorktreeBranch(wtPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

func runSyncCmd(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	fs.StringVar(&root, "root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	root, _ = filepath.Abs(root)

	// If running in a worktree context (detected via .worktree marker), no-op.
	// Worktrees have isolated state; sync only makes sense on the main repo.
	if fileExists(filepath.Join(root, ".belmont", ".worktree")) {
		return nil
	}

	featuresDir := filepath.Join(root, ".belmont", "features")
	features := listFeatures(featuresDir, 50)
	if len(features) == 0 {
		return nil
	}

	syncMasterFeatureStatuses(root, features)

	// Commit if anything changed (best-effort)
	commitBelmontState(root)
	return nil
}

func findWorktree(worktrees []worktreeEntry, slug string) *worktreeEntry {
	for _, wt := range worktrees {
		if filepath.Base(wt.Path) == slug {
			return &wt
		}
	}
	return nil
}

// ============================================================================
// belmont steer — inject user instructions into an in-flight auto run.
//
// The auto loop runs headless AI CLI invocations inside isolated worktrees;
// there is no channel for the user to interject. `belmont steer` writes an
// append-only STEERING.md in each active worktree (or the master feature
// directory for non-parallel runs). executeLoopAction consumes pending
// entries before each phase and prepends them to the agent prompt as a
// higher-priority block than NOTES.md.
// ============================================================================

// steeringEntry represents a single block in STEERING.md.
type steeringEntry struct {
	Timestamp string // RFC3339 UTC from the header
	Milestone string // optional — empty means applies to any milestone
	State     string // "pending" or "consumed <ts> by <phase>"
	Body      string // free-form text between this header and the next
}

var steeringHeaderRe = regexp.MustCompile(`^##\s+(\S+)(?:\s+\[([^\]]+)\])?\s+\(([^)]+)\)\s*$`)

// steeringTarget identifies a single worktree (or master root) to write
// STEERING.md into.
type steeringTarget struct {
	MilestoneID string // empty for non-parallel runs
	Root        string // absolute worktree path (or master root)
	Label       string // e.g. "M5" or "serial" — for log output
}

// readActiveAutoJSON returns auto.json if there's an active run; errors
// otherwise with a helpful message.
func readActiveAutoJSON(root string) (autoJSON, error) {
	autoPath := filepath.Join(root, ".belmont", "auto.json")
	data, err := os.ReadFile(autoPath)
	if err != nil {
		return autoJSON{}, fmt.Errorf("steer: no active auto run (missing %s). steering only applies to in-flight auto mode — start one with `belmont auto`, or steer a manual CLI session by typing directly into it", autoPath)
	}
	var aj autoJSON
	if err := json.Unmarshal(data, &aj); err != nil {
		return autoJSON{}, fmt.Errorf("steer: parse auto.json: %w", err)
	}
	if !aj.Active {
		return autoJSON{}, fmt.Errorf("steer: auto.json exists but no active run — nothing to steer")
	}
	return aj, nil
}

// ============================================================================
// belmont validate — detect milestone structure violations in PROGRESS.md.
//
// Catches the "polish milestone" anti-pattern — where a skill (implement,
// verify, etc.) invents a new milestone like "M5: Polish / follow-ups from
// M1" to hold deferred items. Such milestones declare dependency on their
// source but actually mutate siblings' outputs, producing silent merge
// conflicts in parallel auto runs. See skills/belmont/_partials/milestone-
// immutability.md for the canonical rule this command enforces.
// ============================================================================

// validationViolation is one finding from `belmont validate`.
type validationViolation struct {
	Feature       string `json:"feature"`
	Milestone     string `json:"milestone"`
	MilestoneName string `json:"milestone_name"`
	TaskID        string `json:"task_id,omitempty"`
	Rule          string `json:"rule"`
	Message       string `json:"message"`
}

// Matches milestone names that look like polish / follow-up catch-alls.
// Conservative — only fires on unambiguously bad patterns so legitimate
// cross-cutting milestones ("Accessibility audit across routes") aren't
// flagged. Case-insensitive via (?i).
var polishMilestoneNameRe = regexp.MustCompile(`(?i)(\bpolish\b|\bfollow[- ]?ups?\b|\bcleanup\b|\bverification fixes?\b|\bdesign fidelity fixes?\b|\bdeviations from\s+m\d+|from\s+m\d+\s+implementation\b|\bfwlup[s]?\b)`)

// Matches task IDs that embed a milestone number, e.g. P3-FWLUP-M2-1 or
// P1-M4-FIX-2. Capture group 1 is the milestone number referenced by the ID.
var taskIDMilestoneRefRe = regexp.MustCompile(`^P\d+-(?:FWLUP-)?M(\d+)(?:-|$)`)

// runValidateCmd implements `belmont validate`.
func runValidateCmd(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, format string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug (default: scan every feature)")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("validate: resolve root: %w", err)
	}

	var violations []validationViolation
	if feature != "" {
		v, err := validateFeature(absRoot, feature)
		if err != nil {
			return err
		}
		violations = v
	} else {
		featuresDir := filepath.Join(absRoot, ".belmont", "features")
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			return fmt.Errorf("validate: read features dir %s: %w", featuresDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			v, err := validateFeature(absRoot, entry.Name())
			if err != nil {
				// Missing PROGRESS.md etc. is not fatal across features.
				fmt.Fprintf(os.Stderr, "\033[33m⚠ %s: %s\033[0m\n", entry.Name(), err)
				continue
			}
			violations = append(violations, v...)
		}
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(violations); err != nil {
			return err
		}
	default:
		renderValidationReport(os.Stdout, violations)
	}

	if len(violations) > 0 {
		return fmt.Errorf("validate: found %d milestone-structure violation(s)", len(violations))
	}
	return nil
}

// validateFeature reads a feature's PROGRESS.md and returns any violations.
func validateFeature(root, slug string) ([]validationViolation, error) {
	featuresDir := filepath.Join(root, ".belmont", "features", slug)
	if !dirExists(featuresDir) {
		return nil, fmt.Errorf("feature %q not found at %s", slug, featuresDir)
	}
	// Prefer live worktree state if one exists for this feature, same as
	// `belmont status` does.
	progressPath := filepath.Join(featuresDir, "PROGRESS.md")
	if override := loadAutoWorktrees(root); override != nil {
		if wtFeature, ok := override[slug]; ok {
			if p := filepath.Join(wtFeature, "PROGRESS.md"); fileExists(p) {
				progressPath = p
			}
		}
	}
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", progressPath, err)
	}
	milestones := parseMilestones(string(data))
	return detectViolations(slug, milestones), nil
}

// detectViolations is the pure rule engine — no IO. Takes parsed milestones,
// returns a list of findings. Safe for test coverage.
func detectViolations(slug string, milestones []milestone) []validationViolation {
	var out []validationViolation
	for _, m := range milestones {
		// Rule 1: milestone name matches a polish/follow-up pattern.
		if polishMilestoneNameRe.MatchString(m.Name) {
			out = append(out, validationViolation{
				Feature:       slug,
				Milestone:     m.ID,
				MilestoneName: m.Name,
				Rule:          "polish_milestone_name",
				Message: fmt.Sprintf(
					"milestone %s %q looks like a polish/follow-up catch-all. Follow-ups belong in their source milestone (the one that discovered them) as new `[ ]` tasks, not a dedicated milestone. Run `/belmont:tech-plan` to restructure.",
					m.ID, m.Name),
			})
		}
		// Rule 2: task IDs reference a milestone other than the one they live in.
		currentNum := milestoneNumber(m.ID)
		for _, t := range m.Tasks {
			match := taskIDMilestoneRefRe.FindStringSubmatch(t.ID)
			if len(match) < 2 {
				continue
			}
			refNum, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			if refNum != currentNum {
				out = append(out, validationViolation{
					Feature:       slug,
					Milestone:     m.ID,
					MilestoneName: m.Name,
					TaskID:        t.ID,
					Rule:          "cross_milestone_task_id",
					Message: fmt.Sprintf(
						"task %q in milestone %s names milestone M%d in its ID. It belongs under M%d. Move it there — keeping it here is the dependency-graph lie that causes parallel merge conflicts.",
						t.ID, m.ID, refNum, refNum),
				})
			}
		}
	}
	return out
}

// milestoneNumber extracts the integer from a milestone ID like "M5".
func milestoneNumber(id string) int {
	trimmed := strings.TrimPrefix(id, "M")
	if n, err := strconv.Atoi(trimmed); err == nil {
		return n
	}
	return -1
}

// renderValidationReport writes a human-readable summary to w.
func renderValidationReport(w io.Writer, violations []validationViolation) {
	if len(violations) == 0 {
		fmt.Fprintln(w, "\033[32m✓ No milestone-structure violations found.\033[0m")
		return
	}
	fmt.Fprintf(w, "\033[31m✗ %d violation(s) found:\033[0m\n\n", len(violations))
	// Group by feature → milestone for readability.
	byFeat := map[string][]validationViolation{}
	var featOrder []string
	for _, v := range violations {
		if _, seen := byFeat[v.Feature]; !seen {
			featOrder = append(featOrder, v.Feature)
		}
		byFeat[v.Feature] = append(byFeat[v.Feature], v)
	}
	for _, feat := range featOrder {
		fmt.Fprintf(w, "  \033[1m%s\033[0m\n", feat)
		for _, v := range byFeat[feat] {
			if v.TaskID != "" {
				fmt.Fprintf(w, "    • [%s/%s] %s — %s\n", v.Milestone, v.TaskID, v.Rule, v.Message)
			} else {
				fmt.Fprintf(w, "    • [%s] %s — %s\n", v.Milestone, v.Rule, v.Message)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\033[2mRerun after fixing with `belmont validate`. See skills/belmont/_partials/milestone-immutability.md for the canonical rule.\033[0m")
}

// ============================================================================
// Layer 1 — post-phase scope guard.
//
// After each agent subprocess exits, we compare PROGRESS.md's milestone
// structure to the snapshot taken before the shell-out. Two kinds of
// violation are reverted:
//
//   (A) New `## M<N>:` milestone headings added during a non-tech-plan phase.
//   (B) Checkbox state flips on tasks belonging to a milestone other than the
//       action's target.
//
// Revert rewrites PROGRESS.md to restore the pre-phase bytes for the
// violating milestone blocks, preserves in-scope edits (target milestone
// body, non-milestone sections like activity log), amends the agent's last
// commit (best-effort), and injects a STEERING correction so the next phase
// sees an explicit "do not do that" before it starts work.
//
// This is unbypassable by `git commit --no-verify` because it runs in the
// Belmont Go process after the agent subprocess has exited.
// ============================================================================

// milestoneBlockText captures a single milestone block's exact bytes along
// with its task state map, so we can both diff state and rewrite verbatim.
type milestoneBlockText struct {
	ID         string
	Name       string
	RawLines   []string          // the header line plus every line up to (exclusive) the next block boundary
	TaskStates map[string]string // taskID -> marker rune as string ([ ], [x], [v], [>], [!])
}

// progressSnapshot preserves enough of PROGRESS.md to rebuild it after
// reverting out-of-scope edits. Non-milestone lines (preamble, activity log,
// decisions section) are stored too so they can be preserved verbatim.
type progressSnapshot struct {
	Path   string
	Raw    string
	Blocks []milestoneBlockText
	ByID   map[string]int // milestone ID -> index in Blocks
}

// parseProgressSnapshot splits the PROGRESS.md content into milestone blocks.
// A block begins at a line matching the milestone header regex; it ends when
// the next block begins, or when a `## ` level-2 heading is encountered, or
// at EOF. Non-milestone content is not stored as a separate block; instead
// the Raw field is kept so the rebuilder can do line-boundary-preserving
// replacement via string operations.
func parseProgressSnapshot(path, content string) *progressSnapshot {
	snap := &progressSnapshot{Path: path, Raw: content, ByID: map[string]int{}}
	msHeaderRe := regexp.MustCompile(`(?m)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	taskRe := regexp.MustCompile(`(?m)^\s*-\s+\[(.)\]\s+(\S+?):`)

	lines := strings.Split(content, "\n")
	var current *milestoneBlockText
	flush := func() {
		if current != nil {
			snap.ByID[current.ID] = len(snap.Blocks)
			snap.Blocks = append(snap.Blocks, *current)
			current = nil
		}
	}
	for _, line := range lines {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
			flush()
			id := "M" + m[1]
			name := strings.TrimSpace(m[2])
			name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
			current = &milestoneBlockText{ID: id, Name: name, TaskStates: map[string]string{}}
			current.RawLines = append(current.RawLines, line)
			continue
		}
		// A non-milestone level-2 heading closes the current block.
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ") {
			flush()
			continue
		}
		if current != nil {
			current.RawLines = append(current.RawLines, line)
			if tm := taskRe.FindStringSubmatch(line); len(tm) >= 3 {
				current.TaskStates[tm[2]] = tm[1]
			}
		}
	}
	flush()
	return snap
}

// scopeViolation is one finding from the post-phase guard.
type scopeViolation struct {
	Kind          string // "new_milestone" | "out_of_scope_flip"
	Milestone     string // milestone ID involved
	MilestoneName string
	TaskID        string // for out_of_scope_flip
	FromState     string // for out_of_scope_flip
	ToState       string // for out_of_scope_flip
}

// logScopeGuardRevert prints a one-line summary of each violation to stderr.
func logScopeGuardRevert(feature, milestoneID string, violations []scopeViolation) {
	prefix := ""
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	summary := summarizeScopeViolations(violations)
	fmt.Fprintf(os.Stderr, "%s\033[33m[SCOPE-GUARD]\033[0m reverted %d violation(s) — %s\n", prefix, len(violations), summary)
}

// summarizeScopeViolations produces a terse one-line summary suitable for
// the stream (matches steering's preview style).
func summarizeScopeViolations(violations []scopeViolation) string {
	counts := map[string]int{}
	var sample string
	for _, v := range violations {
		counts[v.Kind]++
		if sample == "" {
			switch v.Kind {
			case "new_milestone":
				sample = fmt.Sprintf("new milestone %s %q", v.Milestone, v.MilestoneName)
			case "out_of_scope_flip":
				sample = fmt.Sprintf("%s in %s (%s→%s)", v.TaskID, v.Milestone, v.FromState, v.ToState)
			}
		}
	}
	var parts []string
	if n := counts["new_milestone"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d new milestone", n))
	}
	if n := counts["out_of_scope_flip"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d out-of-scope flip", n))
	}
	return fmt.Sprintf("%s — first: %s", strings.Join(parts, ", "), sample)
}

// ============================================================================
// Layer 2 — verify evidence check.
//
// Before we accept a [v] flip, require at least one git commit in this
// worktree whose message names the task ID. Rationale: the `verify` skill
// can (and has, in the wild) rubber-stamp tasks whose underlying code is
// still scaffold. If no commit in the worktree names the task, we have no
// evidence it was implemented — revert the flip.
//
// Fires only on actionVerify phases (Layer 1 already guards implement/next).
// Runs after runScopeGuard so in-scope flips are the only candidates.
// ============================================================================

// evidenceMissing records one [v] flip that lacks a commit referencing the task.
type evidenceMissing struct {
	Milestone string
	TaskID    string
	FromState string // prior state (pre)
}

// findEvidenceMissingFlips walks post, identifies tasks that flipped TO "v"
// this phase (vs pre), and returns any without a matching git commit. When
// targetMS is non-empty only tasks under that milestone are evaluated.
func findEvidenceMissingFlips(root string, pre, post *progressSnapshot, targetMS string) []evidenceMissing {
	mergeBase := findMergeBaseRef(root)
	var missing []evidenceMissing
	for _, pb := range post.Blocks {
		if targetMS != "" && pb.ID != targetMS {
			continue
		}
		preIdx, existedPre := pre.ByID[pb.ID]
		for taskID, postState := range pb.TaskStates {
			if postState != "v" {
				continue
			}
			var preState string
			if existedPre {
				preState = pre.Blocks[preIdx].TaskStates[taskID]
			}
			if preState == "v" {
				continue // already verified, not a fresh flip this phase
			}
			if taskHasCommit(root, taskID, mergeBase) {
				continue
			}
			missing = append(missing, evidenceMissing{
				Milestone: pb.ID,
				TaskID:    taskID,
				FromState: preState,
			})
		}
	}
	return missing
}

// findMergeBaseRef returns the best-guess fork point of the current branch.
// Empty string means "no scoping" — fall back to the full log.
func findMergeBaseRef(root string) string {
	for _, candidate := range []string{"main", "master", "origin/main", "origin/master"} {
		cmd := exec.Command("git", "merge-base", "HEAD", candidate)
		cmd.Dir = root
		if out, err := cmd.Output(); err == nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// taskHasCommit reports whether any commit reachable from HEAD names the
// given task ID. When sinceRef is non-empty the search is limited to
// sinceRef..HEAD so older features' commits don't produce false positives.
func taskHasCommit(root, taskID, sinceRef string) bool {
	if taskID == "" {
		return true // nothing to check
	}
	pattern := regexp.MustCompile(`(^|[^A-Za-z0-9-])` + regexp.QuoteMeta(taskID) + `([^A-Za-z0-9-]|$)`)
	args := []string{"log", "--format=%B%x1e"}
	if sinceRef != "" {
		args = append(args, sinceRef+"..HEAD")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// If the git query fails (e.g., shallow clone, bad ref), treat as
		// "evidence present" to avoid false negatives blocking real work.
		return true
	}
	for _, msg := range strings.Split(string(out), "\x1e") {
		if pattern.MatchString(msg) {
			return true
		}
	}
	return false
}

// revertEvidenceMissing rebuilds PROGRESS.md so that each entry in `missing`
// reverts its task line from "[v]" back to the prior state captured in pre.
// Uses a line-level replacement scoped to each task's milestone block.
func revertEvidenceMissing(post, pre *progressSnapshot, missing []evidenceMissing) string {
	// Build a map: milestone -> taskID -> fromState, for O(1) lookup.
	byMS := map[string]map[string]string{}
	for _, m := range missing {
		if byMS[m.Milestone] == nil {
			byMS[m.Milestone] = map[string]string{}
		}
		fromState := m.FromState
		if fromState == "" {
			fromState = " "
		}
		byMS[m.Milestone][m.TaskID] = fromState
	}

	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)
	taskRe := regexp.MustCompile(`^(\s*-\s+\[)(.)\](\s+)(\S+?)(:.*)$`)

	var out strings.Builder
	var currentMS string
	lines := strings.Split(post.Raw, "\n")
	for i, line := range lines {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
			currentMS = "M" + m[1]
		} else {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ") {
				currentMS = ""
			}
		}
		if currentMS != "" {
			if overrides, ok := byMS[currentMS]; ok {
				if tm := taskRe.FindStringSubmatch(line); len(tm) >= 6 {
					taskID := tm[4]
					if from, hit := overrides[taskID]; hit && tm[2] == "v" {
						line = tm[1] + from + "]" + tm[3] + tm[4] + tm[5]
					}
				}
			}
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

// logEvidenceRevert prints a one-line summary per verify-guard revert batch.
func logEvidenceRevert(feature, milestoneID string, missing []evidenceMissing) {
	prefix := ""
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	var ids []string
	for _, m := range missing {
		ids = append(ids, m.TaskID)
	}
	sort.Strings(ids)
	preview := strings.Join(ids, ", ")
	if len(preview) > 100 {
		preview = preview[:99] + "…"
	}
	fmt.Fprintf(os.Stderr, "%s\033[33m[VERIFY-GUARD]\033[0m reverted %d [v] flip(s) lacking commit evidence — %s\n", prefix, len(missing), preview)
}

// nonEmpty returns fallback when s is empty.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ============================================================================
// Layer 3 — merge overlap visibility.
//
// At merge time, when we're landing sibling milestones in sequence, warn if
// the branch being merged touches files that a previously-merged sibling
// also touched. Does not block the merge; the point is visibility so the
// user sees the overlap at the moment intervention is cheap (before pushing
// or before the combined state diverges too far).
//
// This is a diagnostic layer — Layer 0 prevents the most common cause, and
// Layer 1 reverts the state-file manifestation. Layer 3 catches residual
// source-file overlap that slips past both.
// ============================================================================

// branchTouchedFiles returns the list of files whose content differs on
// `branch` compared to the merge base with HEAD. Empty slice on error.
func branchTouchedFiles(root, branch string) []string {
	base := "HEAD"
	if mb := findMergeBaseOfBranch(root, branch); mb != "" {
		base = mb
	}
	cmd := exec.Command("git", "diff", "--name-only", base+".."+branch)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return files
}

// findMergeBaseOfBranch returns the merge-base between HEAD and branch.
// Empty string if git fails or no common ancestor.
func findMergeBaseOfBranch(root, branch string) string {
	cmd := exec.Command("git", "merge-base", "HEAD", branch)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// reportMergeOverlap prints a one-block warning listing files that this
// branch touches and that were also touched by siblings merged earlier.
// mergedFiles is the cumulative map from prior iterations.
func reportMergeOverlap(root, branch, msID string, mergedFiles map[string][]string) {
	if len(mergedFiles) == 0 {
		return
	}
	touched := branchTouchedFiles(root, branch)
	if len(touched) == 0 {
		return
	}
	type entry struct {
		File    string
		Sources []string
	}
	var overlaps []entry
	for _, f := range touched {
		if sources, ok := mergedFiles[f]; ok {
			overlaps = append(overlaps, entry{File: f, Sources: sources})
		}
	}
	if len(overlaps) == 0 {
		return
	}
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].File < overlaps[j].File })
	fmt.Fprintf(os.Stderr, "\n  \033[33m⚠ Merge overlap for %s:\033[0m %d file(s) also modified by earlier sibling(s)\n", msID, len(overlaps))
	maxShow := 8
	for i, o := range overlaps {
		if i == maxShow {
			fmt.Fprintf(os.Stderr, "      \033[2m… and %d more\033[0m\n", len(overlaps)-maxShow)
			break
		}
		fmt.Fprintf(os.Stderr, "      %s \033[2m(also in: %s)\033[0m\n", o.File, strings.Join(o.Sources, ", "))
	}
	fmt.Fprintf(os.Stderr, "  \033[2m  Proceeding with default merge strategy — review the resulting commit before pushing.\033[0m\n\n")
}
