package cli

// Tool is the static definition of a supported CLI coding tool.
type Tool struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Category      string      `json:"category"` // cli | extension | ide
	Icon          string      `json:"icon"`
	Color         string      `json:"color"`
	Description   string      `json:"description"`
	ConfigType    string      `json:"configType"` // env | custom | guide | mitm
	DocsURL       string      `json:"docsUrl,omitempty"`
	DefaultModels []ToolModel `json:"defaultModels,omitempty"`
	Notes         []ToolNote  `json:"notes,omitempty"`
	GuideSteps    []GuideStep `json:"guideSteps,omitempty"`
	CodeBlock     *CodeBlock  `json:"codeBlock,omitempty"`
}

// ToolModel is a suggested model entry for a tool's model selector.
type ToolModel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
}

// ToolNote is an informational banner shown on the tool detail page.
type ToolNote struct {
	Type string `json:"type"` // info | warning | error
	Text string `json:"text"`
}

// GuideStep is a single step in a manual (guide) setup flow.
type GuideStep struct {
	Step  int    `json:"step"`
	Title string `json:"title"`
	Desc  string `json:"desc,omitempty"`
	Value string `json:"value,omitempty"`
	Type  string `json:"type,omitempty"` // apiKeySelector | modelSelector
}

// CodeBlock is a copyable snippet shown at the end of a guide flow.
type CodeBlock struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// Registry is the curated list of 2026 mainstream CLI coding tools, IDE extensions and AI editors.
var Registry = []Tool{
	{
		ID:          "claude",
		Name:        "Claude Code",
		Category:    "cli",
		Icon:        "/providers/claude.svg",
		Color:       "#D97757",
		Description: "Anthropic official Claude Code CLI terminal agent",
		ConfigType:  "env",
		DocsURL:     "https://docs.anthropic.com/en/docs/agents-and-tools/claude-code",
		DefaultModels: []ToolModel{
			{ID: "cc/claude-opus-4-8", Name: "Claude Opus 4.8", Alias: "opus"},
			{ID: "cc/claude-sonnet-5", Name: "Claude Sonnet 5", Alias: "sonnet"},
			{ID: "cc/claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Alias: "haiku"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Cyrene writes ANTHROPIC_BASE_URL into ~/.claude/settings.json, routing all Claude Code sessions through the gateway."},
		},
	},
	{
		ID:          "cursor",
		Name:        "Cursor",
		Category:    "ide",
		Icon:        "/providers/cursor.svg",
		Color:       "#000000",
		Description: "The leading AI code editor with agentic pair programming",
		ConfigType:  "guide",
		DocsURL:     "https://cursor.com",
		Notes: []ToolNote{
			{Type: "info", Text: "Cursor supports custom OpenAI-compatible endpoints directly in Settings → Models."},
			{Type: "warning", Text: "If running Cursor locally without Tunnel, ensure your gateway address is reachable from your host."},
		},
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "Open Cursor Settings → Models"},
			{Step: 2, Title: "Enable OpenAI API", Desc: "Toggle on the OpenAI API key option"},
			{Step: 3, Title: "Base URL", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Add Custom Model", Desc: "Under Models list, click Add Model and input your target gateway model ID"},
			{Step: 6, Title: "Select Model", Type: "modelSelector"},
		},
	},
	{
		ID:          "opencode",
		Name:        "OpenCode",
		Category:    "cli",
		Icon:        "/providers/opencode.svg",
		Color:       "#E87040",
		Description: "Open-source terminal AI coding assistant & agent",
		ConfigType:  "custom",
		DocsURL:     "https://opencode.ai",
		DefaultModels: []ToolModel{
			{ID: "opencode/claude-sonnet-4-20250514", Name: "OpenCode Claude Sonnet 4"},
			{ID: "opencode/kimi-k2.7-code", Name: "OpenCode Kimi K2.7 Code"},
			{ID: "opencode/deepseek-v4-flash", Name: "OpenCode DeepSeek V4 Flash"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "OpenCode reads ~/.config/opencode/opencode.json. Cyrene registers a custom openai-compatible provider named 'cyrene'."},
		},
	},
	{
		ID:          "codex",
		Name:        "OpenAI Codex CLI",
		Category:    "cli",
		Icon:        "/providers/codex.svg",
		Color:       "#10A37F",
		Description: "OpenAI developer CLI coding agent",
		ConfigType:  "custom",
		DocsURL:     "https://developers.openai.com",
		DefaultModels: []ToolModel{
			{ID: "gpt-5.4", Name: "GPT-5.4"},
			{ID: "o4-high", Name: "o4 Reasoning High"},
			{ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Codex uses ~/.codex/config.toml. Cyrene writes a [model_providers.cyrene] entry and sets it as active."},
		},
	},
	{
		ID:          "aider",
		Name:        "Aider",
		Category:    "cli",
		Icon:        "/providers/aider.svg",
		Color:       "#10A37F",
		Description: "AI pair programming in your terminal",
		ConfigType:  "custom",
		DocsURL:     "https://aider.chat",
		DefaultModels: []ToolModel{
			{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3.2"},
			{ID: "claude-sonnet-4-6", Name: "Claude 3.7 Sonnet"},
			{ID: "gpt-5-mini", Name: "GPT-5 Mini"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Aider uses ~/.aider.conf.yml. Cyrene writes openai-api-base, openai-api-key, and default model."},
		},
	},
	{
		ID:          "cline",
		Name:        "Cline",
		Category:    "extension",
		Icon:        "/providers/cline.svg",
		Color:       "#00D1B2",
		Description: "Autonomous coding agent extension for VS Code",
		ConfigType:  "custom",
		DocsURL:     "https://github.com/cline/cline",
		DefaultModels: []ToolModel{
			{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
			{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
			{ID: "gpt-5-codex", Name: "GPT-5 Codex"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Cline stores settings in ~/.cline/data/. Cyrene configures the OpenAI-compatible provider automatically."},
		},
	},
	{
		ID:          "roo-code",
		Name:        "Roo Code",
		Category:    "extension",
		Icon:        "/providers/roo.svg",
		Color:       "#FF5722",
		Description: "Community-driven multi-model autonomous coding agent for VS Code",
		ConfigType:  "guide",
		DocsURL:     "https://roocode.com",
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "In VS Code, click Roo Code icon → Settings gear"},
			{Step: 2, Title: "Select Provider", Desc: "Choose 'OpenAI Compatible' as the API Provider"},
			{Step: 3, Title: "Base URL", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Model ID", Type: "modelSelector"},
		},
	},
	{
		ID:          "continue",
		Name:        "Continue",
		Category:    "extension",
		Icon:        "/providers/continue.svg",
		Color:       "#D97706",
		Description: "Open-source AI code assistant extension for VS Code & JetBrains",
		ConfigType:  "custom",
		DocsURL:     "https://continue.dev",
		DefaultModels: []ToolModel{
			{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
			{ID: "gpt-5-mini", Name: "GPT-5 Mini"},
			{ID: "deepseek-chat", Name: "DeepSeek V3.2"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Continue stores config in ~/.continue/config.json. Cyrene automatically adds a 'Cyrene Gateway' model provider entry."},
		},
	},
	{
		ID:          "copilot",
		Name:        "GitHub Copilot",
		Category:    "extension",
		Icon:        "/providers/copilot.svg",
		Color:       "#1F6FEB",
		Description: "GitHub Copilot custom language models in VS Code",
		ConfigType:  "custom",
		DocsURL:     "https://github.com/features/copilot",
		Notes: []ToolNote{
			{Type: "info", Text: "Writes custom language models into VS Code chatLanguageModels.json under the Cyrene vendor."},
		},
	},
	{
		ID:          "windsurf",
		Name:        "Windsurf",
		Category:    "ide",
		Icon:        "/providers/windsurf.svg",
		Color:       "#0EA5E9",
		Description: "Codeium's agentic AI-powered IDE with Cascade flows",
		ConfigType:  "guide",
		DocsURL:     "https://codeium.com/windsurf",
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "Open Windsurf Settings → Advanced / AI Providers"},
			{Step: 2, Title: "Custom OpenAI Provider", Desc: "Select Custom OpenAI Compatible Provider"},
			{Step: 3, Title: "API Base", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Set Model", Type: "modelSelector"},
		},
	},
	{
		ID:          "trae",
		Name:        "Trae",
		Category:    "ide",
		Icon:        "/providers/trae.svg",
		Color:       "#3B82F6",
		Description: "ByteDance's adaptive AI native IDE with Builder & Chat",
		ConfigType:  "guide",
		DocsURL:     "https://trae.ai",
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "Click Trae Settings → AI Models & Providers"},
			{Step: 2, Title: "Add Custom Provider", Desc: "Select OpenAI Compatible"},
			{Step: 3, Title: "Endpoint URL", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Model Name", Type: "modelSelector"},
		},
	},
	{
		ID:          "dsh",
		Name:        "DeepSeek Harness (dsh)",
		Category:    "cli",
		Icon:        "/providers/deepseek.svg",
		Color:       "#4D6BFE",
		Description: "DeepSeek official multi-agent coding harness CLI",
		ConfigType:  "custom",
		DocsURL:     "https://github.com/deepseek-ai/deepseek-harness",
		DefaultModels: []ToolModel{
			{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
			{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash"},
			{ID: "deepseek-chat", Name: "DeepSeek V3.2 Chat"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "DeepSeek Harness uses ~/.dsh/config.yaml. Cyrene sets the provider to OpenAI-compatible mode routing through the gateway."},
		},
	},
	{
		ID:          "grok-cli",
		Name:        "Grok CLI",
		Category:    "cli",
		Icon:        "/providers/grok-cli.svg",
		Color:       "#1DA1F2",
		Description: "xAI Grok developer TUI & coding CLI",
		ConfigType:  "custom",
		DocsURL:     "https://x.ai/cli",
		DefaultModels: []ToolModel{
			{ID: "grok-4", Name: "Grok 4"},
			{ID: "grok-3-mini", Name: "Grok 3 Mini"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Grok uses ~/.grok/config.toml. Cyrene registers a [model.cyrene] custom provider and activates it."},
		},
	},
	{
		ID:          "qoder",
		Name:        "Qoder",
		Category:    "extension",
		Icon:        "/providers/qoder.svg",
		Color:       "#8B5CF6",
		Description: "Enterprise-grade intelligent programming assistant",
		ConfigType:  "guide",
		DocsURL:     "https://qoder.sh",
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "Open Qoder extension settings"},
			{Step: 2, Title: "Custom Gateway", Desc: "Set custom API endpoint to Cyrene Gateway"},
			{Step: 3, Title: "Base URL", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Model Selection", Type: "modelSelector"},
		},
	},
	{
		ID:          "antigravity",
		Name:        "Google Antigravity",
		Category:    "ide",
		Icon:        "/providers/antigravity.svg",
		Color:       "#4285F4",
		Description: "Google Antigravity AI IDE (intercepted via MITM proxy)",
		ConfigType:  "mitm",
		Notes: []ToolNote{
			{Type: "info", Text: "Antigravity is intercepted via the gateway's MITM proxy. Enable MITM in local mode to transparently route its sessions."},
		},
	},
}

// GetTool returns the tool definition for the given id, or nil if unknown.
func GetTool(id string) *Tool {
	for i := range Registry {
		if Registry[i].ID == id {
			return &Registry[i]
		}
	}
	return nil
}
