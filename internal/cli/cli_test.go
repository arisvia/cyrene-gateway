package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// useTempHome redirects user home directory to a fresh temp dir so adapters
// operate on an isolated filesystem across all platforms (Linux/macOS/Windows).
func useTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	return dir
}

func TestRegistryCompleteness(t *testing.T) {
	// The 9 configurable tools must all resolve to an adapter.
	configurable := []string{
		"claude", "codex", "opencode", "aider", "cline", "continue",
		"copilot", "dsh", "grok-cli",
	}
	m := NewManager()
	for _, id := range configurable {
		if GetTool(id) == nil {
			t.Errorf("tool %q missing from registry", id)
		}
		if m.Adapter(id) == nil {
			t.Errorf("tool %q has no adapter", id)
		}
	}
	// guide/mitm tools exist in registry but have no adapter.
	for _, id := range []string{"cursor", "roo-code", "windsurf", "trae", "qoder", "antigravity"} {
		if GetTool(id) == nil {
			t.Errorf("tool %q missing from registry", id)
		}
		if m.Adapter(id) != nil {
			t.Errorf("tool %q should not have an adapter", id)
		}
	}
	if m.Adapter("nonexistent") != nil {
		t.Error("unknown tool should return nil adapter")
	}
}

func TestAllStatusesDoesNotPanic(t *testing.T) {
	useTempHome(t)
	m := NewManager()
	statuses := m.AllStatuses()
	if len(statuses) != len(Registry) {
		t.Errorf("AllStatuses returned %d entries, want %d", len(statuses), len(Registry))
	}
}

func TestClaudeAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &claudeAdapter{}

	if s := a.Status(); s.Installed {
		t.Fatal("claude should not be installed in empty home")
	}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-test", Model: "cc/claude-sonnet-5"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := a.Status()
	if !s.Installed || !s.HasGateway {
		t.Fatalf("expected installed+gateway after apply, got %+v", s)
	}
	env := s.Settings["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "http://localhost:20128/v1" {
		t.Errorf("base url not normalized: %v", env["ANTHROPIC_BASE_URL"])
	}

	if _, err := a.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if s := a.Status(); s.HasGateway {
		t.Error("gateway config should be gone after reset")
	}
}

func TestCodexAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &codexAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-x", Model: "gpt-5"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := a.Status()
	if !s.HasGateway {
		t.Fatalf("expected gateway after apply, got %+v", s)
	}
	content := readText(a.configPath())
	if base, _ := tomlGetField(content, "model_providers.cyrene", "base_url"); base != "http://localhost:20128/v1" {
		t.Errorf("codex base_url = %q", base)
	}
	auth := readJSON(a.authPath())
	if auth["OPENAI_API_KEY"] != "sk-x" {
		t.Errorf("auth key = %v", auth["OPENAI_API_KEY"])
	}

	if _, err := a.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if s := a.Status(); s.HasGateway {
		t.Error("gateway config should be gone after reset")
	}
}

func TestClineAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &clineAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128/v1", APIKey: "sk-c", Model: "claude-sonnet"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	gs := readJSON(a.globalStatePath())
	// Cline expects base WITHOUT /v1.
	if gs["openAiBaseUrl"] != "http://localhost:20128" {
		t.Errorf("cline base = %v (want /v1 stripped)", gs["openAiBaseUrl"])
	}
	secrets := readJSON(a.secretsPath())
	if secrets["openAiApiKey"] != "sk-c" {
		t.Errorf("cline secret = %v", secrets["openAiApiKey"])
	}
	if s := a.Status(); !s.HasGateway {
		t.Error("expected gateway status")
	}

	a.Reset()
	if s := a.Status(); s.HasGateway {
		t.Error("expected reset")
	}
}

func TestAiderAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &aiderAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-aider", Model: "deepseek/deepseek-chat"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := a.Status()
	if !s.HasGateway {
		t.Fatalf("expected gateway after apply, got %+v", s)
	}
	content := readText(a.configPath())
	if !strings.Contains(content, "openai-api-base: http://localhost:20128/v1") {
		t.Errorf("aider config missing base url:\n%s", content)
	}
	if !strings.Contains(content, "openai-api-key: sk-aider") {
		t.Errorf("aider config missing api key:\n%s", content)
	}

	if _, err := a.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if s := a.Status(); s.HasGateway {
		t.Error("gateway config should be gone after reset")
	}
}

func TestContinueAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &continueAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-cont", Model: "gpt-5-mini"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := a.Status()
	if !s.HasGateway {
		t.Fatalf("expected gateway after apply, got %+v", s)
	}

	if _, err := a.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if s := a.Status(); s.HasGateway {
		t.Error("gateway config should be gone after reset")
	}
}

func TestDshAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &dshAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-d", Model: "deepseek-chat"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if s := a.Status(); !s.HasGateway {
		t.Fatalf("expected gateway, got %+v", s)
	}
	content := readText(a.configPath())
	if !strings.Contains(content, "openai_api_base: http://localhost:20128/v1") {
		t.Errorf("dsh config missing base url:\n%s", content)
	}
	if _, err := a.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	content = readText(a.configPath())
	if !strings.Contains(content, "model_provider: deepseek") {
		t.Errorf("provider after reset = %q", content)
	}
}

func TestGrokCliAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &grokCliAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128", APIKey: "sk-g", Model: "grok-4"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := a.Status()
	if !s.HasGateway {
		t.Fatalf("expected gateway, got %+v", s)
	}
	content := readText(a.configPath())
	if def, _ := tomlGetField(content, "models", "default"); def != "cyrene" {
		t.Errorf("grok default = %q", def)
	}

	a.Reset()
	if s := a.Status(); s.HasGateway {
		t.Error("expected reset")
	}
	content = readText(a.configPath())
	if def, _ := tomlGetField(content, "models", "default"); def != "grok-build" {
		t.Errorf("grok default after reset = %q", def)
	}
}

func TestCopilotAdapterRoundTrip(t *testing.T) {
	useTempHome(t)
	a := &copilotAdapter{}

	_, err := a.Apply(ApplyRequest{BaseURL: "http://localhost:20128/v1", APIKey: "sk-cp", Models: []string{"gpt-5", "claude-sonnet"}})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if s := a.Status(); !s.HasGateway {
		t.Fatalf("expected gateway, got %+v", s)
	}
	a.Reset()
	if s := a.Status(); s.HasGateway {
		t.Error("expected reset")
	}
}
