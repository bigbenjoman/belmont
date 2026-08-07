package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorktreeRepo creates a repo with committed .belmont/ feature state and a
// linked worktree, then applies the same state isolation copyBelmontStateToWorktree
// applies. Returns (mainRoot, worktreePath).
func setupWorktreeRepo(t *testing.T, slug string) (string, string) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "commit.gpgsign", "false")

	feature := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(feature, 0755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	mustWrite(t, filepath.Join(feature, "TECH_PLAN.md"), "approved contract\n")
	mustWrite(t, filepath.Join(root, "app.txt"), "code\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-qm", "init")

	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, root, "worktree", "add", "-q", wt, "-b", "belmont/auto/"+slug)

	// Mirror what copyBelmontStateToWorktree does for isolation.
	untrackBelmontInWorktree(wt, slug)
	return root, wt
}

// TestWorktreeTrackedBelmontEditsAreNotCommittable pins the half of the isolation
// that works: an agent editing an already-tracked .belmont/ file inside a worktree
// cannot commit that edit to the feature branch.
func TestWorktreeTrackedBelmontEditsAreNotCommittable(t *testing.T) {
	_, wt := setupWorktreeRepo(t, "demo")
	tracked := filepath.Join(wt, ".belmont", "features", "demo", "TECH_PLAN.md")
	mustWrite(t, tracked, "AGENT OVERWROTE THE APPROVED CONTRACT\n")

	runGit(t, wt, "add", "-A")
	staged := runGit(t, wt, "diff", "--cached", "--name-only")
	if strings.Contains(staged, "TECH_PLAN.md") {
		t.Fatalf("assume-unchanged failed: worktree staged an edit to tracked .belmont state\nstaged:\n%s", staged)
	}
}

// TestWorktreeNewBelmontFilesAreCommittable pins the OTHER half — deliberately.
//
// New .belmont/ files created inside a worktree ARE staged and committed. That is
// not an oversight: MILESTONE.md is created in the worktree, and on the `belmont
// recover` route this git path is the ONLY way that state returns to main, because
// recoverMerge does not call syncFeatureStateAfterMerge.
//
// A removed helper (writeWorktreeGitExcludes) once tried to exclude these files. It
// wrote to the per-worktree $GIT_DIR/info/exclude, which git never reads — it
// resolves info/exclude from $GIT_COMMON_DIR — so it was a no-op for its entire
// life and the behaviour below is what Belmont has always actually done.
//
// If this test starts failing, someone has made worktree excludes effective. Before
// accepting that, give recoverMerge a syncFeatureStateAfterMerge call, or worktree
// state will be silently stranded on the recover path. Note also that the common
// info/exclude is shared with the main repo, where .belmont/ must stay tracked.
func TestWorktreeNewBelmontFilesAreCommittable(t *testing.T) {
	_, wt := setupWorktreeRepo(t, "demo")
	milestone := filepath.Join(wt, ".belmont", "features", "demo", "MILESTONE.md")
	mustWrite(t, milestone, "# Milestone M1\n")

	runGit(t, wt, "add", "-A")
	staged := runGit(t, wt, "diff", "--cached", "--name-only")
	if !strings.Contains(staged, "MILESTONE.md") {
		t.Fatalf("new .belmont file was NOT staged — worktree excludes have become "+
			"effective. This strands state on the `belmont recover` path, which has no "+
			"syncFeatureStateAfterMerge call. See the comment on this test.\nstaged:\n%s", staged)
	}
}
