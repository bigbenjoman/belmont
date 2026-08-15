package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #48 made `overlayLiveMilestones` return the milestones it could not source from
// their worktrees, so every reader states what it could not read instead of
// quietly substituting master's fork-point copy. That covers the
// SINGLE-FEATURE-PARALLEL path, where each milestone has its own worktree.
//
// On the SERIAL / MULTI-FEATURE path one worktree owns the whole feature, and
// `listFeaturesWithOverrides` swaps the feature's directory for that worktree's.
// The read there had no `else`: an unreadable PROGRESS.md left `milestones` nil,
// so the row rendered `0/0` — and unlike the parallel path there was no fallback
// to master either, so there was no baseline behind the number.
//
// `0/0` is the worst available rendering of "I could not read this". It is
// indistinguishable from a feature with no tasks, and it reads as FINISHED
// NOTHING rather than KNOW NOTHING. Listing mode is also what `status.md`'s fast
// path runs, so it is the half an agent sees. See issue #55.

// listingFixture writes a master feature with four tasks across two milestones,
// and returns the features dir plus a worktree path whose PROGRESS.md is absent.
func listingFixture(t *testing.T) (featuresDir, brokenWorktreeFeatureDir string) {
	t.Helper()
	root := t.TempDir()
	featuresDir = filepath.Join(root, ".belmont", "features")
	masterFeature := filepath.Join(featuresDir, "demo")
	if err := os.MkdirAll(masterFeature, 0o755); err != nil {
		t.Fatal(err)
	}
	progress := `# Progress: demo

## Milestones

### M1: Parsing
- [v] P0-M1-1: Parse the header
- [v] P0-M1-2: Parse the body

### M2: Rendering
- [x] P0-M2-1: Render the header
- [ ] P0-M2-2: Render the body
`
	if err := os.WriteFile(filepath.Join(masterFeature, "PROGRESS.md"), []byte(progress), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(masterFeature, "PRD.md"), []byte("# PRD: Demo Feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The worktree's feature dir exists but holds no PROGRESS.md — the shape a
	// half-created or half-cleaned worktree leaves behind.
	brokenWorktreeFeatureDir = filepath.Join(root, "wt", ".belmont", "features", "demo")
	if err := os.MkdirAll(brokenWorktreeFeatureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return featuresDir, brokenWorktreeFeatureDir
}

func TestListingDoesNotRenderZeroWhenTheWorktreeProgressCannotBeRead(t *testing.T) {
	featuresDir, broken := listingFixture(t)

	got := listFeaturesWithOverrides(featuresDir, 55, map[string]string{"demo": broken}, "", nil)
	if len(got) != 1 {
		t.Fatalf("expected one feature, got %d", len(got))
	}
	f := got[0]

	if f.TasksTotal == 0 && f.MilestonesTotal == 0 {
		t.Errorf("feature rendered as 0/0 — indistinguishable from a feature with no tasks, and it reads as "+
			"finished nothing rather than know nothing. Want master's copy as the baseline (4 tasks, 2 milestones). "+
			"got TasksTotal=%d MilestonesTotal=%d", f.TasksTotal, f.MilestonesTotal)
	}
	if f.TasksTotal != 4 || f.MilestonesTotal != 2 {
		t.Errorf("fell back to something other than master's copy: TasksTotal=%d (want 4) MilestonesTotal=%d (want 2)",
			f.TasksTotal, f.MilestonesTotal)
	}
	if f.TasksVerified != 2 {
		t.Errorf("TasksVerified=%d, want 2 — the fallback must be master's real state, not an empty one", f.TasksVerified)
	}
}

// Falling back silently would be the #48 failure with a different shape: the
// number is now plausible, which makes trusting it worse than a visible zero.
func TestListingReportsTheGapWhenItFallsBackToMaster(t *testing.T) {
	featuresDir, broken := listingFixture(t)

	got := listFeaturesWithOverrides(featuresDir, 55, map[string]string{"demo": broken}, "", nil)
	if len(got) != 1 {
		t.Fatalf("expected one feature, got %d", len(got))
	}
	if len(got[0].LiveGaps) == 0 {
		t.Fatal("the fallback to master was silent — no LiveGap recorded, so every reader reports the counts as live")
	}
	g := got[0].LiveGaps[0]
	if g.Kind != gapUnreadable {
		t.Errorf("gap Kind = %q, want %q", g.Kind, gapUnreadable)
	}
	if !strings.Contains(g.Path, "wt") {
		t.Errorf("gap Path = %q, want the worktree path that could not be read", g.Path)
	}
	// One phrasing, not three: the same describe() the unknown-marker and orphan
	// warnings already share in this view.
	if !strings.Contains(g.describe(), "master's copy is shown instead") {
		t.Errorf("gap does not use the shared phrasing: %q", g.describe())
	}
}

// The listing is what `status.md`'s fast path runs, so the warning has to reach
// the rendered text and not merely the struct.
func TestListingRendersTheWorktreeGapWarning(t *testing.T) {
	featuresDir, broken := listingFixture(t)
	features := listFeaturesWithOverrides(featuresDir, 55, map[string]string{"demo": broken}, "", nil)

	out := renderFeatureListing(statusReport{Features: features}, false, false)
	if !strings.Contains(out, "master's copy") {
		t.Errorf("the listing did not warn that the counts came from master rather than the worktree.\ngot:\n%s", out)
	}
	if !strings.Contains(out, "demo") {
		t.Errorf("the warning did not name the feature.\ngot:\n%s", out)
	}
}

// Guard against over-correcting in two directions at once.
//
// A readable worktree must be used as-is with no gap, and a feature with NO
// override must behave exactly as before — including an archived feature, whose
// PROGRESS.md is legitimately absent and which must not start warning.
func TestListingLeavesTheNormalPathsAlone(t *testing.T) {
	featuresDir, _ := listingFixture(t)

	t.Run("readable worktree wins and reports no gap", func(t *testing.T) {
		wtFeature := filepath.Join(t.TempDir(), ".belmont", "features", "demo")
		if err := os.MkdirAll(wtFeature, 0o755); err != nil {
			t.Fatal(err)
		}
		live := `# Progress: demo

## Milestones

### M1: Parsing
- [v] P0-M1-1: Parse the header
`
		if err := os.WriteFile(filepath.Join(wtFeature, "PROGRESS.md"), []byte(live), 0o644); err != nil {
			t.Fatal(err)
		}
		got := listFeaturesWithOverrides(featuresDir, 55, map[string]string{"demo": wtFeature}, "", nil)
		if got[0].TasksTotal != 1 {
			t.Errorf("the readable worktree copy was not used: TasksTotal=%d, want 1", got[0].TasksTotal)
		}
		if len(got[0].LiveGaps) != 0 {
			t.Errorf("a readable worktree produced a gap: %+v", got[0].LiveGaps)
		}
	})

	t.Run("no override reports no gap", func(t *testing.T) {
		got := listFeaturesWithOverrides(featuresDir, 55, nil, "", nil)
		if len(got[0].LiveGaps) != 0 {
			t.Errorf("a feature with no worktree override produced a gap: %+v", got[0].LiveGaps)
		}
		if got[0].TasksTotal != 4 {
			t.Errorf("TasksTotal=%d, want 4", got[0].TasksTotal)
		}
	})

	t.Run("archived feature stays quiet", func(t *testing.T) {
		root := t.TempDir()
		fd := filepath.Join(root, ".belmont", "features", "old")
		if err := os.MkdirAll(fd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fd, "ARCHIVE.md"), []byte("# Archive: Old Thing\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := listFeaturesWithOverrides(fd[:strings.LastIndex(fd, string(filepath.Separator))], 55, nil, "", nil)
		if len(got) != 1 {
			t.Fatalf("expected one feature, got %d", len(got))
		}
		if len(got[0].LiveGaps) != 0 {
			t.Errorf("an archived feature with no PROGRESS.md produced a gap: %+v", got[0].LiveGaps)
		}
	})
}
