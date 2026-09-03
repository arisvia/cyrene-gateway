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
	hasGW := strings.Contains(content, `model_provider = "cyrene"`) ||
		strings.Contains(content, `[model_providers.cyrene]`) ||
		strings.Contains(content, `model_provider = "9router"`) ||
		strings.Contains(content, `[model_providers.9router]`)
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p}
}

func (a *codexAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	content = tomlSetTopLevel(content, "model", req.Model)
	content = tomlSetTopLevel(content, "model_provider", "cyrene")
	content = tomlSetField(content, "model_providers.cyrene", "name", "Cyrene Gateway")
	content = tomlSetField(content, "model_providers.cyrene", "base_url", ensureV1(req.BaseURL))
	content = tomlSetField(content, "model_providers.cyrene", "wire_api", "responses")
	// Clean up legacy section if present
	content = tomlDeleteSection(content, "model_providers.9router")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	// auth.json holds the API key.
	auth := readJSON(a.authPath())
	if auth == nil {
		auth = map[string]any{}
	}
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	auth["OPENAI_API_KEY"] = key
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
	if v, _ := tomlGetTopLevel(content, "model_provider"); v == "cyrene" || v == "9router" {
		content = tomlDeleteTopLevel(content, "model")
		content = tomlDeleteTopLevel(content, "model_provider")
	}
	content = tomlDeleteSection(content, "model_providers.cyrene")
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

// --- Aider (~/.aider.conf.yml) ---

type aiderAdapter struct{}

func (a *aiderAdapter) configPath() string {
	return filepath.Join(homeDir(), ".aider.conf.yml")
}

func (a *aiderAdapter) Status() Status {
	p := a.configPath()
	if !installed("aider", p) {
		return Status{Installed: false, Message: "Aider is not installed"}
	}
	content := readText(p)
	hasGW := false
	if strings.Contains(content, "openai-api-base:") {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "openai-api-base:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "openai-api-base:"))
				if looksLikeGateway(val) {
					hasGW = true
					break
				}
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p}
}

func (a *aiderAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	var lines []string
	if content != "" {
		for _, l := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "openai-api-base:") ||
				strings.HasPrefix(trimmed, "openai-api-key:") ||
				strings.HasPrefix(trimmed, "model:") {
				continue
			}
			lines = append(lines, l)
		}
	}
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	model := req.Model
	if model == "" {
		model = "deepseek/deepseek-chat"
	}
	newLines := []string{
		"openai-api-base: " + ensureV1(req.BaseURL),
		"openai-api-key: " + key,
		"model: " + model,
	}
	result := strings.TrimSpace(strings.Join(lines, "\n"))
	if result != "" {
		result += "\n\n"
	}
	result += strings.Join(newLines, "\n") + "\n"
	if err := writeText(p, result); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *aiderAdapter) Reset() (Status, error) {
	p := a.configPath()
	content := readText(p)
	if content == "" {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	var lines []string
	for _, l := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "openai-api-base:") ||
			strings.HasPrefix(trimmed, "openai-api-key:") {
			continue
		}
		lines = append(lines, l)
	}
	result := strings.TrimSpace(strings.Join(lines, "\n"))
	if result != "" {
		result += "\n"
	}
	if err := writeText(p, result); err != nil {
		return Status{}, err
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
		return Status{Installed: false, Message: "DeepSeek CLI is not installed"}
	}
	content := readText(p)
	hasGW := false
	if v, ok := tomlGetTopLevel(content, "provider"); ok && v == "openai" {
		if base, ok := tomlGetField(content, "providers.openai", "base_url"); ok && looksLikeGateway(base) {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p}
}

func (a *deepseekTuiAdapter) Apply(req ApplyRequest) (Status, error) {
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	model := req.Model
	if model == "" {
		model = "deepseek-v4-pro"
	}
	base := ensureV1(req.BaseURL)
	content := `provider = "openai"

[providers.openai]
base_url = ` + tomlString(base) + `
api_key = ` + tomlString(key) + `
model = ` + tomlString(model) + `
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

// --- Grok CLI (~/.grok/config.toml) ---

type grokCliAdapter struct{}

func (a *grokCliAdapter) configPath() string {
	return filepath.Join(homeDir(), ".grok", "config.toml")
}

func (a *grokCliAdapter) Status() Status {
	p := a.configPath()
	if !installed("grok", p) {
		return Status{Installed: false, Message: "Grok CLI is not installed"}
	}
	content := readText(p)
	_, hasCyrene := tomlGetField(content, "model.cyrene", "base_url")
	_, has9R := tomlGetField(content, "model.9router", "base_url")
	hasGW := hasCyrene || has9R
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p}
}

func (a *grokCliAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	content := readText(p)
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	model := req.Model
	if model == "" {
		model = "grok-4"
	}
	base := ensureV1(req.BaseURL)
	content = tomlSetField(content, "model.cyrene", "model", model)
	content = tomlSetField(content, "model.cyrene", "base_url", base)
	content = tomlSetField(content, "model.cyrene", "name", "Cyrene")
	content = tomlSetField(content, "model.cyrene", "api_backend", "chat_completions")
	content = tomlSetField(content, "model.cyrene", "api_key", key)
	// Remove legacy section
	content = tomlDeleteSection(content, "model.9router")
	// Set as default model.
	content = tomlSetField(content, "models", "default", "cyrene")
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *grokCliAdapter) Reset() (Status, error) {
	p := a.configPath()
	content := readText(p)
	if content == "" {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	content = tomlDeleteSection(content, "model.cyrene")
	content = tomlDeleteSection(content, "model.9router")
	if v, _ := tomlGetField(content, "models", "default"); v == "cyrene" || v == "9router" {
		content = tomlSetField(content, "models", "default", "grok-build")
	}
	if err := writeText(p, content); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}
