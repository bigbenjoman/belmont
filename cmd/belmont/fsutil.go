package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeStateFile writes data to path so that a concurrent reader observes either
// the complete previous content or the complete new content, never a partial
// file. It replaces the bare os.WriteFile truncate-then-write used by every
// Belmont state writer, which leaves a window in which a reader in another
// worktree sees a half-written PROGRESS.md and parses zero milestones.
//
// The mechanism is write-to-sibling-temp-then-rename. Four things about it are
// load-bearing:
//
//   - The temp file MUST be a sibling of the destination, not in os.TempDir().
//     os.Rename is atomic only within a single filesystem; on macOS /tmp is
//     routinely a different volume from a project's .belmont/, and a
//     cross-device rename degrades to copy-then-delete — which reintroduces
//     precisely the torn window this helper exists to close.
//   - perm is applied explicitly, because os.CreateTemp creates 0600. Without
//     the chmod the first atomic write would silently tighten every state file
//     in the tree from 0644 to 0600.
//   - The temp file is fsynced before the rename, so the rename cannot publish a
//     file whose contents the kernel has not yet written back.
//   - The parent directory is deliberately NOT fsynced. This helper promises
//     visibility to a concurrent reader, not durability across a machine crash,
//     and a directory fsync would cost a synchronous metadata flush on every
//     state write to buy a guarantee nothing here relies on.
//
// Two limits callers must know about. First, rename REPLACES a symlink at path
// rather than writing through it, where os.WriteFile would follow it — callers
// that must preserve symlink-ness resolve that above the call (see
// writeReconciliationResolution in reconcile.go and copyFile in main.go).
// Second, on Windows os.Rename maps to MoveFileEx with MOVEFILE_REPLACE_EXISTING,
// which fails while another process holds the destination open; that is a
// failed write, not a torn one, so the guarantee still holds.
func writeStateFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".belmont-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Every failure path below must clear the temp file. A stale .belmont-tmp-*
	// left inside a feature directory is walked by the directory readers as if
	// it were state, so an orphan is not merely litter.
	defer os.Remove(tmp)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	// Metrics are local-only by decision (P0-2): bookkeeping already accounts
	// for 41-46% of commit volume in these repos, and a metrics file per run
	// would worsen that for no analytical gain.
	ensureGitignoreEntry(projectRoot, ".belmont/metrics/")

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
		if err := writeStateFile(prfaqPath, []byte("Run /belmont:working-backwards to create your PR/FAQ document.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PR_FAQ.md")
	} else {
		fmt.Println("  Exists: .belmont/PR_FAQ.md (keeping)")
	}

	prdPath := filepath.Join(stateDir, "PRD.md")
	if !fileExists(prdPath) {
		if err := writeStateFile(prdPath, []byte("Run the /belmont:product-plan skill to create a plan for your feature.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PRD.md")
	} else {
		fmt.Println("  Exists: .belmont/PRD.md (keeping)")
	}

	return nil
}
