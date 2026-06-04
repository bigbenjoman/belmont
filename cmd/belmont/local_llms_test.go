package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempHome redirects $HOME to a temp dir for the duration of the test, so
// userLocalLLMsPath() points somewhere we own. Returns the temp HOME path.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// clearPiEnv unsets every BELMONT_PI_* env var the resolver reads, so a test
// starts from a known-clean baseline.
func clearPiEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"BELMONT_PI_PROVIDER", "BELMONT_PI_MODEL",
		"BELMONT_PI_PROVIDER_LOW", "BELMONT_PI_MODEL_LOW",
		"BELMONT_PI_PROVIDER_MEDIUM", "BELMONT_PI_MODEL_MEDIUM",
		"BELMONT_PI_PROVIDER_HIGH", "BELMONT_PI_MODEL_HIGH",
	} {
		t.Setenv(v, "")
	}
}

func writeUserConfig(t *testing.T, home, content string) {
	t.Helper()
	path := filepath.Join(home, ".belmont", "local-llms.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProjectConfig(t *testing.T, projectRoot, content string) {
	t.Helper()
	path := filepath.Join(projectRoot, ".belmont", "local-llms.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePiModelFlags_NoConfig_NoEnv(t *testing.T) {
	withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	flags := resolvePiModelFlags(project, "high")
	if len(flags) != 0 {
		t.Errorf("expected empty flags, got: %v", flags)
	}
}

func TestResolvePiModelFlags_UserFileOnly(t *testing.T) {
	home := withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{
		"pi": {
			"tiers": {
				"high": {"provider": "ollama", "model": "deepseek-coder-v3"}
			}
		}
	}`)

	flags := resolvePiModelFlags(project, "high")
	want := []string{"--provider", "ollama", "--model", "deepseek-coder-v3"}
	if !equalStringSlices(flags, want) {
		t.Errorf("flags = %v, want %v", flags, want)
	}

	// A tier not in the config should yield empty flags (Pi falls back to its default).
	if got := resolvePiModelFlags(project, "low"); len(got) != 0 {
		t.Errorf("expected empty flags for unconfigured tier, got: %v", got)
	}
}

func TestResolvePiModelFlags_ProjectOverridesUser(t *testing.T) {
	home := withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{
		"pi": {
			"tiers": {
				"medium": {"provider": "lm-studio", "model": "qwen3-7b"}
			}
		}
	}`)
	writeProjectConfig(t, project, `{
		"pi": {
			"tiers": {
				"medium": {"provider": "lm-studio", "model": "qwen3-30b"}
			}
		}
	}`)

	flags := resolvePiModelFlags(project, "medium")
	want := []string{"--provider", "lm-studio", "--model", "qwen3-30b"}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected project to override user; flags = %v, want %v", flags, want)
	}
}

func TestResolvePiModelFlags_ProjectOverridesUser_PerField(t *testing.T) {
	// Project sets only the model; provider should still come from the user file.
	home := withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{
		"pi": {
			"tiers": {
				"high": {"provider": "ollama", "model": "qwen3-7b"}
			}
		}
	}`)
	writeProjectConfig(t, project, `{
		"pi": {
			"tiers": {
				"high": {"model": "deepseek-coder-v3"}
			}
		}
	}`)

	flags := resolvePiModelFlags(project, "high")
	want := []string{"--provider", "ollama", "--model", "deepseek-coder-v3"}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected per-field merge; flags = %v, want %v", flags, want)
	}
}

func TestResolvePiModelFlags_SingleEnvVarApplyToAllTiers(t *testing.T) {
	withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	t.Setenv("BELMONT_PI_PROVIDER", "lm-studio")
	t.Setenv("BELMONT_PI_MODEL", "qwen3-30b")

	for _, tier := range []string{"low", "medium", "high"} {
		flags := resolvePiModelFlags(project, tier)
		want := []string{"--provider", "lm-studio", "--model", "qwen3-30b"}
		if !equalStringSlices(flags, want) {
			t.Errorf("tier %s: flags = %v, want %v", tier, flags, want)
		}
	}
}

func TestResolvePiModelFlags_PerTierEnvVarOverrideEverything(t *testing.T) {
	home := withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	// Plant config files that would otherwise win.
	writeUserConfig(t, home, `{
		"pi": {"tiers": {"high": {"provider": "ollama", "model": "qwen3-7b"}}}
	}`)
	writeProjectConfig(t, project, `{
		"pi": {"tiers": {"high": {"provider": "lm-studio", "model": "qwen3-30b"}}}
	}`)

	t.Setenv("BELMONT_PI_PROVIDER_HIGH", "vllm")
	t.Setenv("BELMONT_PI_MODEL_HIGH", "deepseek-coder-v3")

	flags := resolvePiModelFlags(project, "high")
	want := []string{"--provider", "vllm", "--model", "deepseek-coder-v3"}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected env vars to win; flags = %v, want %v", flags, want)
	}

	// "medium" wasn't given a per-tier env var; should fall through to the
	// project config.
	flags = resolvePiModelFlags(project, "medium")
	if len(flags) != 0 {
		t.Errorf("medium tier was unconfigured at every level; expected empty flags, got: %v", flags)
	}
}

func TestResolvePiModelFlags_PartialEnvVarFallsThroughForOtherField(t *testing.T) {
	home := withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	// User file provides provider, env var provides model.
	writeUserConfig(t, home, `{
		"pi": {"tiers": {"high": {"provider": "ollama"}}}
	}`)
	t.Setenv("BELMONT_PI_MODEL_HIGH", "deepseek-coder-v3")

	flags := resolvePiModelFlags(project, "high")
	want := []string{"--provider", "ollama", "--model", "deepseek-coder-v3"}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected env model + file provider; flags = %v, want %v", flags, want)
	}
}

func TestResolveModelFlags_PiDispatch(t *testing.T) {
	// Sanity: resolveModelFlags routes Pi through the local-llms chain when
	// tool == "pi", and back through modelTiers for everyone else.
	withTempHome(t)
	clearPiEnv(t)
	project := t.TempDir()

	t.Setenv("BELMONT_PI_PROVIDER", "lm-studio")
	t.Setenv("BELMONT_PI_MODEL", "qwen3-30b")

	flags := resolveModelFlags("pi", "high", project)
	if !equalStringSlices(flags, []string{"--provider", "lm-studio", "--model", "qwen3-30b"}) {
		t.Errorf("pi dispatch failed; got: %v", flags)
	}

	// Claude still uses the modelTiers path — projectRoot is ignored.
	flags = resolveModelFlags("claude", "high", project)
	if len(flags) != 2 || flags[0] != "--model" || flags[1] != "opus" {
		t.Errorf("claude dispatch should return --model opus, got: %v", flags)
	}
}

// clearCodexEnv unsets every BELMONT_CODEX_* env var the resolver reads, so a
// test starts from a known-clean baseline.
func clearCodexEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"BELMONT_CODEX_MODEL",
		"BELMONT_CODEX_MODEL_LOW",
		"BELMONT_CODEX_MODEL_MEDIUM",
		"BELMONT_CODEX_MODEL_HIGH",
		"BELMONT_CODEX_REASONING_EFFORT",
		"BELMONT_CODEX_REASONING_EFFORT_LOW",
		"BELMONT_CODEX_REASONING_EFFORT_MEDIUM",
		"BELMONT_CODEX_REASONING_EFFORT_HIGH",
		"BELMONT_CODEX_SERVICE_TIER",
		"BELMONT_CODEX_SERVICE_TIER_LOW",
		"BELMONT_CODEX_SERVICE_TIER_MEDIUM",
		"BELMONT_CODEX_SERVICE_TIER_HIGH",
	} {
		t.Setenv(v, "")
	}
}

func TestResolveCodexModelFlags_DefaultTiers(t *testing.T) {
	withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	for tier, want := range modelTiers["codex"] {
		flags := resolveCodexModelFlags(project, tier)
		wantFlags := []string{"--model", want, "-c", `model_reasoning_effort="` + codexReasoningEfforts[tier] + `"`}
		if !equalStringSlices(flags, wantFlags) {
			t.Errorf("tier %s: expected %v, got %v", tier, wantFlags, flags)
		}
	}
}

func TestResolveCodexModelFlags_EmptyTier_NoFlags(t *testing.T) {
	withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	flags := resolveCodexModelFlags(project, "")
	if len(flags) != 0 {
		t.Errorf("expected no flags for empty tier, got %v", flags)
	}
}

func TestResolveCodexModelFlags_UserFile(t *testing.T) {
	home := withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{"codex": {"tiers": {"high": {"model": "gpt-5.5"}}}}`)

	flags := resolveCodexModelFlags(project, "high")
	want := []string{"--model", "gpt-5.5", "-c", `model_reasoning_effort="high"`}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected %v, got %v", want, flags)
	}

	flags = resolveCodexModelFlags(project, "low")
	want = []string{"--model", modelTiers["codex"]["low"], "-c", `model_reasoning_effort="low"`}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected built-in low default, got %v", flags)
	}
}

func TestResolveCodexModelFlags_ProjectOverridesUser_PerField(t *testing.T) {
	home := withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{"codex": {"tiers": {"medium": {"model": "gpt-5.5", "reasoning_effort": "high"}}}}`)
	writeProjectConfig(t, project, `{"codex": {"tiers": {"medium": {"model": "gpt-5.4", "service_tier": "fast"}}}}`)

	flags := resolveCodexModelFlags(project, "medium")
	want := []string{"--model", "gpt-5.4", "-c", `model_reasoning_effort="high"`, "-c", `service_tier="fast"`}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected project/user merged flags, got %v", flags)
	}
}

func TestResolveCodexModelFlags_SingleEnvVarsApplyToAllTiers(t *testing.T) {
	withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()
	t.Setenv("BELMONT_CODEX_MODEL", "gpt-5.4-mini")
	t.Setenv("BELMONT_CODEX_REASONING_EFFORT", "low")
	t.Setenv("BELMONT_CODEX_SERVICE_TIER", "fast")

	for _, tier := range []string{"low", "medium", "high"} {
		flags := resolveCodexModelFlags(project, tier)
		want := []string{"--model", "gpt-5.4-mini", "-c", `model_reasoning_effort="low"`, "-c", `service_tier="fast"`}
		if !equalStringSlices(flags, want) {
			t.Errorf("tier %s: expected env override, got %v", tier, flags)
		}
	}
}

func TestResolveCodexModelFlags_PerTierEnvOverridesEverything(t *testing.T) {
	home := withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{"codex": {"tiers": {"high": {"model": "gpt-5.4"}}}}`)
	writeProjectConfig(t, project, `{"codex": {"tiers": {"high": {"model": "gpt-5.4-mini"}}}}`)
	t.Setenv("BELMONT_CODEX_MODEL", "gpt-5.4-mini")
	t.Setenv("BELMONT_CODEX_MODEL_HIGH", "gpt-5.5")
	t.Setenv("BELMONT_CODEX_REASONING_EFFORT", "low")
	t.Setenv("BELMONT_CODEX_REASONING_EFFORT_HIGH", "xhigh")

	flags := resolveCodexModelFlags(project, "high")
	want := []string{"--model", "gpt-5.5", "-c", `model_reasoning_effort="xhigh"`}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected per-tier env var to win, got %v", flags)
	}
}

func TestResolveModelFlags_CodexDispatch(t *testing.T) {
	withTempHome(t)
	clearCodexEnv(t)
	project := t.TempDir()

	t.Setenv("BELMONT_CODEX_MODEL_MEDIUM", "gpt-5.4")
	flags := resolveModelFlags("codex", "medium", project)
	want := []string{"--model", "gpt-5.4", "-c", `model_reasoning_effort="medium"`}
	if !equalStringSlices(flags, want) {
		t.Errorf("expected resolveModelFlags to dispatch to codex chain, got %v", flags)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sanity: malformed JSON in the config file should be reported as an error
// from loadLocalLLMs rather than being silently treated as "no config." That
// ensures users get a clear "you broke your config" diagnostic instead of
// puzzling silent fallback behaviour.
func TestLoadLocalLLMs_MalformedReturnsError(t *testing.T) {
	home := withTempHome(t)
	writeUserConfig(t, home, `{not valid json`)

	if _, err := loadLocalLLMs(""); err == nil {
		t.Errorf("expected error from malformed user config")
	}

	// And the resolver should fall safely through to the env-var-only path
	// (which is also empty here) rather than panicking. resolvePiTierValues
	// swallows loadLocalLLMs errors by design — the alternative would be
	// `belmont auto` aborting because of a typo in a global config file.
	clearPiEnv(t)
	if got := resolvePiModelFlags("", "high"); len(got) != 0 {
		t.Errorf("resolver should return safely on malformed config, got: %v", got)
	}
}

// clearOpencodeEnv unsets every BELMONT_OPENCODE_* env var the resolver
// reads, so a test starts from a known-clean baseline.
func clearOpencodeEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"BELMONT_OPENCODE_MODEL",
		"BELMONT_OPENCODE_MODEL_LOW",
		"BELMONT_OPENCODE_MODEL_MEDIUM",
		"BELMONT_OPENCODE_MODEL_HIGH",
	} {
		t.Setenv(v, "")
	}
}

func TestResolveOpencodeModelFlags_DefaultTiers(t *testing.T) {
	withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	// No config + no env → the built-in modelTiers defaults (unlike Pi,
	// which has no defaults and passes no flags).
	for tier, want := range modelTiers["opencode"] {
		flags := resolveOpencodeModelFlags(project, tier)
		if len(flags) != 2 || flags[0] != "--model" || flags[1] != want {
			t.Errorf("tier %s: expected [--model %s], got %v", tier, want, flags)
		}
	}
}

func TestResolveOpencodeModelFlags_EmptyTier_NoFlags(t *testing.T) {
	withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	flags := resolveOpencodeModelFlags(project, "")
	if len(flags) != 0 {
		t.Errorf("expected no flags for empty tier, got %v", flags)
	}
}

func TestResolveOpencodeModelFlags_UserFileFullID(t *testing.T) {
	home := withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	// Full provider/model ID in the `model` field.
	writeUserConfig(t, home, `{"opencode": {"tiers": {"high": {"model": "opencode/gpt-5.1-codex"}}}}`)

	flags := resolveOpencodeModelFlags(project, "high")
	if len(flags) != 2 || flags[1] != "opencode/gpt-5.1-codex" {
		t.Errorf("expected [--model opencode/gpt-5.1-codex], got %v", flags)
	}

	// Unconfigured tier falls through to the built-in default.
	flags = resolveOpencodeModelFlags(project, "low")
	if len(flags) != 2 || flags[1] != modelTiers["opencode"]["low"] {
		t.Errorf("expected built-in low default, got %v", flags)
	}
}

func TestResolveOpencodeModelFlags_ProviderModelJoined(t *testing.T) {
	home := withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	// Split provider + model (Pi-schema symmetry) is joined with "/".
	writeUserConfig(t, home, `{"opencode": {"tiers": {"medium": {"provider": "lmstudio", "model": "qwen3-coder-30b"}}}}`)

	flags := resolveOpencodeModelFlags(project, "medium")
	if len(flags) != 2 || flags[1] != "lmstudio/qwen3-coder-30b" {
		t.Errorf("expected [--model lmstudio/qwen3-coder-30b], got %v", flags)
	}
}

func TestResolveOpencodeModelFlags_ProjectOverridesUser(t *testing.T) {
	home := withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{"opencode": {"tiers": {"high": {"model": "anthropic/claude-opus-4-8"}}}}`)
	writeProjectConfig(t, project, `{"opencode": {"tiers": {"high": {"model": "opencode/gpt-5.1-codex"}}}}`)

	flags := resolveOpencodeModelFlags(project, "high")
	if len(flags) != 2 || flags[1] != "opencode/gpt-5.1-codex" {
		t.Errorf("expected project override, got %v", flags)
	}
}

func TestResolveOpencodeModelFlags_SingleEnvVarAppliesToAllTiers(t *testing.T) {
	withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()
	t.Setenv("BELMONT_OPENCODE_MODEL", "openai/gpt-5.2")

	for _, tier := range []string{"low", "medium", "high"} {
		flags := resolveOpencodeModelFlags(project, tier)
		if len(flags) != 2 || flags[1] != "openai/gpt-5.2" {
			t.Errorf("tier %s: expected env override, got %v", tier, flags)
		}
	}
}

func TestResolveOpencodeModelFlags_PerTierEnvOverridesEverything(t *testing.T) {
	home := withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	writeUserConfig(t, home, `{"opencode": {"tiers": {"high": {"model": "anthropic/claude-opus-4-8"}}}}`)
	writeProjectConfig(t, project, `{"opencode": {"tiers": {"high": {"model": "anthropic/claude-sonnet-4-6"}}}}`)
	t.Setenv("BELMONT_OPENCODE_MODEL", "openai/gpt-5.2")
	t.Setenv("BELMONT_OPENCODE_MODEL_HIGH", "opencode/gpt-5.1-codex")

	flags := resolveOpencodeModelFlags(project, "high")
	if len(flags) != 2 || flags[1] != "opencode/gpt-5.1-codex" {
		t.Errorf("expected per-tier env var to win, got %v", flags)
	}
}

func TestResolveModelFlags_OpencodeDispatch(t *testing.T) {
	withTempHome(t)
	clearOpencodeEnv(t)
	project := t.TempDir()

	flags := resolveModelFlags("opencode", "medium", project)
	if len(flags) != 2 || flags[0] != "--model" || flags[1] != modelTiers["opencode"]["medium"] {
		t.Errorf("expected resolveModelFlags to dispatch to opencode chain, got %v", flags)
	}
}
