package media

func registerEmbeddingProviders() {
	embeddingProviders := []struct {
		id, name, baseURL string
	}{
		{"openai", "OpenAI", "https://api.openai.com/v1/embeddings"},
		{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1/embeddings"},
		{"mistral", "Mistral", "https://api.mistral.ai/v1/embeddings"},
		{"voyage-ai", "Voyage AI", "https://api.voyageai.com/v1/embeddings"},
		{"fireworks", "Fireworks", "https://api.fireworks.ai/inference/v1/embeddings"},
		{"together", "Together", "https://api.together.xyz/v1/embeddings"},
		{"nebius", "Nebius", "https://api.studio.nebius.ai/v1/embeddings"},
		{"nvidia", "NVIDIA NIM", "https://integrate.api.nvidia.com/v1/embeddings"},
		{"jina-ai", "Jina AI", "https://api.jina.ai/v1/embeddings"},
		{"vercel-ai-gateway", "Vercel AI Gateway", "https://ai-gateway.vercel.sh/v1/embeddings"},
		{"github", "GitHub Models", "https://models.inference.ai.azure.com/embeddings"},
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
