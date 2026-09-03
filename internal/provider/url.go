package provider

import "strings"

// Known endpoint suffixes that indicate a BaseURL is already a full endpoint.
var chatEndpointSuffixes = []string{
	"/chat/completions",
	"/v1/messages",
	"/messages",
	"/responses",
	"/generate",
	"/agent_chat_generation",
}

// IsFullEndpointURL returns true if the URL already points to a specific
// API endpoint (rather than a base URL that needs a path appended).
func IsFullEndpointURL(baseURL string) bool {
	lower := strings.ToLower(strings.TrimRight(baseURL, "/"))
	for _, suffix := range chatEndpointSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// BuildChatURL constructs the chat completions endpoint URL from a base URL.
// If the base URL is already a full endpoint (ends with /chat/completions etc.),
// it is returned as-is. Otherwise the appropriate path is appended based on
// the provider's API type.
func BuildChatURL(baseURL, apiType string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return ""
	}
	if IsFullEndpointURL(base) {
		return base
	}
	switch apiType {
	case "anthropic":
		return base + "/v1/messages"
	default:
		return base + "/chat/completions"
	}
}
// BuildResponsesURL constructs the OpenAI Responses API endpoint URL from a base URL.
func BuildResponsesURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return ""
	}
	if IsFullEndpointURL(base) {
		if strings.HasSuffix(strings.ToLower(base), "/responses") {
			return base
		}
		base = StripEndpointPath(base)
	}
	return base + "/responses"
}

// BuildModelsURL constructs the models list endpoint URL from a base URL.
// Strips known endpoint paths and appends /models.
func BuildModelsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return ""
	}
	lower := strings.ToLower(base)
	// Strip endpoint suffixes to get the API base
	for _, suffix := range chatEndpointSuffixes {
		if strings.HasSuffix(lower, suffix) {
			base = base[:len(base)-len(suffix)]
			break
		}
	}
	base = strings.TrimRight(base, "/")
	// Strip trailing /v1 for providers that store it (e.g. .../v1 -> .../v1/models)
	return base + "/models"
}

// StripEndpointPath removes known endpoint suffixes from a URL, returning
// the API base (e.g. https://api.openai.com/v1).
func StripEndpointPath(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	lower := strings.ToLower(base)
	for _, suffix := range chatEndpointSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimRight(base[:len(base)-len(suffix)], "/")
		}
	}
	return base
}

// BuildGeminiURL constructs a Gemini generateContent URL. The registry stores
// the base as e.g. https://generativelanguage.googleapis.com/v1beta/models —
// the model and verb are appended directly.
func BuildGeminiURL(baseURL, modelName string, stream bool) string {
	base := strings.TrimRight(baseURL, "/")
	// If the base doesn't end with /models, append the standard path
	if !strings.HasSuffix(strings.ToLower(base), "/models") {
		base += "/v1beta/models"
	}
	if stream {
		return base + "/" + modelName + ":streamGenerateContent?alt=sse"
	}
	return base + "/" + modelName + ":generateContent"
}
