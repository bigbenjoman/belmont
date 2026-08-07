package main

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ensureSkillsGenerated re-runs `scripts/generate-skills.sh` against the
// given Belmont source root if its generated output is missing or stale
// (any `_src/` or `_partials/` file newer than any `<skill>/SKILL.md`).
// This makes source-mode `belmont install --source` self-healing — no
// separate `make` step required, even though the generated layout isn't
// committed to git.
func ensureSkillsGenerated(sourceRoot string) error {
	srcDir := filepath.Join(sourceRoot, "skills", "belmont", "_src")
	partialsDir := filepath.Join(sourceRoot, "skills", "belmont", "_partials")
	skillsDir := filepath.Join(sourceRoot, "skills", "belmont")
	script := filepath.Join(sourceRoot, "scripts", "generate-skills.sh")

	if !dirExists(srcDir) {
		// Older Belmont source layouts didn't have _src/ — skip silently.
		return nil
	}
	if !fileExists(script) {
		return nil
	}

	stale := false
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil
	}
	for _, entry := range srcEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		srcInfo, err := os.Stat(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			continue
		}
		genPath := filepath.Join(skillsDir, name, "SKILL.md")
		genInfo, err := os.Stat(genPath)
		if err != nil {
			stale = true
			break
		}
		if srcInfo.ModTime().After(genInfo.ModTime()) {
			stale = true
			break
		}
	}

	// Also check partials — any partial change invalidates everything that
	// includes it, and including that into per-file dependency tracking is
	// not worth the complexity.
	if !stale && dirExists(partialsDir) {
		partialEntries, _ := os.ReadDir(partialsDir)
		var oldestGen time.Time
		first := true
		_ = filepath.WalkDir(skillsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(p) != "SKILL.md" {
				return nil
			}
			info, err := os.Stat(p)
			if err != nil {
				return nil
			}
			if first || info.ModTime().Before(oldestGen) {
				oldestGen = info.ModTime()
				first = false
			}
			return nil
		})
		if !first {
			for _, entry := range partialEntries {
				if entry.IsDir() {
					continue
				}
				info, err := os.Stat(filepath.Join(partialsDir, entry.Name()))
				if err != nil {
					continue
				}
				if info.ModTime().After(oldestGen) {
					stale = true
					break
				}
			}
		}
	}

	if !stale {
		return nil
	}

	fmt.Println("Regenerating skills from _src/ + _partials/ ...")
	cmd := exec.Command("bash", script)
	cmd.Dir = sourceRoot
	cmd.Stdout = io.Discard // already verbose; suppress unless errored
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install: failed to regenerate skills (run scripts/generate-skills.sh manually): %w", err)
	}
	return nil
}

func resolveTools(projectRoot, toolsFlag string, noPrompt bool) ([]string, error) {
	detected := detectTools(projectRoot)

	if toolsFlag != "" {
		switch strings.ToLower(toolsFlag) {
		case "all":
			if len(detected) > 0 {
				return detected, nil
			}
			return allToolNames(), nil
		case "none":
			return nil, nil
		default:
			parts := strings.Split(toolsFlag, ",")
			var selected []string
			for _, p := range parts {
				name := strings.TrimSpace(strings.ToLower(p))
				if name == "" {
					continue
				}
				if !isKnownTool(name) {
					return nil, fmt.Errorf("install: unknown tool %q", name)
				}
				selected = append(selected, name)
			}
			return selected, nil
		}
	}

	if noPrompt {
		if len(detected) > 0 {
			return detected, nil
		}
		return nil, nil
	}

	reader := bufio.NewReader(os.Stdin)

	if len(detected) > 0 {
		fmt.Println("Detected AI tools:")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s\n", i+1, toolLabel(tool))
		}
		fmt.Println("")
		fmt.Println("Install skills for:")
		fmt.Println("  [a] All detected tools")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s only\n", i+1, toolLabel(tool))
		}
		fmt.Println("  [s] Skip (install agents only)")
		fmt.Println("")
		fmt.Print("Choice [a]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		fmt.Println("")

		if choice == "" || strings.EqualFold(choice, "a") {
			return detected, nil
		}
		if strings.EqualFold(choice, "s") {
			return nil, nil
		}
		if idx, err := strconv.Atoi(choice); err == nil {
			if idx >= 1 && idx <= len(detected) {
				return []string{detected[idx-1]}, nil
			}
		}
		return detected, nil
	}

	fmt.Println("No AI tool directories detected.")
	fmt.Println("")
	fmt.Println("Which tool are you using?")
	for i, tool := range toolConfigs {
		fmt.Printf("  [%d] %s\n", i+1, tool.Label)
	}
	fmt.Println("  [s] Skip (install agents only - reference files manually)")
	fmt.Println("")
	fmt.Print("Choice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	fmt.Println("")

	if strings.EqualFold(choice, "s") {
		return nil, nil
	}
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(toolConfigs) {
			return []string{toolConfigs[idx-1].Name}, nil
		}
	}

	fmt.Println("Invalid choice. Installing agents only.")
	return nil, nil
}

// detectTools returns the AI tools Belmont should install for. A tool is
// included when ANY of these signals fires:
//   - its conventional project-level dir exists (`.claude/`, `.codex/`, etc.),
//     signaling the user has used the tool in this repo before;
//   - its CLI binary is present on PATH, signaling the user has the tool
//     installed system-wide (so they'll likely want Belmont wired up here);
//   - a Belmont skill-routing section exists in AGENTS.md (for Codex and
//     Copilot) or GEMINI.md (for Gemini), signaling Belmont was previously
//     installed for that tool here — needed because Phase 1+ installs no
//     longer create `.codex/`, `.gemini/`, or `.copilot/` marker dirs;
//   - a root `opencode.json` / `opencode.jsonc` exists (for opencode, whose
//     `.opencode/` directory is optional when config lives at the root).
func detectTools(projectRoot string) []string {
	tools := []string{"claude", "codex", "cursor", "windsurf", "gemini", "copilot", "pi", "opencode"}
	dirMarker := map[string]string{
		"claude":   ".claude",
		"codex":    ".codex",
		"cursor":   ".cursor",
		"windsurf": ".windsurf",
		"gemini":   ".gemini",
		"copilot":  ".copilot",
		"pi":       ".pi",
		"opencode": ".opencode",
	}
	hasAgentsBelmontSection := fileContainsMarker(filepath.Join(projectRoot, "AGENTS.md"), belmontAgentsSectionStart)
	hasGeminiBelmontSection := fileContainsMarker(filepath.Join(projectRoot, "GEMINI.md"), belmontGeminiSectionStart)

	var detected []string
	for _, tool := range tools {
		if dirExists(filepath.Join(projectRoot, dirMarker[tool])) {
			detected = append(detected, tool)
			continue
		}
		if _, err := exec.LookPath(toolBinary(tool)); err == nil {
			detected = append(detected, tool)
			continue
		}
		// File-based fallback for AGENTS.md / GEMINI.md-routed tools.
		switch tool {
		case "codex", "copilot":
			if hasAgentsBelmontSection {
				detected = append(detected, tool)
			}
		case "gemini":
			if hasGeminiBelmontSection {
				detected = append(detected, tool)
			}
		case "opencode":
			// opencode projects often carry only a root opencode.json(c) —
			// the .opencode/ marker directory is optional.
			if fileExists(filepath.Join(projectRoot, "opencode.json")) || fileExists(filepath.Join(projectRoot, "opencode.jsonc")) {
				detected = append(detected, tool)
			}
		}
	}
	return detected
}

func syncMarkdownDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync the references/ subdir if present (progressive-disclosure detail
	// loaded on demand by skills via relative paths).
	refsSrc := filepath.Join(sourceDir, "references")
	refsDest := filepath.Join(targetDir, "references")
	if dirExists(refsSrc) {
		if err := syncReferencesDir(refsSrc, refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// syncReferencesDir mirrors .md files in a references/ subdirectory.
func syncReferencesDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeCodexSkillInterfaces writes Codex UI metadata at
// `.agents/skills/belmont/<skill>/agents/openai.yaml` for every installed
// skill. Codex's skill loader reads `agents/openai.yaml` next to each
// SKILL.md (core-skills loader.rs: SKILLS_METADATA_DIR="agents",
// SKILLS_METADATA_FILENAME="openai.yaml") and its `$`-mention popup prefers
// `interface.display_name` over the frontmatter `name:`, fuzzy-matching the
// typed filter against it. The shared `belmont:` display-name prefix is what
// groups the skills: typing `$belmont` surfaces the full set with
// descriptions — Codex's `/` menu itself only lists built-ins and cannot be
// extended (the `~/.codex/prompts` custom-prompt feature was removed
// upstream).
//
// Per agentskills.io, the `agents/` subdir is product-specific config that
// other CLIs ignore, so this stays inside the canonical skill folders
// without affecting the seven other supported tools. Idempotent: up-to-date
// files are left untouched. No separate prune pass is needed — a removed
// skill's openai.yaml disappears with its folder when the skill sync prunes
// the whole directory.
func writeCodexSkillInterfaces(skillsTarget string) error {
	entries, err := os.ReadDir(skillsTarget)
	if err != nil {
		return fmt.Errorf("read skills dir %s: %w", skillsTarget, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip generation scaffolding dirs (shouldn't be at this level after
		// install but defensive).
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsTarget, name, "SKILL.md")); err != nil {
			continue
		}
		content := "# Generated by Belmont -- Codex UI metadata; other CLIs ignore this file.\n" +
			"# The shared display-name prefix groups Belmont's skills in Codex's $-mention\n" +
			"# popup: type \"$belmont\" to list them all.\n" +
			"interface:\n" +
			"  display_name: \"belmont:" + name + "\"\n"
		yamlPath := filepath.Join(skillsTarget, name, "agents", "openai.yaml")
		if existing, err := os.ReadFile(yamlPath); err == nil && string(existing) == content {
			fmt.Printf("  = %s/agents/openai.yaml (ok)\n", name)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(yamlPath, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Printf("  + %s/agents/openai.yaml (Codex $-mention display name)\n", name)
	}
	return nil
}

// syncEmbeddedDir mirrors syncMarkdownDir but reads from an embed.FS.
func syncEmbeddedDir(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	hasReferences := false
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "references" {
				hasReferences = true
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	// Clean stale files
	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync references/ subdir from the embed FS.
	refsDest := filepath.Join(targetDir, "references")
	if hasReferences {
		if err := syncEmbeddedReferences(embedFS, root+"/references", refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// syncEmbeddedReferences mirrors the references/ subdir from an embed.FS.
func syncEmbeddedReferences(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}
