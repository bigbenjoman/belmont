package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Issue #48 — the live overlay must not fall back to master in silence
// ---------------------------------------------------------------------------

// overlayFixture builds a single-feature-parallel run with two milestone
// worktrees. Whatever `wt2Progress` returns is what M2's worktree holds: ""
// means the file is absent entirely, which is the half-cleaned-worktree case.
func overlayFixture(t *testing.T, wt2Progress string) (root, wt1, wt2 string) {
	t.Helper()
	root = t.TempDir()
	wt1 = filepath.Join(root, "wt-m1")
	wt2 = filepath.Join(root, "wt-m2")
	write := func(dir, content string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if content == "" {
			return // the directory survives; its PROGRESS.md does not
		}
		if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	clean := "### M1: One\n- [ ] P1-M1-1: a\n\n### M2: Two\n- [ ] P1-M2-1: b\n"
	featureDir := filepath.Join(root, ".belmont", "features", "auth")
	write(featureDir, clean)
	if err := os.WriteFile(filepath.Join(featureDir, "PRD.md"), []byte("# PRD: Auth\n"), 0644); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(wt1, ".belmont", "features", "auth"), clean)
	write(filepath.Join(wt2, ".belmont", "features", "auth"), wt2Progress)

	blob, err := json.Marshal(autoJSON{
		Active:  true,
		Mode:    "single-feature-parallel",
		Feature: "auth",
		Worktrees: map[string]autoJSONEntry{
			"M1": {Path: wt1, Branch: "b1"},
			"M2": {Path: wt2, Branch: "b2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".belmont", "auto.json"), blob, 0644); err != nil {
		t.Fatal(err)
	}
	return root, wt1, wt2
}

// The overlay's two fallbacks used to be silent, and since #42 and #44 they
// decide whether a GATE passes: `belmont validate` prints "✓ No
// milestone-structure violations found" and `belmont auto` starts, on a file the
// lint only partly saw. A false green in the one place a false green starts a
// run.
func TestValidateReportsAWorktreeItCouldNotRead(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wt2Progress string
	}{
		{"PROGRESS.md is gone", ""},
		{"the worktree does not hold this milestone", "### M1: One\n- [ ] P1-M1-1: a\n"},
	} {
		root, _, _ := overlayFixture(t, tc.wt2Progress)
		v, err := validateFeature(root, "auth")
		if err != nil {
			t.Fatalf("%s: validateFeature: %v", tc.name, err)
		}
		var found *validationViolation
		for i := range v {
			if v[i].Rule == ruleUnreadableLiveMilestone {
				found = &v[i]
			}
		}
		if found == nil {
			t.Errorf("%s: validate reported a clean file while sourcing M2 from master's stale copy. Violations seen: %+v", tc.name, v)
			continue
		}
		if found.Milestone != "M2" {
			t.Errorf("%s: violation names milestone %q, want M2", tc.name, found.Milestone)
		}
		// Warning, not error: a half-cleaned worktree must not refuse a run that
		// worked yesterday. `--strict` is what makes CI fail on it.
		if found.Severity != severityWarning {
			t.Errorf("%s: severity = %q, want %q — an unreadable worktree is information Belmont refuses to drop, not a reason to stop the loop",
				tc.name, found.Severity, severityWarning)
		}
		blocking, advisory := splitBySeverity(v)
		if len(blocking) > 0 {
			t.Errorf("%s: %d blocking violation(s) — this must not stop `belmont auto`: %+v", tc.name, len(blocking), blocking)
		}
		if len(advisory) == 0 {
			t.Errorf("%s: nothing advisory, so `belmont validate --strict` still exits 0 on a file it did not fully read", tc.name)
		}
	}
}

// The detail view is where a reader goes to check a milestone's state. Showing
// master's stale copy under a live-run banner, with nothing said, is the same
// class of quiet substitution.
func TestStatusNamesTheMilestoneItFellBackToMaster(t *testing.T) {
	root, _, _ := overlayFixture(t, "")
	report, err := buildStatus(root, 0, "auth")
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}
	out := renderStatus(report, false, false)
	if !strings.Contains(out, "M2") || !strings.Contains(out, "could not be read") {
		t.Errorf("`belmont status --feature auth` never says M2 is being shown from master because its worktree could not be read:\n%s", out)
	}
}

// Listing mode is the half an agent actually reads (`status.md`'s fast path runs
// it with no --feature), so every "this is not what it looks like" signal has to
// appear here too.
func TestListingWarnsWhenAWorktreeCouldNotBeRead(t *testing.T) {
	root, wt1, wt2 := overlayFixture(t, "")
	featuresDir := filepath.Join(root, ".belmont", "features")
	features := listFeaturesWithOverrides(featuresDir, 40,
		map[string]string{"auth": filepath.Join(wt1, ".belmont", "features", "auth")}, "auth",
		map[string]string{
			"M1": filepath.Join(wt1, ".belmont", "features", "auth"),
			"M2": filepath.Join(wt2, ".belmont", "features", "auth"),
		})
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	out := renderFeatureListing(statusReport{Features: features, TaskCounts: map[string]int{}}, false, false)
	if !strings.Contains(out, "M2") || !strings.Contains(out, "could not be read") {
		t.Errorf("the listing shows M2 from master's stale copy without saying so:\n%s", out)
	}
}

// `belmont blockers` already treats a half-cleaned worktree as a case worth
// stating — `Skipped` names a feature whose PROGRESS.md could not be read at
// all. A milestone whose worktree cannot be read is the same scenario arriving
// one level down, and it went unmentioned.
func TestBlockersReportsAWorktreeItCouldNotRead(t *testing.T) {
	root, _, _ := overlayFixture(t, "")
	report, err := buildBlockers(root, "auth")
	if err != nil {
		t.Fatalf("buildBlockers: %v", err)
	}
	out := renderBlockers(report, false, false)
	if !strings.Contains(out, "M2") || !strings.Contains(out, "could not be read") {
		t.Errorf("`belmont blockers` answered from master's stale copy of M2 without saying so:\n%s", out)
	}
}
