package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestTailWriterConcurrentWriters pins P0-M1-FIX-18.
//
// attachUsageCapture leaves cmd.Stderr as the bare tailWriter while setting
// cmd.Stdout to a wrapper. os/exec only collapses the two onto a single pipe and
// a single copying goroutine when interfaceEqual(c.Stderr, c.Stdout) holds, so
// the wrapper guarantees two goroutines reach tailWriter.Write concurrently on
// every claude and codex run. Nothing in the suite drives a real exec.Cmd
// through that path, which is exactly why -race stayed green over a live race.
//
// This drives the shape directly. Without the mutex it fails under -race on
// tw.buf and tw.lineBuf; it also asserts no byte is lost, because the failure
// that matters in production is a corrupt executionResult.Output feeding
// extractDecisionJSON, not a crash.
func TestTailWriterConcurrentWriters(t *testing.T) {
	const (
		writers      = 8
		linesEach    = 200
		bufSize      = 1 << 20 // large enough that nothing is evicted
		prefixMarker = "[x] "
	)

	var out bytes.Buffer
	// prefix non-empty exercises the lineBuf path, which is the second unguarded
	// field and the one that corrupts rendered output rather than just the tail.
	tw := newTailWriter(&out, bufSize, prefixMarker)

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < linesEach; i++ {
				fmt.Fprintf(tw, "writer-%d-line-%d\n", w, i)
			}
		}(w)
	}

	// Concurrent readers: String() reads tw.buf and raced with Write too.
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < linesEach; i++ {
				_ = tw.String()
			}
		}()
	}
	wg.Wait()

	// Every line must arrive intact and prefixed exactly once. Interleaved
	// appends to lineBuf show up here as spliced or double-prefixed lines.
	got := out.String()
	if n := strings.Count(got, prefixMarker); n != writers*linesEach {
		t.Errorf("prefixed lines: got %d, want %d", n, writers*linesEach)
	}
	for w := 0; w < writers; w++ {
		for i := 0; i < linesEach; i++ {
			want := fmt.Sprintf("%swriter-%d-line-%d\n", prefixMarker, w, i)
			if !strings.Contains(got, want) {
				t.Fatalf("missing or corrupted line: %q", strings.TrimSuffix(want, "\n"))
			}
		}
	}

	// The rolling buffer must hold every raw byte too (bufSize is not reached).
	if n := strings.Count(tw.String(), "\n"); n != writers*linesEach {
		t.Errorf("tail buffer lines: got %d, want %d", n, writers*linesEach)
	}
}
