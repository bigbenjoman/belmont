package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `belmont recover` manages worktrees PRESERVED FROM A FAILED MERGE. It finds
// them by scanning a directory — the same directory `runAutoParallel` creates the
// live wave's worktrees in — with no notion of an active run. So mid-run it
// reports the running wave's worktrees as preserved, and `--clean-all` would
// delete them.
//
// What that costs is not "some commits": `.belmont/` is `--assume-unchanged`
// inside a worktree, so every task state the wave has completed but not yet
// merged is in no commit anywhere. Removing the worktree takes it with them.
//
// `.belmont/auto.json` already records the active run and its worktree paths, so
// the information was there the whole time. See issue #52.

// recoverFixture builds a project root holding one worktree-shaped directory
// under the legacy scan path, and (when active) an auto.json claiming it.
func recoverFixture(t *testing.T, active bool) (root, wtPath, slug string) {
	t.Helper()
	root = t.TempDir()
	// Point HOME at a scratch dir so worktreeBasePath cannot reach the real
	// ~/.belmont/worktrees and pick up this developer's actual worktrees.
	t.Setenv("HOME", t.TempDir())

	slug = "M1"
	wtPath = filepath.Join(root, ".belmont", "worktrees", slug)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	// scanWorktreeDir identifies a worktree by the presence of a `.git` entry.
	if err := os.WriteFile(filepath.Join(wtPath, ".git"), []byte("gitdir: /nowhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !active {
		return root, wtPath, slug
	}
	aj := autoJSON{
		Active:  true,
		Started: "2026-08-15T00:00:00Z",
		Mode:    "single-feature-parallel",
		Feature: "demo",
		Worktrees: map[string]autoJSONEntry{
			slug: {Path: wtPath, Branch: "belmont/auto/" + slug},
		},
	}
	data, err := json.MarshalIndent(aj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".belmont", "auto.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, wtPath, slug
}

func TestRecoverRefusesCleanAllDuringAnActiveRun(t *testing.T) {
	root, wtPath, _ := recoverFixture(t, true)

	err := runRecover([]string{"--root", root, "--clean-all"})
	if err == nil {
		t.Fatal("recover --clean-all succeeded during an active run; it would have deleted the running wave's worktrees")
	}
	if !strings.Contains(err.Error(), "demo") {
		t.Errorf("the refusal must name the running feature so the user knows what is in flight; got: %v", err)
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("the live worktree was deleted despite the refusal: %v", statErr)
	}
}

func TestRecoverRefusesToCleanAWorktreeTheRunOwns(t *testing.T) {
	root, wtPath, slug := recoverFixture(t, true)

	err := runRecover([]string{"--root", root, "--clean", slug})
	if err == nil {
		t.Fatal("recover --clean succeeded on a worktree the active run owns")
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("the live worktree was deleted despite the refusal: %v", statErr)
	}
}

// --retry sits on the same footing, which the issue asked to check: merging a
// worktree the wave still owns races the wave's own merge of it.
func TestRecoverRefusesToRetryAMergeTheRunOwns(t *testing.T) {
	root, _, slug := recoverFixture(t, true)

	err := runRecover([]string{"--root", root, "--merge", slug})
	if err == nil {
		t.Fatal("recover --merge succeeded on a worktree the active run owns")
	}
	// A bare error is not enough: this fixture is not a real git worktree, so
	// --merge fails anyway. The test only means something if it fails for THIS
	// reason, named.
	if !strings.Contains(err.Error(), "demo") {
		t.Errorf("recover --merge failed, but not because the run owns the worktree — "+
			"the refusal must name the running feature; got: %v", err)
	}
}

// --list is read-only, so it must not refuse — it must tell the truth. Reporting
// a live worktree under the heading "preserved worktrees" is what invites the
// destructive command in the first place.
func TestRecoverListLabelsLiveWorktreesRatherThanRefusing(t *testing.T) {
	root, _, _ := recoverFixture(t, true)

	out := captureStdout(t, func() {
		if err := runRecover([]string{"--root", root, "--list"}); err != nil {
			t.Fatalf("recover --list refused during an active run; a read-only command must still report: %v", err)
		}
	})
	if !strings.Contains(out, "demo") {
		t.Errorf("the listing did not name the running feature.\ngot:\n%s", out)
	}
	// The fixture has exactly one live worktree, which is also the common case —
	// by the time anyone reaches for `recover` a wave usually has one left. So
	// the singular is the form most readers actually see, and it has to read as
	// English: "1 of these belongs … and is NOT preserved", never "belong … are".
	if !strings.Contains(out, "1 of these belongs") || !strings.Contains(out, "is NOT preserved") {
		t.Errorf("the one-worktree trailer does not agree with itself grammatically.\ngot:\n%s", out)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "in flight") && !strings.Contains(low, "active") && !strings.Contains(low, "running") {
		t.Errorf("the listing did not mark the worktree as belonging to a run still in flight.\ngot:\n%s", out)
	}
}

// The guard against over-correcting: with no run active, recover must behave
// exactly as before. A permanently-refusing recover would be worse than the bug,
// because recover IS the recovery path.
func TestRecoverStillCleansWhenNoRunIsActive(t *testing.T) {
	root, wtPath, slug := recoverFixture(t, false)

	if err := runRecover([]string{"--root", root, "--clean", slug}); err != nil {
		t.Fatalf("recover --clean failed with no active run: %v", err)
	}
	if _, statErr := os.Stat(wtPath); statErr == nil {
		t.Error("recover --clean left the worktree in place when no run was active")
	}
}

// A run killed with SIGKILL leaves auto.json behind with active:true, and that
// must not brick the recovery path forever. --force is the documented way past
// the guard.
func TestRecoverForceOverridesTheActiveRunGuard(t *testing.T) {
	root, wtPath, slug := recoverFixture(t, true)

	if err := runRecover([]string{"--root", root, "--clean", slug, "--force"}); err != nil {
		t.Fatalf("recover --clean --force failed during an active run: %v", err)
	}
	if _, statErr := os.Stat(wtPath); statErr == nil {
		t.Error("--force did not override the active-run guard")
	}
}
