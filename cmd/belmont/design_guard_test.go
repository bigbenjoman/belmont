package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The design authority is written by one interactive skill and read by
// everything else. These tests pin the four states the guard has to tell apart:
// untouched, edited, invented, and absent-throughout.

const designContractFixture = `# UX Design: about

## Design Contract
**Mode**: derived — UI, no Figma
**Approval**: approved 2026-08-08
`

// designGuardEnv lays out a feature dir and returns (root, featurePath).
func designGuardEnv(t *testing.T, slug string) (string, string) {
	t.Helper()
	root := t.TempDir()
	featureDir := filepath.Join(root, ".belmont", "features", slug)
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(featureDir, "UX_DESIGN.md")
}

func steeringBody(t *testing.T, root, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".belmont", "features", slug, "STEERING.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

func TestDesignAuthorityGuard_UnchangedFileIsLeftAlone(t *testing.T) {
	root, path := designGuardEnv(t, "about")
	if err := os.WriteFile(path, []byte(designContractFixture), 0644); err != nil {
		t.Fatal(err)
	}
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	// Agent "runs" and touches nothing.
	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionImplementMilestone, MilestoneID: "M1"}, pre)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != designContractFixture {
		t.Errorf("guard rewrote an untouched file:\n%s", after)
	}
	if body := steeringBody(t, root, "about"); body != "" {
		t.Errorf("guard steered on a no-op phase:\n%s", body)
	}
}

func TestDesignAuthorityGuard_ModifiedFileIsReverted(t *testing.T) {
	root, path := designGuardEnv(t, "about")
	if err := os.WriteFile(path, []byte(designContractFixture), 0644); err != nil {
		t.Fatal(err)
	}
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	// Agent "runs" and restamps the approval — the exact edit the rule forbids.
	edited := strings.Replace(designContractFixture,
		"**Approval**: approved 2026-08-08",
		"**Approval**: approved 2026-08-11 (re-derived during implement)", 1)
	if edited == designContractFixture {
		t.Fatal("fixture no longer contains the approval line this test rewrites")
	}
	if err := os.WriteFile(path, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionImplementMilestone, MilestoneID: "M1"}, pre)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != designContractFixture {
		t.Errorf("guard did not restore the pre-phase contract:\n%s", after)
	}
	body := steeringBody(t, root, "about")
	if !strings.Contains(body, "(pending)") {
		t.Errorf("steering entry is not pending:\n%s", body)
	}
	if !strings.Contains(body, ".belmont/features/about/UX_DESIGN.md") {
		t.Errorf("steering entry does not name the reverted file:\n%s", body)
	}
	if !strings.Contains(body, "modified") {
		t.Errorf("steering entry does not say what the phase did:\n%s", body)
	}
}

func TestDesignAuthorityGuard_CreatedFileIsRemoved(t *testing.T) {
	root, path := designGuardEnv(t, "about")
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	// Agent "runs" and mints a contract nobody approved. Reverting means the
	// file goes away — writing an empty file back would leave downstream
	// readers a **Mode**-less document, which is the one state the design
	// authority is not allowed to be in.
	if err := os.WriteFile(path, []byte(designContractFixture), 0644); err != nil {
		t.Fatal(err)
	}

	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionVerify, MilestoneID: "M1"}, pre)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("guard left an agent-invented UX_DESIGN.md on disk (stat err = %v)", err)
	}
	body := steeringBody(t, root, "about")
	if !strings.Contains(body, "created") {
		t.Errorf("steering entry does not report a creation:\n%s", body)
	}
}

func TestDesignAuthorityGuard_MissingStaysMissingIsNoOp(t *testing.T) {
	root, path := designGuardEnv(t, "about")
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	// Agent "runs" and — correctly — never creates one. Most features have no
	// design authority at all; a guard that fired here would steer every phase
	// of every headless feature with a correction about a file that does not
	// exist.
	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionImplementMilestone, MilestoneID: "M1"}, pre)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("guard created a file out of nothing (stat err = %v)", err)
	}
	if body := steeringBody(t, root, "about"); body != "" {
		t.Errorf("guard steered over a file that never existed:\n%s", body)
	}
}

func TestDesignAuthorityGuard_DeletedFileIsRestored(t *testing.T) {
	root, path := designGuardEnv(t, "about")
	if err := os.WriteFile(path, []byte(designContractFixture), 0644); err != nil {
		t.Fatal(err)
	}
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionImplementMilestone, MilestoneID: "M1"}, pre)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("guard did not restore a deleted contract: %v", err)
	}
	if string(after) != designContractFixture {
		t.Errorf("restored contract does not match the snapshot:\n%s", after)
	}
	if body := steeringBody(t, root, "about"); !strings.Contains(body, "deleted") {
		t.Errorf("steering entry does not report a deletion:\n%s", body)
	}
}

// The master authority is cross-cutting: every feature inherits from it, so a
// phase editing it changes decisions for features it was never scoped to.
func TestDesignAuthorityGuard_ProtectsMasterAuthority(t *testing.T) {
	root, _ := designGuardEnv(t, "about")
	masterPath := filepath.Join(root, ".belmont", "UX_DESIGN.md")
	if err := os.WriteFile(masterPath, []byte(designContractFixture), 0644); err != nil {
		t.Fatal(err)
	}
	pre := snapshotDesignAuthority(root, "about")
	if pre == nil {
		t.Fatal("snapshotDesignAuthority returned nil")
	}

	if err := os.WriteFile(masterPath, []byte("# gutted\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runDesignAuthorityGuard(loopConfig{Feature: "about", Root: root},
		loopAction{Type: actionImplementMilestone, MilestoneID: "M1"}, pre)

	after, err := os.ReadFile(masterPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != designContractFixture {
		t.Errorf("master authority not restored:\n%s", after)
	}
	if body := steeringBody(t, root, "about"); !strings.Contains(body, ".belmont/UX_DESIGN.md") {
		t.Errorf("steering entry does not name the master authority:\n%s", body)
	}
}

func TestSnapshotDesignAuthority_NilWithoutRoot(t *testing.T) {
	if snapshotDesignAuthority("", "about") != nil {
		t.Error("no root means no baseline — the guard must skip, not run against paths rooted at \"\"")
	}
}
