package media

func registerEmbeddingProviders() {
	// OpenAI official embeddings
	mergeProvider("openai", "OpenAI", KindEmbedding,
		[]ModelEntry{
			{ID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Kind: KindEmbedding},
			{ID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Kind: KindEmbedding},
			{ID: "text-embedding-ada-002", Name: "Text Embedding Ada 002", Kind: KindEmbedding},
		},
		ProviderConfig{
			Provider:   "openai",
			Kind:       KindEmbedding,
			BaseURL:    "https://api.openai.com/v1/embeddings",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// OpenRouter embeddings
	mergeProvider("openrouter", "OpenRouter", KindEmbedding,
		[]ModelEntry{
			{ID: "openai/text-embedding-3-small", Name: "Text Embedding 3 Small", Kind: KindEmbedding},
			{ID: "openai/text-embedding-3-large", Name: "Text Embedding 3 Large", Kind: KindEmbedding},
			{ID: "cohere/embed-multilingual-v3.0", Name: "Cohere Embed Multilingual v3.0", Kind: KindEmbedding},
		},
		ProviderConfig{
			Provider:   "openrouter",
			Kind:       KindEmbedding,
			BaseURL:    "https://openrouter.ai/api/v1/embeddings",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// Gemini embeddings
	mergeProvider("gemini", "Gemini", KindEmbedding,
		[]ModelEntry{
			{ID: "text-embedding-004", Name: "Google Text Embedding 004", Kind: KindEmbedding},
			{ID: "text-embedding-005", Name: "Google Text Embedding 005", Kind: KindEmbedding},
			{ID: "gemini-embedding-001", Name: "Gemini Embedding 001", Kind: KindEmbedding},
			{ID: "gemini-embedding-2-preview", Name: "Gemini Embedding 2 Preview", Kind: KindEmbedding},
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
