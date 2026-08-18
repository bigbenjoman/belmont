package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// shouldColor resolves whether ANSI colors should be emitted.
//
//	mode "never"  → false
//	mode "always" → true (regardless of stdout type or NO_COLOR)
//	mode "auto"   → honor NO_COLOR, then fall back to isTerminal(f)
//
// Follows the NO_COLOR convention (https://no-color.org): any non-empty value
// disables colors under "auto" mode only. "always" is the explicit opt-in for
// piped contexts (e.g., when Claude Code runs the CLI as a sub-process but
// renders ANSI in its Bash tool output block).
func shouldColor(mode string, f *os.File) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		if os.Getenv("NO_COLOR") != "" {
			return false, nil
		}
		return isTerminal(f), nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("invalid --color value %q (want auto, always, or never)", mode)
	}
}

func milestoneStatusIcon(m milestone, color bool) string {
	// Checked before everything else: an all-withdrawn milestone satisfies
	// milestoneAllDone (nothing outstanding) and would otherwise render as
	// done — a claim that work happened. It did not.
	if milestoneAllWithdrawn(m) {
		if color {
			return ansiDim + "[-]" + ansiReset
		}
		return "[-]"
	}
	if milestoneAllVerified(m) {
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	} else if milestoneAllDone(m) {
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	} else if milestoneHasBlockers(m) {
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	} else if !milestoneNotStarted(m) {
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	}
	if color {
		return ansiDim + "[ ]" + ansiReset
	}
	return "[ ]"
}

func colorStatus(status string, color bool) string {
	if !color {
		return status
	}
	switch status {
	case "Verified":
		return ansiGreen + status + ansiReset
	case "Complete":
		return ansiCyan + status + ansiReset
	case "In Progress":
		return ansiYellow + status + ansiReset
	case "BLOCKED":
		return ansiRed + status + ansiReset
	case "Not Started", "Archived":
		return ansiDim + status + ansiReset
	default:
		return status
	}
}

func statusLegend(color bool) string {
	if !color {
		return "\nLegend: [v] verified  [x] done  [>] in progress  [!] blocked  [ ] todo  [-] withdrawn\n"
	}
	return fmt.Sprintf("\nLegend: %s[v]%s verified  %s[x]%s done  %s[>]%s in progress  %s[!]%s blocked  %s[ ]%s todo  %s[-]%s withdrawn\n",
		ansiGreen, ansiReset, ansiCyan, ansiReset, ansiYellow, ansiReset, ansiRed, ansiReset,
		ansiDim, ansiReset, ansiDim, ansiReset)
}

func featureStatusIcon(status string, color bool) string {
	bracket := "[ ]"
	switch status {
	case "Verified":
		bracket = "[v]"
	case "Complete":
		bracket = "[x]"
	case "In Progress":
		bracket = "[>]"
	case "Archived":
		bracket = "[-]"
	}
	if !color {
		return bracket
	}
	switch status {
	case "Verified":
		return ansiGreen + bracket + ansiReset
	case "Complete":
		return ansiCyan + bracket + ansiReset
	case "In Progress":
		return ansiYellow + bracket + ansiReset
	default:
		return ansiDim + bracket + ansiReset
	}
}

// ANSI color codes for terminal output
const (
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
)

func taskStatusIcon(status taskStatus, color bool) string {
	switch status {
	case taskVerified:
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	case taskDone:
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	case taskInProgress:
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	case taskBlocked:
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	case taskWithdrawn:
		if color {
			return ansiDim + "[-]" + ansiReset
		}
		return "[-]"
	case taskUnknown:
		// Render as `[?]` rather than `[ ]`. Showing an unparsed entry as todo
		// is how issue #27 stayed invisible: the output looked like ordinary
		// outstanding work.
		if color {
			return ansiRed + "[?]" + ansiReset
		}
		return "[?]"
	default:
		if color {
			return ansiDim + "[ ]" + ansiReset
		}
		return "[ ]"
	}
}

// tailWriter writes all data to an underlying writer and keeps a rolling
// buffer of the last `size` bytes for later retrieval.
type tailWriter struct {
	out     io.Writer
	buf     []byte
	size    int
	prefix  string
	lineBuf []byte // partial line accumulator (used when prefix is set)
}

func newTailWriter(out io.Writer, size int, prefix string) *tailWriter {
	return &tailWriter{out: out, buf: make([]byte, 0, size), size: size, prefix: prefix}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	// Always store raw bytes in buf for error tail reporting
	tw.buf = append(tw.buf, p...)
	if len(tw.buf) > tw.size {
		tw.buf = tw.buf[len(tw.buf)-tw.size:]
	}

	// When no prefix, pass through directly
	if tw.prefix == "" {
		_, err := tw.out.Write(p)
		return len(p), err
	}

	// Line-buffer and prepend prefix to each complete line
	tw.lineBuf = append(tw.lineBuf, p...)
	for {
		idx := bytes.IndexByte(tw.lineBuf, '\n')
		if idx < 0 {
			break
		}
		line := tw.lineBuf[:idx]
		tw.lineBuf = tw.lineBuf[idx+1:]
		tw.out.Write([]byte(tw.prefix + string(line) + "\n"))
	}
	return len(p), nil
}

func (tw *tailWriter) String() string {
	return string(tw.buf)
}

// usageCapture tees a tool's stdout to an inner writer while retaining the one
// line that carries its token count.
//
// It exists because of a specific defect. Non-Claude tools write straight into a
// tailWriter, which keeps only the last 1500 bytes — so executionResult.Output
// is the *tail* of the stream, not the stream. Whether a tool's usage event
// happens to fall inside that tail is luck, not a contract: codex emits
// turn.completed last but the item.completed events before it grow with the
// agent's output, so the usage line drifts out of a fixed-size window on exactly
// the long runs whose cost matters most.
//
// Raising the tail size would not fix it — a bigger window is still a window,
// and it would also change the error-reporting tail, which is unrelated to this
// task. Retaining one line by *content* is position-independent and costs a
// single line of memory.
//
// Claude does not use this path: claudeStreamWriter already parses every line,
// so it captures usage directly.
type usageCapture struct {
	inner   io.Writer
	partial []byte
	match   func(line []byte) *toolUsage
	usage   *toolUsage
}

// maxUsageLineBytes caps the partial-line accumulator. A tool that emits a
// single enormous JSON document with no newline would otherwise grow this
// without bound; past the cap we stop accumulating rather than buffer a run's
// entire output to look for a token count.
const maxUsageLineBytes = 1 << 20

func newUsageCapture(inner io.Writer, match func([]byte) *toolUsage) *usageCapture {
	return &usageCapture{inner: inner, match: match}
}

func (u *usageCapture) Write(p []byte) (int, error) {
	if u.match != nil {
		u.partial = append(u.partial, p...)
		for {
			idx := bytes.IndexByte(u.partial, '\n')
			if idx < 0 {
				break
			}
			line := u.partial[:idx]
			u.partial = u.partial[idx+1:]
			// Cheap pre-filter: skip the JSON parse on the overwhelming
			// majority of lines, which carry no usage block at all.
			if bytes.Contains(line, []byte(`"usage"`)) {
				if usage := u.match(line); usage != nil {
					u.usage = usage
				}
			}
		}
		if len(u.partial) > maxUsageLineBytes {
			u.partial = nil
		}
	}
	return u.inner.Write(p)
}

// Usage returns the last token count seen, or nil if the tool never reported one.
func (u *usageCapture) Usage() *toolUsage {
	if u == nil {
		return nil
	}
	return u.usage
}

// claudeStreamWriter wraps a tailWriter and parses Claude stream-json NDJSON,
// extracting only human-readable content (assistant text + tool use indicators).
type claudeStreamWriter struct {
	tw      *tailWriter
	partial []byte
	prefix  string // e.g. "\033[36m[slug]\033[0m: "
	usage   *toolUsage
}

// Usage returns the token count Claude reported on its `result` event, or nil
// if the stream ended without one (an aborted or failed run). Never estimated.
func (c *claudeStreamWriter) Usage() *toolUsage { return c.usage }

type streamLine struct {
	Type    string        `json:"type"`
	Subtype string        `json:"subtype"`
	Message streamMessage `json:"message"`
	// Usage arrives on the terminal `result` event, not on any assistant event.
	// Before P0-2 every non-assistant line was discarded here, which is exactly
	// where the only token count Claude emits was being thrown away.
	Usage *claudeUsage `json:"usage"`
}

// claudeUsage is the `usage` block of Claude's stream-json `result` event.
// Verified against claude 2.1.234 on 2026-08-18; the event also carries
// modelUsage, total_cost_usd and duration_ms, none of which Belmont records —
// cost is a derived figure that changes under us, tokens are not.
type claudeUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

func (c *claudeStreamWriter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		idx := bytes.IndexByte(c.partial, '\n')
		if idx < 0 {
			break
		}
		line := c.partial[:idx]
		c.partial = c.partial[idx+1:]
		if len(line) == 0 {
			continue
		}
		var sl streamLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue
		}
		if sl.Type == "result" && sl.Usage != nil {
			c.usage = &toolUsage{
				Input:         sl.Usage.InputTokens,
				Output:        sl.Usage.OutputTokens,
				CacheCreation: sl.Usage.CacheCreationInputTokens,
				CacheRead:     sl.Usage.CacheReadInputTokens,
			}
		}
		if sl.Type != "assistant" {
			continue
		}
		for _, item := range sl.Message.Content {
			switch item.Type {
			case "text":
				if item.Text != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + item.Text + "\n"))
					} else {
						c.tw.Write([]byte("  " + item.Text + "\n"))
					}
				}
			case "tool_use":
				if item.Name != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + toolSummary(item.Name, item.Input) + "\n"))
					} else {
						c.tw.Write([]byte("  → " + toolSummary(item.Name, item.Input) + "\n"))
					}
				}
			}
		}
	}
	return len(p), nil
}

// truncateTail returns the last maxLen characters of s.
func truncateTail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// warnPrefix / warnSuffix wrap a warning line in yellow when colour is on.
func warnPrefix(color bool) string {
	if color {
		return ansiYellow
	}
	return ""
}

func warnSuffix(color bool) string {
	if color {
		return ansiReset
	}
	return ""
}
