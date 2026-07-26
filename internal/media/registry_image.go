package media

func registerImageProviders() {
	openaiCompat := []struct {
		id, name, baseURL string
	}{
		{"openai", "OpenAI", "https://api.openai.com/v1/images/generations"},
		{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1/images/generations"},
		{"minimax", "MiniMax", "https://api.minimax.chat/v1/images/generations"},
		{"recraft", "Recraft", "https://external.api.recraft.ai/v1/images/generations"},
		{"vercel-ai-gateway", "Vercel AI Gateway", "https://ai-gateway.vercel.sh/v1/images/generations"},
		{"xai", "xAI", "https://api.x.ai/v1/images/generations"},
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

	// Stability AI
	mergeProvider("stability-ai", "Stability AI", KindImage,
		[]ModelEntry{
			{ID: "stable-diffusion-xl-1024-v1-0", Name: "SDXL 1.0", Kind: KindImage},
			{ID: "sd3-medium", Name: "SD3 Medium", Kind: KindImage},
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

	// Black Forest Labs (FLUX)
	mergeProvider("black-forest-labs", "Black Forest Labs", KindImage,
		[]ModelEntry{
			{ID: "flux-pro", Name: "FLUX Pro", Kind: KindImage},
			{ID: "flux-dev", Name: "FLUX Dev", Kind: KindImage},
			{ID: "flux-schnell", Name: "FLUX Schnell", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "black-forest-labs",
			Kind:       KindImage,
			BaseURL:    "https://api.bfl.ml/v1",
			AuthType:   "apikey",
			AuthHeader: "x-api-key",
			Format:     "bfl",
		},
	)

	// Fal.ai
	mergeProvider("fal-ai", "Fal.ai", KindImage, nil, ProviderConfig{
		Provider:   "fal-ai",
		Kind:       KindImage,
		BaseURL:    "https://fal.run",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "fal",
	})

	// HuggingFace Inference
	mergeProvider("huggingface", "HuggingFace", KindImage, nil, ProviderConfig{
		Provider:   "huggingface",
		Kind:       KindImage,
		BaseURL:    "https://api-inference.huggingface.co/models",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "huggingface",
	})

	// RunwayML
	mergeProvider("runwayml", "RunwayML", KindImage, nil, ProviderConfig{
		Provider:   "runwayml",
		Kind:       KindImage,
		BaseURL:    "https://api.dev.runwayml.com/v1",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "runway",
	})
}
