package cli

import (
	"path/filepath"
	"runtime"
	"strconv"
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
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p, Settings: settings}
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
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p, Settings: gs}
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
			if name, _ := em["name"].(string); name == "9Router" {
				hasGW = true
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
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
		key = "sk_9router"
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
		"name": "9Router", "vendor": "azure", "apiKey": key, "models": modelList,
	}
	replaced := false
	for i, e := range config {
		if em, ok := e.(map[string]any); ok {
			if name, _ := em["name"].(string); name == "9Router" {
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
			if name, _ := em["name"].(string); name == "9Router" {
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

// --- Kilo Code (~/.local/share/kilo/auth.json) ---

type kiloAdapter struct{}

func (a *kiloAdapter) authPath() string {
	return filepath.Join(homeDir(), ".local", "share", "kilo", "auth.json")
}

func (a *kiloAdapter) Status() Status {
	p := a.authPath()
	if !installed("kilo", p) {
		return Status{Installed: false, Message: "Kilo Code is not installed"}
	}
	auth := readJSON(p)
	hasGW := false
	for _, key := range []string{"openai-compatible", "9router"} {
		if entry, ok := auth[key].(map[string]any); ok {
			base, _ := entry["baseUrl"].(string)
			if base == "" {
				base, _ = entry["baseURL"].(string)
			}
			if looksLikeGateway(base) {
				hasGW = true
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p}
}

func (a *kiloAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.authPath()
	auth := readJSON(p)
	if auth == nil {
		auth = map[string]any{}
	}
	auth["openai-compatible"] = map[string]any{
		"type":    "api-key",
		"apiKey":  req.APIKey,
		"baseUrl": ensureV1(req.BaseURL),
		"model":   req.Model,
	}
	if err := writeJSON(p, auth); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *kiloAdapter) Reset() (Status, error) {
	p := a.authPath()
	auth := readJSON(p)
	if auth == nil {
		return Status{Installed: true, Message: "No settings file to reset"}, nil
	}
	delete(auth, "openai-compatible")
	delete(auth, "9router")
	if err := writeJSON(p, auth); err != nil {
		return Status{}, err
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
		if prov, ok := getNested(config, "provider.9router").(map[string]any); ok && prov != nil {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p, Settings: config}
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
		key = "sk_9router"
	}
	provider, _ := getNested(config, "provider.9router").(map[string]any)
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
	setNested(config, "provider.9router", provider)
	if len(models) > 0 {
		config["model"] = "9router/" + models[0]
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
	deleteNested(config, "provider.9router")
	if m, ok := config["model"].(string); ok && len(m) > 8 && m[:8] == "9router/" {
		delete(config, "model")
	}
	if err := writeJSON(p, config); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- Factory Droid (~/.factory/settings.json, customModels array) ---

type droidAdapter struct{}

func (a *droidAdapter) settingsPath() string {
	return filepath.Join(homeDir(), ".factory", "settings.json")
}

func (a *droidAdapter) Status() Status {
	p := a.settingsPath()
	if !installed("droid", p) {
		return Status{Installed: false, Message: "Factory Droid is not installed"}
	}
	settings := readJSON(p)
	hasGW := false
	if cm, ok := settings["customModels"].([]any); ok {
		for _, e := range cm {
			if em, ok := e.(map[string]any); ok {
				if id, _ := em["id"].(string); len(id) >= 13 && id[:13] == "custom:9Router" {
					hasGW = true
				}
			}
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p, Settings: settings}
}

func (a *droidAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		settings = map[string]any{}
	}
	models := req.Models
	if len(models) == 0 && req.Model != "" {
		models = []string{req.Model}
	}
	// Remove existing 9Router entries.
	existing, _ := settings["customModels"].([]any)
	filtered := make([]any, 0, len(existing))
	for _, e := range existing {
		if em, ok := e.(map[string]any); ok {
			if id, _ := em["id"].(string); len(id) >= 13 && id[:13] == "custom:9Router" {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	key := req.APIKey
	if key == "" {
		key = "your_api_key"
	}
	base := ensureV1(req.BaseURL)
	for i, m := range models {
		filtered = append(filtered, map[string]any{
			"model": m, "id": "custom:9Router-" + strconv.Itoa(i), "index": i,
			"baseUrl": base, "apiKey": key, "displayName": m,
			"maxOutputTokens": 131072, "noImageSupport": false, "provider": "openai",
		})
	}
	settings["customModels"] = filtered
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *droidAdapter) Reset() (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		return Status{Installed: true, Message: "No settings file to reset"}, nil
	}
	if cm, ok := settings["customModels"].([]any); ok {
		filtered := make([]any, 0, len(cm))
		for _, e := range cm {
			if em, ok := e.(map[string]any); ok {
				if id, _ := em["id"].(string); len(id) >= 13 && id[:13] == "custom:9Router" {
					continue
				}
			}
			filtered = append(filtered, e)
		}
		if len(filtered) == 0 {
			delete(settings, "customModels")
		} else {
			settings["customModels"] = filtered
		}
	}
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

// --- Open Claw (~/.openclaw/openclaw.json) ---

type openclawAdapter struct{}

func (a *openclawAdapter) settingsPath() string {
	return filepath.Join(homeDir(), ".openclaw", "openclaw.json")
}

func (a *openclawAdapter) Status() Status {
	p := a.settingsPath()
	if !installed("", p) {
		return Status{Installed: false, Message: "Open Claw is not installed"}
	}
	settings := readJSON(p)
	hasGW := false
	if settings != nil {
		if prov, ok := getNested(settings, "models.providers.9router").(map[string]any); ok && prov != nil {
			hasGW = true
		}
	}
	return Status{Installed: true, HasGateway: hasGW, ConfigPath: p, Settings: settings}
}

func (a *openclawAdapter) Apply(req ApplyRequest) (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		settings = map[string]any{}
	}
	key := req.APIKey
	if key == "" {
		key = "your_api_key"
	}
	models := req.Models
	if len(models) == 0 && req.Model != "" {
		models = []string{req.Model}
	}
	base := ensureV1(req.BaseURL)

	// models.providers.9router
	modelList := make([]any, 0, len(models))
	for _, m := range models {
		modelList = append(modelList, map[string]any{"id": m, "name": m})
	}
	setNested(settings, "models.providers.9router", map[string]any{
		"baseUrl": base, "apiKey": key, "api": "openai-completions", "models": modelList,
	})

	// agents.defaults.model.primary = 9router/<model>
	if len(models) > 0 {
		setNested(settings, "agents.defaults.model.primary", "9router/"+models[0])
		allow := map[string]any{}
		for _, m := range models {
			allow["9router/"+m] = map[string]any{}
		}
		setNested(settings, "agents.defaults.models", allow)
	}
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}

func (a *openclawAdapter) Reset() (Status, error) {
	p := a.settingsPath()
	settings := readJSON(p)
	if settings == nil {
		return Status{Installed: true, Message: "No settings file to reset"}, nil
	}
	deleteNested(settings, "models.providers.9router")
	if err := writeJSON(p, settings); err != nil {
		return Status{}, err
	}
	return a.Status(), nil
}
