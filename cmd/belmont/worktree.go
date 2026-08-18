package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// buildWorktreeEnv creates an environment slice with worktree-specific variables.
// It starts from the current process env and appends PORT, BELMONT_PORT,
// BELMONT_WORKTREE, BELMONT_MONOREPO* (when applicable), and any user-defined
// env vars from worktree.json.
func buildWorktreeEnv(port int, extraEnv map[string]string, workspaces []workspaceInfo, primary string, mType monorepoType) []string {
	env := os.Environ()
	if port != 0 {
		baseURL := fmt.Sprintf("http://localhost:%d", port)
		env = append(env,
			// Belmont-native vars
			fmt.Sprintf("PORT=%d", port),
			fmt.Sprintf("BELMONT_PORT=%d", port),
			fmt.Sprintf("BELMONT_BASE_URL=%s", baseURL),
			"BELMONT_WORKTREE=1",
			// Framework-native vars so common test/e2e tools auto-redirect to
			// the worktree's port without the agent having to remember:
			//   - Playwright: overrides config `use.baseURL` and `webServer.url`
			//   - Cypress: overrides `baseUrl` in cypress.config.*
			//   - Vite: consumed if the project's dev script honors VITE_PORT
			// These close the hole where a hardcoded `localhost:3000` in a
			// committed test config would otherwise win over the assigned port.
			fmt.Sprintf("PLAYWRIGHT_BASE_URL=%s", baseURL),
			fmt.Sprintf("CYPRESS_baseUrl=%s", baseURL),
			fmt.Sprintf("VITE_PORT=%d", port),
		)
	}
	env = append(env, monorepoEnvVars(workspaces, primary, mType)...)
	for k, v := range extraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// runWorktreeHookCommands executes a list of shell commands in the worktree directory.
func runWorktreeHookCommands(commands []string, wtPath string, port int, extraEnv map[string]string, workspaces []workspaceInfo, primary string, mType monorepoType) error {
	env := buildWorktreeEnv(port, extraEnv, workspaces, primary, mType)
	for _, cmdStr := range commands {
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = wtPath
		cmd.Env = env
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q failed: %w", cmdStr, err)
		}
	}
	return nil
}

// copyEnvFiles copies .env* files from the project root into the worktree.
// These are gitignored so they don't exist in fresh worktrees, but are needed
// by postinstall scripts (e.g., prisma generate) and dev servers.
//
// In monorepo mode, env files are also seeded into qualifying workspace dirs
// (the ones whose manifest signals env consumption — Prisma deps, postinstall
// scripts, etc.) plus any workspace whose worktree.json override lists
// explicit env_files. See seedWorkspaceEnv.
func copyEnvFiles(projectRoot, wtPath string, workspaces []workspaceInfo, overrides map[string]workspaceOverride) {
	rootEnvFiles := []string{}
	entries, err := os.ReadDir(projectRoot)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if name == ".env" || strings.HasPrefix(name, ".env.") {
				rootEnvFiles = append(rootEnvFiles, name)
				src := filepath.Join(projectRoot, name)
				dst := filepath.Join(wtPath, name)
				data, err := os.ReadFile(src)
				if err != nil {
					continue
				}
				os.WriteFile(dst, data, 0644)
			}
		}
	}
	for _, ws := range workspaces {
		seedWorkspaceEnv(projectRoot, wtPath, ws, overrides[ws.ID], rootEnvFiles)
	}
}

// rebaseWorktreeOnMain rebases a worktree's branch onto the main repo's
// current HEAD. The worktree shares .git/objects with mainRoot, so the target
// SHA is reachable without a network fetch.
//
// Returns the number of new main commits picked up by the rebase. Returns
// errWorktreeDirty (with newCommits==0) when the worktree has uncommitted
// changes — the rebase is skipped, leaving the worktree as-is. On rebase
// conflict the rebase is aborted and a wrapped error is returned, leaving
// the worktree HEAD unchanged.
//
// `.belmont/` files are marked assume-unchanged in worktrees, so the dirty
// check ignores them and only catches genuine in-flight agent/user work.
func rebaseWorktreeOnMain(mainRoot, wtPath string) (newCommits int, err error) {
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = wtPath
	statusOut, statusErr := statusCmd.Output()
	if statusErr != nil {
		return 0, fmt.Errorf("git status: %w", statusErr)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		return 0, errWorktreeDirty
	}

	mainHeadCmd := exec.Command("git", "rev-parse", "HEAD")
	mainHeadCmd.Dir = mainRoot
	mainHeadOut, mainHeadErr := mainHeadCmd.Output()
	if mainHeadErr != nil {
		return 0, fmt.Errorf("rev-parse main HEAD: %w", mainHeadErr)
	}
	mainSHA := strings.TrimSpace(string(mainHeadOut))

	wtHeadCmd := exec.Command("git", "rev-parse", "HEAD")
	wtHeadCmd.Dir = wtPath
	wtHeadOut, wtHeadErr := wtHeadCmd.Output()
	if wtHeadErr != nil {
		return 0, fmt.Errorf("rev-parse worktree HEAD: %w", wtHeadErr)
	}
	wtSHA := strings.TrimSpace(string(wtHeadOut))
	if wtSHA == mainSHA {
		return 0, nil
	}

	mbCmd := exec.Command("git", "merge-base", wtSHA, mainSHA)
	mbCmd.Dir = wtPath
	mbOut, mbErr := mbCmd.Output()
	if mbErr != nil {
		return 0, fmt.Errorf("merge-base: %w", mbErr)
	}
	mergeBase := strings.TrimSpace(string(mbOut))
	if mergeBase == mainSHA {
		return 0, nil
	}

	cntCmd := exec.Command("git", "rev-list", "--count", mergeBase+".."+mainSHA)
	cntCmd.Dir = wtPath
	if cntOut, err := cntCmd.Output(); err == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(string(cntOut)))
		newCommits = n
	}

	rbCmd := exec.Command("git", "rebase", mainSHA)
	rbCmd.Dir = wtPath
	rbOut, rbErr := rbCmd.CombinedOutput()
	if rbErr != nil {
		abort := exec.Command("git", "rebase", "--abort")
		abort.Dir = wtPath
		abort.Run()
		return newCommits, fmt.Errorf("rebase conflict: %s", strings.TrimSpace(string(rbOut)))
	}

	return newCommits, nil
}

// announceWorktreeRebase prints the rebase outcome in the same shape used by
// auto-mode wave output. Centralised so both resume call sites stay in sync.
func announceWorktreeRebase(id string, newCommits int, err error) {
	if err != nil {
		if errors.Is(err, errWorktreeDirty) {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Skipped rebase of %s — worktree has uncommitted changes\033[0m\n", id)
		} else {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Rebase of %s aborted: %s — worktree left on its previous base\033[0m\n", id, err)
		}
		return
	}
	if newCommits > 0 {
		plural := ""
		if newCommits != 1 {
			plural = "s"
		}
		fmt.Fprintf(os.Stderr, "  \033[36m↻ Rebased %s worktree onto main (%d new commit%s)\033[0m\n", id, newCommits, plural)
	}
}

// removeWorktree removes a git worktree and its directory.
func removeWorktree(root, wtPath, _ string) {
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		// Try manual cleanup
		os.RemoveAll(wtPath)
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = root
		pruneCmd.Run()
	}
}

// copyBelmontStateToWorktree copies feature state and read-only context into a worktree.
// The feature's own state (features/<slug>/) is copied as writable — the agent commits these.
// Master context files (PRD.md, PROGRESS.md, etc.) are copied for reference but excluded from git.
// A .worktree marker file is written so belmont sync can detect worktree context and no-op.
func copyBelmontStateToWorktree(root, wtPath, slug string) error {
	srcBelmont := filepath.Join(root, ".belmont")
	dstBelmont := filepath.Join(wtPath, ".belmont")

	// IMPORTANT: Do NOT remove the existing .belmont/ directory. The worktree
	// inherits the full .belmont/ from HEAD, including all features' state.
	// Removing it would cause git to see those deletions, and merging the
	// worktree branch back would delete other features' state from the main branch.
	//
	// Instead, we overlay the current feature's latest state on top and use git
	// excludes to prevent the worktree from committing changes to other features.
	if err := os.MkdirAll(dstBelmont, 0755); err != nil {
		return fmt.Errorf("create .belmont dir in worktree: %w", err)
	}

	// 1. Copy feature's own state (writable — agent commits these)
	// This overlays the latest state on top of whatever was checked out from HEAD
	srcFeature := filepath.Join(srcBelmont, "features", slug)
	dstFeature := filepath.Join(dstBelmont, "features", slug)
	if dirExists(srcFeature) {
		// Preserve worktree-local STEERING.md / STEERING.log.md (written by
		// `belmont steer` and by consumption) across the wipe-and-recopy.
		// Master never holds these, so without the preserve they would be
		// silently clobbered when auto resumes a preserved worktree.
		var steeringData []byte
		steeringPath := filepath.Join(dstFeature, "STEERING.md")
		if data, err := os.ReadFile(steeringPath); err == nil {
			steeringData = data
		}
		// Remove just this feature's dir to get a clean copy
		os.RemoveAll(dstFeature)
		if err := copyDir(srcFeature, dstFeature); err != nil {
			return fmt.Errorf("copy feature state: %w", err)
		}
		if steeringData != nil {
			if err := os.WriteFile(filepath.Join(dstFeature, "STEERING.md"), steeringData, 0644); err != nil {
				return fmt.Errorf("restore STEERING.md: %w", err)
			}
		}
	}

	// 2. Copy read-only context files (master PRD, PROGRESS, etc.)
	contextFiles := []string{"PRD.md", "PROGRESS.md", "PR_FAQ.md", "TECH_PLAN.md", "worktree.json"}
	for _, f := range contextFiles {
		src := filepath.Join(srcBelmont, f)
		if fileExists(src) {
			copyFile(src, filepath.Join(dstBelmont, f)) // best-effort
		}
	}

	// 3. Copy prompts/ if present (needed for AI decision templates)
	promptsSrc := filepath.Join(srcBelmont, "prompts")
	if dirExists(promptsSrc) {
		copyDir(promptsSrc, filepath.Join(dstBelmont, "prompts")) // best-effort
	}

	// 4. Write .worktree marker file (used by sync to detect worktree context)
	markerPath := filepath.Join(dstBelmont, ".worktree")
	os.WriteFile(markerPath, []byte(slug+"\n"), 0644)

	// 5. Mark TRACKED .belmont/ files as assume-unchanged. This prevents the
	// worktree from committing edits to existing state (which would delete other
	// features' state when merged back). The files remain on disk so the AI agent
	// can read cross-feature state.
	//
	// NEW .belmont/ files are deliberately NOT excluded — they are staged by
	// commitWorktreeChanges' `git add -A` and travel back through the merge. That
	// is load-bearing: MILESTONE.md is created inside the worktree, and it is the
	// only return path on the `belmont recover` route, which does not call
	// syncFeatureStateAfterMerge. When they do collide on a merge they surface as
	// ordinary conflicts and route to the reconciliation agent — there is no
	// .belmont-specific handling. See knowledge/auto-mode/worktree-state-isolation.md.
	//
	// A previous `writeWorktreeGitExcludes` tried to exclude them, but wrote to the
	// per-worktree $GIT_DIR/info/exclude, which git never reads — it resolves
	// info/exclude from $GIT_COMMON_DIR. It was a no-op for its entire life. Do not
	// "fix" it by writing to the common dir: that file is shared with the main repo,
	// where .belmont/ MUST stay tracked. A per-worktree exclude is possible via
	// extensions.worktreeConfig + core.excludesFile, but turning it on would strand
	// worktree state on the recover path. See TestWorktreeNewBelmontFilesAreCommittable.
	untrackBelmontInWorktree(wtPath, slug)

	return nil
}

// untrackBelmontInWorktree marks all .belmont/ files as assume-unchanged in the
// worktree's git index. This prevents git from detecting modifications or deletions
// of .belmont/ files, so they won't be included in commits or merges. The files
// remain on disk for the AI agent to read (cross-feature visibility).
func untrackBelmontInWorktree(wtPath, slug string) {
	// Get list of all .belmont/ files currently in git's index
	lsCmd := exec.Command("git", "ls-files", ".belmont/")
	lsCmd.Dir = wtPath
	out, err := lsCmd.Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Mark every .belmont/ file as assume-unchanged so git ignores
		// any modifications or deletions in this worktree
		cmd := exec.Command("git", "update-index", "--assume-unchanged", line)
		cmd.Dir = wtPath
		cmd.Run() // best-effort
	}
}

// createWorktreeIfNeeded creates the git worktree for a branch and seeds it with
// the feature's .belmont/ state. It is a NO-OP when resuming.
//
// The resume guard is the whole point of this helper existing. Seeding calls
// copyBelmontStateToWorktree, which is a destructive replace of the feature
// dir — run against a preserved worktree it overwrites the live PROGRESS.md
// (the agent's completed tasks) with master's fork-time snapshot, and those
// flips are in no commit anywhere because `.belmont/` is assume-unchanged in
// worktrees. knowledge/auto-mode/resume-rebase.md rejects exactly this under
// "Don't re-do".
//
// This lived inline in two callers that drifted apart: runFeatureInWorktree
// guarded it, runMilestoneInWorktree did not. Keep it in one place so they
// cannot diverge again. See issue #29.
func createWorktreeIfNeeded(root, wtPath, branch, slug string, resumed bool) error {
	if resumed {
		// Skip the state copy — but NOT the untracking. `--assume-unchanged` is
		// index state, and `handleStaleWorktree` can reattach a worktree whose
		// directory was removed by running `git worktree add` again, which
		// builds a fresh index with the bits cleared. Seeding used to re-arm it
		// as a side effect of copyBelmontStateToWorktree; skipping the copy
		// removed the only re-arming. Without it the worktree's `.belmont/`
		// edits become committable and merge back over sibling state — the
		// exact failure worktree-state-isolation.md exists to prevent.
		// Idempotent and cheap, so just always do it.
		untrackBelmontInWorktree(wtPath, slug)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
		return fmt.Errorf("create worktree dir: %w", err)
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if err := copyBelmontStateToWorktree(root, wtPath, slug); err != nil {
		return fmt.Errorf("copy .belmont state to worktree: %w", err)
	}
	// Commit the initial feature state so the AI agent starts from a clean git state
	commitWorktreeFeatureState(wtPath, slug)
	return nil
}

// syncFeatureStateAfterMerge copies the feature's .belmont/ state from a worktree
// back to the main repo after a successful merge. Since .belmont/ is excluded from
// git tracking in worktrees (assume-unchanged), state must be synced separately.
func syncFeatureStateAfterMerge(mainRoot, wtPath, slug string) {
	srcFeature := filepath.Join(wtPath, ".belmont", "features", slug)
	dstFeature := filepath.Join(mainRoot, ".belmont", "features", slug)

	if !dirExists(srcFeature) {
		return
	}

	// PROGRESS.md is merged, not replaced. Every other file in the feature dir
	// still takes the worktree's version wholesale.
	//
	// Why: this function runs once per sibling merge inside runWaveParallel's
	// merge loop, and sibling worktrees each hold a FULL copy of the same
	// feature dir taken at fork time. A destructive replace is therefore
	// last-writer-wins across the wave — M2 merges and master records its
	// tasks done, then M3 merges and master reverts them to their fork-time
	// state. The flips are in no commit anywhere (`.belmont/` is
	// assume-unchanged in worktrees, so this copy is the only transport), so
	// they are unrecoverable. resolveProgressConflict — the union merge built
	// for exactly this — never fires, because assume-unchanged means git never
	// registers a conflict on the file. See issue #24.
	//
	// Reading master's copy BEFORE the wipe and merging it back after mirrors
	// the STEERING.md preservation in copyBelmontStateToWorktree.
	progressPath := filepath.Join(dstFeature, "PROGRESS.md")
	masterProgress, masterReadErr := os.ReadFile(progressPath)

	os.RemoveAll(dstFeature)
	if err := copyDir(srcFeature, dstFeature); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to sync feature state for %s: %s\033[0m\n", slug, err)
		return
	}

	if masterReadErr != nil {
		return // master had no PROGRESS.md yet — the worktree's copy stands
	}
	wtProgress, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}
	merged, warnings := mergeProgressState(string(masterProgress), string(wtProgress))
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ %s: %s\033[0m\n", slug, w)
	}
	if merged != string(wtProgress) {
		if err := os.WriteFile(progressPath, []byte(merged), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to merge PROGRESS.md for %s: %s\033[0m\n", slug, err)
		}
	}
}

// taskBodyEnd returns the index of the last line belonging to the task line at
// idx, extending past its indented continuation lines. An indented line under a
// list item is that item's body — the same Markdown reading that fixed issue
// #31 for `  ## ` headings — so splicing directly after the task line would
// land between a task and its own body, re-attaching the body to the inserted
// line.
//
// The body is the run of lines indented DEEPER than the task line itself. It
// ends at the first line at the task's own indent or shallower — a sibling
// bullet, a column-zero heading, or EOF.
//
// A blank line does NOT end it. A task written as a loose list keeps its
// `**Evidence**` behind a blank line, and stopping there strands exactly the
// prose issue #33 is about: the tail re-attaches to the task now above it while
// the moved task arrives asserting done with half its proof. A blank line never
// extends the block on its own, though — only a following deeper-indented line
// does — so a task at the end of a milestone does not swallow the separator
// before the next heading.
//
// That bound is load-bearing for a task line that is itself indented — a nested
// bullet. Stopping only at column zero would run to the end of the *enclosing*
// list item, so a nested task's "body" would swallow its siblings and its
// parent's own `**Evidence**` lines. `moveTaskLines` then relocates all of it,
// which moves task lines nobody flagged and strips a task that was never named
// of its evidence — the same mis-attribution as issue #33, pointed the other
// way, and in silent violation of repair's "only touch the lines it flagged"
// bound.
//
// A task line at column zero is unaffected: every indented line is deeper than
// indent 0, and every unindented one is not, which is exactly the old rule.
func taskBodyEnd(lines []string, idx int) int {
	if idx < 0 || idx >= len(lines) {
		return idx
	}
	own := lineIndentWidth(lines[idx])
	end := idx
	for j := idx + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			// Keep looking, but do not extend the block yet: a trailing blank
			// belongs to the document, not to the task.
			continue
		}
		if lineIndentWidth(lines[j]) <= own {
			break
		}
		// Deeper-indented: this line, and any blanks passed to reach it, are
		// the task's own body.
		end = j
	}
	return end
}

// lineIndentWidth counts a line's leading whitespace characters. Tabs count as
// one, like spaces: Belmont only needs "deeper than", and a real PROGRESS.md
// does not mix the two within one task's block.
func lineIndentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// dedentBlock shifts every line in a carried block left by `by` characters,
// preserving the block's INTERNAL structure: a line indented deeper than the
// block's own bullet stays deeper by the same amount, so a carried parent keeps
// its own children.
//
// Never removes more whitespace than a line actually has. A blank line, or a
// loose-list separator, has none to give, and clamping is what keeps it blank
// rather than turning it into a stray fragment of the next line's text.
func dedentBlock(block []string, by int) []string {
	if by <= 0 {
		return block
	}
	out := make([]string, len(block))
	for i, l := range block {
		cut := lineIndentWidth(l)
		if cut > by {
			cut = by
		}
		out[i] = l[cut:]
	}
	return out
}

// mergeProgressState reconciles master's PROGRESS.md with a worktree's copy,
// keeping the most-advanced state for every task and every task line either
// side has. Returns the merged content plus any warnings worth printing.
//
// The worktree's document is the base, so structure and ordering are exactly
// what a plain copy would have produced. Master then contributes:
//
//   - a more-advanced marker for any task it already recorded (this is the
//     sibling-merge fix: master's `[x]` beats the worktree's stale `[ ]`)
//   - any task line the worktree's fork-time copy predates entirely, e.g. a
//     follow-up a sibling added to its own milestone after this worktree forked
//
// `[!]` blocked is never overwritten in either direction — it is a human signal
// that something needs attention. An unrecognised marker on either side is left
// exactly as written and reported, never ranked; see issue #27.
//
// Tasks are matched by ID, and the ID is `taskIDShape` — the same definition
// `parseTaskID` and `commitNamedTaskIDs` use. It used to be `P\d+-…` only, which
// meant a hand-written ID like `FWLUP-SWEEP-1` was invisible here: never
// reconciled, and left as whichever side happened to be the base. Since
// `.belmont/` is `--assume-unchanged` in a worktree, this function is the only
// transport home, so "not reconciled" meant "silently took one side" on the one
// path state travels by. For a `P<n>-` prefixed ID the behaviour is unchanged —
// that alternative is first in the alternation. See issue #38.
func mergeProgressState(masterContent, worktreeContent string) (string, []string) {
	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Identity comes from mergeTaskLine — the one definition both merge readers
	// share. It requires the `:` delimiter of the hand-written form only, which
	// is what keeps `OAuth-2 migration` from carrying `OAuth-2` as its identity
	// without stranding a colon-less `P<n>-` line. See its comment.

	// A task is a bullet PLUS its indented body, and anything that relocates one
	// relocates both — the rule `moveTaskLines` follows in `repair` and
	// `resolveProgressConflict` follows on the conflict path. Carrying `line`
	// alone stranded a carried task's `**Verification**` / `**Evidence**` lines
	// on the side they came from, where they re-attach to whatever task now
	// precedes them. Nothing catches that: the file parses, the count is
	// unchanged, and `belmont validate` reports clean. See issues #53 and #33.
	//
	// `parent` is the enclosing task's ID, "" at top level, because a nested
	// bullet's meaning is positional: spliced at the end of its milestone it has
	// silently become a child of whichever task sits last. See issue #47 for the
	// same defect on the conflict path.
	type masterTask struct {
		marker    string
		line      string
		block     []string
		milestone string
		parent    string
	}
	// Tasks are matched by ID, so a duplicated ID makes the match ambiguous:
	// a plain map would silently collapse the entries and merge the wrong
	// line. Count occurrences on both sides and refuse to merge any ID that is
	// not unique — same policy as an unrecognised marker. Belmont does not
	// guess, and it says so.
	//
	// The count must be scoped to the milestones region, because the two walks
	// below are. A task-shaped line under `## Session History` — a sibling
	// logging "- [x] P1-M1-1: done, commit abc123" — belongs to no milestone
	// and is never merged, so counting it made the real in-region task look
	// ambiguous: the refusal then dropped master's recorded completion and the
	// warning told the user to de-duplicate an ID that appears once. Same
	// one-definition-per-concept rule as everywhere else; see isSectionBreak.
	var warnings []string
	dupIDs := map[string]bool{}
	countIDs := func(content string) map[string]int {
		n := map[string]int{}
		currentMS := ""
		for _, line := range strings.Split(content, "\n") {
			if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 2 {
				currentMS = "M" + m[1]
				continue
			}
			if isSectionBreak(line) {
				currentMS = ""
				continue
			}
			if currentMS == "" {
				continue
			}
			if _, id, ok := mergeTaskLine(line); ok {
				n[id]++
			}
		}
		return n
	}
	for _, counts := range []map[string]int{countIDs(masterContent), countIDs(worktreeContent)} {
		for id, n := range counts {
			if n > 1 {
				dupIDs[id] = true
			}
		}
	}

	// A duplicated milestone heading gets the same refusal as a duplicated task
	// ID, for the same reason: lastTaskIdx/lastHeaderIdx are last-writer-wins
	// per milestone, so a header-shaped session note — `### M1: retro notes`
	// under `## Session History`, written mid-run after requireUnambiguousMilestones
	// already passed — would anchor a carried task into the history section, and
	// a task-shaped line quoted below the note would be rewritten as if it were
	// the real task. Every other writer declines on this collision
	// (runScopeGuard, runEvidenceCheck, repair); the merge must not be the one
	// that guesses.
	dupMS := map[string]bool{}
	countMSHeadings := func(content string) map[string]int {
		n := map[string]int{}
		for _, line := range strings.Split(content, "\n") {
			if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 2 {
				n["M"+m[1]]++
			}
		}
		return n
	}
	for _, counts := range []map[string]int{countMSHeadings(masterContent), countMSHeadings(worktreeContent)} {
		for id, n := range counts {
			if n > 1 {
				dupMS[id] = true
			}
		}
	}
	for id := range dupMS {
		warnings = append(warnings, fmt.Sprintf(
			"milestone %s has more than one heading — ambiguous, so its tasks were not merged or carried; de-duplicate the heading", id))
	}

	masterTasks := map[string]masterTask{}
	var masterOrder []string // document order, so the carry-over splice is deterministic
	currentMS := ""
	masterLines := strings.Split(masterContent, "\n")
	// Enclosing task bullets, innermost last, each held open until the line its
	// body ends on.
	//
	// The bound is `taskBodyEnd`'s range and NOT an indent comparison, because
	// the rule this feeds — "a nested task travels inside its parent's block" —
	// is expressed in exactly that range. An indent-only stack disagreed with it
	// and silently dropped a task: a body ends at the first non-blank line at the
	// task's own indent or shallower, so a column-zero note closes it, while an
	// indent stack kept the task open across that note. A deeper bullet written
	// after the note was then recorded as that task's child and skipped as
	// travelling inside a block that does not contain it. That was a real
	// regression on the conflict path, caught by review rather than by the
	// gates; see `324a965` ("Two the nested-carry fix broke, and three claims
	// that were not true").
	type openTask struct {
		end int
		id  string
	}
	var open []openTask
	for i := 0; i < len(masterLines); i++ {
		line := masterLines[i]
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 2 {
			currentMS = "M" + m[1]
			open = open[:0]
			continue
		}
		// A `## ` at column zero ends the milestones region — same rule every
		// other reader uses. Without this the merge is the one reader that
		// disagrees, and a task line under `## Session History` would be
		// attributed to the last milestone header seen and spliced into it.
		// See isSectionBreak and issue #31.
		if isSectionBreak(line) {
			currentMS = ""
			open = open[:0]
			continue
		}
		if marker, id, ok := mergeTaskLine(line); ok {
			for len(open) > 0 && i > open[len(open)-1].end {
				open = open[:len(open)-1]
			}
			parent := ""
			if len(open) > 0 {
				parent = open[len(open)-1].id
			}
			end := taskBodyEnd(masterLines, i)
			// Pushed even for a task this merge will not record, so parentage
			// describes the DOCUMENT rather than the merge decision. Dropping a
			// skipped bullet out of the stack would hand its children the
			// grandparent, quietly re-parenting them one level up.
			open = append(open, openTask{end: end, id: id})
			if dupIDs[id] || currentMS == "" || dupMS[currentMS] {
				continue // orphans belong to no milestone; never carry them into one
			}
			if _, dup := masterTasks[id]; !dup {
				masterOrder = append(masterOrder, id)
			}
			masterTasks[id] = masterTask{
				marker:    marker,
				line:      line,
				block:     append([]string{}, masterLines[i:end+1]...),
				milestone: currentMS,
				parent:    parent,
			}
		}
	}
	for id := range dupIDs {
		warnings = append(warnings, fmt.Sprintf(
			"task %s appears more than once — ambiguous, so its state was not merged; de-duplicate the ID", id))
	}
	if len(masterTasks) == 0 {
		// Still return warnings — every master task may have been skipped as a
		// duplicate, and that is exactly when the user needs telling.
		return worktreeContent, warnings
	}

	// Task IDs this side holds, split by whether they are inside the region.
	//
	// The carry-over pass must key off the IN-REGION set only. Keying off both
	// loses state: when the worktree holds an ID solely as an orphan — a
	// fork-stale line an agent stranded behind a new `## ` heading, or a
	// sibling's completion quoted into a log — master's in-region copy is the
	// only counted one, and skipping it deletes the task from the document.
	// `.belmont/` moves only by this copy, so that is unrecoverable.
	//
	// Splicing it back in is safe: countIDs above is region-scoped, so an
	// in-region line plus an orphan sharing its ID is not a duplicate and the
	// next sibling merge will not refuse it. The stray copy is left exactly as
	// written and reported — never silently reconciled, never silently dropped.
	wtInRegionID := map[string]bool{}
	wtOrphanID := map[string]bool{}
	wtTaskIdx := map[string]int{}     // task ID -> index of its bullet line here
	wtTaskMS := map[string]string{}   // task ID -> the milestone it sits under here
	lastTaskIdx := map[string]int{}   // milestone -> index of its final task line
	lastHeaderIdx := map[string]int{} // milestone -> index of its header line
	out := strings.Split(worktreeContent, "\n")

	currentMS = ""
	for i, line := range out {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 2 {
			currentMS = "M" + m[1]
			lastHeaderIdx[currentMS] = i
			continue
		}
		if isSectionBreak(line) {
			currentMS = ""
			continue
		}
		wtLineMarker, wtLineID, wtLineOK := mergeTaskLine(line)
		if !wtLineOK {
			continue
		}
		if currentMS == "" {
			wtOrphanID[wtLineID] = true
			continue // outside every milestone: leave exactly as written
		}
		id := wtLineID
		wtInRegionID[id] = true
		wtTaskIdx[id] = i
		wtTaskMS[id] = currentMS
		if dupMS[currentMS] {
			// Ambiguous heading: never rewrite a line whose milestone cannot be
			// attributed, and never let it anchor a carry — warned above.
			continue
		}
		lastTaskIdx[currentMS] = i
		mt, ok := masterTasks[id]
		if !ok {
			continue
		}

		wtMarker := wtLineMarker

		// Recognition is checked BEFORE anything else, including the blocked
		// rule. An unrecognised marker must never be rewritten — that is the
		// whole of issue #27, and a `[!]`-wins rule that fired first would
		// happily overwrite a `[?]` the user wrote deliberately.
		wtRank, wtOK := markerRank(wtMarker)
		msRank, msOK := markerRank(mt.marker)
		if !wtOK || !msOK {
			warnings = append(warnings, fmt.Sprintf(
				"task %s has an unrecognised marker ([%s] here, [%s] on main) — left as written, not merged",
				id, wtMarker, mt.marker))
			continue
		}

		// Two states win from EITHER side, in both directions, because neither
		// is progress and rank cannot express either: taskBlocked sorts at -1
		// and taskWithdrawn at -2, so literally anything outranks them.
		//
		// `[!]` is a human-attention signal, and its failure modes are not
		// symmetric: keeping a stale blocker costs someone a look (and auto
		// pauses loudly on blocked tasks), whereas dropping a live one means
		// work is silently treated as fine.
		//
		// `[-]` is a decision, and it is checked BEFORE `[!]` — an unblocked
		// route through cancelled work is not something anyone needs. It wins
		// for `[!]`'s reason plus one more: a withdrawn task that loses a merge
		// comes back as outstanding work, which is exactly what pushes people to
		// delete the line instead, and deletion does not survive this function at
		// all (master's copy is carried back in below). A stale sibling must never
		// silently revive dropped work; reviving it is a deliberate edit.
		//
		// Whichever of the two wins, the state it displaced is reported. This
		// function warns about every other state it declines to merge, and a
		// `[v]` quietly overwritten by a `[-]` is exactly the kind of loss the
		// rest of this file exists to prevent.
		wtState, _ := canonicalMarker(wtMarker)
		msState, _ := canonicalMarker(mt.marker)
		if wtState == taskWithdrawn {
			// Already withdrawn here; leave the line as written. If main still
			// records progress on it, that progress is being discarded — say so.
			// Withdrawal winning is the decision; withdrawal winning SILENTLY is
			// the bug, and this function warns about every other state it
			// declines to merge.
			if msState != taskTodo && msState != taskWithdrawn {
				warnings = append(warnings, fmt.Sprintf(
					"task %s is withdrawn here but [%s] on main — the withdrawal wins, so that state was not merged; re-open the task deliberately if the work really did land",
					id, mt.marker))
			}
			continue
		}
		if msState == taskWithdrawn {
			// Same in reverse, and this is the direction that costs more: the
			// worktree's `.belmont/` moves home only by this copy, so a `[v]`
			// or a `[!]` overwritten here exists in no commit anywhere. It is
			// still the right precedence — a stale sibling must not revive
			// cancelled work — but it must not happen quietly.
			if wtState != taskTodo {
				warnings = append(warnings, fmt.Sprintf(
					"task %s is [%s] here but withdrawn on main — the withdrawal wins, so this state was not merged; re-open the task deliberately if the work really did land",
					id, wtMarker))
			}
			out[i] = strings.Replace(line, "["+wtMarker+"]", "["+mt.marker+"]", 1)
			continue
		}
		if wtState == taskBlocked {
			// Already blocked here; leave the line as written. Master's more
			// advanced state is being discarded, and this is the fourth of the
			// four directions in which a decision beats progress — it warned in
			// three of them and was silent in this one.
			if msState != taskTodo && msState != taskBlocked {
				warnings = append(warnings, fmt.Sprintf(
					"task %s is blocked here but [%s] on main — the blocker wins, so that state was not merged",
					id, mt.marker))
			}
			continue
		}
		if msState == taskBlocked {
			if wtState != taskTodo {
				warnings = append(warnings, fmt.Sprintf(
					"task %s is [%s] here but blocked on main — the blocker wins, so this state was not merged",
					id, wtMarker))
			}
			out[i] = strings.Replace(line, "["+wtMarker+"]", "["+mt.marker+"]", 1)
			continue
		}
		if msRank > wtRank {
			out[i] = strings.Replace(line, "["+wtMarker+"]", "["+mt.marker+"]", 1)
		}
	}

	// Carry over task lines master has that this worktree never saw — e.g. a
	// follow-up a sibling added to its own milestone after this worktree
	// forked. Collect first, keyed by the milestone's final task line, then
	// splice in one pass: computing indices as we mutate would mis-place
	// insertions whenever the two documents order milestones differently.
	// A task whose PARENT is also being carried travels inside that parent's
	// block, exactly as `moveTaskLines` relocates a nested bullet with the block
	// enclosing it. Carrying both would splice the child twice, and a duplicated
	// ID is durable damage rather than cosmetic: countIDs above refuses to
	// reconcile that task on every later merge and tells the user to
	// de-duplicate by hand.
	missingHere := map[string]bool{}
	for _, id := range masterOrder {
		if !wtInRegionID[id] {
			missingHere[id] = true
		}
	}

	// Each carried block is filed against the index in `out` it goes after, and
	// `depth` records what kind of anchor that is so a collision can be broken.
	// Two anchors are the SAME index whenever a parent is the last task in its
	// milestone here, and then the order inside the bucket decides PARENTAGE
	// rather than merely tidiness: a new top-level task emitted first would take
	// a carried child with it, which is the re-parenting the parent anchor exists
	// to prevent. Deeper anchors are emitted first, ties in document order.
	type carriedBlock struct {
		depth int // indent of the anchoring task line here; -1 for a milestone anchor
		lines []string
	}
	pending := map[int][]carriedBlock{}
	carried := 0
	for _, id := range masterOrder {
		if wtInRegionID[id] {
			continue // reconciled in place by the walk above
		}
		mt := masterTasks[id]
		// Anchor after the milestone's last task line — past its indented body,
		// so the carry cannot land between a task and its own continuation — or
		// after its header when the milestone exists here but holds no tasks
		// yet: otherwise the first task a sibling adds to an empty milestone
		// has nowhere to land and is dropped from the only copy that exists.
		at, ok := lastTaskIdx[mt.milestone]
		if ok {
			at = taskBodyEnd(out, at)
		} else {
			at, ok = lastHeaderIdx[mt.milestone]
		}
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"task %s exists on main under %s, which this worktree's PROGRESS.md does not contain — not merged",
				id, nonEmpty(mt.milestone, "an unknown milestone")))
			continue
		}
		// A child travels inside its parent's block, so it is not carried in its
		// own right — but this skip has to run AFTER the anchor lookup, not
		// before it. Ordered first, it swallowed every descendant of a carry that
		// then failed: master's M2 nesting P0-M2-1 > P0-M2-2 > P0-M2-3 against a
		// worktree with no `### M2:` heading warned about P0-M2-1 alone, where
		// this function warns about each of the three. The tasks are lost either
		// way — that is the milestone being absent, not this skip — but
		// `.belmont/` is `--assume-unchanged` in a worktree, so those tasks are
		// in no commit and the warning is the only record that they existed.
		// "Skipped with a warning, never silently dropped" is the guarantee, and
		// it is about the naming, not the count. A child's milestone is its
		// parent's (it is parsed from inside the parent's block), so whenever the
		// parent's anchor lookup failed the child's has too, and it reaches the
		// warning above rather than this line.
		if missingHere[mt.parent] {
			continue
		}
		depth := -1
		if mt.parent != "" && wtInRegionID[mt.parent] {
			if wtTaskMS[mt.parent] != mt.milestone {
				// The two sides file the parent under different milestones, so
				// nesting the child under it would move the child too — and a task
				// whose ID names another milestone is `cross_milestone_task_id`,
				// severityError. Fall back to the milestone anchor and say so,
				// rather than relocating work across milestones on a guess.
				warnings = append(warnings, fmt.Sprintf(
					"task %s is nested under %s on main, which this worktree files under %s rather than %s — carried to the end of %s at top level instead of under its parent",
					id, mt.parent, nonEmpty(wtTaskMS[mt.parent], "no milestone"), mt.milestone, mt.milestone))
			} else {
				// Land it inside the parent's body, past the parent's own
				// `**Verification**` / `**Evidence**` lines. Appending at the end
				// of the milestone instead re-parents it to whichever task sits
				// last.
				at = taskBodyEnd(out, wtTaskIdx[mt.parent])
				depth = lineIndentWidth(out[wtTaskIdx[mt.parent]])
			}
		}
		// The block is the bullet plus its body, and that body can hold nested
		// task bullets of its own. Only the block's own ID was checked against
		// this side, so a nested bullet whose ID already exists here would be
		// spliced in a second time — the duplicate-ID damage described above.
		// Excise it instead of skipping the whole carry: the nested task's state
		// is already reconciled in place by the walk above, so nothing is lost,
		// whereas dropping the parent would lose work held in no commit.
		block, dropped := exciseKnownNestedTasks(mt.block, wtInRegionID)
		for _, d := range dropped {
			warnings = append(warnings, fmt.Sprintf(
				"task %s on main nests %s, which already exists here in its own right — %s was carried without it, and the copy here was left as written",
				id, d, id))
		}
		// A block that was written nested on main but is landing on the MILESTONE
		// anchor (depth -1) must be flattened to top level first. Splicing it with
		// main's indentation intact makes it a child of whichever task happens to
		// sit last in that milestone — the exact re-parenting the parent anchor
		// above exists to prevent, arrived at from the other direction. Two inputs
		// reach here: a parent filed under a different milestone on each side
		// (warned above, and the warning says "at top level" because of this), and
		// a parent whose ID is duplicated on main, which never enters `masterOrder`
		// so neither `missingHere` nor `wtInRegionID` is true of it.
		if depth == -1 {
			if by := lineIndentWidth(block[0]); by > 0 {
				block = dedentBlock(block, by)
				if !(mt.parent != "" && wtInRegionID[mt.parent]) {
					// The other input, which had no warning of its own: main nests
					// it under a task this side cannot resolve, so there is no
					// parent to land under and no ambiguity report that mentions
					// placement. Say where it actually went.
					warnings = append(warnings, fmt.Sprintf(
						"task %s is nested under %s on main, which this worktree cannot place — %s was carried to the end of %s at top level",
						id, nonEmpty(mt.parent, "another task"), id, nonEmpty(mt.milestone, "its milestone")))
				}
			}
		}
		pending[at] = append(pending[at], carriedBlock{depth: depth, lines: block})
		carried++
		if wtOrphanID[id] {
			warnings = append(warnings, fmt.Sprintf(
				"task %s is recorded under %s on main but sits outside every milestone here — main's copy was placed under %s and the stray line left as written; move or delete the stray one",
				id, nonEmpty(mt.milestone, "an unknown milestone"), nonEmpty(mt.milestone, "its milestone")))
		}
	}
	if carried == 0 {
		return strings.Join(out, "\n"), warnings
	}

	merged := make([]string, 0, len(out)+len(masterTasks))
	for i, line := range out {
		merged = append(merged, line)
		blocks := pending[i]
		// Stable, so equal-depth blocks keep the order the master document wrote
		// them in.
		sort.SliceStable(blocks, func(a, b int) bool { return blocks[a].depth > blocks[b].depth })
		for _, b := range blocks {
			merged = append(merged, b.lines...)
		}
	}
	return strings.Join(merged, "\n"), warnings
}

// exciseKnownNestedTasks removes, from a carried block, any nested task bullet
// (and that bullet's own body) whose ID the receiving side already holds.
//
// Returns the trimmed block and the IDs removed. The block's first line is the
// carried task's own bullet and is never considered — the caller has already
// established that this side does not have it.
//
// The extent removed is `taskBodyEnd`'s range, not "the bullet line", for the
// same reason the carry itself moves a range: a bullet excised without its body
// leaves that body behind to re-attach to the task above it.
func exciseKnownNestedTasks(block []string, known map[string]bool) ([]string, []string) {
	var dropped []string
	out := make([]string, 0, len(block))
	for i := 0; i < len(block); i++ {
		if i > 0 {
			if _, id, ok := mergeTaskLine(block[i]); ok && known[id] {
				dropped = append(dropped, id)
				i = taskBodyEnd(block, i)
				continue
			}
		}
		out = append(out, block[i])
	}
	return out, dropped
}

// commitWorktreeChanges commits all uncommitted changes in a worktree before merge.
// AI agents may leave uncommitted work (code changes, state files) when the loop
// completes. Without this, git merge --no-ff only sees committed changes and the
// worktree's working directory changes are silently lost.
func commitWorktreeChanges(wtPath, label string) error {
	// Check for any uncommitted changes (tracked + untracked)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = wtPath
	out, err := statusCmd.Output()
	if err != nil {
		return nil // can't check, skip gracefully
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil // nothing to commit
	}

	// Stage everything
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = wtPath
	if _, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A in worktree: %w", err)
	}

	// Commit
	commitCmd := exec.Command("git", "commit", "-m", fmt.Sprintf("belmont: finalize %s", label))
	commitCmd.Dir = wtPath
	if _, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit in worktree: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  \033[2m(committed uncommitted changes for %s)\033[0m\n", label)
	return nil
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

// wave represents a group of milestones that can execute in parallel.
type wave struct {
	Index      int
	Milestones []milestone
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

func findWorktree(worktrees []worktreeEntry, slug string) *worktreeEntry {
	for _, wt := range worktrees {
		if filepath.Base(wt.Path) == slug {
			return &wt
		}
	}
	return nil
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
