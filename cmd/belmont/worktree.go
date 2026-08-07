package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// syncFeatureStateAfterMerge copies the feature's .belmont/ state from a worktree
// back to the main repo after a successful merge. Since .belmont/ is excluded from
// git tracking in worktrees (assume-unchanged), state must be synced separately.
func syncFeatureStateAfterMerge(mainRoot, wtPath, slug string) {
	srcFeature := filepath.Join(wtPath, ".belmont", "features", slug)
	dstFeature := filepath.Join(mainRoot, ".belmont", "features", slug)

	if !dirExists(srcFeature) {
		return
	}

	// Replace the main repo's feature state with the worktree's version
	os.RemoveAll(dstFeature)
	if err := copyDir(srcFeature, dstFeature); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to sync feature state for %s: %s\033[0m\n", slug, err)
	}
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
