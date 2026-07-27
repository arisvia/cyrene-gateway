package cli

// Tool is the static definition of a supported CLI coding tool.
type Tool struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
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

// Registry is the ordered list of supported CLI tools. Order determines the
// card grid layout on the CLI Tools page.
var Registry = []Tool{
	{
		ID:          "claude",
		Name:        "Claude Code",
		Icon:        "/providers/claude.png",
		Color:       "#D97757",
		Description: "Anthropic Claude Code CLI",
		ConfigType:  "env",
		DefaultModels: []ToolModel{
			{ID: "cc/claude-opus-4-8", Name: "Claude Opus", Alias: "opus"},
			{ID: "cc/claude-sonnet-5", Name: "Claude Sonnet", Alias: "sonnet"},
			{ID: "cc/claude-haiku-4-5-20251001", Name: "Claude Haiku", Alias: "haiku"},
		},
	},
	{
		ID:          "codex",
		Name:        "OpenAI Codex CLI",
		Icon:        "/providers/codex.png",
		Color:       "#10A37F",
		Description: "OpenAI Codex CLI / App",
		ConfigType:  "custom",
	},
	{
		ID:          "opencode",
		Name:        "OpenCode",
		Icon:        "/providers/opencode.png",
		Color:       "#E87040",
		Description: "OpenCode AI terminal assistant",
		ConfigType:  "custom",
	},
	{
		ID:          "cline",
		Name:        "Cline",
		Icon:        "/providers/cline.png",
		Color:       "#00D1B2",
		Description: "Cline AI coding assistant",
		ConfigType:  "custom",
	},
	{
		ID:          "copilot",
		Name:        "GitHub Copilot",
		Icon:        "/providers/copilot.png",
		Color:       "#1F6FEB",
		Description: "GitHub Copilot (VS Code chat models)",
		ConfigType:  "custom",
	},
	{
		ID:          "kilo",
		Name:        "Kilo Code",
		Icon:        "/providers/kilocode.png",
		Color:       "#FF6B6B",
		Description: "Kilo Code AI assistant",
		ConfigType:  "custom",
	},
	{
		ID:          "openclaw",
		Name:        "Open Claw",
		Icon:        "/providers/openclaw.png",
		Color:       "#FF6B35",
		Description: "Open Claw AI assistant",
		ConfigType:  "custom",
	},
	{
		ID:          "hermes",
		Name:        "Hermes Agent",
		Icon:        "/providers/hermes.png",
		Color:       "#8B5CF6",
		Description: "Nous Research self-improving AI agent",
		ConfigType:  "custom",
	},
	{
		ID:          "droid",
		Name:        "Factory Droid",
		Icon:        "/providers/droid.png",
		Color:       "#00D4FF",
		Description: "Factory Droid AI assistant",
		ConfigType:  "custom",
	},
	{
		ID:          "grok-build",
		Name:        "Grok Build",
		Icon:        "/providers/grok-cli.png",
		Color:       "#1DA1F2",
		Description: "xAI Grok Build TUI coding agent",
		ConfigType:  "custom",
		DocsURL:     "https://x.ai/cli",
		Notes: []ToolNote{
			{Type: "info", Text: "Grok Build uses ~/.grok/config.toml. Cyrene writes a [model.9router] custom model and sets it as the default."},
			{Type: "info", Text: "After Apply, run grok (or /model 9router) to use the routed model. Switch back anytime with /model grok-build."},
		},
	},
	{
		ID:          "deepseek-tui",
		Name:        "DeepSeek TUI",
		Icon:        "/providers/deepseek-tui.png",
		Color:       "#4D6BFE",
		Description: "DeepSeek Terminal Coding Agent (Rust TUI)",
		ConfigType:  "custom",
		DocsURL:     "https://github.com/DeepSeek-TUI/DeepSeek-TUI",
		DefaultModels: []ToolModel{
			{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
			{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash"},
			{ID: "deepseek-chat", Name: "DeepSeek V3 Chat"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "DeepSeek TUI uses ~/.deepseek/config.toml. Cyrene sets the provider to 'openai' mode with your base_url, api_key, and model."},
		},
	},
	{
		ID:          "jcode",
		Name:        "jcode",
		Icon:        "/providers/jcode.png",
		Color:       "#FF6B35",
		Description: "High-performance Rust-based coding agent harness",
		ConfigType:  "custom",
		DocsURL:     "https://github.com/1jehuang/jcode",
		DefaultModels: []ToolModel{
			{ID: "cc/claude-opus-4-7", Name: "Claude Opus 4.7", Alias: "opus"},
			{ID: "cc/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Alias: "sonnet"},
			{ID: "gemini/gemini-3.1-pro", Name: "Gemini 3.1 Pro", Alias: "gemini"},
		},
		Notes: []ToolNote{
			{Type: "info", Text: "Configure Cyrene as an OpenAI-compatible provider to route all jcode requests through the gateway."},
			{Type: "warning", Text: "Requires jcode installed. Use: jcode --provider-profile 9router"},
		},
	},
	{
		ID:          "cursor",
		Name:        "Cursor",
		Icon:        "/providers/cursor.png",
		Color:       "#000000",
		Description: "Cursor AI code editor (manual setup)",
		ConfigType:  "guide",
		Notes: []ToolNote{
			{Type: "warning", Text: "Requires a Cursor Pro account to use this feature."},
			{Type: "warning", Text: "Cursor routes requests through its own server, so a local endpoint is not supported. Enable Tunnel or a public endpoint first."},
		},
		GuideSteps: []GuideStep{
			{Step: 1, Title: "Open Settings", Desc: "Go to Settings → Models"},
			{Step: 2, Title: "Enable OpenAI API", Desc: "Enable the \"OpenAI API key\" option"},
			{Step: 3, Title: "Base URL", Value: "{{baseUrl}}"},
			{Step: 4, Title: "API Key", Type: "apiKeySelector"},
			{Step: 5, Title: "Add Custom Model", Desc: "Click \"View All Model\" → \"Add Custom Model\""},
			{Step: 6, Title: "Select Model", Type: "modelSelector"},
		},
	},
	{
		ID:          "antigravity",
		Name:        "Antigravity",
		Icon:        "/providers/antigravity.png",
		Color:       "#4285F4",
		Description: "Google Antigravity IDE (requires MITM proxy)",
		ConfigType:  "mitm",
		Notes: []ToolNote{
			{Type: "info", Text: "Antigravity is intercepted via the MITM proxy. Enable MITM (local mode only) to route its traffic through the gateway."},
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
