package media

import "github.com/arisvia/cyrene-gateway/internal/provider"

func registerImageProviders() {
	// OpenAI official DALL·E models
	mergeProvider("openai", "OpenAI", KindImage,
		[]ModelEntry{
			{ID: "dall-e-3", Name: "DALL·E 3", Kind: KindImage},
			{ID: "dall-e-2", Name: "DALL·E 2", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "openai",
			Kind:       KindImage,
			BaseURL:    "https://api.openai.com/v1/images/generations",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// MiniMax Image
	mergeProvider("minimax", "MiniMax", KindImage,
		[]ModelEntry{
			{ID: "image-01", Name: "MiniMax Image-01", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "minimax",
			Kind:       KindImage,
			BaseURL:    "https://api.minimax.chat/v1/images/generations",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// OpenRouter image generation
	mergeProvider("openrouter", "OpenRouter", KindImage,
		[]ModelEntry{
			{ID: "black-forest-labs/flux-1.1-pro", Name: "FLUX 1.1 Pro", Kind: KindImage},
			{ID: "black-forest-labs/flux-1-dev", Name: "FLUX 1 Dev", Kind: KindImage},
			{ID: "black-forest-labs/flux-1-schnell", Name: "FLUX 1 Schnell", Kind: KindImage},
			{ID: "stabilityai/stable-diffusion-3.5-large", Name: "Stable Diffusion 3.5 Large", Kind: KindImage},
			{ID: "google/gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "openrouter",
			Kind:       KindImage,
			BaseURL:    "https://openrouter.ai/api/v1/images/generations",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// Gemini image generation (Google official Imagen 3 & Gemini Flash)
	mergeProvider("gemini", "Gemini", KindImage,
		[]ModelEntry{
			{ID: "imagen-3.0-generate-002", Name: "Google Imagen 3", Kind: KindImage},
			{ID: "imagen-3.0-fast-generate-001", Name: "Google Imagen 3 Fast", Kind: KindImage},
			{ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
			{ID: "gemini-nano-banana-2.1", Name: "Gemini Nano Banana 2.1 (Sandbox)", Kind: KindImage},
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

	// Antigravity (Google Code Assist) image generation via Imagen 3 & Gemini Flash
	mergeProvider("antigravity", "Antigravity", KindImage,
		[]ModelEntry{
			{ID: "imagen-3.0-generate-002", Name: "Google Imagen 3", Kind: KindImage},
			{ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
			{ID: "gemini-nano-banana-2.1", Name: "Gemini Nano Banana 2.1 (Sandbox)", Kind: KindImage},
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

	// Stability AI official lineup
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
}
