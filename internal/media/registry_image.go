package media

import "github.com/arisvia/cyrene-gateway/internal/provider"

func registerImageProviders() {
	openaiCompat := []struct {
		id, name, baseURL string
	}{
		{"openai", "OpenAI", "https://api.openai.com/v1/images/generations"},
		{"minimax", "MiniMax", "https://api.minimax.chat/v1/images/generations"},
	}

	for _, p := range openaiCompat {
		mergeProvider(p.id, p.name, KindImage, nil, ProviderConfig{
			Provider:   p.id,
			Kind:       KindImage,
			BaseURL:    p.baseURL,
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		})
	}

	// Gemini image generation
	mergeProvider("gemini", "Gemini", KindImage,
		[]ModelEntry{
			{ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro Image", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "gemini",
			Kind:       KindImage,
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/models",
			AuthType:   "apikey",
			AuthHeader: "key",
			Format:     "gemini",
		},
	)

	// Antigravity (Google Code Assist) image generation via Gemini Flash Image
	mergeProvider("antigravity", "Google Antigravity", KindImage,
		[]ModelEntry{
			{ID: "gemini-3.1-flash-image", Name: "Gemini 3.1 Flash Image", Kind: KindImage},
			{ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "antigravity",
			Kind:       KindImage,
			BaseURL:    provider.AntigravityBaseURL,
			AuthType:   "oauth",
			AuthHeader: "bearer",
			Format:     "antigravity",
		},
	)

	// Stability AI
	mergeProvider("stability-ai", "Stability AI", KindImage,
		[]ModelEntry{
			{ID: "stable-image-ultra", Name: "Stable Image Ultra", Kind: KindImage},
			{ID: "stable-image-core", Name: "Stable Image Core", Kind: KindImage},
			{ID: "sd3.5-large", Name: "Stable Diffusion 3.5 Large", Kind: KindImage},
			{ID: "sd3.5-large-turbo", Name: "Stable Diffusion 3.5 Large Turbo", Kind: KindImage},
			{ID: "sd3.5-medium", Name: "Stable Diffusion 3.5 Medium", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "stability-ai",
			Kind:       KindImage,
			BaseURL:    "https://api.stability.ai/v2beta/stable-image/generate",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "stability",
		},
	)
	// Stability AI 官方旗舰已就绪
}
