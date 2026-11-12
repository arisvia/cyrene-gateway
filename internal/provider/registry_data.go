package provider

// registry_data.go holds the curated chat-provider registry.
// Originally generated from 9router open-sse/providers/registry/; pruned to
// the curated KEEP set in Phase 36 (registry IDs stay stable — no DB/alias
// migration; display names are vendor-official). Media providers
// (internal/media) are a separate registry and were out of scope.

// ClaudeCodeVersion is the Claude Code CLI release this gateway impersonates.
// The official client gates its own OAuth surfaces on a current version, so a
// bump here moves every claude/claude-oauth request at once.
const (
	ClaudeCodeVersion   = "2.1.280"
	ClaudeCodeUserAgent = "claude-cli/" + ClaudeCodeVersion + " (external, sdk-cli)"
)

// claudeCodeHeaders provides default headers for Anthropic-compatible providers supporting thinking beta
var claudeCodeHeaders = map[string]string{
	"anthropic-version": AnthropicAPIVersion,
	"Anthropic-Beta":    "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-2024-07-31",
}

// CodeBuddy client identity. An intl account is enrolled, refreshed and
// quota-checked as the IDE product, so the chat gateway must present the same
// identity: one token driven by both a CLI and an IDE fingerprint is exactly
// what the channel check treats as a relayed client. CN is CLI end to end.
const (
	CodebuddyVersion = "2.179.2"
	CodebuddyIDEUA   = "IDE/" + CodebuddyVersion + " CodeBuddy/" + CodebuddyVersion
	CodebuddyCLIUA   = "CLI/" + CodebuddyVersion + " CodeBuddy/" + CodebuddyVersion
)

// claudeOAuthHeaders provides headers required by Anthropic OAuth (/v1/messages with Bearer user tokens)
var claudeOAuthHeaders = map[string]string{
	"Anthropic-Version":                         AnthropicAPIVersion,
	"Anthropic-Beta":                            "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
	"User-Agent":                                ClaudeCodeUserAgent,
	"X-App":                                     "cli",
}

func init() {
	Registry = make(map[string]ProviderInfo, 40)

	Registry["alicode"] = ProviderInfo{
		ID: "alicode", Name: "Alibaba (Intl)",

		BaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 10,
		Brand: "Alibaba", Region: "intl",
		Color:     "#FF6A00",
		Website:   "https://www.alibabacloud.com/product/coding",
		Icon:      "cloud",
		TextIcon:  "ALi",
		APIKeyURL: "https://www.alibabacloud.com/product/coding",
	}
	Registry["alicode-cn"] = ProviderInfo{
		ID: "alicode-cn", Name: "Alibaba (CN)",

		BaseURL: "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 20,
		Brand: "Alibaba", Region: "cn",
		Color:     "#FF6A00",
		Website:   "https://bailian.console.aliyun.com",
		Icon:      "cloud",
		TextIcon:  "ALi",
		APIKeyURL: "https://bailian.console.aliyun.com/?apiKey=1",
	}
	Registry["anthropic"] = ProviderInfo{
		ID: "anthropic", Name: "Anthropic",
		BaseURL: "https://api.anthropic.com/v1/messages",
		APIType: "anthropic", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 30,
		Color:      "#D97757",
		Website:    "https://console.anthropic.com",
		Icon:       "smart_toy",
		TextIcon:   "AN",
		APIKeyURL:  "https://console.anthropic.com/settings/keys",
		Headers:    claudeCodeHeaders,
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	}
	Registry["antigravity"] = ProviderInfo{
		ID: "antigravity", Name: "Antigravity",

		BaseURL: "",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 20,
		Color:        "#F59E0B",
		Website:      "https://antigravity.google",
		Icon:         "rocket_launch",
		TokenURL:     "https://oauth2.googleapis.com/token",
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
		ClientSecret: "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",
	}
	Registry["cerebras"] = ProviderInfo{
		ID: "cerebras", Name: "Cerebras",
		BaseURL: "https://api.cerebras.ai/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 60,
		Color:     "#FF4F00",
		Website:   "https://www.cerebras.ai",
		Icon:      "memory",
		TextIcon:  "CB",
		APIKeyURL: "https://cloud.cerebras.ai/platform",
	}
	Registry["claude"] = ProviderInfo{
		ID: "claude", Name: "Claude Code",

		BaseURL: "https://api.anthropic.com/v1/messages",
		APIType: "anthropic", AuthType: "oauth",
		// Phase 36 T4: dual-auth (OAuth preferred, API key fallback —
		// same selection pattern as xai/kimi).
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 10,
		Color:        "#D97757",
		Website:      "https://claude.ai",
		Icon:         "smart_toy",
		APIKeyURL:    "https://console.anthropic.com/settings/keys",
		TokenURL:     "https://api.anthropic.com/v1/oauth/token",
		AuthorizeURL: "https://claude.ai/oauth/authorize",
		ClientID:     "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		URLSuffix:    "?beta=true",
		Headers:      claudeOAuthHeaders,
	}
	Registry["codebuddy-cn"] = ProviderInfo{
		ID: "codebuddy-cn", Name: "CodeBuddy (CN)",

		BaseURL: "https://copilot.tencent.com/v2/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 90,
		Brand: "CodeBuddy", Region: "cn",
		Color:         "#006EFF",
		Website:       "https://copilot.tencent.com",
		Icon:          "smart_toy",
		DeviceCodeURL: "https://copilot.tencent.com/v2/plugin/auth/state",
		TokenURL:      "https://copilot.tencent.com/v2/plugin/auth/token",
		Headers: map[string]string{
			"User-Agent":          CodebuddyCLIUA,
			"X-Product":           "SaaS",
			"X-IDE-Type":          "CLI",
			"X-IDE-Name":          "CLI",
			"x-requested-with":    "XMLHttpRequest",
			"x-codebuddy-request": "1",
		},
		ForceStream:    true,
		ThinkingFormat: "openai",
		ModelsURL:      "https://copilot.tencent.com/v3/config",
		CampaignKind:   "codebuddy",
	}
	Registry["codebuddy"] = ProviderInfo{
		ID: "codebuddy", Name: "CodeBuddy (Intl)",

		BaseURL: "https://www.codebuddy.ai/v2/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 90,
		Brand: "CodeBuddy", Region: "intl",
		Color:         "#006EFF",
		Website:       "https://www.codebuddy.ai",
		Icon:          "smart_toy",
		TextIcon:      "CB",
		DeviceCodeURL: "https://www.codebuddy.ai/v2/plugin/auth/state",
		TokenURL:      "https://www.codebuddy.ai/v2/plugin/auth/token",
		Headers: map[string]string{
			"User-Agent":          CodebuddyIDEUA,
			"X-Product":           "SaaS",
			"X-IDE-Type":          "IDE",
			"X-IDE-Name":          "IDE",
			"X-Domain":            "www.codebuddy.ai",
			"x-requested-with":    "XMLHttpRequest",
			"x-codebuddy-request": "1",
		},
		// The model catalog (/v3/config) is only populated under the CLI
		// identity; the IDE identity returns just productFeatures with an
		// empty model list. The official IDE client reads its model dropdown
		// the same way — CLI for the catalog, IDE for chat — so this is not
		// the relayed-client signal (that is a chat-fingerprint mismatch).
		ModelsHeaders: map[string]string{
			"User-Agent":          CodebuddyCLIUA,
			"X-Product":           "SaaS",
			"X-IDE-Type":          "CLI",
			"X-IDE-Name":          "CLI",
			"X-Domain":            "www.codebuddy.ai",
			"x-requested-with":    "XMLHttpRequest",
			"x-codebuddy-request": "1",
		},
		ForceStream:    true,
		ThinkingFormat: "openai",
		ModelsURL:      "https://www.codebuddy.ai/v3/config",
		CampaignKind:   "codebuddy",
	}
	Registry["cline"] = ProviderInfo{
		ID: "cline", Name: "Cline",

		BaseURL: "https://api.cline.bot/api/v1/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 80,
		Color:    "#5B9BD5",
		Website:  "https://cline.bot",
		Icon:     "smart_toy",
		TextIcon: "CL",
		// Cline's gateway wraps non-stream bodies in {"success":true,"data":{...}};
		// the non-stream reader already unwraps that envelope.
		AuthorizeURL: "https://api.cline.bot/api/v1/auth/authorize",
		TokenURL:     "https://api.cline.bot/api/v1/auth/token",
		AuthHooks:    []string{"clineHeaders"},
		Headers: map[string]string{
			"HTTP-Referer": ClineReferer,
			"X-Title":      ClineClientTitle,
		},
		// Live catalog is OpenRouter-shaped and public.
		ModelsURL:  "https://api.cline.bot/api/v1/ai/cline/models",
		ModelsAuth: "none",
	}
	Registry["kilocode"] = ProviderInfo{
		ID: "kilocode", Name: "Kilo Code",

		BaseURL: "https://api.kilo.ai/api/openrouter/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 70,
		Color:    "#FF6B35",
		Website:  "https://kilocode.ai",
		Icon:     "code",
		TextIcon: "KC",
		// Non-RFC device flow: POST /codes returns {code,verificationUrl}, then
		// GET /codes/{code} answers 202 pending / 410 expired / 200 {status,token}.
		DeviceCodeURL: "https://api.kilo.ai/api/device-auth/codes",
		TokenURL:      "https://api.kilo.ai/api/device-auth/codes",
		AuthHooks:     []string{"kilocodeOrg"},
		// Kilo proxies the OpenRouter catalog (public, OpenRouter-shaped).
		ModelsURL:  "https://api.kilo.ai/api/gateway/models",
		ModelsAuth: "none",
	}
	Registry["cursor"] = ProviderInfo{
		ID: "cursor", Name: "Cursor",

		BaseURL: "https://api2.cursor.sh",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 50,
		Color:   "#00D4AA",
		Website: "https://cursor.com",
		Icon:    "edit_note",
	}
	Registry["deepseek"] = ProviderInfo{
		ID: "deepseek", Name: "DeepSeek",

		BaseURL: "https://api.deepseek.com/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 110,
		Color:     "#4D6BFE",
		Website:   "https://deepseek.com",
		Icon:      "bolt",
		TextIcon:  "DS",
		APIKeyURL: "https://platform.deepseek.com/api_keys",
	}
	Registry["gemini"] = ProviderInfo{
		ID: "gemini", Name: "Gemini",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/models",
		APIType: "gemini", AuthType: "api-key",
		Category: "freeTier", AuthModes: []string{"api-key"}, Priority: 50,
		Color:        "#4285F4",
		Website:      "https://ai.google.dev",
		Icon:         "diamond",
		TextIcon:     "GE",
		HasFree:      true,
		APIKeyURL:    "https://aistudio.google.com/app/apikey",
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		// x-goog-api-key header (raw), not the ?key= query param.
		AuthHeader: "x-goog-api-key",
		AuthScheme: "raw",
		// Live model catalog: the base IS the models list (auth = ?key=).
		ModelsURL:  "https://generativelanguage.googleapis.com/v1beta/models",
		ModelsAuth: "query",
	}
	Registry["github"] = ProviderInfo{
		ID: "github", Name: "GitHub Copilot",

		BaseURL: "https://api.githubcopilot.com/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 40,
		Color:         "#333333",
		Website:       "https://github.com/features/copilot",
		Icon:          "code",
		DeviceCodeURL: "https://github.com/login/device/code",
		TokenURL:      "https://github.com/login/oauth/access_token",
		AuthorizeURL:  "https://github.com/login/oauth/authorize",
		ClientID:      "Iv1.b507a08c87ecfe98",
		Headers: map[string]string{
			"copilot-integration-id":              "vscode-chat",
			"editor-version":                      copilotVSCodeVersion,
			"editor-plugin-version":               copilotChatVersion,
			"user-agent":                          copilotUserAgent,
			"openai-intent":                       "conversation-panel",
			"x-github-api-version":                copilotAPIVersion,
			"x-vscode-user-agent-library-version": "electron-fetch",
			"X-Initiator":                         "user",
		},
	}
	Registry["glm-cn"] = ProviderInfo{
		ID: "glm-cn", Name: "GLM (CN)",

		BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 130,
		Brand: "GLM", Region: "cn",
		Color:     "#DC2626",
		Website:   "https://open.bigmodel.cn",
		Icon:      "code",
		TextIcon:  "GC",
		APIKeyURL: "https://open.bigmodel.cn/usercenter/apikeys",
	}
	Registry["glm"] = ProviderInfo{
		ID: "glm", Name: "GLM Coding (Intl)",
		BaseURL: "https://api.z.ai/api/anthropic/v1/messages",
		APIType: "anthropic", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 140,
		Brand: "GLM", Region: "intl",
		Color:     "#2563EB",
		Website:   "https://open.bigmodel.cn",
		Icon:      "code",
		TextIcon:  "GL",
		APIKeyURL: "https://open.bigmodel.cn/usercenter/apikeys",
		// 9router glm.js transport: claude /coding endpoint, ?beta=true suffix,
		// x-api-key raw auth + CLAUDE_API_HEADERS.
		URLSuffix:  "?beta=true",
		Headers:    claudeCodeHeaders,
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		ModelsURL:  "https://api.z.ai/api/paas/v4/models",
		ModelsAuth: "bearer",
	}
	Registry["grok-cli"] = ProviderInfo{
		ID: "grok-cli", Name: "Grok Build",

		BaseURL:   GrokCLIBaseURL,
		ModelsURL: GrokCLIBaseURL + "/models",
		APIType:   "responses", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth"}, Priority: 275,
		Color:         "#1DA1F2",
		Website:       "https://x.ai",
		Icon:          "auto_awesome",
		TextIcon:      "GC",
		DeviceCodeURL: "https://auth.x.ai/oauth2/device/code",
		TokenURL:      "https://auth.x.ai/oauth2/token",
		ClientID:      "b1a00492-073a-47ea-816f-4c329264a828",
		ForceStream:   true,
		AuthHooks:     []string{"grokCliHeaders"},
		Headers: map[string]string{
			"User-Agent": GrokCLIUserAgent,
		},
	}
	Registry["groq"] = ProviderInfo{
		ID: "groq", Name: "Groq",
		BaseURL: "https://api.groq.com/openai/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 60,
		Color:       "#F55036",
		Website:     "https://groq.com",
		Icon:        "speed",
		TextIcon:    "GQ",
		HasFree:     true,
		APIKeyURL:   "https://console.groq.com/keys",
		ValidateURL: "https://api.groq.com/openai/v1/models",
	}
	Registry["kimi"] = ProviderInfo{
		ID: "kimi", Name: "Kimi Code",

		BaseURL: "https://api.kimi.com/coding/v1/messages",
		APIType: "anthropic", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 170,
		Color:         "#1E3A8A",
		Website:       "https://kimi.moonshot.cn",
		Icon:          "psychology",
		TextIcon:      "KM",
		APIKeyURL:     "https://platform.moonshot.ai/console/api-keys",
		DeviceCodeURL: "https://auth.kimi.com/api/oauth/device_authorization",
		TokenURL:      "https://auth.kimi.com/api/oauth/token",
		ClientID:      "17e5f671-d194-4dfb-9706-5516cb48c098",
		// 9router#2881: platform API keys (platform.kimi.ai) use the general
		// Moonshot OpenAI-compatible endpoint, not the /coding/ Anthropic one.
		ApiKeyBaseURL: "https://api.moonshot.cn/v1",
		ApiKeyAPIType: "openai",
		// Primary (OAuth) transport: claude /coding endpoint, x-api-key raw auth
		// + X-Msh-* fingerprint headers (9router kimi.js transport block).
		URLSuffix:  "?beta=true",
		Headers:    claudeCodeHeaders,
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		AuthHooks:  []string{"kimiHeaders"},
	}
	Registry["mistral"] = ProviderInfo{
		ID: "mistral", Name: "Mistral",

		BaseURL: "https://api.mistral.ai/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 50,
		Color:     "#FF7000",
		Website:   "https://mistral.ai",
		Icon:      "air",
		TextIcon:  "MI",
		APIKeyURL: "https://console.mistral.ai/api-keys",
	}
	Registry["perplexity"] = ProviderInfo{
		ID: "perplexity", Name: "Perplexity",

		BaseURL: "https://api.perplexity.ai/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 60,
		Color:     "#20808D",
		Website:   "https://www.perplexity.ai",
		Icon:      "search",
		TextIcon:  "PX",
		APIKeyURL: "https://www.perplexity.ai/settings/api",
		// Sonar's catalog is not exposed over /models; keep the two documented
		// engines rather than a derive that would 404.
		Models: []ModelRef{
			{ID: "sonar-pro", Name: "Sonar Pro"},
			{ID: "sonar", Name: "Sonar"},
		},
	}
	Registry["cohere"] = ProviderInfo{
		ID: "cohere", Name: "Cohere",

		BaseURL: "https://api.cohere.ai/compatibility/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 60,
		Color:     "#39594D",
		Website:   "https://cohere.com",
		Icon:      "hub",
		TextIcon:  "CO",
		APIKeyURL: "https://dashboard.cohere.com/api-keys",
		// Native /v1/models is Cohere-shaped; /compatibility/v1 mirrors it in
		// the OpenAI envelope the rest of this gateway parses.
		ModelsURL:  "https://api.cohere.ai/compatibility/v1/models",
		ModelsAuth: "bearer",
	}
	Registry["fireworks"] = ProviderInfo{
		ID: "fireworks", Name: "Fireworks AI",

		BaseURL: "https://api.fireworks.ai/inference/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 70,
		Color:     "#7B2EF2",
		Website:   "https://fireworks.ai",
		Icon:      "local_fire_department",
		TextIcon:  "FW",
		APIKeyURL: "https://fireworks.ai/account/api-keys",
	}
	Registry["together"] = ProviderInfo{
		ID: "together", Name: "Together AI",

		BaseURL: "https://api.together.xyz/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 70,
		Color:     "#0F6FFF",
		Website:   "https://www.together.ai",
		Icon:      "group_work",
		TextIcon:  "TG",
		APIKeyURL: "https://api.together.xyz/settings/api-keys",
	}
	Registry["deepinfra"] = ProviderInfo{
		ID: "deepinfra", Name: "DeepInfra",

		BaseURL: "https://api.deepinfra.com/v1/openai/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 80,
		Color:     "#2563EB",
		Website:   "https://deepinfra.com",
		Icon:      "cloud",
		TextIcon:  "DI",
		APIKeyURL: "https://deepinfra.com/dash/api_keys",
	}
	Registry["xiaomi"] = ProviderInfo{
		ID: "xiaomi", Name: "Xiaomi MiMo (Intl)",

		BaseURL: "https://api.xiaomimimo.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 75,
		Brand: "Xiaomi", Region: "intl",
		Color:     "#FF6900",
		Website:   "https://mimo.mi.com",
		Icon:      "bolt",
		TextIcon:  "XM",
		APIKeyURL: "https://platform.xiaomimimo.com/#/console/api-keys",
	}
	// Token Plan keys (tp-/ttp-) are minted per region and only work on that
	// region's host: token-plan-cn is the CN subscription endpoint, distinct
	// from the pay-as-you-go sk- host above.
	Registry["xiaomi-cn"] = ProviderInfo{
		ID: "xiaomi-cn", Name: "Xiaomi MiMo (CN)",

		BaseURL: "https://token-plan-cn.xiaomimimo.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 75,
		Brand: "Xiaomi", Region: "cn",
		Color:     "#FF6900",
		Website:   "https://mimo.mi.com",
		Icon:      "bolt",
		TextIcon:  "XM",
		APIKeyURL: "https://platform.xiaomimimo.com/#/console/plan-manage",
	}
	Registry["minimax-cn"] = ProviderInfo{
		ID: "minimax-cn", Name: "MiniMax (CN)",

		BaseURL: "https://api.minimaxi.com/anthropic/v1/messages",
		APIType: "anthropic", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 190,
		Brand: "MiniMax", Region: "cn",
		Color:     "#DC2626",
		Website:   "https://www.minimaxi.com",
		Icon:      "memory",
		TextIcon:  "MC",
		APIKeyURL: "https://platform.minimaxi.com/user-center/basic-information/interface-key",
		// CN mirror of the OpenAI-compatible catalog (see minimax above).
		ModelsURL:  "https://api.minimaxi.com/v1/models",
		ModelsAuth: "bearer",
		// 9router minimax-cn.js transport: claude endpoint, ?beta=true,
		// x-api-key raw auth + CLAUDE_API_HEADERS.
		URLSuffix:  "?beta=true",
		Headers:    claudeCodeHeaders,
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	}
	Registry["minimax"] = ProviderInfo{
		ID: "minimax", Name: "MiniMax Coding (Intl)",

		BaseURL: "https://api.minimax.io/anthropic/v1/messages",
		APIType: "anthropic", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 90,
		Brand: "MiniMax", Region: "intl",
		Color:     "#7C3AED",
		Website:   "https://www.minimaxi.com",
		Icon:      "memory",
		TextIcon:  "MM",
		APIKeyURL: "https://platform.minimaxi.com/user-center/basic-information/interface-key",
		// The anthropic /v1/messages base has no catalog; MiniMax exposes
		// /v1/models on the OpenAI-compatible host.
		ModelsURL:  "https://api.minimax.io/v1/models",
		ModelsAuth: "bearer",
		// 9router minimax.js transport: claude endpoint, ?beta=true,
		// x-api-key raw auth + CLAUDE_API_HEADERS.
		URLSuffix:  "?beta=true",
		Headers:    claudeCodeHeaders,
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	}
	Registry["nvidia"] = ProviderInfo{
		ID: "nvidia", Name: "NVIDIA NIM",

		BaseURL: "https://integrate.api.nvidia.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "freeTier", AuthModes: []string{"api-key"}, Priority: 20,
		Color:     "#76B900",
		Website:   "https://developer.nvidia.com/nim",
		Icon:      "developer_board",
		TextIcon:  "NV",
		HasFree:   true,
		APIKeyURL: "https://build.nvidia.com/settings/api-keys",
	}
	Registry["openai"] = ProviderInfo{
		ID: "openai", Name: "OpenAI",

		BaseURL: "https://api.openai.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"oauth", "api-key"}, Priority: 30,
		OAuthBaseURL: "https://chatgpt.com/backend-api/codex/responses",
		OAuthAPIType: "responses",
		Color:        "#10A37F",
		Website:      "https://platform.openai.com",
		Icon:         "auto_awesome",
		TextIcon:     "OA",
		APIKeyURL:    "https://platform.openai.com/api-keys",
		TokenURL:     "https://auth.openai.com/oauth/token",
		AuthorizeURL: "https://auth.openai.com/oauth/authorize",
		ClientID:     "app_EMoamEEZ73f0CkXaXp7hrann",
		ForceStream:  true,
	}
	Registry["opencode"] = ProviderInfo{
		ID: "opencode", Name: "OpenCode",

		Brand: "OpenCode", Region: "zen",
		BaseURL: "https://opencode.ai/zen/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 40,
		Color:     "#E87040",
		Website:   "https://opencode.ai",
		Icon:      "terminal",
		TextIcon:  "OC",
		APIKeyURL: "https://opencode.ai/auth",
		AuthHooks: []string{"opencodeHeaders"},
		Headers: map[string]string{
			"x-opencode-client": "desktop",
		},
		ModelsURL:  "https://opencode.ai/zen/v1/models",
		ModelsAuth: "none",
	}
	Registry["opencode-go"] = ProviderInfo{
		ID: "opencode-go", Name: "OpenCode Go",

		Brand: "OpenCode", Region: "go",
		BaseURL: "https://opencode.ai/zen/go/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 40,
		Color:     "#E87040",
		Website:   "https://opencode.ai",
		Icon:      "terminal",
		TextIcon:  "OCG",
		APIKeyURL: "https://opencode.ai/auth",
		AuthHooks: []string{"opencodeHeaders"},
		Headers: map[string]string{
			"x-opencode-client": "desktop",
		},
		ModelsURL:  "https://opencode.ai/zen/go/v1/models",
		ModelsAuth: "none",
	}
	Registry["openrouter"] = ProviderInfo{
		ID: "openrouter", Name: "OpenRouter",

		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "freeTier", AuthModes: []string{"api-key"}, Priority: 10,
		Color:     "#F97316",
		Website:   "https://openrouter.ai",
		Icon:      "router",
		TextIcon:  "OR",
		HasFree:   true,
		APIKeyURL: "https://openrouter.ai/settings/keys",
	}
	Registry["qoder"] = ProviderInfo{
		ID: "qoder", Name: "Qoder (Intl)",

		BaseURL: "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 30,
		Brand: "Qoder", Region: "intl",
		Color:         "#EC4899",
		Website:       "https://qoder.com",
		Icon:          "water_drop",
		HasFree:       true,
		AuthHint:      "Personal Access Token (pt-...) from https://qoder.com/account/integrations",
		DeviceCodeURL: "https://openapi.qoder.sh/api/v1/deviceToken/poll",
		LoginURL:      "https://qoder.com/device/selectAccounts",
		CampaignKind:  "qoder",
	}
	Registry["qoder-cn"] = ProviderInfo{
		ID: "qoder-cn", Name: "Qoder (CN)",

		BaseURL: "https://gateway.qoder.com.cn/algo/api/v2/service/pro/sse/agent_chat_generation",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 30,
		Brand: "Qoder", Region: "cn",
		Color:         "#EC4899",
		Website:       "https://qoder.com.cn",
		Icon:          "water_drop",
		HasFree:       true,
		AuthHint:      "Personal Access Token (pt-...) from https://qoder.com.cn/account/integrations",
		DeviceCodeURL: "https://openapi.qoder.com.cn/api/v1/deviceToken/poll",
		LoginURL:      "https://qoder.com.cn/device/selectAccounts",
		CampaignKind:  "qoder",
	}
	Registry["tencent"] = ProviderInfo{
		ID: "tencent", Name: "Tencent Hunyuan",

		BaseURL: "https://api.hunyuan.cloud.tencent.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "apikey", AuthModes: []string{"api-key"}, Priority: 130,
		Color:       "#0052D9",
		Website:     "https://cloud.tencent.com/product/hunyuan",
		Icon:        "cloud",
		TextIcon:    "HY",
		ValidateURL: "https://api.hunyuan.cloud.tencent.com/v1/models",
		APIKeyURL:   "https://console.cloud.tencent.com/hunyuan/api-key",
	}
	Registry["vertex"] = ProviderInfo{
		ID: "vertex", Name: "Vertex AI",

		BaseURL: "https://aiplatform.googleapis.com",
		APIType: "openai", AuthType: "api-key",
		Category: "freeTier", AuthModes: []string{"api-key"}, Priority: 40,
		Color:     "#4285F4",
		Website:   "https://cloud.google.com/vertex-ai",
		Icon:      "cloud",
		TextIcon:  "VX",
		APIKeyURL: "https://console.cloud.google.com/apis/credentials",
		// Vertex's catalog is project+location scoped, so there is no single
		// public /models URL; keep a curated list instead of a dead derive.
		Models: []ModelRef{
			{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview"},
			{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash Lite Preview"},
			{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview"},
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash"},
		},
	}
	Registry["xai"] = ProviderInfo{
		ID: "xai", Name: "xAI (Grok)",
		BaseURL: "https://api.x.ai/v1/chat/completions",
		APIType: "openai", AuthType: "oauth",
		Category: "oauth", AuthModes: []string{"oauth", "api-key"}, Priority: 280,
		Color:        "#1DA1F2",
		Website:      "https://x.ai",
		Icon:         "auto_awesome",
		TextIcon:     "XA",
		APIKeyURL:    "https://console.x.ai",
		AuthorizeURL: "https://auth.x.ai/oauth2/authorize",
		TokenURL:     "https://auth.x.ai/oauth2/token",
		ClientID:     "b1a00492-073a-47ea-816f-4c329264a828",
	}
	Registry["custom-openai"] = ProviderInfo{
		ID: "custom-openai", Name: "OpenAI Compatible",
		BaseURL: "https://api.openai.com/v1/chat/completions",
		APIType: "openai", AuthType: "api-key",
		Category: "custom", AuthModes: []string{"api-key"}, Priority: 50,
		Color:    "#10A37F",
		Website:  "https://platform.openai.com",
		Icon:     "tune",
		TextIcon: "OA",
		AuthHint: "输入支持 OpenAI 规范的标准接口 Base URL（例如 https://my-llm.com/v1）与对应 API Key",
	}
	Registry["custom-anthropic"] = ProviderInfo{
		ID: "custom-anthropic", Name: "Anthropic Compatible",
		BaseURL: "https://api.anthropic.com/v1/messages",
		APIType: "anthropic", AuthType: "api-key",
		Category: "custom", AuthModes: []string{"api-key"}, Priority: 50,
		Color:    "#D97706",
		Website:  "https://anthropic.com",
		Icon:     "tune",
		TextIcon: "AN",
		AuthHint: "输入支持 Anthropic /v1/messages 规范的标准接口 Base URL 与对应 API Key",
	}

}
