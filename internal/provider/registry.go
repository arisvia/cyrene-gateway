package provider

import "strings"

// ProviderInfo defines a known provider's configuration
type ProviderInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Alias    string   `json:"alias,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	BaseURL  string   `json:"baseUrl"`
	APIType  string   `json:"apiType"`  // "openai", "anthropic", "gemini"
	AuthType string   `json:"authType"` // "api-key", "oauth"
}

// Registry is the static provider registry
var Registry = map[string]ProviderInfo{
	"openai": {
		ID: "openai", Name: "OpenAI", Alias: "oai",
		Aliases: []string{"openai"},
		BaseURL: "https://api.openai.com/v1", APIType: "openai", AuthType: "api-key",
	},
	"anthropic": {
		ID: "anthropic", Name: "Anthropic", Alias: "claude",
		Aliases: []string{"claude", "anthropic"},
		BaseURL: "https://api.anthropic.com", APIType: "anthropic", AuthType: "api-key",
	},
	"gemini": {
		ID: "gemini", Name: "Google Gemini", Alias: "google",
		Aliases: []string{"google", "gemini"},
		BaseURL: "https://generativelanguage.googleapis.com", APIType: "gemini", AuthType: "api-key",
	},
	"openrouter": {
		ID: "openrouter", Name: "OpenRouter", Alias: "or",
		Aliases: []string{"or", "openrouter"},
		BaseURL: "https://openrouter.ai/api/v1", APIType: "openai", AuthType: "api-key",
	},
	"deepseek": {
		ID: "deepseek", Name: "DeepSeek", Alias: "ds",
		Aliases: []string{"ds", "deepseek"},
		BaseURL: "https://api.deepseek.com/v1", APIType: "openai", AuthType: "api-key",
	},
	"groq": {
		ID: "groq", Name: "Groq", Alias: "groq",
		Aliases: []string{"groq"},
		BaseURL: "https://api.groq.com/openai/v1", APIType: "openai", AuthType: "api-key",
	},
	"mistral": {
		ID: "mistral", Name: "Mistral AI", Alias: "mistral",
		Aliases: []string{"mistral"},
		BaseURL: "https://api.mistral.ai/v1", APIType: "openai", AuthType: "api-key",
	},
	"together": {
		ID: "together", Name: "Together AI", Alias: "together",
		Aliases: []string{"together"},
		BaseURL: "https://api.together.xyz/v1", APIType: "openai", AuthType: "api-key",
	},
	"fireworks": {
		ID: "fireworks", Name: "Fireworks AI", Alias: "fw",
		Aliases: []string{"fw", "fireworks"},
		BaseURL: "https://api.fireworks.ai/inference/v1", APIType: "openai", AuthType: "api-key",
	},
	"siliconflow": {
		ID: "siliconflow", Name: "SiliconFlow", Alias: "sf",
		Aliases: []string{"sf", "siliconflow"},
		BaseURL: "https://api.siliconflow.cn/v1", APIType: "openai", AuthType: "api-key",
	},
	"ollama": {
		ID: "ollama", Name: "Ollama (Local)", Alias: "ollama",
		Aliases: []string{"ollama"},
		BaseURL: "http://localhost:11434/v1", APIType: "openai", AuthType: "none",
	},
	"xai": {
		ID: "xai", Name: "xAI (Grok)", Alias: "xai",
		Aliases: []string{"xai", "grok"},
		BaseURL: "https://api.x.ai/v1", APIType: "openai", AuthType: "api-key",
	},
	"nvidia": {
		ID: "nvidia", Name: "NVIDIA NIM", Alias: "nvidia",
		Aliases: []string{"nvidia", "nim"},
		BaseURL: "https://integrate.api.nvidia.com/v1", APIType: "openai", AuthType: "api-key",
	},
	"azure": {
		ID: "azure", Name: "Azure OpenAI", Alias: "azure",
		Aliases: []string{"azure"},
		BaseURL: "", APIType: "openai", AuthType: "api-key",
	},
	"vertex": {
		ID: "vertex", Name: "Google Vertex AI", Alias: "vertex",
		Aliases: []string{"vertex"},
		BaseURL: "", APIType: "gemini", AuthType: "oauth",
	},
}

// aliasMap is built once for fast lookup
var aliasMap = buildAliasMap()

func buildAliasMap() map[string]string {
	m := make(map[string]string)
	for id, p := range Registry {
		m[id] = id
		if p.Alias != "" {
			m[p.Alias] = id
		}
		for _, a := range p.Aliases {
			m[a] = id
		}
	}
	return m
}

// ResolveProviderAlias resolves a provider alias to its canonical ID
func ResolveProviderAlias(aliasOrID string) string {
	if id, ok := aliasMap[aliasOrID]; ok {
		return id
	}
	return aliasOrID
}

// GetProvider returns provider info by ID
func GetProvider(id string) (ProviderInfo, bool) {
	p, ok := Registry[id]
	return p, ok
}

// modelPrefixProviders maps model name prefixes to providers
var modelPrefixProviders = []struct {
	prefix   string
	provider string
}{
	{"claude-", "anthropic"},
	{"gemini-", "gemini"},
	{"gpt-", "openai"},
	{"o1-", "openai"},
	{"o3-", "openai"},
	{"o4-", "openai"},
	{"deepseek-", "deepseek"},
	{"grok-", "xai"},
	{"llama-", "openrouter"},
	{"mistral-", "mistral"},
}

// InferProviderFromModel infers provider from model name prefix
func InferProviderFromModel(modelName string) string {
	lower := strings.ToLower(modelName)
	for _, mp := range modelPrefixProviders {
		if strings.HasPrefix(lower, mp.prefix) {
			return mp.provider
		}
	}
	return "openai"
}
