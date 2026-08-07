package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// seedWorkspaceEnv copies root .env* into a workspace directory if the
// workspace either declares explicit env_files or its manifest signals env
// consumption. Quiet on the no-op path. Warns (does not error) when the
// destination would not be gitignored.
func seedWorkspaceEnv(projectRoot, wtPath string, ws workspaceInfo, override workspaceOverride, rootEnvFiles []string) {
	wantSeed := len(override.EnvFiles) > 0 || ws.Signals.consumesEnv()
	if !wantSeed {
		return
	}
	wsDir := filepath.Join(wtPath, ws.Path)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		return
	}
	// Copy root .env* into workspace dir (preserving filenames).
	for _, name := range rootEnvFiles {
		src := filepath.Join(projectRoot, name)
		dst := filepath.Join(wsDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			continue
		}
		warnIfNotGitignored(wtPath, filepath.Join(ws.Path, name))
	}
	// Copy explicit env_files (paths relative to project root).
	for _, rel := range override.EnvFiles {
		clean := filepath.Clean(rel)
		src := filepath.Join(projectRoot, clean)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		// Drop the file by its base name into the workspace dir. This matches
		// what users typically want (e.g. "packages/web/.env.local" lands as
		// .env.local inside the workspace), but if the file is outside the
		// workspace we still seed it so Prisma-style consumers find it.
		dst := filepath.Join(wsDir, filepath.Base(clean))
		if err := os.WriteFile(dst, data, 0644); err != nil {
			continue
		}
		warnIfNotGitignored(wtPath, filepath.Join(ws.Path, filepath.Base(clean)))
	}
}

// detectWorkspaces probes root for monorepo signal files and returns the
// discovered workspaces along with the dominant type. Returns (nil, "") for
// non-monorepos (single-package early-return preserved). Tolerant to parse
// failures: any malformed signal file is treated as "no signal here".
func detectWorkspaces(root string) ([]workspaceInfo, monorepoType) {
	if root == "" {
		return nil, monorepoNone
	}

	// Probe each signal in order of dominance. JS-flavored systems share
	// workspace lists (turbo + pnpm + npm-workspaces all coexist), so
	// detection collects from any present source.
	var ws []workspaceInfo
	var mType monorepoType
	jsCollected := false

	collectJS := func() {
		if jsCollected {
			return
		}
		jsCollected = true
		if pnpmWs, ok := parsePnpmWorkspaces(root); ok {
			ws = append(ws, pnpmWs...)
			return
		}
		if pkgWs, ok := parsePackageJSONWorkspaces(root); ok {
			ws = append(ws, pkgWs...)
			return
		}
	}

	if fileExists(filepath.Join(root, "turbo.json")) {
		mType = monorepoTurborepo
		collectJS()
	}
	if fileExists(filepath.Join(root, "nx.json")) {
		if mType == monorepoNone {
			mType = monorepoNx
		}
		collectJS()
	}
	if fileExists(filepath.Join(root, "pnpm-workspace.yaml")) {
		if mType == monorepoNone {
			mType = monorepoPnpm
		}
		collectJS()
	}
	if mType == monorepoNone {
		// No turbo/nx/pnpm signal — check raw package.json workspaces.
		if pkgWs, ok := parsePackageJSONWorkspaces(root); ok && len(pkgWs) > 0 {
			ws = append(ws, pkgWs...)
			// Distinguish between npm/yarn/bun by lockfile presence.
			switch {
			case fileExists(filepath.Join(root, "yarn.lock")):
				mType = monorepoYarn
			case fileExists(filepath.Join(root, "bun.lockb")), fileExists(filepath.Join(root, "bun.lock")):
				mType = monorepoBun
			default:
				mType = monorepoNpm
			}
			jsCollected = true
		}
	}
	if fileExists(filepath.Join(root, "lerna.json")) {
		if mType == monorepoNone {
			mType = monorepoLerna
		}
		if !jsCollected {
			if lWs, ok := parseLernaWorkspaces(root); ok {
				ws = append(ws, lWs...)
				jsCollected = true
			}
		}
	}
	if fileExists(filepath.Join(root, "rush.json")) {
		if mType == monorepoNone {
			mType = monorepoRush
		}
		if rWs, ok := parseRushWorkspaces(root); ok {
			ws = append(ws, rWs...)
		}
	}
	// Cargo workspace — additive (a repo can contain both JS and Rust workspaces).
	if cWs, ok := parseCargoWorkspaces(root); ok && len(cWs) > 0 {
		if mType == monorepoNone {
			mType = monorepoCargo
		}
		ws = append(ws, cWs...)
	}
	// Go workspace.
	if fileExists(filepath.Join(root, "go.work")) {
		if gWs, ok := parseGoWorkspaces(root); ok && len(gWs) > 0 {
			if mType == monorepoNone {
				mType = monorepoGo
			}
			ws = append(ws, gWs...)
		}
	}
	// Python workspaces (uv / poetry).
	if pWs, pType, ok := parsePyprojectWorkspaces(root); ok && len(pWs) > 0 {
		if mType == monorepoNone {
			mType = pType
		}
		ws = append(ws, pWs...)
	}

	if len(ws) == 0 {
		return nil, monorepoNone
	}
	ws = dedupeWorkspaces(ws)
	return ws, mType
}

// dedupeWorkspaces removes duplicate entries (same path) while keeping the
// first occurrence (which is also the highest-precedence detection source).
func dedupeWorkspaces(ws []workspaceInfo) []workspaceInfo {
	seen := map[string]bool{}
	out := make([]workspaceInfo, 0, len(ws))
	for _, w := range ws {
		if seen[w.Path] {
			continue
		}
		seen[w.Path] = true
		out = append(out, w)
	}
	return out
}

// parsePnpmWorkspaces parses pnpm-workspace.yaml's `packages:` glob list.
// Stdlib-only; tolerant to unknown fields and malformed YAML.
func parsePnpmWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml"))
	if err != nil {
		return nil, false
	}
	var globs []string
	inPackages := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if !inPackages {
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				v := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				v = strings.Trim(v, `"'`)
				if v != "" {
					globs = append(globs, v)
				}
			}
			continue
		}
		inPackages = false
		if strings.HasPrefix(trimmed, "packages:") {
			inPackages = true
		}
	}
	return expandJSWorkspaceGlobs(root, globs), true
}

// parsePackageJSONWorkspaces parses the `workspaces` field from root
// package.json. Supports both array form ["packages/*"] and the object form
// {"packages":["..."],"nohoist":[...]}.
func parsePackageJSONWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, false
	}
	var raw struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false
	}
	if len(raw.Workspaces) == 0 {
		return nil, false
	}
	var globs []string
	if err := json.Unmarshal(raw.Workspaces, &globs); err != nil {
		var obj struct {
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(raw.Workspaces, &obj); err != nil {
			return nil, false
		}
		globs = obj.Packages
	}
	if len(globs) == 0 {
		return nil, false
	}
	return expandJSWorkspaceGlobs(root, globs), true
}

// parseLernaWorkspaces reads lerna.json's `packages` field.
func parseLernaWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "lerna.json"))
	if err != nil {
		return nil, false
	}
	var cfg struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	if len(cfg.Packages) == 0 {
		return nil, false
	}
	return expandJSWorkspaceGlobs(root, cfg.Packages), true
}

// parseRushWorkspaces reads rush.json's projects[].projectFolder list.
func parseRushWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "rush.json"))
	if err != nil {
		return nil, false
	}
	var cfg struct {
		Projects []struct {
			PackageName   string `json:"packageName"`
			ProjectFolder string `json:"projectFolder"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	out := make([]workspaceInfo, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		if p.ProjectFolder == "" {
			continue
		}
		ws := workspaceInfo{
			ID:   p.PackageName,
			Path: filepath.Clean(p.ProjectFolder),
		}
		if ws.ID == "" {
			ws.ID = filepath.Base(ws.Path)
		}
		ws.Manifest = filepath.Join(root, ws.Path, "package.json")
		ws.Signals, ws.HasDev = jsManifestSignals(ws.Manifest)
		out = append(out, ws)
	}
	return out, len(out) > 0
}

// expandJSWorkspaceGlobs walks the project root and matches each glob entry
// against subdirectories that contain a package.json. Supports literal paths
// and trailing /* / /** wildcards (the only forms widely used by JS workspace
// configs). Negation patterns (!path) are honored.
func expandJSWorkspaceGlobs(root string, globs []string) []workspaceInfo {
	if len(globs) == 0 {
		return nil
	}
	var includes, excludes []string
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if strings.HasPrefix(g, "!") {
			excludes = append(excludes, strings.TrimPrefix(g, "!"))
			continue
		}
		includes = append(includes, g)
	}
	var matched []string
	for _, inc := range includes {
		matched = append(matched, expandWorkspaceGlob(root, inc, "package.json")...)
	}
	if len(excludes) > 0 {
		exSet := map[string]bool{}
		for _, ex := range excludes {
			for _, p := range expandWorkspaceGlob(root, ex, "package.json") {
				exSet[p] = true
			}
		}
		filtered := matched[:0]
		for _, p := range matched {
			if !exSet[p] {
				filtered = append(filtered, p)
			}
		}
		matched = filtered
	}
	out := make([]workspaceInfo, 0, len(matched))
	for _, rel := range matched {
		manifest := filepath.Join(root, rel, "package.json")
		ws := workspaceInfo{
			ID:       jsManifestName(manifest),
			Path:     rel,
			Manifest: manifest,
		}
		if ws.ID == "" {
			ws.ID = filepath.Base(rel)
		}
		ws.Signals, ws.HasDev = jsManifestSignals(manifest)
		out = append(out, ws)
	}
	return out
}

// expandWorkspaceGlob expands a single glob pattern (literal, /*, or /**)
// against the project root and returns relative paths to directories
// containing the requiredFile (e.g. "package.json" or "Cargo.toml").
func expandWorkspaceGlob(root, glob, requiredFile string) []string {
	glob = filepath.Clean(glob)
	// Normalize Windows-style backslashes.
	glob = strings.ReplaceAll(glob, "\\", "/")
	// Strip trailing slash if any.
	glob = strings.TrimSuffix(glob, "/")

	doubleStar := strings.HasSuffix(glob, "/**")
	singleStar := strings.HasSuffix(glob, "/*")
	switch {
	case doubleStar:
		base := strings.TrimSuffix(glob, "/**")
		return walkForManifests(root, base, requiredFile, -1)
	case singleStar:
		base := strings.TrimSuffix(glob, "/*")
		return walkForManifests(root, base, requiredFile, 1)
	default:
		// Literal path.
		if fileExists(filepath.Join(root, glob, requiredFile)) {
			return []string{glob}
		}
		return nil
	}
}

// parseCargoWorkspaces parses Cargo.toml's [workspace] members glob list.
// Stdlib-only; recognises only the flat top-level [workspace] block plus a
// `members = [...]` array (the common shape).
func parseCargoWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return nil, false
	}
	text := string(data)
	wsIdx := strings.Index(text, "[workspace]")
	if wsIdx < 0 {
		return nil, false
	}
	// Find the `members = [...]` array within the [workspace] section.
	// Conservative scan: stop at the next [section] header.
	rest := text[wsIdx:]
	end := len(rest)
	for i := 1; i < len(rest); i++ {
		if rest[i] == '[' && (i == 0 || rest[i-1] == '\n') && !strings.HasPrefix(rest[i:], "[workspace]") {
			end = i
			break
		}
	}
	section := rest[:end]
	mIdx := strings.Index(section, "members")
	if mIdx < 0 {
		return nil, false
	}
	// Find opening [ and closing ].
	openB := strings.Index(section[mIdx:], "[")
	if openB < 0 {
		return nil, false
	}
	openB += mIdx
	closeB := strings.Index(section[openB:], "]")
	if closeB < 0 {
		return nil, false
	}
	closeB += openB
	listBody := section[openB+1 : closeB]
	var globs []string
	for _, item := range strings.Split(listBody, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		if item != "" {
			globs = append(globs, item)
		}
	}
	if len(globs) == 0 {
		return nil, false
	}
	var out []workspaceInfo
	for _, g := range globs {
		paths := expandWorkspaceGlob(root, g, "Cargo.toml")
		for _, rel := range paths {
			manifest := filepath.Join(root, rel, "Cargo.toml")
			ws := workspaceInfo{
				ID:       cargoCrateName(manifest),
				Path:     rel,
				Manifest: manifest,
			}
			if ws.ID == "" {
				ws.ID = filepath.Base(rel)
			}
			if fileExists(filepath.Join(root, rel, "build.rs")) {
				ws.Signals.BuildRs = true
			}
			out = append(out, ws)
		}
	}
	return out, len(out) > 0
}

// parseGoWorkspaces parses go.work's `use (...)` directives.
func parseGoWorkspaces(root string) ([]workspaceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return nil, false
	}
	var paths []string
	inUse := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := raw
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "use (") || trimmed == "use (" {
			inUse = true
			continue
		}
		if inUse {
			if trimmed == ")" {
				inUse = false
				continue
			}
			paths = append(paths, strings.Trim(trimmed, `"`))
			continue
		}
		if strings.HasPrefix(trimmed, "use ") {
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "use "))
			v = strings.Trim(v, `"`)
			if v != "" {
				paths = append(paths, v)
			}
		}
	}
	var out []workspaceInfo
	for _, p := range paths {
		clean := filepath.Clean(p)
		// "." represents the root module; skip it (single-package shape).
		if clean == "." {
			continue
		}
		manifest := filepath.Join(root, clean, "go.mod")
		if !fileExists(manifest) {
			continue
		}
		ws := workspaceInfo{
			ID:       filepath.Base(clean),
			Path:     clean,
			Manifest: manifest,
		}
		out = append(out, ws)
	}
	return out, len(out) > 0
}

// parsePyprojectWorkspaces detects [tool.uv.workspace] or [tool.poetry.group]
// members in a root pyproject.toml. Returns (workspaces, type, ok).
func parsePyprojectWorkspaces(root string) ([]workspaceInfo, monorepoType, bool) {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return nil, monorepoNone, false
	}
	text := string(data)
	// uv: [tool.uv.workspace] members = [...]
	if uvIdx := strings.Index(text, "[tool.uv.workspace]"); uvIdx >= 0 {
		section := sliceUntilNextHeader(text[uvIdx:])
		if globs := extractTomlArray(section, "members"); len(globs) > 0 {
			var out []workspaceInfo
			for _, g := range globs {
				for _, rel := range expandWorkspaceGlob(root, g, "pyproject.toml") {
					manifest := filepath.Join(root, rel, "pyproject.toml")
					ws := workspaceInfo{
						ID:       pythonProjectName(manifest),
						Path:     rel,
						Manifest: manifest,
					}
					if ws.ID == "" {
						ws.ID = filepath.Base(rel)
					}
					ws.Signals = pythonManifestSignals(manifest)
					out = append(out, ws)
				}
			}
			if len(out) > 0 {
				return out, monorepoUv, true
			}
		}
	}
	// poetry: [tool.poetry.group.<name>.dependencies] is package-deps grouping,
	// not a monorepo. Use a heuristic: poetry has no native monorepo, but users
	// often declare `[tool.poetry] packages = [{include = "x"}]`. Skip for now —
	// uv is the modern path. A repo with pyproject.toml subdirs but no uv config
	// will be detected as single-package.
	return nil, monorepoNone, false
}

// resolveWorkspaces decides the final set of workspaces for a worktree run.
// Explicit overrides in worktree.json take precedence over auto-detection.
// Returns workspaces, primary workspace ID, and the detected type. Any of
// these may be zero values when the project is single-package.
func resolveWorkspaces(root string, hooks *worktreeHooks) ([]workspaceInfo, string, monorepoType) {
	if hooks != nil && len(hooks.Workspaces) > 0 {
		ws := make([]workspaceInfo, 0, len(hooks.Workspaces))
		for id, override := range hooks.Workspaces {
			info := workspaceInfo{ID: id, Path: filepath.Clean(override.Path)}
			info.Manifest = guessManifest(filepath.Join(root, info.Path))
			if info.Manifest != "" {
				switch filepath.Base(info.Manifest) {
				case "package.json":
					info.Signals, info.HasDev = jsManifestSignals(info.Manifest)
				case "pyproject.toml":
					info.Signals = pythonManifestSignals(info.Manifest)
				case "Cargo.toml":
					if fileExists(filepath.Join(root, info.Path, "build.rs")) {
						info.Signals.BuildRs = true
					}
				}
			}
			ws = append(ws, info)
		}
		sort.Slice(ws, func(i, j int) bool { return ws[i].ID < ws[j].ID })
		_, mType := detectWorkspaces(root)
		return ws, pickPrimary(ws, hooks.PrimaryWorkspace), mType
	}
	ws, mType := detectWorkspaces(root)
	if len(ws) == 0 {
		return nil, "", monorepoNone
	}
	primary := ""
	if hooks != nil {
		primary = hooks.PrimaryWorkspace
	}
	return ws, pickPrimary(ws, primary), mType
}

// pickPrimary chooses the primary workspace ID. Order: explicit override
// (if it matches a known workspace) → first workspace with HasDev → first
// workspace overall.
func pickPrimary(ws []workspaceInfo, override string) string {
	if override != "" {
		for _, w := range ws {
			if w.ID == override {
				return override
			}
		}
	}
	for _, w := range ws {
		if w.HasDev {
			return w.ID
		}
	}
	if len(ws) > 0 {
		return ws[0].ID
	}
	return ""
}

// monorepoEnvVars returns the BELMONT_MONOREPO* env vars to inject into a
// subprocess. Returns nil for non-monorepo projects.
func monorepoEnvVars(workspaces []workspaceInfo, primary string, mType monorepoType) []string {
	if mType == monorepoNone || len(workspaces) == 0 {
		return nil
	}
	type wsEntry struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	entries := make([]wsEntry, 0, len(workspaces))
	primaryPath := ""
	for _, w := range workspaces {
		entries = append(entries, wsEntry{ID: w.ID, Path: w.Path})
		if w.ID == primary {
			primaryPath = w.Path
		}
	}
	wsJSON, err := json.Marshal(entries)
	if err != nil {
		wsJSON = []byte("[]")
	}
	out := []string{
		"BELMONT_MONOREPO=1",
		fmt.Sprintf("BELMONT_MONOREPO_TYPE=%s", string(mType)),
		fmt.Sprintf("BELMONT_WORKSPACES=%s", string(wsJSON)),
	}
	if primary != "" {
		out = append(out, fmt.Sprintf("BELMONT_PRIMARY_WORKSPACE=%s", primary))
	}
	if primaryPath != "" {
		out = append(out, fmt.Sprintf("BELMONT_PRIMARY_WORKSPACE_PATH=%s", primaryPath))
	}
	return out
}

// walkForManifests scans subdirectories of root/base looking for the named
// manifest file. depth=-1 means recurse without bound; depth=1 means only
// immediate children. node_modules and .git directories are skipped.
func walkForManifests(root, base, manifestName string, depth int) []string {
	startDir := filepath.Join(root, base)
	st, err := os.Stat(startDir)
	if err != nil || !st.IsDir() {
		return nil
	}
	var out []string
	var walk func(rel string, remaining int)
	walk = func(rel string, remaining int) {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".") {
				continue
			}
			child := filepath.Join(rel, name)
			if fileExists(filepath.Join(root, child, manifestName)) {
				out = append(out, child)
				continue
			}
			if remaining == 0 {
				continue
			}
			next := remaining
			if remaining > 0 {
				next = remaining - 1
			}
			walk(child, next)
		}
	}
	walk(base, depth)
	return out
}

// jsManifestSignals reads a package.json and returns env-consumption signals
// plus whether the manifest has a `dev` script (used for primary-workspace
// selection).
func jsManifestSignals(path string) (envSignals, bool) {
	var sig envSignals
	data, err := os.ReadFile(path)
	if err != nil {
		return sig, false
	}
	var pkg struct {
		Scripts          map[string]string `json:"scripts"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return sig, false
	}
	hasDev := false
	for k := range pkg.Scripts {
		if k == "dev" {
			hasDev = true
		}
		if k == "postinstall" || strings.HasPrefix(k, "postinstall:") {
			sig.Postinstall = true
		}
	}
	merged := map[string]struct{}{}
	for k := range pkg.Dependencies {
		merged[k] = struct{}{}
	}
	for k := range pkg.DevDependencies {
		merged[k] = struct{}{}
	}
	for k := range pkg.PeerDependencies {
		merged[k] = struct{}{}
	}
	for _, prismaDep := range []string{"prisma", "@prisma/client"} {
		if _, ok := merged[prismaDep]; ok {
			sig.PrismaDep = true
		}
	}
	for _, dotenvDep := range []string{"dotenv", "dotenv-cli", "drizzle-kit", "tsx", "vite-node"} {
		if _, ok := merged[dotenvDep]; ok {
			sig.DotenvDep = true
		}
	}
	return sig, hasDev
}
