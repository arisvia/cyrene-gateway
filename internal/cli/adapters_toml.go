package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// --- OpenAI Codex (~/.codex/config.toml + auth.json) ---

type codexAdapter struct{}

func (a *codexAdapter) dir() string        { return filepath.Join(homeDir(), ".codex") }
func (a *codexAdapter) configPath() string { return filepath.Join(a.dir(), "config.toml") }
func (a *codexAdapter) authPath() string   { return filepath.Join(a.dir(), "auth.json") }

func (a *codexAdapter) Status() Status {
	p := a.configPath()
	if !installed("codex", p) {
		return Status{Installed: false, Message: "Codex CLI is not installed"}
	}
	content := readText(p)
	hasGW := strings.Contains(content, `model_provider = "9router"`) ||
		strings.Contains(content, "[model_providers.9router]")
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *codexAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	content = tomlSetTopLevel(content, "model", req.Model)
	content = tomlSetTopLevel(content, "model_provider", "9router")
	content = tomlSetField(content, "model_providers.9router", "name", "9Router")
	content = tomlSetField(content, "model_providers.9router", "base_url", ensureV1(req.BaseURL))
	content = tomlSetField(content, "model_providers.9router", "wire_api", "responses")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	// auth.json holds the API key.
	auth := readJSON(a.authPath())
	if auth == nil {
		auth = map[string]any{}
	}
	auth["OPENAI_API_KEY"] = req.APIKey
	auth["auth_mode"] = "apikey"
	if err := writeJSON(a.authPath(), auth); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *codexAdapter) Reset() (Status, error) {
	p := a.configPath()
	content := readText(p)
	if content == "" {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	if v, _ := tomlGetTopLevel(content, "model_provider"); v == "9router" {
		content = tomlDeleteTopLevel(content, "model")
		content = tomlDeleteTopLevel(content, "model_provider")
	}
	content = tomlDeleteSection(content, "model_providers.9router")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	auth := readJSON(a.authPath())
	if auth != nil {
		delete(auth, "OPENAI_API_KEY")
		delete(auth, "auth_mode")
		if len(auth) == 0 {
			os.Remove(a.authPath())
		} else {
			writeJSON(a.authPath(), auth)
		}
	}
	return a.Status(), nil
}

// --- DeepSeek TUI (~/.deepseek/config.toml) ---

type deepseekTuiAdapter struct{}

func (a *deepseekTuiAdapter) configPath() string {
	return filepath.Join(homeDir(), ".deepseek", "config.toml")
}

func (a *deepseekTuiAdapter) Status() Status {
	p := a.configPath()
	if !installed("deepseek", p) {
		return Status{Installed: false, Message: "DeepSeek TUI is not installed"}
	}
	content := readText(p)
	hasGW := false
	if v, ok := tomlGetTopLevel(content, "provider"); ok && v == "openai" {
		if base, ok := tomlGetField(content, "providers.openai", "base_url"); ok && looksLikeGateway(base) {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *deepseekTuiAdapter) Apply(req ApplyRequest) (Status, error) {
	key := req.APIKey
	if key == "" {
		key = "sk_9router"
	}
	base := ensureV1(req.BaseURL)
	content := `provider = "openai"

[providers.openai]
base_url = ` + tomlString(base) + `
api_key = ` + tomlString(key) + `
model = ` + tomlString(req.Model) + `
`
	if err := writeText(a.configPath(), content); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *deepseekTuiAdapter) Reset() (Status, error) {
	p := a.configPath()
	if !fileExists(p) {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	if err := writeText(p, "provider = \"deepseek\"\n"); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- Grok Build (~/.grok/config.toml) ---

type grokBuildAdapter struct{}

func (a *grokBuildAdapter) configPath() string {
	return filepath.Join(homeDir(), ".grok", "config.toml")
}

func (a *grokBuildAdapter) Status() Status {
	p := a.configPath()
	if !installed("grok", p) {
		return Status{Installed: false, Message: "Grok Build is not installed"}
	}
	content := readText(p)
	_, hasGW := tomlGetField(content, "model.9router", "base_url")
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *grokBuildAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	key := req.APIKey
	if key == "" {
		key = "sk_9router"
	}
	base := ensureV1(req.BaseURL)
	content = tomlSetField(content, "model.9router", "model", req.Model)
	content = tomlSetField(content, "model.9router", "base_url", base)
	content = tomlSetField(content, "model.9router", "name", "9Router")
	content = tomlSetField(content, "model.9router", "api_backend", "chat_completions")
	content = tomlSetField(content, "model.9router", "api_key", key)
	// Set as default model.
	content = tomlSetField(content, "models", "default", "9router")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *grokBuildAdapter) Reset() (Status, error) {
	p := a.configPath()
	content := readText(p)
	if content == "" {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	content = tomlDeleteSection(content, "model.9router")
	if v, _ := tomlGetField(content, "models", "default"); v == "9router" {
		content = tomlSetField(content, "models", "default", "grok-build")
	}
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- jcode (~/.jcode/config.toml + provider env file) ---

type jcodeAdapter struct{}

func (a *jcodeAdapter) configPath() string {
	return filepath.Join(homeDir(), ".jcode", "config.toml")
}

func (a *jcodeAdapter) envPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(xdg, "jcode", "provider-9router.env")
}

func (a *jcodeAdapter) Status() Status {
	p := a.configPath()
	if !installed("jcode", p) {
		return Status{Installed: false, Message: "jcode is not installed"}
	}
	content := readText(p)
	_, hasGW := tomlGetField(content, "providers.9router", "base_url")
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *jcodeAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	base := ensureV1(req.BaseURL)
	defaultModel := req.Model
	if defaultModel == "" && len(req.Models) > 0 {
		defaultModel = req.Models[0]
	}
	if defaultModel == "" {
		defaultModel = "cc/claude-opus-4-7"
	}
	content = tomlSetField(content, "providers.9router", "type", "openai-compatible")
	content = tomlSetField(content, "providers.9router", "base_url", base)
	content = tomlSetField(content, "providers.9router", "auth", "bearer")
	content = tomlSetField(content, "providers.9router", "api_key_env", "JCODE_9ROUTER_API_KEY")
	content = tomlSetField(content, "providers.9router", "env_file", "provider-9router.env")
	content = tomlSetField(content, "providers.9router", "default_model", defaultModel)
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	// Write the API key to the provider env file.
	envText := readText(a.envPath())
	envText = envUpsert(envText, "JCODE_9ROUTER_API_KEY", req.APIKey)
	if err := writeText(a.envPath(), envText); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *jcodeAdapter) Reset() (Status, error) {
	p := a.configPath()
	content := readText(p)
	if content == "" {
		return Status{Installed: true, Message: "No configuration to remove"}, nil
	}
	content = tomlDeleteSection(content, "providers.9router")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	envText := readText(a.envPath())
	if envText != "" {
		writeText(a.envPath(), envRemove(envText, "JCODE_9ROUTER_API_KEY"))
	}
	return a.Status(), nil
}

// --- Hermes Agent (~/.hermes/config.yaml + .env) ---

type hermesAdapter struct{}

func (a *hermesAdapter) configPath() string {
	return filepath.Join(homeDir(), ".hermes", "config.yaml")
}
func (a *hermesAdapter) envPath() string { return filepath.Join(homeDir(), ".hermes", ".env") }

func (a *hermesAdapter) Status() Status {
	p := a.configPath()
	if !installed("hermes", p) {
		return Status{Installed: false, Message: "Hermes Agent is not installed"}
	}
	yaml := readText(p)
	hasGW := false
	if strings.Contains(yaml, "provider: \"custom\"") || strings.Contains(yaml, "provider: custom") {
		if idx := strings.Index(yaml, "base_url:"); idx >= 0 {
			line := yamlLineValue(yaml[idx:])
			if looksLikeGateway(line) {
				hasGW = true
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *hermesAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	yaml := readText(p)
	base := ensureV1(req.BaseURL)
	block := "model:\n  default: " + yamlQuote(req.Model) + "\n  provider: \"custom\"\n  base_url: " + yamlQuote(base) + "\n"
	yaml = upsertYAMLModelBlock(yaml, block)
	if err := writeText(p, yaml); err != nil {
		return Status{}, err
	}
	if req.APIKey != "" {
		envText := readText(a.envPath())
		envText = envUpsert(envText, "OPENAI_API_KEY", req.APIKey)
		if err := writeText(a.envPath(), envText); err != nil {
			return Status{}, err
		}
	}
	return a.Status(), nil
}

func (a *hermesAdapter) Reset() (Status, error) {
	p := a.configPath()
	yaml := readText(p)
	if yaml == "" {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	yaml = removeYAMLModelBlock(yaml)
	if err := writeText(p, yaml); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// yamlQuote quotes a value for YAML output.
func yamlQuote(v string) string { return `"` + v + `"` }

// yamlLineValue extracts the scalar value from a "key: value" line fragment.
func yamlLineValue(fragment string) string {
	end := strings.IndexAny(fragment, "\r\n")
	if end < 0 {
		end = len(fragment)
	}
	line := fragment[:end]
	idx := strings.Index(line, ":")
	if idx < 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(line[idx+1:]), `"'`)
}

// upsertYAMLModelBlock replaces an existing top-level "model:" block or
// prepends the new block.
func upsertYAMLModelBlock(yaml, block string) string {
	if removeYAMLModelBlock(yaml) != yaml {
		return removeYAMLModelBlock(yaml) + block
	}
	if yaml != "" {
		return block + "\n" + yaml
	}
	return block
}

// removeYAMLModelBlock removes a top-level "model:" block (the model: line and
// its indented children).
func removeYAMLModelBlock(yaml string) string {
	lines := strings.Split(yaml, "\n")
	var out []string
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if !skipping && (trimmed == "model:" || strings.HasPrefix(trimmed, "model: ")) {
			skipping = true
			continue
		}
		if skipping {
			// Continue skipping indented (child) or blank lines.
			if trimmed == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				continue
			}
			skipping = false
		}
		out = append(out, line)
	}
	result := strings.Join(out, "\n")
	return strings.TrimLeft(result, "\n")
}
