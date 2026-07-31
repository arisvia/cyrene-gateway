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
	AuthType string   `json:"authType"` // "api-key", "oauth", "cookie", "none"

	// Category and auth metadata (Phase 10)
	Category  string   `json:"category"`            // "apikey", "oauth", "freeTier", "free", "webCookie"
	AuthModes []string `json:"authModes,omitempty"` // supported auth modes
	Priority  int      `json:"priority,omitempty"`  // lower = higher priority

	// Display metadata
	Color    string `json:"color,omitempty"`
	Website  string `json:"website,omitempty"`
	Icon     string `json:"icon,omitempty"`
	TextIcon string `json:"textIcon,omitempty"`

	// APIKeyURL is the "Get API Key" link surfaced in the panel, ported from
	// 9router registry display.notice.apiKeyUrl (Phase 31).
	APIKeyURL string `json:"apiKeyUrl,omitempty"`

	// Flags
	Hidden  bool `json:"hidden,omitempty"`
	HasFree bool `json:"hasFree,omitempty"`
	NoAuth  bool `json:"noAuth,omitempty"`

	// OAuth configuration
	DeviceCodeURL string `json:"deviceCodeUrl,omitempty"`
	TokenURL      string `json:"tokenUrl,omitempty"`
	AuthorizeURL  string `json:"authorizeUrl,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	LoginURL      string `json:"loginUrl,omitempty"` // browser-based device login page (qoder)

	// Auth-mode-specific overrides (9router#2881): when a connection uses
	// api-key auth on a provider whose primary transport is OAuth, route to
	// a different base URL / API type.
	ApiKeyBaseURL string `json:"apiKeyBaseUrl,omitempty"`
	ApiKeyAPIType string `json:"apiKeyApiType,omitempty"`

	// Extra headers sent on every upstream request (e.g. x-opencode-client)
	Headers map[string]string `json:"headers,omitempty"`

	// Transport config (Phase 30): explicit upstream adaptation ported from
	// 9router registry transport blocks. When set, these override the
	// format-derived defaults in ResolveTransport.
	URLSuffix  string   `json:"urlSuffix,omitempty"`  // appended to base URL (e.g. "?beta=true")
	AuthHeader string   `json:"authHeader,omitempty"` // header carrying the token (e.g. "x-api-key")
	AuthScheme string   `json:"authScheme,omitempty"` // "bearer" | "raw" | "query"
	AuthHooks  []string `json:"authHooks,omitempty"`  // provider header overlays (e.g. "kimiHeaders")

	// ValidateURL is the endpoint used for connection testing (9router transport.validateUrl).
	ValidateURL string `json:"validateUrl,omitempty"`
	// ThinkingFormat indicates how reasoning tokens are surfaced ("openai" = reasoning_content field).
	ThinkingFormat string `json:"thinkingFormat,omitempty"`
	// ForceStream forces streaming even when the client requests non-stream (9router transport.forceStream).
	ForceStream bool `json:"forceStream,omitempty"`
}

// EffectiveBaseURL returns the base URL to use for a connection, considering
// auth-mode-specific overrides. If the connection uses an API key and the
// provider defines ApiKeyBaseURL, that takes precedence over the default.
func (p ProviderInfo) EffectiveBaseURL(connAuthType string, hasAPIKey bool) (baseURL, apiType string) {
	baseURL = p.BaseURL
	apiType = p.APIType
	if hasAPIKey && p.ApiKeyBaseURL != "" {
		baseURL = p.ApiKeyBaseURL
		if p.ApiKeyAPIType != "" {
			apiType = p.ApiKeyAPIType
		}
	}
	return baseURL, apiType
}

// Registry is the static provider registry, populated in init() by registry_data.go
var Registry map[string]ProviderInfo

// aliasMap is built once for fast lookup
var aliasMap map[string]string

// legacyAliases provides backward-compatible aliases from the original registry
var legacyAliases = map[string]string{
	"oai":    "openai",
	"claude": "anthropic",
	"google": "gemini",
	"or":     "openrouter",
	"ds":     "deepseek",
	"sf":     "siliconflow",
	"fw":     "fireworks",
	"nim":    "nvidia",
	"grok":   "xai",
}

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
	// Apply legacy aliases (do not override existing entries)
	for alias, id := range legacyAliases {
		if _, exists := m[alias]; !exists {
			m[alias] = id
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

// RegistryByCategory groups providers by category with counts
type RegistryByCategory struct {
	Category  string         `json:"category"`
	Count     int            `json:"count"`
	Providers []ProviderInfo `json:"providers"`
}

// GetRegistryByCategory returns providers grouped by category
func GetRegistryByCategory() []RegistryByCategory {
	catMap := make(map[string][]ProviderInfo)
	for _, p := range Registry {
		catMap[p.Category] = append(catMap[p.Category], p)
	}

	// Fixed category order
	order := []string{"apikey", "oauth", "freeTier", "free", "webCookie"}
	result := make([]RegistryByCategory, 0, len(order))
	for _, cat := range order {
		if providers, ok := catMap[cat]; ok {
			result = append(result, RegistryByCategory{
				Category:  cat,
				Count:     len(providers),
				Providers: providers,
			})
		}
	}
	return result
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
	{"qwen", "qwen"},
	{"kimi-", "kimi"},
	{"glm-", "glm"},
	{"minimax-", "minimax"},
	{"jina-", "jina-ai"},
	{"accounts/fireworks/", "fireworks"},
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
