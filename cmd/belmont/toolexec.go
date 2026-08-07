package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// resolveModelFlags returns the --model <id> flag pair (and for Pi, the
// preceding --provider <name>) for the given tool+tier, or nil if the tool
// doesn't support model selection or the tier is unknown/empty. For copilot
// with no tier, returns --model auto (copilot's explicit "pick a sensible
// model" token).
//
// projectRoot is consulted only for Pi, Codex, and opencode — see
// resolvePiModelFlags / resolveCodexModelFlags / resolveOpencodeModelFlags
// for the resolution chain (env vars >
// .belmont/local-llms.json > ~/.belmont/local-llms.json > tier default for
// Codex + opencode / nil for Pi). Pass "" if no project context is available;
// these tools still honour env vars and the user-level config file.
func resolveModelFlags(tool, tier, projectRoot string) []string {
	if !toolSupportsModel[tool] {
		return nil
	}
	if tool == "pi" {
		return resolvePiModelFlags(projectRoot, tier)
	}
	if tool == "codex" {
		return resolveCodexModelFlags(projectRoot, tier)
	}
	if tool == "opencode" {
		return resolveOpencodeModelFlags(projectRoot, tier)
	}
	if tier == "" {
		if tool == "copilot" {
			return []string{"--model", "auto"}
		}
		return nil
	}
	tiers, ok := modelTiers[tool]
	if !ok {
		return nil
	}
	model, ok := tiers[tier]
	if !ok {
		return nil
	}
	return []string{"--model", model}
}

// parseModelTiers reads .belmont/features/<slug>/models.yaml with a minimal
// line-based parser. Flat schema only: top-level scalar keys (profile, planning)
// and one nested map (tiers:). Unknown keys are ignored so users can add comments
// or extra fields without breaking parse. Stdlib only (no YAML dependency).
// Returns a zero-value struct if the file does not exist.
func parseModelTiers(path string) (modelTierConfig, error) {
	cfg := modelTierConfig{Tiers: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	inTiers := false
	for _, raw := range strings.Split(string(data), "\n") {
		// Strip # comments (naive — does not support # inside quoted values).
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		isIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		content := strings.TrimSpace(line)
		if !isIndented {
			inTiers = false
			k, v, ok := splitYAMLKV(content)
			if !ok {
				continue
			}
			switch k {
			case "profile":
				cfg.Profile = v
			case "planning":
				cfg.Planning = v
			case "tiers":
				if v == "" {
					inTiers = true
				}
			}
			continue
		}
		if !inTiers {
			continue
		}
		k, v, ok := splitYAMLKV(content)
		if !ok || k == "" || v == "" {
			continue
		}
		cfg.Tiers[k] = v
	}
	return cfg, nil
}

// tierForAction returns the tier label ("low"|"medium"|"high") for the given
// action under the supplied tier config. Planning actions always return the
// global planningTier. Agent-mapped actions look up the agent's configured
// tier. If the agent has no configured tier, returns "" so callers fall back
// to the tool's default model (no --model flag).
func tierForAction(t loopActionType, cfg modelTierConfig) string {
	if t == actionReplan {
		return planningTier
	}
	agent := actionAgent(t)
	if agent == "" {
		return ""
	}
	if tier, ok := cfg.Tiers[agent]; ok && tier != "" {
		return tier
	}
	return ""
}

// reconciliationTier returns the tier for reconciliation work given a config,
// falling back to reconciliationDefaultTier when not specified.
func reconciliationTier(cfg modelTierConfig) string {
	if t, ok := cfg.Tiers["reconciliation"]; ok && t != "" {
		return t
	}
	return reconciliationDefaultTier
}

func allToolNames() []string {
	names := make([]string, 0, len(toolConfigs))
	for _, tool := range toolConfigs {
		names = append(names, tool.Name)
	}
	return names
}

func isKnownTool(name string) bool {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolLabel(name string) string {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return tool.Label
		}
	}
	return name
}

func detectTool() string {
	for _, tool := range []string{"claude", "codex", "gemini", "copilot", "cursor", "windsurf", "pi", "opencode"} {
		if _, err := exec.LookPath(toolBinary(tool)); err == nil {
			return tool
		}
	}
	return ""
}

func toolSummary(name string, input map[string]interface{}) string {
	switch name {
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return name + " " + filepath.Base(fp)
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "…"
			}
			return name + " " + cmd
		}
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + strconv.Quote(pat)
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + pat
		}
	case "Agent":
		if desc, ok := input["description"].(string); ok {
			if len(desc) > 120 {
				desc = desc[:120] + "…"
			}
			return name + " " + desc
		}
	case "Skill":
		if sk, ok := input["skill"].(string); ok {
			return name + " " + sk
		}
	}
	return name
}

// adaptPromptForTool rewrites Belmont's slash-command-style auto-mode
// prompts ("/belmont:<skill> --feature X") into a tool-native form when the
// target tool doesn't activate skills via slash syntax.
//
// Claude Code accepts "/belmont:<skill>" verbatim because Belmont installs
// per-skill slash commands at .claude/commands/belmont/<skill>.md.
//
// Pi uses "/" for its own command palette — "/belmont:X" lands as literal
// text. Pi activates skills via natural-language description match (the
// agentskills.io standard). We rewrite to an explicit "run the X skill;
// read its SKILL.md and follow it" form so a local Qwen-class model has
// concrete instructions even if its description-matching pass is weaker
// than a frontier model's.
//
// opencode uses "/" for its own command palette too, and `opencode run`
// passes the message to the model as plain text — a literal "/belmont:X"
// neither matches an opencode command (Belmont's installed commands are
// slash-namespaced "/belmont/X", and discovered skills are named by
// SKILL.md frontmatter, e.g. "implement") nor reliably maps to the skill
// tool. The same explicit rewrite removes the ambiguity.
//
// Codex / Gemini / Cursor / Copilot are left unchanged — they recognise
// "/belmont:<skill>" as a skill reference today. Revisit per-tool if any
// of them changes behaviour.
func adaptPromptForTool(prompt, tool string) string {
	if tool != "pi" && tool != "opencode" {
		return prompt
	}
	re := regexp.MustCompile(`^/belmont:([\w-]+)(?:\s+--feature\s+(\S+))?`)
	return re.ReplaceAllStringFunc(prompt, func(match string) string {
		groups := re.FindStringSubmatch(match)
		skill := groups[1]
		feature := ""
		if len(groups) > 2 {
			feature = groups[2]
		}
		if feature == "" {
			return fmt.Sprintf(
				"Run the belmont:%s skill. Read .agents/skills/belmont/%s/SKILL.md fully and follow the instructions in its body to completion.",
				skill, skill)
		}
		return fmt.Sprintf(
			"Run the belmont:%s skill against feature %q. Read .agents/skills/belmont/%s/SKILL.md fully and follow the instructions in its body to completion. Project state for this feature lives at .belmont/features/%s/ (PRD.md, TECH_PLAN.md, PROGRESS.md).",
			skill, feature, skill, feature)
	})
}

// toolBinary returns the executable name on PATH for a given tool. For most
// tools the binary matches the tool's logical name, but Cursor's CLI is
// installed as `cursor-agent` (the `cursor` IDE binary is separate).
func toolBinary(tool string) string {
	if tool == "cursor" {
		return "cursor-agent"
	}
	return tool
}

// toolHeadlessArgs returns the argv (excluding binary name) for a one-shot
// headless invocation of the given AI tool. streaming controls Claude's
// output format: stream-json+verbose for live event streaming (auto loop +
// reverify), plain json otherwise (AI decision / triage / reconciliation).
// Other tools' output format is identical regardless of streaming. Returns
// nil for unknown tools — callers should validate `tool` before calling.
func toolHeadlessArgs(tool, prompt, root string, modelFlags []string, streaming bool) []string {
	switch tool {
	case "claude":
		args := []string{"-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--allowedTools", "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill,WebFetch,WebSearch,mcp__*",
			"--output-format"}
		if streaming {
			args = append(args, "stream-json", "--verbose")
		} else {
			args = append(args, "json")
		}
		return append(args, modelFlags...)
	case "codex":
		args := []string{"exec", prompt,
			"--dangerously-bypass-approvals-and-sandbox",
			"--json", "-C", root}
		return append(args, modelFlags...)
	case "gemini":
		args := []string{"-p", prompt, "--approval-mode", "yolo", "--output-format", "json"}
		return append(args, modelFlags...)
	case "copilot":
		// `--yolo` is the documented alias of `--allow-all`. Both work; we use
		// `--yolo` for stability across the alias's lifetime.
		args := []string{"-p", prompt, "--yolo"}
		return append(args, modelFlags...)
	case "cursor":
		// Cursor's prompt is a trailing positional; flags must come first.
		args := []string{"-p", "--force", "--output-format", "json"}
		args = append(args, modelFlags...)
		return append(args, prompt)
	case "pi":
		// Pi's print mode runs the full agent loop with YOLO defaults — no
		// auto-approve flag exists or is needed. modelFlags is `--provider X
		// --model Y` produced by resolvePiModelFlags; may be empty when the
		// user has no local-llms.json + no env override, in which case Pi
		// falls back to its own default model. Pi's `-p` does not emit
		// structured JSON — extractDecisionJSON handles plain-text shapes.
		args := []string{"-p"}
		args = append(args, modelFlags...)
		return append(args, prompt)
	case "opencode":
		// `opencode run` is opencode's one-shot non-interactive mode.
		// --dangerously-skip-permissions auto-approves everything not
		// explicitly denied in opencode.json (config `deny` rules still
		// win, so users keep a guardrail). We deliberately do NOT pass
		// `--format json`: that emits a JSON *event stream* whose text is
		// escaped, which the decision extractor can't see through. The
		// default format prints the response as plain text, which
		// extractDecisionJSON's plain-text shapes handle (same path as
		// Pi). modelFlags is `--model provider/model` produced by
		// resolveOpencodeModelFlags.
		args := []string{"run", "--dangerously-skip-permissions"}
		args = append(args, modelFlags...)
		return append(args, prompt)
	}
	return nil
}

// buildToolCommand creates a non-streaming exec.Cmd for the given tool with a
// prompt. Used by AI decision / triage / reconciliation calls that want a
// single JSON response.
func buildToolCommand(tool, prompt, root string, extraFlags ...string) *exec.Cmd {
	args := toolHeadlessArgs(tool, prompt, root, extraFlags, false)
	if args == nil {
		return exec.Command("echo", "unsupported tool")
	}
	cmd := exec.Command(toolBinary(tool), args...)
	cmd.Dir = root
	return cmd
}

// extractDecisionJSON finds the JSON object in tool output.
//
// Most tools wrap their output in a JSON envelope (`{"result": "..."}`), so
// the first step strips that. Pi's `-p` mode emits plain text with no
// envelope and no `--output-format json` flag — its decision JSON is either
// the entire output, embedded in a fenced code block (```json ... ```), or
// inline in surrounding prose. We try the cheap regex first (matches every
// tool's typical "{ ... action: ... }" object), then fall back to fenced
// extraction, then a brace-depth scan for the first balanced top-level
// object containing the action field. The fallbacks also benefit non-Pi
// tools when they happen to wrap JSON in markdown.
func extractDecisionJSON(output, tool string) (string, error) {
	text := output

	// Strip the JSON envelope used by every tool except Pi.
	if tool == "claude" || tool == "codex" || tool == "gemini" || tool == "cursor" {
		var wrapper struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal([]byte(output), &wrapper); err == nil && wrapper.Result != "" {
			text = wrapper.Result
		}
	}

	// Cheap path: the action object on a single level (no nested braces).
	// This covers the historical Claude / Codex / Gemini / Cursor shape.
	flatRe := regexp.MustCompile(`\{[^{}]*"action"\s*:\s*"[^"]+?"[^{}]*\}`)
	if match := flatRe.FindString(text); match != "" {
		return match, nil
	}

	// Fenced markdown block. Pi often wraps JSON in ```json ... ``` or
	// just ``` ... ```. Pull out each fenced span and try parsing.
	for _, body := range fencedJSONBodies(text) {
		if match := flatRe.FindString(body); match != "" {
			return match, nil
		}
		if match := firstBalancedJSONObjectWithAction(body); match != "" {
			return match, nil
		}
	}

	// Brace-depth scan over the whole text — handles inline JSON with
	// nested objects, which the flat regex misses.
	if match := firstBalancedJSONObjectWithAction(text); match != "" {
		return match, nil
	}

	return "", fmt.Errorf("no decision JSON found in output: %s", truncateTail(text, 200))
}

// Model tier registry: maps (tool, tier) to the CLI --model identifier.
// Tiers (low/medium/high) are stable across releases; model IDs get bumped
// here as tools ship new versions. This is the single source of truth —
// skill bodies reference the same mapping via _partials/tier-registry.md.
var modelTiers = map[string]map[string]string{
	"claude": {
		"low":    "haiku",
		"medium": "sonnet",
		"high":   "opus",
	},
	"codex": {
		"low":    "gpt-5.4-mini",
		"medium": "gpt-5.4",
		"high":   "gpt-5.5",
	},
	"gemini": {
		"low":    "gemini-2.5-flash-lite",
		"medium": "gemini-2.5-flash",
		"high":   "gemini-2.5-pro",
	},
	"cursor": {
		"low":    "sonnet-4",
		"medium": "sonnet-4-thinking",
		"high":   "gpt-5",
	},
	"copilot": {
		"low":    "haiku-4.5",
		"medium": "claude-sonnet-4.5",
		"high":   "gpt-5.4",
	},
	// opencode model IDs are `provider/model` pairs. The defaults below assume
	// the Anthropic provider (opencode's most common frontier setup); users on
	// another provider (opencode zen, OpenAI, local models, …) override per
	// tier via `opencode.tiers.<tier>.model` in local-llms.json or
	// BELMONT_OPENCODE_MODEL_<TIER> env vars — see resolveOpencodeModelFlags.
	"opencode": {
		"low":    "anthropic/claude-haiku-4-5",
		"medium": "anthropic/claude-sonnet-4-6",
		"high":   "anthropic/claude-opus-4-8",
	},
}
