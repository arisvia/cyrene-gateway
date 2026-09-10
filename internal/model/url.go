package model

import "strings"

// ChatEndpointSuffixes lists known endpoint paths that indicate a URL
// is pointing to an inference endpoint rather than the API base or models endpoint.
var ChatEndpointSuffixes = []string{
	"/chat/completions",
	"/completions",
	"/messages",
	"/responses",
	"/generate",
	"/agent_chat_generation",
}

// DeriveModelsURL constructs the models list endpoint URL from a base URL.
// It strips known endpoint paths (e.g. /chat/completions, /messages)
// and appends /models.
func DeriveModelsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return ""
	}
	lower := strings.ToLower(base)
	for _, suffix := range ChatEndpointSuffixes {
		if strings.HasSuffix(lower, suffix) {
			base = base[:len(base)-len(suffix)]
			break
		}
	}
	base = strings.TrimRight(base, "/")
	return base + "/models"
}
