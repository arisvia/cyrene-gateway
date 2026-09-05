package media

import "github.com/arisvia/cyrene-gateway/internal/provider"

func registerWebProviders() {
	// Web Fetch providers
	fetchProviders := []struct {
		id, name, baseURL, authHeader string
	}{
		{"firecrawl", "Firecrawl", "https://api.firecrawl.dev/v1/scrape", "bearer"},
		{"tavily", "Tavily", "https://api.tavily.com/extract", "bearer"},
	}

	for _, p := range fetchProviders {
		mergeProvider(p.id, p.name, KindWebFetch, nil, ProviderConfig{
			Provider:   p.id,
			Kind:       KindWebFetch,
			BaseURL:    p.baseURL,
			AuthType:   "apikey",
			AuthHeader: p.authHeader,
			Format:     p.id,
		})
	}

	// Web Search providers
	searchProviders := []struct {
		id, name, baseURL, authHeader string
	}{
		{"brave-search", "Brave Search", "https://api.search.brave.com/res/v1/web/search", "x-subscription-token"},
		{"tavily", "Tavily", "https://api.tavily.com/search", "bearer"},
		{"exa", "Exa", "https://api.exa.ai/search", "x-api-key"},
	}

	for _, p := range searchProviders {
		mergeProvider(p.id, p.name, KindWebSearch, nil, ProviderConfig{
			Provider:   p.id,
			Kind:       KindWebSearch,
			BaseURL:    p.baseURL,
			AuthType:   "apikey",
			AuthHeader: p.authHeader,
			Format:     p.id,
		})
	}

	// Antigravity (Google Code Assist search grounding via gemini-2.5-flash)
	mergeProvider("antigravity", "Google Antigravity", KindWebSearch, nil, ProviderConfig{
		Provider:   "antigravity",
		BaseURL:    provider.AntigravityBaseURL,
		AuthType:   "oauth",
		AuthHeader: "bearer",
		Format:     "antigravity",
	})
}
