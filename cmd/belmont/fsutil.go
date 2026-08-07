package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
