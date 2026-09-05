package media

func registerEmbeddingProviders() {
	embeddingProviders := []struct {
		id, name, baseURL string
	}{
		{"openai", "OpenAI", "https://api.openai.com/v1/embeddings"},
	}

	for _, p := range embeddingProviders {
		mergeProvider(p.id, p.name, KindEmbedding, nil, ProviderConfig{
			Provider:   p.id,
			Kind:       KindEmbedding,
			BaseURL:    p.baseURL,
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		})
	}

	// Gemini uses a different format
	mergeProvider("gemini", "Gemini", KindEmbedding,
		[]ModelEntry{
			{ID: "gemini-embedding-2-preview", Name: "Gemini Embedding 2 Preview", Kind: KindEmbedding},
			{ID: "gemini-embedding-001", Name: "Gemini Embedding 001", Kind: KindEmbedding},
			{ID: "text-embedding-005", Name: "Text Embedding 005", Kind: KindEmbedding},
			{ID: "text-embedding-004", Name: "Text Embedding 004", Kind: KindEmbedding},
		},
		ProviderConfig{
			Provider:   "gemini",
			Kind:       KindEmbedding,
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/models",
			AuthType:   "apikey",
			AuthHeader: "key",
			Format:     "gemini",
		},
	)
}
