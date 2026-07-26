package media

func registerWebProviders() {
	// Web Fetch providers
	fetchProviders := []struct {
		id, name, baseURL, authHeader string
	}{
		{"firecrawl", "Firecrawl", "https://api.firecrawl.dev/v1/scrape", "bearer"},
		{"jina-reader", "Jina Reader", "https://r.jina.ai", "bearer"},
		{"tavily", "Tavily", "https://api.tavily.com/extract", "bearer"},
		{"exa", "Exa", "https://api.exa.ai/contents", "x-api-key"},
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
		{"serper", "Serper", "https://google.serper.dev/search", "x-api-key"},
		{"searchapi", "SearchAPI", "https://www.searchapi.io/api/v1/search", "bearer"},
		{"youcom", "You.com", "https://api.ydc-index.io/search", "x-api-key"},
		{"linkup", "Linkup", "https://api.linkup.so/v1/search", "bearer"},
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

	// SearXNG (self-hosted, no auth)
	mergeProvider("searxng", "SearXNG", KindWebSearch, nil, ProviderConfig{
		Provider:   "searxng",
		Kind:       KindWebSearch,
		BaseURL:    "",
		AuthType:   "none",
		AuthHeader: "",
		Format:     "searxng",
	})

	// Google PSE
	mergeProvider("google-pse", "Google PSE", KindWebSearch, nil, ProviderConfig{
		Provider:   "google-pse",
		Kind:       KindWebSearch,
		BaseURL:    "https://www.googleapis.com/customsearch/v1",
		AuthType:   "apikey",
		AuthHeader: "key",
		Format:     "google-pse",
	})
}
