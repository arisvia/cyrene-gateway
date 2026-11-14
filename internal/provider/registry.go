package provider

import "strings"

// ProviderInfo defines a known provider's configuration
type ProviderInfo struct {
	// Extra headers sent on every upstream request (e.g. x-opencode-client)
	Headers map[string]string `json:"headers,omitempty"`
	// ModelsHeaders overrides Headers for the model-catalog fetch only. Some
	// providers serve the catalog under a different client identity than the
	// one required for chat: CodeBuddy (Intl) must chat as the IDE product but
	// its /v3/config model list is populated only under the CLI identity, so
	// the catalog read uses these headers while chat keeps the IDE Headers.
	ModelsHeaders map[string]string `json:"modelsHeaders,omitempty"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	BaseURL       string            `json:"baseUrl"`
	APIType       string            `json:"apiType"`  // "openai", "anthropic", "gemini"
	AuthType      string            `json:"authType"` // "api-key", "oauth"
	// Category and auth metadata (Phase 10)
	Category string `json:"category"` // "apikey", "oauth", "freeTier", "custom"
	// Brand/Region grouping (Phase 36): brand siblings (e.g. glm/glm-cn) are
	// rendered as one panel card with a region switcher. IDs stay stable.
	Brand  string `json:"brand,omitempty"`
	Region string `json:"region,omitempty"` // "intl" | "cn"
	// AuthHint is a human hint for the credential the panel should collect
	// (9router authHint), e.g. qoder's PAT source (Phase 36).
	AuthHint string `json:"authHint,omitempty"`
	// ModelsURL / ModelsAuth (Phase 36): explicit live model-catalog endpoint
	// when it cannot be derived from BaseURL. ModelsAuth is "none" (public) or
	// "bearer" (default). Empty ModelsURL = derive via BuildModelsURL.
	ModelsURL  string `json:"modelsUrl,omitempty"`
	ModelsAuth string `json:"modelsAuth,omitempty"`
	// Display metadata
	Color    string `json:"color,omitempty"`
	Website  string `json:"website,omitempty"`
	Icon     string `json:"icon,omitempty"`
	TextIcon string `json:"textIcon,omitempty"`
	// APIKeyURL is the "Get API Key" link surfaced in the panel, ported from
	// 9router registry display.notice.apiKeyUrl (Phase 31).
	APIKeyURL string `json:"apiKeyUrl,omitempty"`
	// OAuth configuration
	DeviceCodeURL string `json:"deviceCodeUrl,omitempty"`
	TokenURL      string `json:"tokenUrl,omitempty"`
	AuthorizeURL  string `json:"authorizeUrl,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	// ClientSecret is a credential, not panel configuration: excluded from JSON
	// so /api/registry cannot leak it. The webui reads no such field, and OAuth
	// runs server-side, so nothing needs it across the API boundary.
	ClientSecret string `json:"-"`
	LoginURL     string `json:"loginUrl,omitempty"` // browser-based device login page (qoder)
	// Auth-mode-specific overrides (9router#2881): when a connection uses
	// api-key auth on a provider whose primary transport is OAuth, route to
	// a different base URL / API type.
	ApiKeyBaseURL string `json:"apiKeyBaseUrl,omitempty"`
	ApiKeyAPIType string `json:"apiKeyApiType,omitempty"`
	OAuthBaseURL  string `json:"oauthBaseUrl,omitempty"`
	OAuthAPIType  string `json:"oauthApiType,omitempty"`
	// Transport config (Phase 30): explicit upstream adaptation ported from
	// 9router registry transport blocks. When set, these override the
	// format-derived defaults in ResolveTransport.
	URLSuffix  string `json:"urlSuffix,omitempty"`  // appended to base URL (e.g. "?beta=true")
	AuthHeader string `json:"authHeader,omitempty"` // header carrying the token (e.g. "x-api-key")
	AuthScheme string `json:"authScheme,omitempty"` // "bearer" | "raw" | "query"
	// ValidateURL is the endpoint used for connection testing (9router transport.validateUrl).
	ValidateURL string `json:"validateUrl,omitempty"`
	// ThinkingFormat indicates how reasoning tokens are surfaced ("openai" = reasoning_content field).
	ThinkingFormat string   `json:"thinkingFormat,omitempty"`
	AuthModes      []string `json:"authModes,omitempty"` // supported auth modes
	AuthHooks      []string `json:"authHooks,omitempty"` // provider header overlays (e.g. "kimiHeaders")
	Priority       int      `json:"priority,omitempty"`  // lower = higher priority
	Hidden         bool     `json:"hidden,omitempty"`
	HasFree        bool     `json:"hasFree,omitempty"`
	// ForceStream forces streaming even when the client requests non-stream (9router transport.forceStream).
	ForceStream bool `json:"forceStream,omitempty"`
	// CampaignKind selects the upstream promotional-activity adapter used by
	// the per-account claim endpoint ("qoder" campaigns API, "codebuddy" daily
	// check-in). Empty = provider has no known claimable activity.
	CampaignKind string `json:"campaignKind,omitempty"`
	// Models lists static/curated model entries for providers without a dynamic /models endpoint.
	Models []ModelRef `json:"models,omitempty"`
}

// EffectiveBaseURL returns the base URL to use for a connection, considering
// auth-mode-specific overrides. If the connection uses an API key and the
// provider defines ApiKeyBaseURL, that takes precedence over the default.
func (p ProviderInfo) EffectiveBaseURL(connAuthType string, hasAPIKey bool) (baseURL, apiType string) {
	baseURL = p.BaseURL
	apiType = p.APIType
	if connAuthType == "oauth" && p.OAuthBaseURL != "" {
		baseURL = p.OAuthBaseURL
		if p.OAuthAPIType != "" {
			apiType = p.OAuthAPIType
		}
		return baseURL, apiType
	}
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

// GetProvider returns provider info by id. There is exactly one spelling per
// provider, so an unknown key is a miss rather than an alias to resolve.
func GetProvider(id string) (ProviderInfo, bool) {
	p, ok := Registry[id]
	return p, ok
}

// RegistryByCategory groups providers by category with counts
type RegistryByCategory struct {
	Category  string         `json:"category"`
	Providers []ProviderInfo `json:"providers"`
	Count     int            `json:"count"`
}

// GetRegistryByCategory returns providers grouped by category
func GetRegistryByCategory() []RegistryByCategory {
	catMap := make(map[string][]ProviderInfo)
	for _, p := range Registry {
		catMap[p.Category] = append(catMap[p.Category], p)
	}

	// Fixed category order
	order := []string{"apikey", "oauth", "freeTier", "custom"}
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
	{"kimi-", "kimi"},
	{"glm-", "glm"},
	{"minimax-", "minimax"},
}

// InferProviderFromModel is the last-resort naming rule, consulted only after
// ResolveModel has failed to find the model in any connected provider's synced
// catalog: it maps the vendor-visible id prefix onto its provider. An id this rule
// does not recognise is reported as unknown ("") rather than guessed as "openai" —
// that guess used to send unrecognised models to whatever OpenAI credential existed,
// and to a confusing "no active credentials for provider: openai" when none did.
func InferProviderFromModel(modelName string) string {
	lower := strings.ToLower(modelName)
	for _, mp := range modelPrefixProviders {
		if strings.HasPrefix(lower, mp.prefix) {
			return mp.provider
		}
	}
	return ""
}
