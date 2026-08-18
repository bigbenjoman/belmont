package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests pin writeStateFile, the atomic state-write helper added by P0-1.
//
// The defect they exist to prevent: every Belmont state writer used a bare
// os.WriteFile, which truncates and then writes. A reader in another worktree
// running concurrently — `belmont status` while `belmont auto` rewrites a
// register, which is the normal case under parallel waves — could observe the
// file mid-write and parse zero milestones from a PROGRESS.md that was fine a
// millisecond earlier and fine a millisecond later.
//
// TestWriteStateFileIsNeverObservedPartiallyWritten is the proof the task
// requires, and it is meaningful only under `go test -race`.

// TestWriteStateFileIsNeverObservedPartiallyWritten runs a writer and a reader
// concurrently against one path and asserts the reader only ever sees a
// complete document. The two payloads differ in length as well as content, so a
// torn read shows up either as a truncated prefix or as a mixture of both — the
// old truncate-then-write fails this within a handful of iterations.
func TestWriteStateFileIsNeverObservedPartiallyWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PROGRESS.md")

	// Large enough that a single write() cannot be assumed atomic by the
	// filesystem, which is what makes the rename (rather than the write size)
	// the thing under test.
	small := "# Progress\n" + strings.Repeat("- [ ] P0-1: small\n", 2000)
	large := "# Progress\n" + strings.Repeat("- [v] P0-1: large payload line\n", 5000)

	if err := writeStateFile(path, []byte(small), 0644); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	const iterations = 300
	var wg sync.WaitGroup
	stop := make(chan struct{})
	errs := make(chan error, 64)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := 0; i < iterations; i++ {
			payload := small
			if i%2 == 0 {
				payload = large
			}
			if err := writeStateFile(path, []byte(payload), 0644); err != nil {
				errs <- fmt.Errorf("write %d: %w", i, err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		reads := 0
		for {
			select {
			case <-stop:
				if reads == 0 {
					errs <- fmt.Errorf("reader never observed the file; the test proved nothing")
				}
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				// ENOENT would itself be a visible torn state: the whole point
				// is that the path always resolves to a complete document.
				errs <- fmt.Errorf("read: %w", err)
				return
			}
			reads++
			got := string(data)
			if got != small && got != large {
				errs <- fmt.Errorf("torn read: %d bytes, neither payload (prefix %q)",
					len(got), got[:min(60, len(got))])
				return
			}
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestWriteStateFilePreservesCallerPermissions pins the chmod. os.CreateTemp
// makes the temp file 0600; without an explicit chmod before the rename the
// first atomic write would silently tighten every state file in the tree from
// 0644 to 0600, locking other users out of a shared checkout.
func TestWriteStateFilePreservesCallerPermissions(t *testing.T) {
	dir := t.TempDir()

	for _, perm := range []os.FileMode{0644, 0600, 0664} {
		path := filepath.Join(dir, fmt.Sprintf("state-%o.md", perm))
		if err := writeStateFile(path, []byte("x\n"), perm); err != nil {
			t.Fatalf("write %o: %v", perm, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %o: %v", perm, err)
		}
		if got := info.Mode().Perm(); got != perm {
			t.Errorf("perm %o: got %o, want %o (os.CreateTemp's 0600 leaked through)", perm, got, perm)
		}
	}
}

// TestWriteStateFileOverwriteKeepsExistingPermissions confirms a rewrite of an
// existing file lands on the permissions the caller asked for rather than
// inheriting the temp file's.
func TestWriteStateFileOverwriteKeepsExistingPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PROGRESS.md")

	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writeStateFile(path, []byte("replaced\n"), 0644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "replaced\n" {
		t.Errorf("content: got %q, want %q", data, "replaced\n")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Errorf("perm: got %o, want 0644", got)
	}
}

// TestWriteStateFileLeavesNoTempFiles pins the cleanup. The temp file is a
// sibling of the destination, so it lands inside a feature directory — and the
// directory walkers treat anything in there as state. An orphaned
// .belmont-tmp-* is not merely litter.
func TestWriteStateFileLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PROGRESS.md")

	for i := 0; i < 20; i++ {
		if err := writeStateFile(path, []byte(fmt.Sprintf("round %d\n", i)), 0644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".belmont-tmp-") {
			t.Errorf("orphaned temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only PROGRESS.md, got %v", names)
	}
}

// TestWriteStateFileTempIsSiblingNotTempDir pins the sibling-temp requirement.
// os.Rename is atomic only within one filesystem; on macOS /tmp is routinely a
// different volume from a project's .belmont/, and a cross-device rename
// degrades to copy-then-delete, reopening the very window this helper closes.
// A destination directory that cannot be created in proves the temp file is
// being staged there rather than in os.TempDir().
func TestWriteStateFileTempIsSiblingNotTempDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	err := writeStateFile(filepath.Join(locked, "PROGRESS.md"), []byte("x\n"), 0644)
	if err == nil {
		t.Fatal("expected failure creating the temp file in a read-only destination directory; " +
			"a nil error means the temp file was staged somewhere other than a sibling path")
	}
}

// TestWriteStateFileReplacesSymlinkRatherThanWritingThrough documents a real
// behaviour change from os.WriteFile, which follows a symlink at the
// destination. Rename replaces it. Both callers that care resolve symlink-ness
// above the call (writeReconciliationResolution, copyFile); this test exists so
// a future caller discovers the rule here rather than in production.
func TestWriteStateFileReplacesSymlinkRatherThanWritingThrough(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "link.md")

	if err := os.WriteFile(target, []byte("target content\n"), 0644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := writeStateFile(link, []byte("new content\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected the symlink to be replaced by a regular file")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "target content\n" {
		t.Errorf("symlink target was written through: got %q", data)
	}
}
