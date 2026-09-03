package cli

import (
	"path/filepath"
	"runtime"
)

// --- Claude Code (~/.claude/settings.json, env block) ---

type claudeAdapter struct{}

func (a *claudeAdapter) settingsPath() string {
	return filepath.Join(homeDir(), ".claude", "settings.json")
}

func (a *claudeAdapter) Status() Status {
	p := a.settingsPath()
	if !installed("claude", p) {
		return Status{Installed: false, Message: "Claude CLI is not installed"}
	}
	settings := readJSON(p)
	hasGW := false
	if env, ok := settings["env"].(map[string]any); ok {
		if u, ok := env["ANTHROPIC_BASE_URL"].(string); ok && u != "" {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p, Settings: settings}
}

func (a *claudeAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		settings = map[string]any{}
	}
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env["ANTHROPIC_BASE_URL"] = ensureV1(req.BaseURL)
	if req.APIKey != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = req.APIKey
	}
	if req.Model != "" {
		env["ANTHROPIC_MODEL"] = req.Model
	}
	settings["env"] = env
	settings["hasCompletedOnboarding"] = true
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *claudeAdapter) Reset() (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		return Status{Installed: true, Message: "No settings file to reset"}, nil
	}
	if env, ok := settings["env"].(map[string]any); ok {
		for _, k := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL",
			"ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL"} {
			delete(env, k)
		}
		if len(env) == 0 {
			delete(settings, "env")
		}
	}
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- Cline (~/.cline/data/globalState.json + secrets.json) ---

type clineAdapter struct{}

func (a *clineAdapter) dataDir() string { return filepath.Join(homeDir(), ".cline", "data") }
func (a *clineAdapter) globalStatePath() string {
	return filepath.Join(a.dataDir(), "globalState.json")
}
func (a *clineAdapter) secretsPath() string { return filepath.Join(a.dataDir(), "secrets.json") }

func (a *clineAdapter) Status() Status {
	p := a.globalStatePath()
	if !installed("cline", p) {
		return Status{Installed: false, Message: "Cline is not installed"}
	}
	gs := readJSON(p)
	hasGW := false
	if gs != nil {
		act, _ := gs["actModeApiProvider"].(string)
		base, _ := gs["openAiBaseUrl"].(string)
		hasGW = act == "openai" && looksLikeGateway(base)
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p, Settings: gs}
}

func (a *clineAdapter) Apply(req ApplyRequest) (Status, error) {
	gs := readJSON(a.globalStatePath())
	if gs == nil {
		gs = map[string]any{}
	}
	gs["actModeApiProvider"] = "openai"
	gs["planModeApiProvider"] = "openai"
	gs["openAiBaseUrl"] = stripV1(req.BaseURL) // Cline expects base WITHOUT /v1
	gs["openAiModelId"] = req.Model
	gs["planModeOpenAiModelId"] = req.Model
	if err := writeJSON(a.globalStatePath(), gs); err != nil {
		return Status{}, err
	}
	secrets := readJSON(a.secretsPath())
	if secrets == nil {
		secrets = map[string]any{}
	}
	secrets["openAiApiKey"] = req.APIKey
	if err := writeJSON(a.secretsPath(), secrets); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *clineAdapter) Reset() (Status, error) {
	gs := readJSON(a.globalStatePath())
	if gs == nil {
		return Status{Installed: true, Message: "No settings file to reset"}, nil
	}
	if act, _ := gs["actModeApiProvider"].(string); act == "openai" {
		delete(gs, "openAiBaseUrl")
		delete(gs, "openAiModelId")
		delete(gs, "planModeOpenAiModelId")
		gs["actModeApiProvider"] = "cline"
		gs["planModeApiProvider"] = "cline"
	}
	if err := writeJSON(a.globalStatePath(), gs); err != nil {
		return Status{}, err
	}
	secrets := readJSON(a.secretsPath())
	if secrets != nil {
		delete(secrets, "openAiApiKey")
		writeJSON(a.secretsPath(), secrets)
	}
	return a.Status(), nil
}

// --- OpenCode (~/.config/opencode/opencode.json) ---

type opencodeAdapter struct{}

func (a *opencodeAdapter) configPath() string {
	return filepath.Join(homeDir(), ".config", "opencode", "opencode.json")
}

func (a *opencodeAdapter) Status() Status {
	p := a.configPath()
	if !installed("opencode", p) {
		return Status{Installed: false, Message: "OpenCode is not installed"}
	}
	config := readJSON(p)
	hasGW := false
	if config != nil {
		if prov := getNested(config, "provider.cyrene"); prov != nil {
			hasGW = true
		} else if prov := getNested(config, "provider.9router"); prov != nil {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p, Settings: config}
}

func (a *opencodeAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	config := readJSON(p)
	if config == nil {
		config = map[string]any{}
	}
	models := req.Models
	if len(models) == 0 && req.Model != "" {
		models = []string{req.Model}
	}
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	provider, _ := getNested(config, "provider.cyrene").(map[string]any)
	if provider == nil {
		provider = map[string]any{}
	}
	provider["npm"] = "@ai-sdk/openai-compatible"
	provider["options"] = map[string]any{
		"baseURL": ensureV1(req.BaseURL),
		"apiKey":  key,
	}
	modelMap := map[string]any{}
	for _, m := range models {
		modelMap[m] = map[string]any{
			"name":       m,
			"modalities": map[string]any{"input": []any{"text", "image"}, "output": []any{"text"}},
		}
	}
	provider["models"] = modelMap
	setNested(config, "provider.cyrene", provider)
	// Remove legacy 9router provider if present
	deleteNested(config, "provider.9router")

	if len(models) > 0 {
		config["model"] = "cyrene/" + models[0]
	}
	if err := writeJSON(p, config); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *opencodeAdapter) Reset() (Status, error) {
	p := a.configPath()
	config := readJSON(p)
	if config == nil {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	deleteNested(config, "provider.cyrene")
	deleteNested(config, "provider.9router")
	if m, ok := config["model"].(string); ok {
		if (len(m) > 7 && m[:7] == "cyrene/") || (len(m) > 8 && m[:8] == "9router/") {
			delete(config, "model")
		}
	}
	if err := writeJSON(p, config); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- GitHub Copilot (VS Code chatLanguageModels.json) ---

type copilotAdapter struct{}

func (a *copilotAdapter) configPath() string {
	home := homeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Code", "User", "chatLanguageModels.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "chatLanguageModels.json")
	default:
		return filepath.Join(home, ".config", "Code", "User", "chatLanguageModels.json")
	}
}

func (a *copilotAdapter) Status() Status {
	p := a.configPath()
	config := readJSONArray(p)
	hasGW := false
	for _, e := range config {
		if em, ok := e.(map[string]any); ok {
			if name, _ := em["name"].(string); name == "Cyrene" || name == "9Router" {
				hasGW = true
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p}
}

func (a *copilotAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	config := readJSONArray(p)
	models := req.Models
	if len(models) == 0 && req.Model != "" {
		models = []string{req.Model}
	}
	endpoint := req.BaseURL + "/chat/completions#models.ai.azure.com"
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	modelList := make([]any, 0, len(models))
	for _, id := range models {
		modelList = append(modelList, map[string]any{
			"id": id, "name": id, "url": endpoint,
			"toolCalling": true, "vision": false,
			"maxInputTokens": 128000, "maxOutputTokens": 16000,
		})
	}
	entry := map[string]any{
		"name": "Cyrene", "vendor": "azure", "apiKey": key, "models": modelList,
	}
	replaced := false
	for i, e := range config {
		if em, ok := e.(map[string]any); ok {
			if name, _ := em["name"].(string); name == "Cyrene" || name == "9Router" {
				config[i] = entry
				replaced = true
			}
		}
	}
	if !replaced {
		config = append(config, entry)
	}
	if err := writeJSON(p, config); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *copilotAdapter) Reset() (Status, error) {
	p := a.configPath()
	config := readJSONArray(p)
	if config == nil {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	filtered := make([]any, 0, len(config))
	for _, e := range config {
		if em, ok := e.(map[string]any); ok {
			if name, _ := em["name"].(string); name == "Cyrene" || name == "9Router" {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	if err := writeJSON(p, filtered); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- Continue (~/.continue/config.json) ---

type continueAdapter struct{}

func (a *continueAdapter) configPath() string {
	return filepath.Join(homeDir(), ".continue", "config.json")
}

func (a *continueAdapter) Status() Status {
	p := a.configPath()
	if !installed("continue", p) {
		return Status{Installed: false, Message: "Continue extension is not installed"}
	}
	config := readJSON(p)
	hasGW := false
	if config != nil {
		if models, ok := config["models"].([]any); ok {
			for _, m := range models {
				if mm, ok := m.(map[string]any); ok {
					title, _ := mm["title"].(string)
					base, _ := mm["apiBase"].(string)
					if title == "Cyrene Gateway" || title == "9Router" || looksLikeGateway(base) {
						hasGW = true
						break
					}
				}
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, Has9Router: hasGW, ConfigPath: p, Settings: config}
}

func (a *continueAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.configPath()
	config := readJSON(p)
	if config == nil {
		config = map[string]any{}
	}
	models, _ := config["models"].([]any)
	filtered := make([]any, 0, len(models))
	for _, m := range models {
		if mm, ok := m.(map[string]any); ok {
			title, _ := mm["title"].(string)
			if title == "Cyrene Gateway" || title == "9Router" {
				continue
			}
		}
		filtered = append(filtered, m)
	}
	key := req.APIKey
	if key == "" {
		key = "sk-cyrene"
	}
	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	entry := map[string]any{
		"title":    "Cyrene Gateway",
		"provider": "openai",
		"model":    model,
		"apiKey":   key,
		"apiBase":  ensureV1(req.BaseURL),
	}
	filtered = append([]any{entry}, filtered...)
	config["models"] = filtered
	if err := writeJSON(p, config); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *continueAdapter) Reset() (Status, error) {
	p := a.configPath()
	config := readJSON(p)
	if config == nil {
		return Status{Installed: true, Message: "No config file to reset"}, nil
	}
	if models, ok := config["models"].([]any); ok {
		filtered := make([]any, 0, len(models))
		for _, m := range models {
			if mm, ok := m.(map[string]any); ok {
				title, _ := mm["title"].(string)
				if title == "Cyrene Gateway" || title == "9Router" {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		config["models"] = filtered
		if err := writeJSON(p, config); err != nil {
			return Status{}, err
		}
	}
	return a.Status(), nil
}
